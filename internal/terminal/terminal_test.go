//go:build !windows

package terminal

import (
	"errors"
	"fmt"
	"image/color"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"

	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/testkit"
)

// testTimeout bounds every wait on a running program: one that hangs on an unanswered query
// fails here rather than hanging the suite.
const testTimeout = 5 * time.Second

// The fixture terminal's size and the rows the fixture paints its status on.
const (
	fixtureCols      = 60
	fixtureRows      = 12
	fixtureStatusRow = fixtureRows - 3
	testFPS          = 30
)

// escapeScript starts a process in a session of its own that still holds the pty as its output,
// has it print its pid on row 0, and waits. Killing the program's process group leaves that
// process running with the slave open, so reading the master never ends by itself.
const escapeScript = `setsid sh -c 'echo "escaped $$"; exec sleep 600' & wait`

// The fixture's texts the tests wait for.
const (
	statusAnswered = "ready, terminal answered"
	typedStatus    = "you typed: hello"
	ctrlC          = "\x03"
)

// rowText is row y of a snapshot as plain text, trailing blanks trimmed, a wide grapheme counted
// once.
func rowText(snapshot take.Snapshot, y int) string {
	if y >= len(snapshot.Cells) {
		return ""
	}
	var text strings.Builder
	for _, cell := range snapshot.Cells[y] {
		switch {
		case cell.Width == 0:
		case cell.Grapheme == "":
			text.WriteString(" ")
		default:
			text.WriteString(cell.Grapheme)
		}
	}
	return strings.TrimRight(text.String(), " ")
}

// startFixture runs the fixture TUI in a fixture-sized terminal and closes it when the test ends.
func startFixture(t *testing.T) *Terminal {
	t.Helper()
	term, err := Start(testkit.BuildFixtureTUI(t), nil, Options{Cols: fixtureCols, Rows: fixtureRows, FPS: testFPS})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { term.Close() })
	return term
}

// awaitRow waits until row y reads want, matching (and so recording) that screen.
func awaitRow(t *testing.T, term *Terminal, y int, want string) {
	t.Helper()
	testkit.Eventually(t, testTimeout, func() error {
		if term.Match(func(s take.Snapshot) bool { return rowText(s, y) == want }) {
			return nil
		}
		return fmt.Errorf("row %d reads %q, want %q", y, rowText(term.Current(), y), want)
	})
}

// awaitExit waits for the program to exit.
func awaitExit(t *testing.T, term *Terminal) {
	t.Helper()
	select {
	case <-term.Done():
	case <-time.After(testTimeout):
		t.Fatalf("the program did not exit within %s", testTimeout)
	}
}

func TestTerminal_DrivesTheFixture(t *testing.T) {
	t.Parallel()
	term := startFixture(t)

	// The fixture asks the terminal who it is (DA1) before its first paint; this status proves
	// the emulator's reply reached it.
	awaitRow(t, term, fixtureStatusRow, statusAnswered)
	term.Event(take.Event{At: time.Hour, Kind: take.EventBeatStart, Beat: 1, Text: "greet"})
	if err := term.Send([]byte("hello\r")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	awaitRow(t, term, fixtureStatusRow, typedStatus)
	if err := term.Send([]byte(ctrlC)); err != nil {
		t.Fatalf("Send ctrl+c: %v", err)
	}
	awaitExit(t, term)
	if code := term.ExitCode(); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}

	recording := term.Close()
	if recording.Cols != fixtureCols || recording.Rows != fixtureRows || recording.FPS != testFPS {
		t.Fatalf("take header = %dx%d@%d, want %dx%d@%d",
			recording.Cols, recording.Rows, recording.FPS, fixtureCols, fixtureRows, testFPS)
	}
	if len(recording.Snapshots) == 0 || recording.Snapshots[0].At != 0 {
		t.Fatalf("the take does not open on a snapshot at 0 s: %d snapshot(s)", len(recording.Snapshots))
	}
	if got := rowText(recording.Snapshots[0], fixtureStatusRow); got != "" {
		t.Errorf("the first snapshot's status row = %q, want the blank screen", got)
	}
	var found []string
	for _, want := range []string{statusAnswered, typedStatus} {
		for _, snapshot := range recording.Snapshots {
			if rowText(snapshot, fixtureStatusRow) == want {
				found = append(found, want)
				break
			}
		}
	}
	if len(found) != 2 {
		t.Errorf("the take shows the statuses %q, want both %q and %q", found, statusAnswered, typedStatus)
	}
	for index := 1; index < len(recording.Snapshots); index++ {
		previous, snapshot := recording.Snapshots[index-1], recording.Snapshots[index]
		if snapshot.At < previous.At {
			t.Errorf("snapshot %d at %s comes before snapshot %d at %s", index, snapshot.At, index-1, previous.At)
		}
		if sameScreen(previous, snapshot) {
			t.Errorf("snapshots %d and %d show the same screen", index-1, index)
		}
	}
	if len(recording.Events) != 1 {
		t.Fatalf("events = %+v, want the one beat-start", recording.Events)
	}
	if event := recording.Events[0]; event.Kind != take.EventBeatStart || event.Beat != 1 ||
		event.Text != "greet" || event.At >= time.Hour {
		t.Errorf("event = %+v, want beat 1's start stamped on the take's clock", event)
	}
	if again := term.Close(); again != recording {
		t.Error("a second Close returned a different take")
	}
}

func TestTerminal_CollapsesIdenticalGrids(t *testing.T) {
	t.Parallel()
	// The second write repaints exactly what is already there.
	term, err := Start("sh", []string{"-c", `printf hi; sleep 0.3; printf '\rhi'; sleep 0.3`},
		Options{Cols: 20, Rows: 3, FPS: testFPS})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { term.Close() })
	awaitExit(t, term)
	recording := term.Close()

	if len(recording.Snapshots) != 2 {
		t.Fatalf("the take holds %d snapshots, want 2 (the blank screen, then \"hi\")", len(recording.Snapshots))
	}
	if got := rowText(recording.Snapshots[1], 0); got != "hi" {
		t.Errorf("row 0 = %q, want %q", got, "hi")
	}
}

func TestTerminal_CloseKillsARunningProgram(t *testing.T) {
	t.Parallel()
	term, err := Start("sleep", []string{"30"}, Options{Cols: 10, Rows: 2, FPS: testFPS})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	closed := make(chan *take.Take, 1)
	go func() { closed <- term.Close() }()
	select {
	case recording := <-closed:
		if recording == nil || len(recording.Snapshots) == 0 {
			t.Fatal("Close returned no take")
		}
	case <-time.After(testTimeout):
		t.Fatal("Close did not return")
	}
	if code := term.ExitCode(); code != signalExitCode {
		t.Errorf("exit code of a killed program = %d, want %d", code, signalExitCode)
	}
}

func TestTerminal_CloseReturnsWhileAnEscapedProcessHoldsThePTY(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("no setsid to start a process outside the program's session")
	}
	term := startShell(t, escapeScript)
	var escaped int
	testkit.Eventually(t, testTimeout, func() error {
		row := rowText(term.Current(), 0)
		pid, isCut := strings.CutPrefix(row, "escaped ")
		if !isCut {
			return fmt.Errorf("row 0 reads %q, want the escaped process's pid", row)
		}
		parsed, err := strconv.Atoi(pid)
		if err != nil {
			return fmt.Errorf("row 0 reads %q: %w", row, err)
		}
		escaped = parsed
		return nil
	})
	// Registered after startShell's cleanup, so it runs first: a Close stuck on the escaped
	// process's slave returns once that process is gone, and the suite does not hang on it.
	t.Cleanup(func() { _ = syscall.Kill(escaped, syscall.SIGKILL) })
	if group, err := syscall.Getpgid(escaped); err != nil || group == term.cmd.Process.Pid {
		t.Fatalf("process group of the escaped process = %d (%v), want one apart from %d", group, err, term.cmd.Process.Pid)
	}

	closed := make(chan struct{})
	go func() {
		term.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(testTimeout):
		t.Fatalf("Close did not return within %s while the escaped process %d held the pty", testTimeout, escaped)
	}
	if err := syscall.Kill(escaped, 0); err != nil {
		t.Errorf("the escaped process %d is gone (kill 0: %v), so the pty was not held through Close", escaped, err)
	}
}

func TestStart_RefusesBadOptions(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		opts Options
	}{
		{name: "no columns", opts: Options{Cols: 0, Rows: 5, FPS: testFPS}},
		{name: "too many rows", opts: Options{Cols: 40, Rows: maxSize + 1, FPS: testFPS}},
		{name: "no fps", opts: Options{Cols: 40, Rows: 5, FPS: 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := Start("true", nil, test.opts); err == nil {
				t.Errorf("Start(%+v) succeeded, want an error", test.opts)
			}
		})
	}
}

func TestAddSnapshot(t *testing.T) {
	t.Parallel()
	grid := func(text string) [][]take.Cell {
		return [][]take.Cell{{{Grapheme: text, Width: 1}}}
	}
	first := take.Snapshot{At: 0, Cells: grid("a")}
	for _, test := range []struct {
		name    string
		next    take.Snapshot
		isAdded bool
	}{
		{name: "same picture later", next: take.Snapshot{At: time.Second, Cells: grid("a")}, isAdded: false},
		{name: "a cell changed", next: take.Snapshot{At: time.Second, Cells: grid("b")}, isAdded: true},
		{name: "the cursor moved", next: take.Snapshot{At: time.Second, Cursor: take.Cursor{X: 1}, Cells: grid("a")}, isAdded: true},
		{name: "the cursor showed", next: take.Snapshot{At: time.Second, Cursor: take.Cursor{Visible: true}, Cells: grid("a")}, isAdded: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			recording := &take.Take{Snapshots: []take.Snapshot{first}}
			if got := addSnapshot(recording, test.next); got != test.isAdded {
				t.Errorf("addSnapshot = %v, want %v", got, test.isAdded)
			}
			if want := 1 + btoi(test.isAdded); len(recording.Snapshots) != want {
				t.Errorf("the take holds %d snapshots, want %d", len(recording.Snapshots), want)
			}
		})
	}
	if empty := (&take.Take{}); !addSnapshot(empty, first) {
		t.Error("addSnapshot refused the first snapshot of an empty take")
	}
}

// btoi is 1 for true and 0 for false.
func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestColorOf(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name  string
		color color.Color
		want  take.Color
	}{
		{name: "default", color: nil, want: take.Color{}},
		{name: "basic", color: ansi.BasicColor(1), want: take.Color{Kind: take.ColorPalette, Index: 1}},
		{name: "indexed", color: ansi.IndexedColor(200), want: take.Color{Kind: take.ColorPalette, Index: 200}},
		{name: "truecolor", color: ansi.RGBColor{R: 0x12, G: 0x34, B: 0x56},
			want: take.Color{Kind: take.ColorRGB, R: 0x12, G: 0x34, B: 0x56}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := colorOf(test.color); got != test.want {
				t.Errorf("colorOf(%v) = %+v, want %+v", test.color, got, test.want)
			}
		})
	}
}

func TestGridOf_ReadsStyledCells(t *testing.T) {
	t.Parallel()
	emulator := vt.NewEmulator(6, 2)
	if _, err := emulator.WriteString("\x1b[1;3;4;7;31mhi\x1b[0m\x1b[2;48;2;1;2;3m!\x1b[0m界"); err != nil {
		t.Fatalf("write to the emulator: %v", err)
	}
	row := gridOf(emulator.Width(), emulator.Height(), emulator.CellAt)[0]

	styled := take.Cell{Grapheme: "h", Width: 1, FG: take.Color{Kind: take.ColorPalette, Index: 1},
		Bold: true, Italic: true, Underline: true, Reverse: true}
	for _, test := range []struct {
		x    int
		want take.Cell
	}{
		{x: 0, want: styled},
		{x: 2, want: take.Cell{Grapheme: "!", Width: 1, BG: take.Color{Kind: take.ColorRGB, R: 1, G: 2, B: 3}, Faint: true}},
		{x: 3, want: take.Cell{Grapheme: "界", Width: 2}},
		{x: 4, want: take.Cell{Width: 0}},
		{x: 5, want: take.Cell{Grapheme: blankGrapheme, Width: 1}},
	} {
		if got := row[test.x]; got != test.want {
			t.Errorf("cell (%d,0) = %+v, want %+v", test.x, got, test.want)
		}
	}
	if got := cellOf((*uv.Cell)(nil)); got != (take.Cell{Grapheme: blankGrapheme, Width: 1}) {
		t.Errorf("cellOf(nil) = %+v, want a blank one column wide", got)
	}
}

func TestClampMargins_KeepsLineOperationsInsideTheScreen(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		sequence string
	}{
		// A bottom margin below the last row, then a home and a delete-line inside it.
		{name: "bottom margin", sequence: "\x1b[1;99r\x1b[H\x1b[M"},
		// DECLRMM on, a right margin past the last column, then the same.
		{name: "right margin", sequence: "\x1b[?69h\x1b[1;99s\x1b[H\x1b[M"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			emulator := vt.NewEmulator(10, 5)
			clampMargins(emulator)
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("the emulator panicked on %q: %v", test.sequence, recovered)
				}
			}()
			if _, err := emulator.WriteString("top\r\n" + test.sequence + "ok"); err != nil {
				t.Fatalf("write to the emulator: %v", err)
			}
			if got := cellOf(emulator.CellAt(0, 0)).Grapheme; got != "o" {
				t.Errorf("cell (0,0) = %q after the delete-line, want %q", got, "o")
			}
		})
	}
}

func TestClampParam(t *testing.T) {
	t.Parallel()
	const limit = 5
	for _, test := range []struct {
		name   string
		params ansi.Params
		want   int
	}{
		{name: "beyond the limit", params: ansi.Params{ansi.Param(1), ansi.Param(99)}, want: limit},
		{name: "within the limit", params: ansi.Params{ansi.Param(1), ansi.Param(3)}, want: 3},
		{name: "missing", params: ansi.Params{ansi.Param(1)}, want: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			clampParam(test.params, marginEnd, limit)
			got := -1
			if len(test.params) > marginEnd {
				got = test.params[marginEnd].Param(-1)
			}
			if got != test.want {
				t.Errorf("parameter = %d, want %d", got, test.want)
			}
		})
	}
}

func TestReplyQueue_DrainsBeforeEOF(t *testing.T) {
	t.Parallel()
	queue := newReplyQueue()
	queue.write([]byte("ab"))
	queue.close()
	buffer := make([]byte, 1)
	var read []byte
	for {
		n, err := queue.Read(buffer)
		read = append(read, buffer[:n]...)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatalf("Read: %v, want io.EOF", err)
			}
			break
		}
	}
	if string(read) != "ab" {
		t.Errorf("read %q, want %q", read, "ab")
	}
}
