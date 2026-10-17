package compose

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"testing"
	"time"

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

// newTestCompositor schedules the take over the board's beats and builds its compositor.
func newTestCompositor(t *testing.T, board *storyboard.Storyboard, recorded *take.Take) (*Compositor, *schedule.Schedule) {
	t.Helper()
	board.Frame.Width = testWidth
	clipSchedule, err := schedule.New(board, recorded)
	if err != nil {
		t.Fatalf("schedule.New: %v", err)
	}
	compositor, err := New(board, recorded, clipSchedule, target.Resolver{}, testLayout, testBackground)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return compositor, clipSchedule
}

// zoomClickTake is a two-beat take: beat 1 idles for 2 s, beat 2 clicks box at 3 s and runs to
// 6 s, zoomed 2× onto the clicked target. Every beat fits its duration, so source and output
// clocks agree.
func zoomClickTake(box take.Box) (*storyboard.Storyboard, *take.Take) {
	clicked := storyboard.Target{Text: "target"}
	board := &storyboard.Storyboard{Beats: []storyboard.Beat{
		{ID: 1, Duration: 2 * sec},
		{
			ID: 2, Duration: 4 * sec,
			Do:   []storyboard.Action{{Click: &storyboard.ClickAction{Target: clicked}}},
			Zoom: &storyboard.Zoom{Target: clicked, Factor: 2},
		},
	}}
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
			board, recorded := zoomClickTake(box)

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
	board, recorded := zoomClickTake(take.Box{X: 20, Y: 6, W: 3, H: 1})
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
	board, recorded := zoomClickTake(box)
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
	wantRadius := cursorDiameter / 2 * compositor.unit()
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

func TestComposeZoomResolvesItsTargetOnTheSnapshot(t *testing.T) {
	t.Parallel()
	gauge := screen(0, map[int]string{3: "     ctx 42%"})
	board := &storyboard.Storyboard{Beats: []storyboard.Beat{
		{ID: 1, Duration: 4 * sec, Zoom: &storyboard.Zoom{Target: storyboard.Target{Text: `ctx \d+%`}}},
	}}
	end := gauge
	end.At = 4 * sec
	recorded := &take.Take{
		Snapshots: []take.Snapshot{gauge, end},
		Events:    []take.Event{beatStart(0, 1)},
	}

	compositor, _ := newTestCompositor(t, board, recorded)

	if len(compositor.zooms) != 1 {
		t.Fatalf("zooms: want one, got %+v", compositor.zooms)
	}
	zoom := compositor.zooms[0]
	if want := testLayout.boxRect(take.Box{X: 5, Y: 3, W: 7, H: 1}).centre(); zoom.Centre != want {
		t.Errorf("zoom centre: want the resolved box's %+v, got %+v", want, zoom.Centre)
	}
	if zoom.In != storyboard.DefaultZoomRamp || zoom.Out != storyboard.DefaultZoomRamp {
		t.Errorf("ramps: want the %s default, got in %s out %s", storyboard.DefaultZoomRamp, zoom.In, zoom.Out)
	}
	// Auto-fit: (7 + 4) cells × 20 px = 220 px filling 70 % of 1000 px is 3.18×, clamped to 2.5.
	if zoom.Factor != autoFitMax {
		t.Errorf("auto-fit factor: want %g, got %g", autoFitMax, zoom.Factor)
	}
}

func TestComposeZoomBoxWalksBackThroughTheBeat(t *testing.T) {
	t.Parallel()
	shown := map[int]string{3: "     ctx 42%"}
	wantCentre := testLayout.boxRect(take.Box{X: 5, Y: 3, W: 7, H: 1}).centre()
	for _, tc := range []struct {
		name      string
		snapshots []take.Snapshot
	}{
		{
			name:      "shown mid-beat, gone by its end",
			snapshots: []take.Snapshot{screen(0, nil), screen(2*sec, nil), screen(3*sec, shown), screen(5*sec, nil), screen(8*sec, nil)},
		},
		{
			name:      "shown only on the snapshot in effect at the beat's start",
			snapshots: []take.Snapshot{screen(0, nil), screen(1*sec, shown), screen(3*sec, nil), screen(8*sec, nil)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			board := &storyboard.Storyboard{Beats: []storyboard.Beat{
				{ID: 1, Duration: 2 * sec},
				{ID: 2, Duration: 4 * sec, Zoom: &storyboard.Zoom{Target: storyboard.Target{Text: `ctx \d+%`}, Factor: 2}},
				{ID: 3, Duration: 2 * sec},
			}}
			recorded := &take.Take{
				Snapshots: tc.snapshots,
				Events:    []take.Event{beatStart(0, 1), beatStart(2*sec, 2), beatStart(6*sec, 3)},
			}

			compositor, _ := newTestCompositor(t, board, recorded)

			if len(compositor.zooms) != 1 || compositor.zooms[0].Centre != wantCentre {
				t.Errorf("zooms: want one centred on %+v, got %+v", wantCentre, compositor.zooms)
			}
		})
	}
}

func TestNewRefusesAZoomTargetTheBeatNeverShows(t *testing.T) {
	t.Parallel()
	shown := map[int]string{3: "     ctx 42%"}
	board := &storyboard.Storyboard{
		Frame: storyboard.Frame{Width: testWidth},
		Beats: []storyboard.Beat{
			{ID: 1, Duration: 2 * sec, Zoom: &storyboard.Zoom{Target: storyboard.Target{Text: `ctx \d+%`}}},
			{ID: 2, Duration: 2 * sec},
		},
	}
	// Before the beat, and from the next beat's start on: never inside beat 1's span.
	recorded := &take.Take{
		Snapshots: []take.Snapshot{screen(0, nil), screen(2*sec, shown), screen(4*sec, shown)},
		Events:    []take.Event{beatStart(1*sec, 1), beatStart(2*sec, 2)},
	}
	clipSchedule, err := schedule.New(board, recorded)
	if err != nil {
		t.Fatalf("schedule.New: %v", err)
	}

	_, err = New(board, recorded, clipSchedule, target.Resolver{}, testLayout, testBackground)

	if !errors.Is(err, target.ErrNotOnScreen) || !strings.Contains(err.Error(), "beat 1: zoom") {
		t.Errorf("New: want beat 1's zoom target not on screen, got %v", err)
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

			_, err = New(board, recorded, clipSchedule, target.Resolver{}, tc.layout, testBackground)

			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("New: want an error naming %q, got %v", tc.want, err)
			}
		})
	}
}

func TestComposeAutoFitFactor(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		width int
		want  float64
	}{
		{name: "narrow target clamps to the maximum", width: 6, want: autoFitMax}, // 700 / 200 = 3.5
		{name: "fits between the bounds", width: 21, want: 1.4},                   // 700 / 500
		{name: "wide target clamps to the minimum", width: 36, want: autoFitMin},  // 700 / 800
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := autoFitFactor(take.Box{W: tc.width, H: 1}, testLayout); math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("auto-fit for %d cells: want %g, got %g", tc.width, tc.want, got)
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

func TestZoomRampScalesGeometrically(t *testing.T) {
	t.Parallel()
	zoom := zoomPlan{Start: 0, End: 10 * sec, In: 2 * sec, Out: 2 * sec, Factor: 4}
	for _, tc := range []struct {
		name string
		at   time.Duration
		want float64
	}{
		{name: "before the section", at: -sec, want: 1},
		{name: "at the section's start", at: 0, want: 1},
		{name: "half-way up the push", at: sec, want: 2},
		{name: "at the top of the ramp", at: 2 * sec, want: 4},
		{name: "held", at: 5 * sec, want: 4},
		{name: "half-way down the pull", at: 9 * sec, want: 2},
		{name: "at the section's end", at: 10 * sec, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := zoom.factorAt(tc.at); math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("factor at %s: want %g, got %g", tc.at, tc.want, got)
			}
		})
	}
}

func TestZoomRampsShrinkToFitTheSection(t *testing.T) {
	t.Parallel()
	board, recorded := zoomClickTake(take.Box{X: 20, Y: 6, W: 3, H: 1})
	board.Beats[1].Zoom.In, board.Beats[1].Zoom.Out = 3*sec, 5*sec

	compositor, _ := newTestCompositor(t, board, recorded)

	// 8 s of ramps in a 4 s section keep their 3:5 share.
	if zoom := compositor.zooms[0]; zoom.In != 1500*time.Millisecond || zoom.Out != 2500*time.Millisecond {
		t.Errorf("ramps: want in 1.5s out 2.5s, got in %s out %s", zoom.In, zoom.Out)
	}
}

func TestCompositorCell(t *testing.T) {
	t.Parallel()
	board, recorded := zoomClickTake(take.Box{X: 30, Y: 7, W: 1, H: 1})
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
	board, recorded := zoomClickTake(box)
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
