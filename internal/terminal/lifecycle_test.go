//go:build !windows

package terminal

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/testkit"
)

// The shell wrappers the tests launch the fixture through, with its path as $1. Neither ends on
// the fixture, so no shell can exec it in place: sh stays between the pty and the app, as it does
// for a launch command written without exec.
const (
	wrapperScript       = `"$1"; status=$?; exit "$status"`
	bannerWrapperScript = `echo wrapper banner; sleep 0.2; "$1"; status=$?; exit "$status"`
	wrapperBanner       = "wrapper banner"
)

// ignoresQuitScript ignores ctrl+c in a shell and in a grandchild it starts, and prints the
// grandchild's pid on row 0 so a test can watch it go. Both ignore the hangup the kernel sends
// when the session leader dies too, so only a kill of the whole process group ends the grandchild.
const ignoresQuitScript = `trap '' INT HUP; sleep 60 & echo "grandchild $!"; wait`

// Waits the lifecycle tests hand the terminal: short enough to keep the suite quick, long enough
// for a loaded host.
const (
	quitTimeout   = 3 * time.Second
	refusedQuit   = 300 * time.Millisecond
	quietSpell    = 150 * time.Millisecond
	busySpell     = 2 * time.Second
	settleTimeout = 400 * time.Millisecond
	keyGap        = 20 * time.Millisecond
)

// startWrapped runs the fixture TUI through sh running script, in a fixture-sized terminal, and
// closes it when the test ends.
func startWrapped(t *testing.T, script string, opts Options) *Terminal {
	t.Helper()
	opts.Cols, opts.Rows, opts.FPS = fixtureCols, fixtureRows, testFPS
	term, err := Start("sh", []string{"-c", script, "sh", testkit.BuildFixtureTUI(t)}, opts)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { term.Close() })
	return term
}

// startShell runs script under sh in a small terminal and closes it when the test ends.
func startShell(t *testing.T, script string) *Terminal {
	t.Helper()
	term, err := Start("sh", []string{"-c", script}, Options{Cols: 40, Rows: 3, FPS: testFPS})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { term.Close() })
	return term
}

// isClosed reports whether ch is closed already.
func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// showsText reports whether any row of snapshot holds text.
func showsText(snapshot take.Snapshot, text string) bool {
	for y := range snapshot.Cells {
		if strings.Contains(rowText(snapshot, y), text) {
			return true
		}
	}
	return false
}

func TestTerminal_QuitsAWrappedFixtureWithCtrlC(t *testing.T) {
	t.Parallel()
	term := startWrapped(t, wrapperScript, Options{})
	awaitRow(t, term, fixtureStatusRow, statusAnswered)

	if !term.Quit([][]byte{[]byte(ctrlC)}, keyGap, quitTimeout) {
		t.Fatalf("Quit reported the fixture still running after %s", quitTimeout)
	}
	if code := term.ExitCode(); code != 0 {
		t.Errorf("exit code = %d, want the fixture's 0 passed on by its wrapper", code)
	}
	if !term.Quit([][]byte{[]byte(ctrlC)}, keyGap, time.Nanosecond) {
		t.Error("Quit on a program already gone reported it still running")
	}
}

func TestTerminal_CloseKillsTheGroupAQuitCouldNotEnd(t *testing.T) {
	t.Parallel()
	term := startShell(t, ignoresQuitScript)
	var grandchild int
	testkit.Eventually(t, testTimeout, func() error {
		row := rowText(term.Current(), 0)
		pid, isCut := strings.CutPrefix(row, "grandchild ")
		if !isCut {
			return fmt.Errorf("row 0 reads %q, want the grandchild's pid", row)
		}
		parsed, err := strconv.Atoi(pid)
		if err != nil {
			return fmt.Errorf("row 0 reads %q: %w", row, err)
		}
		grandchild = parsed
		return nil
	})

	if term.Quit([][]byte{[]byte(ctrlC), []byte(ctrlC)}, keyGap, refusedQuit) {
		t.Fatal("Quit reported a program that ignores ctrl+c gone")
	}
	// A grandchild left running would hold the pty and keep Close waiting on it.
	closed := make(chan struct{})
	go func() {
		term.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(testTimeout):
		t.Fatalf("Close did not return within %s: the wrapper's grandchild %d outlived the kill", testTimeout, grandchild)
	}
	if code := term.ExitCode(); code != signalExitCode {
		t.Errorf("exit code = %d, want %d for a killed wrapper", code, signalExitCode)
	}
	testkit.Eventually(t, testTimeout, func() error {
		if err := syscall.Kill(grandchild, 0); !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("the wrapper's grandchild %d is still there (kill 0: %v)", grandchild, err)
		}
		return nil
	})
}

func TestTerminal_FromFirstPaintSkipsTheWrapper(t *testing.T) {
	t.Parallel()
	term := startWrapped(t, bannerWrapperScript, Options{FromFirstPaint: true})
	launched := time.Now()
	term.Event(take.Event{Kind: take.EventBeatStart, Beat: 1, Text: "before paint"})

	select {
	case <-term.Painted():
	case <-time.After(testTimeout):
		t.Fatalf("Painted did not close within %s", testTimeout)
	}
	awaitRow(t, term, fixtureStatusRow, statusAnswered)
	// The fixture restores the primary screen, banner and all, on its way out.
	term.Freeze()
	if !term.Quit([][]byte{[]byte(ctrlC)}, keyGap, quitTimeout) {
		t.Fatalf("Quit reported the fixture still running after %s", quitTimeout)
	}
	recording := term.Close()

	if len(recording.Snapshots) == 0 {
		t.Fatal("the take holds no snapshot")
	}
	first := recording.Snapshots[0]
	if first.At != 0 || !hasDrawnCell(first) {
		t.Errorf("the first snapshot is at %s and drawn: %v, want the first paint at 0 s", first.At, hasDrawnCell(first))
	}
	for index, snapshot := range recording.Snapshots {
		if showsText(snapshot, wrapperBanner) {
			t.Errorf("snapshot %d shows the wrapper's banner", index)
		}
	}
	if !recording.Started.After(launched) {
		t.Errorf("the take started at %s, want after the launch at %s", recording.Started, launched)
	}
	if len(recording.Events) != 1 || recording.Events[0].At != 0 {
		t.Errorf("events = %+v, want the one logged before the paint, at 0 s", recording.Events)
	}
}

// hasDrawnCell reports whether any cell of snapshot shows something other than a blank.
func hasDrawnCell(snapshot take.Snapshot) bool {
	for _, row := range snapshot.Cells {
		for _, cell := range row {
			if cell.Grapheme != "" && cell.Grapheme != blankGrapheme {
				return true
			}
		}
	}
	return false
}

func TestTerminal_FromFirstPaintRecordsNothingUnpainted(t *testing.T) {
	t.Parallel()
	term, err := Start("sh", []string{"-c", "echo only the primary screen; sleep 0.2"},
		Options{Cols: 40, Rows: 3, FPS: testFPS, FromFirstPaint: true})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { term.Close() })
	awaitExit(t, term)
	recording := term.Close()

	if isClosed(term.Painted()) {
		t.Error("Painted closed for a program that never drew on the alternate screen")
	}
	if len(recording.Snapshots) != 0 {
		t.Errorf("the take holds %d snapshot(s), want none before a first paint", len(recording.Snapshots))
	}
}

func TestTerminal_PaintedIsClosedWithoutFromFirstPaint(t *testing.T) {
	t.Parallel()
	term := startFixture(t)
	if !isClosed(term.Painted()) {
		t.Error("Painted is open on a take that records from launch")
	}
}

func TestTerminal_FreezeEndsTheTake(t *testing.T) {
	t.Parallel()
	term := startFixture(t)
	awaitRow(t, term, fixtureStatusRow, statusAnswered)
	term.Freeze()
	term.Event(take.Event{Kind: take.EventBeatStart, Beat: 1, Text: "after freeze"})
	if err := term.Send([]byte("hello\r")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	// Matching still sees the screen; it records nothing.
	awaitRow(t, term, fixtureStatusRow, typedStatus)
	recording := term.Close()

	for index, snapshot := range recording.Snapshots {
		if rowText(snapshot, fixtureStatusRow) == typedStatus {
			t.Errorf("snapshot %d shows what was typed after the freeze", index)
		}
	}
	if last := recording.Snapshots[len(recording.Snapshots)-1]; rowText(last, fixtureStatusRow) != statusAnswered {
		t.Errorf("the take ends on status %q, want %q, the screen at the freeze",
			rowText(last, fixtureStatusRow), statusAnswered)
	}
	if len(recording.Events) != 0 {
		t.Errorf("events = %+v, want none: the one logged after the freeze is dropped", recording.Events)
	}
}

func TestTerminal_Settle(t *testing.T) {
	t.Parallel()
	const busyScript = `while :; do printf x; sleep 0.01; done`
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, test := range []struct {
		name    string
		script  string
		ctx     context.Context
		quiet   time.Duration
		timeout time.Duration
		want    bool
	}{
		{name: "a quiet program settles", script: "printf ready; sleep 30", ctx: context.Background(),
			quiet: quietSpell, timeout: testTimeout, want: true},
		{name: "a busy program does not", script: busyScript, ctx: context.Background(),
			quiet: busySpell, timeout: settleTimeout, want: false},
		{name: "a cancelled wait does not", script: busyScript, ctx: cancelled,
			quiet: busySpell, timeout: testTimeout, want: false},
		{name: "a program that exits settles", script: busyScript + " & sleep 0.1; exit 0", ctx: context.Background(),
			quiet: busySpell, timeout: testTimeout, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			term := startShell(t, test.script)
			if got := term.Settle(test.ctx, test.quiet, test.timeout); got != test.want {
				t.Errorf("Settle = %v, want %v", got, test.want)
			}
		})
	}
}
