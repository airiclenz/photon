package storyboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
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

// typedValue is value as an Edit's JSON value; nil is the JSON null.
func typedValue(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("encode the test value %v: %v", value, err))
	}
	return encoded
}

// beatEdit is an Edit of field on the beat with id, its value typed as value encodes.
func beatEdit(id int, field string, value any) Edit {
	return Edit{Owner: Owner{Kind: OwnerBeat, Index: id}, Field: field, Value: typedValue(value)}
}

// frameEdit is an Edit of the clip's frame field, its value typed as value encodes.
func frameEdit(field string, value any) Edit {
	return Edit{Owner: Owner{Kind: OwnerClip}, Field: field, Value: typedValue(value)}
}

func TestApply_Edit_ChangesOnlyTheEditedText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		edit     Edit
		old, new string
	}{
		{"duration in ms stays in ms", beatEdit(1, FieldDuration, 1850),
			"duration: 1800ms # kept", "duration: 1850ms # kept"},
		{"duration in seconds stays in seconds", beatEdit(2, FieldDuration, 2350),
			"    duration: 2s\n    hold", "    duration: 2.35s\n    hold"},
		{"whole seconds have no decimals", beatEdit(2, FieldDuration, 3000),
			"    duration: 2s\n    hold", "    duration: 3s\n    hold"},
		{"another form is written as a Go duration", beatEdit(4, FieldDuration, 91500),
			"duration: 1m30s", "duration: 1m31.5s"},
		{"hold is set", beatEdit(2, FieldHold, 250), "hold: 500ms", "hold: 250ms"},
		{"hold goes after the do list", beatEdit(5, FieldHold, 400),
			"      - pause: {for: 1s}\n", "      - pause: {for: 1s}\n    hold: 400ms\n"},
		{"hold goes before the blank line after the beat", beatEdit(1, FieldHold, 400),
			"timeout: 5s}\n", "timeout: 5s}\n    hold: 400ms\n"},
		{"cut on is inserted", beatEdit(5, FieldCut, true),
			"      - pause: {for: 1s}\n", "      - pause: {for: 1s}\n    cut: true\n"},
		{"duration is inserted into a cut beat", beatEdit(3, FieldDuration, 1200),
			"      - key: {name: enter}\n", "      - key: {name: enter}\n    duration: 1.2s\n"},
		{"auto factor deletes a block key", beatEdit(2, FieldZoomFactor, 0), "      factor: 1.5\n", ""},
		{"auto factor deletes a flow key", beatEdit(4, FieldZoomFactor, 0),
			"{target: {text: 'beta', nth: 2}, factor: 2}", "{target: {text: 'beta', nth: 2}}"},
		{"factor is set", beatEdit(2, FieldZoomFactor, 1.8), "factor: 1.5", "factor: 1.8"},
		{"factor goes after a block target", beatEdit(6, FieldZoomFactor, 2.5),
			"        text: 'gamma'\n", "        text: 'gamma'\n      factor: 2.5\n"},
		{"target on a block zoom", beatEdit(2, FieldZoomTarget, json.RawMessage(`{"text": "hi", "nth": 2}`)),
			"target: {text: 'hello', area: status}", "target: {text: 'hi', nth: 2}"},
		{"target on a flow zoom", beatEdit(4, FieldZoomTarget, json.RawMessage(`{"text": "gamma", "area": "list"}`)),
			"{target: {text: 'beta', nth: 2}, factor: 2}", "{target: {text: 'gamma', area: 'list'}, factor: 2}"},
		{"a target's last match is written as nth: last", beatEdit(2, FieldZoomTarget, json.RawMessage(`{"text": "hi", "nth": "last"}`)),
			"target: {text: 'hello', area: status}", "target: {text: 'hi', nth: last}"},
		{"target on a block zoom holding only its target", beatEdit(6, FieldZoomTarget, json.RawMessage(`{"text": "delta"}`)),
			"    zoom:\n      target:\n        text: 'gamma'\n", "    zoom: {target: {text: 'delta'}}\n"},
		{"zoom in goes after the factor", beatEdit(2, FieldZoomIn, 1500),
			"      factor: 1.5\n", "      factor: 1.5\n      in: 1.5s\n"},
		{"zoom out goes last in a flow zoom", beatEdit(4, FieldZoomOut, 750), "factor: 2}", "factor: 2, out: 750ms}"},
		{"zoom is added as a flow mapping", beatEdit(5, FieldZoom, json.RawMessage(`{"text": "x", "area": "list"}`)),
			"      - pause: {for: 1s}\n", "      - pause: {for: 1s}\n    zoom: {target: {text: 'x', area: 'list'}}\n"},
		{"a block zoom is removed", beatEdit(2, FieldZoom, nil),
			"    zoom:\n      target: {text: 'hello', area: status}\n      factor: 1.5\n", ""},
		{"a flow zoom is removed", beatEdit(4, FieldZoom, nil),
			"    zoom: {target: {text: 'beta', nth: 2}, factor: 2}\n", ""},
		{"frame width in a flow frame", frameEdit(FieldFrameWidth, 640), "width: 600", "width: 640"},
		{"frame fps in a flow frame", frameEdit(FieldFrameFPS, 30), "fps: 24", "fps: 30"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := readBoard(t)
			if count := strings.Count(string(src), tc.old); count != 1 {
				t.Fatalf("the fixture holds %q %d times, want once", tc.old, count)
			}
			want := strings.Replace(string(src), tc.old, tc.new, 1)

			got, _, err := Apply(src, boardPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if string(got) != want {
				t.Errorf("Apply(%+v) =\n%s\nwant\n%s", tc.edit, got, want)
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
		{"a zero hold is not written out", beatEdit(5, FieldHold, 0)},
		{"cut off on a beat with no cut", beatEdit(5, FieldCut, false)},
		{"auto factor on a zoom with no factor", beatEdit(6, FieldZoomFactor, 0)},
		{"a default ramp is not written out", beatEdit(2, FieldZoomIn, 1000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := readBoard(t)

			got, _, err := Apply(src, boardPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if string(got) != string(src) {
				t.Errorf("Apply(%+v) changed the source:\n%s", tc.edit, got)
			}
		})
	}
}

func TestApply_Edit_ReturnsTheStoryboardTheBytesDecodeTo(t *testing.T) {
	t.Parallel()
	src := readBoard(t)

	_, board, err := Apply(src, boardPath, beatEdit(2, FieldHold, 250))

	if err != nil {
		t.Fatalf("Apply: %v", err)
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
		{"hold reaching the duration", beatEdit(1, FieldHold, 1800)},
		{"cut off on a beat with no duration", beatEdit(3, FieldCut, false)},
		{"a factor beyond the range", beatEdit(2, FieldZoomFactor, 3.5)},
		{"a target with no text", beatEdit(2, FieldZoomTarget, json.RawMessage(`{"area": "list"}`))},
		{"a zero frame width", frameEdit(FieldFrameWidth, 0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := readBoard(t)

			_, _, err := Apply(src, boardPath, tc.edit)

			var invalid *ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("Apply(%+v) = %v, want a *ValidationError", tc.edit, err)
			}
			if !strings.Contains(err.Error(), tc.edit.Field) {
				t.Errorf("Apply(%+v) = %q, want it to name %s", tc.edit, err, tc.edit.Field)
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
		{"a zero ramp names the default", beatEdit(2, FieldZoomIn, 0), "zoom.in: want a ramp longer than 0ms; a ramp left out runs the 1s default"},
		{"a zero out ramp names the default", beatEdit(4, FieldZoomOut, 0), "zoom.out: want a ramp longer than 0ms; a ramp left out runs the 1s default"},
		{"a zoom field on a beat with no zoom", beatEdit(5, FieldZoomTarget, json.RawMessage(`{"text": "x"}`)), "beat 5: zoom.target: the beat has no zoom"},
		{"a second zoom", beatEdit(2, FieldZoom, json.RawMessage(`{"text": "x"}`)), "beat 2: zoom: the beat already zooms"},
		{"removing a zoom that is not there", beatEdit(5, FieldZoom, nil), "beat 5: zoom: the beat has no zoom to remove"},
		{"an unknown beat", beatEdit(9, FieldDuration, 1000), "beat 9: duration: the storyboard has no beat with id 9"},
		{"an unknown field", beatEdit(1, "title", "x"), "beat 1: title: unknown field"},
		{"a frame field on a beat", beatEdit(1, FieldFrameFPS, 30), "beat 1: frame.fps: want a clip owner"},
		{"a frame field on a clip index past 0", Edit{Owner: Owner{Kind: OwnerClip, Index: 1},
			Field: FieldFrameFPS, Value: typedValue(30)}, "frame.fps: want the clip at index 0"},
		{"a beat field on a camera move", Edit{Owner: Owner{Kind: OwnerCamera}, Field: FieldDuration, Value: typedValue("1000")}, "camera[0]: duration: want a beat owner"},
		{"a null duration", beatEdit(2, FieldDuration, nil), "beat 2: duration: want a value, got null"},
		{"an absent duration", Edit{Owner: Owner{Kind: OwnerBeat, Index: 2}, Field: FieldDuration},
			"beat 2: duration: want a value, got none"},
		{"a null zoom target", beatEdit(2, FieldZoomTarget, nil), "beat 2: zoom.target: want a value, got null"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := readBoard(t)

			_, _, err := Apply(src, boardPath, tc.edit)

			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Errorf("Apply(%+v) = %v, want an error containing %q", tc.edit, err, tc.message)
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
		{"milliseconds", beatEdit(2, FieldDuration, 1850), true},
		{"milliseconds as a string", beatEdit(2, FieldDuration, "1850"), false},
		{"a duration with a unit", beatEdit(2, FieldDuration, "1.85s"), false},
		{"fractional milliseconds", beatEdit(2, FieldDuration, 1850.5), false},
		{"a negative hold", beatEdit(2, FieldHold, -5), false},
		{"a ramp with a unit", beatEdit(2, FieldZoomIn, "1s"), false},
		{"a ramp as a bool", beatEdit(2, FieldZoomOut, true), false},
		{"cut true", beatEdit(5, FieldCut, true), true},
		{"cut false", beatEdit(2, FieldCut, false), true},
		{"cut as a string", beatEdit(2, FieldCut, "true"), false},
		{"cut as a number", beatEdit(2, FieldCut, 1), false},
		{"a decimal factor", beatEdit(2, FieldZoomFactor, 1.8), true},
		{"a whole factor", beatEdit(2, FieldZoomFactor, 2), true},
		{"factor 0 for auto", beatEdit(2, FieldZoomFactor, 0), true},
		{"a negative factor", beatEdit(2, FieldZoomFactor, -1.5), false},
		{"auto as a string", beatEdit(2, FieldZoomFactor, "auto"), false},
		{"a frame integer", frameEdit(FieldFrameWidth, 640), true},
		{"a frame decimal", frameEdit(FieldFrameWidth, 6.5), false},
		{"a frame integer as a string", frameEdit(FieldFrameFPS, "30"), false},
		{"a negative frame integer", frameEdit(FieldFrameFPS, -30), false},
		{"a target", beatEdit(2, FieldZoomTarget, json.RawMessage(`{"text": "ok", "area": "list", "nth": 2}`)), true},
		{"a target with nth last", beatEdit(2, FieldZoomTarget, json.RawMessage(`{"text": "ok", "nth": "last"}`)), true},
		{"a target with nth 0", beatEdit(2, FieldZoomTarget, json.RawMessage(`{"text": "ok", "nth": 0}`)), false},
		{"a target with nth first", beatEdit(2, FieldZoomTarget, json.RawMessage(`{"text": "ok", "nth": "first"}`)), false},
		{"a target with nth as a decimal", beatEdit(2, FieldZoomTarget, json.RawMessage(`{"text": "ok", "nth": 1.5}`)), false},
		{"a target as YAML flow text", beatEdit(2, FieldZoomTarget, "{text: 'ok'}"), false},
		{"a target with an unknown key", beatEdit(2, FieldZoomTarget, json.RawMessage(`{"text": "ok", "bogus": 1}`)), false},
		{"a target with a numeric text", beatEdit(2, FieldZoomTarget, json.RawMessage(`{"text": 7}`)), false},
		{"a zoom added onto a target", beatEdit(5, FieldZoom, json.RawMessage(`{"text": "ok", "area": "list"}`)), true},
		{"a zoom added onto a sequence", beatEdit(5, FieldZoom, []string{"ok"}), false},
		{"a zoom removed", beatEdit(2, FieldZoom, nil), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := readBoard(t)

			_, _, err := Apply(src, boardPath, tc.edit)

			if tc.isAccepted && err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if !tc.isAccepted && (err == nil || !strings.Contains(err.Error(), tc.edit.Field+": want")) {
				t.Errorf("Apply(%+v) = %v, want a spelling error naming %s", tc.edit, err, tc.edit.Field)
			}
		})
	}
}

func TestKeyOrder_StructTags_GiveTheStoryboardKeyOrder(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		of   reflect.Type
		want []string
	}{
		{"a beat", reflect.TypeFor[Beat](),
			[]string{"id", "title", "why", "notes", "do", "duration", "hold", "cut", "zoom", "expect"}},
		{"a zoom", reflect.TypeFor[Zoom](), []string{"target", "factor", "in", "out"}},
		{"the frame", reflect.TypeFor[Frame](),
			[]string{"cols", "rows", "padding", "font_size", "line_height", "scale", "width", "fps", "max_colors"}},
		{"a storyboard skips its path", reflect.TypeFor[Storyboard](),
			[]string{"clip", "ship", "frame", "expect", "beats", "camera", "captions"}},
		{"a point leaves its inline target out", reflect.TypeFor[Point](), []string{"cell"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := keyOrder(tc.of)

			if !slices.Equal(got, tc.want) {
				t.Errorf("keyOrder(%s) = %q, want %q", tc.of, got, tc.want)
			}
		})
	}
}
