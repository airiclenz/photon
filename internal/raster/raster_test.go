package raster

import (
	"bytes"
	"flag"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/airiclenz/photon/internal/config"
	"github.com/airiclenz/photon/internal/fonts"
	"github.com/airiclenz/photon/internal/take"
)

// update rewrites the golden images instead of comparing against them:
// `go test ./internal/raster/ -run Golden -update`.
var update = flag.Bool("update", false, "rewrite the golden images under testdata")

// goldenTolerance is how far one channel of a pixel may stray from its golden. Glyph edges are
// anti-aliased in floating point, which differs in the last bits between architectures; a wrong
// colour, a missing glyph or a shifted one is far past it.
const goldenTolerance = 16

// loadFaces parses the embedded faces once for every test; a parsed set is read-only.
var loadFaces = sync.OnceValues(func() (fonts.Set, error) { return fonts.Load("") })

// testGeometry is the small, fixed geometry the tests draw at: Source Code Pro at 15 px with a
// 1.2 line height makes 9 × 18 pixel cells.
func testGeometry(cols, rows int) Geometry {
	return Geometry{Cols: cols, Rows: rows, Padding: 8, FontSize: 15, LineHeight: 1.2}
}

// defaultTheme is the theme [config.Load] fills in when a project sets none.
func defaultTheme() config.Theme {
	return config.Theme{FG: config.DefaultForeground, BG: config.DefaultBackground, ANSI: config.DefaultANSI[:]}
}

// newTestRasterizer builds a rasterizer over the embedded faces and theme at testGeometry.
func newTestRasterizer(t *testing.T, theme config.Theme, cols, rows int) *Rasterizer {
	t.Helper()
	faces, err := loadFaces()
	if err != nil {
		t.Fatalf("fonts.Load: %v", err)
	}
	r, err := New(faces, theme, testGeometry(cols, rows))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

// blankCells is a cols × rows grid of default spaces.
func blankCells(cols, rows int) [][]take.Cell {
	cells := make([][]take.Cell, rows)
	for y := range cells {
		cells[y] = make([]take.Cell, cols)
		for x := range cells[y] {
			cells[y][x] = take.Cell{Grapheme: " ", Width: 1}
		}
	}
	return cells
}

// segment is a run of text drawn in one cell style; the style's Grapheme and Width are ignored.
type segment struct {
	text  string
	style take.Cell
}

// row lays segments out one cell per rune.
func row(segments ...segment) []take.Cell {
	var cells []take.Cell
	for _, s := range segments {
		for _, ru := range s.text {
			cell := s.style
			cell.Grapheme, cell.Width = string(ru), 1
			cells = append(cells, cell)
		}
	}
	return cells
}

// cellRect is the pixel rectangle of cell (x, y) spanning columns columns.
func cellRect(r *Rasterizer, x, y, columns int) image.Rectangle {
	padding := testGeometry(0, 0).Padding
	origin := image.Pt(padding+x*r.CellWidth(), padding+y*r.LineHeight())
	return image.Rectangle{Min: origin, Max: origin.Add(image.Pt(columns*r.CellWidth(), r.LineHeight()))}
}

// assertFilled fails unless every pixel of rect is want.
func assertFilled(t *testing.T, img *image.RGBA, rect image.Rectangle, want color.RGBA) {
	t.Helper()
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if got := img.RGBAAt(x, y); got != want {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

// count counts the pixels of rect that are want.
func count(img *image.RGBA, rect image.Rectangle, want color.RGBA) int {
	matched := 0
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if img.RGBAAt(x, y) == want {
				matched++
			}
		}
	}
	return matched
}

// inkedRows counts the rows of rect holding a pixel other than background.
func inkedRows(img *image.RGBA, rect image.Rectangle, background color.RGBA) int {
	inked := 0
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if img.RGBAAt(x, y) != background {
				inked++
				break
			}
		}
	}
	return inked
}

func TestNew_DerivesCellMetricsFromTheFace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                    string
		geometry                Geometry
		wantCellWidth, wantLine int
	}{
		// Source Code Pro advances 600 of 1000 units per em.
		{"font 15", Geometry{Cols: 20, Rows: 4, Padding: 8, FontSize: 15, LineHeight: 1.2}, 9, 18},
		{"font 30", Geometry{Cols: 135, Rows: 44, Padding: 32, FontSize: 30, LineHeight: 1.0}, 18, 30},
		{"fractional line", Geometry{Cols: 10, Rows: 3, Padding: 0, FontSize: 20, LineHeight: 1.33}, 12, 27},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			faces, err := loadFaces()
			if err != nil {
				t.Fatalf("fonts.Load: %v", err)
			}

			r, err := New(faces, defaultTheme(), tc.geometry)

			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if r.CellWidth() != tc.wantCellWidth || r.LineHeight() != tc.wantLine {
				t.Fatalf("cell %d × %d, want %d × %d", r.CellWidth(), r.LineHeight(), tc.wantCellWidth, tc.wantLine)
			}
			g := tc.geometry
			want := image.Pt(2*g.Padding+g.Cols*tc.wantCellWidth, 2*g.Padding+g.Rows*tc.wantLine)
			if got := r.Size(); got != want {
				t.Fatalf("Size() = %v, want %v", got, want)
			}
			if got := r.Frame(take.Snapshot{}).Bounds().Size(); got != want {
				t.Fatalf("frame size = %v, want %v", got, want)
			}
		})
	}
}

func TestNew_RejectsInvalidInput(t *testing.T) {
	t.Parallel()
	faces, err := loadFaces()
	if err != nil {
		t.Fatalf("fonts.Load: %v", err)
	}
	shortTheme := defaultTheme()
	shortTheme.ANSI = shortTheme.ANSI[:3]
	tests := []struct {
		name     string
		faces    fonts.Set
		theme    config.Theme
		geometry Geometry
	}{
		{"no columns", faces, defaultTheme(), Geometry{Rows: 1, FontSize: 15, LineHeight: 1}},
		{"no rows", faces, defaultTheme(), Geometry{Cols: 1, FontSize: 15, LineHeight: 1}},
		{"negative padding", faces, defaultTheme(), Geometry{Cols: 1, Rows: 1, Padding: -1, FontSize: 15, LineHeight: 1}},
		{"no font size", faces, defaultTheme(), Geometry{Cols: 1, Rows: 1, LineHeight: 1}},
		{"no line height", faces, defaultTheme(), Geometry{Cols: 1, Rows: 1, FontSize: 15}},
		{"short ANSI palette", faces, shortTheme, testGeometry(1, 1)},
		{"no faces", fonts.Set{}, defaultTheme(), testGeometry(1, 1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := New(tc.faces, tc.theme, tc.geometry)

			if err == nil {
				t.Fatal("New succeeded")
			}
		})
	}
}

// TestFrame_ResolvesCellColours paints an underlined space, so the cell holds only its background
// and an underline in its foreground — both integer fills, the same on every platform.
func TestFrame_ResolvesCellColours(t *testing.T) {
	t.Parallel()
	theme := defaultTheme()
	theme.ANSI = append([]config.Color(nil), theme.ANSI...)
	theme.ANSI[1] = "#010203"
	fg, bg := theme.FG.RGBA(), theme.BG.RGBA()
	rgb := func(r, g, b uint8) color.RGBA { return color.RGBA{R: r, G: g, B: b, A: opaque} }
	palette := func(index uint8) take.Color { return take.Color{Kind: take.ColorPalette, Index: index} }
	truecolor := func(r, g, b uint8) take.Color { return take.Color{Kind: take.ColorRGB, R: r, G: g, B: b} }
	tests := []struct {
		name   string
		cell   take.Cell
		wantFG color.RGBA
		wantBG color.RGBA
	}{
		{"defaults are the theme's", take.Cell{}, fg, bg},
		{"palette 0-15 is the theme's", take.Cell{FG: palette(1), BG: palette(8)}, rgb(1, 2, 3), theme.ANSI[8].RGBA()},
		{"palette above 15 is xterm", take.Cell{FG: palette(196)}, rgb(0xff, 0, 0), bg},
		{"cube origin is black", take.Cell{FG: palette(16)}, rgb(0, 0, 0), bg},
		{"cube blue", take.Cell{BG: palette(21)}, fg, rgb(0, 0, 0xff)},
		{"cube mixed", take.Cell{FG: palette(110)}, rgb(0x87, 0xaf, 0xd7), bg},
		{"first grey", take.Cell{FG: palette(232)}, rgb(8, 8, 8), bg},
		{"last grey", take.Cell{FG: palette(255)}, rgb(0xee, 0xee, 0xee), bg},
		{"truecolor passes through", take.Cell{FG: truecolor(0x12, 0x34, 0x56)}, rgb(0x12, 0x34, 0x56), bg},
		{"reverse swaps", take.Cell{FG: palette(2), Reverse: true}, bg, theme.ANSI[2].RGBA()},
		{
			"faint is 60 percent over the background",
			take.Cell{FG: truecolor(0xff, 0xff, 0xff), BG: truecolor(0, 0, 0), Faint: true},
			rgb(0x99, 0x99, 0x99), rgb(0, 0, 0),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newTestRasterizer(t, theme, 1, 1)
			cell := tc.cell
			cell.Grapheme, cell.Width, cell.Underline = " ", 1, true

			img := r.Frame(take.Snapshot{Cells: [][]take.Cell{{cell}}})

			rect := cellRect(r, 0, 0, 1)
			if got := img.RGBAAt(rect.Min.X, rect.Min.Y); got != tc.wantBG {
				t.Errorf("background = %v, want %v", got, tc.wantBG)
			}
			if got := count(img, rect, tc.wantFG); got < r.CellWidth() {
				t.Errorf("%d pixels of the foreground %v, want an underline of at least %d", got, tc.wantFG, r.CellWidth())
			}
		})
	}
}

func TestFrame_WideCellSpansItsColumns(t *testing.T) {
	t.Parallel()
	theme := defaultTheme()
	r := newTestRasterizer(t, theme, 3, 1)
	cells := blankCells(3, 1)
	cells[0][0] = take.Cell{Grapheme: " ", Width: 2, BG: take.Color{Kind: take.ColorPalette, Index: 4}}
	cells[0][1] = take.Cell{Width: 0}

	img := r.Frame(take.Snapshot{Cells: cells})

	assertFilled(t, img, cellRect(r, 0, 0, 2), theme.ANSI[4].RGBA())
	assertFilled(t, img, cellRect(r, 2, 0, 1), theme.BG.RGBA())
}

func TestFrame_ClipsAGridLargerThanTheGeometry(t *testing.T) {
	t.Parallel()
	theme := defaultTheme()
	r := newTestRasterizer(t, theme, 2, 1)
	cells := blankCells(3, 2)
	red := take.Color{Kind: take.ColorRGB, R: 0xff}
	for y := range cells {
		for x := range cells[y] {
			cells[y][x].BG = red
		}
	}

	img := r.Frame(take.Snapshot{Cells: cells})

	if got, want := img.Bounds().Size(), r.Size(); got != want {
		t.Fatalf("frame size = %v, want %v", got, want)
	}
	assertFilled(t, img, cellRect(r, 0, 0, 2), color.RGBA{R: 0xff, A: opaque})
	bottomPadding := image.Rect(0, cellRect(r, 0, 0, 1).Max.Y, r.Size().X, r.Size().Y)
	assertFilled(t, img, bottomPadding, theme.BG.RGBA())
}

func TestResolves(t *testing.T) {
	t.Parallel()
	r := newTestRasterizer(t, defaultTheme(), 1, 1)
	tests := []struct {
		name string
		ru   rune
		want bool
	}{
		{"ascii from Source Code Pro", 'a', true},
		{"math rune from the math fallback", '𝔸', true},
		{"control caret from the last fallback", '⌃', true},
		{"symbol from the first fallback", '★', true},
		{"private use rune no face has", '\U0010FFFD', false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := r.Resolves(tc.ru); got != tc.want {
				t.Fatalf("Resolves(%U) = %v, want %v", tc.ru, got, tc.want)
			}
		})
	}
}

func TestFrame_DrawsOnlyRunesAFaceHas(t *testing.T) {
	t.Parallel()
	theme := defaultTheme()
	tests := []struct {
		name      string
		grapheme  string
		wantInked bool
	}{
		{"math rune through the fallback", "𝔸", true},
		{"rune no face has", "\U0010FFFD", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newTestRasterizer(t, theme, 1, 1)

			img := r.Frame(take.Snapshot{Cells: [][]take.Cell{{{Grapheme: tc.grapheme, Width: 1}}}})

			inked := inkedRows(img, cellRect(r, 0, 0, 1), theme.BG.RGBA()) > 0
			if inked != tc.wantInked {
				t.Fatalf("%q inked = %v, want %v", tc.grapheme, inked, tc.wantInked)
			}
		})
	}
}

// TestFrame_ScalesAnOverWideGlyphIntoItsCell draws ⨌, which Noto Sans Math advances three cells
// wide and draws a full line tall. Scaled down to one cell it keeps well under half the line.
func TestFrame_ScalesAnOverWideGlyphIntoItsCell(t *testing.T) {
	t.Parallel()
	theme := defaultTheme()
	r := newTestRasterizer(t, theme, 3, 1)
	cells := blankCells(3, 1)
	cells[0][1].Grapheme = "⨌"

	img := r.Frame(take.Snapshot{Cells: cells})

	rows := inkedRows(img, cellRect(r, 1, 0, 1), theme.BG.RGBA())
	if rows == 0 || rows > r.LineHeight()/2 {
		t.Fatalf("⨌ inks %d of %d rows, want some and at most half", rows, r.LineHeight())
	}
	assertFilled(t, img, cellRect(r, 0, 0, 1), theme.BG.RGBA())
	assertFilled(t, img, cellRect(r, 2, 0, 1), theme.BG.RGBA())
}

// goldenSnapshots are the text frames pinned under testdata, by golden file name.
func goldenSnapshots() map[string][][]take.Cell {
	palette := func(index uint8) take.Color { return take.Color{Kind: take.ColorPalette, Index: index} }
	truecolor := func(r, g, b uint8) take.Color { return take.Color{Kind: take.ColorRGB, R: r, G: g, B: b} }
	plain := take.Cell{}
	return map[string][][]take.Cell{
		"plain.png": {
			row(segment{"The quick brown fox", plain}),
			row(segment{"jumps over 0123456789", plain}),
			row(segment{"{}[]<>=+-*/ café ñ", plain}),
		},
		"coloured.png": {
			row(
				segment{"r", take.Cell{FG: palette(1)}}, segment{"g", take.Cell{FG: palette(2)}},
				segment{"y", take.Cell{FG: palette(3)}}, segment{"b", take.Cell{FG: palette(4)}},
				segment{"m", take.Cell{FG: palette(5)}}, segment{"c", take.Cell{FG: palette(6)}},
				segment{" bright", take.Cell{FG: palette(13)}},
			),
			row(
				segment{"xterm", take.Cell{FG: palette(196)}}, segment{" cube", take.Cell{FG: palette(110)}},
				segment{" grey", take.Cell{FG: palette(244)}},
			),
			row(
				segment{"true", take.Cell{FG: truecolor(0xfa, 0xb3, 0x87)}},
				segment{" on", take.Cell{FG: palette(0), BG: palette(7)}},
				segment{" blue ", take.Cell{BG: palette(4)}},
			),
		},
		"attributed.png": {
			row(segment{"bold", take.Cell{Bold: true}}, segment{" italic", take.Cell{Italic: true}}),
			row(segment{"both", take.Cell{Bold: true, Italic: true}}, segment{" faint", take.Cell{Faint: true}}),
			row(segment{"under", take.Cell{Underline: true}}, segment{" reverse", take.Cell{Reverse: true}}),
		},
	}
}

func TestFrame_MatchesTextGoldens(t *testing.T) {
	t.Parallel()
	for name, cells := range goldenSnapshots() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newTestRasterizer(t, defaultTheme(), 22, len(cells))

			img := r.Frame(take.Snapshot{Cells: cells})

			path := filepath.Join("testdata", name)
			if *update {
				writeGolden(t, path, img)
				return
			}
			assertMatchesGolden(t, img, readGolden(t, path))
		})
	}
}

// writeGolden saves img as the PNG golden at path.
func writeGolden(t *testing.T, path string, img *image.RGBA) {
	t.Helper()
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatalf("encode %s: %v", path, err)
	}
	if err := os.WriteFile(path, encoded.Bytes(), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// readGolden loads the PNG golden at path.
func readGolden(t *testing.T, path string) image.Image {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open golden (rewrite with -update): %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close %s: %v", path, err)
		}
	}()
	golden, err := png.Decode(file)
	if err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return golden
}

// assertMatchesGolden fails unless img has golden's size and every channel of every pixel is
// within goldenTolerance of it.
func assertMatchesGolden(t *testing.T, img *image.RGBA, golden image.Image) {
	t.Helper()
	if img.Bounds() != golden.Bounds() {
		t.Fatalf("frame bounds %v, golden %v", img.Bounds(), golden.Bounds())
	}
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			got := img.RGBAAt(x, y)
			want := color.RGBAModel.Convert(golden.At(x, y)).(color.RGBA)
			if !withinTolerance(got, want) {
				t.Fatalf("pixel (%d,%d) = %v, golden %v", x, y, got, want)
			}
		}
	}
}

// withinTolerance reports whether every channel of a is within goldenTolerance of b's.
func withinTolerance(a, b color.RGBA) bool {
	near := func(p, q uint8) bool { return max(p, q)-min(p, q) <= goldenTolerance }
	return near(a.R, b.R) && near(a.G, b.G) && near(a.B, b.B) && near(a.A, b.A)
}
