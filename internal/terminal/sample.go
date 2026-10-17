//go:build !windows

package terminal

import (
	"time"

	"github.com/airiclenz/photon/internal/take"
)

// sample records the screen at most FPS times a second, and only when output arrived since the
// last sample. A snapshot is stamped with the tick that took it, so two samples are never closer
// than one frame ([Terminal.Match] may add a matched screen between two). When stopSampling
// closes it takes one last sample, so the take ends on what the program last painted.
func (t *Terminal) sample() {
	defer close(t.samplerDone)
	ticker := time.NewTicker(time.Second / time.Duration(t.opts.FPS))
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			t.sampleIfDirty()
		case <-t.stopSampling:
			t.sampleIfDirty()
			return
		}
	}
}

// sampleIfDirty records the screen when output arrived since the last sample and the take is
// recording.
func (t *Terminal) sampleIfDirty() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dirty && t.recordingLocked() {
		t.sampleLocked(time.Since(t.start))
	}
}

// sampleLocked adds the screen as it stands, stamped at, to the take unless it shows the same
// picture as the last snapshot. The caller holds mu and has checked the take is recording.
func (t *Terminal) sampleLocked(at time.Duration) {
	t.dirty = false
	addSnapshot(t.take, t.snapshotLocked(at))
}

// recordingLocked reports whether a screen reaches the take now: not before a FromFirstPaint
// take's first paint, and not after [Terminal.Freeze]. The caller holds mu.
func (t *Terminal) recordingLocked() bool {
	return !t.awaitingPaint && !t.frozen
}

// snapshotLocked reads the emulator into a snapshot stamped at. The caller holds mu.
func (t *Terminal) snapshotLocked(at time.Duration) take.Snapshot {
	position := t.emulator.CursorPosition()
	return take.Snapshot{
		At:     at,
		Cursor: take.Cursor{X: position.X, Y: position.Y, Visible: t.cursorVisible},
		Cells:  gridOf(t.emulator.Width(), t.emulator.Height(), t.emulator.CellAt),
	}
}

// addSnapshot appends snapshot to recording unless it shows the same picture as the last one,
// and reports whether it did. A take holds a picture once, from the moment it first appeared.
func addSnapshot(recording *take.Take, snapshot take.Snapshot) bool {
	if n := len(recording.Snapshots); n > 0 && sameScreen(recording.Snapshots[n-1], snapshot) {
		return false
	}
	recording.Snapshots = append(recording.Snapshots, snapshot)
	return true
}

// sameScreen reports whether two snapshots show the same picture: the same cells and the same
// cursor. Their times do not count; the question is whether the second adds a frame.
func sameScreen(a, b take.Snapshot) bool {
	if a.Cursor != b.Cursor || len(a.Cells) != len(b.Cells) {
		return false
	}
	for y := range a.Cells {
		if len(a.Cells[y]) != len(b.Cells[y]) {
			return false
		}
		for x := range a.Cells[y] {
			if a.Cells[y][x] != b.Cells[y][x] {
				return false
			}
		}
	}
	return true
}
