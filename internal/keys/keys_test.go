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

func TestMotionEncodesAHeldButtonSGRReportAtTheCell(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		x, y int
		want string
	}{
		{name: "top-left cell", x: 0, y: 0, want: "\x1b[<32;1;1M"},
		{name: "column 9 row 4", x: 9, y: 4, want: "\x1b[<32;10;5M"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := Motion(tc.x, tc.y)

			if string(got) != tc.want {
				t.Errorf("Motion(%d, %d) = %q, want %q", tc.x, tc.y, got, tc.want)
			}
		})
	}
}

func TestMouseReportsEncodeEveryButtonAndModifier(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		mouse       Mouse
		wantPress   string
		wantMotion  string
		wantRelease string
	}{
		{name: "left", mouse: Mouse{},
			wantPress: "\x1b[<0;3;2M", wantMotion: "\x1b[<32;3;2M", wantRelease: "\x1b[<0;3;2m"},
		{name: "middle", mouse: Mouse{Button: MiddleButton},
			wantPress: "\x1b[<1;3;2M", wantMotion: "\x1b[<33;3;2M", wantRelease: "\x1b[<1;3;2m"},
		{name: "right", mouse: Mouse{Button: RightButton},
			wantPress: "\x1b[<2;3;2M", wantMotion: "\x1b[<34;3;2M", wantRelease: "\x1b[<2;3;2m"},
		{name: "left+shift", mouse: Mouse{Mods: Shift},
			wantPress: "\x1b[<4;3;2M", wantMotion: "\x1b[<36;3;2M", wantRelease: "\x1b[<4;3;2m"},
		{name: "left+alt", mouse: Mouse{Mods: Alt},
			wantPress: "\x1b[<8;3;2M", wantMotion: "\x1b[<40;3;2M", wantRelease: "\x1b[<8;3;2m"},
		{name: "left+ctrl", mouse: Mouse{Mods: Ctrl},
			wantPress: "\x1b[<16;3;2M", wantMotion: "\x1b[<48;3;2M", wantRelease: "\x1b[<16;3;2m"},
		{name: "middle+shift+alt", mouse: Mouse{Button: MiddleButton, Mods: Shift | Alt},
			wantPress: "\x1b[<13;3;2M", wantMotion: "\x1b[<45;3;2M", wantRelease: "\x1b[<13;3;2m"},
		{name: "right+ctrl+shift", mouse: Mouse{Button: RightButton, Mods: Ctrl | Shift},
			wantPress: "\x1b[<22;3;2M", wantMotion: "\x1b[<54;3;2M", wantRelease: "\x1b[<22;3;2m"},
		{name: "right+shift+alt+ctrl", mouse: Mouse{Button: RightButton, Mods: Shift | Alt | Ctrl},
			wantPress: "\x1b[<30;3;2M", wantMotion: "\x1b[<62;3;2M", wantRelease: "\x1b[<30;3;2m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			press := MousePress(2, 1, tc.mouse)
			motion := MouseMotion(2, 1, tc.mouse)
			release := MouseRelease(2, 1, tc.mouse)

			if string(press) != tc.wantPress {
				t.Errorf("MousePress(2, 1, %+v) = %q, want %q", tc.mouse, press, tc.wantPress)
			}
			if string(motion) != tc.wantMotion {
				t.Errorf("MouseMotion(2, 1, %+v) = %q, want %q", tc.mouse, motion, tc.wantMotion)
			}
			if string(release) != tc.wantRelease {
				t.Errorf("MouseRelease(2, 1, %+v) = %q, want %q", tc.mouse, release, tc.wantRelease)
			}
		})
	}
}

func TestClickMotionAndReleaseMatchTheZeroMouse(t *testing.T) {
	t.Parallel()

	if got, want := Click(4, 7), MousePress(4, 7, Mouse{}); !bytes.Equal(got, want) {
		t.Errorf("Click(4, 7) = %q, want MousePress with the zero Mouse %q", got, want)
	}
	if got, want := Motion(4, 7), MouseMotion(4, 7, Mouse{}); !bytes.Equal(got, want) {
		t.Errorf("Motion(4, 7) = %q, want MouseMotion with the zero Mouse %q", got, want)
	}
	if got, want := Release(4, 7), MouseRelease(4, 7, Mouse{}); !bytes.Equal(got, want) {
		t.Errorf("Release(4, 7) = %q, want MouseRelease with the zero Mouse %q", got, want)
	}
}

func TestParseButtonReadsTheStoryboardSpelling(t *testing.T) {
	t.Parallel()

	cases := []struct {
		spelling string
		want     Button
	}{
		{spelling: "", want: LeftButton},
		{spelling: "left", want: LeftButton},
		{spelling: "middle", want: MiddleButton},
		{spelling: "right", want: RightButton},
	}
	for _, tc := range cases {
		t.Run(tc.spelling, func(t *testing.T) {
			t.Parallel()

			got, err := ParseButton(tc.spelling)

			if err != nil {
				t.Fatalf("ParseButton(%q) error = %v, want nil", tc.spelling, err)
			}
			if got != tc.want {
				t.Errorf("ParseButton(%q) = %d, want %d", tc.spelling, got, tc.want)
			}
		})
	}
}

func TestParseButtonRefusesAnUnknownName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		spelling string
		wantErr  string
	}{
		{spelling: "Left", wantErr: `unknown button "Left", want one of left, middle, right`},
		{spelling: "wheel", wantErr: `unknown button "wheel", want one of left, middle, right`},
		{spelling: " right", wantErr: `unknown button " right", want one of left, middle, right`},
	}
	for _, tc := range cases {
		t.Run(tc.spelling, func(t *testing.T) {
			t.Parallel()

			_, err := ParseButton(tc.spelling)

			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("ParseButton(%q) error = %v, want %q", tc.spelling, err, tc.wantErr)
			}
		})
	}
}

func TestParseModsReadsTheStoryboardSpelling(t *testing.T) {
	t.Parallel()

	cases := []struct {
		spelling string
		want     Mods
	}{
		{spelling: "", want: 0},
		{spelling: "shift", want: Shift},
		{spelling: "alt", want: Alt},
		{spelling: "ctrl", want: Ctrl},
		{spelling: "ctrl+shift", want: Ctrl | Shift},
		{spelling: "shift+ctrl", want: Ctrl | Shift},
		{spelling: "alt+ctrl+shift", want: Shift | Alt | Ctrl},
	}
	for _, tc := range cases {
		t.Run(tc.spelling, func(t *testing.T) {
			t.Parallel()

			got, err := ParseMods(tc.spelling)

			if err != nil {
				t.Fatalf("ParseMods(%q) error = %v, want nil", tc.spelling, err)
			}
			if got != tc.want {
				t.Errorf("ParseMods(%q) = %d, want %d", tc.spelling, got, tc.want)
			}
		})
	}
}

func TestParseModsRefusesAnUnknownOrRepeatedName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		spelling string
		wantErr  string
	}{
		{spelling: "meta", wantErr: `unknown modifier "meta", want one of shift, alt, ctrl`},
		{spelling: "Ctrl", wantErr: `unknown modifier "Ctrl", want one of shift, alt, ctrl`},
		{spelling: "ctrl+", wantErr: `unknown modifier "", want one of shift, alt, ctrl`},
		{spelling: "ctrl-shift", wantErr: `unknown modifier "ctrl-shift", want one of shift, alt, ctrl`},
		{spelling: "shift+shift", wantErr: `modifier "shift" given twice`},
		{spelling: "ctrl+alt+ctrl", wantErr: `modifier "ctrl" given twice`},
	}
	for _, tc := range cases {
		t.Run(tc.spelling, func(t *testing.T) {
			t.Parallel()

			_, err := ParseMods(tc.spelling)

			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("ParseMods(%q) error = %v, want %q", tc.spelling, err, tc.wantErr)
			}
		})
	}
}

func TestEmptySpellingsParseToTheZeroMouse(t *testing.T) {
	t.Parallel()

	button, buttonErr := ParseButton("")
	mods, modsErr := ParseMods("")

	if buttonErr != nil || modsErr != nil {
		t.Fatalf("ParseButton(\"\"), ParseMods(\"\") errors = %v, %v; want nil, nil", buttonErr, modsErr)
	}
	if got := (Mouse{Button: button, Mods: mods}); got != (Mouse{}) {
		t.Errorf("Mouse from the empty spellings = %+v, want the zero Mouse", got)
	}
}
