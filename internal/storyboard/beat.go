package storyboard

import (
	"fmt"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Beat is one step of a storyboard: the actions photon performs, the section's Duration in the
// clip (Hold is its leading stretch shown at 1×; a beat with no hold has hold 0), an optional Zoom
// onto an on-screen target, and what a keeper take shows. A Cut beat is recorded but dropped from
// the clip, so its Duration and Hold are not required.
type Beat struct {
	ID       int           `yaml:"id"`
	Title    string        `yaml:"title"`
	Why      string        `yaml:"why"`
	Notes    string        `yaml:"notes"`
	Do       []Action      `yaml:"do"`
	Duration time.Duration `yaml:"duration"`
	Hold     time.Duration `yaml:"hold"`
	Cut      bool          `yaml:"cut"`
	Zoom     *Zoom         `yaml:"zoom"`
	Expect   []Expect      `yaml:"expect"`
}

// Zoom is a push into an on-screen target: Factor is the magnification — absent (0), the
// compositor fits it to the target — and In and Out the ramps, which [Load] sets to
// [DefaultZoomRamp] when the storyboard leaves them out.
type Zoom struct {
	Target Target        `yaml:"target"`
	Factor float64       `yaml:"factor"`
	In     time.Duration `yaml:"in"`
	Out    time.Duration `yaml:"out"`
}

// DefaultZoomRamp is how long a zoom's in or out ramp runs when the storyboard gives none.
const DefaultZoomRamp = 1000 * time.Millisecond

// Action is one thing a beat does. Exactly one of its forms is set.
type Action struct {
	Type  *TypeAction  `yaml:"type"`
	Key   *KeyAction   `yaml:"key"`
	Click *ClickAction `yaml:"click"`
	Wait  *WaitAction  `yaml:"wait"`
	Pause *PauseAction `yaml:"pause"`
}

// TypeAction types Text, with the humanized keystroke profile unless Humanize is false.
type TypeAction struct {
	Text string `yaml:"text"`
	// Humanize is a pointer so an absent key reads as the default, true.
	Humanize *bool `yaml:"humanize"`
}

// IsHumanized reports whether the text is typed with the humanized profile: true unless the
// storyboard sets `humanize: false`.
func (t TypeAction) IsHumanized() bool { return t.Humanize == nil || *t.Humanize }

// KeyAction presses the named key Repeat times; a Repeat of 0 presses it once. Name is one the
// keys package accepts.
type KeyAction struct {
	Name   string `yaml:"name"`
	Repeat int    `yaml:"repeat"`
}

// ClickAction clicks the resolved Target Times times; a Times of 0 clicks once.
type ClickAction struct {
	Target Target `yaml:"target"`
	Times  int    `yaml:"times"`
}

// WaitAction blocks until the Screen regex matches the terminal (or, with Gone, no longer
// matches) and fails the beat once Timeout passes.
type WaitAction struct {
	Screen  string        `yaml:"screen"`
	Gone    bool          `yaml:"gone"`
	Timeout time.Duration `yaml:"timeout"`
}

// MaxWaitTimeout bounds a wait's timeout, so a storyboard typo cannot stall a take for long.
const MaxWaitTimeout = 180 * time.Second

// PauseAction idles for For.
type PauseAction struct {
	For time.Duration `yaml:"for"`
}

// Target is an on-screen place a click or zoom resolves on the current screen: Text is a regex
// over the screen's rows, Nth picks which match counts and Area limits the rows searched to an
// area the config declares (empty or [AreaAny] means the whole screen).
type Target struct {
	Text string `yaml:"text"`
	Nth  Nth    `yaml:"nth"`
	Area string `yaml:"area"`
}

// AreaAny is the area spelling that searches the whole screen, as an empty area does.
const AreaAny = "any"

// Nth picks which of a target's matches counts: the zero value and NthFirst are the first match,
// a positive value the Nth, and NthLast the last one. It is its own scalar because a strict decode
// would refuse `nth: last` into a plain int before validation ever ran.
type Nth int

// The named spellings of Nth.
const (
	NthFirst Nth = 1
	NthLast  Nth = -1
)

// UnmarshalYAML accepts `first`, `last` or a positive integer.
func (n *Nth) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind != yaml.ScalarNode {
		return fmt.Errorf("nth: want first, last or a positive integer, got a %s", nodeKindName(value.Kind))
	}
	switch value.Value {
	case "first":
		*n = NthFirst
		return nil
	case "last":
		*n = NthLast
		return nil
	}
	parsed, err := strconv.Atoi(value.Value)
	if err != nil || parsed < 1 {
		return fmt.Errorf("nth: want first, last or a positive integer, got %q", value.Value)
	}
	*n = Nth(parsed)
	return nil
}

// nodeKindName spells a yaml.Node kind for an error message.
func nodeKindName(kind yaml.Kind) string {
	switch kind {
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	default:
		return "node"
	}
}

// Expect is one thing a keeper take must show, judged PASS or FAIL by check. Exactly one form is
// set. Seen is a regex that must match at least one screen recorded inside the beat (any screen
// of the take, for a take-level expect), searched in Area when one is given. Run is a command
// whose exit status is the verdict: how a project checks what photon cannot see.
type Expect struct {
	Seen string `yaml:"seen"`
	Area string `yaml:"area"`
	Run  string `yaml:"run"`
}
