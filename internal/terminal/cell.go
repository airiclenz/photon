//go:build !windows

package terminal

import (
	"image/color"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/airiclenz/photon/internal/take"
)

// blankGrapheme is what a cell the emulator has no record of shows.
const blankGrapheme = " "

// rgbShift turns a 16-bit colour channel, as [color.Color.RGBA] reports it, into 8 bits.
const rgbShift = 8

// gridOf reads a whole cols × rows grid through cellAt, one slice per row.
func gridOf(cols, rows int, cellAt func(x, y int) *uv.Cell) [][]take.Cell {
	grid := make([][]take.Cell, rows)
	for y := range grid {
		row := make([]take.Cell, cols)
		for x := range row {
			row[x] = cellOf(cellAt(x, y))
		}
		grid[y] = row
	}
	return grid
}

// cellOf reads one emulator cell as a take cell. A nil cell is a blank one column wide.
func cellOf(cell *uv.Cell) take.Cell {
	if cell == nil {
		return take.Cell{Grapheme: blankGrapheme, Width: 1}
	}
	attrs := cell.Style.Attrs
	return take.Cell{
		Grapheme:  cell.Content,
		Width:     cell.Width,
		FG:        colorOf(cell.Style.Fg),
		BG:        colorOf(cell.Style.Bg),
		Bold:      attrs&uv.AttrBold != 0,
		Faint:     attrs&uv.AttrFaint != 0,
		Italic:    attrs&uv.AttrItalic != 0,
		Underline: cell.Style.Underline != 0,
		Reverse:   attrs&uv.AttrReverse != 0,
	}
}

// colorOf reads an emulator colour as the program asked for it: nil is the terminal's default, a
// basic or indexed colour stays a palette index for the theme to resolve, and anything else is
// its truecolor value.
func colorOf(c color.Color) take.Color {
	switch value := c.(type) {
	case nil:
		return take.Color{}
	case ansi.BasicColor:
		return take.Color{Kind: take.ColorPalette, Index: uint8(value)}
	case ansi.IndexedColor:
		return take.Color{Kind: take.ColorPalette, Index: uint8(value)}
	}
	r, g, b, _ := c.RGBA()
	return take.Color{
		Kind: take.ColorRGB,
		R:    uint8(r >> rgbShift), G: uint8(g >> rgbShift), B: uint8(b >> rgbShift),
	}
}
