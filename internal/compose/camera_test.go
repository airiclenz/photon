package compose

import (
	"math"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/storyboard"
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
