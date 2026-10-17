package raster

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"path/filepath"
	"testing"

	"github.com/airiclenz/photon/internal/take"
)

// boxGoldenName is the golden for boxGoldenCells under testdata.
const boxGoldenName = "box.png"

// boxGoldenCells is a fixed grid of light, heavy, mixed and double lines, dashes, blocks, eighths,
// shades and quadrants, with a few coloured, reversed and faint cells. Every pixel of it is an
// axis-aligned integer fill, so its golden is pinned byte for byte.
func boxGoldenCells() [][]take.Cell {
	palette := func(index uint8) take.Color { return take.Color{Kind: take.ColorPalette, Index: index} }
	plain := take.Cell{}
	return [][]take.Cell{
		row(segment{"┌──┬──┐ ┏━━┓ ╔══╗ ▀▄", plain}),
		row(segment{"│  ┼  │ ┃┝┶┃ ╠╬╦╣ ▌▐", plain}),
		row(segment{"└──┴──┘ ┗━━┛ ╚══╝ ░▒", plain}),
		row(
			segment{"▁", take.Cell{Faint: true}}, segment{"▂", take.Cell{FG: palette(2)}},
			segment{"▃▄▅▆▇█▉▊▋▌▍▎▏▔▕", plain},
			segment{"█", take.Cell{FG: palette(3), Reverse: true}}, segment{"┊┅", plain},
		),
		row(
			segment{"▖▗▘▙▚▛▜▝▞▟", take.Cell{FG: palette(4)}},
			segment{"▓╞╪╡╓╥╖ ╌┇", plain},
		),
	}
}

func TestFrame_MatchesBoxGolden(t *testing.T) {
	t.Parallel()
	cells := boxGoldenCells()
	r := newTestRasterizer(t, defaultTheme(), len(cells[0]), len(cells))

	img := r.Frame(take.Snapshot{Cells: cells})

	path := filepath.Join("testdata", boxGoldenName)
	if *update {
		writeGolden(t, path, img)
		return
	}
	golden := readGolden(t, path)
	if img.Bounds() != golden.Bounds() {
		t.Fatalf("frame bounds %v, golden %v", img.Bounds(), golden.Bounds())
	}
	want := image.NewRGBA(golden.Bounds())
	draw.Draw(want, want.Rect, golden, golden.Bounds().Min, draw.Src)
	if !bytes.Equal(img.Pix, want.Pix) {
		t.Fatalf("frame differs from %s (rewrite with -update)", path)
	}
}

func TestFrame_InksEveryProceduralRune(t *testing.T) {
	t.Parallel()
	theme := defaultTheme()
	r := newTestRasterizer(t, theme, 1, 1)

	for ru := rune(boxDrawingFirst); ru <= blockElementsLast; ru++ {
		img := r.Frame(take.Snapshot{Cells: [][]take.Cell{{{Grapheme: string(ru), Width: 1}}}})

		if inkedRows(img, cellRect(r, 0, 0, 1), theme.BG.RGBA()) == 0 {
			t.Errorf("%c (%U) painted nothing", ru, ru)
		}
	}
}

// singleCellFrame draws grapheme alone in a one-cell frame and returns it with the cell's rect.
func singleCellFrame(t *testing.T, grapheme string) (*Rasterizer, *image.RGBA, image.Rectangle) {
	t.Helper()
	r := newTestRasterizer(t, defaultTheme(), 1, 1)
	img := r.Frame(take.Snapshot{Cells: [][]take.Cell{{{Grapheme: grapheme, Width: 1}}}})
	return r, img, cellRect(r, 0, 0, 1)
}

// assertInked fails unless every point of points is off the background.
func assertInked(t *testing.T, img *image.RGBA, points []image.Point, label string) {
	t.Helper()
	background := defaultTheme().BG.RGBA()
	for _, p := range points {
		if img.RGBAAt(p.X, p.Y) == background {
			t.Errorf("%s has no ink at %v", label, p)
		}
	}
}

// assertBlank fails unless every point of points is the background.
func assertBlank(t *testing.T, img *image.RGBA, points []image.Point, label string) {
	t.Helper()
	background := defaultTheme().BG.RGBA()
	for _, p := range points {
		if got := img.RGBAAt(p.X, p.Y); got != background {
			t.Errorf("%s inked %v: %v", label, p, got)
		}
	}
}

// TestFrame_DrawsRoundedCornersToTheirEdges checks each rounded corner by coverage rather than
// against a golden, since arc anti-aliasing differs between architectures: it leaves the cell on
// the light line's column and row at the two edges it joins, and leaves the far corner empty.
func TestFrame_DrawsRoundedCornersToTheirEdges(t *testing.T) {
	t.Parallel()
	tests := []struct {
		grapheme              string
		joinsRight, joinsDown bool
	}{
		{"╭", true, true},
		{"╮", false, true},
		{"╯", false, false},
		{"╰", true, false},
	}
	for _, tc := range tests {
		t.Run(tc.grapheme, func(t *testing.T) {
			t.Parallel()
			r, img, rect := singleCellFrame(t, tc.grapheme)

			stroke := lightStroke(r.CellWidth())
			lineX := rect.Min.X + (r.CellWidth()-stroke)/2
			lineY := rect.Min.Y + (r.LineHeight()-stroke)/2
			sideX, farX := rect.Min.X, rect.Max.X-1
			if tc.joinsRight {
				sideX, farX = farX, sideX
			}
			edgeY, farY := rect.Min.Y, rect.Max.Y-1
			if tc.joinsDown {
				edgeY, farY = farY, edgeY
			}
			assertInked(t, img, []image.Point{{lineX, edgeY}, {sideX, lineY}}, tc.grapheme)
			assertBlank(t, img, []image.Point{{farX, farY}}, tc.grapheme)
		})
	}
}

// TestFrame_DrawsDiagonalsCornerToCorner checks the diagonals by coverage, as the rounded corners
// are: ink in each corner the line reaches, none in the corners it does not, nor at the middle of
// the top and left edges.
func TestFrame_DrawsDiagonalsCornerToCorner(t *testing.T) {
	t.Parallel()
	const (
		topLeft = iota
		topRight
		bottomLeft
		bottomRight
	)
	tests := []struct {
		grapheme string
		inked    []int
		blank    []int
	}{
		{"╱", []int{bottomLeft, topRight}, []int{topLeft, bottomRight}},
		{"╲", []int{topLeft, bottomRight}, []int{topRight, bottomLeft}},
		{"╳", []int{topLeft, topRight, bottomLeft, bottomRight}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.grapheme, func(t *testing.T) {
			t.Parallel()
			_, img, rect := singleCellFrame(t, tc.grapheme)

			corners := [...]image.Point{
				topLeft:     rect.Min,
				topRight:    {rect.Max.X - 1, rect.Min.Y},
				bottomLeft:  {rect.Min.X, rect.Max.Y - 1},
				bottomRight: {rect.Max.X - 1, rect.Max.Y - 1},
			}
			pick := func(which []int) []image.Point {
				points := make([]image.Point, 0, len(which))
				for _, corner := range which {
					points = append(points, corners[corner])
				}
				return points
			}
			edgeMiddles := []image.Point{
				{(rect.Min.X + rect.Max.X) / 2, rect.Min.Y},
				{rect.Min.X, (rect.Min.Y + rect.Max.Y) / 2},
			}
			assertInked(t, img, pick(tc.inked), tc.grapheme)
			assertBlank(t, img, append(pick(tc.blank), edgeMiddles...), tc.grapheme)
		})
	}
}

func TestResolves_ProceduralAndMathRunes(t *testing.T) {
	t.Parallel()
	r := newTestRasterizer(t, defaultTheme(), 1, 1)

	for _, ru := range []rune{'─', '╬', '╭', '╳', '▀', '░', '▟', '𝔸'} {
		t.Run(fmt.Sprintf("%U", ru), func(t *testing.T) {
			t.Parallel()

			if !r.Resolves(ru) {
				t.Fatalf("Resolves(%c) = false, want true", ru)
			}
		})
	}
}
