package compose

import (
	"image"
	"image/color"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

// dragTestGap is the source time between a test drag's press, its moves and its release.
const dragTestGap = 250 * time.Millisecond

// The test drag: pressed on (4,2) at 2 s, it moves along row 2 every dragTestGap and releases
// on (16,2) a gap after its final move at 3 s, so its motion spans exactly 2 s to 3 s.
var (
	dragPress   = take.Point{X: 4, Y: 2}
	dragMoves   = []take.Point{{X: 5, Y: 2}, {X: 8, Y: 2}, {X: 13, Y: 2}, {X: 16, Y: 2}}
	dragPressAt = 2 * sec
	dragFinalAt = dragPressAt + 4*dragTestGap
	dragRelease = dragFinalAt + dragTestGap
)

// dragAt is the events of the beat's first action, a drag: its `from` target event on the
// one-cell box at press, the press at source time at, one move per cell in moves every
// dragTestGap after it, and the release a gap after the last move on its cell — on the press
// cell when there are no moves.
func dragAt(at time.Duration, beat int, press take.Point, moves ...take.Point) []take.Event {
	box := take.Box{X: press.X, Y: press.Y, W: 1, H: 1}
	pressed := press
	events := []take.Event{
		{At: at, Kind: take.EventTarget, Beat: beat, Form: "drag", Text: dragFromText, Box: &box},
		{At: at, Kind: take.EventPress, Beat: beat, Form: "drag", Cell: &pressed},
	}
	released := press
	for index, move := range moves {
		cell := move
		events = append(events, take.Event{
			At: at + time.Duration(index+1)*dragTestGap, Kind: take.EventMove, Beat: beat, Form: "drag", Cell: &cell,
		})
		released = move
	}
	events = append(events, take.Event{
		At: at + time.Duration(len(moves)+1)*dragTestGap, Kind: take.EventRelease, Beat: beat, Form: "drag", Cell: &released,
	})
	return events
}

// newDragCompositor is the compositor of a one-beat, 10-second take holding the test drag.
func newDragCompositor(t *testing.T) *Compositor {
	t.Helper()
	board := &storyboard.Storyboard{Beats: []storyboard.Beat{{ID: 1, Duration: 10 * sec}}}
	events := append([]take.Event{beatStart(0, 1)}, dragAt(dragPressAt, 1, dragPress, dragMoves...)...)
	recorded := &take.Take{Snapshots: []take.Snapshot{{At: 0}, {At: 10 * sec}}, Events: events}
	compositor, _ := newTestCompositor(t, board, recorded)
	return compositor
}

func TestCursorFollowsADrag(t *testing.T) {
	t.Parallel()
	compositor := newDragCompositor(t)
	home := testLayout.home()
	press := testLayout.cellCentre(dragPress.X, dragPress.Y)
	final := testLayout.cellCentre(dragMoves[3].X, dragMoves[3].Y)

	for _, tc := range []struct {
		name string
		at   time.Duration
		want pointF
	}{
		{name: "half-way through the glide to the press", at: dragPressAt - cursorGlide/2, want: lerp(home, press, 0.5)},
		{name: "at the press", at: dragPressAt, want: press},
		{name: "at the drag's time midpoint", at: (dragPressAt + dragFinalAt) / 2, want: lerp(press, final, 0.5)},
		{name: "at the final move", at: dragFinalAt, want: final},
		{name: "at the release", at: dragRelease, want: final},
		{name: "after the release", at: 8 * sec, want: final},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := compositor.cursor.position(tc.at)
			if math.Hypot(got.X-tc.want.X, got.Y-tc.want.Y) > 1e-9 {
				t.Errorf("position at %s: want %+v, got %+v", tc.at, tc.want, got)
			}
		})
	}
}

func TestCursorEasesIntoADrag(t *testing.T) {
	t.Parallel()
	compositor := newDragCompositor(t)
	press := testLayout.cellCentre(dragPress.X, dragPress.Y)
	final := testLayout.cellCentre(dragMoves[3].X, dragMoves[3].Y)
	at := dragPressAt + (dragFinalAt-dragPressAt)/10

	got := compositor.cursor.position(at)
	share := (got.X - press.X) / (final.X - press.X)
	if share <= 0 || share >= 0.1 || got.Y != press.Y {
		t.Errorf("position a tenth into the drag: want on the row, under the linear share 0.1, got %+v (share %g)", got, share)
	}
}

func TestCursorRingsOnADragsPressAndRelease(t *testing.T) {
	t.Parallel()
	compositor := newDragCompositor(t)
	press := testLayout.cellCentre(dragPress.X, dragPress.Y)
	final := testLayout.cellCentre(dragMoves[3].X, dragMoves[3].Y)

	for _, tc := range []struct {
		name     string
		at       time.Duration
		wantRing bool
		centre   pointF
	}{
		{name: "at the press", at: dragPressAt, wantRing: true, centre: press},
		{name: "mid-drag", at: dragPressAt + ringPulse, wantRing: false},
		{name: "at the final move", at: dragFinalAt, wantRing: false},
		{name: "at the release", at: dragRelease, wantRing: true, centre: final},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, centre, ok := compositor.cursor.ring(tc.at)
			if ok != tc.wantRing {
				t.Fatalf("ring at %s: want shown %v, got %v", tc.at, tc.wantRing, ok)
			}
			if ok && centre != tc.centre {
				t.Errorf("ring at %s: want centred on %+v, got %+v", tc.at, tc.centre, centre)
			}
		})
	}
}

func TestCursorStaysThroughADragAndFadesAfterTheRelease(t *testing.T) {
	t.Parallel()
	compositor := newDragCompositor(t)

	for _, tc := range []struct {
		name string
		at   time.Duration
		want float64
	}{
		{name: "mid-drag", at: (dragPressAt + dragFinalAt) / 2, want: 1},
		{name: "at the release", at: dragRelease, want: 1},
		{name: "lingering after the release", at: dragRelease + cursorLinger, want: 1},
		{name: "half-way through the fade-out", at: dragRelease + cursorLinger + cursorFadeOut/2, want: 0.5},
		{name: "after the fade-out", at: dragRelease + cursorLinger + cursorFadeOut, want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := compositor.cursor.opacity(tc.at); math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("opacity at %s: want %g, got %g", tc.at, tc.want, got)
			}
		})
	}
}

func TestComposeDrawsTheCursorMidDrag(t *testing.T) {
	t.Parallel()
	compositor := newDragCompositor(t)
	at := (dragPressAt + dragFinalAt) / 2
	press := testLayout.cellCentre(dragPress.X, dragPress.Y)
	final := testLayout.cellCentre(dragMoves[3].X, dragMoves[3].Y)

	bright := func(c color.RGBA) bool { return c.G > testBackground.G+0x30 }
	got, count := centroidWhere(compositor.Compose(at, solidFrame(testBackground)), bright)
	if count == 0 {
		t.Fatal("no cursor drawn mid-drag")
	}
	want := compositor.toOutput(lerp(press, final, 0.5), compositor.crop(at))
	if math.Hypot(got.X-want.X, got.Y-want.Y) > 1 {
		t.Errorf("cursor mid-drag: want its dot at %+v, got %+v", want, got)
	}
}

func TestComposeDragInsideACutBeatDrawsNoCursor(t *testing.T) {
	t.Parallel()
	board := &storyboard.Storyboard{Beats: []storyboard.Beat{
		{ID: 1, Duration: 2 * sec},
		{ID: 2, Cut: true},
		{ID: 3, Duration: 2 * sec},
	}}
	events := []take.Event{beatStart(0, 1), beatStart(2*sec, 2)}
	events = append(events, dragAt(2500*time.Millisecond, 2, dragPress, dragMoves...)...)
	events = append(events, beatStart(4*sec, 3))
	recorded := &take.Take{Snapshots: []take.Snapshot{{At: 0}, {At: 6 * sec}}, Events: events}
	compositor, clipSchedule := newTestCompositor(t, board, recorded)

	if len(compositor.cursor.Marks) != 0 {
		t.Fatalf("cursor marks: want the cut drag dropped, got %+v", compositor.cursor.Marks)
	}
	blank := solidFrame(testBackground)
	for _, frame := range clipSchedule.Frames(10) {
		composed := compositor.Compose(frame.Out, blank)
		if _, count := centroidWhere(composed, func(c color.RGBA) bool { return c != testBackground }); count != 0 {
			t.Fatalf("frame at %s: want nothing drawn over the background, got %d pixels", frame.Out, count)
		}
	}
}

func TestComposeZoomAimsAtADragsLoggedBox(t *testing.T) {
	t.Parallel()
	handle := storyboard.Target{Text: "handle"}
	slot := storyboard.Target{Text: "slot"}
	fromBox := take.Box{X: dragPress.X, Y: dragPress.Y, W: 1, H: 1}
	toBox := take.Box{X: 14, Y: 2, W: 5, H: 1}

	for _, tc := range []struct {
		name   string
		zoomed storyboard.Target
		want   take.Box
	}{
		{name: "on the drag's from target", zoomed: handle, want: fromBox},
		{name: "on the drag's to target", zoomed: slot, want: toBox},
	} {
		t.Run(tc.name, func(t *testing.T) {
			drag := &storyboard.DragAction{From: handle, To: storyboard.DragEnd{Target: slot}}
			board := &storyboard.Storyboard{Beats: []storyboard.Beat{{
				ID: 1, Duration: 10 * sec,
				Do:   []storyboard.Action{{Drag: drag}},
				Zoom: &storyboard.Zoom{Target: tc.zoomed, Factor: 2},
			}}}
			dragged := dragAt(dragPressAt, 1, dragPress, dragMoves...)
			logged := toBox
			dragged = slices.Insert(dragged, 1, take.Event{
				At: dragPressAt, Kind: take.EventTarget, Beat: 1, Form: "drag", Text: dragToText, Box: &logged,
			})
			// No snapshot shows either target, so only the logged box can aim the zoom.
			recorded := &take.Take{
				Snapshots: []take.Snapshot{{At: 0}, {At: 10 * sec}},
				Events:    append([]take.Event{beatStart(0, 1)}, dragged...),
			}
			compositor, _ := newTestCompositor(t, board, recorded)

			want := testLayout.boxRect(tc.want).centre()
			if got := beatZoomAim(t, compositor); got != want {
				t.Errorf("pan centre: want the logged box's centre %+v, got %+v", want, got)
			}
		})
	}
}

func TestComposeDrawsACaptionOverTheCursor(t *testing.T) {
	t.Parallel()
	position := storyboard.Pixels{X: 40, Y: 40}
	covering := boxCaption("caption\nover the cursor", at(1, 0), 10*sec)
	covering.Place, covering.Align, covering.Position = "", "", &position
	board := &storyboard.Storyboard{
		Beats:    []storyboard.Beat{{ID: 1, Duration: 10 * sec}},
		Captions: []storyboard.Caption{covering},
	}
	events := append([]take.Event{beatStart(0, 1)}, dragAt(dragPressAt, 1, dragPress, dragMoves...)...)
	recorded := &take.Take{Snapshots: []take.Snapshot{{At: 0}, {At: 10 * sec}}, Events: events}
	captioned, _ := newTestCompositor(t, board, recorded)
	plain := newDragCompositor(t)
	src := solidFrame(testBackground)
	centre := plain.toOutput(plain.cursor.position(dragPressAt), plain.crop(dragPressAt))
	dot := image.Pt(int(centre.X), int(centre.Y))
	if got := plain.Compose(dragPressAt, src).RGBAAt(dot.X, dot.Y); got == testBackground {
		t.Fatalf("without the caption: want the cursor drawn at %v, got the background", dot)
	}
	box, layer := captionBox(t, captioned, dragPressAt)
	if !dot.In(box) {
		t.Fatalf("caption box %v: want it over the cursor at %v", box, dot)
	}

	frame := captioned.Compose(dragPressAt, src)

	assertSameRegion(t, frame, layer, box)
}
