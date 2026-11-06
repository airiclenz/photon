package take

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"
)

// FormatName is the format a take file's header names, and FormatVersion the newest version of
// it this photon writes and reads.
const (
	FormatName    = "photon-take"
	FormatVersion = 1
)

// ErrFormat is what a file that is not a readable take gets: another format, a missing or
// unknown version, a corrupt or truncated stream.
var ErrFormat = errors.New("not a photon take")

// ErrNewerVersion is what a take written by a newer photon gets: its header's version is past
// [FormatVersion].
var ErrNewerVersion = errors.New("take written by a newer photon")

// errTrailingData is what a line holding more than its one JSON value gets.
var errTrailingData = errors.New("data after the line's one value")

// The parallel decode of the lines after the header: at most maxLineWorkers workers decode
// lines, and at most linesInFlightPerWorker lines per worker are read but not yet in the take,
// which bounds the memory a long take costs while it loads.
const (
	maxLineWorkers         = 4
	linesInFlightPerWorker = 4
)

// firstLineNumber is the number of the first line after the header.
const firstLineNumber = 2

// header is a take file's first line: the format and version, then the take's own fields.
type header struct {
	Format  string    `json:"format"`
	Version int       `json:"version"`
	Cols    int       `json:"cols"`
	Rows    int       `json:"rows"`
	FPS     int       `json:"fps"`
	Started time.Time `json:"started"`
}

// line is one line of a take file after the header: exactly one of its fields is set.
type line struct {
	Snapshot *Snapshot `json:"snap,omitempty"`
	Event    *Event    `json:"event,omitempty"`
}

// Write writes t to w as a gzip-compressed JSON-lines stream: the header first, then every
// snapshot and every event in time order, one per line. Lines keep a take streamable, and gzip
// keeps the repeated cell spellings cheap.
func Write(w io.Writer, t *Take) error {
	zw := gzip.NewWriter(w)
	enc := json.NewEncoder(zw)
	head := header{
		Format: FormatName, Version: FormatVersion,
		Cols: t.Cols, Rows: t.Rows, FPS: t.FPS, Started: t.Started,
	}
	if err := enc.Encode(head); err != nil {
		return fmt.Errorf("write the take header: %w", err)
	}

	snapshot, event := 0, 0
	for snapshot < len(t.Snapshots) || event < len(t.Events) {
		var next line
		isSnapshotFirst := event >= len(t.Events) ||
			(snapshot < len(t.Snapshots) && t.Snapshots[snapshot].At <= t.Events[event].At)
		if isSnapshotFirst {
			next.Snapshot = &t.Snapshots[snapshot]
			snapshot++
		} else {
			next.Event = &t.Events[event]
			event++
		}
		if err := enc.Encode(next); err != nil {
			return fmt.Errorf("write the take: %w", err)
		}
	}

	if err := zw.Close(); err != nil {
		return fmt.Errorf("finish the take: %w", err)
	}
	return nil
}

// Read reads a take [Write] wrote. A stream that is not one is an [ErrFormat] error, one from a
// newer photon an [ErrNewerVersion] error. The lines after the header are decoded in parallel;
// the error a bad stream gets names its lowest-numbered failing line, and every goroutine Read
// starts has ended by the time it returns.
func Read(r io.Reader) (*Take, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFormat, err)
	}
	defer zr.Close() //nolint:errcheck // a read-side close reports nothing the decode has not

	stream := bufio.NewReader(zr)
	first, err := stream.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: read the header: %w", ErrFormat, err)
	}
	t, err := readHeader(first)
	if err != nil {
		return nil, err
	}
	if err := readLines(stream, t); err != nil {
		return nil, err
	}
	return t, nil
}

// readHeader checks the first line's bytes, returning a take holding its fields. The format and
// version are read leniently first, so a newer photon's header, which may carry fields this one
// does not know, still reports [ErrNewerVersion]; only then is the line decoded strictly.
func readHeader(raw []byte) (*Take, error) {
	var stamp struct {
		Format  string `json:"format"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(raw, &stamp); err != nil {
		return nil, fmt.Errorf("%w: read the header: %w", ErrFormat, err)
	}
	switch {
	case stamp.Format != FormatName:
		return nil, fmt.Errorf("%w: the header names format %q", ErrFormat, stamp.Format)
	case stamp.Version > FormatVersion:
		return nil, fmt.Errorf("%w: version %d, this photon reads up to %d",
			ErrNewerVersion, stamp.Version, FormatVersion)
	case stamp.Version < 1:
		return nil, fmt.Errorf("%w: the header gives version %d", ErrFormat, stamp.Version)
	}

	var head header
	if err := decodeStrict(raw, &head); err != nil {
		return nil, fmt.Errorf("%w: read the header: %w", ErrFormat, err)
	}
	if head.Cols <= 0 || head.Rows <= 0 {
		return nil, fmt.Errorf("%w: the header gives a %dx%d terminal",
			ErrFormat, head.Cols, head.Rows)
	}
	return &Take{Cols: head.Cols, Rows: head.Rows, FPS: head.FPS, Started: head.Started}, nil
}

// decodeStrict decodes data, one JSON value and nothing after it but whitespace, into value,
// refusing a field value does not have.
func decodeStrict(data []byte, value any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(value); err != nil {
		return err
	}
	if rest := data[dec.InputOffset():]; len(bytes.TrimSpace(rest)) > 0 {
		return errTrailingData
	}
	return nil
}

// lineResult is what became of one line: the line decoded, why it failed, or (isEnd) the end of
// the stream.
type lineResult struct {
	decoded line
	err     error
	isEnd   bool
}

// lineJob is one line for a worker to decode: its number, its bytes, and the slot its result
// goes to.
type lineJob struct {
	number int
	data   []byte
	result chan<- lineResult
}

// readLines decodes every line after the header into t, to the end of the stream. One goroutine
// reads the lines, and for each one first reserves a result slot in line order; workers decode
// the lines into their slots; readLines takes the slots in order, so the take keeps the stream's
// order and the first error it meets is the lowest-numbered failing line's. A stream cut short
// fails: gzip reports a missing trailer as an unexpected end.
func readLines(stream *bufio.Reader, t *Take) error {
	workers := min(runtime.GOMAXPROCS(0), maxLineWorkers)
	// A line is in flight from its slot's reservation until readLines takes the slot, and one
	// taken slot sits outside the channel: hence the one less.
	order := make(chan chan lineResult, workers*linesInFlightPerWorker-1)
	jobs := make(chan lineJob)
	stop := make(chan struct{})
	var running sync.WaitGroup

	feed := lineFeed{stream: stream, number: firstLineNumber - 1}
	running.Go(func() { feed.run(order, jobs, stop) })
	for range workers {
		running.Go(func() {
			for job := range jobs {
				decoded, err := decodeLine(job.number, job.data)
				job.result <- lineResult{decoded: decoded, err: err}
			}
		})
	}

	err := collectLines(order, t)
	close(stop)
	running.Wait()
	return err
}

// collectLines takes the result slots in line order and adds each decoded line to t, up to the
// end of the stream or the first failing line, whose error it returns.
func collectLines(order <-chan chan lineResult, t *Take) error {
	for {
		slot := <-order
		result := <-slot
		switch {
		case result.err != nil:
			return result.err
		case result.isEnd:
			return nil
		case result.decoded.Snapshot != nil:
			t.Snapshots = append(t.Snapshots, *result.decoded.Snapshot)
		default:
			t.Events = append(t.Events, *result.decoded.Event)
		}
	}
}

// decodeLine decodes the bytes of line number strictly: exactly one snapshot or one event.
func decodeLine(number int, data []byte) (line, error) {
	var decoded line
	if err := decodeStrict(data, &decoded); err != nil {
		return line{}, fmt.Errorf("%w: line %d: %w", ErrFormat, number, err)
	}
	if (decoded.Snapshot == nil) == (decoded.Event == nil) {
		return line{}, fmt.Errorf("%w: line %d is neither one snapshot nor one event",
			ErrFormat, number)
	}
	return decoded, nil
}

// lineFeed reads the lines after a take's header and hands them out for decoding.
type lineFeed struct {
	stream *bufio.Reader
	// number is the number of the last line read.
	number int
}

// run reserves a result slot on order, reads the next line and hands it to the workers on jobs,
// until it has filled a slot with the end of the stream or an error, or stop closes. It closes
// jobs when it returns, so the workers end once they have decoded what they hold.
func (f *lineFeed) run(order chan<- chan lineResult, jobs chan<- lineJob, stop <-chan struct{}) {
	defer close(jobs)
	for {
		slot := make(chan lineResult, 1)
		select {
		case order <- slot:
		case <-stop:
			return
		}

		data, blank, err := f.next()
		switch {
		case len(data) > 0 && blank != 0:
			slot <- lineResult{err: fmt.Errorf("%w: line %d is blank", ErrFormat, blank)}
			return
		case err != nil && !errors.Is(err, io.EOF):
			slot <- lineResult{err: fmt.Errorf("%w: line %d: %w", ErrFormat, f.number, err)}
			return
		case len(data) == 0:
			slot <- lineResult{isEnd: true}
			return
		}

		select {
		case jobs <- lineJob{number: f.number, data: data, result: slot}:
		case <-stop:
			return
		}
	}
}

// next reads up to the next line holding more than whitespace and returns it (nil when the
// stream ends first), the number of the first whitespace-only line it skipped on the way (0 for
// none), and the error that ended the stream. Whitespace-only lines are allowed only at the end
// of the stream, which is why their number is reported.
func (f *lineFeed) next() (data []byte, blank int, err error) {
	for {
		f.number++
		read, readErr := f.stream.ReadBytes('\n')
		if len(bytes.TrimSpace(read)) > 0 {
			return read, blank, readErr
		}
		if readErr != nil {
			return nil, blank, readErr
		}
		if blank == 0 {
			blank = f.number
		}
	}
}

// Save writes t to the file at path, replacing it.
func Save(path string, t *Take) (err error) {
	f, err := os.Create(path) //nolint:gosec // the path is photon's own take path for the clip
	if err != nil {
		return fmt.Errorf("create the take: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close the take %s: %w", path, closeErr)
		}
	}()
	if err := Write(f, t); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// Load reads the take file at path. Every error names the file; one that is not a readable take
// wraps [ErrFormat], one from a newer photon [ErrNewerVersion].
func Load(path string) (*Take, error) {
	f, err := os.Open(path) //nolint:gosec // the path is photon's own take path for the clip
	if err != nil {
		return nil, fmt.Errorf("open the take: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only

	t, err := Read(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return t, nil
}
