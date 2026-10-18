package tune

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/storyboard"
)

// The beat and frame fields storyboard applies, as the tune tests that build their edits name
// them.
const (
	fieldDuration   = storyboard.FieldDuration
	fieldHold       = storyboard.FieldHold
	fieldCut        = storyboard.FieldCut
	fieldZoomTarget = storyboard.FieldZoomTarget
	fieldZoomIn     = storyboard.FieldZoomIn
	fieldFrameWidth = storyboard.FieldFrameWidth
)

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
	owner := storyboard.Owner{Kind: storyboard.OwnerBeat, Index: id}
	return Edit{Owner: owner, Field: field, Value: typedValue(value)}
}

// frameEdit is an Edit of the clip's frame field, its value typed as value encodes.
func frameEdit(field string, value any) Edit {
	return Edit{Owner: storyboard.Owner{Kind: storyboard.OwnerClip}, Field: field, Value: typedValue(value)}
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

// cameraEdit is an Edit of the camera move at index, its value typed as value encodes.
func cameraEdit(index int, field string, value any) Edit {
	owner := storyboard.Owner{Kind: storyboard.OwnerCamera, Index: index}
	return Edit{Owner: owner, Field: field, Value: typedValue(value)}
}

func TestApply_CameraEdit_ChangesOnlyTheEditedText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		edit     Edit
		old, new string
	}{
		{"a block beat at becomes a clip time", cameraEdit(0, fieldCameraAt, MomentState{Beat: 0, OffsetMS: 150}),
			"at:\n        beat: 1\n        offset: 100ms\n", "at: 150ms\n"},
		{"a clip time becomes a beat start", cameraEdit(1, fieldCameraAt, MomentState{Beat: 1, OffsetMS: 0}),
			"at: 200ms # after the zoom starts", "at: {beat: 1} # after the zoom starts"},
		{"a block clip time becomes a beat moment", cameraEdit(1, fieldCameraAt, MomentState{Beat: 1, OffsetMS: 300}),
			"at: 200ms # after the zoom starts", "at: {beat: 1, offset: 300ms} # after the zoom starts"},
		{"a flow beat at becomes a clip time", cameraEdit(3, fieldCameraAt, MomentState{Beat: 0, OffsetMS: 600}),
			"at: {beat: 1, offset: 500ms}", "at: 600ms"},
		{"a flow clip time becomes a beat moment", cameraEdit(2, fieldCameraAt, MomentState{Beat: 1, OffsetMS: 850}),
			"at: 900ms", "at: {beat: 1, offset: 850ms}"},
		{"an absent at is inserted after to", cameraEdit(4, fieldCameraAt, MomentState{Beat: 0, OffsetMS: 50}),
			"{to: 1.2, over: 50ms}", "{to: 1.2, at: 50ms, over: 50ms}"},
		{"a block over keeps its unit", cameraEdit(0, fieldCameraOver, 250), "over: 300ms", "over: 250ms"},
		{"a flow over keeps its unit", cameraEdit(3, fieldCameraOver, 350), "over: 400ms", "over: 350ms"},
		{"smooth deletes a block ease", cameraEdit(0, fieldCameraEase, "smooth"), "      ease: linear\n", ""},
		{"smooth deletes a flow ease", cameraEdit(3, fieldCameraEase, "smooth"), ", ease: linear}", "}"},
		{"linear goes after a block over", cameraEdit(1, fieldCameraEase, "linear"),
			"      over: 600ms\n", "      over: 600ms\n      ease: linear\n"},
		{"linear goes last in a flow move", cameraEdit(2, fieldCameraEase, "linear"),
			"over: 100ms}", "over: 100ms, ease: linear}"},
		{"a block zoom's factor", cameraEdit(0, fieldCameraTo, 2.5), "to: 1.5", "to: 2.5"},
		{"a flow zoom's factor", cameraEdit(3, fieldCameraTo, 1.25), "to: 2,", "to: 1.25,"},
		{"a block pan target becomes a cell", cameraEdit(1, fieldCameraTo, json.RawMessage(`{"cell": {"x": 1, "y": 1}}`)),
			"to:\n        text: 'failing'\n", "to: {cell: {x: 1, y: 1}}\n"},
		{"a flow pan cell becomes a target", cameraEdit(2, fieldCameraTo, json.RawMessage(`{"text": "tests", "nth": "last"}`)),
			"to: {cell: {x: 3, y: 2}}", "to: {text: 'tests', nth: last}"},
		{"a block pan target becomes another", cameraEdit(1, fieldCameraTo, json.RawMessage(`{"text": "ok", "nth": 2, "area": "list"}`)),
			"to:\n        text: 'failing'\n", "to: {text: 'ok', nth: 2, area: 'list'}\n"},
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
		{"a clip start on a move without at", cameraEdit(4, fieldCameraAt, MomentState{Beat: 0, OffsetMS: 0})},
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

	_, _, err := apply([]byte(cameraEditBoard), cameraEditPath, cameraEdit(0, fieldCameraAt, MomentState{Beat: 0, OffsetMS: 450}))

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
		{"an index past the last move", cameraEditBoard, cameraEdit(9, fieldCameraOver, 100),
			"camera[9]: camera.over: the storyboard has no camera move at index 9"},
		{"a negative index", cameraEditBoard, cameraEdit(-1, fieldCameraOver, 100),
			"camera[-1]: camera.over: the storyboard has no camera move at index -1"},
		{"a storyboard without a camera", sessionStoryboard, cameraEdit(0, fieldCameraEase, "linear"),
			"camera[0]: camera.ease: the storyboard has no camera move at index 0"},
		{"the destination of a via pan", cameraCaptionsStoryboard, cameraEdit(1, fieldCameraTo, json.RawMessage(`{"text": "x"}`)),
			"camera[1]: camera.to: the pan follows a via path"},
		{"a null at", cameraEditBoard, cameraEdit(1, fieldCameraAt, nil), "camera[1]: camera.at: want a value, got null"},
		{"a null over", cameraEditBoard, cameraEdit(1, fieldCameraOver, nil), "camera[1]: camera.over: want a value, got null"},
		{"a null ease", cameraEditBoard, cameraEdit(1, fieldCameraEase, nil), "camera[1]: camera.ease: want a value, got null"},
		{"a null to", cameraEditBoard, cameraEdit(1, fieldCameraTo, nil), "camera[1]: camera.to: want a value, got null"},
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
		{"a beat moment", cameraEdit(1, fieldCameraAt, json.RawMessage(`{"beat": 1, "offset_ms": 300}`)), true},
		{"a beat start", cameraEdit(1, fieldCameraAt, json.RawMessage(`{"beat": 1, "offset_ms": 0}`)), true},
		{"a clip time", cameraEdit(1, fieldCameraAt, json.RawMessage(`{"beat": 0, "offset_ms": 150}`)), true},
		{"a moment without an offset", cameraEdit(1, fieldCameraAt, json.RawMessage(`{"beat": 1}`)), false},
		{"a moment without a beat", cameraEdit(1, fieldCameraAt, json.RawMessage(`{"offset_ms": 150}`)), false},
		{"a moment with an unknown key", cameraEdit(1, fieldCameraAt, json.RawMessage(`{"beat": 1, "offset_ms": 0, "bogus": 2}`)), false},
		{"a moment at a negative beat", cameraEdit(1, fieldCameraAt, json.RawMessage(`{"beat": -1, "offset_ms": 0}`)), false},
		{"a negative clip time", cameraEdit(1, fieldCameraAt, json.RawMessage(`{"beat": 0, "offset_ms": -5}`)), false},
		{"a fractional offset", cameraEdit(1, fieldCameraAt, json.RawMessage(`{"beat": 0, "offset_ms": 1.5}`)), false},
		{"a moment as YAML text", cameraEdit(1, fieldCameraAt, "{beat: 1, offset: 300ms}"), false},
		{"a clip time as text", cameraEdit(1, fieldCameraAt, "150ms"), false},
		{"milliseconds of over", cameraEdit(0, fieldCameraOver, 250), true},
		{"an over as a string", cameraEdit(0, fieldCameraOver, "250"), false},
		{"an over with a unit", cameraEdit(0, fieldCameraOver, "250ms"), false},
		{"a negative over", cameraEdit(0, fieldCameraOver, -5), false},
		{"linear", cameraEdit(1, fieldCameraEase, "linear"), true},
		{"smooth", cameraEdit(0, fieldCameraEase, "smooth"), true},
		{"an ease as another word", cameraEdit(1, fieldCameraEase, "fast"), false},
		{"an empty ease", cameraEdit(1, fieldCameraEase, ""), false},
		{"an ease as a number", cameraEdit(1, fieldCameraEase, 1), false},
		{"a zoom factor", cameraEdit(0, fieldCameraTo, 2.5), true},
		{"a whole zoom factor", cameraEdit(0, fieldCameraTo, 2), true},
		{"a zoom factor as a string", cameraEdit(0, fieldCameraTo, "2.5"), false},
		{"a zoom factor of auto", cameraEdit(0, fieldCameraTo, "auto"), false},
		{"a pan target", cameraEdit(2, fieldCameraTo, json.RawMessage(`{"text": "tests"}`)), true},
		{"a pan target with nth last", cameraEdit(2, fieldCameraTo, json.RawMessage(`{"text": "tests", "nth": "last"}`)), true},
		{"a pan target with nth 0", cameraEdit(2, fieldCameraTo, json.RawMessage(`{"text": "tests", "nth": 0}`)), false},
		{"a pan cell", cameraEdit(1, fieldCameraTo, json.RawMessage(`{"cell": {"x": 1, "y": 1}}`)), true},
		{"a pan cell with an unknown key", cameraEdit(1, fieldCameraTo, json.RawMessage(`{"cell": {"x": 1, "z": 1}}`)), false},
		{"a pan target and cell at once", cameraEdit(1, fieldCameraTo, json.RawMessage(`{"text": "tests", "cell": {"x": 1, "y": 1}}`)), false},
		{"a pan point as a sequence", cameraEdit(1, fieldCameraTo, []int{1, 2}), false},
		{"a pan point as YAML flow text", cameraEdit(1, fieldCameraTo, "{text: 'tests'}"), false},
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

// captionEditPath is the name the caption edit tests parse their storyboard under.
const captionEditPath = "captions.yaml"

// captionEditBoard is sessionStoryboard with captions of block and flow forms: beat 1 lasts 1s,
// beat 2 1.5s, so the clip ends at 2.5s.
const captionEditBoard = sessionStoryboard + `captions:
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
	owner := storyboard.Owner{Kind: storyboard.OwnerCaption, Index: index}
	return Edit{Owner: owner, Field: field, Value: typedValue(value)}
}

func TestApply_CaptionEdit_ChangesOnlyTheEditedText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		edit     Edit
		old, new string
	}{
		{"a block beat at becomes a clip time", captionEdit(0, fieldCaptionAt, MomentState{Beat: 0, OffsetMS: 150}),
			"    at:\n      beat: 1\n      offset: 100ms\n", "    at: 150ms\n"},
		{"a flow beat at moves to another beat", captionEdit(1, fieldCaptionAt, MomentState{Beat: 1, OffsetMS: 900}),
			"at: {beat: 2},", "at: {beat: 1, offset: 900ms},"},
		{"an absent at is inserted after text", captionEdit(3, fieldCaptionAt, MomentState{Beat: 0, OffsetMS: 100}),
			"{text: 'Start', for: 300ms}", "{text: 'Start', at: 100ms, for: 300ms}"},
		{"for keeps its unit", captionEdit(0, fieldCaptionFor, 650), "for: 700ms # short", "for: 650ms # short"},
		{"until replaces a block for", captionEdit(0, fieldCaptionUntil, MomentState{Beat: 1, OffsetMS: 900}),
			"    for: 700ms # short\n", "    until: {beat: 1, offset: 900ms}\n"},
		{"for replaces a flow until", captionEdit(1, fieldCaptionFor, 1000),
			", until: {beat: 2, offset: 1200ms}", ", for: 1s"},
		{"until is set", captionEdit(2, fieldCaptionUntil, MomentState{Beat: 0, OffsetMS: 2200}), "until: 2s", "until: 2200ms"},
		{"a null until becomes the for of its stretch", captionEdit(2, fieldCaptionUntil, nil),
			"    at: 1200ms\n    until: 2s\n", "    at: 1200ms\n    for: 800ms\n"},
		{"a null flow until becomes the for of its stretch", captionEdit(1, fieldCaptionUntil, nil),
			", until: {beat: 2, offset: 1200ms}", ", for: 1.2s"},
		{"text replaces a block scalar", captionEdit(0, fieldCaptionText, "Look\nhere"),
			"text: |\n      Watch\n      this\n", "text: \"Look\\nhere\"\n"},
		{"text replaces a single-quoted string", captionEdit(1, fieldCaptionText, "All good"),
			"'All green'", `"All good"`},
		{"text escapes its quotes", captionEdit(2, fieldCaptionText, `Done "now"`),
			`text: "Done"`, `text: "Done \"now\""`},
		{"bottom deletes a place", captionEdit(0, fieldCaptionPlace, "bottom"), "    place: top\n", ""},
		{"place is set", captionEdit(0, fieldCaptionPlace, "middle"), "place: top", "place: middle"},
		{"place goes after until in a flow caption", captionEdit(1, fieldCaptionPlace, "top"),
			"1200ms}, align", "1200ms}, place: top, align"},
		{"middle deletes an align", captionEdit(1, fieldCaptionAlign, "middle"), ", align: right}", "}"},
		{"align goes after place", captionEdit(0, fieldCaptionAlign, "left"),
			"    place: top\n", "    place: top\n    align: left\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if count := strings.Count(captionEditBoard, tc.old); count != 1 {
				t.Fatalf("the board holds %q %d times, want once", tc.old, count)
			}
			want := strings.Replace(captionEditBoard, tc.old, tc.new, 1)

			got, _, err := apply([]byte(captionEditBoard), captionEditPath, tc.edit)

			if err != nil {
				t.Fatalf("apply(%+v): %v", tc.edit, err)
			}
			if string(got) != want {
				t.Errorf("apply(%+v) =\n%s\nwant\n%s", tc.edit, got, want)
			}
		})
	}
}

func TestApply_CaptionEditToWhatAnAbsentKeyMeans_LeavesTheSourceAsItIs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		edit Edit
	}{
		{"a clip start on a caption without at", captionEdit(3, fieldCaptionAt, MomentState{Beat: 0, OffsetMS: 0})},
		{"a null until on a caption without until", captionEdit(3, fieldCaptionUntil, nil)},
		{"bottom on a caption without place", captionEdit(2, fieldCaptionPlace, "bottom")},
		{"middle on a caption without align", captionEdit(2, fieldCaptionAlign, "middle")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, _, err := apply([]byte(captionEditBoard), captionEditPath, tc.edit)

			if err != nil {
				t.Fatalf("apply(%+v): %v", tc.edit, err)
			}
			if string(got) != captionEditBoard {
				t.Errorf("apply(%+v) changed the source:\n%s", tc.edit, got)
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
				got, _, err := apply([]byte(captionEditBoard), captionEditPath, captionEdit(index, fieldCaptionText, tc.text))
				if err != nil {
					t.Fatalf("apply caption %d: %v", index, err)
				}

				board, err := storyboard.Parse(got, captionEditPath)

				if err != nil {
					t.Fatalf("storyboard.Parse after editing caption %d: %v", index, err)
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
		{"a stop before the start", captionEdit(2, fieldCaptionUntil, MomentState{Beat: 0, OffsetMS: 1000}), "until: want a stop after the start"},
		{"a for past the clip's end", captionEdit(0, fieldCaptionFor, 5000), "for: want the caption to end by the clip's end"},
		{"a start past the clip's end", captionEdit(3, fieldCaptionAt, MomentState{Beat: 0, OffsetMS: 3000}), "at: want a start in"},
		{"an empty text", captionEdit(1, fieldCaptionText, ""), "text: missing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := apply([]byte(captionEditBoard), captionEditPath, tc.edit)

			var invalid *storyboard.ValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("apply(%+v) = %v, want a *storyboard.ValidationError", tc.edit, err)
			}
			for _, want := range []string{editName(tc.edit) + ": ", tc.message} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("apply(%+v) = %q, want it to contain %q", tc.edit, err, want)
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
		{"an index past the last caption", captionEditBoard, captionEdit(9, fieldCaptionFor, 100),
			"captions[9]: caption.for: the storyboard has no caption at index 9"},
		{"a negative index", captionEditBoard, captionEdit(-1, fieldCaptionFor, 100),
			"captions[-1]: caption.for: the storyboard has no caption at index -1"},
		{"a storyboard without captions", sessionStoryboard, captionEdit(0, fieldCaptionPlace, "top"),
			"captions[0]: caption.place: the storyboard has no caption at index 0"},
		{"an unknown caption field", captionEditBoard, captionEdit(0, "caption.color", "#000000"),
			"captions[0]: caption.color: unknown field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := apply([]byte(tc.board), captionEditPath, tc.edit)

			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Errorf("apply(%+v) = %v, want an error containing %q", tc.edit, err, tc.message)
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
		{"a null at", captionEdit(1, fieldCaptionAt, nil), "captions[1]: caption.at: want a value, got null"},
		{"a null for", captionEdit(1, fieldCaptionFor, nil), "captions[1]: caption.for: want a value, got null"},
		{"a null text", captionEdit(1, fieldCaptionText, nil), "captions[1]: caption.text: want a value, got null"},
		{"a null place", captionEdit(1, fieldCaptionPlace, nil), "captions[1]: caption.place: want a value, got null"},
		{"a null align", captionEdit(1, fieldCaptionAlign, nil), "captions[1]: caption.align: want a value, got null"},
		{"an until with no value", Edit{Owner: storyboard.Owner{Kind: storyboard.OwnerCaption, Index: 1}, Field: fieldCaptionUntil},
			"captions[1]: caption.until: want a value, got none"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := apply([]byte(captionEditBoard), captionEditPath, tc.edit)

			if err == nil || err.Error() != tc.message {
				t.Errorf("apply(%+v) = %v, want %q", tc.edit, err, tc.message)
			}
		})
	}

	t.Run("a null until is cleared", func(t *testing.T) {
		t.Parallel()

		_, board, err := apply([]byte(captionEditBoard), captionEditPath, captionEdit(1, fieldCaptionUntil, nil))
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
		{"a beat moment", captionEdit(0, fieldCaptionAt, json.RawMessage(`{"beat": 1, "offset_ms": 200}`)), true},
		{"a moment as YAML text", captionEdit(0, fieldCaptionAt, "{beat: 1, offset: 200ms}"), false},
		{"a moment without an offset", captionEdit(0, fieldCaptionAt, json.RawMessage(`{"beat": 1}`)), false},
		{"a moment with another key", captionEdit(0, fieldCaptionAt,
			json.RawMessage(`{"beat": 1, "offset_ms": 0, "offset": "200ms"}`)), false},
		{"milliseconds of for", captionEdit(0, fieldCaptionFor, 500), true},
		{"a for as text", captionEdit(0, fieldCaptionFor, "500"), false},
		{"a for with a fraction", captionEdit(0, fieldCaptionFor, 500.5), false},
		{"a negative for", captionEdit(0, fieldCaptionFor, -500), false},
		{"a for of 0", captionEdit(0, fieldCaptionFor, 0), false},
		{"an until as a clip time", captionEdit(1, fieldCaptionUntil, json.RawMessage(`{"beat": 0, "offset_ms": 2000}`)), true},
		{"an until as YAML text", captionEdit(1, fieldCaptionUntil, "2s"), false},
		{"a text", captionEdit(0, fieldCaptionText, "Look"), true},
		{"a text as a number", captionEdit(0, fieldCaptionText, 42), false},
		{"a place", captionEdit(0, fieldCaptionPlace, "middle"), true},
		{"a place as another word", captionEdit(0, fieldCaptionPlace, "center"), false},
		{"an empty place", captionEdit(0, fieldCaptionPlace, ""), false},
		{"a place as a number", captionEdit(0, fieldCaptionPlace, 1), false},
		{"an align", captionEdit(0, fieldCaptionAlign, "right"), true},
		{"an align as a place", captionEdit(0, fieldCaptionAlign, "top"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := apply([]byte(captionEditBoard), captionEditPath, tc.edit)

			if tc.isAccepted && err != nil {
				t.Fatalf("apply(%+v): %v", tc.edit, err)
			}
			if !tc.isAccepted && (err == nil || !strings.Contains(err.Error(), tc.edit.Field+": want")) {
				t.Errorf("apply(%+v) = %v, want a spelling error naming %s", tc.edit, err, tc.edit.Field)
			}
		})
	}
}

func TestSession_CaptionEdit_SwapsTheEndAndUndoes(t *testing.T) {
	t.Parallel()
	f := newSessionFixtureOf(t, cameraCaptionsStoryboard)

	f.apply(t, captionEdit(1, fieldCaptionFor, 1000))
	edited := f.session.State()
	undoErr := f.session.Undo()
	undone := f.session.State()

	if caption := edited.Captions[1]; caption.ForMS != 1000 || caption.Until != nil || caption.StopMS != 2000 || !edited.IsDirty {
		t.Errorf("after the edit: captions[1] %+v, dirty %v; want for 1s, no until, a stop at 2s, dirty", caption, edited.IsDirty)
	}
	if undoErr != nil {
		t.Fatalf("Undo: %v", undoErr)
	}
	if caption := undone.Captions[1]; caption.ForMS != 0 || caption.Until == nil || undone.IsDirty {
		t.Errorf("after the undo: captions[1] %+v, dirty %v; want the until back, the bytes on disk", caption, undone.IsDirty)
	}
}
