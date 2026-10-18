package yamledit

import (
	"errors"
	"strings"
	"testing"
)

// beatOrder is a key order matching the first beat of testdata/commented.yaml, with keys it
// lacks between those it holds.
var beatOrder = []string{"id", "title", "why", "duration", "hold", "zoom", "do", "cut"}

// frameOrder is a key order matching the frame of testdata/commented.yaml, with keys it lacks
// between those it holds.
var frameOrder = []string{"cols", "rows", "scale", "width", "fps"}

func TestUpsertKeepsEveryOtherByte(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		mapping  Path
		key      string
		value    string
		order    []string
		old, new string
	}{
		{"block present scalar", Path{"beats", "0"}, "duration", "2500ms", beatOrder,
			"duration: 1800ms", "duration: 2500ms"},
		{"block present flow mapping", Path{"beats", "0"}, "zoom", "{factor: 3}", beatOrder,
			"zoom: {target: {text: 'Größe → ✓', area: list}, factor: 2, in: 1s, out: 1s}",
			"zoom: {factor: 3}"},
		{"block present block sequence", Path{"beats", "0"}, "do", "[]", beatOrder,
			"    do:\n      - type: {text: \"say \\\"hi\\\"\\tnow\"}\n      - key: {name: enter}\n",
			"    do: []\n"},
		{"block absent key in order", Path{"beats", "0"}, "hold", "500ms", beatOrder,
			"    duration: 1800ms  # measured\n", "    duration: 1800ms  # measured\n    hold: 500ms\n"},
		{"block absent key out of order", nil, "end", "true", nil,
			"    cut: true\n", "    cut: true\nend: true\n"},
		{"flow present scalar", Path{"frame"}, "fps", "30", frameOrder,
			"fps: 24", "fps: 30"},
		{"flow present nested mapping", Path{"beats", "0", "zoom"}, "target", "{text: 'x'}", nil,
			"target: {text: 'Größe → ✓', area: list}", "target: {text: 'x'}"},
		{"flow absent key in order", Path{"frame"}, "scale", "2", frameOrder,
			"rows: 24, width", "rows: 24, scale: 2, width"},
		{"flow absent key out of order", Path{"frame"}, "speed", "1", frameOrder,
			"fps: 24}", "fps: 24, speed: 1}"},
		{"sequence item present first key", Path{"beats", "1"}, "id", "7", nil,
			"  - id: 2\n", "  - id: 7\n"},
		{"sequence item absent key in order", Path{"beats", "1"}, "why", "'x'", beatOrder,
			`    title: "Überblick \u2713"` + "\n", `    title: "Überblick \u2713"` + "\n    why: 'x'\n"},
		{"sequence item absent key out of order", Path{"beats", "1"}, "speed", "2", beatOrder,
			"    cut: true\n", "    cut: true\n    speed: 2\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := readCommented(t)
			want := swap(t, src, tc.old, tc.new)
			document := parse(t, src)

			err := document.Upsert(tc.mapping, tc.key, tc.value, tc.order)

			if err != nil {
				t.Fatalf("Upsert: %v", err)
			}
			if got := string(document.Bytes()); got != want {
				t.Errorf("edited source:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestUpsertRefuses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		src      string
		mapping  Path
		key      string
		want     error
		wantPath string
	}{
		{"a present block scalar", "a: |\n  one\n", nil, "a", ErrUnsupported, "a"},
		{"a missing mapping", "beats:\n  - id: 1\n", Path{"beats", "4"}, "hold", ErrNotFound, "beats[4]"},
		{"a sequence as the mapping", "beats:\n  - id: 1\n", Path{"beats"}, "0", ErrKind, "beats[0]"},
		{"a scalar as the mapping", "a: 1\n", Path{"a"}, "b", ErrKind, "a.b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			document := parse(t, tc.src)

			err := document.Upsert(tc.mapping, tc.key, "x", nil)

			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if !strings.Contains(err.Error(), tc.wantPath) {
				t.Errorf("error %q does not name the path %s", err, tc.wantPath)
			}
			if got := string(document.Bytes()); got != tc.src {
				t.Errorf("a refused upsert changed the source:\n%s", got)
			}
		})
	}
}

func TestDeleteIfPresentKeepsEveryOtherByte(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		path     Path
		old, new string
	}{
		{"block present", Path{"beats", "1", "hold"}, "    hold: 1s\n", ""},
		{"block absent", Path{"beats", "1", "why"}, "", ""},
		{"flow present", Path{"frame", "rows"}, "rows: 24, ", ""},
		{"flow absent", Path{"frame", "scale"}, "", ""},
		{"sequence item present first key", Path{"beats", "1", "id"}, "  - id: 2\n    title", "  - title"},
		{"sequence item absent", Path{"beats", "5", "hold"}, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := readCommented(t)
			want := src
			if tc.old != "" {
				want = swap(t, src, tc.old, tc.new)
			}
			document := parse(t, src)

			err := document.DeleteIfPresent(tc.path)

			if err != nil {
				t.Fatalf("DeleteIfPresent: %v", err)
			}
			if got := string(document.Bytes()); got != want {
				t.Errorf("edited source:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func TestDeleteIfPresentRefuses(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		path     Path
		wantPath string
	}{
		{"a step into a scalar", Path{"beats", "0", "duration", "x"}, "beats[0].duration.x"},
		{"a sequence item", Path{"beats", "0"}, "beats[0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := readCommented(t)
			document := parse(t, src)

			err := document.DeleteIfPresent(tc.path)

			if !errors.Is(err, ErrKind) {
				t.Fatalf("error = %v, want %v", err, ErrKind)
			}
			if !strings.Contains(err.Error(), tc.wantPath) {
				t.Errorf("error %q does not name the path %s", err, tc.wantPath)
			}
			if got := string(document.Bytes()); got != src {
				t.Errorf("a refused delete changed the source:\n%s", got)
			}
		})
	}
}

func TestScalarReadsTheDecodedValue(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		path      Path
		want      string
		isPresent bool
	}{
		{"block plain", Path{"beats", "0", "duration"}, "1800ms", true},
		{"flow plain", Path{"frame", "cols"}, "80", true},
		{"single-quoted with a doubled quote", Path{"beats", "0", "title"}, "it's open", true},
		{"double-quoted with an escape", Path{"beats", "1", "title"}, "Überblick ✓", true},
		{"quoted in nested flow", Path{"beats", "0", "zoom", "target", "text"}, "Größe → ✓", true},
		{"a mapping", Path{"frame"}, "", false},
		{"a sequence", Path{"beats"}, "", false},
		{"an absent key", Path{"beats", "1", "why"}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			document := parse(t, readCommented(t))

			got, isPresent := document.Scalar(tc.path)

			if got != tc.want || isPresent != tc.isPresent {
				t.Errorf("Scalar(%s) = %q, %t, want %q, %t", tc.path, got, isPresent, tc.want, tc.isPresent)
			}
		})
	}
}

func TestHasAnswersPresence(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		path Path
		want bool
	}{
		{"the root", nil, true},
		{"a sequence index in range", Path{"beats", "1"}, true},
		{"a sequence index out of range", Path{"beats", "2"}, false},
		{"a key inside a sequence item", Path{"beats", "1", "hold"}, true},
		{"a nested flow key", Path{"beats", "0", "zoom", "target", "area"}, true},
		{"an absent flow key", Path{"frame", "scale"}, false},
		{"a step into a scalar", Path{"beats", "0", "duration", "x"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			document := parse(t, readCommented(t))

			got := document.Has(tc.path)

			if got != tc.want {
				t.Errorf("Has(%s) = %t, want %t", tc.path, got, tc.want)
			}
		})
	}
}
