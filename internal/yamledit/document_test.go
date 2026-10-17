package yamledit

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// commentedFile is the hand-kept storyboard every round-trip case edits.
const commentedFile = "commented.yaml"

// edit is one call on a Document.
type edit func(document *Document) error

// readCommented returns the source of testdata/commented.yaml.
func readCommented(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", commentedFile))
	if err != nil {
		t.Fatalf("read %s: %v", commentedFile, err)
	}
	return string(raw)
}

// parse holds src for editing, failing t when it does not parse.
func parse(t *testing.T, src string) *Document {
	t.Helper()
	document, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return document
}

// swap returns src with old, which must occur in it exactly once, replaced by replacement: the
// golden result of an edit that touches only that span.
func swap(t *testing.T, src, old, replacement string) string {
	t.Helper()
	if count := strings.Count(src, old); count != 1 {
		t.Fatalf("fragment %q occurs %d times in the source, want once", old, count)
	}
	return strings.Replace(src, old, replacement, 1)
}

func TestEditsKeepEveryOtherByte(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		edit     edit
		old, new string
	}{
		{"set block plain", func(d *Document) error { return d.Set(Path{"beats", "0", "duration"}, "1850ms") },
			"duration: 1800ms", "duration: 1850ms"},
		{"set flow after a non-ASCII quoted scalar", func(d *Document) error { return d.Set(Path{"beats", "0", "zoom", "factor"}, "3") },
			"factor: 2", "factor: 3"},
		{"set flow frame field", func(d *Document) error { return d.Set(Path{"frame", "width"}, "640") },
			"width: 600", "width: 640"},
		{"set single-quoted with a doubled quote", func(d *Document) error { return d.Set(Path{"beats", "0", "title"}, Quote("it's shut")) },
			`'it''s open'`, `'it''s shut'`},
		{"set double-quoted with an escape", func(d *Document) error { return d.Set(Path{"beats", "1", "title"}, Quote("Plan")) },
			`"Überblick \u2713"`, `'Plan'`},
		{"set double-quoted in a flow sequence item", func(d *Document) error {
			return d.Set(Path{"beats", "0", "do", "0", "type", "text"}, Quote("bye"))
		}, `"say \"hi\"\tnow"`, `'bye'`},
		{"set non-ASCII quoted in nested flow", func(d *Document) error {
			return d.Set(Path{"beats", "0", "zoom", "target", "text"}, Quote("Größe"))
		}, `'Größe → ✓'`, `'Größe'`},
		{"insert block into a beat after a commented line", func(d *Document) error {
			return d.Insert(Path{"beats", "0"}, "hold", "500ms", []string{"id", "title", "duration"})
		}, "    duration: 1800ms  # measured\n", "    duration: 1800ms  # measured\n    hold: 500ms\n"},
		{"insert block after a key with nested lines", func(d *Document) error {
			return d.Insert(Path{"beats", "0"}, "cut", "true", []string{"do"})
		}, "      - key: {name: enter}\n", "      - key: {name: enter}\n    cut: true\n"},
		{"insert block into a sequence item with no after key", func(d *Document) error {
			return d.Insert(Path{"beats", "1"}, "why", "'x'", nil)
		}, "  - id: 2\n", "  - id: 2\n    why: 'x'\n"},
		{"insert block first at the root", func(d *Document) error { return d.Insert(nil, "x", "1", nil) },
			"clip: demo", "x: 1\nclip: demo"},
		{"insert block last at the root", func(d *Document) error { return d.Insert(nil, "end", "true", []string{"beats"}) },
			"    cut: true\n", "    cut: true\nend: true\n"},
		{"insert flow in the middle", func(d *Document) error {
			return d.Insert(Path{"frame"}, "scale", "2", []string{"cols", "rows"})
		}, "rows: 24, width", "rows: 24, scale: 2, width"},
		{"insert flow first", func(d *Document) error { return d.Insert(Path{"frame"}, "padding", "16", nil) },
			"{cols: 80", "{padding: 16, cols: 80"},
		{"insert flow after a nested flow value", func(d *Document) error {
			return d.Insert(Path{"beats", "0", "zoom"}, "x", "1", []string{"target"})
		}, "area: list}, factor", "area: list}, x: 1, factor"},
		{"insert flow last", func(d *Document) error {
			return d.Insert(Path{"beats", "0", "do", "1", "key"}, "repeat", "2", []string{"name"})
		}, "{name: enter}", "{name: enter, repeat: 2}"},
		{"delete block", func(d *Document) error { return d.Delete(Path{"beats", "1", "hold"}) },
			"    hold: 1s\n", ""},
		{"delete block with nested lines", func(d *Document) error { return d.Delete(Path{"beats", "0", "do"}) },
			"    do:\n      - type: {text: \"say \\\"hi\\\"\\tnow\"}\n      - key: {name: enter}\n", ""},
		{"delete block last key of the file", func(d *Document) error { return d.Delete(Path{"beats", "1", "cut"}) },
			"    cut: true\n", ""},
		{"delete block root key", func(d *Document) error { return d.Delete(Path{"ship"}) },
			"ship: demo.gif\n", ""},
		{"delete block first key of a sequence item", func(d *Document) error { return d.Delete(Path{"beats", "1", "id"}) },
			"  - id: 2\n    title", "  - title"},
		{"delete flow in the middle", func(d *Document) error { return d.Delete(Path{"frame", "rows"}) },
			"rows: 24, ", ""},
		{"delete flow last", func(d *Document) error { return d.Delete(Path{"frame", "fps"}) },
			", fps: 24", ""},
		{"delete flow first with a nested value", func(d *Document) error { return d.Delete(Path{"beats", "0", "zoom", "target"}) },
			"target: {text: 'Größe → ✓', area: list}, ", ""},
		{"delete flow only entry", func(d *Document) error { return d.Delete(Path{"beats", "0", "do", "1", "key", "name"}) },
			"{name: enter}", "{}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := readCommented(t)
			want := swap(t, src, tc.old, tc.new)
			document := parse(t, src)

			err := tc.edit(document)

			if err != nil {
				t.Fatalf("edit: %v", err)
			}
			if got := string(document.Bytes()); got != want {
				t.Errorf("edited source:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestEditsChainOnTrueOffsets(t *testing.T) {
	t.Parallel()
	src := readCommented(t)
	want := swap(t, src, `'it''s open'`, `'a longer title than before'`)
	want = swap(t, want, "    duration: 1800ms  # measured\n", "    duration: 1800ms  # measured\n    hold: 250ms\n")
	want = swap(t, want, ", fps: 24", "")
	want = swap(t, want, "factor: 2", "factor: 0")
	document := parse(t, src)

	edits := []edit{
		func(d *Document) error {
			return d.Set(Path{"beats", "0", "title"}, Quote("a longer title than before"))
		},
		func(d *Document) error { return d.Insert(Path{"beats", "0"}, "hold", "250ms", []string{"duration"}) },
		func(d *Document) error { return d.Delete(Path{"frame", "fps"}) },
		func(d *Document) error { return d.Set(Path{"beats", "0", "zoom", "factor"}, "0") },
	}
	for index, apply := range edits {
		if err := apply(document); err != nil {
			t.Fatalf("edit %d: %v", index, err)
		}
	}

	if got := string(document.Bytes()); got != want {
		t.Errorf("edited source:\n%s\nwant:\n%s", got, want)
	}
}

func TestEditsRefuse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		edit     edit
		want     error
		wantPath string
	}{
		{"set a missing index", func(d *Document) error { return d.Set(Path{"beats", "5", "duration"}, "1s") },
			ErrNotFound, "beats[5].duration"},
		{"set a missing key", func(d *Document) error { return d.Set(Path{"beats", "0", "nope"}, "1s") },
			ErrNotFound, "beats[0].nope"},
		{"step a sequence by a key", func(d *Document) error { return d.Set(Path{"beats", "x"}, "1") },
			ErrNotFound, "beats.x"},
		{"insert into a missing mapping", func(d *Document) error { return d.Insert(Path{"beats", "9"}, "hold", "1s", nil) },
			ErrNotFound, "beats[9]"},
		{"delete a missing key", func(d *Document) error { return d.Delete(Path{"frame", "nope"}) },
			ErrNotFound, "frame.nope"},
		{"set a mapping", func(d *Document) error { return d.Set(Path{"beats", "0", "zoom"}, "1") },
			ErrKind, "beats[0].zoom"},
		{"set a sequence", func(d *Document) error { return d.Set(Path{"beats"}, "1") },
			ErrKind, "beats"},
		{"step into a scalar", func(d *Document) error { return d.Set(Path{"beats", "0", "duration", "x"}, "1") },
			ErrKind, "beats[0].duration.x"},
		{"insert into a scalar", func(d *Document) error { return d.Insert(Path{"beats", "0", "duration"}, "k", "v", nil) },
			ErrKind, "beats[0].duration.k"},
		{"delete a sequence item", func(d *Document) error { return d.Delete(Path{"beats", "0"}) },
			ErrKind, "beats[0]"},
		{"delete the root", func(d *Document) error { return d.Delete(nil) },
			ErrKind, rootName},
		{"insert a present key", func(d *Document) error { return d.Insert(Path{"frame"}, "fps", "30", nil) },
			ErrExists, "frame.fps"},
		{"set a value spanning lines", func(d *Document) error { return d.Set(Path{"clip"}, "a\nb") },
			ErrUnsupported, "clip"},
		{"set a value breaking the YAML", func(d *Document) error { return d.Set(Path{"frame", "width"}, "{") },
			ErrUnsupported, "frame.width"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := readCommented(t)
			document := parse(t, src)

			err := tc.edit(document)

			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if !strings.Contains(err.Error(), tc.wantPath) {
				t.Errorf("error %q does not name the path %s", err, tc.wantPath)
			}
			if got := string(document.Bytes()); got != src {
				t.Errorf("a refused edit changed the source:\n%s", got)
			}
		})
	}
}

func TestSetRefusesUnscannableScalars(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
	}{
		{"block literal", "a: |\n  one\n"},
		{"block folded", "a: >\n  one\n"},
		{"plain over two lines", "a: one\n  two\nb: 1\n"},
		{"tagged", "a: !!str one\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			document := parse(t, tc.src)

			err := document.Set(Path{"a"}, "x")

			if !errors.Is(err, ErrUnsupported) {
				t.Errorf("error = %v, want %v", err, ErrUnsupported)
			}
		})
	}
}

func TestInsertAtEndWithoutTrailingNewline(t *testing.T) {
	t.Parallel()
	document := parse(t, "a: 1")

	err := document.Insert(nil, "b", "2", []string{"a"})

	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got, want := string(document.Bytes()), "a: 1\nb: 2"; got != want {
		t.Errorf("source = %q, want %q", got, want)
	}
}

func TestInsertIntoEmptyFlowMapping(t *testing.T) {
	t.Parallel()
	document := parse(t, "frame: {}  # none yet\n")

	err := document.Insert(Path{"frame"}, "fps", "24", nil)

	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if got, want := string(document.Bytes()), "frame: {fps: 24}  # none yet\n"; got != want {
		t.Errorf("source = %q, want %q", got, want)
	}
}

func TestParseRefuses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		src  string
	}{
		{"invalid YAML", "a: [1\n"},
		{"no document", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := Parse([]byte(tc.src))

			if err == nil {
				t.Error("Parse accepted the source")
			}
		})
	}
}

func TestQuoteReadsBack(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, text, want string
	}{
		{"plain text", "Größe", `'Größe'`},
		{"an apostrophe", "it's", `'it''s'`},
		{"a line break", "a\nb", `"a\nb"`},
		{"a tab", "a\tb", `"a\tb"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			quoted := Quote(tc.text)

			if quoted != tc.want {
				t.Errorf("Quote(%q) = %s, want %s", tc.text, quoted, tc.want)
			}
			var decoded map[string]string
			if err := yaml.Unmarshal([]byte("v: "+quoted), &decoded); err != nil {
				t.Fatalf("decode %s: %v", quoted, err)
			}
			if decoded["v"] != tc.text {
				t.Errorf("%s decodes to %q, want %q", quoted, decoded["v"], tc.text)
			}
		})
	}
}

func TestPathString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		path Path
		want string
	}{
		{Path{"beats", "0", "duration"}, "beats[0].duration"},
		{Path{"frame", "width"}, "frame.width"},
		{nil, rootName},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			if got := tc.path.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}
