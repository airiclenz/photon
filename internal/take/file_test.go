package take

import (
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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

func TestLoad_RefusesWhatItCannotRead(t *testing.T) {
	t.Parallel()
	whole := encodeTake(t, sampleTake())
	cases := []struct {
		name string
		data []byte
		want error
	}{
		{"plain text", []byte("not gzip at all"), ErrFormat},
		{"another format", gzipLines(t, `{"format":"other-take","version":1,"cols":3,"rows":2}`), ErrFormat},
		{"no version", gzipLines(t, `{"format":"photon-take","cols":3,"rows":2}`), ErrFormat},
		{"newer version", gzipLines(t, `{"format":"photon-take","version":2,"cols":3,"rows":2}`), ErrNewerVersion},
		{"newer version with a new field",
			gzipLines(t, `{"format":"photon-take","version":2,"cols":3,"rows":2,"palette":"dark"}`),
			ErrNewerVersion},
		{"unknown header field",
			gzipLines(t, `{"format":"photon-take","version":1,"cols":3,"rows":2,"palette":"dark"}`),
			ErrFormat},
		{"no terminal", gzipLines(t, `{"format":"photon-take","version":1,"cols":0,"rows":2}`), ErrFormat},
		{"unknown line", gzipLines(t, `{"format":"photon-take","version":1,"cols":3,"rows":2}`, `{}`), ErrFormat},
		{"bad colour", gzipLines(t, `{"format":"photon-take","version":1,"cols":3,"rows":2}`,
			`{"snap":{"at":0,"cursor":{"x":0,"y":0,"visible":false},"cells":[[{"g":"a","w":1,"fg":"red"}]]}}`), ErrFormat},
		{"truncated gzip", whole[:len(whole)-4], ErrFormat},
		{"truncated mid-stream", whole[:len(whole)/2], ErrFormat},
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
			if !strings.Contains(err.Error(), path) {
				t.Fatalf("Load error = %v, want it to name %s", err, path)
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
