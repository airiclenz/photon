package testkit

import (
	"os"
	"testing"
)

func TestBuildFixtureTUI_PathOutlivesTheTestThatBuiltIt(t *testing.T) {
	var built string
	t.Run("first build", func(t *testing.T) {
		built = BuildFixtureTUI(t)
	})

	t.Run("later call after the first one's cleanup", func(t *testing.T) {
		again := BuildFixtureTUI(t)

		if again != built {
			t.Fatalf("BuildFixtureTUI = %q, want the first call's %q", again, built)
		}
		info, err := os.Stat(again)
		if err != nil {
			t.Fatalf("stat the fixture: %v", err)
		}
		if info.IsDir() {
			t.Fatalf("%s is a directory, want the executable", again)
		}
	})
}

func TestExecutableName_AddsTheWindowsSuffix(t *testing.T) {
	t.Parallel()
	cases := []struct {
		goos string
		want string
	}{
		{goos: "linux", want: "fixturetui"},
		{goos: "darwin", want: "fixturetui"},
		{goos: "windows", want: "fixturetui.exe"},
	}

	for _, tc := range cases {
		t.Run(tc.goos, func(t *testing.T) {
			t.Parallel()

			if got := executableName("fixturetui", tc.goos); got != tc.want {
				t.Errorf("executableName(%q) = %q, want %q", tc.goos, got, tc.want)
			}
		})
	}
}
