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

func TestCameraPanEasesAlongAStraightLine(t *testing.T) {
	t.Parallel()
	clipCamera := newCamera(cameraLayout, nil, []move[pointF]{
		{Start: sec, Over: 2 * sec, To: pointF{X: 700, Y: 100}},
	})

	got := clipCamera.centreAt(2 * sec)

	// The ease is symmetric, so half the time is half the way from the rest centre.
	if want := (pointF{X: 600, Y: 200}); math.Abs(got.X-want.X) > 1e-9 || math.Abs(got.Y-want.Y) > 1e-9 {
		t.Errorf("centre half-way through the pan: want %+v, got %+v", want, got)
	}
}

func TestCameraTracksHoldBetweenMoves(t *testing.T) {
	t.Parallel()
	first, second := pointF{X: 200, Y: 150}, pointF{X: 800, Y: 450}
	clipCamera := newCamera(cameraLayout, []move[float64]{
		{Start: 4 * sec, Over: sec, To: 1.5},
		{Start: sec, Over: sec, To: 3},
	}, []move[pointF]{
		{Start: 5 * sec, Over: sec, To: second},
		{Start: sec, To: first},
	})
	for _, tc := range []struct {
		name       string
		at         time.Duration
		wantFactor float64
		wantCentre pointF
	}{
		{name: "at rest before every move", at: 0, wantFactor: 1, wantCentre: pointF{X: 500, Y: 300}},
		{name: "between the moves", at: 3 * sec, wantFactor: 3, wantCentre: first},
		{name: "after the last moves", at: 10 * sec, wantFactor: 1.5, wantCentre: second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := clipCamera.factorAt(tc.at); math.Abs(got-tc.wantFactor) > 1e-9 {
				t.Errorf("factor at %s: want %g, got %g", tc.at, tc.wantFactor, got)
			}
			if got := clipCamera.centreAt(tc.at); got != tc.wantCentre {
				t.Errorf("centre at %s: want %+v, got %+v", tc.at, tc.wantCentre, got)
			}
		})
	}
}

func TestCameraPanFollowsItsPath(t *testing.T) {
	t.Parallel()
	rest, peak, side := pointF{X: 500, Y: 300}, pointF{X: 700, Y: 100}, pointF{X: 900, Y: 300}
	for _, tc := range []struct {
		name string
		pans []move[pointF]
		at   time.Duration
		want pointF
	}{
		{
			name: "half-way by length is the waypoint of a symmetric path",
			pans: []move[pointF]{{Start: sec, Over: 2 * sec, Via: []pointF{peak}, To: side}},
			at:   2 * sec,
			want: peak,
		},
		{
			name: "a linear ease keeps an even pace",
			pans: []move[pointF]{{Start: 0, Over: 4 * sec, To: side, Ease: storyboard.EaseLinear}},
			at:   sec,
			want: pointF{X: 600, Y: 300},
		},
		{
			name: "a smooth ease starts slow",
			pans: []move[pointF]{{Start: 0, Over: 4 * sec, To: side, Ease: storyboard.EaseSmooth}},
			at:   sec,
			want: lerp(rest, side, easeInOut(0.25)),
		},
		{
			name: "a pan starts where the previous one ended",
			pans: []move[pointF]{{Start: 3 * sec, Over: 2 * sec, To: rest}, {Start: 0, Over: sec, To: side}},
			at:   4 * sec,
			want: pointF{X: 700, Y: 300},
		},
		{
			name: "a pan to the centre it holds stays still",
			pans: []move[pointF]{{Start: 0, Over: 2 * sec, Via: []pointF{rest}, To: rest}},
			at:   sec,
			want: rest,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clipCamera := newCamera(cameraLayout, nil, tc.pans)

			got := clipCamera.centreAt(tc.at)

			if math.Abs(got.X-tc.want.X) > 1e-6 || math.Abs(got.Y-tc.want.Y) > 1e-6 {
				t.Errorf("centre at %s: want %+v, got %+v", tc.at, tc.want, got)
			}
		})
	}
}

// cameraTake is a storyboard of beats of the given lengths whose camera list is camera, and a
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

func TestComposePlansOverlappingCameraMovesAcrossBeats(t *testing.T) {
	t.Parallel()
	lines := map[int]string{2: "     alpha", 11: "                              omega"}
	const camera = `  - zoom: {to: 2, at: {beat: 1, offset: 1s}, over: 2s}
  - pan: {to: {text: alpha}, at: {beat: 1, offset: 2s}, over: 2s}
  - pan: {via: [{cell: {x: 10, y: 3}}, {text: omega}], at: {beat: 2, offset: 1s}, over: 3s}
  - zoom: {to: 1, at: {beat: 2, offset: 2s}, over: 4s}
`
	board, recorded := cameraTake(t, []take.Snapshot{screen(0, lines), screen(12*sec, lines)}, camera, 4*sec, 4*sec, 4*sec)

	compositor, _ := newTestCompositor(t, board, recorded)

	rest := pointF{X: 500, Y: 300}
	alpha := testLayout.boxRect(take.Box{X: 5, Y: 2, W: 5, H: 1}).centre()
	omega := testLayout.boxRect(take.Box{X: 30, Y: 11, W: 5, H: 1}).centre()
	curve := newPath(alpha, []pointF{testLayout.cellCentre(10, 3), omega})
	for _, tc := range []struct {
		name       string
		at         time.Duration
		wantFactor float64
		wantCentre pointF
	}{
		{name: "zoom-in starts", at: sec, wantFactor: 1, wantCentre: rest},
		{name: "zoom-in half-way, first pan starts", at: 2 * sec, wantFactor: math.Sqrt2, wantCentre: rest},
		{name: "zoom-in ends, first pan half-way", at: 3 * sec, wantFactor: 2, wantCentre: lerp(rest, alpha, 0.5)},
		{name: "first pan ends on the next beat", at: 4 * sec, wantFactor: 2, wantCentre: alpha},
		{name: "via pan starts", at: 5 * sec, wantFactor: 2, wantCentre: alpha},
		{name: "zoom-out starts during the via pan", at: 6 * sec, wantFactor: 2, wantCentre: curve.at(easeInOut(1.0 / 3))},
		{
			name:       "via pan half-way",
			at:         6500 * time.Millisecond,
			wantFactor: math.Pow(2, 1-easeInOut(0.125)),
			wantCentre: curve.at(0.5),
		},
		{name: "via pan ends, zoom-out half-way", at: 8 * sec, wantFactor: math.Sqrt2, wantCentre: omega},
		{name: "zoom-out ends in the third beat", at: 10 * sec, wantFactor: 1, wantCentre: omega},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := compositor.camera.factorAt(tc.at); math.Abs(got-tc.wantFactor) > 1e-9 {
				t.Errorf("factor at %s: want %g, got %g", tc.at, tc.wantFactor, got)
			}
			got := compositor.camera.centreAt(tc.at)
			if math.Abs(got.X-tc.wantCentre.X) > 1e-6 || math.Abs(got.Y-tc.wantCentre.Y) > 1e-6 {
				t.Errorf("centre at %s: want %+v, got %+v", tc.at, tc.wantCentre, got)
			}
		})
	}
}

func TestComposePanResolvesOnTheScreenAtItsEnd(t *testing.T) {
	t.Parallel()
	early := map[int]string{2: "     alpha"}
	late := map[int]string{9: "                              alpha"}
	wantLate := testLayout.boxRect(take.Box{X: 30, Y: 9, W: 5, H: 1}).centre()
	for _, tc := range []struct {
		name      string
		snapshots []take.Snapshot
	}{
		{
			name:      "moved by the move's end",
			snapshots: []take.Snapshot{screen(0, early), screen(2*sec, late), screen(8*sec, late)},
		},
		{
			name:      "gone by the move's end, shown at its start",
			snapshots: []take.Snapshot{screen(0, early), screen(sec, late), screen(2*sec, nil), screen(8*sec, nil)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			const camera = "  - pan: {to: {text: alpha}, at: {beat: 1, offset: 1500ms}, over: 1500ms}\n"
			board, recorded := cameraTake(t, tc.snapshots, camera, 8*sec)

			compositor, _ := newTestCompositor(t, board, recorded)

			if got := compositor.camera.pan[0].To; got != wantLate {
				t.Errorf("pan destination: want %+v, got %+v", wantLate, got)
			}
		})
	}
}

func TestNewRefusesAPanTargetItsMoveNeverShows(t *testing.T) {
	t.Parallel()
	shown := map[int]string{3: "     ghost"}
	// Shown before the move and from the second beat on: never across the move's [1s, 2s].
	board, recorded := cameraTake(t,
		[]take.Snapshot{screen(0, shown), screen(500*time.Millisecond, nil), screen(4*sec, shown)},
		"  - pan: {to: {text: ghost}, at: {beat: 1, offset: 1s}, over: 1s}\n",
		4*sec, 4*sec,
	)
	clipSchedule, err := schedule.New(board, recorded)
	if err != nil {
		t.Fatalf("schedule.New: %v", err)
	}

	_, err = New(board, recorded, clipSchedule, target.Resolver{}, testLayout, testBackground, fonts.CaptionSet{})

	if !errors.Is(err, target.ErrNotOnScreen) || !strings.HasPrefix(err.Error(), "compose: camera[0]: pan: to") {
		t.Errorf("New: want camera[0]'s pan target not on screen, got %v", err)
	}
	if got, want := storyboard.OwnerOf(err), (storyboard.Owner{Kind: storyboard.OwnerCamera, Index: 0}); got != want {
		t.Errorf("OwnerOf(New error) = %+v, want %+v", got, want)
	}
}

func TestComposeCursorMapsThroughAPannedCrop(t *testing.T) {
	t.Parallel()
	const camera = `  - zoom: {to: 2, at: {beat: 1}, over: 1s}
  - pan: {to: {cell: {x: 30, y: 8}}, at: {beat: 1}, over: 1s}
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
		t.Errorf("cursor at the press: want %+v through the panned crop, got %+v", want, cursor)
	}
}
