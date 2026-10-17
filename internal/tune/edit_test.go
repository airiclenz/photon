package tune

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/storyboard"
)

// boardPath is the storyboard every edit test starts from.
const boardPath = "testdata/board.yaml"

// readBoard returns the test storyboard's source.
func readBoard(t *testing.T) []byte {
	t.Helper()
	src, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatalf("read %s: %v", boardPath, err)
	}
	return src
}

func TestApply_Edit_ChangesOnlyTheEditedText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		edit     Edit
		old, new string
	}{
		{"duration in ms stays in ms", Edit{1, fieldDuration, "1850"},
			"duration: 1800ms # kept", "duration: 1850ms # kept"},
		{"duration in seconds stays in seconds", Edit{2, fieldDuration, "2350"},
			"    duration: 2s\n    hold", "    duration: 2.35s\n    hold"},
		{"whole seconds have no decimals", Edit{2, fieldDuration, "3000"},
			"    duration: 2s\n    hold", "    duration: 3s\n    hold"},
		{"another form is written as a Go duration", Edit{4, fieldDuration, "91500"},
			"duration: 1m30s", "duration: 1m31.5s"},
		{"hold is set", Edit{2, fieldHold, "250"}, "hold: 500ms", "hold: 250ms"},
		{"hold goes after the do list", Edit{5, fieldHold, "400"},
			"      - pause: {for: 1s}\n", "      - pause: {for: 1s}\n    hold: 400ms\n"},
		{"hold goes before the blank line after the beat", Edit{1, fieldHold, "400"},
			"timeout: 5s}\n", "timeout: 5s}\n    hold: 400ms\n"},
		{"cut on is inserted", Edit{5, fieldCut, valueTrue},
			"      - pause: {for: 1s}\n", "      - pause: {for: 1s}\n    cut: true\n"},
		{"duration is inserted into a cut beat", Edit{3, fieldDuration, "1200"},
			"      - key: {name: enter}\n", "      - key: {name: enter}\n    duration: 1.2s\n"},
		{"auto factor deletes a block key", Edit{2, fieldZoomFactor, valueAuto}, "      factor: 1.5\n", ""},
		{"auto factor deletes a flow key", Edit{4, fieldZoomFactor, valueAuto},
			"{target: {text: 'beta', nth: 2}, factor: 2}", "{target: {text: 'beta', nth: 2}}"},
		{"factor is set", Edit{2, fieldZoomFactor, "1.8"}, "factor: 1.5", "factor: 1.8"},
		{"factor goes after a block target", Edit{6, fieldZoomFactor, "2.5"},
			"        text: 'gamma'\n", "        text: 'gamma'\n      factor: 2.5\n"},
		{"target on a block zoom", Edit{2, fieldZoomTarget, "{text: 'hi', nth: 2}"},
			"target: {text: 'hello', area: status}", "target: {text: 'hi', nth: 2}"},
		{"target on a flow zoom", Edit{4, fieldZoomTarget, "{text: 'gamma', area: list}"},
			"{target: {text: 'beta', nth: 2}, factor: 2}", "{target: {text: 'gamma', area: list}, factor: 2}"},
		{"target on a block zoom holding only its target", Edit{6, fieldZoomTarget, "{text: 'delta'}"},
			"    zoom:\n      target:\n        text: 'gamma'\n", "    zoom: {target: {text: 'delta'}}\n"},
		{"zoom in goes after the factor", Edit{2, fieldZoomIn, "1500"},
			"      factor: 1.5\n", "      factor: 1.5\n      in: 1.5s\n"},
		{"zoom out goes last in a flow zoom", Edit{4, fieldZoomOut, "750"}, "factor: 2}", "factor: 2, out: 750ms}"},
		{"zoom is added as a flow mapping", Edit{5, fieldZoom, "{text: 'x', area: list}"},
			"      - pause: {for: 1s}\n", "      - pause: {for: 1s}\n    zoom: {target: {text: 'x', area: list}}\n"},
		{"a block zoom is removed", Edit{2, fieldZoom, ""},
			"    zoom:\n      target: {text: 'hello', area: status}\n      factor: 1.5\n", ""},
		{"a flow zoom is removed", Edit{4, fieldZoom, ""},
			"    zoom: {target: {text: 'beta', nth: 2}, factor: 2}\n", ""},
		{"frame width in a flow frame", Edit{0, fieldFrameWidth, "640"}, "width: 600", "width: 640"},
		{"frame fps in a flow frame", Edit{0, fieldFrameFPS, "30"}, "fps: 24", "fps: 30"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := readBoard(t)
			if count := strings.Count(string(src), tc.old); count != 1 {
				t.Fatalf("the fixture holds %q %d times, want once", tc.old, count)
			}
			want := strings.Replace(string(src), tc.old, tc.new, 1)

			got, _, err := apply(src, boardPath, tc.edit)

			if err != nil {
				t.Fatalf("apply(%+v): %v", tc.edit, err)
			}
			if string(got) != want {
				t.Errorf("apply(%+v) =\n%s\nwant\n%s", tc.edit, got, want)
			}
		})
	}
}

func TestApply_DefaultOrAbsentKey_LeavesTheSourceAsItIs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		edit Edit
	}{
		{"a zero hold is not written out", Edit{5, fieldHold, "0"}},
		{"cut off on a beat with no cut", Edit{5, fieldCut, valueFalse}},
		{"auto factor on a zoom with no factor", Edit{6, fieldZoomFactor, valueAuto}},
		{"a default ramp is not written out", Edit{2, fieldZoomIn, "1000"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := readBoard(t)

			got, _, err := apply(src, boardPath, tc.edit)

			if err != nil {
				t.Fatalf("apply(%+v): %v", tc.edit, err)
			}
			if string(got) != string(src) {
				t.Errorf("apply(%+v) changed the source:\n%s", tc.edit, got)
			}
		})
	}
}

func TestApply_Edit_ReturnsTheStoryboardTheBytesDecodeTo(t *testing.T) {
	t.Parallel()
	src := readBoard(t)

	_, board, err := apply(src, boardPath, Edit{Beat: 2, Field: fieldHold, Value: "250"})

	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got, want := board.Beats[1].Hold, 250*time.Millisecond; got != want {
		t.Errorf("beat 2 hold = %s, want %s", got, want)
	}
}

func TestApply_InvalidResult_IsRefusedWithTheValidationError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		edit Edit
	}{
		{"hold reaching the duration", Edit{1, fieldHold, "1800"}},
		{"cut off on a beat with no duration", Edit{3, fieldCut, valueFalse}},
		{"a factor beyond the range", Edit{2, fieldZoomFactor, "3.5"}},
		{"a target with no text", Edit{2, fieldZoomTarget, "{area: list}"}},
		{"a zero frame width", Edit{0, fieldFrameWidth, "0"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := readBoard(t)

			_, _, err := apply(src, boardPath, tc.edit)

			var invalid *storyboard.ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("apply(%+v) = %v, want a *storyboard.ValidationError", tc.edit, err)
			}
			if !strings.Contains(err.Error(), tc.edit.Field) {
				t.Errorf("apply(%+v) = %q, want it to name %s", tc.edit, err, tc.edit.Field)
			}
		})
	}
}

func TestApply_EditOnWhatIsNotThere_IsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		edit    Edit
		message string
	}{
		{"a zero ramp names the default", Edit{2, fieldZoomIn, "0"}, "zoom.in: want a ramp longer than 0ms; a ramp left out runs the 1s default"},
		{"a zero out ramp names the default", Edit{4, fieldZoomOut, "0"}, "zoom.out: want a ramp longer than 0ms; a ramp left out runs the 1s default"},
		{"a zoom field on a beat with no zoom", Edit{5, fieldZoomTarget, "{text: 'x'}"}, "beat 5: zoom.target: the beat has no zoom"},
		{"a second zoom", Edit{2, fieldZoom, "{text: 'x'}"}, "beat 2: zoom: the beat already zooms"},
		{"removing a zoom that is not there", Edit{5, fieldZoom, ""}, "beat 5: zoom: the beat has no zoom to remove"},
		{"an unknown beat", Edit{9, fieldDuration, "1000"}, "beat 9: duration: the storyboard has no beat with id 9"},
		{"an unknown field", Edit{1, "title", "x"}, "beat 1: title: unknown field"},
		{"a frame field on a beat", Edit{1, fieldFrameFPS, "30"}, "frame.fps: want beat 0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := readBoard(t)

			_, _, err := apply(src, boardPath, tc.edit)

			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Errorf("apply(%+v) = %v, want an error containing %q", tc.edit, err, tc.message)
			}
		})
	}
}

func TestApply_WireSpellings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		edit       Edit
		isAccepted bool
	}{
		{"milliseconds", Edit{2, fieldDuration, "1850"}, true},
		{"a duration with a unit", Edit{2, fieldDuration, "1.85s"}, false},
		{"a negative hold", Edit{2, fieldHold, "-5"}, false},
		{"a ramp with a unit", Edit{2, fieldZoomIn, "1s"}, false},
		{"a ramp as words", Edit{2, fieldZoomOut, "long"}, false},
		{"cut true", Edit{5, fieldCut, "true"}, true},
		{"cut false", Edit{2, fieldCut, "false"}, true},
		{"cut as yes", Edit{2, fieldCut, "yes"}, false},
		{"a decimal factor", Edit{2, fieldZoomFactor, "1.8"}, true},
		{"a whole factor", Edit{2, fieldZoomFactor, "2"}, true},
		{"auto factor", Edit{2, fieldZoomFactor, "auto"}, true},
		{"factor 0 for auto", Edit{2, fieldZoomFactor, "0"}, false},
		{"a factor as words", Edit{2, fieldZoomFactor, "fast"}, false},
		{"a frame integer", Edit{0, fieldFrameWidth, "640"}, true},
		{"a frame decimal", Edit{0, fieldFrameWidth, "6.5"}, false},
		{"an empty frame value", Edit{0, fieldFrameFPS, ""}, false},
		{"a target flow mapping", Edit{2, fieldZoomTarget, "{text: 'ok', area: list, nth: 2}"}, true},
		{"a target with nth last", Edit{2, fieldZoomTarget, "{text: 'ok', nth: last}"}, true},
		{"a target as a block mapping", Edit{2, fieldZoomTarget, "text: ok"}, false},
		{"a target with an unknown key", Edit{2, fieldZoomTarget, "{text: ok, bogus: 1}"}, false},
		{"a target with a comment", Edit{2, fieldZoomTarget, "{text: ok} # note }"}, false},
		{"a zoom added onto a target", Edit{5, fieldZoom, "{text: 'ok', area: list}"}, true},
		{"a zoom added onto a sequence", Edit{5, fieldZoom, "[ok]"}, false},
		{"a zoom removed", Edit{2, fieldZoom, ""}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := readBoard(t)

			_, _, err := apply(src, boardPath, tc.edit)

			if tc.isAccepted && err != nil {
				t.Fatalf("apply(%+v): %v", tc.edit, err)
			}
			if !tc.isAccepted && (err == nil || !strings.Contains(err.Error(), tc.edit.Field+": want")) {
				t.Errorf("apply(%+v) = %v, want a spelling error naming %s", tc.edit, err, tc.edit.Field)
			}
		})
	}
}
