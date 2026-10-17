package schedule

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

// maxSectionSpeed is the fastest a section may play before the schedule warns: past it typing
// and streaming blur, and the beat wants a longer duration or a shorter take.
const maxSectionSpeed = 6

// Section is one scheduled beat: its span of the take (source times, half-open), the section
// timing its storyboard beat gives it, where it starts on the output clock, and the Speed its
// stretch after the hold plays at. A beat longer than its Duration plays 1× for its Hold and
// Speed = (length−Hold)/(Duration−Hold) after it; a beat that fits plays 1× throughout (Speed 1)
// and freezes its last frame for the rest of its Duration. A Cut section has no output time.
type Section struct {
	take.BeatSpan
	Duration time.Duration
	Hold     time.Duration
	Cut      bool
	OutStart time.Duration
	Speed    float64
}

// Schedule maps the take's source clock onto the clip's output clock, section by section, in
// the order the take started the beats. Length is the clip's total length — the sum of the kept
// beats' durations — and Warnings names every beat that plays faster than 6×.
type Schedule struct {
	Sections []Section
	Length   time.Duration
	Warnings []string
}

// Frame is one output frame: its output time, the source time it shows and that time's beat.
type Frame struct {
	Out    time.Duration
	Source time.Duration
	Beat   int
}

// New lays the take's beats onto the storyboard's section timings, end to end on the output
// clock. Every storyboard beat must have started exactly once in the take, and the take must
// start no beat the storyboard lacks; each kept beat needs a positive duration with a hold in
// [0, duration). A schedule whose every beat is cut is an error, since it has no clip to show.
func New(board *storyboard.Storyboard, recorded *take.Take) (*Schedule, error) {
	spans, err := recorded.BeatSpans()
	if err != nil {
		return nil, fmt.Errorf("schedule: %w", err)
	}
	sections, err := sectionsOf(board, spans)
	if err != nil {
		return nil, err
	}

	schedule := &Schedule{Sections: sections}
	for index := range schedule.Sections {
		section := &schedule.Sections[index]
		section.OutStart, section.Speed = schedule.Length, 1
		if section.Cut {
			continue
		}
		if !section.fits() {
			section.Speed = float64(section.length()-section.Hold) / float64(section.Duration-section.Hold)
		}
		if section.Speed > maxSectionSpeed {
			schedule.Warnings = append(schedule.Warnings, fmt.Sprintf(
				"beat %d plays at %.1f× (%s of take in %s), above %d×",
				section.Beat, section.Speed, section.length(), section.Duration, maxSectionSpeed))
		}
		schedule.Length += section.Duration
	}
	if schedule.Length == 0 {
		return nil, errors.New("schedule: every beat is cut")
	}
	return schedule, nil
}

// sectionsOf pairs each span with its storyboard beat's timing, in span order, after checking
// that the take started exactly the storyboard's beats and that every kept beat's timing can be
// scheduled.
func sectionsOf(board *storyboard.Storyboard, spans []take.BeatSpan) ([]Section, error) {
	started := make(map[int]bool, len(spans))
	for _, span := range spans {
		started[span.Beat] = true
	}
	beats := make(map[int]storyboard.Beat, len(board.Beats))
	for _, beat := range board.Beats {
		if !started[beat.ID] {
			return nil, fmt.Errorf("schedule: beat %d never started in the take", beat.ID)
		}
		beats[beat.ID] = beat
	}

	sections := make([]Section, 0, len(spans))
	for _, span := range spans {
		beat, ok := beats[span.Beat]
		if !ok {
			return nil, fmt.Errorf("schedule: the take started beat %d, which the storyboard does not declare", span.Beat)
		}
		if err := checkTiming(beat); err != nil {
			return nil, err
		}
		sections = append(sections, Section{BeatSpan: span, Duration: beat.Duration, Hold: beat.Hold, Cut: beat.Cut})
	}
	return sections, nil
}

// checkTiming rejects a kept beat whose duration or hold cannot be scheduled. The storyboard
// loader refuses both already; this guards a board built any other way.
func checkTiming(beat storyboard.Beat) error {
	switch {
	case beat.Cut:
		return nil
	case beat.Duration <= 0:
		return fmt.Errorf("schedule: beat %d: duration: want >0, got %s", beat.ID, beat.Duration)
	case beat.Hold < 0 || beat.Hold >= beat.Duration:
		return fmt.Errorf("schedule: beat %d: hold: want in [0, %s), got %s", beat.ID, beat.Duration, beat.Hold)
	}
	return nil
}

// Source maps an output time onto the source time it shows and that time's beat. It reports
// false outside [0, Length).
func (s *Schedule) Source(out time.Duration) (source time.Duration, beat int, ok bool) {
	if out < 0 || out >= s.Length {
		return 0, 0, false
	}
	for _, section := range s.Sections {
		if section.Cut || out >= section.OutStart+section.Duration {
			continue
		}
		return section.sourceAt(out - section.OutStart), section.Beat, true
	}
	return 0, 0, false
}

// Output maps a source time onto the output time it first shows at: the continuous inverse of
// [Schedule.Source] — 1× over a hold, divided by the speed after it — defined for every instant
// from the first beat's start to the take's end, a frozen or unsampled one included. It reports
// false for an instant inside a cut beat and for one outside the beats.
func (s *Schedule) Output(source time.Duration) (time.Duration, bool) {
	for index, section := range s.Sections {
		isLast := index == len(s.Sections)-1
		isInside := source >= section.Start &&
			(source < section.End || (source == section.End && (isLast || section.length() == 0)))
		if !isInside {
			continue
		}
		if section.Cut {
			return 0, false
		}
		return section.outputAt(source - section.Start), true
	}
	return 0, false
}

// Frames samples the schedule at fps: frame i sits at output time i/fps, and there are as many
// frames as the clip's length rounds to, so the clip runs its total length to within one frame.
// A non-positive fps yields no frames.
func (s *Schedule) Frames(fps int) []Frame {
	if fps <= 0 {
		return nil
	}
	count := int(math.Round(s.Length.Seconds() * float64(fps)))
	frames := make([]Frame, 0, count)
	for index := range count {
		out := time.Duration(int64(index) * int64(time.Second) / int64(fps))
		source, beat, ok := s.Source(out)
		if !ok {
			break
		}
		frames = append(frames, Frame{Out: out, Source: source, Beat: beat})
	}
	return frames
}

// length is the beat's real length in the take.
func (s Section) length() time.Duration { return s.End - s.Start }

// fits reports whether the beat plays at 1× within its duration, freezing for what remains.
func (s Section) fits() bool { return s.length() <= s.Duration }

// lastInstant is the latest source offset inside the beat: its span is half-open, so the frame
// it freezes on is the one just before the next beat starts.
func (s Section) lastInstant() time.Duration {
	return max(0, s.length()-time.Nanosecond)
}

// sourceAt maps an output offset into the section onto a source time.
func (s Section) sourceAt(offset time.Duration) time.Duration {
	switch {
	case s.fits():
		return s.Start + min(offset, s.lastInstant())
	case offset < s.Hold:
		return s.Start + offset
	default:
		played := time.Duration(math.Round(float64(offset-s.Hold) * s.Speed))
		return s.Start + min(s.Hold+played, s.lastInstant())
	}
}

// outputAt maps a source offset into the section onto an output time.
func (s Section) outputAt(offset time.Duration) time.Duration {
	if s.fits() || offset < s.Hold {
		return s.OutStart + offset
	}
	return s.OutStart + s.Hold + time.Duration(math.Round(float64(offset-s.Hold)/s.Speed))
}
