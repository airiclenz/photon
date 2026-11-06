package engine

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/airiclenz/photon/internal/keys"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

// The engine's pacing. Each gap is a floor the program must see between two writes, not a
// target the render keeps: the render retimes every beat to its own duration.
const (
	// clickHoldGap parts a click's press from its release: a program that acts on the release
	// must read the two as two reports, never one read.
	clickHoldGap = 40 * time.Millisecond
	// dragStepGap precedes each of a drag's steps and its release, so every move reaches the
	// program as its own read and the eased path plays out at even time gaps.
	dragStepGap = 40 * time.Millisecond
	// clickRepeatGap parts one click's release from the next click's press, so `times: 2` is two
	// clicks rather than a double-click.
	clickRepeatGap = 250 * time.Millisecond
	// keyRepeatGap follows every key press. It sits above the 50 ms escape timeout common TUI
	// input parsers use, so an esc is never read together with the bytes after it — two esc
	// presses are two keys, never one alt+esc.
	keyRepeatGap = 100 * time.Millisecond
	// waitPollInterval is how often a wait re-reads the screen.
	waitPollInterval = 20 * time.Millisecond
)

// The action forms, as a storyboard spells them and as an event's Form names them.
const (
	formType  = "type"
	formKey   = "key"
	formClick = "click"
	formDrag  = "drag"
	formWait  = "wait"
	formPause = "pause"
)

// errNoForm is an action with none of its forms set, which a loaded storyboard never holds.
var errNoForm = errors.New("no action form set")

// formOf names the form an action takes, or "" when none is set.
func formOf(action storyboard.Action) string {
	switch {
	case action.Type != nil:
		return formType
	case action.Key != nil:
		return formKey
	case action.Click != nil:
		return formClick
	case action.Drag != nil:
		return formDrag
	case action.Wait != nil:
		return formWait
	case action.Pause != nil:
		return formPause
	}
	return ""
}

// textOf is the free text an action's events carry: a type's string or a key's name.
func textOf(action storyboard.Action) string {
	switch {
	case action.Type != nil:
		return action.Type.Text
	case action.Key != nil:
		return action.Key.Name
	}
	return ""
}

// perform runs one action; step is its action event, which a click's or a drag's own events
// extend.
func (e *Engine) perform(ctx context.Context, step take.Event, action storyboard.Action) error {
	switch {
	case action.Type != nil:
		return e.typeText(ctx, *action.Type)
	case action.Key != nil:
		return e.pressKey(ctx, *action.Key)
	case action.Click != nil:
		return e.click(ctx, step, *action.Click)
	case action.Drag != nil:
		return e.drag(ctx, step, *action.Drag)
	case action.Wait != nil:
		return e.wait(ctx, *action.Wait)
	case action.Pause != nil:
		return sleepContext(ctx, action.Pause.For)
	}
	return errNoForm
}

// typeText types the text one character per [Source.Type], paced by the humanized profile, or —
// with humanize off — in a single one.
func (e *Engine) typeText(ctx context.Context, action storyboard.TypeAction) error {
	if !action.IsHumanized() {
		return e.source.Type(action.Text)
	}
	gaps := humanize(action.Text)
	for index, character := range []rune(action.Text) {
		if err := e.source.Type(string(character)); err != nil {
			return err
		}
		if index < len(gaps) {
			if err := sleepContext(ctx, gaps[index]); err != nil {
				return err
			}
		}
	}
	return nil
}

// pressKey presses the named key Repeat times (once for a Repeat of 0), each press one
// [Source.Key] and each followed by keyRepeatGap: an escape sequence split across writes, or an
// esc glued to whatever is written next, would reach the program as a different key. A name
// [keys.Lookup] does not accept fails before anything is sent, whatever the source.
func (e *Engine) pressKey(ctx context.Context, action storyboard.KeyAction) error {
	if _, isKnown := keys.Lookup(action.Name); !isKnown {
		return fmt.Errorf("key: unknown key name %q", action.Name)
	}
	for range max(action.Repeat, 1) {
		if err := e.source.Key(action.Name); err != nil {
			return err
		}
		if err := sleepContext(ctx, keyRepeatGap); err != nil {
			return err
		}
	}
	return nil
}

// click resolves the target on the screen as it stands, logs the box, then sends Times clicks
// (once for a Times of 0) of the action's button, its modifiers held, at its centre cell — each
// a pointer press and, clickHoldGap later, its release — logging the cell before each press.
func (e *Engine) click(ctx context.Context, step take.Event, action storyboard.ClickAction) error {
	mouse, err := action.Mouse()
	if err != nil {
		return fmt.Errorf("click: %w", err)
	}
	box, err := e.resolver.Target(e.source.Current(), action.Target)
	if err != nil {
		return fmt.Errorf("click: %w", err)
	}
	resolved := step
	resolved.Box = &box
	e.logStep(take.EventTarget, resolved)

	centre := centreOf(box)
	for press := range max(action.Times, 1) {
		if press > 0 {
			if err := sleepContext(ctx, clickRepeatGap); err != nil {
				return err
			}
		}
		clicked := step
		clicked.Cell = &centre
		e.logStep(take.EventClick, clicked)
		if err := e.sendClick(ctx, centre, mouse); err != nil {
			return err
		}
	}
	return nil
}

// centreOf is the cell a click or a drag aims at in box: its middle cell, the upper-left of the
// middle pair on an even side.
func centreOf(box take.Box) take.Point {
	return take.Point{X: box.X + (box.W-1)/2, Y: box.Y + (box.H-1)/2}
}

// sendClick sends one click of mouse's button, its modifiers held, at cell: the press,
// clickHoldGap, then the release.
func (e *Engine) sendClick(ctx context.Context, cell take.Point, mouse keys.Mouse) error {
	press := PointerEvent{Kind: keys.PointerPress, X: cell.X, Y: cell.Y, Mouse: mouse}
	if err := e.source.Pointer(press); err != nil {
		return err
	}
	if err := sleepContext(ctx, clickHoldGap); err != nil {
		return err
	}
	release := PointerEvent{Kind: keys.PointerRelease, X: cell.X, Y: cell.Y, Mouse: mouse}
	return e.source.Pointer(release)
}

// wait polls the whole screen until the regex matches it (or, with Gone, no longer does). The
// screen that satisfies it is recorded into the take ([Source.Match]), so a `seen` expect on a
// beat that ends on a wait always finds the screen the wait saw. It fails at the timeout, or at
// once when the program exits first.
func (e *Engine) wait(ctx context.Context, action storyboard.WaitAction) error {
	if action.Timeout <= 0 || action.Timeout > storyboard.MaxWaitTimeout {
		return fmt.Errorf("wait: timeout %s outside (0, %s]",
			action.Timeout, storyboard.MaxWaitTimeout)
	}
	pattern, err := regexp.Compile(action.Screen)
	if err != nil {
		return fmt.Errorf("wait: %w", err)
	}
	want := "shown"
	if action.Gone {
		want = "gone"
	}
	deadline := time.NewTimer(action.Timeout)
	defer deadline.Stop()
	poll := time.NewTicker(waitPollInterval)
	defer poll.Stop()
	for {
		isSatisfied, err := e.screenSatisfies(pattern, action.Gone)
		if err != nil {
			return fmt.Errorf("wait for /%s/ %s: %w", action.Screen, want, err)
		}
		if isSatisfied {
			return nil
		}
		select {
		case <-poll.C:
		case <-deadline.C:
			return fmt.Errorf("wait for /%s/ %s: timed out after %s",
				action.Screen, want, action.Timeout)
		case <-e.source.Done():
			return fmt.Errorf("wait for /%s/ %s: the program exited", action.Screen, want)
		case <-ctx.Done():
			return fmt.Errorf("wait for /%s/ %s: %w", action.Screen, want, ctx.Err())
		}
	}
}

// screenSatisfies reports whether pattern matches the whole screen as it stands (with gone,
// whether it does not), recording that screen when it does.
func (e *Engine) screenSatisfies(pattern *regexp.Regexp, gone bool) (bool, error) {
	var textErr error
	isSatisfied := e.source.Match(func(screen take.Snapshot) bool {
		text, err := e.resolver.Text(screen, storyboard.AreaAny)
		if err != nil {
			textErr = err
			return false
		}
		return pattern.MatchString(text) != gone
	})
	return isSatisfied, textErr
}

// sleepContext waits d, or less when ctx ends first.
func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
