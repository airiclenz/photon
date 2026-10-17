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

// The button codes the fixture acts on: the left button, and the left button held while the
// pointer moves, which button-event tracking (mode 1002) reports as the button plus 32.
const (
	leftButton       = 0
	leftButtonMotion = 32
)

// draggedDetail follows draggedPrefix on the status row after a drag: the press cell, the release
// cell and the number of held-button motions between them.
const draggedDetail = "%d,%d to %d,%d after %d moves"

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

// gesture is the left-button drag in progress: whether the button is held, the cell it was
// pressed at and how many held-button motions have arrived since.
type gesture struct {
	isHeld bool
	from   cell
	moves  int
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

// handleMouse acts on one SGR mouse report: a left-button press starts a gesture and selects the
// item under it, a held-button motion counts towards the gesture, and a left-button release ends
// it. Every other button is ignored.
func (s *screen) handleMouse(report mouseReport) {
	switch {
	case report.button == leftButton && report.isRelease:
		s.release(report.at)
	case report.button == leftButton:
		s.drag = gesture{isHeld: true, from: report.at}
		s.selectAt(report.at)
	case report.button == leftButtonMotion && s.drag.isHeld:
		s.drag.moves++
	}
}

// release ends the gesture at to. One that moved at least once was a drag and shows on the status
// row; a press and release without motion was a click and leaves the status as the press set it.
func (s *screen) release(to cell) {
	if s.drag.isHeld && s.drag.moves > 0 {
		from := s.drag.from
		detail := fmt.Sprintf(draggedDetail, from.column, from.row, to.column, to.row, s.drag.moves)
		s.status = draggedPrefix + detail
	}
	s.drag = gesture{}
}
