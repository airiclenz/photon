package caption

import (
	"bytes"
	"flag"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// update rewrites the golden images instead of comparing against them:
// `go test ./internal/caption/ -run Golden -update`.
var update = flag.Bool("update", false, "rewrite the golden images under testdata")

// goldenTolerance is how far one channel of a pixel may stray from its golden. Glyph edges are
// anti-aliased in floating point, which differs in the last bits between architectures; a wrong
// colour, a missing glyph or a shifted one is far past it.
const goldenTolerance = 16

// wrappedCaption is a caption whose first line is too long for the test frame and wraps, centred
// in a box, above a hard line break.
const wrappedCaption = "  - {text: \"A caption long enough to wrap onto more than one line of the frame," +
	" each line centred in its box\\nthen a hard break\", at: 0s, for: 1s," +
	" font: {size: 28}, place: middle, align: middle, box: {color: '#203040', opacity: 1}}\n"

func TestDraw_WrappedCaption_MatchesItsGolden(t *testing.T) {
	layer := newTestLayer(t, wrappedCaption, testOut, testUnit)

	frame := drawAt(layer, testOut, 0)

	path := filepath.Join("testdata", "wrapped.png")
	if *update {
		writeGolden(t, path, frame)
		return
	}
	assertMatchesGolden(t, frame, readGolden(t, path))
}

// styledCaption is a caption whose coloured bold span is too long for its line and wraps across
// lines, between plain words and beside a coloured regular span.
const styledCaption = "  - {text: \"Plain words, then [bold #ffc040]a bold coloured span long enough to" +
	" wrap across lines[/] and [#80e0ff]a regular one[/]\", at: 0s, for: 1s," +
	" font: {size: 28}, place: middle, align: middle, box: {color: '#203040', opacity: 1}}\n"

func TestDraw_StyledSpan_MatchesItsGolden(t *testing.T) {
	layer := newTestLayer(t, styledCaption, testOut, testUnit)

	frame := drawAt(layer, testOut, 0)

	path := filepath.Join("testdata", "styled.png")
	if *update {
		writeGolden(t, path, frame)
		return
	}
	assertMatchesGolden(t, frame, readGolden(t, path))
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
