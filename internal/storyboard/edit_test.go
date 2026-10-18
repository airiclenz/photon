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
		{"frame width in a flow frame", frameEdit(FieldFrameWidth, 640), "width: 600", "width: 640"},
		{"frame fps in a flow frame", frameEdit(FieldFrameFPS, 30), "fps: 24", "fps: 30"},
		{"frame padding in a flow frame", frameEdit(FieldFramePadding, 0), "padding: 16", "padding: 0"},
		{"frame font size in a flow frame", frameEdit(FieldFrameFontSize, 18), "font_size: 14", "font_size: 18"},
		{"frame line height in a flow frame", frameEdit(FieldFrameLineHeight, 1.35), "line_height: 1.2", "line_height: 1.35"},
		{"frame scale in a flow frame", frameEdit(FieldFrameScale, 3), "scale: 2", "scale: 3"},
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

func TestApply_FrameEdit_InABlockFrame_ChangesOnlyTheEditedLine(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		edit     Edit
		old, new string
	}{
		{"padding", frameEdit(FieldFramePadding, 8), "  padding: 16\n", "  padding: 8\n"},
		{"font size", frameEdit(FieldFrameFontSize, 12.5), "  font_size: 14\n", "  font_size: 12.5\n"},
		{"line height", frameEdit(FieldFrameLineHeight, 1.4), "  line_height: 1.2\n", "  line_height: 1.4\n"},
		{"scale", frameEdit(FieldFrameScale, 1), "  scale: 2\n", "  scale: 1\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src, _ := readEveryForm(t)
			if count := strings.Count(string(src), tc.old); count != 1 {
				t.Fatalf("the fixture holds %q %d times, want once", tc.old, count)
			}
			want := strings.Replace(string(src), tc.old, tc.new, 1)

			got, _, err := Apply(src, everyFormPath, tc.edit)

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
		{"a zero frame width", frameEdit(FieldFrameWidth, 0)},
		{"a frame font size below the caption range", frameEdit(FieldFrameFontSize, 5)},
		{"a frame font size beyond the caption range", frameEdit(FieldFrameFontSize, 73)},
		{"a zero frame scale", frameEdit(FieldFrameScale, 0)},
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
		{"an unknown beat", beatEdit(9, FieldDuration, 1000), "beat 9: duration: the storyboard has no beat with id 9"},
		{"an unknown field", beatEdit(1, "title", "x"), "beat 1: title: unknown field"},
		{"a frame field on a beat", beatEdit(1, FieldFrameFPS, 30), "beat 1: frame.fps: want a clip owner"},
		{"a frame field on a clip index past 0", Edit{Owner: Owner{Kind: OwnerClip, Index: 1},
			Field: FieldFrameFPS, Value: typedValue(30)}, "frame.fps: want the clip at index 0"},
		{"a beat field on a camera move", Edit{Owner: Owner{Kind: OwnerCamera}, Field: FieldDuration, Value: typedValue("1000")}, "camera[0]: duration: want a beat owner"},
		{"a null duration", beatEdit(2, FieldDuration, nil), "beat 2: duration: want a value, got null"},
		{"an absent duration", Edit{Owner: Owner{Kind: OwnerBeat, Index: 2}, Field: FieldDuration},
			"beat 2: duration: want a value, got none"},
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
		{"cut true", beatEdit(5, FieldCut, true), true},
		{"cut false", beatEdit(2, FieldCut, false), true},
		{"cut as a string", beatEdit(2, FieldCut, "true"), false},
		{"cut as a number", beatEdit(2, FieldCut, 1), false},
		{"a frame integer", frameEdit(FieldFrameWidth, 640), true},
		{"a frame decimal", frameEdit(FieldFrameWidth, 6.5), false},
		{"a frame integer as a string", frameEdit(FieldFrameFPS, "30"), false},
		{"a negative frame integer", frameEdit(FieldFrameFPS, -30), false},
		{"a negative frame padding", frameEdit(FieldFramePadding, -1), false},
		{"a decimal frame scale", frameEdit(FieldFrameScale, 1.5), false},
		{"a decimal frame font size", frameEdit(FieldFrameFontSize, 14.5), true},
		{"a whole frame line height", frameEdit(FieldFrameLineHeight, 2), true},
		{"a zero frame line height", frameEdit(FieldFrameLineHeight, 0), false},
		{"a frame font size as a string", frameEdit(FieldFrameFontSize, "14"), false},
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

func TestNth_JSON_RoundTripsAndRefusesOtherSpellings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		wire      string
		want      Nth
		isRefused bool
	}{
		{name: "the first match by number", wire: `1`, want: NthFirst},
		{name: "a later match", wire: `2`, want: 2},
		{name: "the last match", wire: `"last"`, want: NthLast},
		{name: "zero", wire: `0`, isRefused: true},
		{name: "a negative number", wire: `-1`, isRefused: true},
		{name: "the word first", wire: `"first"`, isRefused: true},
		{name: "a decimal", wire: `1.5`, isRefused: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var decoded Nth

			decodeErr := json.Unmarshal([]byte(test.wire), &decoded)
			encoded, encodeErr := json.Marshal(decoded)

			if test.isRefused {
				if decodeErr == nil || !strings.Contains(decodeErr.Error(), `want a whole number from 1 or "last"`) {
					t.Errorf("decode %s: want the nth refused, got %v and %d", test.wire, decodeErr, decoded)
				}
				return
			}
			if decodeErr != nil || decoded != test.want {
				t.Fatalf("decode %s: want %d, got %d and %v", test.wire, test.want, decoded, decodeErr)
			}
			if encodeErr != nil || string(encoded) != test.wire {
				t.Errorf("encode %d: want %s, got %s and %v", decoded, test.wire, encoded, encodeErr)
			}
		})
	}
}

func TestTarget_JSON_LeavesTheFirstMatchAndTheWholeScreenOut(t *testing.T) {
	t.Parallel()
	tests := []struct {
		target Target
		want   string
	}{
		{Target{Text: "ok"}, `{"text":"ok"}`},
		{Target{Text: "ok", Area: "list"}, `{"text":"ok","area":"list"}`},
		{Target{Text: "ok", Nth: 2}, `{"text":"ok","nth":2}`},
		{Target{Text: "ok", Nth: NthLast, Area: "list"}, `{"text":"ok","nth":"last","area":"list"}`},
	}
	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			t.Parallel()

			encoded, err := json.Marshal(test.target)

			if err != nil || string(encoded) != test.want {
				t.Fatalf("encode %+v: want %s, got %s and %v", test.target, test.want, encoded, err)
			}
			var decoded Target
			if err := json.Unmarshal(encoded, &decoded); err != nil || decoded != test.target {
				t.Errorf("decode %s: want %+v, got %+v and %v", encoded, test.target, decoded, err)
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
		{"the frame", reflect.TypeFor[Frame](),
			[]string{"cols", "rows", "padding", "font_size", "line_height", "scale", "width", "fps", "max_colors"}},
		{"a storyboard skips its path", reflect.TypeFor[Storyboard](),
			[]string{"clip", "ship", "frame", "expect", "beats", "camera", "captions"}},
		{"a point leaves its inline target out", reflect.TypeFor[Point](), []string{"cell"}},
		{"a zoom move skips its unexported start", reflect.TypeFor[ZoomMove](), []string{"to", "target", "at", "over", "ease"}},
		{"a pan move", reflect.TypeFor[PanMove](), []string{"to", "via", "at", "over", "ease"}},
		{"a caption skips its resolved times and colours", reflect.TypeFor[Caption](), []string{
			"text", "at", "for", "until", "font", "place", "align", "offset", "position", "color", "box", "fade",
		}},
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

// editBaseBoard is the storyboard the camera and caption edit tests build on: beat 1 lasts 1s and
// beat 2 1.5s.
const editBaseBoard = `# A storyboard the camera and caption edits start from.
clip: tuned
ship: out/tuned.gif
frame: {cols: 20, rows: 4, padding: 4, font_size: 10, line_height: 1.2, scale: 2, width: 160, fps: 10, max_colors: 16}
beats:
  - id: 1
    title: typing
    duration: 1s
  - id: 2
    title: result
    do:
      - pause: {for: 400ms}
    duration: 1500ms # kept in ms
`

// cameraCaptionsBoard is editBaseBoard with a camera of one zoom and three pans — a via path and
// a pan to a target and to a cell — inside beat 1, and two captions: one set by for in beat 1,
// one by until in beat 2.
const cameraCaptionsBoard = editBaseBoard + `camera:
  - zoom: {to: 2, at: 100ms, over: 300ms}
  - pan: {via: [{cell: {x: 2, y: 1}}, {text: 'failing'}], at: {beat: 1, offset: 200ms}, over: 600ms, ease: linear}
  - pan: {to: {text: 'tests', nth: last}, at: 800ms, over: 100ms}
  - pan: {to: {cell: {x: 3, y: 2}}, at: 900ms, over: 100ms}
captions:
  - {text: 'Watch this', at: {beat: 1, offset: 100ms}, for: 700ms, place: top, align: left}
  - {text: "All green\n", at: {beat: 2}, until: {beat: 2, offset: 1200ms}}
`

// moment is a moment as an Edit types it: beat 0 for a clip time offsetMS after the clip's first
// frame, or offsetMS past the start of the beat with that id.
func moment(beat int, offsetMS int64) json.RawMessage {
	return typedValue(map[string]int64{"beat": int64(beat), "offset_ms": offsetMS})
}

// cameraEditPath is the name the camera edit tests parse their storyboard under.
const cameraEditPath = "camera.yaml"

// cameraEditBoard is editBaseBoard with a camera of block and flow moves, all inside beat 1:
// zooms over [100ms, 400ms), [500ms, 900ms) and [0s, 50ms), pans over [200ms, 800ms) and
// [900ms, 1s).
const cameraEditBoard = editBaseBoard + `camera:
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

// cameraEdit is an Edit of the camera move at index, its value typed as value encodes.
func cameraEdit(index int, field string, value any) Edit {
	owner := Owner{Kind: OwnerCamera, Index: index}
	return Edit{Owner: owner, Field: field, Value: typedValue(value)}
}

func TestApply_CameraEdit_ChangesOnlyTheEditedText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		edit     Edit
		old, new string
	}{
		{"a block beat at becomes a clip time", cameraEdit(0, FieldCameraAt, moment(0, 150)),
			"at:\n        beat: 1\n        offset: 100ms\n", "at: 150ms\n"},
		{"a clip time becomes a beat start", cameraEdit(1, FieldCameraAt, moment(1, 0)),
			"at: 200ms # after the zoom starts", "at: {beat: 1} # after the zoom starts"},
		{"a block clip time becomes a beat moment", cameraEdit(1, FieldCameraAt, moment(1, 300)),
			"at: 200ms # after the zoom starts", "at: {beat: 1, offset: 300ms} # after the zoom starts"},
		{"a flow beat at becomes a clip time", cameraEdit(3, FieldCameraAt, moment(0, 600)),
			"at: {beat: 1, offset: 500ms}", "at: 600ms"},
		{"a flow clip time becomes a beat moment", cameraEdit(2, FieldCameraAt, moment(1, 850)),
			"at: 900ms", "at: {beat: 1, offset: 850ms}"},
		{"an absent at is inserted after to", cameraEdit(4, FieldCameraAt, moment(0, 50)),
			"{to: 1.2, over: 50ms}", "{to: 1.2, at: 50ms, over: 50ms}"},
		{"a block over keeps its unit", cameraEdit(0, FieldCameraOver, 250), "over: 300ms", "over: 250ms"},
		{"a flow over keeps its unit", cameraEdit(3, FieldCameraOver, 350), "over: 400ms", "over: 350ms"},
		{"smooth deletes a block ease", cameraEdit(0, FieldCameraEase, "smooth"), "      ease: linear\n", ""},
		{"smooth deletes a flow ease", cameraEdit(3, FieldCameraEase, "smooth"), ", ease: linear}", "}"},
		{"linear goes after a block over", cameraEdit(1, FieldCameraEase, "linear"),
			"      over: 600ms\n", "      over: 600ms\n      ease: linear\n"},
		{"linear goes last in a flow move", cameraEdit(2, FieldCameraEase, "linear"),
			"over: 100ms}", "over: 100ms, ease: linear}"},
		{"a block zoom's factor", cameraEdit(0, FieldCameraTo, 2.5), "to: 1.5", "to: 2.5"},
		{"a flow zoom's factor", cameraEdit(3, FieldCameraTo, 1.25), "to: 2,", "to: 1.25,"},
		{"a block pan target becomes a cell", cameraEdit(1, FieldCameraTo, json.RawMessage(`{"cell": {"x": 1, "y": 1}}`)),
			"to:\n        text: 'failing'\n", "to: {cell: {x: 1, y: 1}}\n"},
		{"a flow pan cell becomes a target", cameraEdit(2, FieldCameraTo, json.RawMessage(`{"text": "tests", "nth": "last"}`)),
			"to: {cell: {x: 3, y: 2}}", "to: {text: 'tests', nth: last}"},
		{"a block pan target becomes another", cameraEdit(1, FieldCameraTo, json.RawMessage(`{"text": "ok", "nth": 2, "area": "list"}`)),
			"to:\n        text: 'failing'\n", "to: {text: 'ok', nth: 2, area: 'list'}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if count := strings.Count(cameraEditBoard, tc.old); count != 1 {
				t.Fatalf("the board holds %q %d times, want once", tc.old, count)
			}
			want := strings.Replace(cameraEditBoard, tc.old, tc.new, 1)

			got, _, err := Apply([]byte(cameraEditBoard), cameraEditPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if string(got) != want {
				t.Errorf("Apply(%+v) =\n%s\nwant\n%s", tc.edit, got, want)
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
		{"a clip start on a move without at", cameraEdit(4, FieldCameraAt, moment(0, 0))},
		{"smooth on a move without ease", cameraEdit(1, FieldCameraEase, "smooth")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, _, err := Apply([]byte(cameraEditBoard), cameraEditPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if string(got) != cameraEditBoard {
				t.Errorf("Apply(%+v) changed the source:\n%s", tc.edit, got)
			}
		})
	}
}

func TestApply_CameraEditMakingZoomsOverlap_IsRefusedNamingBoth(t *testing.T) {
	t.Parallel()

	_, _, err := Apply([]byte(cameraEditBoard), cameraEditPath, cameraEdit(0, FieldCameraAt, moment(0, 450)))

	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("apply = %v, want a *ValidationError", err)
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
		{"an index past the last move", cameraEditBoard, cameraEdit(9, FieldCameraOver, 100),
			"camera[9]: camera.over: the storyboard has no camera move at index 9"},
		{"a negative index", cameraEditBoard, cameraEdit(-1, FieldCameraOver, 100),
			"camera[-1]: camera.over: the storyboard has no camera move at index -1"},
		{"a storyboard without a camera", editBaseBoard, cameraEdit(0, FieldCameraEase, "linear"),
			"camera[0]: camera.ease: the storyboard has no camera move at index 0"},
		{"the destination of a via pan", cameraCaptionsBoard, cameraEdit(1, FieldCameraTo, json.RawMessage(`{"text": "x"}`)),
			"camera[1]: camera.to: the pan follows a via path"},
		{"a null at", cameraEditBoard, cameraEdit(1, FieldCameraAt, nil), "camera[1]: camera.at: want a value, got null"},
		{"a null over", cameraEditBoard, cameraEdit(1, FieldCameraOver, nil), "camera[1]: camera.over: want a value, got null"},
		{"a null ease", cameraEditBoard, cameraEdit(1, FieldCameraEase, nil), "camera[1]: camera.ease: want a value, got null"},
		{"a null to", cameraEditBoard, cameraEdit(1, FieldCameraTo, nil), "camera[1]: camera.to: want a value, got null"},
		{"an unknown camera field", cameraEditBoard, cameraEdit(0, "camera.speed", "x"),
			"camera[0]: camera.speed: unknown field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := Apply([]byte(tc.board), cameraEditPath, tc.edit)

			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Errorf("Apply(%+v) = %v, want an error containing %q", tc.edit, err, tc.message)
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
		{"a beat moment", cameraEdit(1, FieldCameraAt, json.RawMessage(`{"beat": 1, "offset_ms": 300}`)), true},
		{"a beat start", cameraEdit(1, FieldCameraAt, json.RawMessage(`{"beat": 1, "offset_ms": 0}`)), true},
		{"a clip time", cameraEdit(1, FieldCameraAt, json.RawMessage(`{"beat": 0, "offset_ms": 150}`)), true},
		{"a moment without an offset", cameraEdit(1, FieldCameraAt, json.RawMessage(`{"beat": 1}`)), false},
		{"a moment without a beat", cameraEdit(1, FieldCameraAt, json.RawMessage(`{"offset_ms": 150}`)), false},
		{"a moment with an unknown key", cameraEdit(1, FieldCameraAt, json.RawMessage(`{"beat": 1, "offset_ms": 0, "bogus": 2}`)), false},
		{"a moment at a negative beat", cameraEdit(1, FieldCameraAt, json.RawMessage(`{"beat": -1, "offset_ms": 0}`)), false},
		{"a negative clip time", cameraEdit(1, FieldCameraAt, json.RawMessage(`{"beat": 0, "offset_ms": -5}`)), false},
		{"a fractional offset", cameraEdit(1, FieldCameraAt, json.RawMessage(`{"beat": 0, "offset_ms": 1.5}`)), false},
		{"a moment as YAML text", cameraEdit(1, FieldCameraAt, "{beat: 1, offset: 300ms}"), false},
		{"a clip time as text", cameraEdit(1, FieldCameraAt, "150ms"), false},
		{"milliseconds of over", cameraEdit(0, FieldCameraOver, 250), true},
		{"an over as a string", cameraEdit(0, FieldCameraOver, "250"), false},
		{"an over with a unit", cameraEdit(0, FieldCameraOver, "250ms"), false},
		{"a negative over", cameraEdit(0, FieldCameraOver, -5), false},
		{"linear", cameraEdit(1, FieldCameraEase, "linear"), true},
		{"smooth", cameraEdit(0, FieldCameraEase, "smooth"), true},
		{"an ease as another word", cameraEdit(1, FieldCameraEase, "fast"), false},
		{"an empty ease", cameraEdit(1, FieldCameraEase, ""), false},
		{"an ease as a number", cameraEdit(1, FieldCameraEase, 1), false},
		{"a zoom factor", cameraEdit(0, FieldCameraTo, 2.5), true},
		{"a whole zoom factor", cameraEdit(0, FieldCameraTo, 2), true},
		{"a zoom factor as a string", cameraEdit(0, FieldCameraTo, "2.5"), false},
		{"a zoom factor of auto", cameraEdit(0, FieldCameraTo, "auto"), false},
		{"a pan target", cameraEdit(2, FieldCameraTo, json.RawMessage(`{"text": "tests"}`)), true},
		{"a pan target with nth last", cameraEdit(2, FieldCameraTo, json.RawMessage(`{"text": "tests", "nth": "last"}`)), true},
		{"a pan target with nth 0", cameraEdit(2, FieldCameraTo, json.RawMessage(`{"text": "tests", "nth": 0}`)), false},
		{"a pan cell", cameraEdit(1, FieldCameraTo, json.RawMessage(`{"cell": {"x": 1, "y": 1}}`)), true},
		{"a pan cell with an unknown key", cameraEdit(1, FieldCameraTo, json.RawMessage(`{"cell": {"x": 1, "z": 1}}`)), false},
		{"a pan target and cell at once", cameraEdit(1, FieldCameraTo, json.RawMessage(`{"text": "tests", "cell": {"x": 1, "y": 1}}`)), false},
		{"a pan point as a sequence", cameraEdit(1, FieldCameraTo, []int{1, 2}), false},
		{"a pan point as YAML flow text", cameraEdit(1, FieldCameraTo, "{text: 'tests'}"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := Apply([]byte(cameraEditBoard), cameraEditPath, tc.edit)

			if tc.isAccepted && err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if !tc.isAccepted && (err == nil || !strings.Contains(err.Error(), tc.edit.Field+": want")) {
				t.Errorf("Apply(%+v) = %v, want a spelling error naming %s", tc.edit, err, tc.edit.Field)
			}
		})
	}
}

// captionEditPath is the name the caption edit tests parse their storyboard under.
const captionEditPath = "captions.yaml"

// captionEditBoard is editBaseBoard with captions of block and flow forms: beat 1 lasts 1s,
// beat 2 1.5s, so the clip ends at 2.5s.
const captionEditBoard = editBaseBoard + `captions:
  - text: |
      Watch
      this
    at:
      beat: 1
      offset: 100ms
    for: 700ms # short
    place: top
  - {text: 'All green', at: {beat: 2}, until: {beat: 2, offset: 1200ms}, align: right}
  - text: "Done"
    at: 1200ms
    until: 2s
  - {text: 'Start', for: 300ms}
`

// captionEdit is an Edit of the caption at index, its value typed as value encodes.
func captionEdit(index int, field string, value any) Edit {
	owner := Owner{Kind: OwnerCaption, Index: index}
	return Edit{Owner: owner, Field: field, Value: typedValue(value)}
}

func TestApply_CaptionEdit_ChangesOnlyTheEditedText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		edit     Edit
		old, new string
	}{
		{"a block beat at becomes a clip time", captionEdit(0, FieldCaptionAt, moment(0, 150)),
			"    at:\n      beat: 1\n      offset: 100ms\n", "    at: 150ms\n"},
		{"a flow beat at moves to another beat", captionEdit(1, FieldCaptionAt, moment(1, 900)),
			"at: {beat: 2},", "at: {beat: 1, offset: 900ms},"},
		{"an absent at is inserted after text", captionEdit(3, FieldCaptionAt, moment(0, 100)),
			"{text: 'Start', for: 300ms}", "{text: 'Start', at: 100ms, for: 300ms}"},
		{"for keeps its unit", captionEdit(0, FieldCaptionFor, 650), "for: 700ms # short", "for: 650ms # short"},
		{"until replaces a block for", captionEdit(0, FieldCaptionUntil, moment(1, 900)),
			"    for: 700ms # short\n", "    until: {beat: 1, offset: 900ms}\n"},
		{"for replaces a flow until", captionEdit(1, FieldCaptionFor, 1000),
			", until: {beat: 2, offset: 1200ms}", ", for: 1s"},
		{"until is set", captionEdit(2, FieldCaptionUntil, moment(0, 2200)), "until: 2s", "until: 2200ms"},
		{"a null until becomes the for of its stretch", captionEdit(2, FieldCaptionUntil, nil),
			"    at: 1200ms\n    until: 2s\n", "    at: 1200ms\n    for: 800ms\n"},
		{"a null flow until becomes the for of its stretch", captionEdit(1, FieldCaptionUntil, nil),
			", until: {beat: 2, offset: 1200ms}", ", for: 1.2s"},
		{"text replaces a block scalar", captionEdit(0, FieldCaptionText, "Look\nhere"),
			"text: |\n      Watch\n      this\n", "text: \"Look\\nhere\"\n"},
		{"text replaces a single-quoted string", captionEdit(1, FieldCaptionText, "All good"),
			"'All green'", `"All good"`},
		{"text escapes its quotes", captionEdit(2, FieldCaptionText, `Done "now"`),
			`text: "Done"`, `text: "Done \"now\""`},
		{"bottom deletes a place", captionEdit(0, FieldCaptionPlace, "bottom"), "    place: top\n", ""},
		{"place is set", captionEdit(0, FieldCaptionPlace, "middle"), "place: top", "place: middle"},
		{"place goes after until in a flow caption", captionEdit(1, FieldCaptionPlace, "top"),
			"1200ms}, align", "1200ms}, place: top, align"},
		{"middle deletes an align", captionEdit(1, FieldCaptionAlign, "middle"), ", align: right}", "}"},
		{"align goes after place", captionEdit(0, FieldCaptionAlign, "left"),
			"    place: top\n", "    place: top\n    align: left\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if count := strings.Count(captionEditBoard, tc.old); count != 1 {
				t.Fatalf("the board holds %q %d times, want once", tc.old, count)
			}
			want := strings.Replace(captionEditBoard, tc.old, tc.new, 1)

			got, _, err := Apply([]byte(captionEditBoard), captionEditPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if string(got) != want {
				t.Errorf("Apply(%+v) =\n%s\nwant\n%s", tc.edit, got, want)
			}
		})
	}
}

func TestApply_CaptionShift_MovesTheStartAndEndTogether(t *testing.T) {
	t.Parallel()
	commentedBoard := strings.Replace(captionEditBoard, "until: 2s\n", "until: 2s # the end\n", 1)
	cases := []struct {
		name     string
		board    string
		edit     Edit
		old, new string
	}{
		{"a block beat at keeps its beat and its for", captionEditBoard, captionEdit(0, FieldCaptionShift, 200),
			"    at:\n      beat: 1\n      offset: 100ms\n", "    at: {beat: 1, offset: 300ms}\n"},
		{"a flow until caption shifts earlier, each moment on its beat", captionEditBoard,
			captionEdit(1, FieldCaptionShift, -100),
			"at: {beat: 2}, until: {beat: 2, offset: 1200ms}", "at: {beat: 2, offset: -100ms}, until: {beat: 2, offset: 1100ms}"},
		{"a block until caption shifts later", captionEditBoard, captionEdit(2, FieldCaptionShift, 300),
			"    at: 1200ms\n    until: 2s\n", "    at: 1500ms\n    until: 2300ms\n"},
		{"an until's comment stays", commentedBoard, captionEdit(2, FieldCaptionShift, -200),
			"    at: 1200ms\n    until: 2s # the end\n", "    at: 1000ms\n    until: 1800ms # the end\n"},
		{"an absent at is inserted after text", captionEditBoard, captionEdit(3, FieldCaptionShift, 100),
			"{text: 'Start', for: 300ms}", "{text: 'Start', at: 100ms, for: 300ms}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if count := strings.Count(tc.board, tc.old); count != 1 {
				t.Fatalf("the board holds %q %d times, want once", tc.old, count)
			}
			want := strings.Replace(tc.board, tc.old, tc.new, 1)

			got, _, err := Apply([]byte(tc.board), captionEditPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if string(got) != want {
				t.Errorf("Apply(%+v) =\n%s\nwant\n%s", tc.edit, got, want)
			}
		})
	}
}

func TestApply_CaptionShift_KeepsEachCaptionsLength(t *testing.T) {
	t.Parallel()
	const delta = 100 * time.Millisecond
	before, err := Parse([]byte(captionEditBoard), captionEditPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	for index, old := range before.Captions {
		_, board, err := Apply([]byte(captionEditBoard), captionEditPath,
			captionEdit(index, FieldCaptionShift, delta.Milliseconds()))

		if err != nil {
			t.Fatalf("shift caption %d: %v", index, err)
		}
		shifted := board.Captions[index]
		if shifted.Start() != old.Start()+delta || shifted.Stop() != old.Stop()+delta {
			t.Errorf("caption %d runs %v to %v, want %v to %v", index, shifted.Start(), shifted.Stop(),
				old.Start()+delta, old.Stop()+delta)
		}
	}
}

func TestApply_CaptionEditToWhatAnAbsentKeyMeans_LeavesTheSourceAsItIs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		edit Edit
	}{
		{"a clip start on a caption without at", captionEdit(3, FieldCaptionAt, moment(0, 0))},
		{"a null until on a caption without until", captionEdit(3, FieldCaptionUntil, nil)},
		{"a shift of 0", captionEdit(1, FieldCaptionShift, 0)},
		{"bottom on a caption without place", captionEdit(2, FieldCaptionPlace, "bottom")},
		{"middle on a caption without align", captionEdit(2, FieldCaptionAlign, "middle")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, _, err := Apply([]byte(captionEditBoard), captionEditPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if string(got) != captionEditBoard {
				t.Errorf("Apply(%+v) changed the source:\n%s", tc.edit, got)
			}
		})
	}
}

func TestApply_CaptionText_RoundTripsThroughTheStoryboard(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		text string
	}{
		{"two lines", "Line one\nLine two"},
		{"three lines in a block scalar's place", "a\nb\nc"},
		{"quotes and a backslash", `say "hi" \ 'there'`},
		{"a tab and a non-ASCII rune", "x\ty → z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, index := range []int{0, 1} {
				got, _, err := Apply([]byte(captionEditBoard), captionEditPath, captionEdit(index, FieldCaptionText, tc.text))
				if err != nil {
					t.Fatalf("apply caption %d: %v", index, err)
				}

				board, err := Parse(got, captionEditPath)

				if err != nil {
					t.Fatalf("Parse after editing caption %d: %v", index, err)
				}
				if board.Captions[index].Text != tc.text {
					t.Errorf("caption %d text %q, want %q", index, board.Captions[index].Text, tc.text)
				}
			}
		})
	}
}

func TestApply_CaptionEditFailingValidation_IsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		edit    Edit
		message string
	}{
		{"a stop before the start", captionEdit(2, FieldCaptionUntil, moment(0, 1000)), "until: want a stop after the start"},
		{"a for past the clip's end", captionEdit(0, FieldCaptionFor, 5000), "for: want the caption to end by the clip's end"},
		{"a start past the clip's end", captionEdit(3, FieldCaptionAt, moment(0, 3000)), "at: want a start in"},
		{"an empty text", captionEdit(1, FieldCaptionText, ""), "text: missing"},
		{"a shift before the clip's start", captionEdit(2, FieldCaptionShift, -1300), "at: want a start in"},
		{"a shift past the clip's end", captionEdit(2, FieldCaptionShift, 600),
			"until: want the caption to end by the clip's end"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := Apply([]byte(captionEditBoard), captionEditPath, tc.edit)

			var invalid *ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("Apply(%+v) = %v, want a *ValidationError", tc.edit, err)
			}
			for _, want := range []string{editName(tc.edit) + ": ", tc.message} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Apply(%+v) = %q, want it to contain %q", tc.edit, err, want)
				}
			}
		})
	}
}

func TestApply_CaptionEditOnWhatIsNotThere_IsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		board   string
		edit    Edit
		message string
	}{
		{"an index past the last caption", captionEditBoard, captionEdit(9, FieldCaptionFor, 100),
			"captions[9]: caption.for: the storyboard has no caption at index 9"},
		{"a negative index", captionEditBoard, captionEdit(-1, FieldCaptionFor, 100),
			"captions[-1]: caption.for: the storyboard has no caption at index -1"},
		{"a storyboard without captions", editBaseBoard, captionEdit(0, FieldCaptionPlace, "top"),
			"captions[0]: caption.place: the storyboard has no caption at index 0"},
		{"an unknown caption field", captionEditBoard, captionEdit(0, "caption.colour", "#000000"),
			"captions[0]: caption.colour: unknown field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := Apply([]byte(tc.board), captionEditPath, tc.edit)

			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Errorf("Apply(%+v) = %v, want an error containing %q", tc.edit, err, tc.message)
			}
		})
	}
}

func TestApply_CaptionNull_IsRefusedButForUntil(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		edit    Edit
		message string
	}{
		{"a null at", captionEdit(1, FieldCaptionAt, nil), "captions[1]: caption.at: want a value, got null"},
		{"a null for", captionEdit(1, FieldCaptionFor, nil), "captions[1]: caption.for: want a value, got null"},
		{"a null text", captionEdit(1, FieldCaptionText, nil), "captions[1]: caption.text: want a value, got null"},
		{"a null place", captionEdit(1, FieldCaptionPlace, nil), "captions[1]: caption.place: want a value, got null"},
		{"a null align", captionEdit(1, FieldCaptionAlign, nil), "captions[1]: caption.align: want a value, got null"},
		{"a null offset", captionEdit(1, FieldCaptionOffset, nil), "captions[1]: caption.offset: want a value, got null"},
		{"a null shift", captionEdit(1, FieldCaptionShift, nil), "captions[1]: caption.shift: want a value, got null"},
		{"an until with no value", Edit{Owner: Owner{Kind: OwnerCaption, Index: 1}, Field: FieldCaptionUntil},
			"captions[1]: caption.until: want a value, got none"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := Apply([]byte(captionEditBoard), captionEditPath, tc.edit)

			if err == nil || err.Error() != tc.message {
				t.Errorf("Apply(%+v) = %v, want %q", tc.edit, err, tc.message)
			}
		})
	}

	t.Run("a null until is cleared", func(t *testing.T) {
		t.Parallel()

		_, board, err := Apply([]byte(captionEditBoard), captionEditPath, captionEdit(1, FieldCaptionUntil, nil))
		if err != nil {
			t.Fatalf("apply: %v", err)
		}

		caption := board.Captions[1]
		if caption.Until != nil || caption.For != 1200*time.Millisecond || caption.Stop() != 2200*time.Millisecond {
			t.Errorf("captions[1] until %v, for %v, stop %v; want no until, for 1.2s, the stop still at 2.2s",
				caption.Until, caption.For, caption.Stop())
		}
	})
}

func TestApply_CaptionWireSpellings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		edit       Edit
		isAccepted bool
	}{
		{"a beat moment", captionEdit(0, FieldCaptionAt, json.RawMessage(`{"beat": 1, "offset_ms": 200}`)), true},
		{"a moment as YAML text", captionEdit(0, FieldCaptionAt, "{beat: 1, offset: 200ms}"), false},
		{"a moment without an offset", captionEdit(0, FieldCaptionAt, json.RawMessage(`{"beat": 1}`)), false},
		{"a moment with another key", captionEdit(0, FieldCaptionAt,
			json.RawMessage(`{"beat": 1, "offset_ms": 0, "offset": "200ms"}`)), false},
		{"milliseconds of for", captionEdit(0, FieldCaptionFor, 500), true},
		{"a for as text", captionEdit(0, FieldCaptionFor, "500"), false},
		{"a for with a fraction", captionEdit(0, FieldCaptionFor, 500.5), false},
		{"a negative for", captionEdit(0, FieldCaptionFor, -500), false},
		{"a for of 0", captionEdit(0, FieldCaptionFor, 0), false},
		{"an until as a clip time", captionEdit(1, FieldCaptionUntil, json.RawMessage(`{"beat": 0, "offset_ms": 2000}`)), true},
		{"an until as YAML text", captionEdit(1, FieldCaptionUntil, "2s"), false},
		{"a negative shift", captionEdit(1, FieldCaptionShift, -100), true},
		{"a shift as text", captionEdit(1, FieldCaptionShift, "100"), false},
		{"a shift with a unit", captionEdit(1, FieldCaptionShift, "100ms"), false},
		{"a shift with a fraction", captionEdit(1, FieldCaptionShift, 100.5), false},
		{"a shift no duration holds", captionEdit(1, FieldCaptionShift, json.RawMessage(`9223372036855`)), false},
		{"a shift taking a moment past what a duration holds", captionEdit(0, FieldCaptionShift,
			json.RawMessage(`9223372036854`)), false},
		{"a text", captionEdit(0, FieldCaptionText, "Look"), true},
		{"a text as a number", captionEdit(0, FieldCaptionText, 42), false},
		{"a place", captionEdit(0, FieldCaptionPlace, "middle"), true},
		{"a place as another word", captionEdit(0, FieldCaptionPlace, "center"), false},
		{"an empty place", captionEdit(0, FieldCaptionPlace, ""), false},
		{"a place as a number", captionEdit(0, FieldCaptionPlace, 1), false},
		{"an align", captionEdit(0, FieldCaptionAlign, "right"), true},
		{"an align as a place", captionEdit(0, FieldCaptionAlign, "top"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := Apply([]byte(captionEditBoard), captionEditPath, tc.edit)

			if tc.isAccepted && err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if !tc.isAccepted && (err == nil || !strings.Contains(err.Error(), tc.edit.Field+": want")) {
				t.Errorf("Apply(%+v) = %v, want a spelling error naming %s", tc.edit, err, tc.edit.Field)
			}
		})
	}
}
