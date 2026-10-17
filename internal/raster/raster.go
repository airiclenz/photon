package raster

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"unicode/utf8"

	"github.com/airiclenz/photon/internal/config"
	"github.com/airiclenz/photon/internal/fonts"
	"github.com/airiclenz/photon/internal/take"
)

// faintPercent is the share of the foreground a faint cell keeps over its background.
const faintPercent = 60

// fullPercent is the whole of a blend: percent of one colour plus fullPercent−percent of the
// other. blendRounding rounds the integer mix to the nearest value.
const (
	fullPercent   = 100
	blendRounding = fullPercent / 2
)

// underlineStrokeDivisor sets an underline's thickness: an eighth of the cell width, at least
// one pixel.
const underlineStrokeDivisor = 8

// Geometry is what a frame is laid out from: the terminal's Cols × Rows, the Padding in pixels
// around the grid, the FontSize in pixels per em and the LineHeight as a multiple of it. The pixel
// size follows from these and the face's advance — see [Rasterizer.Size].
type Geometry struct {
	Cols       int
	Rows       int
	Padding    int
	FontSize   float64
	LineHeight float64
}

// Rasterizer paints take snapshots into RGBA frames. It is safe for concurrent use.
type Rasterizer struct {
	geometry Geometry
	palette  palette
	glyphs   *glyphs
}

// New builds a rasterizer over faces, drawing in theme's colours at geometry. A geometry with no
// cells, a negative padding or a non-positive font size or line height is an error, as is a theme
// whose ANSI palette is not exactly 16 colours; the theme's colours are taken as [config.Load]
// validated them.
func New(faces fonts.Set, theme config.Theme, geometry Geometry) (*Rasterizer, error) {
	if geometry.Cols <= 0 || geometry.Rows <= 0 || geometry.Padding < 0 ||
		geometry.FontSize <= 0 || geometry.LineHeight <= 0 {
		return nil, fmt.Errorf("raster: invalid geometry %+v", geometry)
	}
	colours, err := newPalette(theme)
	if err != nil {
		return nil, err
	}
	chain, err := newGlyphs(faces, geometry.FontSize, geometry.LineHeight)
	if err != nil {
		return nil, err
	}
	return &Rasterizer{geometry: geometry, palette: colours, glyphs: chain}, nil
}

// CellWidth is the pixel width of one terminal column.
func (r *Rasterizer) CellWidth() int { return r.glyphs.cellWidth }

// LineHeight is the pixel height of one terminal row.
func (r *Rasterizer) LineHeight() int { return r.glyphs.lineHeight }

// Size is the frame's pixel size: the grid plus the padding on every side.
func (r *Rasterizer) Size() image.Point {
	return image.Pt(
		2*r.geometry.Padding+r.geometry.Cols*r.CellWidth(),
		2*r.geometry.Padding+r.geometry.Rows*r.LineHeight(),
	)
}

// Resolves reports whether ru draws as something: a glyph in Source Code Pro or one of its
// fallbacks.
func (r *Rasterizer) Resolves(ru rune) bool {
	return r.glyphs.resolves(ru)
}

// Frame paints one snapshot. Rows and columns past the geometry are ignored; a short grid leaves
// the rest of the frame on the theme background.
func (r *Rasterizer) Frame(s take.Snapshot) *image.RGBA {
	img := image.NewRGBA(image.Rectangle{Max: r.Size()})
	draw.Draw(img, img.Bounds(), &image.Uniform{C: r.palette.bg}, image.Point{}, draw.Src)
	for y, row := range s.Cells {
		if y >= r.geometry.Rows {
			break
		}
		for x, cell := range row {
			if x >= r.geometry.Cols {
				break
			}
			// Width 0 is a column a wide grapheme covers, already painted from its first.
			if cell.Width == 0 {
				continue
			}
			r.drawCell(img, x, y, cell)
		}
	}
	return img
}

// drawCell paints one cell: its background across its whole span, then its glyph and underline
// in its foreground.
func (r *Rasterizer) drawCell(img *image.RGBA, x, y int, cell take.Cell) {
	cellWidth, lineHeight := r.CellWidth(), r.LineHeight()
	columns := min(max(cell.Width, 1), r.geometry.Cols-x)
	span := columns * cellWidth
	origin := image.Pt(r.geometry.Padding+x*cellWidth, r.geometry.Padding+y*lineHeight)
	rect := image.Rectangle{Min: origin, Max: origin.Add(image.Pt(span, lineHeight))}

	fg, bg := r.palette.cellColors(cell)
	draw.Draw(img, rect, &image.Uniform{C: bg}, image.Point{}, draw.Src)
	ink := &image.Uniform{C: fg}
	if ru, _ := utf8.DecodeRuneInString(cell.Grapheme); ru != utf8.RuneError && ru != ' ' {
		if mask := r.glyphs.mask(ru, styleOf(cell), span); mask != nil {
			draw.DrawMask(img, rect, ink, image.Point{}, mask, image.Point{}, draw.Over)
		}
	}
	if cell.Underline {
		thickness := underlineThickness(cellWidth)
		top := origin.Y + min(r.glyphs.baseline+thickness, lineHeight-thickness)
		line := image.Rect(rect.Min.X, top, rect.Max.X, top+thickness)
		draw.Draw(img, line, ink, image.Point{}, draw.Src)
	}
}

// underlineThickness is an underline's thickness in a cell cellWidth pixels wide.
func underlineThickness(cellWidth int) int {
	return max(1, int(math.Round(float64(cellWidth)/underlineStrokeDivisor)))
}

// styleOf picks the Source Code Pro file a cell draws with.
func styleOf(cell take.Cell) fontStyle {
	switch {
	case cell.Bold:
		return styleBold
	case cell.Italic:
		return styleItalic
	}
	return styleRegular
}

// The xterm 256-colour palette past the sixteen a theme sets: a 6×6×6 colour cube from index 16,
// whose channel level n is 0 or 55+40n, then a grey ramp from index 232 at 8+10n.
const (
	ansiColors    = 16
	cubeFirst     = 16
	cubeSide      = 6
	cubeLevelBase = 55
	cubeLevelStep = 40
	greyFirst     = 232
	greyLevelBase = 8
	greyLevelStep = 10
)

// opaque is the alpha of every colour a frame paints.
const opaque = 0xff

// palette is a theme resolved to RGBA: the default foreground and background and the sixteen ANSI
// colours.
type palette struct {
	fg   color.RGBA
	bg   color.RGBA
	ansi [ansiColors]color.RGBA
}

// newPalette resolves theme; an ANSI palette that is not exactly 16 colours is an error.
func newPalette(theme config.Theme) (palette, error) {
	if len(theme.ANSI) != ansiColors {
		return palette{}, fmt.Errorf("raster: theme has %d ANSI colours, want %d", len(theme.ANSI), ansiColors)
	}
	p := palette{fg: theme.FG.RGBA(), bg: theme.BG.RGBA()}
	for i, c := range theme.ANSI {
		p.ansi[i] = c.RGBA()
	}
	return p, nil
}

// cellColors resolves a cell's foreground and background, then applies reverse (a swap) and
// faint (the foreground at faintPercent over the background).
func (p palette) cellColors(cell take.Cell) (fg, bg color.RGBA) {
	fg = p.resolve(cell.FG, p.fg)
	bg = p.resolve(cell.BG, p.bg)
	if cell.Reverse {
		fg, bg = bg, fg
	}
	if cell.Faint {
		fg = blend(bg, fg, faintPercent)
	}
	return fg, bg
}

// resolve turns a take colour into RGBA: the terminal default to fallback, palette 0–15 to the
// theme's ANSI colours, 16–255 through the xterm palette, truecolor as itself.
func (p palette) resolve(c take.Color, fallback color.RGBA) color.RGBA {
	switch c.Kind {
	case take.ColorPalette:
		if int(c.Index) < ansiColors {
			return p.ansi[c.Index]
		}
		return xtermColor(c.Index)
	case take.ColorRGB:
		return color.RGBA{R: c.R, G: c.G, B: c.B, A: opaque}
	}
	return fallback
}

// xtermColor is entry index (16–255) of the xterm palette.
func xtermColor(index uint8) color.RGBA {
	if index >= greyFirst {
		level := uint8(greyLevelBase + greyLevelStep*int(index-greyFirst))
		return color.RGBA{R: level, G: level, B: level, A: opaque}
	}
	offset := int(index - cubeFirst)
	return color.RGBA{
		R: cubeLevel(offset / (cubeSide * cubeSide)),
		G: cubeLevel(offset / cubeSide % cubeSide),
		B: cubeLevel(offset % cubeSide),
		A: opaque,
	}
}

// cubeLevel is the channel value of colour-cube step n (0–5).
func cubeLevel(n int) uint8 {
	if n == 0 {
		return 0
	}
	return uint8(cubeLevelBase + cubeLevelStep*n)
}

// blend mixes percent of over into under, channel by channel, in integers so every platform
// paints the same bytes.
func blend(under, over color.RGBA, percent int) color.RGBA {
	mix := func(a, b uint8) uint8 {
		return uint8((int(b)*percent + int(a)*(fullPercent-percent) + blendRounding) / fullPercent)
	}
	return color.RGBA{R: mix(under.R, over.R), G: mix(under.G, over.G), B: mix(under.B, over.B), A: opaque}
}
