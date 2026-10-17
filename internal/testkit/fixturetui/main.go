package main

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

// Terminal control sequences the fixture writes.
const (
	enterAltScreen  = "\x1b[?1049h"
	leaveAltScreen  = "\x1b[?1049l"
	enableSGRMouse  = "\x1b[?1002h\x1b[?1006h"
	disableSGRMouse = "\x1b[?1006l\x1b[?1002l"
	clearScreen     = "\x1b[H\x1b[2J"
	queryDA1        = "\x1b[c"
)

// Input bytes the fixture acts on.
const (
	ctrlC     = 0x03
	backspace = 0x08
	escape    = 0x1b
	deleteKey = 0x7f
	space     = 0x20
)

// The screen's fixed text and geometry.
const (
	title         = "photon fixture"
	inputPrompt   = "> "
	listIndent    = "  "
	selectedMark  = "* "
	footerText    = "type and enter to submit, click an item, ctrl+c to quit"
	hairlineGlyph = "─"
	firstListRow  = 2
	fallbackCols  = 80
	fallbackRows  = 24
)

// Status texts; typedPrefix, selectedPrefix and draggedPrefix lead the row after an enter, a
// click and a drag.
const (
	statusAnswered = "ready, terminal answered"
	statusSilent   = "ready, terminal silent"
	typedPrefix    = "you typed: "
	selectedPrefix = "selected: "
	draggedPrefix  = "dragged: "
)

// inputLogEnv names the file each input chunk is appended to, one Go-quoted chunk per line.
const inputLogEnv = "FIXTURE_INPUT_LOG"

// answerWait is how long the fixture waits for the terminal's answer to its DA1 query before it
// goes on without one.
const answerWait = 300 * time.Millisecond

// readBufferSize is the most input one read takes.
const readBufferSize = 256

// listItems are the rows a click selects.
var listItems = []string{"alpha", "beta", "gamma"}

func main() {
	os.Exit(run())
}

// run is the fixture's whole life: it returns the exit code once ctrl+c arrives or its input
// closes.
func run() int {
	inputLog, err := openInputLog()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixture tui:", err)
		return 1
	}
	if inputLog != nil {
		defer closeLog(inputLog)
	}

	stdin := int(os.Stdin.Fd())
	saved, err := term.MakeRaw(stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixture tui: raw mode:", err)
		return 1
	}
	defer restore(stdin, saved)

	write(enterAltScreen + enableSGRMouse)
	defer write(disableSGRMouse + leaveAltScreen)

	chunks := readChunks()
	model := screen{status: statusSilent}
	leftover, isAnswered := awaitAnswer(chunks)
	if isAnswered {
		model.status = statusAnswered
	}
	write(model.render(terminalSize()))
	if model.consume(leftover, inputLog) {
		return 0
	}
	for chunk := range chunks {
		if model.consume(chunk, inputLog) {
			return 0
		}
	}
	return 0
}

// screen is the fixture's state: what it draws and the input it has not finished parsing.
type screen struct {
	input    string
	status   string
	selected int
	pending  []byte
	hasPick  bool
	drag     gesture
}

// consume handles one chunk of input, logging it and repainting, and reports whether it asked the
// fixture to quit.
func (s *screen) consume(chunk []byte, inputLog *os.File) bool {
	if len(chunk) == 0 {
		return false
	}
	logChunk(inputLog, chunk)
	isQuit := s.handle(chunk)
	if !isQuit {
		write(s.render(terminalSize()))
	}
	return isQuit
}

// handle applies chunk to the state and reports whether it held ctrl+c. An escape sequence split
// across chunks waits in pending for the rest.
func (s *screen) handle(chunk []byte) bool {
	data := append(s.pending, chunk...)
	s.pending = nil
	for len(data) > 0 {
		switch b := data[0]; {
		case b == ctrlC:
			return true
		case b == escape:
			size, isComplete := sequenceLength(data)
			if !isComplete {
				s.pending = append([]byte(nil), data...)
				return false
			}
			s.handleSequence(data[:size])
			data = data[size:]
		case b == '\r' || b == '\n':
			s.status = typedPrefix + s.input
			s.input = ""
			data = data[1:]
		case b == deleteKey || b == backspace:
			s.input = dropLastRune(s.input)
			data = data[1:]
		case b < space:
			data = data[1:]
		default:
			r, size := utf8.DecodeRune(data)
			if r == utf8.RuneError && !utf8.FullRune(data) {
				s.pending = append([]byte(nil), data...)
				return false
			}
			s.input += string(data[:size])
			data = data[size:]
		}
	}
	return false
}

// handleSequence acts on one complete escape sequence: an SGR mouse report goes to
// handleMouse, and every other sequence is ignored.
func (s *screen) handleSequence(sequence []byte) {
	if report, isMouse := parseSGR(sequence); isMouse {
		s.handleMouse(report)
	}
}

// selectAt selects the list item whose text covers at, if any.
func (s *screen) selectAt(at cell) {
	index := at.row - firstListRow
	if index < 0 || index >= len(listItems) {
		return
	}
	start := len(listIndent)
	if at.column < start || at.column >= start+len(listItems[index]) {
		return
	}
	s.selected = index
	s.hasPick = true
	s.status = selectedPrefix + listItems[index]
}

// render returns the bytes that repaint the whole screen at cols × rows: the title, the list, the
// input row, then the status, footer and hairline on the last three rows, with the cursor left at
// the end of the input.
func (s *screen) render(cols, rows int) string {
	inputRow := firstListRow + len(listItems) + 1
	lines := map[int]string{0: title, inputRow: inputPrompt + s.input}
	for index, item := range listItems {
		mark := listIndent
		if s.hasPick && index == s.selected {
			mark = selectedMark
		}
		lines[firstListRow+index] = mark + item
	}
	lines[rows-3] = s.status
	lines[rows-2] = footerText
	lines[rows-1] = strings.Repeat(hairlineGlyph, cols)

	var out strings.Builder
	out.WriteString(clearScreen)
	for row := range rows {
		if line, ok := lines[row]; ok {
			fmt.Fprintf(&out, "\x1b[%d;1H%s", row+1, clip(line, cols))
		}
	}
	cursorColumn := min(utf8.RuneCountInString(inputPrompt+s.input), cols-1)
	fmt.Fprintf(&out, "\x1b[%d;%dH", inputRow+1, cursorColumn+1)
	return out.String()
}

// sequenceLength returns the length of the escape sequence data starts with and whether data
// holds all of it. A CSI runs to its final byte, an SS3 is three bytes, and any other escape is
// the escape byte alone.
func sequenceLength(data []byte) (int, bool) {
	if len(data) < 2 {
		return len(data), false
	}
	switch data[1] {
	case '[':
		for index := 2; index < len(data); index++ {
			if data[index] >= 0x40 && data[index] <= 0x7e {
				return index + 1, true
			}
		}
		return len(data), false
	case 'O':
		if len(data) < 3 {
			return len(data), false
		}
		return 3, true
	default:
		return 1, true
	}
}

// awaitAnswer asks the terminal who it is (DA1) and waits up to answerWait for the reply. It
// returns whatever input arrived that was not the reply, and whether a reply came. A terminal
// that never answers costs only the wait.
func awaitAnswer(chunks <-chan []byte) ([]byte, bool) {
	write(queryDA1)
	var received []byte
	deadline := time.After(answerWait)
	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				return received, false
			}
			received = append(received, chunk...)
			if rest, isAnswered := cutAnswer(received); isAnswered {
				return rest, true
			}
		case <-deadline:
			return received, false
		}
	}
}

// cutAnswer removes a complete DA1 reply (`ESC [ ? … c`) from data and reports whether one was
// there.
func cutAnswer(data []byte) ([]byte, bool) {
	start := bytes.Index(data, []byte("\x1b[?"))
	if start < 0 {
		return data, false
	}
	end := bytes.IndexByte(data[start:], 'c')
	if end < 0 {
		return data, false
	}
	rest := append([]byte(nil), data[:start]...)
	return append(rest, data[start+end+1:]...), true
}

// readChunks reads standard input on its own goroutine, one chunk per read, and closes the
// channel when the input ends.
func readChunks() <-chan []byte {
	chunks := make(chan []byte)
	go func() {
		defer close(chunks)
		buffer := make([]byte, readBufferSize)
		for {
			n, err := os.Stdin.Read(buffer)
			if n > 0 {
				chunks <- append([]byte(nil), buffer[:n]...)
			}
			if err != nil {
				return
			}
		}
	}()
	return chunks
}

// openInputLog opens the file inputLogEnv names for appending, or returns nil when it is unset.
func openInputLog() (*os.File, error) {
	path := os.Getenv(inputLogEnv)
	if path == "" {
		return nil, nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open the input log: %w", err)
	}
	return file, nil
}

// logChunk appends chunk to the input log as one Go-quoted line, so a reader can tell the reads
// apart. A failed write is reported on stderr: the fixture keeps running for the test to judge.
func logChunk(inputLog *os.File, chunk []byte) {
	if inputLog == nil {
		return
	}
	if _, err := fmt.Fprintln(inputLog, strconv.Quote(string(chunk))); err != nil {
		fmt.Fprintln(os.Stderr, "fixture tui: write the input log:", err)
	}
}

// terminalSize returns the terminal's columns and rows, or the fallback size when it cannot be
// read.
func terminalSize() (int, int) {
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || cols <= 0 || rows <= 0 {
		return fallbackCols, fallbackRows
	}
	return cols, rows
}

// clip cuts line to at most cols runes.
func clip(line string, cols int) string {
	if utf8.RuneCountInString(line) <= cols {
		return line
	}
	return string([]rune(line)[:cols])
}

// dropLastRune removes the last rune of text, if any.
func dropLastRune(text string) string {
	_, size := utf8.DecodeLastRuneInString(text)
	return text[:len(text)-size]
}

// write sends text to the terminal, reporting a failure on stderr.
func write(text string) {
	if _, err := os.Stdout.WriteString(text); err != nil {
		fmt.Fprintln(os.Stderr, "fixture tui: write the screen:", err)
	}
}

// restore puts the terminal back in the mode it had before raw mode.
func restore(stdin int, saved *term.State) {
	if err := term.Restore(stdin, saved); err != nil {
		fmt.Fprintln(os.Stderr, "fixture tui: restore the terminal:", err)
	}
}

// closeLog closes the input log, reporting a failure on stderr.
func closeLog(file *os.File) {
	if err := file.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "fixture tui: close the input log:", err)
	}
}
