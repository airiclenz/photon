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
// newer photon an [ErrNewerVersion] error.
func Read(r io.Reader) (*Take, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFormat, err)
	}
	defer zr.Close() //nolint:errcheck // a read-side close reports nothing the decode has not

	dec := json.NewDecoder(bufio.NewReader(zr))
	dec.DisallowUnknownFields()
	t, err := readHeader(dec)
	if err != nil {
		return nil, err
	}
	if err := readLines(dec, t); err != nil {
		return nil, err
	}
	return t, nil
}

// readHeader decodes and checks the first line, returning a take holding its fields. The format
// and version are read leniently first, so a newer photon's header, which may carry fields this
// one does not know, still reports [ErrNewerVersion]; only then is the line decoded strictly.
func readHeader(dec *json.Decoder) (*Take, error) {
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("%w: read the header: %w", ErrFormat, err)
	}

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
	strict := json.NewDecoder(bytes.NewReader(raw))
	strict.DisallowUnknownFields()
	if err := strict.Decode(&head); err != nil {
		return nil, fmt.Errorf("%w: read the header: %w", ErrFormat, err)
	}
	if head.Cols <= 0 || head.Rows <= 0 {
		return nil, fmt.Errorf("%w: the header gives a %dx%d terminal",
			ErrFormat, head.Cols, head.Rows)
	}
	return &Take{Cols: head.Cols, Rows: head.Rows, FPS: head.FPS, Started: head.Started}, nil
}

// readLines decodes every line after the header into t, to the end of the stream. A stream cut
// short fails here: gzip reports a missing trailer as an unexpected end.
func readLines(dec *json.Decoder, t *Take) error {
	for number := 2; ; number++ {
		var next line
		err := dec.Decode(&next)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%w: line %d: %w", ErrFormat, number, err)
		}
		switch {
		case next.Snapshot != nil && next.Event == nil:
			t.Snapshots = append(t.Snapshots, *next.Snapshot)
		case next.Event != nil && next.Snapshot == nil:
			t.Events = append(t.Events, *next.Event)
		default:
			return fmt.Errorf("%w: line %d is neither one snapshot nor one event",
				ErrFormat, number)
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
