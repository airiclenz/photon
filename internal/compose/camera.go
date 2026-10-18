package compose

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/airiclenz/photon/internal/schedule"
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

// planMoves plans every camera move of the storyboard onto the camera's tracks, each starting at
// its `at` on the output clock and lasting its `over`. A pan's points resolve as
// [zoomPlanner.panPoint] does, on the screens clipSchedule shows across the move. It fails,
// under camera[i] as a [storyboard.LocatedError], on a move whose start is not on the clip's
// clock and on a pan target that no screen of its move shows, wrapping [target.ErrNotOnScreen].
func (p zoomPlanner) planMoves(clipSchedule *schedule.Schedule) (zooms []move[float64], pans []move[pointF], err error) {
	clock := p.board.Clock()
	for index, cameraMove := range p.board.Camera {
		switch {
		case cameraMove.Zoom != nil:
			zoom := cameraMove.Zoom
			start, err := clock.Resolve(zoom.At)
			if err != nil {
				return nil, nil, fmt.Errorf("compose: %w", storyboard.AtCamera(index, fmt.Errorf("zoom: at: %w", err)))
			}
			zooms = append(zooms, move[float64]{Start: start, Over: zoom.Over, To: zoom.To, Ease: zoom.Ease})
		case cameraMove.Pan != nil:
			pan, err := p.planPan(clock, *cameraMove.Pan, clipSchedule)
			if err != nil {
				return nil, nil, fmt.Errorf("compose: %w", storyboard.AtCamera(index, fmt.Errorf("pan: %w", err)))
			}
			pans = append(pans, pan)
		}
	}
	return zooms, pans, nil
}

// planPan is the pan move on the output clock with its points resolved to source pixels: its To,
// or the last of its Via as the destination and the rest as the waypoints between. Where the
// path starts is the camera's business: the centre its pan track holds at the move's start. Its
// start resolves on clock, the board's output clock.
func (p zoomPlanner) planPan(clock storyboard.OutputClock, pan storyboard.PanMove, clipSchedule *schedule.Schedule) (move[pointF], error) {
	start, err := clock.Resolve(pan.At)
	if err != nil {
		return move[pointF]{}, fmt.Errorf("at: %w", err)
	}
	since, until := shownAt(clipSchedule, start), shownAt(clipSchedule, start+pan.Over)
	points := pan.Via
	if pan.To != nil {
		points = []storyboard.Point{*pan.To}
	}
	if len(points) == 0 {
		return move[pointF]{}, errors.New("want a to or a via")
	}
	centres := make([]pointF, 0, len(points))
	for index, point := range points {
		centre, err := p.panPoint(point, since, until)
		if err != nil && pan.To != nil {
			return move[pointF]{}, fmt.Errorf("to: %w", err)
		}
		if err != nil {
			return move[pointF]{}, fmt.Errorf("via[%d]: %w", index, err)
		}
		centres = append(centres, centre)
	}
	last := len(centres) - 1
	return move[pointF]{Start: start, Over: pan.Over, To: centres[last], Via: centres[:last], Ease: pan.Ease}, nil
}

// panPoint is the source pixel a pan's point aims at: a cell's centre, or the centre of the box
// its target resolves to on the screen shown at source time until — the move's end — walking
// back to the screen shown at source time since, the move's start, while the target is absent.
func (p zoomPlanner) panPoint(point storyboard.Point, since, until time.Duration) (pointF, error) {
	if point.Cell != nil {
		return p.layout.cellCentre(point.Cell.X, point.Cell.Y), nil
	}
	box, err := p.resolveShown(point.Target, p.recorded.ScreensThrough(since, until), "the move")
	if err != nil {
		return pointF{}, err
	}
	return p.layout.boxRect(box).centre(), nil
}

// shownAt is the source time of the screen clipSchedule shows at output time out, clamped into
// the clip's [0, Length), so a move that ends as the clip ends resolves on its last frame.
func shownAt(clipSchedule *schedule.Schedule, out time.Duration) time.Duration {
	clamped := min(max(out, 0), clipSchedule.Length-time.Nanosecond)
	// Every instant of [0, Length) maps onto a source time, so only an empty clip reports false,
	// and its source time 0 is as good as any.
	source, _, isShown := clipSchedule.Source(clamped)
	if !isShown {
		return 0
	}
	return source
}
