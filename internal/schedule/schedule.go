package schedule

import (
	"errors"
	"fmt"
	"math"
	"slices"
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
// storyboard order, which is the order the take started the beats. Length is the clip's total
// length — the sum of the kept beats' durations — and Warnings names every beat that plays
// faster than 6×.
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
// clock. The board must come from [storyboard.Parse] or [storyboard.Load], whose validation
// already settles every beat's section timing and refuses a board with no kept beat; New does not
// check them again. Every storyboard beat must have started exactly once in the take, in
// storyboard order, and the take must start no beat the storyboard lacks: each disagreement is
// its own `schedule: `-prefixed error, all of them joined, and one about a storyboard beat is
// filed under that beat ([storyboard.AtBeat]). Every section starts where
// [storyboard.OutputClock.SectionStart] places its beat and the schedule's Length is the
// clock's, so the schedule agrees with the storyboard's clock by construction.
func New(board *storyboard.Storyboard, recorded *take.Take) (*Schedule, error) {
	if err := recorded.MatchBeats(board.BeatIDs()); err != nil {
		return nil, mismatchOf(board, err)
	}
	spans, err := recorded.BeatSpans()
	if err != nil {
		return nil, fmt.Errorf("schedule: %w", err)
	}
	sections := sectionsOf(board, spans)

	clock := board.Clock()
	schedule := &Schedule{Sections: sections, Length: clock.Length()}
	for index := range schedule.Sections {
		section := &schedule.Sections[index]
		section.Speed = 1
		section.OutStart, err = clock.SectionStart(section.Beat)
		if err != nil {
			return nil, fmt.Errorf("schedule: %w", err)
		}
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
	}
	return schedule, nil
}

// mismatchOf turns [take.Take.MatchBeats]' error into the schedule's: each of its problems
// prefixed `schedule: ` on its own, a [take.BeatError] about a beat the storyboard declares filed
// under that beat with the same text, and the prefixed problems joined again.
func mismatchOf(board *storyboard.Storyboard, err error) error {
	problems := []error{err}
	if joined, isJoined := err.(interface{ Unwrap() []error }); isJoined {
		problems = joined.Unwrap()
	}
	wrapped := make([]error, 0, len(problems))
	for _, problem := range problems {
		var mismatch *take.BeatError
		if errors.As(problem, &mismatch) && declares(board, mismatch.Beat) {
			problem = storyboard.AtBeat(mismatch.Beat, errors.New(mismatch.Problem))
		}
		wrapped = append(wrapped, fmt.Errorf("schedule: %w", problem))
	}
	return errors.Join(wrapped...)
}

// declares reports whether the storyboard has a beat with id.
func declares(board *storyboard.Storyboard, id int) bool {
	return slices.ContainsFunc(board.Beats, func(beat storyboard.Beat) bool { return beat.ID == id })
}

// sectionsOf pairs each span with its storyboard beat's timing, in span order.
// [take.Take.MatchBeats] has already matched the spans to the storyboard's beats.
func sectionsOf(board *storyboard.Storyboard, spans []take.BeatSpan) []Section {
	beats := make(map[int]storyboard.Beat, len(board.Beats))
	for _, beat := range board.Beats {
		beats[beat.ID] = beat
	}

	sections := make([]Section, 0, len(spans))
	for _, span := range spans {
		beat := beats[span.Beat]
		sections = append(sections, Section{BeatSpan: span, Duration: beat.Duration, Hold: beat.Hold, Cut: beat.Cut})
	}
	return sections
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
