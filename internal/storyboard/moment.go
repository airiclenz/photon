package storyboard

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v3"
)

// Moment is a point on the clip's output clock. Beat 0 is a bare clip time, Offset after the
// clip's first frame (`at: 4500ms`); a positive Beat names a beat, and the moment is that beat's
// section start on the output clock plus Offset, which may be negative (`at: {beat: 3, offset:
// -200ms}`). Beat ids are ≥1, so 0 never names a beat. [Storyboard.ClipTime] resolves one.
type Moment struct {
	Beat   int           `yaml:"beat"`
	Offset time.Duration `yaml:"offset"`
}

// Moment's mapping keys, which UnmarshalYAML checks itself.
const (
	momentKeyBeat   = "beat"
	momentKeyOffset = "offset"
)

// UnmarshalYAML accepts a duration scalar, a bare clip time, or a mapping of `beat` (a beat id,
// required) and `offset` (a duration, default 0). A yaml.Node decode is not strict, so the
// mapping's keys are checked here: an unknown key is an error, as it is everywhere else in a
// storyboard.
func (m *Moment) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		offset, err := time.ParseDuration(value.Value)
		if err != nil {
			return fmt.Errorf("moment: want a duration such as 4500ms, got %q", value.Value)
		}
		*m = Moment{Offset: offset}
		return nil
	case yaml.MappingNode:
		return m.decodeMapping(value)
	default:
		return fmt.Errorf("moment: want a duration or a {beat, offset} mapping, got a %s", nodeKindName(value.Kind))
	}
}

// decodeMapping decodes the `{beat: N, offset: D}` form after refusing any other key.
func (m *Moment) decodeMapping(value *yaml.Node) error {
	for index := 0; index < len(value.Content); index += 2 {
		key := value.Content[index].Value
		if key != momentKeyBeat && key != momentKeyOffset {
			return fmt.Errorf("moment: unknown key %q, want %s or %s", key, momentKeyBeat, momentKeyOffset)
		}
	}

	// A plain struct of the same shape decodes without recursing into this method.
	type plainMoment Moment
	var decoded plainMoment
	if err := value.Decode(&decoded); err != nil {
		return fmt.Errorf("moment: %w", err)
	}
	if decoded.Beat < 1 {
		return fmt.Errorf("moment: %s: want a beat id ≥1, got %d", momentKeyBeat, decoded.Beat)
	}
	*m = Moment(decoded)
	return nil
}

// ClipTime resolves a moment onto the output clock: a bare clip time is its Offset, and a
// beat moment is the sum of the durations of the kept beats before that beat, in storyboard
// order, plus its Offset. The rig runs the beats in storyboard order, so a beat's start equals
// its schedule section's OutStart. ClipTime does not check that the result lies inside the
// clip; it fails only for a beat the storyboard does not declare or one that is cut, which has
// no time on the output clock.
func (s *Storyboard) ClipTime(moment Moment) (time.Duration, error) {
	if moment.Beat == 0 {
		return moment.Offset, nil
	}
	var start time.Duration
	for _, beat := range s.Beats {
		if beat.ID == moment.Beat {
			if beat.Cut {
				return 0, fmt.Errorf("beat %d is cut, so it has no clip time", beat.ID)
			}
			return start + moment.Offset, nil
		}
		if !beat.Cut {
			start += beat.Duration
		}
	}
	return 0, fmt.Errorf("beat %d is not in the storyboard", moment.Beat)
}

// Length is the clip's length on the output clock: the sum of the kept beats' durations, which
// is the schedule's Length.
func (s *Storyboard) Length() time.Duration {
	var length time.Duration
	for _, beat := range s.Beats {
		if !beat.Cut {
			length += beat.Duration
		}
	}
	return length
}
