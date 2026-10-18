package caption

import (
	"bytes"
	"image"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/airiclenz/photon/internal/fonts"
	"github.com/airiclenz/photon/internal/storyboard"
)

// boardHeader is a valid storyboard up to its captions: beat 1 plays [0s, 2s), beat 2 is cut and
// beat 3 plays [2s, 5s) on the output clock.
const boardHeader = `clip: captions
ship: captions.gif
frame: {cols: 80, rows: 24, padding: 16, font_size: 14, line_height: 1.2, scale: 2, width: 600, fps: 24, max_colors: 128}
expect:
  - seen: 'done'
beats:
  - id: 1
    title: open
    duration: 2s
  - id: 2
    title: dropped
    duration: 1s
    cut: true
  - id: 3
    title: next
    duration: 3s
captions:
`

// The shipped frame most tests draw on: a 1200-pixel-wide source at scale 2 shipped 600 wide,
// so one design pixel spans half a shipped pixel.
var testOut = image.Pt(600, 360)

const testUnit = 0.5

// newTestLayer builds the layer of the storyboard whose captions list is captions.
func newTestLayer(t *testing.T, captions string, out image.Point, unit float64) *Layer {
	t.Helper()
	layer, err := New(parseBoard(t, captions), loadFaces(t), out, unit)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return layer
}

// parseBoard parses boardHeader followed by captions.
func parseBoard(t *testing.T, captions string) *storyboard.Storyboard {
	t.Helper()
	board, err := storyboard.Parse([]byte(boardHeader+captions), "board.yaml")
	if err != nil {
		t.Fatalf("parse storyboard: %v", err)
	}
	return board
}

// loadFaces loads the embedded caption faces.
func loadFaces(t *testing.T) fonts.CaptionSet {
	t.Helper()
	faces, err := fonts.LoadCaption("", "")
	if err != nil {
		t.Fatalf("load caption faces: %v", err)
	}
	return faces
}

// drawAt draws layer at output time at onto a fresh transparent frame of size out.
func drawAt(layer *Layer, out image.Point, at time.Duration) *image.RGBA {
	frame := image.NewRGBA(image.Rectangle{Max: out})
	layer.Draw(frame, at)
	return frame
}

// inkRect is the bounding rectangle of the frame's painted pixels; empty when none is.
func inkRect(frame *image.RGBA) image.Rectangle {
	var ink image.Rectangle
	bounds := frame.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if frame.RGBAAt(x, y).A != 0 {
				ink = ink.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return ink
}

func TestDraw_AnchorForms_ShowOverTheResolvedSpan(t *testing.T) {
	tests := []struct {
		name        string
		caption     string
		start, stop time.Duration
	}{
		{"bare clip time with for", "  - {text: Hi, at: 500ms, for: 1s}\n", 500 * time.Millisecond, 1500 * time.Millisecond},
		{"beat with offset", "  - {text: Hi, at: {beat: 3, offset: 250ms}, for: 1s}\n", 2250 * time.Millisecond, 3250 * time.Millisecond},
		{"beat with negative offset until a beat", "  - {text: Hi, at: {beat: 3, offset: -500ms}, until: {beat: 3, offset: 1s}}\n", 1500 * time.Millisecond, 3 * time.Second},
		{"beat until a bare clip time", "  - {text: Hi, at: {beat: 1}, until: 4s}\n", 0, 4 * time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			layer := newTestLayer(t, test.caption, testOut, testUnit)

			for _, sample := range []struct {
				at      time.Duration
				isShown bool
			}{
				{test.start - time.Millisecond, false},
				{test.start, true},
				{test.stop - time.Millisecond, true},
				{test.stop, false},
			} {
				isShown := !inkRect(drawAt(layer, testOut, sample.at)).Empty()

				if isShown != sample.isShown {
					t.Errorf("at %s: shown %t, want %t", sample.at, isShown, sample.isShown)
				}
			}
		})
	}
}

func TestDraw_Fades_RampTheBoxLinearly(t *testing.T) {
	const caption = "  - {text: Hi, at: 1s, for: 2s, box: {color: '#0000ff', opacity: 1}, fade: {in: 400ms, out: 800ms}}\n"
	layer := newTestLayer(t, caption, testOut, testUnit)
	corner := inkRect(drawAt(layer, testOut, 2*time.Second)).Min
	tests := []struct {
		name string
		at   time.Duration
		want float64
	}{
		{"fade-in start", 1 * time.Second, 0},
		{"fade-in middle", 1200 * time.Millisecond, 0.5},
		{"fade-in end", 1400 * time.Millisecond, 1},
		{"fade-out start", 2200 * time.Millisecond, 1},
		{"fade-out middle", 2600 * time.Millisecond, 0.5},
		{"fade-out end", 3 * time.Second, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			blue := drawAt(layer, testOut, test.at).RGBAAt(corner.X, corner.Y).B

			if want := math.Round(255 * test.want); math.Abs(float64(blue)-want) > 1 {
				t.Errorf("box blue at %s = %d, want %g", test.at, blue, want)
			}
		})
	}
}

func TestDraw_TextColour_AppearsInsideTheBoxOnly(t *testing.T) {
	const placement = "text: \"Hello\\nworld\", at: 0s, for: 1s, color: '#ff0000'"
	boxed := newTestLayer(t, "  - {"+placement+", box: {color: '#0000ff', opacity: 1}}\n", testOut, testUnit)
	bare := newTestLayer(t, "  - {"+placement+", box: {opacity: 0}}\n", testOut, testUnit)
	box := inkRect(drawAt(boxed, testOut, 0))

	frame := drawAt(bare, testOut, 0)

	var red int
	for y := range testOut.Y {
		for x := range testOut.X {
			pixel := frame.RGBAAt(x, y)
			if pixel.A == 0 {
				continue
			}
			if !image.Pt(x, y).In(box) {
				t.Fatalf("pixel (%d, %d) painted outside the box %v", x, y, box)
			}
			if pixel.R > 0 && pixel.G == 0 && pixel.B == 0 {
				red++
			}
		}
	}
	if red == 0 {
		t.Errorf("no pixel of the text colour inside the box %v", box)
	}
}

func TestNew_SizeAtFrameFontSize_HasTheShippedTerminalEm(t *testing.T) {
	const fontSize = 14
	tests := []struct {
		name        string
		scale       int
		sourceWidth int
		outWidth    int
	}{
		{"scale 2 shipped at half width", 2, 1200, 600},
		{"scale 2 shipped at full width", 2, 1200, 1200},
		{"scale 3 shipped at a third", 3, 1800, 600},
	}
	faces := loadFaces(t)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// The terminal's text is rasterized at font_size × scale pixels and downscaled to the
			// shipped width; the compositor's unit is scale/2 × the same downscale.
			downscale := float64(test.outWidth) / float64(test.sourceWidth)
			terminalEm := fontSize * float64(test.scale) * downscale
			unit := float64(test.scale) / designScale * downscale
			out := image.Pt(test.outWidth, 400)
			layer := newTestLayer(t, "  - {text: H, at: 0s, for: 1s, color: '#ffffff', box: {opacity: 0}}\n", out, unit)

			inkRows := countInkRows(drawAt(layer, out, 0))

			want := capHeight(t, faces.Regular.Font, terminalEm)
			if math.Abs(float64(inkRows)-want) > 1 {
				t.Errorf("H is %d rows tall, want the cap height %.1f of a %.1f-pixel em", inkRows, want, terminalEm)
			}
		})
	}
}

// countInkRows counts the frame's rows holding a pixel at least half covered.
func countInkRows(frame *image.RGBA) int {
	const halfCovered = 128
	var rows int
	bounds := frame.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if frame.RGBAAt(x, y).A >= halfCovered {
				rows++
				break
			}
		}
	}
	return rows
}

// capHeight is f's cap height in pixels at an em of em pixels.
func capHeight(t *testing.T, f *sfnt.Font, em float64) float64 {
	t.Helper()
	metrics, err := f.Metrics(&sfnt.Buffer{}, fixed.Int26_6(math.Round(em*fixedOne)), font.HintingNone)
	if err != nil {
		t.Fatalf("font metrics: %v", err)
	}
	return float64(metrics.CapHeight) / fixedOne
}

func TestDraw_ConcurrentCalls_MatchASequentialDraw(t *testing.T) {
	const captions = "  - {text: First, at: 0s, for: 3s, place: top}\n" +
		"  - {text: \"Second\\nline\", at: 500ms, for: 2s, fade: {in: 1s}}\n"
	layer := newTestLayer(t, captions, testOut, testUnit)
	const at = 1200 * time.Millisecond
	want := drawAt(layer, testOut, at).Pix

	const workers = 8
	var group sync.WaitGroup
	for range workers {
		group.Go(func() {
			if got := drawAt(layer, testOut, at).Pix; !bytes.Equal(got, want) {
				t.Error("a concurrent Draw differs from the sequential one")
			}
		})
	}
	group.Wait()
}

func TestNew_BadArguments_Fail(t *testing.T) {
	board := parseBoard(t, "  - {text: Hi, at: 0s, for: 1s}\n")
	faces := loadFaces(t)
	tests := []struct {
		name  string
		faces fonts.CaptionSet
		out   image.Point
		unit  float64
		want  string
	}{
		{"empty frame", faces, image.Point{}, testUnit, "positive size"},
		{"zero unit", faces, testOut, 0, "positive unit"},
		{"missing face", fonts.CaptionSet{Regular: faces.Regular}, testOut, testUnit, "missing a face"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(board, test.faces, test.out, test.unit)

			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("New error = %v, want one containing %q", err, test.want)
			}
		})
	}
}

func TestBoxes_AreTheRectanglesDrawPaints(t *testing.T) {
	const captions = "  - {text: Hi, at: 0s, for: 1s, place: top, align: left, box: {color: '#0000ff', opacity: 1}}\n" +
		"  - {text: There, at: 2s, for: 1s, position: {x: 100, y: 80}, box: {color: '#0000ff', opacity: 1}}\n"
	layer := newTestLayer(t, captions, testOut, testUnit)

	boxes := layer.Boxes()

	want := []image.Rectangle{
		inkRect(drawAt(layer, testOut, 0)),
		inkRect(drawAt(layer, testOut, 2*time.Second)),
	}
	if !slices.Equal(boxes, want) {
		t.Errorf("Boxes() = %v, want the painted boxes %v", boxes, want)
	}
	if wantMin := image.Pt(50, 40); len(boxes) == 2 && boxes[1].Min != wantMin {
		t.Errorf("the positioned caption's box starts at %v, want %v", boxes[1].Min, wantMin)
	}
}
