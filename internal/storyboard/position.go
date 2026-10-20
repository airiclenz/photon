package storyboard

import "time"

// Position is one place the camera aims at, from its start for Hold: a Point — a cell of the
// grid, or a text target resolved on the screen shown at the position's start — whose fields sit
// inline beside its timing. A Hold of 0, or an absent one, makes it a point in time. The gap
// between one position's end and the next one's start is a transition, which is not stored: the
// camera glides across it with the next position's Ease, and a gap of 0 is a hard cut.
//
// A position's authored At stays as written; validation also resolves it onto the output clock,
// which [Position.Start] reports. A position built as a struct literal rather than loaded reports
// a zero start.
type Position struct {
	Point `yaml:",inline"`
	At    Moment        `yaml:"at"`
	Hold  time.Duration `yaml:"hold"`
	Ease  Ease          `yaml:"ease"`

	start time.Duration
}

// Start is where the position starts on the clip's output clock: its At, resolved.
func (p Position) Start() time.Duration { return p.start }

// validate checks the position's point against frame, its hold and ease, and its place on the
// clip's output clock, and returns the stretch [start, start+hold) it holds there. isPlaced is
// false when the position does not fit in the clip, so it holds no stretch to check for overlaps.
func (p Position) validate(
	frame Frame, clock OutputClock, report reporter,
) (placed span, isPlaced bool) {
	p.Point.validate("", frame, report)
	validateEase(p.Ease, report)
	isPlaced = true
	if p.Hold < 0 {
		report("hold: want ≥0, got %s", p.Hold)
		isPlaced = false
	}
	start, err := clock.Resolve(p.At)
	if err != nil {
		report("at: %v", err)
		return span{}, false
	}
	length := clock.Length()
	if start < 0 || start >= length {
		report("at: want a start in [0s, %s), got %s", length, start)
		return span{}, false
	}
	if isPlaced && start+p.Hold > length {
		report("hold: want the position to end by the clip's end %s, got an end at %s",
			length, start+p.Hold)
		return span{}, false
	}
	return span{start: start, end: start + p.Hold}, isPlaced
}
