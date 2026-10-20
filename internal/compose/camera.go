package compose

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/airiclenz/photon/internal/schedule"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
)

// move is one move of the zoom track on the output clock: from Start it eases the track from the
// value it holds then to To over Over; an Over of 0 sets To at Start. Ease picks the pace of its
// progress as [eased] runs it: smootherstep, the default when empty, linear, ease-in or ease-out.
type move[V any] struct {
	Start, Over time.Duration
	To          V
	Ease        storyboard.Ease
}

// camera is what the crop looks through: a zoom track of the magnification and a position track
// of where it aims. The zoom track is a list of moves sorted by start that never overlap; between
// moves it holds the factor its last move ended on, and before its first it rests at factor 1. A
// camera is built once and only read after, so it is safe for concurrent use.
type camera struct {
	zoom      []move[float64]
	positions positionTrack
}

// newCamera is the camera of zooms and positions over a frame laid out as layout, the zooms sorted
// by start and the positions laid on their track.
func newCamera(layout Layout, zooms []move[float64], positions []position) camera {
	sortByStart(zooms)
	return camera{zoom: zooms, positions: newPositionTrack(layout.Size, positions)}
}

// factorAt is the magnification at output time t. A zoom move eases the factor's exponent, so
// each frame of the move scales by the same ratio at the same eased pace — a linear factor would
// rush the start of a push-in and crawl at its end.
func (c camera) factorAt(t time.Duration) float64 {
	return trackAt(c.zoom, 1.0, t, func(m move[float64], from, share float64) float64 {
		return math.Exp(math.Log(from) + (math.Log(m.To)-math.Log(from))*share)
	})
}

// centreAt is the point the camera aims at at output time t, before the crop is clamped: the
// position track's aim, its glides run at the zoom track's factor.
func (c camera) centreAt(t time.Duration) pointF {
	return c.positions.centreAt(t, c.factorAt)
}

// progress is the eased share of the move done at output time t, clamped to [0, 1].
func (m move[V]) progress(t time.Duration) float64 {
	return eased(m.Ease, float64(t-m.Start)/float64(m.Over))
}

// eased is share, clamped to [0, 1], run through ease: linear keeps it, ease-in is share³ (slow
// to start), ease-out is 1−(1−share)³ (slow to settle) and smooth — also an empty ease — is
// smootherstep.
func eased(ease storyboard.Ease, share float64) float64 {
	clamped := clamp01(share)
	switch ease {
	case storyboard.EaseLinear:
		return clamped
	case storyboard.EaseIn:
		return clamped * clamped * clamped
	case storyboard.EaseOut:
		remaining := 1 - clamped
		return 1 - remaining*remaining*remaining
	default:
		return easeInOut(clamped)
	}
}

// sortByStart sorts a track's moves by start, keeping the given order of moves that start
// together.
func sortByStart[V any](moves []move[V]) {
	slices.SortStableFunc(moves, func(a, b move[V]) int { return cmp.Compare(a.Start, b.Start) })
}

// trackAt is the value at output time t of the track of moves resting at rest: the value the
// last move finished by t ended on, or, inside a move, blend of the move, the value it started
// from and its eased progress.
func trackAt[V any](
	moves []move[V],
	rest V,
	t time.Duration,
	blend func(m move[V], from V, share float64) V,
) V {
	value := rest
	for _, m := range moves {
		if t < m.Start {
			break
		}
		if t >= m.Start+m.Over {
			value = m.To
			continue
		}
		return blend(m, value, m.progress(t))
	}
	return value
}

// zoomPlanner plans the camera of one clip from its storyboard's positions and zooms, resolving
// the positions' text targets on the take's snapshots.
type zoomPlanner struct {
	board    *storyboard.Storyboard
	recorded *take.Take
	resolver target.Resolver
	layout   Layout
}

// plan plans the storyboard's zooms onto the zoom track, each starting at the start Parse
// resolved on the output clock and lasting its `over`, and its positions onto the position track,
// each aimed as [zoomPlanner.aim] resolves it. It fails, under camera.positions[i] as a
// [storyboard.LocatedError], on a text target the screen shown at its position's start does not
// show, wrapping [target.ErrNotOnScreen].
func (p zoomPlanner) plan(clipSchedule *schedule.Schedule) (camera, error) {
	boardCamera := p.board.Camera
	zooms := make([]move[float64], 0, len(boardCamera.Zooms))
	for _, zoom := range boardCamera.Zooms {
		zooms = append(zooms, move[float64]{
			Start: zoom.Start(), Over: zoom.Over, To: zoom.To, Ease: zoom.Ease,
		})
	}
	positions := make([]position, 0, len(boardCamera.Positions))
	for index, placed := range boardCamera.Positions {
		aim, err := p.aim(placed, clipSchedule)
		if err != nil {
			return camera{}, fmt.Errorf("compose: %w", storyboard.AtPosition(index, err))
		}
		positions = append(positions, position{
			Start: placed.Start(), End: placed.Start() + placed.Hold, Aim: aim, Ease: placed.Ease,
		})
	}
	return newCamera(p.layout, zooms, positions), nil
}

// aim is the source pixel a position aims at: its cell's centre, or the centre of the box its
// text target resolves to on the screen clipSchedule shows at the position's start — that screen
// only, so the aim never depends on what the take shows later.
func (p zoomPlanner) aim(
	placed storyboard.Position,
	clipSchedule *schedule.Schedule,
) (pointF, error) {
	if placed.Cell != nil {
		return p.layout.cellCentre(placed.Cell.X, placed.Cell.Y), nil
	}
	shown := p.recorded.ScreenAt(shownAt(clipSchedule, placed.Start()))
	if shown < 0 {
		return pointF{}, fmt.Errorf("target /%s/: the take has no snapshot", placed.Text)
	}
	box, err := p.resolver.Target(p.recorded.Snapshots[shown], placed.Target)
	if err != nil {
		return pointF{}, err
	}
	return p.layout.boxRect(box).centre(), nil
}

// shownAt is the source time of the screen clipSchedule shows at output time out, clamped into
// the clip's [0, Length).
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
