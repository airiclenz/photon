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
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

// combiningGrapheme is "e" followed by U+0301 COMBINING ACUTE ACCENT: one grapheme, two code
// points, which a cell must keep together.
const combiningGrapheme = "é"

// sampleTake is a small take holding every cell attribute, every colour form, a wide and a
// combining-mark grapheme, and every event kind interleaved with the snapshots.
func sampleTake() *Take {
	blank := func() [][]Cell {
		row := func() []Cell {
			return []Cell{{Grapheme: " ", Width: 1}, {Grapheme: " ", Width: 1}, {Grapheme: " ", Width: 1}}
		}
		return [][]Cell{row(), row()}
	}
	painted := blank()
	painted[0][0] = Cell{
		Grapheme: "a", Width: 1,
		FG:   Color{Kind: ColorPalette, Index: 9},
		BG:   Color{Kind: ColorRGB, R: 0x1e, G: 0x1e, B: 0x2e},
		Bold: true, Italic: true,
	}
	painted[0][1] = Cell{Grapheme: "漢", Width: 2, Faint: true, Underline: true, Reverse: true}
	painted[0][2] = Cell{Width: 0}
	painted[1][0] = Cell{Grapheme: combiningGrapheme, Width: 1, FG: Color{Kind: ColorPalette, Index: 0}}
	painted[1][1] = Cell{Grapheme: "z", Width: 1, BG: Color{Kind: ColorPalette, Index: 255}}

	return &Take{
		Cols:    3,
		Rows:    2,
		FPS:     30,
		Started: time.Date(2026, time.October, 17, 9, 30, 0, 0, time.UTC),
		Snapshots: []Snapshot{
			{At: 0, Cursor: Cursor{Visible: true}, Cells: blank()},
			{At: 250 * time.Millisecond, Cursor: Cursor{X: 2, Y: 1}, Cells: painted},
		},
		Events: []Event{
			{At: 100 * time.Millisecond, Kind: EventBeatStart, Beat: 1, Text: "open"},
			{At: 120 * time.Millisecond, Kind: EventActionStart, Beat: 1, Form: "type", Text: "a漢" + combiningGrapheme},
			{At: 250 * time.Millisecond, Kind: EventActionEnd, Beat: 1, Form: "type"},
			{At: 300 * time.Millisecond, Kind: EventTarget, Beat: 1, Action: 1, Form: "click",
				Box: &Box{X: 0, Y: 1, W: 2, H: 1}},
			{At: 310 * time.Millisecond, Kind: EventClick, Beat: 1, Action: 1, Form: "click",
				Cell: &Point{X: 0, Y: 1}},
			{At: 400 * time.Millisecond, Kind: EventTarget, Beat: 1, Action: 2, Form: "drag", Text: "from",
				Box: &Box{X: 0, Y: 0, W: 1, H: 1}},
			{At: 410 * time.Millisecond, Kind: EventPress, Beat: 1, Action: 2, Form: "drag",
				Cell: &Point{X: 0, Y: 0}},
			{At: 420 * time.Millisecond, Kind: EventMove, Beat: 1, Action: 2, Form: "drag",
				Cell: &Point{X: 1, Y: 0}},
			{At: 430 * time.Millisecond, Kind: EventRelease, Beat: 1, Action: 2, Form: "drag",
				Cell: &Point{X: 2, Y: 0}},
		},
	}
}

func TestFile_SaveLoadRoundTrip(t *testing.T) {
	t.Parallel()
	want := sampleTake()
	path := filepath.Join(t.TempDir(), "hello.take")

	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)

	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip changed the take:\n got %+v\nwant %+v", got, want)
	}
}

func TestWrite_HeaderNamesFormatAndVersion(t *testing.T) {
	t.Parallel()
	var file bytes.Buffer
	if err := Write(&file, sampleTake()); err != nil {
		t.Fatalf("Write: %v", err)
	}

	first := firstLine(t, file.Bytes())

	const wantPrefix = `{"format":"photon-take","version":1,`
	if !strings.HasPrefix(first, wantPrefix) {
		t.Fatalf("header = %s, want it to start %s", first, wantPrefix)
	}
}

// sampleTakeLinesV1 is the decompressed lines sampleTake's file had before format version 2: a
// terminal take must still write exactly these.
var sampleTakeLinesV1 = []string{
	`{"format":"photon-take","version":1,"cols":3,"rows":2,"fps":30,"started":"2026-10-17T09:30:00Z"}`,
	`{"snap":{"at":0,"cursor":{"x":0,"y":0,"visible":true},"cells":[[{"g":" ","w":1},{"g":" ","w":1},{"g":" ","w":1}],[{"g":" ","w":1},{"g":" ","w":1},{"g":" ","w":1}]]}}`,
	`{"event":{"at":100000000,"kind":"beat-start","beat":1,"text":"open"}}`,
	`{"event":{"at":120000000,"kind":"action-start","beat":1,"form":"type","text":"a漢` + combiningGrapheme + `"}}`,
	`{"snap":{"at":250000000,"cursor":{"x":2,"y":1,"visible":false},"cells":[[{"g":"a","w":1,"fg":"@9","bg":"#1e1e2e","b":true,"i":true},{"g":"漢","w":2,"f":true,"u":true,"rv":true},{"w":0}],[{"g":"` + combiningGrapheme + `","w":1,"fg":"@0"},{"g":"z","w":1,"bg":"@255"},{"g":" ","w":1}]]}}`,
	`{"event":{"at":250000000,"kind":"action-end","beat":1,"form":"type"}}`,
	`{"event":{"at":300000000,"kind":"target","beat":1,"action":1,"form":"click","box":{"x":0,"y":1,"w":2,"h":1}}}`,
	`{"event":{"at":310000000,"kind":"click","beat":1,"action":1,"form":"click","cell":{"x":0,"y":1}}}`,
	`{"event":{"at":400000000,"kind":"target","beat":1,"action":2,"form":"drag","text":"from","box":{"x":0,"y":0,"w":1,"h":1}}}`,
	`{"event":{"at":410000000,"kind":"press","beat":1,"action":2,"form":"drag","cell":{"x":0,"y":0}}}`,
	`{"event":{"at":420000000,"kind":"move","beat":1,"action":2,"form":"drag","cell":{"x":1,"y":0}}}`,
	`{"event":{"at":430000000,"kind":"release","beat":1,"action":2,"form":"drag","cell":{"x":2,"y":0}}}`,
}

func TestWrite_TerminalTakeLinesUnchangedFromVersionOne(t *testing.T) {
	t.Parallel()

	got := takeLines(t, sampleTake())

	if !reflect.DeepEqual(got, sampleTakeLinesV1) {
		t.Fatalf("terminal take lines changed:\n got %q\nwant %q", got, sampleTakeLinesV1)
	}
}

// imageLayerTake is sampleTake with cell geometry and one snapshot carrying text runs,
// selector boxes and an image layer key frame.
func imageLayerTake() *Take {
	take := sampleTake()
	take.Cell = CellSize{W: 1.5, H: 1.5}
	take.Snapshots = append(take.Snapshots, Snapshot{
		At: take.Snapshots[len(take.Snapshots)-1].At + time.Millisecond,
		Runs: []TextRun{
			{Text: "Go", Box: Box{X: 4, Y: 8, W: 14, H: 18}, Edges: []int{4, 11, 18}},
		},
		Selectors: map[string][]Box{"#go": {{X: 2, Y: 6, W: 18, H: 22}}},
		Image: &ImageEntry{
			Key:  true,
			Rect: Box{W: 30, H: 40},
			PNG:  []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0xff},
		},
	})
	return take
}

func TestFile_VersionTwoRoundTrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		take func() *Take
	}{
		{"image layer, runs and cell", imageLayerTake},
		{"cell geometry alone", func() *Take {
			take := sampleTake()
			take.Cell = CellSize{W: 2, H: 2}
			return take
		}},
		{"runs alone", func() *Take {
			take := sampleTake()
			take.Snapshots[0].Runs = []TextRun{{Text: "a", Box: Box{W: 1, H: 1}, Edges: []int{0, 1}}}
			return take
		}},
		{"image patch alone", func() *Take {
			take := sampleTake()
			take.Snapshots[1].Image = &ImageEntry{Rect: Box{X: 3, Y: 4, W: 5, H: 6}, PNG: []byte{1, 2, 3}}
			return take
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			want := c.take()
			path := filepath.Join(t.TempDir(), "v2.take")

			if err := Save(path, want); err != nil {
				t.Fatalf("Save: %v", err)
			}
			got, err := Load(path)

			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("round trip changed the take:\n got %+v\nwant %+v", got, want)
			}
			const wantPrefix = `{"format":"photon-take","version":2,`
			if first := takeLines(t, want)[0]; !strings.HasPrefix(first, wantPrefix) {
				t.Fatalf("header = %s, want it to start %s", first, wantPrefix)
			}
		})
	}
}

func TestLoad_VersionOneReadsSelectors(t *testing.T) {
	t.Parallel()
	data := gzipLines(t,
		`{"format":"photon-take","version":1,"cols":1,"rows":1}`,
		`{"snap":{"at":0,"cursor":{"x":0,"y":0,"visible":false},"cells":[[{"g":"a","w":1}]],"sel":{"#a":[{"x":0,"y":0,"w":1,"h":1}]}}}`,
	)

	got, err := Read(bytes.NewReader(data))

	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want := map[string][]Box{"#a": {{W: 1, H: 1}}}
	if sel := got.Snapshots[0].Selectors; !reflect.DeepEqual(sel, want) {
		t.Fatalf("Selectors = %+v, want %+v", sel, want)
	}
}

func TestLoad_RefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	whole := encodeTake(t, sampleTake())
	const (
		head  = `{"format":"photon-take","version":1,"cols":3,"rows":2}`
		event = `{"event":{"at":0,"kind":"beat-start","beat":1}}`
		snap  = `{"snap":{"at":0,"cursor":{"x":0,"y":0,"visible":false},"cells":[[{"g":"a","w":1}]]}}`
	)
	cases := []struct {
		name  string
		data  []byte
		want  error
		texts []string
	}{
		{"plain text", []byte("not gzip at all"), ErrFormat, nil},
		{"another format", gzipLines(t, `{"format":"other-take","version":1,"cols":3,"rows":2}`), ErrFormat, nil},
		{"no version", gzipLines(t, `{"format":"photon-take","cols":3,"rows":2}`), ErrFormat, nil},
		{"newer version", gzipLines(t, `{"format":"photon-take","version":3,"cols":3,"rows":2}`), ErrNewerVersion, nil},
		{"newer version with a new field",
			gzipLines(t, `{"format":"photon-take","version":3,"cols":3,"rows":2,"palette":"dark"}`),
			ErrNewerVersion, nil},
		{"unknown header field",
			gzipLines(t, `{"format":"photon-take","version":1,"cols":3,"rows":2,"palette":"dark"}`),
			ErrFormat, nil},
		{"no terminal", gzipLines(t, `{"format":"photon-take","version":1,"cols":0,"rows":2}`), ErrFormat, nil},
		{"cell geometry on version 1",
			gzipLines(t, `{"format":"photon-take","version":1,"cols":3,"rows":2,"cell":{"w":2,"h":2}}`),
			ErrFormat, []string{"cell geometry"}},
		{"image entry on version 1", gzipLines(t, head, event,
			`{"snap":{"at":0,"cursor":{"x":0,"y":0,"visible":false},"cells":[],"img":{"key":true,"rect":{"x":0,"y":0,"w":1,"h":1},"png":""}}}`),
			ErrFormat, []string{"line 3:", `"img"`}},
		{"text runs on version 1", gzipLines(t, head,
			`{"snap":{"at":0,"cursor":{"x":0,"y":0,"visible":false},"cells":[],"runs":[]}}`),
			ErrFormat, []string{"line 2:", `"runs"`}},
		{"unknown line", gzipLines(t, head, `{}`), ErrFormat,
			[]string{"line 2 ", "neither one snapshot nor one event"}},
		{"bad colour", gzipLines(t, head,
			`{"snap":{"at":0,"cursor":{"x":0,"y":0,"visible":false},"cells":[[{"g":"a","w":1,"fg":"red"}]]}}`), ErrFormat,
			[]string{"line 2:"}},
		{"corrupt line", gzipLines(t, head, event, snap, `{"event":{"at":`, event), ErrFormat,
			[]string{"line 4:"}},
		{"two-field line", gzipLines(t, head, event,
			`{"snap":{"at":0,"cursor":{"x":0,"y":0,"visible":false},"cells":[]},"event":{"at":0,"kind":"click","beat":1}}`),
			ErrFormat, []string{"line 3 ", "neither one snapshot nor one event"}},
		{"unknown field", gzipLines(t, head, event, event, `{"event":{"at":0,"kind":"click","beat":1,"colour":"red"}}`),
			ErrFormat, []string{"line 4:", "colour"}},
		{"trailing bytes after a value", gzipLines(t, head, snap, event+` {"event":{}}`), ErrFormat,
			[]string{"line 3:", errTrailingData.Error()}},
		{"blank line before a value", gzipLines(t, head, event, "  ", "", event), ErrFormat,
			[]string{"line 3 is blank"}},
		{"truncated gzip", whole[:len(whole)-4], ErrFormat, nil},
		{"truncated mid-stream", whole[:len(whole)/2], ErrFormat, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "bad.take")
			if err := os.WriteFile(path, c.data, 0o600); err != nil {
				t.Fatalf("write the fixture: %v", err)
			}

			_, err := Load(path)

			if !errors.Is(err, c.want) {
				t.Fatalf("Load error = %v, want %v", err, c.want)
			}
			for _, text := range append([]string{path}, c.texts...) {
				if !strings.Contains(err.Error(), text) {
					t.Errorf("Load error = %v, want it to say %q", err, text)
				}
			}
		})
	}
}

func TestRead_NamesTheLowestBadLine(t *testing.T) {
	t.Parallel()
	lines := takeLines(t, largeTake(smallCols, smallRows, smallSnapshots))
	lines[6] = `{"event":{"at":"seven"}}`
	lines[2] = `{"snap":{"at":"three"}}`

	_, err := Read(bytes.NewReader(gzipLines(t, lines...)))

	if !errors.Is(err, ErrFormat) {
		t.Fatalf("Read error = %v, want %v", err, ErrFormat)
	}
	if !strings.Contains(err.Error(), "line 3:") || strings.Contains(err.Error(), "line 7") {
		t.Fatalf("Read error = %v, want it to name line 3 and not line 7", err)
	}
}

func TestRead_SkipsTrailingWhitespace(t *testing.T) {
	t.Parallel()
	want := sampleTake()
	lines := append(takeLines(t, want), "", " \t", "")

	got, err := Read(bytes.NewReader(gzipLines(t, lines...)))

	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Read changed the take:\n got %+v\nwant %+v", got, want)
	}
}

// Not parallel: it counts the goroutines the whole test binary runs.
func TestRead_EndsItsGoroutinesOnABadStream(t *testing.T) {
	lines := takeLines(t, largeTake(smallCols, smallRows, smallSnapshots))
	lines[3] = `{"snap":`
	whole := encodeTake(t, largeTake(smallCols, smallRows, smallSnapshots))
	cases := []struct {
		name string
		data []byte
	}{
		{"corrupt", gzipLines(t, lines...)},
		{"truncated", whole[:len(whole)/2]},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := runtime.NumGoroutine()

			_, err := Read(bytes.NewReader(c.data))

			if !errors.Is(err, ErrFormat) {
				t.Fatalf("Read error = %v, want %v", err, ErrFormat)
			}
			if after := settledGoroutines(before); after != before {
				t.Fatalf("goroutines = %d after Read, want %d as before it", after, before)
			}
		})
	}
}

func TestReadMatchesSequentialDecoder(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		take *Take
	}{
		{"sample", sampleTake()},
		{"large", largeTake(smallCols, smallRows, smallSnapshots)},
		{"image layer", imageLayerTake()},
		{"empty", &Take{Cols: 3, Rows: 2, FPS: 30,
			Started: time.Date(2026, time.November, 6, 9, 30, 0, 0, time.UTC)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			data := encodeTake(t, c.take)

			got, err := Read(bytes.NewReader(data))

			if err != nil {
				t.Fatalf("Read: %v", err)
			}
			want, err := readSequentially(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("readSequentially: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("Read and the sequential decoder differ:\n got %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestColor_TextSpellings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		color    Color
		spelling string
	}{
		{Color{}, ""},
		{Color{Kind: ColorPalette, Index: 0}, "@0"},
		{Color{Kind: ColorPalette, Index: 208}, "@208"},
		{Color{Kind: ColorRGB, R: 0x1e, G: 0x1e, B: 0x2e}, "#1e1e2e"},
	}
	for _, c := range cases {
		text, err := c.color.MarshalText()
		if err != nil || string(text) != c.spelling {
			t.Errorf("MarshalText(%+v) = %q, %v, want %q", c.color, text, err, c.spelling)
		}
		var read Color
		if err := read.UnmarshalText([]byte(c.spelling)); err != nil || read != c.color {
			t.Errorf("UnmarshalText(%q) = %+v, %v, want %+v", c.spelling, read, err, c.color)
		}
	}
	for _, bad := range []string{"@", "@256", "@-1", "#12345", "#zzzzzz", "#+12345", "red"} {
		var read Color
		if err := read.UnmarshalText([]byte(bad)); err == nil {
			t.Errorf("UnmarshalText(%q) = %+v, want an error", bad, read)
		}
	}
}

// encodeTake is t as a take file's bytes.
func encodeTake(t *testing.T, take *Take) []byte {
	t.Helper()
	var file bytes.Buffer
	if err := Write(&file, take); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return file.Bytes()
}

// gzipLines is a gzip stream of the given lines, each ended by a newline.
func gzipLines(t *testing.T, lines ...string) []byte {
	t.Helper()
	var file bytes.Buffer
	zw := gzip.NewWriter(&file)
	for _, text := range lines {
		if _, err := zw.Write([]byte(text + "\n")); err != nil {
			t.Fatalf("gzip a fixture line: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("finish the fixture: %v", err)
	}
	return file.Bytes()
}

// firstLine is the first line of the decompressed take file data.
func firstLine(t *testing.T, data []byte) string {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("open the gzip stream: %v", err)
	}
	var plain bytes.Buffer
	if _, err := plain.ReadFrom(zr); err != nil {
		t.Fatalf("decompress: %v", err)
	}
	first, _, _ := strings.Cut(plain.String(), "\n")
	return first
}

// takeLines is the decompressed lines of take's file, without their newlines.
func takeLines(t *testing.T, take *Take) []string {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(encodeTake(t, take)))
	if err != nil {
		t.Fatalf("open the gzip stream: %v", err)
	}
	var plain bytes.Buffer
	if _, err := plain.ReadFrom(zr); err != nil {
		t.Fatalf("decompress: %v", err)
	}
	return strings.Split(strings.TrimSuffix(plain.String(), "\n"), "\n")
}

// goroutineSettleTimeout is how long settledGoroutines waits for goroutines that have finished
// their work to leave the count.
const goroutineSettleTimeout = 2 * time.Second

// settledGoroutines is the goroutine count once it has fallen back to want, or what it is when
// goroutineSettleTimeout runs out: a goroutine past its last statement is counted a moment longer.
func settledGoroutines(want int) int {
	deadline := time.Now().Add(goroutineSettleTimeout)
	for runtime.NumGoroutine() > want && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	return runtime.NumGoroutine()
}

// readSequentially is the take decoder before lines were decoded in parallel: one strict
// decoder reading value after value to the end of the stream. The tests hold Read to it.
func readSequentially(r io.Reader) (*Take, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFormat, err)
	}
	defer zr.Close() //nolint:errcheck // a read-side close reports nothing the decode has not

	dec := json.NewDecoder(bufio.NewReader(zr))
	dec.DisallowUnknownFields()
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("%w: read the header: %w", ErrFormat, err)
	}
	take, version, err := readHeader(raw)
	if err != nil {
		return nil, err
	}
	for number := firstLineNumber; ; number++ {
		var next line
		err := dec.Decode(&next)
		if errors.Is(err, io.EOF) {
			return take, nil
		}
		if err != nil {
			return nil, fmt.Errorf("%w: line %d: %w", ErrFormat, number, err)
		}
		if err := checkLineVersion(number, next, version); err != nil {
			return nil, err
		}
		switch {
		case next.Snapshot != nil && next.Event == nil:
			take.Snapshots = append(take.Snapshots, *next.Snapshot)
		case next.Event != nil && next.Snapshot == nil:
			take.Events = append(take.Events, *next.Event)
		default:
			return nil, fmt.Errorf("%w: line %d is neither one snapshot nor one event",
				ErrFormat, number)
		}
	}
}
