package main

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

// The SGR mouse report's framing: the prefix it opens with and the final bytes that end a press
// or a motion and a release.
const (
	sgrPrefix    = "\x1b[<"
	pressFinal   = 'M'
	releaseFinal = 'm'
	sgrFields    = 3
)

// The parts of an SGR report's button code: the low bits name the button (0 left, 1 middle,
// 2 right), the modifier bits add shift, alt and ctrl, the motion bit marks the pointer moving
// with the button held under button-event tracking (mode 1002), and the wheel bit marks a wheel
// or extra button, which the fixture ignores, as it does the button bits' unused value.
const (
	buttonBits   = 3
	unusedButton = 3
	leftButton   = 0
	shiftBit     = 4
	altBit       = 8
	ctrlBit      = 16
	modifierBits = shiftBit | altBit | ctrlBit
	motionBit    = 32
	wheelBit     = 64
)

// buttonNames spells each button the low bits name, indexed by them.
var buttonNames = []string{"left", "middle", "right"}

// modifierName is one modifier bit of a button code and the name a storyboard gives it.
type modifierName struct {
	bit  int
	name string
}

// modifierNames spells each modifier bit, in the order a drag's status lists them.
var modifierNames = []modifierName{{shiftBit, "shift"}, {altBit, "alt"}, {ctrlBit, "ctrl"}}

// draggedDetail follows draggedPrefix on the status row after a drag: the press cell, the release
// cell and the number of held-button motions between them.
const draggedDetail = "%d,%d to %d,%d after %d moves"

// draggedPress follows draggedDetail after a drag that was not a plain left-button drag: the
// button, then each modifier held with it, joined with `+` (`right+ctrl`).
const draggedPress = " with %s"

// cell is a 0-based screen position.
type cell struct {
	column int
	row    int
}

// mouseReport is one SGR mouse report: its button code, the 0-based cell it names and whether it
// is a release.
type mouseReport struct {
	button    int
	at        cell
	isRelease bool
}

// gesture is the drag in progress: whether a button is held, which button with which modifier
// bits, the cell it was pressed at and how many held-button motions have arrived since.
type gesture struct {
	isHeld    bool
	button    int
	modifiers int
	from      cell
	moves     int
}

// parseSGR reads an SGR mouse report (`ESC [ < button ; col ; row M` or `… m`) and returns it
// with its 1-based cell turned 0-based. Any other or malformed sequence reports false.
func parseSGR(sequence []byte) (mouseReport, bool) {
	body, isSGR := bytes.CutPrefix(sequence, []byte(sgrPrefix))
	if !isSGR || len(body) == 0 {
		return mouseReport{}, false
	}
	final := body[len(body)-1]
	if final != pressFinal && final != releaseFinal {
		return mouseReport{}, false
	}
	fields := strings.Split(string(body[:len(body)-1]), ";")
	if len(fields) != sgrFields {
		return mouseReport{}, false
	}
	button, errButton := strconv.Atoi(fields[0])
	x, errX := strconv.Atoi(fields[1])
	y, errY := strconv.Atoi(fields[2])
	if errButton != nil || errX != nil || errY != nil {
		return mouseReport{}, false
	}
	return mouseReport{
		button:    button,
		at:        cell{column: x - 1, row: y - 1},
		isRelease: final == releaseFinal,
	}, true
}

// handleMouse acts on one SGR mouse report: a press of any button, with any modifiers, starts a
// gesture, and a left-button press also selects the item under it; a held-button motion counts
// towards the gesture, and a release ends it. Wheel and extra-button reports are ignored.
func (s *screen) handleMouse(report mouseReport) {
	button := report.button & buttonBits
	switch {
	case report.button&wheelBit != 0 || button == unusedButton:
		return
	case report.isRelease:
		s.release(report.at)
	case report.button&motionBit != 0:
		if s.drag.isHeld {
			s.drag.moves++
		}
	default:
		s.drag = gesture{
			isHeld:    true,
			button:    button,
			modifiers: report.button & modifierBits,
			from:      report.at,
		}
		if button == leftButton {
			s.selectAt(report.at)
		}
	}
}

// release ends the gesture at to. One that moved at least once was a drag and shows on the status
// row, naming its button and modifiers unless it was a plain left-button drag; a press and release
// without motion was a click and leaves the status as the press set it.
func (s *screen) release(to cell) {
	if s.drag.isHeld && s.drag.moves > 0 {
		from := s.drag.from
		detail := fmt.Sprintf(draggedDetail, from.column, from.row, to.column, to.row, s.drag.moves)
		s.status = draggedPrefix + detail + s.drag.pressText()
	}
	s.drag = gesture{}
}

// pressText is the status suffix naming the gesture's button and modifiers — empty for a plain
// left-button press, so that drag's status reads as it always has.
func (g gesture) pressText() string {
	if g.button == leftButton && g.modifiers == 0 {
		return ""
	}
	names := []string{buttonNames[g.button]}
	for _, modifier := range modifierNames {
		if g.modifiers&modifier.bit != 0 {
			names = append(names, modifier.name)
		}
	}
	return fmt.Sprintf(draggedPress, strings.Join(names, "+"))
}
