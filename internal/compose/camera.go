package compose

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/airiclenz/photon/internal/storyboard"
)

// move is one move of a camera track on the output clock: from Start it eases the track from the
// value it holds then to To over Over; an Over of 0 sets To at Start. Ease picks the pace of its
// progress: smootherstep, the default when empty, or linear. Via lists the points a pan passes
// through on its way to To, in order; a zoom move leaves it empty.
type move[V any] struct {
	Start, Over time.Duration
	To          V
	Via         []V
	Ease        storyboard.Ease
}

// panSegment is a move of the pan track with the path it carries the crop's centre along: from
// the centre the track holds at the move's start, through the move's Via, to its To.
type panSegment struct {
	move[pointF]
	route path
}

// camera is what the crop looks through: a zoom track of the magnification and a pan track of the
// crop's centre in source pixels. Each track is a list of moves sorted by start that never
// overlap; between moves a track holds the value its last move ended on, and before its first it
// rests at factor 1 and the frame's centre. A camera is built once and only read after, so it is
// safe for concurrent use.
type camera struct {
	zoom []move[float64]
	pan  []panSegment
	rest pointF
}

// newCamera is the camera of zooms and pans over a frame laid out as layout, each track sorted
// by start. Each pan's path is laid once here, from where the pan track rests or the previous pan
// ended.
func newCamera(layout Layout, zooms []move[float64], pans []move[pointF]) camera {
	sortByStart(zooms)
	sortByStart(pans)
	rest := pointF{X: float64(layout.Size.X) / 2, Y: float64(layout.Size.Y) / 2}
	segments := make([]panSegment, 0, len(pans))
	held := rest
	for _, pan := range pans {
		route := newPath(held, append(slices.Clone(pan.Via), pan.To))
		segments = append(segments, panSegment{move: pan, route: route})
		held = pan.To
	}
	return camera{zoom: zooms, pan: segments, rest: rest}
}

// factorAt is the magnification at output time t. A zoom move eases the factor's exponent, so
// each frame of the move scales by the same ratio at the same eased pace — a linear factor would
// rush the start of a push-in and crawl at its end.
func (c camera) factorAt(t time.Duration) float64 {
	return trackAt(c.zoom, 1.0, t, func(m move[float64], from, share float64) float64 {
		return math.Exp(math.Log(from) + (math.Log(m.To)-math.Log(from))*share)
	})
}

// centreAt is the crop's centre at output time t, carried along a pan move's path by arc length.
func (c camera) centreAt(t time.Duration) pointF {
	return trackAt(c.pan, c.rest, t, func(segment panSegment, _ pointF, share float64) pointF {
		return segment.route.at(share)
	})
}

// timing is the move itself, the part of a track's entry trackAt walks by.
func (m move[V]) timing() move[V] { return m }

// progress is the eased share of the move done at output time t, clamped to [0, 1].
func (m move[V]) progress(t time.Duration) float64 {
	share := clamp01(float64(t-m.Start) / float64(m.Over))
	if m.Ease == storyboard.EaseLinear {
		return share
	}
	return easeInOut(share)
}

// sortByStart sorts a track's moves by start, keeping the given order of moves that start
// together.
func sortByStart[V any](moves []move[V]) {
	slices.SortStableFunc(moves, func(a, b move[V]) int { return cmp.Compare(a.Start, b.Start) })
}

// trackAt is the value at output time t of the track of entries resting at rest: the value the
// last move finished by t ended on, or, inside a move, blend of its entry, the value it started
// from and its eased progress.
func trackAt[V any, M interface{ timing() move[V] }](
	entries []M,
	rest V,
	t time.Duration,
	blend func(entry M, from V, share float64) V,
) V {
	value := rest
	for _, entry := range entries {
		m := entry.timing()
		if t < m.Start {
			break
		}
		if t >= m.Start+m.Over {
			value = m.To
			continue
		}
		return blend(entry, value, m.progress(t))
	}
	return value
}
