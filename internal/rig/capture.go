package rig

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/airiclenz/photon/internal/browser"
	"github.com/airiclenz/photon/internal/engine"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/terminal"
)

// quitKeyGap parts two quit keys, like any two key presses of a beat: above the 50 ms escape
// timeout common TUI input parsers use, so two presses are read as two keys.
const quitKeyGap = 100 * time.Millisecond

// capture is what recording one take drives: the [engine.Source] the beats run against, plus the
// lifecycle around them — the first paint the beats wait for, the idle wait and freeze that end
// the take, and the shutdown that hands the take back. The record path speaks only to it, so a
// take's capture is a program in a pty or a page in a browser alike.
type capture interface {
	engine.Source
	// Painted closes once the capture shows its first frame, the take's first snapshot.
	Painted() <-chan struct{}
	// Settle waits until the capture has shown nothing new for quiet, and reports whether it did
	// so, or the program exited, within timeout. A cancelled ctx reports false.
	Settle(ctx context.Context, quiet, timeout time.Duration) bool
	// Freeze ends the take where it stands: from then on nothing reaches it.
	Freeze()
	// exitError says how the program ended, once [engine.Source.Done] has closed.
	exitError() error
	// finish shuts the capture down — the way a person would, unless ctx is cancelled — and
	// returns its take, nil when nothing was recorded, with any error recording or shutting down
	// met. It is called once, last.
	finish(ctx context.Context) (*take.Take, error)
}

// terminalCapture is a [capture] of a program in a pty: the terminal itself, plus the end-of-take
// keys that quit it and how long it is given to leave before it is killed.
type terminalCapture struct {
	*terminal.Terminal
	quitKeys  [][]byte
	killAfter time.Duration
}

// exitError names the program's exit status.
func (c terminalCapture) exitError() error {
	return fmt.Errorf("the program exited (status %d)", c.ExitCode())
}

// finish types the quit keys, quitKeyGap apart, and gives the program killAfter to leave; then it
// closes the terminal, which kills a program still running with its whole process group. A
// cancelled ctx skips the quit and goes straight to the kill. A terminal meets no error here.
func (c terminalCapture) finish(ctx context.Context) (*take.Take, error) {
	if ctx.Err() == nil {
		c.Quit(c.quitKeys, quitKeyGap, c.killAfter)
	}
	return c.Close(), nil
}

// errBrowserGone is how a page's take ends early: the tab or the browser went away.
var errBrowserGone = errors.New("the browser closed")

// browserCapture is a [capture] of a page in headless Chromium: the browser itself, which has
// loaded the page by the time it starts, and which a url launch closes at the end of the take,
// with no keys to quit it.
type browserCapture struct {
	*browser.Browser
}

// Painted is closed from the start: [browser.Start] returns once the page has loaded, and the
// screencast's first frame, which starts the take's clock, follows of itself. An event logged
// before that frame lands at 0 s, and a wait it answers is answered by the page as it stands.
func (browserCapture) Painted() <-chan struct{} {
	painted := make(chan struct{})
	close(painted)
	return painted
}

// exitError says the browser went away.
func (browserCapture) exitError() error { return errBrowserGone }

// finish closes the browser and returns its take with any error recording or closing met. A
// cancelled ctx has already ended the browser, so the close only collects the take.
func (c browserCapture) finish(context.Context) (*take.Take, error) {
	return c.Close()
}
