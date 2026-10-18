//go:build !windows

package terminal

import (
	"context"
	"time"
)

// settlePollInterval is how often [Terminal.Settle] looks at how long the program has been quiet.
const settlePollInterval = 20 * time.Millisecond

// Painted closes at the program's first paint when the take records from it
// ([Options.FromFirstPaint]), and is already closed otherwise.
func (t *Terminal) Painted() <-chan struct{} { return t.painted }

// Freeze ends the take where it stands: the screen is recorded one last time if it changed since
// the last sample, and from then on no snapshot or event reaches the take, though the program
// keeps running and its output is still read. What a program paints on its way out, a quit hint
// or its restored primary screen, is how the recording ended, not part of the clip.
func (t *Terminal) Freeze() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dirty && t.recordingLocked() {
		t.sampleLocked(time.Since(t.start))
	}
	t.frozen = true
}

// Settle waits until the program has written nothing for quiet, and reports whether it did so,
// or exited, within timeout. A program with work in flight is seldom silent that long: a TUI
// animates while it is busy and goes still when it is idle. A cancelled ctx reports false.
func (t *Terminal) Settle(ctx context.Context, quiet, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	poll := time.NewTicker(settlePollInterval)
	defer poll.Stop()
	for {
		t.mu.Lock()
		silent := time.Since(t.lastOutput)
		t.mu.Unlock()
		if silent >= quiet {
			return true
		}
		select {
		case <-poll.C:
		case <-t.exited:
			return true
		case <-deadline.C:
			return false
		case <-ctx.Done():
			return false
		}
	}
}

// Quit asks the program to exit by sending each of keys as input, gap apart, and reports whether
// it exited within timeout of the last. A program already gone reports true at once. Quit only
// types: the keys reach whatever reads the pty, the app itself even with a shell wrapped around
// it. A program that does not leave is [Terminal.Close]'s to kill, with its whole process group.
func (t *Terminal) Quit(keys [][]byte, gap, timeout time.Duration) bool {
	for index, key := range keys {
		select {
		case <-t.exited:
			return true
		default:
		}
		if index > 0 {
			time.Sleep(gap)
		}
		if t.Send(key) != nil {
			// The program closed its input on the way out; the wait below sees it go.
			break
		}
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	select {
	case <-t.exited:
		return true
	case <-deadline.C:
		return false
	}
}

// screenDrawnLocked reports whether any cell of the screen shows something other than a blank.
// The caller holds mu.
func (t *Terminal) screenDrawnLocked() bool {
	for y := range t.emulator.Height() {
		for x := range t.emulator.Width() {
			cell := t.emulator.CellAt(x, y)
			if cell != nil && cell.Content != "" && cell.Content != blankGrapheme {
				return true
			}
		}
	}
	return false
}

// startAtPaintLocked restarts the take's clock at the first paint and records that paint as the
// take's first snapshot, at 0 s. An event logged before it is moved to 0 s rather than dropped.
// The caller holds mu.
func (t *Terminal) startAtPaintLocked() {
	now := time.Now()
	shift := now.Sub(t.start)
	for index := range t.take.Events {
		t.take.Events[index].At = max(t.take.Events[index].At-shift, 0)
	}
	t.start = now
	t.take.Started = now.UTC().Round(0)
	t.awaitingPaint = false
	t.sampleLocked(0)
	close(t.painted)
}
