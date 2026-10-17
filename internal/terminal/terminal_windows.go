//go:build windows

package terminal

import (
	"context"
	"errors"
	"time"

	"github.com/airiclenz/photon/internal/take"
)

// ErrNoPTY is what [Start] returns on Windows. Recording is built on a unix pty pair (a session
// leader with a controlling terminal, a process-group kill), and a ConPTY stand-in would be a
// different recorder under the same name. Everything that reads a take still builds and runs
// here.
var ErrNoPTY = errors.New("recording needs a unix pty; not available on Windows")

// noExitCode is what [Terminal.ExitCode] reports here, where nothing runs.
const noExitCode = -1

// Options is the terminal a program runs in; see the unix build for what each field means.
type Options struct {
	Cols, Rows     int
	FPS            int
	Env            []string
	Dir            string
	FromFirstPaint bool
}

// Terminal is the type and nothing else on Windows: [Start] never returns one.
type Terminal struct{}

// Start refuses with [ErrNoPTY]: there is no pty to run a program under.
func Start(string, []string, Options) (*Terminal, error) { return nil, ErrNoPTY }

// Send refuses with [ErrNoPTY].
func (*Terminal) Send([]byte) error { return ErrNoPTY }

// Event does nothing.
func (*Terminal) Event(take.Event) {}

// Current is the empty snapshot.
func (*Terminal) Current() take.Snapshot { return take.Snapshot{} }

// Match judges the empty snapshot and records nothing.
func (*Terminal) Match(match func(take.Snapshot) bool) bool { return match(take.Snapshot{}) }

// Done is already closed: there is no program to wait for.
func (*Terminal) Done() <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}

// Painted is already closed: there is no paint to wait for.
func (*Terminal) Painted() <-chan struct{} {
	painted := make(chan struct{})
	close(painted)
	return painted
}

// Freeze does nothing: there is no take to end.
func (*Terminal) Freeze() {}

// Settle reports true at once, as for a program already gone: there is nothing to wait on.
func (*Terminal) Settle(context.Context, time.Duration, time.Duration) bool { return true }

// Quit reports true at once, as for a program already gone: nothing types and nothing runs.
func (*Terminal) Quit([][]byte, time.Duration, time.Duration) bool { return true }

// ExitCode is -1: nothing ran.
func (*Terminal) ExitCode() int { return noExitCode }

// Close returns no take.
func (*Terminal) Close() *take.Take { return nil }
