package compose

import (
	"cmp"
	"math"
	"slices"
	"time"
)

// move is one move of a camera track on the output clock: from Start it eases the track from the
// value it holds then to To over Over; an Over of 0 sets To at Start.
type move[V any] struct {
	Start, Over time.Duration
	To          V
}

// camera is what the crop looks through: a zoom track of the magnification and a pan track of the
// crop's centre in source pixels. Each track is a list of moves sorted by start that never
// overlap; between moves a track holds the value its last move ended on, and before its first it
// rests at factor 1 and the frame's centre. A camera is built once and only read after, so it is
// safe for concurrent use.
type camera struct {
	zoom []move[float64]
	pan  []move[pointF]
	rest pointF
}

// newCamera is the camera of zooms and pans over a frame laid out as layout, each track sorted
// by start.
func newCamera(layout Layout, zooms []move[float64], pans []move[pointF]) camera {
	sortByStart(zooms)
	sortByStart(pans)
	return camera{
		zoom: zooms,
		pan:  pans,
		rest: pointF{X: float64(layout.Size.X) / 2, Y: float64(layout.Size.Y) / 2},
	}
}

// factorAt is the magnification at output time t. A zoom move eases the factor's exponent, so
// each frame of the move scales by the same ratio at the same eased pace — a linear factor would
// rush the start of a push-in and crawl at its end.
func (c camera) factorAt(t time.Duration) float64 {
	return trackAt(c.zoom, 1, t, func(from, to, share float64) float64 {
		return math.Exp(math.Log(from) + (math.Log(to)-math.Log(from))*share)
	})
}

// centreAt is the crop's centre at output time t, eased along a straight line through a pan move.
func (c camera) centreAt(t time.Duration) pointF {
	return trackAt(c.pan, c.rest, t, lerp)
}

// sortByStart sorts a track's moves by start, keeping the given order of moves that start
// together.
func sortByStart[V any](moves []move[V]) {
	slices.SortStableFunc(moves, func(a, b move[V]) int { return cmp.Compare(a.Start, b.Start) })
}

// trackAt is the value at output time t of the track of moves resting at rest: the value the
// last move finished by t ended on, or, inside a move, blend of the value it started from and its
// own at the move's smootherstep-eased progress.
func trackAt[V any](moves []move[V], rest V, t time.Duration, blend func(from, to V, share float64) V) V {
	value := rest
	for _, m := range moves {
		if t < m.Start {
			break
		}
		if t >= m.Start+m.Over {
			value = m.To
			continue
		}
		return blend(value, m.To, easeInOut(float64(t-m.Start)/float64(m.Over)))
	}
	return value
}
