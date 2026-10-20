package compose

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/fonts"
	"github.com/airiclenz/photon/internal/schedule"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
)

// cameraLayout is a 1000 × 600 frame whose camera rests on its centre, (500, 300).
var cameraLayout = Layout{Size: testLayout.Size}

func TestZoomRampScalesGeometrically(t *testing.T) {
	t.Parallel()
	clipCamera := newCamera(cameraLayout, []move[float64]{
		{Start: 0, Over: 2 * sec, To: 4},
		{Start: 8 * sec, Over: 2 * sec, To: 1},
	}, nil)
	for _, tc := range []struct {
		name string
		at   time.Duration
		want float64
	}{
		{name: "before the push", at: -sec, want: 1},
		{name: "at the push's start", at: 0, want: 1},
		{name: "half-way up the push", at: sec, want: 2},
		{name: "at the top of the push", at: 2 * sec, want: 4},
		{name: "held", at: 5 * sec, want: 4},
		{name: "half-way down the pull", at: 9 * sec, want: 2},
		{name: "at the pull's end", at: 10 * sec, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := clipCamera.factorAt(tc.at); math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("factor at %s: want %g, got %g", tc.at, tc.want, got)
			}
		})
	}
}

func TestCameraZoomEasesInLogSpace(t *testing.T) {
	t.Parallel()
	clipCamera := newCamera(cameraLayout, []move[float64]{
		{Start: 0, To: 2},
		{Start: sec, Over: 2 * sec, To: 8},
	}, nil)

	got := clipCamera.factorAt(2 * sec)

	// Half-way from 2× to 8× by exponent is the geometric mean, 4×, not the arithmetic 5×.
	if math.Abs(got-4) > 1e-9 {
		t.Errorf("factor half-way from 2 to 8: want 4, got %g", got)
	}
}

func TestCameraZoomProgressRunsByItsEase(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		ease  storyboard.Ease
		share float64
	}{
		{name: "linear", ease: storyboard.EaseLinear, share: 0.25},
		{name: "smooth", ease: storyboard.EaseSmooth, share: 0.103515625},
		{name: "empty eases as smooth", ease: "", share: 0.103515625},
		{name: "ease-in", ease: storyboard.EaseIn, share: 0.015625},
		{name: "ease-out", ease: storyboard.EaseOut, share: 0.578125},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clipCamera := newCamera(cameraLayout, []move[float64]{
				{Start: 0, Over: 4 * sec, To: 16, Ease: tc.ease},
			}, nil)

			got := clipCamera.factorAt(sec)

			// The factor eases by exponent, so a quarter of the time in is 16 to the eased share.
			if want := math.Pow(16, tc.share); math.Abs(got-want) > 1e-9 {
				t.Errorf("factor a quarter through the zoom: want %g, got %g", want, got)
			}
		})
	}
}

func TestCameraZoomHoldsBetweenMoves(t *testing.T) {
	t.Parallel()
	clipCamera := newCamera(cameraLayout, []move[float64]{
		{Start: 4 * sec, Over: sec, To: 1.5},
		{Start: sec, Over: sec, To: 3},
	}, nil)
	for _, tc := range []struct {
		name string
		at   time.Duration
		want float64
	}{
		{name: "at rest before every move", at: 0, want: 1},
		{name: "between the moves", at: 3 * sec, want: 3},
		{name: "after the last move", at: 10 * sec, want: 1.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := clipCamera.factorAt(tc.at); math.Abs(got-tc.want) > 1e-9 {
				t.Errorf("factor at %s: want %g, got %g", tc.at, tc.want, got)
			}
		})
	}
}

// cameraTake is a storyboard of beats of the given lengths whose camera mapping is camera, and a
// take of the given snapshots over them, started end to end from source time 0, so each beat
// fits its duration and source and output clocks agree.
func cameraTake(
	t *testing.T,
	snapshots []take.Snapshot,
	camera string,
	durations ...time.Duration,
) (*storyboard.Storyboard, *take.Take) {
	t.Helper()
	var beats strings.Builder
	recorded := &take.Take{Snapshots: snapshots}
	var start time.Duration
	for index, duration := range durations {
		fmt.Fprintf(&beats, "  - {id: %d, title: beat %d, duration: %s}\n", index+1, index+1, duration)
		recorded.Events = append(recorded.Events, beatStart(start, index+1))
		start += duration
	}
	return parseBoard(t, "beats:\n"+beats.String()+"camera:\n"+camera), recorded
}

func TestComposeFollowsThePositionsAndZooms(t *testing.T) {
	t.Parallel()
	// alpha is on screen from 2.5s, where the position that aims at it starts at 3s; the screen
	// before shows it elsewhere.
	early := map[int]string{2: "     alpha"}
	late := map[int]string{7: "                              alpha"}
	// A held bar on a cell over [1s, 2s], a linear glide to alpha's held bar over [3s, 4s], a cut
	// to a point on a cell at 4s, a 2x push over [0s, 1s) and a pull back over [6s, 7s).
	const camera = `  positions:
    - {cell: {x: 24, y: 9}, at: {beat: 2}}
    - {cell: {x: 14, y: 4}, at: 1s, hold: 1s}
    - {text: alpha, at: 3s, hold: 1s, ease: linear}
  zooms:
    - {to: 2, at: 0s, over: 1s}
    - {to: 1, at: {beat: 2, offset: 2s}, over: 1s}
`
	board, recorded := cameraTake(t, []take.Snapshot{screen(0, early), screen(2500*time.Millisecond, late)}, camera, 4*sec, 4*sec)

	compositor, _ := newTestCompositor(t, board, recorded)

	held := testLayout.cellCentre(14, 4)
	alpha := testLayout.boxRect(take.Box{X: 30, Y: 7, W: 5, H: 1}).centre()
	point := testLayout.cellCentre(24, 9)
	for _, tc := range []struct {
		name       string
		at         time.Duration
		wantFactor float64
		wantCentre pointF
	}{
		{name: "before the first position, pushing in", at: 500 * time.Millisecond, wantFactor: math.Sqrt2, wantCentre: held},
		{name: "on the held bar", at: 1500 * time.Millisecond, wantFactor: 2, wantCentre: held},
		{name: "half-way through the glide", at: 2500 * time.Millisecond, wantFactor: 2, wantCentre: lerp(held, alpha, 0.5)},
		{name: "on the text target", at: 3500 * time.Millisecond, wantFactor: 2, wantCentre: alpha},
		{name: "cut to the point", at: 4 * sec, wantFactor: 2, wantCentre: point},
		{name: "after the last position, pulled back", at: 7 * sec, wantFactor: 1, wantCentre: point},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := compositor.camera.factorAt(tc.at); math.Abs(got-tc.wantFactor) > 1e-9 {
				t.Errorf("factor at %s: want %g, got %g", tc.at, tc.wantFactor, got)
			}
			want := cropAround(testLayout.Size, tc.wantFactor, tc.wantCentre).centre()
			if got := compositor.crop(tc.at).centre(); math.Abs(got.X-want.X) > 1e-6 || math.Abs(got.Y-want.Y) > 1e-6 {
				t.Errorf("crop centre at %s: want %+v, got %+v", tc.at, want, got)
			}
		})
	}
}

func TestComposePositionResolvesOnTheScreenAtItsStart(t *testing.T) {
	t.Parallel()
	early := map[int]string{2: "     alpha"}
	late := map[int]string{9: "                              alpha"}
	const camera = `  positions:
    - {text: alpha, at: 1s, hold: 500ms}
    - {text: alpha, at: 3s}
`
	board, recorded := cameraTake(t, []take.Snapshot{screen(0, early), screen(2*sec, late), screen(4*sec, late)}, camera, 4*sec)

	compositor, _ := newTestCompositor(t, board, recorded)

	for _, tc := range []struct {
		name string
		at   time.Duration
		want pointF
	}{
		{name: "the screen before the move", at: sec, want: testLayout.boxRect(take.Box{X: 5, Y: 2, W: 5, H: 1}).centre()},
		{name: "the screen after it", at: 3 * sec, want: testLayout.boxRect(take.Box{X: 30, Y: 9, W: 5, H: 1}).centre()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := compositor.camera.centreAt(tc.at); got != tc.want {
				t.Errorf("aim at %s: want %+v, got %+v", tc.at, tc.want, got)
			}
		})
	}
}

func TestNewRefusesAPositionTargetOffScreenAtItsStart(t *testing.T) {
	t.Parallel()
	shown := map[int]string{3: "     ghost"}
	// Shown before the position's start and after it, never on the screen shown at 1s.
	board, recorded := cameraTake(t,
		[]take.Snapshot{screen(0, shown), screen(500*time.Millisecond, nil), screen(1500*time.Millisecond, shown), screen(4*sec, shown)},
		"  positions:\n    - {text: ghost, at: 1s}\n",
		4*sec,
	)
	clipSchedule, err := schedule.New(board, recorded)
	if err != nil {
		t.Fatalf("schedule.New: %v", err)
	}

	_, err = New(board, recorded, clipSchedule, target.Resolver{}, testLayout, testBackground, fonts.CaptionSet{})

	if !errors.Is(err, target.ErrNotOnScreen) || !strings.HasPrefix(err.Error(), "compose: camera.positions[0]: target /ghost/") {
		t.Errorf("New: want camera.positions[0]'s target not on screen, got %v", err)
	}
	if got, want := storyboard.OwnerOf(err), (storyboard.Owner{Kind: storyboard.OwnerPosition, Index: 0}); got != want {
		t.Errorf("OwnerOf(New error) = %+v, want %+v", got, want)
	}
}

func TestComposeZoomWithoutPositionsAimsAtTheFrameCentre(t *testing.T) {
	t.Parallel()
	const camera = "  zooms:\n    - {to: 2, at: {beat: 1, offset: 1s}, over: 2s}\n"
	board, recorded := cameraTake(t, []take.Snapshot{{At: 0}, {At: 4 * sec}}, camera, 4*sec)

	compositor, _ := newTestCompositor(t, board, recorded)

	if got, want := compositor.camera.centreAt(3*sec), (pointF{X: 500, Y: 300}); got != want {
		t.Errorf("aim: want the frame's centre %+v, got %+v", want, got)
	}
}

func TestComposeCursorMapsThroughACropAimedByAPosition(t *testing.T) {
	t.Parallel()
	const camera = `  positions:
    - {cell: {x: 30, y: 8}, at: {beat: 1}}
  zooms:
    - {to: 2, at: {beat: 1}, over: 1s}
`
	board, recorded := cameraTake(t, []take.Snapshot{{At: 0}, {At: 4 * sec}}, camera, 4*sec)
	recorded.Events = append(recorded.Events, clickAt(3*sec, 1, take.Box{X: 33, Y: 9, W: 1, H: 1})...)
	compositor, _ := newTestCompositor(t, board, recorded)

	frame := compositor.Compose(3*sec, solidFrame(testBackground))

	// The crop is 500 × 300 source pixels centred on cell (30, 8)'s (620, 350), so its corner is
	// (370, 200), and the 500-pixel shipped frame shows it 1:1: cell (33, 9)'s centre (680, 390)
	// lands at (310, 190).
	bright := func(c color.RGBA) bool { return c.G > testBackground.G+0x30 }
	cursor, count := centroidWhere(frame, bright)
	if count == 0 {
		t.Fatal("no cursor drawn at the press")
	}
	if want := (pointF{X: 310, Y: 190}); math.Hypot(cursor.X-want.X, cursor.Y-want.Y) > 1 {
		t.Errorf("cursor at the press: want %+v through the aimed crop, got %+v", want, cursor)
	}
}
