package storyboard

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Beat is one step of a storyboard: the actions photon performs, the section's Duration in the
// clip (Hold is its leading stretch shown at 1×; a beat with no hold has hold 0) and what a keeper
// take shows. A Cut beat is recorded but dropped from the clip, so its Duration and Hold are not
// required. A beat does not zoom: the camera's zoom and pan moves do. RemovedZoom marks a `zoom`
// key, which a beat no longer takes; validation refuses it.
type Beat struct {
	ID          int           `yaml:"id"`
	Title       string        `yaml:"title"`
	Why         string        `yaml:"why"`
	Notes       string        `yaml:"notes"`
	Do          []Action      `yaml:"do"`
	Duration    time.Duration `yaml:"duration"`
	Hold        time.Duration `yaml:"hold"`
	Cut         bool          `yaml:"cut"`
	RemovedZoom removedKey    `yaml:"zoom"`
	Expect      []Expect      `yaml:"expect"`
}

// Action is one thing a beat does. Exactly one of its forms is set.
type Action struct {
	Type  *TypeAction  `yaml:"type"`
	Key   *KeyAction   `yaml:"key"`
	Click *ClickAction `yaml:"click"`
	Drag  *DragAction  `yaml:"drag"`
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

// DragAction presses the left button at the resolved From target, moves with it held to To in
// Steps steps and releases it there. Steps is a pointer so an absent key reads as the default,
// [DefaultDragSteps], while an explicit value below 1 is a validation problem.
type DragAction struct {
	From  Target  `yaml:"from"`
	To    DragEnd `yaml:"to"`
	Steps *int    `yaml:"steps"`
}

// DefaultDragSteps is how many held-button moves a drag makes when the storyboard gives no steps.
const DefaultDragSteps = 12

// StepCount returns the drag's steps: the storyboard's value, or [DefaultDragSteps] when it gives
// none.
func (d DragAction) StepCount() int {
	if d.Steps == nil {
		return DefaultDragSteps
	}
	return *d.Steps
}

// DragEnd is where a drag releases: either a target resolved on the screen as it stands (its
// fields sit inline, so `to: {text: …}` reads as a target) or an Offset from the press cell.
// Exactly one of Text and Offset is set.
type DragEnd struct {
	Target `yaml:",inline"`
	Offset *Offset `yaml:"offset"`
}

// Offset is a move in cells: X columns to the right and Y rows down; negative values go left and
// up.
type Offset struct {
	X int `yaml:"x"`
	Y int `yaml:"y"`
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

// Target is an on-screen place a click, drag or pan resolves on the current screen: Text is a
// regex over the screen's rows, Nth picks which match counts and Area limits the rows searched to
// an area the config declares (empty or [AreaAny] means the whole screen). It is also a target on
// the JSON wire, {"text", "nth", "area"}, as a pan's camera.to [Edit] types it and photon tune's
// pick proposes it: a zero Nth and an empty Area are left out.
type Target struct {
	Text string `yaml:"text" json:"text"`
	Nth  Nth    `yaml:"nth" json:"nth,omitempty"`
	Area string `yaml:"area" json:"area,omitempty"`
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
		return fmt.Errorf(
			"nth: want first, last or a positive integer, got a %s", nodeKindName(value.Kind))
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

// MarshalJSON writes NthLast as "last" and any other nth as its number.
func (n Nth) MarshalJSON() ([]byte, error) {
	if n == NthLast {
		return json.Marshal(nthLastWord)
	}
	return json.Marshal(int(n))
}

// UnmarshalJSON reads a whole number from 1 or "last", and refuses anything else: the first match
// travels as an absent nth, never as "first" or 0.
func (n *Nth) UnmarshalJSON(data []byte) error {
	var count int
	if err := json.Unmarshal(data, &count); err == nil && count >= int(NthFirst) {
		*n = Nth(count)
		return nil
	}
	var word string
	if err := json.Unmarshal(data, &word); err == nil && word == nthLastWord {
		*n = NthLast
		return nil
	}
	return fmt.Errorf("nth: want a whole number from 1 or %q, got %s", nthLastWord, data)
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
