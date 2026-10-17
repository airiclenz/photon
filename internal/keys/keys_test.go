package keys

import (
	"bytes"
	"slices"
	"testing"
)

// wantSequences pins the exact bytes of every key name photon accepts.
var wantSequences = map[string]string{
	"enter":     "\r",
	"esc":       "\x1b",
	"tab":       "\t",
	"shift+tab": "\x1b[Z",
	"space":     " ",
	"backspace": "\x7f",
	"ctrl+c":    "\x03",
	"up":        "\x1b[A",
	"down":      "\x1b[B",
	"left":      "\x1b[D",
	"right":     "\x1b[C",
	"pgup":      "\x1b[5~",
	"pgdown":    "\x1b[6~",
	"alt+up":    "\x1b[1;3A",
	"alt+down":  "\x1b[1;3B",
	"f1":        "\x1bOP",
	"f2":        "\x1bOQ",
	"f3":        "\x1bOR",
	"f4":        "\x1bOS",
	"f5":        "\x1b[15~",
	"f6":        "\x1b[17~",
	"f7":        "\x1b[18~",
	"f8":        "\x1b[19~",
	"f9":        "\x1b[20~",
	"f10":       "\x1b[21~",
	"f11":       "\x1b[23~",
	"f12":       "\x1b[24~",
}

func TestLookupReturnsEveryNamesBytes(t *testing.T) {
	t.Parallel()

	for name, want := range wantSequences {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, found := Lookup(name)

			if !found {
				t.Fatalf("Lookup(%q) reports not found", name)
			}
			if !bytes.Equal(got, []byte(want)) {
				t.Errorf("Lookup(%q) = %q, want %q", name, got, want)
			}
		})
	}
}

func TestLookupReportsAnUnknownNameAsNotFound(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"", "Enter", "ctrl+x", "f13", "shift-tab", " enter"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, found := Lookup(name)

			if found || got != nil {
				t.Errorf("Lookup(%q) = %q, %v; want nil, false", name, got, found)
			}
		})
	}
}

func TestLookupReturnsTheCallersOwnSlice(t *testing.T) {
	t.Parallel()

	first, _ := Lookup("up")
	first[0] = 'X'

	second, _ := Lookup("up")

	if !bytes.Equal(second, []byte(wantSequences["up"])) {
		t.Errorf("Lookup(\"up\") after a caller wrote to an earlier result = %q, want %q",
			second, wantSequences["up"])
	}
}

func TestNamesListsExactlyTheAcceptedNames(t *testing.T) {
	t.Parallel()

	names := Names()

	want := make([]string, 0, len(wantSequences))
	for name := range wantSequences {
		want = append(want, name)
	}
	got := slices.Clone(names)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("Names() = %q, want the names %q", names, want)
	}
}

func TestClickAndReleaseEncodeAnSGRReportAtTheCell(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		x, y        int
		wantPress   string
		wantRelease string
	}{
		{name: "top-left cell", x: 0, y: 0, wantPress: "\x1b[<0;1;1M", wantRelease: "\x1b[<0;1;1m"},
		{name: "bottom-right of 80x24", x: 79, y: 23, wantPress: "\x1b[<0;80;24M", wantRelease: "\x1b[<0;80;24m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			press := Click(tc.x, tc.y)
			release := Release(tc.x, tc.y)

			if string(press) != tc.wantPress {
				t.Errorf("Click(%d, %d) = %q, want %q", tc.x, tc.y, press, tc.wantPress)
			}
			if string(release) != tc.wantRelease {
				t.Errorf("Release(%d, %d) = %q, want %q", tc.x, tc.y, release, tc.wantRelease)
			}
		})
	}
}
