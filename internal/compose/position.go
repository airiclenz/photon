package compose

import (
	"cmp"
	"image"
	"slices"
	"time"

	"github.com/airiclenz/photon/internal/storyboard"
)

// position is one position of the position track on the output clock: the camera aims at Aim, a
// point in source pixels, from Start to End. Ease is the pace of the transition into it from the
// position before; the first position's Ease is never used.
type position struct {
	Start, End time.Duration
	Aim        pointF
	Ease       storyboard.Ease
}

// positionTrack is where the camera aims over a frame of size: a list of positions sorted by
// start. Before the first position it aims at the first's aim, during a position's hold at that
// aim, and after the last at the last's aim; across the gap between two positions it glides from
// the centre of the crop shown as the earlier one ends to the centre of the crop shown as the
// later one starts, eased by the later one's ease, and a gap of 0 is a cut. With no positions it
// aims at the frame's centre. A track is built once and only read after, so it is safe for
// concurrent use.
type positionTrack struct {
	positions []position
	size      image.Point
}

// newPositionTrack is the track of positions over a frame of size, sorted by start in a copy so
// the caller's list keeps its order; positions that start together keep the given order.
func newPositionTrack(size image.Point, positions []position) positionTrack {
	sorted := slices.Clone(positions)
	slices.SortStableFunc(sorted, func(a, b position) int { return cmp.Compare(a.Start, b.Start) })
	return positionTrack{positions: sorted, size: size}
}

// centreAt is the point the camera aims at at output time t, before the crop is clamped, given
// the zoom track's factorAt. A glide runs between the crop centres shown at its two ends, each
// clamped at the factor of its moment, so a crop pinned against an edge starts moving as the glide
// starts and is still moving until it ends.
func (p positionTrack) centreAt(t time.Duration, factorAt func(time.Duration) float64) pointF {
	if len(p.positions) == 0 {
		return pointF{X: float64(p.size.X) / 2, Y: float64(p.size.Y) / 2}
	}
	// started counts the positions that have started by t: the search never matches, so it lands
	// on the first position still to come.
	started, _ := slices.BinarySearchFunc(
		p.positions,
		t,
		func(entry position, at time.Duration) int {
			if entry.Start <= at {
				return -1
			}
			return 1
		},
	)
	if started == 0 {
		return p.positions[0].Aim
	}
	current := p.positions[started-1]
	if t <= current.End || started == len(p.positions) {
		return current.Aim
	}
	next := p.positions[started]
	from := cropAround(p.size, factorAt(current.End), current.Aim).centre()
	to := cropAround(p.size, factorAt(next.Start), next.Aim).centre()
	share := float64(t-current.End) / float64(next.Start-current.End)
	return lerp(from, to, eased(next.Ease, share))
}
