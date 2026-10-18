package compose

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/f64"

	"github.com/airiclenz/photon/internal/fonts"
	"github.com/airiclenz/photon/internal/schedule"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
)

// sec is one second, to keep the take timings readable.
const sec = time.Second

// testLayout is a small source frame: 49 × 14 cells of 20 × 40 pixels inside a 10-pixel
// padding, rasterized at the design scale and shipped at half its width.
var testLayout = Layout{Size: image.Pt(1000, 600), Padding: 10, CellWidth: 20, LineHeight: 40, Scale: 2}

// The test frame's grid, in cells.
const (
	testCols = 49
	testRows = 14
)

// testWidth is the shipped width the tests use.
const testWidth = 500

// testBackground is the colour a zoomed frame is cleared to, and the tests' blank frame.
var testBackground = color.RGBA{0x1e, 0x1e, 0x2e, 0xff}

// beatStart is the event that starts beat at source time at.
func beatStart(at time.Duration, beat int) take.Event {
	return take.Event{At: at, Kind: take.EventBeatStart, Beat: beat}
}

// clickAt is the target and click events of the beat's first action, a click on box's centre.
func clickAt(at time.Duration, beat int, box take.Box) []take.Event {
	cell := take.Point{X: box.X + box.W/2, Y: box.Y + box.H/2}
	return []take.Event{
		{At: at, Kind: take.EventTarget, Beat: beat, Form: "click", Box: &box},
		{At: at, Kind: take.EventClick, Beat: beat, Form: "click", Cell: &cell},
	}
}

// newTestCompositor schedules the take over the board's beats and builds its compositor, drawing
// any captions in the embedded caption faces.
func newTestCompositor(t *testing.T, board *storyboard.Storyboard, recorded *take.Take) (*Compositor, *schedule.Schedule) {
	t.Helper()
	board.Frame.Width = testWidth
	clipSchedule, err := schedule.New(board, recorded)
	if err != nil {
		t.Fatalf("schedule.New: %v", err)
	}
	var captionFaces fonts.CaptionSet
	if len(board.Captions) > 0 {
		if captionFaces, err = fonts.LoadCaption("", ""); err != nil {
			t.Fatalf("fonts.LoadCaption: %v", err)
		}
	}
	compositor, err := New(board, recorded, clipSchedule, target.Resolver{}, testLayout, testBackground, captionFaces)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return compositor, clipSchedule
}

// boardHeader is a storyboard of the test frame up to its beats.
const boardHeader = `clip: compose
ship: compose.gif
frame: {cols: 49, rows: 14, padding: 10, font_size: 10, line_height: 2, scale: 2, width: 500, fps: 24, max_colors: 128}
expect:
  - seen: 'done'
`

// parseBoard parses boardHeader followed by body, the storyboard's beats and any camera and
// captions, so the board's camera moves and captions carry their resolved starts and colours.
func parseBoard(t *testing.T, body string) *storyboard.Storyboard {
	t.Helper()
	board, err := storyboard.Parse([]byte(boardHeader+body), "board.yaml")
	if err != nil {
		t.Fatalf("parse storyboard: %v", err)
	}
	return board
}

// clickBeats is the beats list of zoomClickTake's storyboard: beat 1 idles for 2 s, beat 2 clicks
// the text "target" and lasts 4 s.
const clickBeats = `beats:
  - {id: 1, title: idle, duration: 2s}
  - id: 2
    title: click
    duration: 4s
    do:
      - click: {target: {text: target}}
`

// zoomCamera is the camera list that zooms 2× onto the centre cell of box through beat 2 of
// clickBeats: a pan there over the second before the beat, a push over the beat's first second
// and a pull over its last.
func zoomCamera(box take.Box) string {
	return fmt.Sprintf(`camera:
  - pan: {to: {cell: {x: %d, y: %d}}, at: {beat: 1, offset: 1s}, over: 1s}
  - zoom: {to: 2, at: {beat: 2}, over: 1s}
  - zoom: {to: 1, at: {beat: 2, offset: 3s}, over: 1s}
`, box.X+box.W/2, box.Y+box.H/2)
}

// zoomClickTake is a two-beat take: beat 1 idles for 2 s, beat 2 clicks box at 3 s and runs to
// 6 s, zoomed 2× onto the clicked box's centre cell by camera moves. Every beat fits its
// duration, so source and output clocks agree.
func zoomClickTake(t *testing.T, box take.Box) (*storyboard.Storyboard, *take.Take) {
	t.Helper()
	board := parseBoard(t, clickBeats+zoomCamera(box))
	events := []take.Event{beatStart(0, 1), beatStart(2*sec, 2)}
	recorded := &take.Take{
		Snapshots: []take.Snapshot{{At: 0}, {At: 6 * sec}},
		Events:    append(events, clickAt(3*sec, 2, box)...),
	}
	return board, recorded
}

// screen is a blank testCols × testRows snapshot at source time at, with text written into the
// rows lines names.
func screen(at time.Duration, lines map[int]string) take.Snapshot {
	cells := make([][]take.Cell, testRows)
	for y := range cells {
		row := make([]take.Cell, testCols)
		for x := range row {
			row[x] = take.Cell{Grapheme: " ", Width: 1}
		}
		for x, r := range lines[y] {
			row[x] = take.Cell{Grapheme: string(r), Width: 1}
		}
		cells[y] = row
	}
	return take.Snapshot{At: at, Cells: cells}
}

// solidFrame is a source frame filled with colour.
func solidFrame(colour color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rectangle{Max: testLayout.Size})
	draw.Draw(img, img.Bounds(), &image.Uniform{C: colour}, image.Point{}, draw.Src)
	return img
}

// centroidWhere is the mean position of the pixels (centres) matching keep, and how many did.
func centroidWhere(img *image.RGBA, keep func(color.RGBA) bool) (pointF, int) {
	var sum pointF
	count := 0
	for y := img.Rect.Min.Y; y < img.Rect.Max.Y; y++ {
		for x := img.Rect.Min.X; x < img.Rect.Max.X; x++ {
			if keep(img.RGBAAt(x, y)) {
				sum.X += float64(x) + 0.5
				sum.Y += float64(y) + 0.5
				count++
			}
		}
	}
	if count == 0 {
		return pointF{}, 0
	}
	return pointF{sum.X / float64(count), sum.Y / float64(count)}, count
}

func TestComposeZoomCropStaysInsideTheFrame(t *testing.T) {
	t.Parallel()
	for _, box := range []take.Box{{X: 0, Y: 0, W: 3, H: 1}, {X: 45, Y: 13, W: 4, H: 1}} {
		t.Run(fmt.Sprintf("box at %d,%d", box.X, box.Y), func(t *testing.T) {
			t.Parallel()
			board, recorded := zoomClickTake(t, box)

			compositor, _ := newTestCompositor(t, board, recorded)

			crop := compositor.crop(4 * sec)
			if crop.W != 500 || crop.H != 300 {
				t.Errorf("crop at full zoom: want 500×300, got %g×%g", crop.W, crop.H)
			}
			if crop.X < 0 || crop.Y < 0 || crop.X+crop.W > 1000 || crop.Y+crop.H > 600 {
				t.Errorf("crop %+v leaves the 1000×600 frame", crop)
			}
			for _, at := range []time.Duration{2*sec + 200*time.Millisecond, 5*sec + 800*time.Millisecond} {
				ramp := compositor.crop(at)
				if ramp.W <= 500 || ramp.W >= 1000 || ramp.X < 0 || ramp.X+ramp.W > 1000 {
					t.Errorf("crop mid-ramp at %s: want inside the frame and between the factors, got %+v", at, ramp)
				}
			}
			if full := compositor.crop(1 * sec); full != (rectF{W: 1000, H: 600}) {
				t.Errorf("crop before the zoomed beat: want the whole frame, got %+v", full)
			}
			if got := compositor.Compose(4*sec, solidFrame(testBackground)).Bounds().Size(); got != image.Pt(500, 300) {
				t.Errorf("composed frame: want 500×300, got %v", got)
			}
		})
	}
}

func TestComposeCursorGlidesAlongTheSegment(t *testing.T) {
	t.Parallel()
	board := &storyboard.Storyboard{Beats: []storyboard.Beat{{ID: 1, Duration: 10 * sec}}}
	events := []take.Event{beatStart(0, 1)}
	events = append(events, clickAt(2*sec, 1, take.Box{X: 4, Y: 2, W: 1, H: 1})...)
	events = append(events, clickAt(5*sec, 1, take.Box{X: 40, Y: 11, W: 1, H: 1})...)
	recorded := &take.Take{Snapshots: []take.Snapshot{{At: 0}, {At: 10 * sec}}, Events: events}
	compositor, _ := newTestCompositor(t, board, recorded)
	home, first, second := testLayout.home(), testLayout.cellCentre(4, 2), testLayout.cellCentre(40, 11)

	for _, tc := range []struct {
		name     string
		at       time.Duration
		from, to pointF
		share    float64
	}{
		{name: "before the first glide", at: 0, from: home, to: first, share: 0},
		{name: "at the first glide's start", at: 2*sec - cursorGlide, from: home, to: first, share: 0},
		{name: "half-way to the first click", at: 2*sec - cursorGlide/2, from: home, to: first, share: 0.5},
		{name: "at the first press", at: 2 * sec, from: home, to: first, share: 1},
		{name: "resting on the first click", at: 3 * sec, from: first, to: second, share: 0},
		{name: "half-way to the second click", at: 5*sec - cursorGlide/2, from: first, to: second, share: 0.5},
		{name: "at the second press", at: 5 * sec, from: first, to: second, share: 1},
		{name: "after the last click", at: 9 * sec, from: first, to: second, share: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := compositor.cursor.position(tc.at)

			want := lerp(tc.from, tc.to, tc.share)
			if math.Hypot(got.X-want.X, got.Y-want.Y) > 1e-9 {
				t.Errorf("position at %s: want %+v, got %+v", tc.at, want, got)
			}
		})
	}
}

func TestComposeCursorFadesAroundTheBeatsClicks(t *testing.T) {
	t.Parallel()
	board := &storyboard.Storyboard{Beats: []storyboard.Beat{{ID: 1, Duration: 10 * sec}}}
	events := []take.Event{beatStart(0, 1)}
	events = append(events, clickAt(2*sec, 1, take.Box{X: 4, Y: 2, W: 1, H: 1})...)
	events = append(events, clickAt(5*sec, 1, take.Box{X: 40, Y: 11, W: 1, H: 1})...)
	recorded := &take.Take{Snapshots: []take.Snapshot{{At: 0}, {At: 10 * sec}}, Events: events}
	compositor, _ := newTestCompositor(t, board, recorded)

	for _, tc := range []struct {
		name string
		at   time.Duration
		want float64
	}{
		{name: "before the fade-in", at: 2*sec - cursorGlide - cursorFadeIn, want: 0},
		{name: "half-way through the fade-in", at: 2*sec - cursorGlide - cursorFadeIn/2, want: 0.5},
		{name: "at the first glide's start", at: 2*sec - cursorGlide, want: 1},
		{name: "between the clicks", at: 3 * sec, want: 1},
		{name: "lingering after the last click", at: 5*sec + cursorLinger, want: 1},
		{name: "after the fade-out", at: 5*sec + cursorLinger + cursorFadeOut, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := compositor.cursor.opacity(tc.at); math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("opacity at %s: want %g, got %g", tc.at, tc.want, got)
			}
		})
	}
}

func TestComposeRingGrowsLinearly(t *testing.T) {
	t.Parallel()
	board, recorded := zoomClickTake(t, take.Box{X: 20, Y: 6, W: 3, H: 1})
	compositor, _ := newTestCompositor(t, board, recorded)

	diameter, opacity, _, ok := compositor.cursor.ring(3*sec + ringPulse/2)
	if !ok || math.Abs(diameter-50) > 0.5 || math.Abs(opacity-0.4) > 0.01 {
		t.Errorf("ring half-way through its pulse: want ≈50 px at 40%%, got %g px at %g (shown %v)", diameter, opacity, ok)
	}
	if diameter, _, _, _ := compositor.cursor.ring(3 * sec); diameter != cursorDiameter {
		t.Errorf("ring at the press: want %g px, got %g", cursorDiameter, diameter)
	}
	if _, _, _, ok := compositor.cursor.ring(3*sec + ringPulse); ok {
		t.Error("ring after its pulse: want none")
	}
}

func TestComposeZoomedCursorMapsToTheZoomedTarget(t *testing.T) {
	t.Parallel()
	box := take.Box{X: 30, Y: 7, W: 1, H: 1}
	board, recorded := zoomClickTake(t, box)
	compositor, _ := newTestCompositor(t, board, recorded)
	press := 3 * sec

	// Where the zoom puts the target cell: paint it red and find it in the output, drawn by a
	// compositor with the same zoom and no clicks.
	marked := solidFrame(testBackground)
	rect := testLayout.boxRect(box)
	draw.Draw(marked, image.Rect(int(rect.X), int(rect.Y), int(rect.X+rect.W), int(rect.Y+rect.H)),
		&image.Uniform{C: color.RGBA{0xff, 0, 0, 0xff}}, image.Point{}, draw.Src)
	noClicks := *compositor
	noClicks.cursor = cursorTrack{Home: testLayout.home()}
	red := func(c color.RGBA) bool { return c.R > 0x80 && c.G < 0x60 }
	cell, count := centroidWhere(noClicks.Compose(press, marked), red)
	if count == 0 {
		t.Fatal("the zoomed target cell is not in the composed frame")
	}

	// Where the cursor lands at the press, over a blank frame.
	bright := func(c color.RGBA) bool { return c.G > testBackground.G+0x30 }
	cursor, count := centroidWhere(compositor.Compose(press, solidFrame(testBackground)), bright)
	if count == 0 {
		t.Fatal("no cursor drawn at the press")
	}
	if math.Hypot(cursor.X-cell.X, cursor.Y-cell.Y) > 1 {
		t.Errorf("cursor at the press: want it on the zoomed target cell %+v, got %+v", cell, cursor)
	}

	// Constant size: the dot's footprint does not grow with the zoom.
	wantRadius := cursorDiameter / 2 * compositor.unit
	if limit := math.Pi * wantRadius * wantRadius * 1.3; float64(count) > limit {
		t.Errorf("cursor footprint: want at most %.0f pixels at constant size, got %d", limit, count)
	}
}

func TestComposeClickInsideACutBeatDrawsNoCursor(t *testing.T) {
	t.Parallel()
	board := &storyboard.Storyboard{Beats: []storyboard.Beat{
		{ID: 1, Duration: 2 * sec},
		{ID: 2, Cut: true, Do: []storyboard.Action{{Click: &storyboard.ClickAction{Target: storyboard.Target{Text: "x"}}}}},
		{ID: 3, Duration: 2 * sec},
	}}
	events := []take.Event{beatStart(0, 1), beatStart(2*sec, 2)}
	events = append(events, clickAt(3*sec, 2, take.Box{X: 20, Y: 6, W: 1, H: 1})...)
	events = append(events, beatStart(4*sec, 3))
	recorded := &take.Take{Snapshots: []take.Snapshot{{At: 0}, {At: 6 * sec}}, Events: events}
	compositor, clipSchedule := newTestCompositor(t, board, recorded)

	if len(compositor.cursor.Marks) != 0 {
		t.Fatalf("cursor marks: want the cut click dropped, got %+v", compositor.cursor.Marks)
	}
	blank := solidFrame(testBackground)
	for _, frame := range clipSchedule.Frames(10) {
		composed := compositor.Compose(frame.Out, blank)
		if _, count := centroidWhere(composed, func(c color.RGBA) bool { return c != testBackground }); count != 0 {
			t.Fatalf("frame at %s: want nothing drawn over the background, got %d pixels", frame.Out, count)
		}
	}
}

func TestNewRefusesAnUnusableFrame(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		width  int
		layout Layout
		want   string
	}{
		{name: "no shipped width", width: 0, layout: testLayout, want: "frame.width"},
		{name: "no cell size", width: testWidth, layout: Layout{Size: testLayout.Size}, want: "invalid cell layout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			board := &storyboard.Storyboard{Frame: storyboard.Frame{Width: tc.width}, Beats: []storyboard.Beat{{ID: 1, Duration: sec}}}
			recorded := &take.Take{Snapshots: []take.Snapshot{{At: 0}, {At: sec}}, Events: []take.Event{beatStart(0, 1)}}
			clipSchedule, err := schedule.New(board, recorded)
			if err != nil {
				t.Fatalf("schedule.New: %v", err)
			}

			_, err = New(board, recorded, clipSchedule, target.Resolver{}, tc.layout, testBackground, fonts.CaptionSet{})

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("New: want an error naming %q, got %v", tc.want, err)
			}
		})
	}
}

func TestEaseInOutSettlesAtBothEnds(t *testing.T) {
	t.Parallel()
	const h = 1e-4
	for _, edge := range []float64{0, 1} {
		if got := easeInOut(edge); got != edge {
			t.Errorf("ease(%g): want %g, got %g", edge, edge, got)
		}
	}
	if got := easeInOut(0.5); got != 0.5 {
		t.Errorf("ease(0.5): want the half-way point 0.5, got %g", got)
	}
	// Speed and acceleration both vanish at the ends: the second difference is O(h³), not O(h²).
	if got := easeInOut(2*h) - 2*easeInOut(h) + easeInOut(0); math.Abs(got) > h*h*1e-2 {
		t.Errorf("ease at its start: want no acceleration, got a second difference of %g", got)
	}
}

func TestCompositorCell(t *testing.T) {
	t.Parallel()
	board, recorded := zoomClickTake(t, take.Box{X: 30, Y: 7, W: 1, H: 1})
	compositor, _ := newTestCompositor(t, board, recorded)
	tests := []struct {
		name    string
		at      time.Duration
		pixel   image.Point
		wantCol int
		wantRow int
		wantOK  bool
	}{
		{name: "unzoomed, first cell", at: 1 * sec, pixel: image.Pt(5, 5), wantCol: 0, wantRow: 0, wantOK: true},
		{name: "unzoomed, frame centre", at: 1 * sec, pixel: image.Pt(250, 150), wantCol: 24, wantRow: 7, wantOK: true},
		{name: "mid-ramp, top-left corner", at: 2*sec + 500*time.Millisecond, pixel: image.Pt(0, 0), wantCol: 12, wantRow: 2, wantOK: true},
		{name: "fully zoomed, top-left corner", at: 4 * sec, pixel: image.Pt(0, 0), wantCol: 18, wantRow: 3, wantOK: true},
		{name: "fully zoomed, frame centre on the target", at: 4 * sec, pixel: image.Pt(250, 150), wantCol: 30, wantRow: 7, wantOK: true},
		{name: "on the padding", at: 1 * sec, pixel: image.Pt(0, 0), wantCol: 0, wantRow: 0, wantOK: false},
		{name: "left of the frame", at: 1 * sec, pixel: image.Pt(-3, 150), wantCol: 0, wantRow: 7, wantOK: false},
		{name: "right of the frame", at: 1 * sec, pixel: image.Pt(testWidth+4, 150), wantCol: testCols - 1, wantRow: 7, wantOK: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			col, row, ok := compositor.Cell(test.at, test.pixel)

			if col != test.wantCol || row != test.wantRow || ok != test.wantOK {
				t.Errorf("Cell(%s, %v): want (%d, %d, %t), got (%d, %d, %t)",
					test.at, test.pixel, test.wantCol, test.wantRow, test.wantOK, col, row, ok)
			}
		})
	}
}

func TestCompositorRect(t *testing.T) {
	t.Parallel()
	box := take.Box{X: 30, Y: 7, W: 1, H: 1}
	board, recorded := zoomClickTake(t, box)
	compositor, _ := newTestCompositor(t, board, recorded)
	tests := []struct {
		name string
		at   time.Duration
		want image.Rectangle
	}{
		{name: "unzoomed", at: 1 * sec, want: image.Rect(305, 145, 315, 165)},
		{name: "fully zoomed", at: 4 * sec, want: image.Rect(240, 130, 260, 170)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := compositor.Rect(test.at, box)

			if got != test.want {
				t.Errorf("Rect(%s, %+v): want %v, got %v", test.at, box, test.want, got)
			}
		})
	}
}

// testCaptionBox is the colour of a test caption's box, which no other test pixel takes.
var testCaptionBox = color.RGBA{0x20, 0x40, 0x60, 0xff}

// bottomMiddle is the flow-mapping fields that place a caption at the bottom middle.
const bottomMiddle = "place: bottom, align: middle"

// boxCaption is a captions-list entry: text in an opaque box of testCaptionBox, from the moment
// at for length, laid out by placement's flow-mapping fields, with no fade.
func boxCaption(text, at, length, placement string) string {
	return fmt.Sprintf(
		"  - {text: %q, at: %s, for: %s, font: {size: 14, weight: regular}, %s, color: '#ffffff', box: {color: '#204060', opacity: 1}}\n",
		text, at, length, placement,
	)
}

// stripedFrame is a source frame of vertical stripes, so a zoomed frame differs from the
// unzoomed one wherever the push shows.
func stripedFrame() *image.RGBA {
	const stripe = 7
	img := solidFrame(testBackground)
	for x := 0; x < testLayout.Size.X; x += 2 * stripe {
		band := image.Rect(x, 0, x+stripe, testLayout.Size.Y)
		draw.Draw(img, band, &image.Uniform{C: color.RGBA{0xf0, 0xc0, 0x40, 0xff}}, image.Point{}, draw.Src)
	}
	return img
}

// captionBox draws compositor's captions alone at output time at onto a transparent shipped
// frame and returns the rectangle they cover and that frame, failing when none shows.
func captionBox(t *testing.T, compositor *Compositor, at time.Duration) (image.Rectangle, *image.RGBA) {
	t.Helper()
	layer := image.NewRGBA(image.Rectangle{Max: compositor.Size()})
	compositor.captions.Draw(layer, at)
	var covered image.Rectangle
	for y := range layer.Rect.Max.Y {
		for x := range layer.Rect.Max.X {
			if layer.RGBAAt(x, y).A > 0 {
				covered = covered.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	if covered.Empty() {
		t.Fatalf("no caption shows at %s", at)
	}
	return covered, layer
}

// assertSameRegion fails the test at the first pixel of region where got and want differ.
func assertSameRegion(t *testing.T, got, want *image.RGBA, region image.Rectangle) {
	t.Helper()
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			if g, w := got.RGBAAt(x, y), want.RGBAAt(x, y); g != w {
				t.Fatalf("pixel (%d,%d) of %v: want %v, got %v", x, y, region, w, g)
			}
		}
	}
}

// countColour is how many of img's pixels are exactly colour.
func countColour(img *image.RGBA, colour color.RGBA) int {
	_, count := centroidWhere(img, func(c color.RGBA) bool { return c == colour })
	return count
}

func TestComposeZoomLeavesACaptionInPlace(t *testing.T) {
	t.Parallel()
	clicked := take.Box{X: 30, Y: 3, W: 6, H: 1}
	_, recorded := zoomClickTake(t, clicked)
	board := parseBoard(t, clickBeats+zoomCamera(clicked)+"captions:\n"+
		boxCaption("Caption", "{beat: 1}", "6s", bottomMiddle))
	compositor, _ := newTestCompositor(t, board, recorded)
	src := stripedFrame()
	unzoomed, zoomed := 1*sec, 4*sec
	if crop := compositor.crop(zoomed); crop.W >= float64(testLayout.Size.X) {
		t.Fatalf("crop at %s: want the camera's zoom pushed in, got %+v", zoomed, crop)
	}

	plain, pushed := compositor.Compose(unzoomed, src), compositor.Compose(zoomed, src)

	box, _ := captionBox(t, compositor, unzoomed)
	if zoomedBox, _ := captionBox(t, compositor, zoomed); zoomedBox != box {
		t.Fatalf("caption box: want %v at both times, got %v zoomed", box, zoomedBox)
	}
	if countColour(plain, testCaptionBox) == 0 {
		t.Fatal("unzoomed frame: want the caption's box drawn")
	}
	assertSameRegion(t, pushed, plain, box)
	if bytes.Equal(pushed.Pix, plain.Pix) {
		t.Error("want the zoomed frame to differ from the unzoomed one outside the caption")
	}
}

func TestComposeBeatAnchoredCaptionFollowsTheBeatsDuration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		firstBeat     time.Duration
		shown, hidden time.Duration
	}{
		{name: "beat 1 lasts 2s", firstBeat: 2 * sec, shown: 2500 * time.Millisecond, hidden: 3500 * time.Millisecond},
		{name: "beat 1 lasts 3s", firstBeat: 3 * sec, shown: 3500 * time.Millisecond, hidden: 2500 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			board := parseBoard(t, fmt.Sprintf(`beats:
  - {id: 1, title: one, duration: %s}
  - {id: 2, title: two, duration: 4s}
captions:
`, tc.firstBeat)+boxCaption("Beat two", "{beat: 2}", "1s", bottomMiddle))
			recorded := &take.Take{
				Snapshots: []take.Snapshot{{At: 0}, {At: 6 * sec}},
				Events:    []take.Event{beatStart(0, 1), beatStart(2*sec, 2)},
			}
			compositor, _ := newTestCompositor(t, board, recorded)
			src := solidFrame(testBackground)

			if countColour(compositor.Compose(tc.shown, src), testCaptionBox) == 0 {
				t.Errorf("at %s: want the caption shown", tc.shown)
			}
			if countColour(compositor.Compose(tc.hidden, src), testCaptionBox) != 0 {
				t.Errorf("at %s: want the caption gone", tc.hidden)
			}
		})
	}
}

func TestCompositorCaptionBoxes(t *testing.T) {
	t.Parallel()
	recorded := &take.Take{
		Snapshots: []take.Snapshot{{At: 0}, {At: 6 * sec}},
		Events:    []take.Event{beatStart(0, 1), beatStart(2*sec, 2)},
	}
	board := parseBoard(t, `beats:
  - {id: 1, title: one, duration: 2s}
  - {id: 2, title: two, duration: 4s}
captions:
`+boxCaption("First", "{beat: 1}", "1s", bottomMiddle)+
		boxCaption("Second", "{beat: 2}", "1s", "position: {x: 40, y: 40}"))
	compositor, _ := newTestCompositor(t, board, recorded)
	first, _ := captionBox(t, compositor, 0)
	second, _ := captionBox(t, compositor, 2*sec)

	boxes := compositor.CaptionBoxes()

	if want := []image.Rectangle{first, second}; !slices.Equal(boxes, want) {
		t.Errorf("CaptionBoxes() = %v, want the drawn boxes %v", boxes, want)
	}
	if _, unit, _ := Output(testLayout, board.Frame.Width); compositor.Unit() != unit {
		t.Errorf("Unit() = %g, want Output's %g", compositor.Unit(), unit)
	}
}

func TestCompositorCaptionBoxes_NoCaptions_IsNil(t *testing.T) {
	t.Parallel()
	board, recorded := zoomClickTake(t, take.Box{X: 30, Y: 7, W: 1, H: 1})
	compositor, _ := newTestCompositor(t, board, recorded)

	boxes := compositor.CaptionBoxes()

	if boxes != nil {
		t.Errorf("CaptionBoxes() = %v, want nil for a storyboard with no captions", boxes)
	}
}

// directCompose is Compose's direct path before ComposeDraft shared its body: CatmullRom's Scale
// of the whole source at a factor of 1, else its Transform of the crop in layout pixels, then the
// cursor and the captions.
func directCompose(compositor *Compositor, at time.Duration, src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(image.Rectangle{Max: compositor.out})
	crop := compositor.crop(at)
	if crop.W >= float64(compositor.layout.Size.X) {
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	} else {
		draw.Draw(dst, dst.Bounds(), &image.Uniform{C: compositor.background}, image.Point{}, draw.Src)
		scaleX, scaleY := float64(compositor.out.X)/crop.W, float64(compositor.out.Y)/crop.H
		transform := f64.Aff3{scaleX, 0, -crop.X * scaleX, 0, scaleY, -crop.Y * scaleY}
		xdraw.CatmullRom.Transform(dst, transform, src, src.Bounds(), xdraw.Src, nil)
	}
	compositor.drawCursor(dst, at, crop)
	if compositor.captions != nil {
		compositor.captions.Draw(dst, at)
	}
	return dst
}

// draftBoard is zoomClickTake's two beats with a caption over both, the second beat zoomed 2×
// onto the centre cell of clicked when zoomed is set.
func draftBoard(t *testing.T, clicked take.Box, zoomed bool) *storyboard.Storyboard {
	t.Helper()
	camera := ""
	if zoomed {
		camera = zoomCamera(clicked)
	}
	return parseBoard(t, clickBeats+camera+"captions:\n"+
		boxCaption("Caption", "{beat: 1}", "6s", bottomMiddle))
}

// draftColour is the uniform source colour the draft tests compose, unlike the zoom's background.
var draftColour = color.RGBA{0x40, 0x80, 0x30, 0xff}

// uniformFrame is a source frame of size filled with draftColour.
func uniformFrame(size image.Point) *image.RGBA {
	img := image.NewRGBA(image.Rectangle{Max: size})
	draw.Draw(img, img.Bounds(), &image.Uniform{C: draftColour}, image.Point{}, draw.Src)
	return img
}

func TestComposeMatchesTheDirectPath(t *testing.T) {
	t.Parallel()
	clicked := take.Box{X: 30, Y: 3, W: 6, H: 1}
	_, recorded := zoomClickTake(t, clicked)
	compositor, _ := newTestCompositor(t, draftBoard(t, clicked, true), recorded)
	src := stripedFrame()
	for _, tc := range []struct {
		name   string
		at     time.Duration
		zoomed bool
	}{
		{name: "at factor 1", at: 1 * sec},
		{name: "in a zoom", at: 4 * sec, zoomed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if crop := compositor.crop(tc.at); (crop.W < float64(testLayout.Size.X)) != tc.zoomed {
				t.Fatalf("crop at %s: want zoomed %v, got %+v", tc.at, tc.zoomed, crop)
			}

			got := compositor.Compose(tc.at, src)

			if want := directCompose(compositor, tc.at, src); !bytes.Equal(got.Pix, want.Pix) {
				t.Error("Compose: want the direct path's bytes, got different pixels")
			}
		})
	}
}

func TestComposeDraftAtTheLayoutsSizeFillsTheShippedFrame(t *testing.T) {
	t.Parallel()
	clicked := take.Box{X: 30, Y: 3, W: 6, H: 1}
	_, recorded := zoomClickTake(t, clicked)
	compositor, _ := newTestCompositor(t, draftBoard(t, clicked, true), recorded)

	got := compositor.ComposeDraft(1*sec, stripedFrame())

	if got.Bounds() != (image.Rectangle{Max: compositor.Size()}) {
		t.Errorf("draft bounds: want %v, got %v", image.Rectangle{Max: compositor.Size()}, got.Bounds())
	}
}

func TestComposeDraftDrawsTheCursorAndCaptionsAsCompose(t *testing.T) {
	t.Parallel()
	draftSize := image.Pt(617, 353)
	for _, tc := range []struct {
		name   string
		zoomed bool
	}{
		{name: "at factor 1"},
		{name: "in a zoom", zoomed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clicked := take.Box{X: 30, Y: 3, W: 6, H: 1}
			_, recorded := zoomClickTake(t, clicked)
			compositor, _ := newTestCompositor(t, draftBoard(t, clicked, tc.zoomed), recorded)
			at := 3 * sec
			if crop := compositor.crop(at); (crop.W < float64(testLayout.Size.X)) != tc.zoomed {
				t.Fatalf("crop at %s: want zoomed %v, got %+v", at, tc.zoomed, crop)
			}
			exact := compositor.Compose(at, uniformFrame(testLayout.Size))

			draft := compositor.ComposeDraft(at, uniformFrame(draftSize))

			if draft.Bounds() != exact.Bounds() {
				t.Fatalf("draft bounds: want %v, got %v", exact.Bounds(), draft.Bounds())
			}
			box, _ := captionBox(t, compositor, at)
			var drawn image.Rectangle
			cursorPixels := 0
			for y := range exact.Rect.Max.Y {
				for x := range exact.Rect.Max.X {
					if exact.RGBAAt(x, y) == draftColour {
						continue
					}
					drawn = drawn.Union(image.Rect(x, y, x+1, y+1))
					if !image.Pt(x, y).In(box) {
						cursorPixels++
					}
				}
			}
			if cursorPixels == 0 {
				t.Fatalf("exact frame at %s: want the cursor drawn outside the caption", at)
			}
			assertSameRegion(t, draft, exact, drawn)
		})
	}
}
