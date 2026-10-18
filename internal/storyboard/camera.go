package storyboard

import (
	"fmt"
	"time"
)

// CameraMove is one move of the clip's camera, placed on the clip's output clock. Exactly one of
// Zoom and Pan is set. The camera has two independent tracks: a zoom move runs on the zoom track
// and a pan move on the pan track, so a zoom and a pan may overlap, while two moves on one track
// may not. A kept beat's [Zoom] holds both tracks for the whole of its section. A move whose `at`
// is absent starts at the clip's first frame.
//
// A move's authored At stays as written; [Load] also resolves it onto the output clock, which
// [ZoomMove.Start] and [PanMove.Start] report. A move built as a struct literal rather than loaded
// reports a zero start.
type CameraMove struct {
	Zoom *ZoomMove `yaml:"zoom"`
	Pan  *PanMove  `yaml:"pan"`
}

// ZoomMove changes the magnification to To, a factor in [1, 3], starting At and lasting Over.
type ZoomMove struct {
	To   float64       `yaml:"to"`
	At   Moment        `yaml:"at"`
	Over time.Duration `yaml:"over"`
	Ease Ease          `yaml:"ease"`

	start time.Duration
}

// Start is where the zoom move starts on the clip's output clock: its At, resolved.
func (z ZoomMove) Start() time.Duration { return z.start }

// PanMove moves the camera's centre to a point, starting At and lasting Over. Exactly one of To
// and Via is set: To is the destination of a straight move, and Via is a path of at least two
// points whose last is the destination. A target point resolves on the screen shown at the
// move's end, walking back to the move's start when it is absent there.
type PanMove struct {
	To   *Point        `yaml:"to"`
	Via  []Point       `yaml:"via"`
	At   Moment        `yaml:"at"`
	Over time.Duration `yaml:"over"`
	Ease Ease          `yaml:"ease"`

	start time.Duration
}

// Start is where the pan move starts on the clip's output clock: its At, resolved.
func (p PanMove) Start() time.Duration { return p.start }

// Point is where a pan aims: either an on-screen target (its fields sit inline, so `to: {text:
// …}` reads as a target) or a Cell of the terminal grid. Exactly one of Text and Cell is set.
type Point struct {
	Target `yaml:",inline"`
	Cell   *Cell `yaml:"cell"`
}

// Cell is one cell of the terminal grid: column X and row Y, both 0-based.
type Cell struct {
	X int `yaml:"x"`
	Y int `yaml:"y"`
}

// Ease is how a camera move's progress runs over its length. [Load] sets an absent one to
// [EaseSmooth].
type Ease string

// The spellings of Ease.
const (
	EaseSmooth Ease = "smooth"
	EaseLinear Ease = "linear"
)

// Pan path bounds: a via path runs from at least one waypoint to its destination.
const minViaPoints = 2

// The camera's tracks, as validation names them.
const (
	trackZoom = "zoom"
	trackPan  = "pan"
)

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

// moveTiming is the part every camera move form shares: where it starts, how long it runs and
// how it eases.
type moveTiming struct {
	at   Moment
	over time.Duration
	ease Ease
}

// validateCamera checks every camera move's form and its place on the clip's clock, keeping the
// start it resolves, then that no two moves on one track overlap and that no move overlaps a kept
// beat's zoom section. Problems are filed under camera[i], an overlap under the later move.
func (s *Storyboard) validateCamera(collect collector) {
	held := map[string][]span{trackZoom: nil, trackPan: nil}
	clock := s.Clock()
	beatZooms := s.beatZoomSpans(clock)
	for index, move := range s.Camera {
		holder := fmt.Sprintf("camera[%d]", index)
		moveReport := collect.at(Owner{Kind: OwnerCamera, Index: index})
		track, timing, isFormValid := move.validateForm(s.Frame, moveReport)
		if !isFormValid {
			continue
		}
		placed, isPlaced := placeMove(clock, timing, track, moveReport)
		if !isPlaced {
			continue
		}
		move.place(placed.start)
		placed.holder = holder
		for _, other := range held[track] {
			if placed.overlaps(other) {
				moveReport("%s: overlaps %s on the %s track (%s and %s)",
					track, other.holder, track, other, placed)
			}
		}
		for _, zoom := range beatZooms {
			if placed.overlaps(zoom) {
				moveReport("%s: overlaps %s section %s, which holds both tracks",
					track, zoom.holder, zoom)
			}
		}
		held[track] = append(held[track], placed)
	}
}

// beatZoomSpans returns the section of every kept beat that zooms, on clock. A beat's section is
// read by its place in the storyboard, not its id, so a duplicated id still finds its own beat.
func (s *Storyboard) beatZoomSpans(clock OutputClock) []span {
	var spans []span
	for index, beat := range s.Beats {
		if beat.Cut || beat.Zoom == nil {
			continue
		}
		start := clock.sections[index].start
		holder := fmt.Sprintf("beat %d's zoom", beat.ID)
		spans = append(spans, span{holder, start, start + beat.Duration})
	}
	return spans
}

// validateForm checks that exactly one move form is set and that its fields fit it, and returns
// the form's track and timing. isFormValid is false only when no single form is set.
func (m CameraMove) validateForm(
	frame Frame, report reporter,
) (track string, timing moveTiming, isFormValid bool) {
	if (m.Zoom == nil) == (m.Pan == nil) {
		report("want exactly one of %s or %s", trackZoom, trackPan)
		return "", moveTiming{}, false
	}
	if m.Zoom != nil {
		m.Zoom.validate(prefixed(report, trackZoom+"."))
		return trackZoom, moveTiming{m.Zoom.At, m.Zoom.Over, m.Zoom.Ease}, true
	}
	m.Pan.validate(frame, report)
	return trackPan, moveTiming{m.Pan.At, m.Pan.Over, m.Pan.Ease}, true
}

// place keeps start, the move's At resolved on the output clock, on whichever form is set.
func (m CameraMove) place(start time.Duration) {
	if m.Zoom != nil {
		m.Zoom.start = start
		return
	}
	m.Pan.start = start
}

// validate checks the zoom move's factor.
func (z ZoomMove) validate(report reporter) {
	if z.To < minZoomFactor || z.To > maxZoomFactor {
		report("to: want in [%d, %d], got %g", minZoomFactor, maxZoomFactor, z.To)
	}
}

// validate checks that the pan sets exactly one of to and via, that a via path is long enough
// and every point. Problems are filed under pan.
func (p PanMove) validate(frame Frame, report reporter) {
	if (p.To == nil) == (p.Via == nil) {
		report("%s: want exactly one of to or via", trackPan)
		return
	}
	if p.To != nil {
		p.To.validate(trackPan+".to", frame, report)
		return
	}
	if len(p.Via) < minViaPoints {
		report("%s.via: want at least %d points, got %d", trackPan, minViaPoints, len(p.Via))
	}
	for index, point := range p.Via {
		point.validate(fmt.Sprintf("%s.via[%d]", trackPan, index), frame, report)
	}
}

// validate checks that the point sets exactly one of text and cell, then the target's fields or
// that the cell lies inside the frame. A cell is a fixed place, so a target's nth and area do
// not apply to it. Problems are filed under field.
func (p Point) validate(field string, frame Frame, report reporter) {
	if (p.Text == "") == (p.Cell == nil) {
		report("%s: want exactly one of text or cell", field)
		return
	}
	if p.Cell == nil {
		p.Target.validate(prefixed(report, field+"."))
		return
	}
	if p.Nth != 0 {
		report("%s.nth: only a text point takes an nth", field)
	}
	if p.Area != "" {
		report("%s.area: only a text point takes an area", field)
	}
	cell := *p.Cell
	if cell.X < 0 || cell.X >= frame.Cols || cell.Y < 0 || cell.Y >= frame.Rows {
		report("%s.cell: want x in [0, %d) and y in [0, %d), got x %d, y %d",
			field, frame.Cols, frame.Rows, cell.X, cell.Y)
	}
}

// placeMove checks the move's length, start, end and ease, and returns the stretch it holds on
// the output clock. isPlaced is false when the move has no length or does not fit in the clip,
// so it holds no stretch to check for overlaps. Problems are filed under the move's form.
func placeMove(
	clock OutputClock, timing moveTiming, form string, report reporter,
) (placed span, isPlaced bool) {
	if timing.ease != "" && timing.ease != EaseSmooth && timing.ease != EaseLinear {
		report("%s.ease: want %s or %s, got %q", form, EaseSmooth, EaseLinear, timing.ease)
	}
	isPlaced = true
	if timing.over <= 0 {
		report("%s.over: want >0, got %s", form, timing.over)
		isPlaced = false
	}
	start, err := clock.Resolve(timing.at)
	if err != nil {
		report("%s.at: %v", form, err)
		return span{}, false
	}
	length := clock.Length()
	if start < 0 || start >= length {
		report("%s.at: want a start in [0s, %s), got %s", form, length, start)
		return span{}, false
	}
	if isPlaced && start+timing.over > length {
		report("%s.over: want the move to end by the clip's end %s, got an end at %s",
			form, length, start+timing.over)
		return span{}, false
	}
	return span{start: start, end: start + timing.over}, isPlaced
}
