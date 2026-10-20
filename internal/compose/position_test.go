package compose

import (
	"math"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/storyboard"
)

// doubled is a zoom track held at 2×: its crop is 500 × 300 on [cameraLayout], so a crop centre
// clamps into [250, 750] × [150, 450].
func doubled(time.Duration) float64 { return 2 }

// Three positions on [cameraLayout]: first holds 1s–3s, a glide runs 3s–5s into second, which
// holds to 6s, where third cuts in and holds to 8s. Every aim sits where a 2× crop is not clamped.
var (
	firstPosition  = position{Start: sec, End: 3 * sec, Aim: pointF{X: 300, Y: 200}}
	secondPosition = position{Start: 5 * sec, End: 6 * sec, Aim: pointF{X: 700, Y: 400}}
	thirdPosition  = position{Start: 6 * sec, End: 8 * sec, Aim: pointF{X: 400, Y: 350}}
)

// assertNear fails when got is not want within a billionth of a pixel on either axis.
func assertNear(t *testing.T, label string, want, got pointF) {
	t.Helper()
	if math.Abs(got.X-want.X) > 1e-9 || math.Abs(got.Y-want.Y) > 1e-9 {
		t.Errorf("%s: want %+v, got %+v", label, want, got)
	}
}

func TestPositionTrackHoldsCutsAndRests(t *testing.T) {
	t.Parallel()
	track := newPositionTrack(cameraLayout.Size, []position{firstPosition, secondPosition, thirdPosition})
	for _, tc := range []struct {
		name string
		at   time.Duration
		want pointF
	}{
		{name: "before the first", at: 0, want: firstPosition.Aim},
		{name: "at the first's start", at: sec, want: firstPosition.Aim},
		{name: "holding the first", at: 2 * sec, want: firstPosition.Aim},
		{name: "at the first's end", at: 3 * sec, want: firstPosition.Aim},
		{name: "at the glide's end", at: 5 * sec, want: secondPosition.Aim},
		{name: "just before the cut", at: 6*sec - time.Millisecond, want: secondPosition.Aim},
		{name: "at the cut", at: 6 * sec, want: thirdPosition.Aim},
		{name: "after the last", at: 9 * sec, want: thirdPosition.Aim},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := track.centreAt(tc.at, doubled)

			assertNear(t, "centre at "+tc.at.String(), tc.want, got)
		})
	}
}

func TestPositionTrackGlidesByTheNextPositionsEase(t *testing.T) {
	t.Parallel()
	// A quarter of the way through the glide, where smootherstep and linear differ.
	quarter := firstPosition.End + (secondPosition.Start-firstPosition.End)/4
	for _, tc := range []struct {
		ease  storyboard.Ease
		share float64
	}{
		{ease: storyboard.EaseLinear, share: 0.25},
		{ease: storyboard.EaseSmooth, share: 0.103515625},
		{ease: storyboard.EaseIn, share: 0.015625},
		{ease: storyboard.EaseOut, share: 0.578125},
	} {
		t.Run(string(tc.ease), func(t *testing.T) {
			t.Parallel()
			next := secondPosition
			next.Ease = tc.ease
			track := newPositionTrack(cameraLayout.Size, []position{firstPosition, next})

			got := track.centreAt(quarter, doubled)

			want := lerp(firstPosition.Aim, next.Aim, tc.share)
			assertNear(t, "centre a quarter through the glide", want, got)
		})
	}
}

func TestPositionTrackWithoutPositionsAimsAtTheFrameCentre(t *testing.T) {
	t.Parallel()
	track := newPositionTrack(cameraLayout.Size, nil)

	got := track.centreAt(2*sec, doubled)

	assertNear(t, "centre with no positions", pointF{X: 500, Y: 300}, got)
}

func TestPositionTrackGlidesBetweenClampedCropsWithoutDeadTime(t *testing.T) {
	t.Parallel()
	// Both aims sit in opposite corners, beyond where a 2× crop can centre.
	corner := position{Start: 0, End: sec, Aim: pointF{X: 0, Y: 0}}
	opposite := position{
		Start: 3 * sec, End: 4 * sec, Aim: pointF{X: 1000, Y: 600}, Ease: storyboard.EaseLinear,
	}
	track := newPositionTrack(cameraLayout.Size, []position{corner, opposite})
	justAfterStart, justBeforeEnd := corner.End+20*time.Millisecond, opposite.Start-20*time.Millisecond

	early := cropAround(cameraLayout.Size, 2, track.centreAt(justAfterStart, doubled))
	late := cropAround(cameraLayout.Size, 2, track.centreAt(justBeforeEnd, doubled))

	if early.X <= 0 || early.Y <= 0 {
		t.Errorf("crop just after the glide starts: want it off the top-left edge, got %+v", early)
	}
	if late.X >= 500 || late.Y >= 300 {
		t.Errorf("crop just before the glide ends: want it short of the bottom-right edge, got %+v", late)
	}
}

func TestPositionTrackSortsPositionsGivenOutOfOrder(t *testing.T) {
	t.Parallel()
	ordered := newPositionTrack(cameraLayout.Size, []position{firstPosition, secondPosition, thirdPosition})
	shuffled := newPositionTrack(cameraLayout.Size, []position{thirdPosition, firstPosition, secondPosition})
	for _, at := range []time.Duration{0, 2 * sec, 4 * sec, 5500 * time.Millisecond, 7 * sec} {
		t.Run(at.String(), func(t *testing.T) {
			t.Parallel()

			got := shuffled.centreAt(at, doubled)

			assertNear(t, "centre at "+at.String(), ordered.centreAt(at, doubled), got)
		})
	}
}
