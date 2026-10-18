package storyboard

import (
	"fmt"
	"time"
)

// OutputClock is the clip's output clock: the kept beats laid end to end in storyboard order,
// each starting where the kept beats before it end. A cut beat takes no time on it. The rig runs
// the beats in storyboard order, so a beat's section start equals its schedule section's
// OutStart and the clock's Length is the schedule's Length. [Storyboard.Clock] builds one.
type OutputClock struct {
	sections []clockSection
	length   time.Duration
}

// clockSection is where one beat of the storyboard sits on the output clock, in storyboard order.
type clockSection struct {
	beat  int
	start time.Duration
	isCut bool
}

// Clock builds the storyboard's output clock from its beats as they stand. It sums the kept
// beats' durations without checking them, since validation runs it on boards that break rules.
func (s *Storyboard) Clock() OutputClock {
	sections := make([]clockSection, 0, len(s.Beats))
	var start time.Duration
	for _, beat := range s.Beats {
		sections = append(sections, clockSection{beat: beat.ID, start: start, isCut: beat.Cut})
		if !beat.Cut {
			start += beat.Duration
		}
	}
	return OutputClock{sections: sections, length: start}
}

// Length is the clip's length on the output clock: the sum of the kept beats' durations.
func (c OutputClock) Length() time.Duration {
	return c.length
}

// SectionStart is where beat starts on the output clock: the sum of the durations of the kept
// beats before it. A cut beat starts where it sits, so its start is the next kept beat's. Where
// two beats share an id the first one answers. It fails only for a beat the storyboard does not
// declare.
func (c OutputClock) SectionStart(beat int) (time.Duration, error) {
	section, isDeclared := c.section(beat)
	if !isDeclared {
		return 0, fmt.Errorf("beat %d is not in the storyboard", beat)
	}
	return section.start, nil
}

// Resolve places a moment on the output clock: a bare clip time is its Offset, and a beat moment
// is that beat's section start plus its Offset. It does not check that the result lies inside the
// clip; it fails only for a beat the storyboard does not declare or one that is cut, which has no
// time on the output clock.
func (c OutputClock) Resolve(moment Moment) (time.Duration, error) {
	if moment.Beat == 0 {
		return moment.Offset, nil
	}
	section, isDeclared := c.section(moment.Beat)
	if !isDeclared {
		return 0, fmt.Errorf("beat %d is not in the storyboard", moment.Beat)
	}
	if section.isCut {
		return 0, fmt.Errorf("beat %d is cut, so it has no clip time", moment.Beat)
	}
	return section.start + moment.Offset, nil
}

// section finds the first beat with the id beat.
func (c OutputClock) section(beat int) (clockSection, bool) {
	for _, section := range c.sections {
		if section.beat == beat {
			return section, true
		}
	}
	return clockSection{}, false
}
