package raster

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"math"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/airiclenz/photon/internal/fonts"
)

// cellWidthProbe is the rune whose advance sets the cell width; every Source Code Pro glyph
// shares it.
const cellWidthProbe = 'M'

// pixelsPerPoint makes a face's Size a size in pixels: at 72 DPI one point is one pixel.
const pixelsPerPoint = 72

// fixedOne is one pixel in 26.6 fixed point.
const fixedOne = 64

// fontStyle picks one of Source Code Pro's three files. Bold italic has no file of its own and
// draws bold.
type fontStyle int

const (
	styleRegular fontStyle = iota
	styleBold
	styleItalic
	styleCount
)

// glyphKey identifies one cached glyph mask: the rune, the style it is drawn in and how many
// pixels wide its cell span is.
type glyphKey struct {
	r     rune
	style fontStyle
	span  int
}

// glyphs is the face chain at one font size: the cell metrics it sets and the glyph masks it
// draws, cached. It is safe for concurrent use: the faces and the cache are guarded by one mutex,
// because an opentype face reuses its buffers between calls.
type glyphs struct {
	size       float64
	cellWidth  int
	lineHeight int
	baseline   int

	// primary is Source Code Pro indexed by style; fallbacks is the chain tried after it.
	primary   [styleCount]*sfnt.Font
	fallbacks []*sfnt.Font

	mu     sync.Mutex
	buffer sfnt.Buffer
	faces  map[*sfnt.Font]font.Face
	masks  map[glyphKey]*image.Alpha
}

// newGlyphs builds the face chain from set at size pixels per em, and derives the cell metrics: the
// cell width from the probe's advance, the line height as size·lineHeight rounded, and a baseline
// that centres the face's ascent and descent in the line.
func newGlyphs(set fonts.Set, size, lineHeight float64) (*glyphs, error) {
	g := &glyphs{
		size:    size,
		primary: [styleCount]*sfnt.Font{set.Regular.Font, set.Bold.Font, set.Italic.Font},
		faces:   map[*sfnt.Font]font.Face{},
		masks:   map[glyphKey]*image.Alpha{},
	}
	for _, fallback := range set.Fallbacks {
		g.fallbacks = append(g.fallbacks, fallback.Font)
	}
	for _, f := range append(g.primary[:], g.fallbacks...) {
		if f == nil {
			return nil, errors.New("raster: the face set is missing a face")
		}
		face, err := newFace(f, size)
		if err != nil {
			return nil, err
		}
		g.faces[f] = face
	}

	regular := g.faces[g.primary[styleRegular]]
	advance, ok := regular.GlyphAdvance(cellWidthProbe)
	if !ok {
		return nil, fmt.Errorf("raster: %s has no advance for %q", set.Regular.Name, cellWidthProbe)
	}
	g.cellWidth = int(math.Round(fixedToFloat(advance)))
	g.lineHeight = int(math.Round(size * lineHeight))
	metrics := regular.Metrics()
	ascent, descent := fixedToFloat(metrics.Ascent), fixedToFloat(metrics.Descent)
	g.baseline = int(math.Round((float64(g.lineHeight)-(ascent+descent))/2 + ascent))
	return g, nil
}

// newFace returns f's face at size pixels per em.
func newFace(f *sfnt.Font, size float64) (font.Face, error) {
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     pixelsPerPoint,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil, fmt.Errorf("raster: face at size %g: %w", size, err)
	}
	return face, nil
}

// resolves reports whether ru draws as something: procedurally, or from a face in the chain.
func (g *glyphs) resolves(ru rune) bool {
	if isProcedural(ru) {
		return true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.fontForLocked(ru, styleRegular) != nil
}

// mask returns the coverage mask for ru drawn in style, centred in a cell span pixels wide and
// one line high, or nil when no face draws it. A box-drawing or block-element rune is drawn
// procedurally across the whole span, in one weight whatever the style. Masks are cached.
func (g *glyphs) mask(ru rune, style fontStyle, span int) *image.Alpha {
	procedural := isProcedural(ru)
	if procedural {
		style = styleRegular
	}
	key := glyphKey{r: ru, style: style, span: span}
	g.mu.Lock()
	defer g.mu.Unlock()
	if mask, ok := g.masks[key]; ok {
		return mask
	}
	var mask *image.Alpha
	if procedural {
		mask = proceduralGlyph(ru, span, g.lineHeight, lightStroke(g.cellWidth))
	} else {
		mask = g.drawLocked(ru, style, span)
	}
	g.masks[key] = mask
	return mask
}

// drawLocked draws ru from the first face in the chain that has it, centred in the span. A
// glyph wider than the span (a fallback symbol drawn in a proportional face) is drawn from a face
// scaled down to fit it.
func (g *glyphs) drawLocked(ru rune, style fontStyle, span int) *image.Alpha {
	f := g.fontForLocked(ru, style)
	if f == nil {
		return nil
	}
	face := g.faces[f]
	advance, ok := face.GlyphAdvance(ru)
	if !ok {
		return nil
	}
	if width := fixedToFloat(advance); width > float64(span) {
		// The scaled face is used for this one mask, which the cache then keeps. An opentype face
		// holds no resource beyond memory, so it needs no Close.
		scaled, err := newFace(f, g.size*float64(span)/width)
		if err == nil {
			face = scaled
			advance, _ = face.GlyphAdvance(ru)
		}
	}
	mask := image.NewAlpha(image.Rect(0, 0, span, g.lineHeight))
	dot := fixed.Point26_6{X: (fixed.I(span) - advance) / 2, Y: fixed.I(g.baseline)}
	bounds, glyphMask, maskPoint, _, ok := face.Glyph(dot, ru)
	if !ok {
		return nil
	}
	draw.DrawMask(mask, bounds, image.Opaque, image.Point{}, glyphMask, maskPoint, draw.Over)
	return mask
}

// fontForLocked walks the chain for ru from style's face; nil means no face has it.
func (g *glyphs) fontForLocked(ru rune, style fontStyle) *sfnt.Font {
	for _, f := range g.chain(style) {
		index, err := f.GlyphIndex(&g.buffer, ru)
		if err == nil && index != 0 {
			return f
		}
	}
	return nil
}

// chain is the order faces are tried in for a style: the style's Source Code Pro file, regular
// Source Code Pro, then the fallbacks in order.
func (g *glyphs) chain(style fontStyle) []*sfnt.Font {
	chain := make([]*sfnt.Font, 0, 2+len(g.fallbacks))
	chain = append(chain, g.primary[style], g.primary[styleRegular])
	return append(chain, g.fallbacks...)
}

// fixedToFloat converts a 26.6 fixed-point value to pixels.
func fixedToFloat(v fixed.Int26_6) float64 { return float64(v) / fixedOne }
