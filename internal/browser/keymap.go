package browser

import (
	"fmt"
	"slices"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp/kb"
)

// keyStroke is one key name photon accepts and the keystroke a page receives for it: the DOM key
// chromedp's keyboard table holds for domKey, with modifiers, CDP's modifier bits, held.
type keyStroke struct {
	name      string
	domKey    string
	modifiers kb.Modifier
}

// keyStrokes maps every name [keys.Lookup] accepts to its keystroke. kb's table gives each DOM
// key its key value, code and virtual key code, as a US keyboard sends them; this table only
// picks the key and the modifiers. A name keys adds and this table lacks is a key a page cannot
// receive yet, which [Browser.Key] refuses.
var keyStrokes = []keyStroke{
	{"enter", kb.Enter, kb.ModifierNone},
	{"esc", kb.Escape, kb.ModifierNone},
	{"tab", kb.Tab, kb.ModifierNone},
	{"shift+tab", kb.Tab, kb.ModifierShift},
	{"space", " ", kb.ModifierNone},
	{"backspace", kb.Backspace, kb.ModifierNone},
	{"ctrl+c", "c", kb.ModifierCtrl},
	{"up", kb.ArrowUp, kb.ModifierNone},
	{"down", kb.ArrowDown, kb.ModifierNone},
	{"left", kb.ArrowLeft, kb.ModifierNone},
	{"right", kb.ArrowRight, kb.ModifierNone},
	{"pgup", kb.PageUp, kb.ModifierNone},
	{"pgdown", kb.PageDown, kb.ModifierNone},
	{"alt+up", kb.ArrowUp, kb.ModifierAlt},
	{"alt+down", kb.ArrowDown, kb.ModifierAlt},
	{"f1", kb.F1, kb.ModifierNone},
	{"f2", kb.F2, kb.ModifierNone},
	{"f3", kb.F3, kb.ModifierNone},
	{"f4", kb.F4, kb.ModifierNone},
	{"f5", kb.F5, kb.ModifierNone},
	{"f6", kb.F6, kb.ModifierNone},
	{"f7", kb.F7, kb.ModifierNone},
	{"f8", kb.F8, kb.ModifierNone},
	{"f9", kb.F9, kb.ModifierNone},
	{"f10", kb.F10, kb.ModifierNone},
	{"f11", kb.F11, kb.ModifierNone},
	{"f12", kb.F12, kb.ModifierNone},
}

// keyEvents returns the key events one press of the named key dispatches, in order: a keyDown, a
// char for a key that types text, and a keyUp, each with the key's modifiers. A key held with
// ctrl or alt types no text, as on a real keyboard, so it has no char. A name the table lacks is
// an error naming it.
func keyEvents(name string) ([]input.DispatchKeyEventParams, error) {
	at := slices.IndexFunc(keyStrokes, func(k keyStroke) bool { return k.name == name })
	if at < 0 {
		return nil, fmt.Errorf("key %q cannot be sent to a page", name)
	}
	stroke := keyStrokes[at]
	isTextSuppressed := stroke.modifiers&^kb.ModifierShift != kb.ModifierNone
	var events []input.DispatchKeyEventParams
	for _, domRune := range stroke.domKey {
		for _, event := range kb.Encode(domRune) {
			event.Modifiers |= stroke.modifiers.Int64()
			if event.Type == kb.KeyChar && isTextSuppressed {
				continue
			}
			events = append(events, *event)
		}
	}
	return events, nil
}

// typeEvents returns the key events typing text dispatches: for each rune in order, a keyDown, a
// char that types it and a keyUp, as a US keyboard sends them. A newline is the Enter key; a rune
// the keyboard has no key for is an unidentified key that still types it, and a control rune
// with no key types nothing.
func typeEvents(text string) []input.DispatchKeyEventParams {
	var events []input.DispatchKeyEventParams
	for _, typed := range text {
		for _, event := range kb.Encode(typed) {
			events = append(events, *event)
		}
	}
	return events
}
