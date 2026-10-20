package storyboard

import (
	"cmp"
	"fmt"
	"slices"
	"time"
)

// Camera is the clip's camera on two tracks: Positions say where it aims and Zooms how far it
// magnifies. Either list may be given in any order; validation places each entry on the clip's
// output clock and refuses two positions, or two zooms, that overlap.
type Camera struct {
	Positions []Position `yaml:"positions"`
	Zooms     []ZoomMove `yaml:"zooms"`
}

// ZoomMove changes the magnification to To, a factor in [1, 3], starting At and lasting Over. It
// holds the zoom track only and aims nowhere: the camera's positions aim it.
type ZoomMove struct {
	To   float64       `yaml:"to"`
	At   Moment        `yaml:"at"`
	Over time.Duration `yaml:"over"`
	Ease Ease          `yaml:"ease"`

	start time.Duration
}

// Start is where the zoom move starts on the clip's output clock: its At, resolved.
func (z ZoomMove) Start() time.Duration { return z.start }

// Point is where a camera position aims: either an on-screen target (its fields sit inline, so
// `{text: …}` reads as a target) or a Cell of the terminal grid. Exactly one of Text and Cell is
// set.
type Point struct {
	Target `yaml:",inline"`
	Cell   *Cell `yaml:"cell"`
}

// Cell is one cell of the terminal grid: column X and row Y, both 0-based.
type Cell struct {
	X int `yaml:"x"`
	Y int `yaml:"y"`
}

// Ease is how a zoom's progress, or a transition's into a position, runs over its length. [Load]
// sets an absent one on a position or a zoom to [EaseSmooth].
type Ease string

// The spellings of Ease: [EaseLinear] runs at an even pace, [EaseSmooth] starts and ends gently,
// [EaseIn] starts slow and [EaseOut] ends slow.
const (
	EaseSmooth Ease = "smooth"
	EaseLinear Ease = "linear"
	EaseIn     Ease = "ease-in"
	EaseOut    Ease = "ease-out"
)

// validateEase checks that an ease, when set, is one of the four spellings. Problems are filed
// under ease.
func validateEase(ease Ease, report reporter) {
	if ease == "" || slices.Contains([]Ease{EaseSmooth, EaseLinear, EaseIn, EaseOut}, ease) {
		return
	}
	report("ease: want %s, %s, %s or %s, got %q", EaseSmooth, EaseLinear, EaseIn, EaseOut, ease)
}

// span is a stretch [start, end) of the clip's output clock that holds a camera track, with the
// name validation gives its holder.
type span struct {
	holder     string
	start, end time.Duration
}

// overlaps reports whether the two half-open stretches share any time.
func (s span) overlaps(other span) bool {
	return s.start < other.end && other.start < s.end
}

// String spells the stretch as [start, end).
func (s span) String() string {
	return fmt.Sprintf("[%s, %s)", s.start, s.end)
}

// heldSpan is the stretch one camera entry holds, with the owner its problems are filed under.
type heldSpan struct {
	owner Owner
	span
}

// reportOverlaps files every two entries of one track that overlap under the later of them by
// start, naming the earlier. Two entries that start together overlap even when one holds no time.
// It sorts held by start, keeping list order between equal starts.
func reportOverlaps(held []heldSpan, collect collector) {
	slices.SortStableFunc(held, func(a, b heldSpan) int { return cmp.Compare(a.start, b.start) })
	for later, entry := range held {
		for _, earlier := range held[:later] {
			if earlier.start == entry.start || earlier.overlaps(entry.span) {
				collect.at(entry.owner)("overlaps %s (%s and %s)",
					earlier.holder, earlier.span, entry.span)
			}
		}
	}
}

// validate checks every position and zoom against frame and the clip's output clock, keeping the
// start each resolves, then that no two positions and no two zooms overlap. Problems are filed
// under camera.positions[i] or camera.zooms[i]; an overlap under the later of the two by start.
func (c Camera) validate(frame Frame, clock OutputClock, collect collector) {
	positions := make([]heldSpan, 0, len(c.Positions))
	for index := range c.Positions {
		position := &c.Positions[index]
		owner := Owner{Kind: OwnerPosition, Index: index}
		placed, isPlaced := position.validate(frame, clock, collect.at(owner))
		if !isPlaced {
			continue
		}
		position.start = placed.start
		placed.holder = fmt.Sprintf("camera.positions[%d]", index)
		positions = append(positions, heldSpan{owner: owner, span: placed})
	}
	reportOverlaps(positions, collect)

	zooms := make([]heldSpan, 0, len(c.Zooms))
	for index := range c.Zooms {
		zoom := &c.Zooms[index]
		owner := Owner{Kind: OwnerZoom, Index: index}
		report := collect.at(owner)
		zoom.validate(report)
		placed, isPlaced := placeMove(clock, moveTiming{zoom.At, zoom.Over, zoom.Ease}, report)
		if !isPlaced {
			continue
		}
		zoom.start = placed.start
		placed.holder = fmt.Sprintf("camera.zooms[%d]", index)
		zooms = append(zooms, heldSpan{owner: owner, span: placed})
	}
	reportOverlaps(zooms, collect)
}

// moveTiming is the part of a zoom that places it on the clip's clock: where it starts, how long
// it runs and how it eases.
type moveTiming struct {
	at   Moment
	over time.Duration
	ease Ease
}

// validate checks the zoom move's factor.
func (z ZoomMove) validate(report reporter) {
	if z.To < minZoomFactor || z.To > maxZoomFactor {
		report("to: want in [%d, %d], got %g", minZoomFactor, maxZoomFactor, z.To)
	}
}

// validate checks that the point sets exactly one of text and cell, then the target's fields or
// that the cell lies inside the frame. A cell is a fixed place, so a target's nth and area do
// not apply to it. Problems are filed under field, or under the owner itself when field is empty,
// as for a position whose point keys sit beside its own.
func (p Point) validate(field string, frame Frame, report reporter) {
	whole, keys := report, report
	if field != "" {
		whole, keys = prefixed(report, field+": "), prefixed(report, field+".")
	}
	if (p.Text == "") == (p.Cell == nil) {
		whole("want exactly one of text or cell")
		return
	}
	if p.Cell == nil {
		p.Target.validate(keys)
		return
	}
	if p.Nth != 0 {
		keys("nth: only a text point takes an nth")
	}
	if p.Area != "" {
		keys("area: only a text point takes an area")
	}
	cell := *p.Cell
	if cell.X < 0 || cell.X >= frame.Cols || cell.Y < 0 || cell.Y >= frame.Rows {
		keys("cell: want x in [0, %d) and y in [0, %d), got x %d, y %d",
			frame.Cols, frame.Rows, cell.X, cell.Y)
	}
}

// placeMove checks a zoom's length, start, end and ease, and returns the stretch it holds on the
// output clock. isPlaced is false when the zoom has no length or does not fit in the clip, so it
// holds no stretch to check for overlaps. Problems are filed through report, which names the zoom.
func placeMove(
	clock OutputClock, timing moveTiming, report reporter,
) (placed span, isPlaced bool) {
	validateEase(timing.ease, report)
	isPlaced = true
	if timing.over <= 0 {
		report("over: want >0, got %s", timing.over)
		isPlaced = false
	}
	start, err := clock.Resolve(timing.at)
	if err != nil {
		report("at: %v", err)
		return span{}, false
	}
	length := clock.Length()
	if start < 0 || start >= length {
		report("at: want a start in [0s, %s), got %s", length, start)
		return span{}, false
	}
	if isPlaced && start+timing.over > length {
		report("over: want the move to end by the clip's end %s, got an end at %s",
			length, start+timing.over)
		return span{}, false
	}
	return span{start: start, end: start + timing.over}, isPlaced
}
