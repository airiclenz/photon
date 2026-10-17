//go:build !windows

package terminal

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"

	"github.com/airiclenz/photon/internal/take"
)

// maxSize is the largest terminal side a take may ask for: a pty's window is a pair of uint16s,
// and a size beyond this is a typo rather than an unusual terminal.
const maxSize = 4096

// What the terminal tells the program it is. They are appended after the caller's environment:
// exec keeps the last of a duplicated variable, so these always win.
const (
	termVariable      = "TERM=xterm-256color"
	colorTermVariable = "COLORTERM=truecolor"
)

// replySentinel ends the reply pump. It travels through the emulator's own reply pipe, the one
// thing that pump blocks on, and cannot be mistaken for a real reply, since no terminal reply
// holds a NUL. Closing the emulator instead would set a flag its reader reads unsynchronised.
const replySentinel = "\x00photon-terminal-close\x00"

// drainTimeout bounds how long [Terminal.Close] waits, once the program is reaped, for the pty
// to hand over the program's last bytes.
const drainTimeout = 2 * time.Second

// Buffer sizes of the pumps: one read of the program's output, one of the emulator's replies.
const (
	outputBufferSize = 32 * 1024
	replyBufferSize  = 4096
)

// signalExitCode is the exit code [Terminal.ExitCode] reports for a program a signal ended.
const signalExitCode = -1

// Options is the terminal a program runs in.
type Options struct {
	// Cols and Rows are the terminal's size in cells, each within 1..4096.
	Cols, Rows int
	// FPS caps how many snapshots a second the take samples; it must be positive.
	FPS int
	// Env is the program's whole environment: nothing is inherited, so a take says what the
	// program saw. TERM and COLORTERM are set last, whatever Env holds: what the terminal claims
	// to be is the terminal's business.
	Env []string
	// Dir is the program's working directory; "" is photon's own.
	Dir string
}

// validate reports an Options no terminal can be started with.
func (o Options) validate() error {
	if o.Cols <= 0 || o.Rows <= 0 || o.Cols > maxSize || o.Rows > maxSize {
		return fmt.Errorf("a terminal needs a size within 1..%d, got %dx%d", maxSize, o.Cols, o.Rows)
	}
	if o.FPS <= 0 {
		return fmt.Errorf("a terminal needs a positive fps, got %d", o.FPS)
	}
	return nil
}

// Terminal is a program running under a pseudo-terminal, emulated by x/vt and recorded into a
// [take.Take] as it paints. The program's output goes into the emulator, the emulator's replies
// to its queries go back to it as input, and photon's input goes in through [Terminal.Send].
// Every method is safe to call from any goroutine.
type Terminal struct {
	opts   Options
	start  time.Time
	cmd    *exec.Cmd
	master *os.File

	// mu guards the emulator and everything read off it: a snapshot is consistent only if no
	// output is parsed while it is taken.
	mu            sync.Mutex
	emulator      *vt.Emulator
	cursorVisible bool
	dirty         bool
	take          *take.Take

	// writeMu serialises the two writers of the master, photon's input and the emulator's
	// replies, so a reply is never spliced into the middle of a key's sequence.
	writeMu sync.Mutex
	replies *replyQueue

	exitCode int
	exited   chan struct{}

	stopSampling chan struct{}
	outputDone   chan struct{}
	repliesDone  chan struct{}
	answersDone  chan struct{}
	samplerDone  chan struct{}

	closing sync.Once
}

// Start runs name with args under an opts.Cols × opts.Rows pty and begins recording: the take
// opens on the blank screen at 0 s. The program is a session leader with the pty as its
// controlling terminal. Close the terminal to end the recording and get the take.
func Start(name string, args []string, opts Options) (*Terminal, error) {
	if err := opts.validate(); err != nil {
		return nil, err
	}
	emulator := vt.NewEmulator(opts.Cols, opts.Rows)
	clampMargins(emulator)
	t := &Terminal{
		opts:          opts,
		emulator:      emulator,
		cursorVisible: true,
		replies:       newReplyQueue(),
		exited:        make(chan struct{}),
		stopSampling:  make(chan struct{}),
		outputDone:    make(chan struct{}),
		repliesDone:   make(chan struct{}),
		answersDone:   make(chan struct{}),
		samplerDone:   make(chan struct{}),
	}
	// The callback runs inside emulator.Write, which only ever runs with mu held.
	emulator.SetCallbacks(vt.Callbacks{CursorVisibility: func(visible bool) { t.cursorVisible = visible }})

	cmd, master, err := startUnderPTY(name, args, opts)
	if err != nil {
		return nil, fmt.Errorf("start %s under a pty: %w", name, err)
	}
	t.cmd, t.master = cmd, master
	t.start = time.Now()
	t.take = &take.Take{Cols: opts.Cols, Rows: opts.Rows, FPS: opts.FPS, Started: t.start.UTC().Round(0)}
	t.mu.Lock()
	t.sampleLocked(0)
	t.mu.Unlock()

	go t.pumpOutput()
	go t.pumpReplies()
	go t.pumpAnswers()
	go t.sample()
	go t.reap()
	return t, nil
}

// startUnderPTY launches name on a fresh opts-sized pty and returns the running command and the
// pty's master. The parent's copy of the slave is closed either way, so once the program and
// everything it started are gone, reading the master ends.
func startUnderPTY(name string, args []string, opts Options) (*exec.Cmd, *os.File, error) {
	master, tty, err := pty.Open()
	if err != nil {
		return nil, nil, fmt.Errorf("open a pty: %w", err)
	}
	size := &pty.Winsize{Rows: uint16(opts.Rows), Cols: uint16(opts.Cols)}
	if err := pty.Setsize(master, size); err != nil {
		return nil, nil, errors.Join(fmt.Errorf("size the pty: %w", err), tty.Close(), master.Close())
	}
	cmd := exec.Command(name, args...)
	cmd.Env = append(slices.Clone(opts.Env), termVariable, colorTermVariable)
	cmd.Dir = opts.Dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}

	startErr := cmd.Start()
	ttyErr := tty.Close()
	if startErr != nil {
		return nil, nil, errors.Join(startErr, ttyErr, master.Close())
	}
	if ttyErr != nil {
		return nil, nil, errors.Join(fmt.Errorf("close the parent's end of the pty: %w", ttyErr),
			abandon(cmd), master.Close())
	}
	return cmd, master, nil
}

// abandon kills a program Start started but cannot record, with its whole process group, and
// reaps it. It reports a kill that failed; the exit a kill causes is the expected outcome.
func abandon(cmd *exec.Cmd) error {
	killErr := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	waitErr := cmd.Wait()
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		waitErr = nil
	}
	return errors.Join(killErr, waitErr)
}

// Send writes p to the program as input: typed text, a key's sequence, a mouse report.
func (t *Terminal) Send(p []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if _, err := t.master.Write(p); err != nil {
		return fmt.Errorf("write to the terminal: %w", err)
	}
	return nil
}

// Event logs event into the take, stamped now on the take's clock: whatever At it carries is
// replaced.
func (t *Terminal) Event(event take.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	event.At = time.Since(t.start)
	t.take.Events = append(t.take.Events, event)
}

// Current is what the terminal shows right now, stamped on the take's clock. It is read, not
// recorded: the take gets its snapshots from the sampler and from [Terminal.Match] alone.
func (t *Terminal) Current() take.Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshotLocked(time.Since(t.start))
}

// Match reports whether match holds for the screen as it stands, and when it does records that
// screen into the take, stamped now. The sampler takes one snapshot a frame, so a screen photon
// waited on could otherwise come and go between two samples, and what the take shows is what a
// check later judges. Matching and recording happen under one lock: the screen recorded is
// exactly the screen that matched.
func (t *Terminal) Match(match func(take.Snapshot) bool) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	screen := t.snapshotLocked(time.Since(t.start))
	if !match(screen) {
		return false
	}
	t.dirty = false
	addSnapshot(t.take, screen)
	return true
}

// Done closes once the program has exited and been reaped.
func (t *Terminal) Done() <-chan struct{} { return t.exited }

// ExitCode is the program's exit status, valid once [Terminal.Done] has closed; it waits for
// that. A program a signal ended reports -1.
func (t *Terminal) ExitCode() int {
	<-t.exited
	return t.exitCode
}

// Close ends the recording and returns the take. A program still running is killed with its whole
// process group; whatever it painted before it went is flushed into the take first. Close waits
// for every goroutine the terminal started, and a second call returns the same take.
func (t *Terminal) Close() *take.Take {
	t.closing.Do(func() {
		select {
		case <-t.exited:
		default:
			// Setsid made the program its own process-group leader, so -pid reaches everything it
			// started: a grandchild still holding the pty would keep the output pump alive. The
			// kill fails only when the group is already gone, and then reap sees the exit anyway.
			_ = syscall.Kill(-t.cmd.Process.Pid, syscall.SIGKILL)
		}
		<-t.exited
		// The output pump ends by itself once the last holder of the slave is gone and its buffer
		// is read. Something the program left behind holding the slave never lets that happen;
		// its bytes were pumped long ago, so after drainTimeout closing the master ends the pump.
		drain := time.NewTimer(drainTimeout)
		select {
		case <-t.outputDone:
		case <-drain.C:
		}
		drain.Stop()
		// Nothing reads or writes the master after this; a failed close leaves nothing to undo.
		_ = t.master.Close()
		<-t.outputDone
		close(t.stopSampling)
		<-t.samplerDone

		// Ends the reply pump and, through the queue it closes behind it, the answer pump. The
		// write fails only on a closed pipe, and this package never closes the emulator's.
		_, _ = io.WriteString(t.emulator.InputPipe(), replySentinel)
		<-t.repliesDone
		<-t.answersDone
	})
	return t.take
}

// pumpOutput feeds everything the program paints into the emulator. It ends when reading the
// master fails, which a pty master does once the program and everything it started are gone.
func (t *Terminal) pumpOutput() {
	defer close(t.outputDone)
	buffer := make([]byte, outputBufferSize)
	for {
		n, readErr := t.master.Read(buffer)
		if n > 0 && !t.paint(buffer[:n]) {
			return
		}
		if readErr != nil {
			return
		}
	}
}

// paint feeds one read of the program's output into the emulator, and reports whether the
// emulator took it; it refuses only once closed, after which nothing can be painted.
func (t *Terminal) paint(output []byte) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := t.emulator.Write(output); err != nil {
		return false
	}
	t.dirty = true
	return true
}

// pumpReplies drains the emulator's reply pipe into the reply queue. The pipe is unbuffered: an
// undrained reply would block the emulator's Write, and with it the output pump, while mu is held.
func (t *Terminal) pumpReplies() {
	defer close(t.repliesDone)
	defer t.replies.close()
	buffer := make([]byte, replyBufferSize)
	for {
		n, err := t.emulator.Read(buffer)
		if n > 0 {
			if bytes.Equal(buffer[:n], []byte(replySentinel)) {
				return
			}
			t.replies.write(buffer[:n])
		}
		if err != nil {
			return
		}
	}
}

// pumpAnswers hands the queued replies to the program. It is apart from pumpReplies so a program
// slow to read its input stalls only this goroutine, never the emulator. A write that fails means
// the program's input is gone; the replies after it have nowhere to go.
func (t *Terminal) pumpAnswers() {
	defer close(t.answersDone)
	buffer := make([]byte, replyBufferSize)
	for {
		n, err := t.replies.Read(buffer)
		if n > 0 && t.Send(buffer[:n]) != nil {
			return
		}
		if err != nil {
			return
		}
	}
}

// reap waits for the program and records how it ended.
func (t *Terminal) reap() {
	err := t.cmd.Wait()
	code := 0
	var exitErr *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exitErr):
		code = exitErr.ExitCode()
	default:
		code = signalExitCode
	}
	t.exitCode = code
	close(t.exited)
}

// replyQueue is an unbounded pipe with a blocking read, between the emulator's replies and the
// program's input. A bounded one could stall the emulator; a non-blocking read would spin.
type replyQueue struct {
	mu     sync.Mutex
	ready  *sync.Cond
	buffer []byte
	closed bool
}

// newReplyQueue returns an empty, open queue.
func newReplyQueue() *replyQueue {
	q := &replyQueue{}
	q.ready = sync.NewCond(&q.mu)
	return q
}

// Read blocks until there is a reply to hand on, or the queue is closed and drained.
func (q *replyQueue) Read(p []byte) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.buffer) == 0 && !q.closed {
		q.ready.Wait()
	}
	if len(q.buffer) == 0 {
		return 0, io.EOF
	}
	n := copy(p, q.buffer)
	q.buffer = q.buffer[n:]
	return n, nil
}

// write queues p behind the replies already waiting.
func (q *replyQueue) write(p []byte) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.buffer = append(q.buffer, p...)
	q.ready.Broadcast()
}

// close lets Read return io.EOF once the queue is drained.
func (q *replyQueue) close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.ready.Broadcast()
}
