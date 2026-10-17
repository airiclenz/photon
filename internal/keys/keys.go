package keys

import (
	"fmt"
	"slices"
)

// key is one named keystroke and the bytes a terminal sends for it.
type key struct {
	name     string
	sequence string
}

// table holds every key name photon accepts, in the order [Names] lists them. Arrows and page keys
// come in pairs because a storyboard that can walk a list one way but not back cannot show that
// walking off the end is safe.
var table = []key{
	// enter is CR, not LF: a terminal in raw mode sends \r for the Return key, and \n is a
	// different key event entirely.
	{"enter", "\r"},
	// esc is a lone escape byte. A program's reader resolves it only after its escape timeout,
	// since until then the byte could still be the start of a longer sequence.
	{"esc", "\x1b"},
	{"tab", "\t"},
	{"shift+tab", "\x1b[Z"},
	{"space", " "},
	{"backspace", "\x7f"},
	{"ctrl+c", "\x03"},
	{"up", "\x1b[A"},
	{"down", "\x1b[B"},
	{"left", "\x1b[D"},
	{"right", "\x1b[C"},
	{"pgup", "\x1b[5~"},
	{"pgdown", "\x1b[6~"},
	// alt+up and alt+down use the CSI parameterised form (modifier 3 is Alt) rather than the
	// ESC-prefixed one: both decode to the same key, and this is what a modern terminal sends.
	{"alt+up", "\x1b[1;3A"},
	{"alt+down", "\x1b[1;3B"},
	// F1 to F4 are SS3 sequences and F5 upwards are CSI ~ sequences: the historical split every
	// terminal still carries, including the gaps at 16 and 22.
	{"f1", "\x1bOP"},
	{"f2", "\x1bOQ"},
	{"f3", "\x1bOR"},
	{"f4", "\x1bOS"},
	{"f5", "\x1b[15~"},
	{"f6", "\x1b[17~"},
	{"f7", "\x1b[18~"},
	{"f8", "\x1b[19~"},
	{"f9", "\x1b[20~"},
	{"f10", "\x1b[21~"},
	{"f11", "\x1b[23~"},
	{"f12", "\x1b[24~"},
}

// Mouse report final bytes: an SGR report ending in M is a press, one ending in m a release.
const (
	pressFinal   = 'M'
	releaseFinal = 'm'
)

// SGR button codes: leftButton is the left mouse button, and motionButton the same button held
// while the pointer moves — the left button's code plus the motion bit (32).
const (
	leftButton   = 0
	motionButton = 32
)

// Lookup returns the bytes a terminal sends for the named key, and false when the name is not
// one photon accepts. Names are matched exactly: they are lowercase, and modifiers are joined
// with `+` (`shift+tab`, `ctrl+c`). The returned slice is the caller's own.
func Lookup(name string) ([]byte, bool) {
	at := slices.IndexFunc(table, func(k key) bool { return k.name == name })
	if at < 0 {
		return nil, false
	}
	return []byte(table[at].sequence), true
}

// Names returns every key name [Lookup] accepts, in a fixed order — the editing keys, then the
// arrows and page keys, then f1 to f12 — for a validation message to list. The returned slice is
// the caller's own.
func Names() []string {
	names := make([]string, 0, len(table))
	for _, k := range table {
		names = append(names, k.name)
	}
	return names
}

// Click returns the press half of one left-button click at a cell, as a terminal in SGR mouse
// mode (1006) reports it: `ESC [ < 0 ; col ; row M`. x and y are the zero-based column and row of
// the cell; the report itself is one-based, so the conversion happens here and a caller never
// does the arithmetic that puts the pointer one cell off. Both must be non-negative.
//
// The press and its [Release] are separate because a terminal sends them separately: a program
// that ends a drag or a selection on release would be left mid-gesture by a press alone.
func Click(x, y int) []byte { return sgrMouse(leftButton, x, y, pressFinal) }

// Release returns the release that ends the click [Click] began at the same zero-based cell:
// the same report, ending in `m`.
func Release(x, y int) []byte { return sgrMouse(leftButton, x, y, releaseFinal) }

// Motion returns the report of the pointer moving onto a zero-based cell with the left button
// held, as a terminal in SGR mouse mode with button-event tracking (1002) sends it between a
// [Click] and its [Release]: `ESC [ < 32 ; col ; row M`. Like [Click], it takes the zero-based
// cell and writes the one-based report; both must be non-negative.
func Motion(x, y int) []byte { return sgrMouse(motionButton, x, y, pressFinal) }

// sgrMouse is the one place the report's shape is written: the button code, the one-based column
// and row, and the final byte that tells a press or a motion from a release.
func sgrMouse(button, x, y int, final rune) []byte {
	return fmt.Appendf(nil, "\x1b[<%d;%d;%d%c", button, x+1, y+1, final)
}
