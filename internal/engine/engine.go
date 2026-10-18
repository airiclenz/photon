package engine

import (
	"context"
	"fmt"

	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
)

// Terminal is the running program the engine drives. The pty terminal is the production
// adapter; tests drive a fake. Every method must be safe to call from any goroutine.
type Terminal interface {
	// Send writes p to the program as input, in one write.
	Send(p []byte) error
	// Current is what the terminal shows right now. It is read, not recorded.
	Current() take.Snapshot
	// Match reports whether match holds for the screen as it stands and, when it does, records
	// that screen into the take, so a screen a wait saw is a screen the take holds.
	Match(match func(take.Snapshot) bool) bool
	// Done closes once the program has exited.
	Done() <-chan struct{}
	// Event logs event into the take, stamped on the take's clock.
	Event(event take.Event)
}

// Engine performs a storyboard's beats against a live [Terminal]: it types, presses keys, clicks
// and drags on-screen targets with real SGR mouse reports and waits on the screen, logging every
// beat start, action start and end, resolved target, click and drag cell into the terminal's
// take. It reads the
// screen only through [Terminal.Current] and [Terminal.Match] and writes only through
// [Terminal.Send], so what it sees is what the take records and what it sends is what the
// program reads.
type Engine struct {
	term     Terminal
	resolver target.Resolver
}

// New returns an engine driving term, resolving click and drag targets with resolver.
func New(term Terminal, resolver target.Resolver) *Engine {
	return &Engine{term: term, resolver: resolver}
}

// Run performs beats in order and stops at the first that fails, returning its *[BeatError].
// A cancelled ctx ends the action in progress and fails its beat with the context's error.
func (e *Engine) Run(ctx context.Context, beats []storyboard.Beat) error {
	for _, beat := range beats {
		if err := e.runBeat(ctx, beat); err != nil {
			return err
		}
	}
	return nil
}

// runBeat performs one beat's actions in order. A failing action ends the beat with a
// *BeatError naming the beat and the action; its action-end event is then never logged.
func (e *Engine) runBeat(ctx context.Context, beat storyboard.Beat) error {
	e.term.Event(take.Event{Kind: take.EventBeatStart, Beat: beat.ID, Text: beat.Title})
	for index, action := range beat.Do {
		step := take.Event{Beat: beat.ID, Action: index, Form: formOf(action), Text: textOf(action)}
		e.logStep(take.EventActionStart, step)
		if err := e.perform(ctx, step, action); err != nil {
			return &BeatError{
				Beat: beat.ID, Title: beat.Title, Action: index, Form: step.Form, Err: err,
			}
		}
		e.logStep(take.EventActionEnd, step)
	}
	return nil
}

// logStep logs step as an event of kind.
func (e *Engine) logStep(kind take.EventKind, step take.Event) {
	step.Kind = kind
	e.term.Event(step)
}

// BeatError is a beat that failed: which beat, which of its actions (its 0-based index in the
// beat's action list, and its form), and why.
type BeatError struct {
	Beat   int
	Title  string
	Action int
	Form   string
	Err    error
}

// Error names the beat and the action, then the cause.
func (e *BeatError) Error() string {
	return fmt.Sprintf("beat %d (%s): do[%d] %s: %v", e.Beat, e.Title, e.Action, e.Form, e.Err)
}

// Unwrap is the cause.
func (e *BeatError) Unwrap() error { return e.Err }
