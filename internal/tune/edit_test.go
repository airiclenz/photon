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
		{"duration in ms stays in ms", Edit{Beat: 1, Field: fieldDuration, Value: "1850"},
			"duration: 1800ms # kept", "duration: 1850ms # kept"},
		{"duration in seconds stays in seconds", Edit{Beat: 2, Field: fieldDuration, Value: "2350"},
			"    duration: 2s\n    hold", "    duration: 2.35s\n    hold"},
		{"whole seconds have no decimals", Edit{Beat: 2, Field: fieldDuration, Value: "3000"},
			"    duration: 2s\n    hold", "    duration: 3s\n    hold"},
		{"another form is written as a Go duration", Edit{Beat: 4, Field: fieldDuration, Value: "91500"},
			"duration: 1m30s", "duration: 1m31.5s"},
		{"hold is set", Edit{Beat: 2, Field: fieldHold, Value: "250"}, "hold: 500ms", "hold: 250ms"},
		{"hold goes after the do list", Edit{Beat: 5, Field: fieldHold, Value: "400"},
			"      - pause: {for: 1s}\n", "      - pause: {for: 1s}\n    hold: 400ms\n"},
		{"hold goes before the blank line after the beat", Edit{Beat: 1, Field: fieldHold, Value: "400"},
			"timeout: 5s}\n", "timeout: 5s}\n    hold: 400ms\n"},
		{"cut on is inserted", Edit{Beat: 5, Field: fieldCut, Value: valueTrue},
			"      - pause: {for: 1s}\n", "      - pause: {for: 1s}\n    cut: true\n"},
		{"duration is inserted into a cut beat", Edit{Beat: 3, Field: fieldDuration, Value: "1200"},
			"      - key: {name: enter}\n", "      - key: {name: enter}\n    duration: 1.2s\n"},
		{"auto factor deletes a block key", Edit{Beat: 2, Field: fieldZoomFactor, Value: valueAuto}, "      factor: 1.5\n", ""},
		{"auto factor deletes a flow key", Edit{Beat: 4, Field: fieldZoomFactor, Value: valueAuto},
			"{target: {text: 'beta', nth: 2}, factor: 2}", "{target: {text: 'beta', nth: 2}}"},
		{"factor is set", Edit{Beat: 2, Field: fieldZoomFactor, Value: "1.8"}, "factor: 1.5", "factor: 1.8"},
		{"factor goes after a block target", Edit{Beat: 6, Field: fieldZoomFactor, Value: "2.5"},
			"        text: 'gamma'\n", "        text: 'gamma'\n      factor: 2.5\n"},
		{"target on a block zoom", Edit{Beat: 2, Field: fieldZoomTarget, Value: "{text: 'hi', nth: 2}"},
			"target: {text: 'hello', area: status}", "target: {text: 'hi', nth: 2}"},
		{"target on a flow zoom", Edit{Beat: 4, Field: fieldZoomTarget, Value: "{text: 'gamma', area: list}"},
			"{target: {text: 'beta', nth: 2}, factor: 2}", "{target: {text: 'gamma', area: list}, factor: 2}"},
		{"target on a block zoom holding only its target", Edit{Beat: 6, Field: fieldZoomTarget, Value: "{text: 'delta'}"},
			"    zoom:\n      target:\n        text: 'gamma'\n", "    zoom: {target: {text: 'delta'}}\n"},
		{"zoom in goes after the factor", Edit{Beat: 2, Field: fieldZoomIn, Value: "1500"},
			"      factor: 1.5\n", "      factor: 1.5\n      in: 1.5s\n"},
		{"zoom out goes last in a flow zoom", Edit{Beat: 4, Field: fieldZoomOut, Value: "750"}, "factor: 2}", "factor: 2, out: 750ms}"},
		{"zoom is added as a flow mapping", Edit{Beat: 5, Field: fieldZoom, Value: "{text: 'x', area: list}"},
			"      - pause: {for: 1s}\n", "      - pause: {for: 1s}\n    zoom: {target: {text: 'x', area: list}}\n"},
		{"a block zoom is removed", Edit{Beat: 2, Field: fieldZoom, Value: ""},
			"    zoom:\n      target: {text: 'hello', area: status}\n      factor: 1.5\n", ""},
		{"a flow zoom is removed", Edit{Beat: 4, Field: fieldZoom, Value: ""},
			"    zoom: {target: {text: 'beta', nth: 2}, factor: 2}\n", ""},
		{"frame width in a flow frame", Edit{Beat: 0, Field: fieldFrameWidth, Value: "640"}, "width: 600", "width: 640"},
		{"frame fps in a flow frame", Edit{Beat: 0, Field: fieldFrameFPS, Value: "30"}, "fps: 24", "fps: 30"},
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
		{"a zero hold is not written out", Edit{Beat: 5, Field: fieldHold, Value: "0"}},
		{"cut off on a beat with no cut", Edit{Beat: 5, Field: fieldCut, Value: valueFalse}},
		{"auto factor on a zoom with no factor", Edit{Beat: 6, Field: fieldZoomFactor, Value: valueAuto}},
		{"a default ramp is not written out", Edit{Beat: 2, Field: fieldZoomIn, Value: "1000"}},
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
		{"hold reaching the duration", Edit{Beat: 1, Field: fieldHold, Value: "1800"}},
		{"cut off on a beat with no duration", Edit{Beat: 3, Field: fieldCut, Value: valueFalse}},
		{"a factor beyond the range", Edit{Beat: 2, Field: fieldZoomFactor, Value: "3.5"}},
		{"a target with no text", Edit{Beat: 2, Field: fieldZoomTarget, Value: "{area: list}"}},
		{"a zero frame width", Edit{Beat: 0, Field: fieldFrameWidth, Value: "0"}},
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
		{"a zero ramp names the default", Edit{Beat: 2, Field: fieldZoomIn, Value: "0"}, "zoom.in: want a ramp longer than 0ms; a ramp left out runs the 1s default"},
		{"a zero out ramp names the default", Edit{Beat: 4, Field: fieldZoomOut, Value: "0"}, "zoom.out: want a ramp longer than 0ms; a ramp left out runs the 1s default"},
		{"a zoom field on a beat with no zoom", Edit{Beat: 5, Field: fieldZoomTarget, Value: "{text: 'x'}"}, "beat 5: zoom.target: the beat has no zoom"},
		{"a second zoom", Edit{Beat: 2, Field: fieldZoom, Value: "{text: 'x'}"}, "beat 2: zoom: the beat already zooms"},
		{"removing a zoom that is not there", Edit{Beat: 5, Field: fieldZoom, Value: ""}, "beat 5: zoom: the beat has no zoom to remove"},
		{"an unknown beat", Edit{Beat: 9, Field: fieldDuration, Value: "1000"}, "beat 9: duration: the storyboard has no beat with id 9"},
		{"an unknown field", Edit{Beat: 1, Field: "title", Value: "x"}, "beat 1: title: unknown field"},
		{"a frame field on a beat", Edit{Beat: 1, Field: fieldFrameFPS, Value: "30"}, "frame.fps: want beat 0"},
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
		{"milliseconds", Edit{Beat: 2, Field: fieldDuration, Value: "1850"}, true},
		{"a duration with a unit", Edit{Beat: 2, Field: fieldDuration, Value: "1.85s"}, false},
		{"a negative hold", Edit{Beat: 2, Field: fieldHold, Value: "-5"}, false},
		{"a ramp with a unit", Edit{Beat: 2, Field: fieldZoomIn, Value: "1s"}, false},
		{"a ramp as words", Edit{Beat: 2, Field: fieldZoomOut, Value: "long"}, false},
		{"cut true", Edit{Beat: 5, Field: fieldCut, Value: "true"}, true},
		{"cut false", Edit{Beat: 2, Field: fieldCut, Value: "false"}, true},
		{"cut as yes", Edit{Beat: 2, Field: fieldCut, Value: "yes"}, false},
		{"a decimal factor", Edit{Beat: 2, Field: fieldZoomFactor, Value: "1.8"}, true},
		{"a whole factor", Edit{Beat: 2, Field: fieldZoomFactor, Value: "2"}, true},
		{"auto factor", Edit{Beat: 2, Field: fieldZoomFactor, Value: "auto"}, true},
		{"factor 0 for auto", Edit{Beat: 2, Field: fieldZoomFactor, Value: "0"}, false},
		{"a factor as words", Edit{Beat: 2, Field: fieldZoomFactor, Value: "fast"}, false},
		{"a frame integer", Edit{Beat: 0, Field: fieldFrameWidth, Value: "640"}, true},
		{"a frame decimal", Edit{Beat: 0, Field: fieldFrameWidth, Value: "6.5"}, false},
		{"an empty frame value", Edit{Beat: 0, Field: fieldFrameFPS, Value: ""}, false},
		{"a target flow mapping", Edit{Beat: 2, Field: fieldZoomTarget, Value: "{text: 'ok', area: list, nth: 2}"}, true},
		{"a target with nth last", Edit{Beat: 2, Field: fieldZoomTarget, Value: "{text: 'ok', nth: last}"}, true},
		{"a target as a block mapping", Edit{Beat: 2, Field: fieldZoomTarget, Value: "text: ok"}, false},
		{"a target with an unknown key", Edit{Beat: 2, Field: fieldZoomTarget, Value: "{text: ok, bogus: 1}"}, false},
		{"a target with a comment", Edit{Beat: 2, Field: fieldZoomTarget, Value: "{text: ok} # note }"}, false},
		{"a zoom added onto a target", Edit{Beat: 5, Field: fieldZoom, Value: "{text: 'ok', area: list}"}, true},
		{"a zoom added onto a sequence", Edit{Beat: 5, Field: fieldZoom, Value: "[ok]"}, false},
		{"a zoom removed", Edit{Beat: 2, Field: fieldZoom, Value: ""}, true},
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

// cameraEditPath is the name the camera edit tests parse their storyboard under.
const cameraEditPath = "camera.yaml"

// cameraEditBoard is sessionStoryboard with a camera of block and flow moves, all inside beat 1:
// zooms over [100ms, 400ms), [500ms, 900ms) and [0s, 50ms), pans over [200ms, 800ms) and
// [900ms, 1s).
const cameraEditBoard = sessionStoryboard + `camera:
  - zoom:
      to: 1.5
      at:
        beat: 1
        offset: 100ms
      over: 300ms
      ease: linear
  - pan:
      to:
        text: 'failing'
      at: 200ms # after the zoom starts
      over: 600ms
  - pan: {to: {cell: {x: 3, y: 2}}, at: 900ms, over: 100ms}
  - zoom: {to: 2, at: {beat: 1, offset: 500ms}, over: 400ms, ease: linear}
  - zoom: {to: 1.2, over: 50ms}
`

// cameraEdit is an Edit of the camera move at index.
func cameraEdit(index int, field, value string) Edit {
	return Edit{Index: index, Field: field, Value: value}
}

func TestApply_CameraEdit_ChangesOnlyTheEditedText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		edit     Edit
		old, new string
	}{
		{"a block beat at becomes a clip time", cameraEdit(0, fieldCameraAt, "150ms"),
			"at:\n        beat: 1\n        offset: 100ms\n", "at: 150ms\n"},
		{"a block clip time becomes a beat moment", cameraEdit(1, fieldCameraAt, "{beat: 1, offset: 300ms}"),
			"at: 200ms # after the zoom starts", "at: {beat: 1, offset: 300ms} # after the zoom starts"},
		{"a flow beat at becomes a clip time", cameraEdit(3, fieldCameraAt, "600ms"),
			"at: {beat: 1, offset: 500ms}", "at: 600ms"},
		{"a flow clip time becomes a beat moment", cameraEdit(2, fieldCameraAt, " {beat: 1, offset: 850ms} "),
			"at: 900ms", "at: {beat: 1, offset: 850ms}"},
		{"an absent at is inserted after to", cameraEdit(4, fieldCameraAt, "50ms"),
			"{to: 1.2, over: 50ms}", "{to: 1.2, at: 50ms, over: 50ms}"},
		{"a block over keeps its unit", cameraEdit(0, fieldCameraOver, "250"), "over: 300ms", "over: 250ms"},
		{"a flow over keeps its unit", cameraEdit(3, fieldCameraOver, "350"), "over: 400ms", "over: 350ms"},
		{"smooth deletes a block ease", cameraEdit(0, fieldCameraEase, "smooth"), "      ease: linear\n", ""},
		{"smooth deletes a flow ease", cameraEdit(3, fieldCameraEase, "smooth"), ", ease: linear}", "}"},
		{"linear goes after a block over", cameraEdit(1, fieldCameraEase, "linear"),
			"      over: 600ms\n", "      over: 600ms\n      ease: linear\n"},
		{"linear goes last in a flow move", cameraEdit(2, fieldCameraEase, "linear"),
			"over: 100ms}", "over: 100ms, ease: linear}"},
		{"a block zoom's factor", cameraEdit(0, fieldCameraTo, "2.5"), "to: 1.5", "to: 2.5"},
		{"a flow zoom's factor", cameraEdit(3, fieldCameraTo, "1.25"), "to: 2,", "to: 1.25,"},
		{"a block pan target becomes a cell", cameraEdit(1, fieldCameraTo, "{cell: {x: 1, y: 1}}"),
			"to:\n        text: 'failing'\n", "to: {cell: {x: 1, y: 1}}\n"},
		{"a flow pan cell becomes a target", cameraEdit(2, fieldCameraTo, "{text: 'tests', nth: last}"),
			"to: {cell: {x: 3, y: 2}}", "to: {text: 'tests', nth: last}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if count := strings.Count(cameraEditBoard, tc.old); count != 1 {
				t.Fatalf("the board holds %q %d times, want once", tc.old, count)
			}
			want := strings.Replace(cameraEditBoard, tc.old, tc.new, 1)

			got, _, err := apply([]byte(cameraEditBoard), cameraEditPath, tc.edit)

			if err != nil {
				t.Fatalf("apply(%+v): %v", tc.edit, err)
			}
			if string(got) != want {
				t.Errorf("apply(%+v) =\n%s\nwant\n%s", tc.edit, got, want)
			}
		})
	}
}

func TestApply_CameraEditToWhatAnAbsentKeyMeans_LeavesTheSourceAsItIs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		edit Edit
	}{
		{"a clip start on a move without at", cameraEdit(4, fieldCameraAt, "0ms")},
		{"smooth on a move without ease", cameraEdit(1, fieldCameraEase, "smooth")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, _, err := apply([]byte(cameraEditBoard), cameraEditPath, tc.edit)

			if err != nil {
				t.Fatalf("apply(%+v): %v", tc.edit, err)
			}
			if string(got) != cameraEditBoard {
				t.Errorf("apply(%+v) changed the source:\n%s", tc.edit, got)
			}
		})
	}
}

func TestApply_CameraEditMakingZoomsOverlap_IsRefusedNamingBoth(t *testing.T) {
	t.Parallel()

	_, _, err := apply([]byte(cameraEditBoard), cameraEditPath, cameraEdit(0, fieldCameraAt, "450ms"))

	var invalid *storyboard.ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("apply = %v, want a *storyboard.ValidationError", err)
	}
	for _, want := range []string{"camera[0]: camera.at: ", "camera[3]: zoom: overlaps camera[0] on the zoom track"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("apply = %q, want it to contain %q", err, want)
		}
	}
}

func TestApply_CameraEditOnWhatIsNotThere_IsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		board   string
		edit    Edit
		message string
	}{
		{"an index past the last move", cameraEditBoard, cameraEdit(9, fieldCameraOver, "100"),
			"camera[9]: camera.over: the storyboard has no camera move at index 9"},
		{"a negative index", cameraEditBoard, cameraEdit(-1, fieldCameraOver, "100"),
			"camera[-1]: camera.over: the storyboard has no camera move at index -1"},
		{"a storyboard without a camera", sessionStoryboard, cameraEdit(0, fieldCameraEase, "linear"),
			"camera[0]: camera.ease: the storyboard has no camera move at index 0"},
		{"the destination of a via pan", cameraCaptionsStoryboard, cameraEdit(1, fieldCameraTo, "{text: 'x'}"),
			"camera[1]: camera.to: the pan follows a via path"},
		{"an unknown camera field", cameraEditBoard, cameraEdit(0, "camera.via", "x"),
			"camera[0]: camera.via: unknown field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := apply([]byte(tc.board), cameraEditPath, tc.edit)

			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Errorf("apply(%+v) = %v, want an error containing %q", tc.edit, err, tc.message)
			}
		})
	}
}

func TestApply_CameraWireSpellings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		edit       Edit
		isAccepted bool
	}{
		{"a beat moment", cameraEdit(1, fieldCameraAt, "{beat: 1, offset: 300ms}"), true},
		{"a beat moment without an offset", cameraEdit(1, fieldCameraAt, "{beat: 1}"), true},
		{"a clip time", cameraEdit(1, fieldCameraAt, "150ms"), true},
		{"a quoted clip time", cameraEdit(1, fieldCameraAt, "'150ms'"), true},
		{"a moment as a block mapping", cameraEdit(1, fieldCameraAt, "beat: 1"), false},
		{"a moment with an unknown key", cameraEdit(1, fieldCameraAt, "{beat: 1, bogus: 2}"), false},
		{"a moment at beat 0", cameraEdit(1, fieldCameraAt, "{beat: 0}"), false},
		{"a moment as words", cameraEdit(1, fieldCameraAt, "soon"), false},
		{"a moment with a comment", cameraEdit(1, fieldCameraAt, "150ms # soon"), false},
		{"a moment as a sequence", cameraEdit(1, fieldCameraAt, "[150ms]"), false},
		{"a moment over two lines", cameraEdit(1, fieldCameraAt, "{beat: 1,\noffset: 0s}"), false},
		{"milliseconds of over", cameraEdit(0, fieldCameraOver, "250"), true},
		{"an over with a unit", cameraEdit(0, fieldCameraOver, "250ms"), false},
		{"linear", cameraEdit(1, fieldCameraEase, "linear"), true},
		{"an ease as another word", cameraEdit(1, fieldCameraEase, "fast"), false},
		{"an empty ease", cameraEdit(1, fieldCameraEase, ""), false},
		{"a zoom factor", cameraEdit(0, fieldCameraTo, "2.5"), true},
		{"a zoom factor as words", cameraEdit(0, fieldCameraTo, "big"), false},
		{"a zoom factor of auto", cameraEdit(0, fieldCameraTo, "auto"), false},
		{"a pan target", cameraEdit(2, fieldCameraTo, "{text: 'tests'}"), true},
		{"a pan cell", cameraEdit(1, fieldCameraTo, "{cell: {x: 1, y: 1}}"), true},
		{"a pan cell with an unknown key", cameraEdit(1, fieldCameraTo, "{cell: {x: 1, z: 1}}"), false},
		{"a pan point as a sequence", cameraEdit(1, fieldCameraTo, "[1, 2]"), false},
		{"a pan point as a block mapping", cameraEdit(1, fieldCameraTo, "text: x"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := apply([]byte(cameraEditBoard), cameraEditPath, tc.edit)

			if tc.isAccepted && err != nil {
				t.Fatalf("apply(%+v): %v", tc.edit, err)
			}
			if !tc.isAccepted && (err == nil || !strings.Contains(err.Error(), tc.edit.Field+": want")) {
				t.Errorf("apply(%+v) = %v, want a spelling error naming %s", tc.edit, err, tc.edit.Field)
			}
		})
	}
}
