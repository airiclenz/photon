package keys

import (
	"fmt"
	"slices"
	"strings"
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

// motionBit is the bit an SGR report adds to its button code when the pointer moves with the
// button held, rather than the button going down.
const motionBit = 32

// Button is one mouse button, as the SGR report codes it. Its zero value is the left button, so a
// [Mouse] that names no button presses the left one.
type Button int

// The mouse buttons photon can press, valued as their SGR button codes.
const (
	LeftButton   Button = 0
	MiddleButton Button = 1
	RightButton  Button = 2
)

// Mods is the set of modifier keys held while a mouse button acts, as the bits an SGR report adds
// to its button code. Its zero value holds no modifier. Combine them with `|`.
type Mods int

// The modifier keys an SGR mouse report can carry, valued as the bits they add to the button code.
const (
	Shift Mods = 4
	Alt   Mods = 8
	Ctrl  Mods = 16
)

// Mouse is the button a mouse report presses and the modifier keys held with it. The zero Mouse is
// the left button with no modifier — the gesture [Click], [Motion] and [Release] report.
type Mouse struct {
	Button Button
	Mods   Mods
}

// buttonName is one [Button] and the name a storyboard gives it.
type buttonName struct {
	name   string
	button Button
}

// buttonNames spells each [Button] the way a storyboard names it, in the order an error lists them.
var buttonNames = []buttonName{
	{"left", LeftButton},
	{"middle", MiddleButton},
	{"right", RightButton},
}

// modName is one modifier key and the name a storyboard gives it.
type modName struct {
	name string
	mod  Mods
}

// modNames spells each modifier the way a storyboard names it — the same names the key table
// joins with `+` (`shift+tab`, `alt+up`, `ctrl+c`) — in the order an error lists them.
var modNames = []modName{
	{"shift", Shift},
	{"alt", Alt},
	{"ctrl", Ctrl},
}

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
func Click(x, y int) []byte { return MousePress(x, y, Mouse{}) }

// Release returns the release that ends the click [Click] began at the same zero-based cell:
// the same report, ending in `m`.
func Release(x, y int) []byte { return MouseRelease(x, y, Mouse{}) }

// Motion returns the report of the pointer moving onto a zero-based cell with the left button
// held, as a terminal in SGR mouse mode with button-event tracking (1002) sends it between a
// [Click] and its [Release]: `ESC [ < 32 ; col ; row M`. Like [Click], it takes the zero-based
// cell and writes the one-based report; both must be non-negative.
func Motion(x, y int) []byte { return MouseMotion(x, y, Mouse{}) }

// MousePress returns the report of mouse's button going down at a zero-based cell with its
// modifier keys held: `ESC [ < code ; col ; row M`, where code is the button's code (0 left,
// 1 middle, 2 right) plus 4 for shift, 8 for alt and 16 for ctrl. Like [Click], it takes the
// zero-based cell and writes the one-based report; both must be non-negative.
func MousePress(x, y int, mouse Mouse) []byte {
	return sgrMouse(mouse.code(), x, y, pressFinal)
}

// MouseMotion returns the report of the pointer moving onto a zero-based cell with mouse's button
// and modifier keys held, as a terminal with button-event tracking (1002) sends it between a
// [MousePress] and its [MouseRelease]: the press code plus the motion bit (32), ending in `M`.
func MouseMotion(x, y int, mouse Mouse) []byte {
	return sgrMouse(mouse.code()+motionBit, x, y, pressFinal)
}

// MouseRelease returns the release that ends the press [MousePress] began with the same mouse at
// a zero-based cell: the press report's code, ending in `m`.
func MouseRelease(x, y int, mouse Mouse) []byte {
	return sgrMouse(mouse.code(), x, y, releaseFinal)
}

// ParseButton returns the [Button] a storyboard names: `left`, `middle` or `right`. The empty
// name is the left button, so a storyboard that names no button presses the left one; any other
// name is an error listing the accepted ones. Names are matched exactly, in lowercase.
func ParseButton(name string) (Button, error) {
	if name == "" {
		return LeftButton, nil
	}
	at := slices.IndexFunc(buttonNames, func(b buttonName) bool { return b.name == name })
	if at < 0 {
		names := make([]string, 0, len(buttonNames))
		for _, b := range buttonNames {
			names = append(names, b.name)
		}
		return LeftButton, fmt.Errorf("unknown button %q, want one of %s", name, strings.Join(names, ", "))
	}
	return buttonNames[at].button, nil
}

// ParseMods returns the modifier keys a storyboard names, joined with `+` (`ctrl+shift`) from
// `shift`, `alt` and `ctrl`, the names the key table uses. The empty spelling holds no modifier;
// an unknown name, an empty one between `+` signs, or a name given twice is an error. Names are
// matched exactly, in lowercase, and their order does not matter.
func ParseMods(spelling string) (Mods, error) {
	if spelling == "" {
		return 0, nil
	}
	var mods Mods
	for name := range strings.SplitSeq(spelling, "+") {
		at := slices.IndexFunc(modNames, func(m modName) bool { return m.name == name })
		if at < 0 {
			names := make([]string, 0, len(modNames))
			for _, m := range modNames {
				names = append(names, m.name)
			}
			return 0, fmt.Errorf("unknown modifier %q, want one of %s", name, strings.Join(names, ", "))
		}
		mod := modNames[at].mod
		if mods&mod != 0 {
			return 0, fmt.Errorf("modifier %q given twice", name)
		}
		mods |= mod
	}
	return mods, nil
}

// code is the SGR button code of a press or release with this mouse: the button's code plus the
// bit of every modifier held.
func (m Mouse) code() int { return int(m.Button) + int(m.Mods) }

// sgrMouse is the one place the report's shape is written: the button code, the one-based column
// and row, and the final byte that tells a press or a motion from a release.
func sgrMouse(button, x, y int, final rune) []byte {
	return fmt.Appendf(nil, "\x1b[<%d;%d;%d%c", button, x+1, y+1, final)
}
