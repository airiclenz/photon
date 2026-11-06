package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"github.com/airiclenz/photon/internal/keys"
)

// inputTimeout caps dispatching one call's input events, so a browser that stops answering fails
// the action rather than hanging it.
const inputTimeout = 10 * time.Second

// pressClickCount is the click count a press and its release carry: each is one click of its own,
// as the engine performs a click as one press and one release.
const pressClickCount = 1

// mouseButtons maps each [keys.Button] to the button CDP names and the bit it sets in a mouse
// event's mask of held buttons.
var mouseButtons = map[keys.Button]struct {
	button input.MouseButton
	mask   int64
}{
	keys.LeftButton:   {input.MouseButtonLeft, 1},
	keys.RightButton:  {input.MouseButtonRight, 2},
	keys.MiddleButton: {input.MouseButtonMiddle, 4},
}

// pointerTypes maps each [keys.PointerKind] to the CDP mouse event it dispatches.
var pointerTypes = map[keys.PointerKind]input.DispatchMouseEventType{
	keys.PointerPress:   input.DispatchMouseEventTypeMousePressed,
	keys.PointerMove:    input.DispatchMouseEventTypeMouseMoved,
	keys.PointerRelease: input.DispatchMouseEventTypeMouseReleased,
}

// modifierBits maps each SGR modifier bit a [keys.Mods] holds to CDP's bit for that modifier.
var modifierBits = []struct {
	sgr keys.Mods
	cdp kb.Modifier
}{
	{keys.Shift, kb.ModifierShift},
	{keys.Alt, kb.ModifierAlt},
	{keys.Ctrl, kb.ModifierCtrl},
}

// Type types text into the page's focused element: each rune a keyDown, a char and a keyUp, as a
// US keyboard sends them, so the page sees keydown, keypress, input and keyup for each. A newline
// presses Enter. The events go out one at a time, not in one write, and a concurrent call's
// events may land between them. A browser that does not take them within a fixed time, or has
// gone, is an error.
func (b *Browser) Type(text string) error {
	if err := b.dispatchKeys(typeEvents(text)); err != nil {
		return fmt.Errorf("type %q: %w", text, err)
	}
	return nil
}

// Key presses the named key once on the page, with the modifiers its name holds (`shift+tab`,
// `ctrl+c`): a keyDown, a char for a key that types text, and a keyUp. Every name [keys.Lookup]
// accepts is one a page can receive; any other name is an error naming it, and nothing is sent.
func (b *Browser) Key(name string) error {
	events, err := keyEvents(name)
	if err != nil {
		return err
	}
	if err := b.dispatchKeys(events); err != nil {
		return fmt.Errorf("press %q: %w", name, err)
	}
	return nil
}

// Pointer dispatches event to the page as one mouse event at its cell, which is one CSS pixel of
// the viewport: a press, a move with the button held, or a release, with its button and modifier
// keys. An event of an unknown kind or button is an error, and nothing is sent.
func (b *Browser) Pointer(event keys.Pointer) error {
	params, err := mouseEvent(event)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(b.tab, inputTimeout)
	defer cancel()
	if _, err := chromedp.Call(ctx, input.DispatchMouseEvent, params); err != nil {
		return fmt.Errorf("dispatch a %s at %d,%d: %w", params.Type, event.X, event.Y, err)
	}
	return nil
}

// dispatchKeys dispatches events to the page in order, within inputTimeout for them all.
func (b *Browser) dispatchKeys(events []input.DispatchKeyEventParams) error {
	ctx, cancel := context.WithTimeout(b.tab, inputTimeout)
	defer cancel()
	for _, event := range events {
		if _, err := chromedp.Call(ctx, input.DispatchKeyEvent, event); err != nil {
			return err
		}
	}
	return nil
}

// mouseEvent returns the CDP mouse event that performs event: its kind, its cell as the CSS
// point, its button both as the button acting and as the only button held, its modifiers as CDP
// bits, and a click count on a press and a release. An unknown kind or button is an error.
func mouseEvent(event keys.Pointer) (input.DispatchMouseEventParams, error) {
	eventType, isKnownKind := pointerTypes[event.Kind]
	if !isKnownKind {
		return input.DispatchMouseEventParams{}, fmt.Errorf(
			"unknown pointer event kind %d", event.Kind,
		)
	}
	button, isKnownButton := mouseButtons[event.Mouse.Button]
	if !isKnownButton {
		return input.DispatchMouseEventParams{}, fmt.Errorf(
			"unknown mouse button %d", event.Mouse.Button,
		)
	}
	params := input.DispatchMouseEventParams{
		Type:      eventType,
		X:         float64(event.X),
		Y:         float64(event.Y),
		Modifiers: cdpModifiers(event.Mouse.Mods).Int64(),
		Button:    button.button,
		Buttons:   button.mask,
	}
	if event.Kind != keys.PointerMove {
		params.ClickCount = pressClickCount
	}
	if event.Kind == keys.PointerRelease {
		// A release leaves no button held.
		params.Buttons = 0
	}
	return params, nil
}

// cdpModifiers converts mods, which holds SGR modifier bits (shift 4, alt 8, ctrl 16), to CDP's
// modifier bits (alt 1, ctrl 2, shift 8).
func cdpModifiers(mods keys.Mods) kb.Modifier {
	converted := kb.ModifierNone
	for _, bit := range modifierBits {
		if mods&bit.sgr != 0 {
			converted |= bit.cdp
		}
	}
	return converted
}
