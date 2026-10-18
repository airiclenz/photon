package storyboard

import (
	"errors"
	"strings"
	"testing"
)

// listEditPath is the name the list edit tests parse their storyboard under.
const listEditPath = "list.yaml"

// blockCameraBoard is editBaseBoard with a block camera of a zoom over [100ms, 300ms) and a pan
// over [900ms, 1s), leaving room in beat 1 for a new move of 600ms on either track.
const blockCameraBoard = editBaseBoard + `camera:
  # the opening zoom
  - zoom:
      to: 2
      at: 100ms
      over: 200ms
  - pan:
      to: {cell: {x: 3, y: 2}}
      at: 900ms
      over: 100ms
`

// flowCameraBoard is blockCameraBoard's camera written as one flow list.
const flowCameraBoard = editBaseBoard +
	"camera: [{zoom: {to: 2, at: 100ms, over: 200ms}}, {pan: {to: {cell: {x: 3, y: 2}}, at: 900ms, over: 100ms}}]\n"

// flowCaptionsBoard is editBaseBoard with a flow captions list of two captions.
const flowCaptionsBoard = editBaseBoard + "captions: [{text: 'One', for: 500ms}, {text: 'Two', at: 600ms, for: 300ms}]\n"

// newEntry is a list insert's value: an entry of kind starting at the moment beat and offsetMS
// name.
func newEntry(kind string, beat int, offsetMS int64) map[string]any {
	return map[string]any{"kind": kind, "at": map[string]int64{"beat": int64(beat), "offset_ms": offsetMS}}
}

func TestApply_ListEdit_ChangesOnlyTheListsText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		board    string
		edit     Edit
		old, new string
	}{
		{"a pan inserted between block moves", blockCameraBoard,
			cameraEdit(1, FieldCameraInsert, newEntry("pan", 0, 300)),
			"  - pan:\n", "  - {pan: {to: {cell: {x: 10, y: 2}}, at: 300ms, over: 600ms}}\n  - pan:\n"},
		{"a zoom appended to a flow camera", flowCameraBoard,
			cameraEdit(2, FieldCameraInsert, newEntry("zoom", 1, 300)),
			"over: 100ms}}]", "over: 100ms}}, {zoom: {to: 1.5, at: {beat: 1, offset: 300ms}, over: 600ms}}]"},
		{"a caption inserted first in a block list", captionEditBoard,
			captionEdit(0, FieldCaptionInsert, newEntry("caption", 0, 0)),
			"captions:\n", "captions:\n  - {text: \"Caption\", for: 2s}\n"},
		{"a caption inserted into a flow list", flowCaptionsBoard,
			captionEdit(1, FieldCaptionInsert, newEntry("caption", 1, 0)),
			"for: 500ms}, ", "for: 500ms}, {text: \"Caption\", at: {beat: 1}, for: 2s}, "},
		{"a block move removed", blockCameraBoard,
			cameraEdit(1, FieldCameraRemove, nil),
			"  - pan:\n      to: {cell: {x: 3, y: 2}}\n      at: 900ms\n      over: 100ms\n", ""},
		{"a flow caption removed", flowCaptionsBoard,
			captionEdit(0, FieldCaptionRemove, nil),
			"[{text: 'One', for: 500ms}, ", "["},
		{"a block move moved last", blockCameraBoard,
			cameraEdit(0, FieldCameraMove, 1),
			"  - zoom:\n      to: 2\n      at: 100ms\n      over: 200ms\n  - pan:\n      to: {cell: {x: 3, y: 2}}\n      at: 900ms\n      over: 100ms\n",
			"  - pan:\n      to: {cell: {x: 3, y: 2}}\n      at: 900ms\n      over: 100ms\n  - zoom:\n      to: 2\n      at: 100ms\n      over: 200ms\n"},
		{"a flow caption moved first", flowCaptionsBoard,
			captionEdit(1, FieldCaptionMove, 0),
			"[{text: 'One', for: 500ms}, {text: 'Two', at: 600ms, for: 300ms}]",
			"[{text: 'Two', at: 600ms, for: 300ms}, {text: 'One', for: 500ms}]"},
		{"a move to its own index", blockCameraBoard, cameraEdit(1, FieldCameraMove, 1), "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := strings.Replace(tc.board, tc.old, tc.new, 1)

			got, _, err := Apply([]byte(tc.board), listEditPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if string(got) != want {
				t.Errorf("Apply(%+v) =\n%s\nwant\n%s", tc.edit, got, want)
			}
		})
	}
}

func TestApply_InsertIntoAnAbsentOrEmptyList_WritesAFlowList(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		board, want string
		edit        Edit
	}{
		{"a camera after the beats", editBaseBoard,
			editBaseBoard + "camera: [{zoom: {to: 1.5, over: 600ms}}]\n",
			cameraEdit(0, FieldCameraInsert, newEntry("zoom", 0, 0))},
		{"captions after the camera", cameraEditBoard,
			cameraEditBoard + "captions: [{text: \"Caption\", at: 200ms, for: 2s}]\n",
			captionEdit(0, FieldCaptionInsert, newEntry("caption", 0, 200))},
		{"an empty camera", editBaseBoard + "camera: []\n",
			editBaseBoard + "camera: [{pan: {to: {cell: {x: 10, y: 2}}, over: 600ms}}]\n",
			cameraEdit(0, FieldCameraInsert, newEntry("pan", 0, 0))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, board, err := Apply([]byte(tc.board), listEditPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if string(got) != tc.want {
				t.Errorf("Apply(%+v) =\n%s\nwant\n%s", tc.edit, got, tc.want)
			}
			if len(board.Camera)+len(board.Captions) == 0 {
				t.Errorf("Apply(%+v) returned a storyboard without the new entry", tc.edit)
			}
		})
	}
}

func TestApply_RemoveTheOnlyEntry_DeletesTheListKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		board string
		edit  Edit
	}{
		{"a block camera", editBaseBoard + "camera:\n  - zoom: {to: 2, at: 100ms, over: 200ms}\n",
			cameraEdit(0, FieldCameraRemove, nil)},
		{"a flow captions list", editBaseBoard + "captions: [{text: 'Hi', for: 1s}]\n",
			captionEdit(0, FieldCaptionRemove, nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, _, err := Apply([]byte(tc.board), listEditPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if string(got) != editBaseBoard {
				t.Errorf("Apply(%+v) =\n%s\nwant\n%s", tc.edit, got, editBaseBoard)
			}
		})
	}
}

func TestApply_InsertOverlappingAMove_IsRefusedNamingBoth(t *testing.T) {
	t.Parallel()

	_, _, err := Apply([]byte(blockCameraBoard), listEditPath, cameraEdit(0, FieldCameraInsert, newEntry("zoom", 0, 100)))

	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("apply = %v, want a *ValidationError", err)
	}
	for _, want := range []string{"camera[0]: camera.insert: ", "camera[1]: zoom: overlaps camera[0] on the zoom track"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("apply = %q, want it to contain %q", err, want)
		}
	}
}

func TestApply_ListEditOutOfRangeOrMistyped_IsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		board   string
		edit    Edit
		message string
	}{
		{"an insert past the list's end", blockCameraBoard, cameraEdit(3, FieldCameraInsert, newEntry("zoom", 0, 300)),
			"camera[3]: camera.insert: want an index from 0 to 2 for the new camera move, got 3"},
		{"an insert at a negative index", flowCaptionsBoard, captionEdit(-1, FieldCaptionInsert, newEntry("caption", 0, 0)),
			"captions[-1]: caption.insert: want an index from 0 to 2 for the new caption, got -1"},
		{"a remove past the last move", blockCameraBoard, cameraEdit(2, FieldCameraRemove, nil),
			"camera[2]: camera.remove: the storyboard has no camera move at index 2"},
		{"a remove from an absent list", editBaseBoard, captionEdit(0, FieldCaptionRemove, nil),
			"captions[0]: caption.remove: the storyboard has no caption at index 0"},
		{"a move of a negative index", flowCaptionsBoard, captionEdit(-1, FieldCaptionMove, 0),
			"captions[-1]: caption.move: the storyboard has no caption at index -1"},
		{"a move past the last entry", blockCameraBoard, cameraEdit(0, FieldCameraMove, 2),
			"camera[0]: camera.move: want a destination index from 0 to 1, got 2"},
		{"a move to a fraction", blockCameraBoard, cameraEdit(0, FieldCameraMove, 0.5),
			"camera[0]: camera.move: want a destination index from 0 to 1, got 0.5"},
		{"a remove with a value", blockCameraBoard, cameraEdit(0, FieldCameraRemove, 0),
			"camera[0]: camera.remove: want null, got 0"},
		{"a null move", blockCameraBoard, cameraEdit(0, FieldCameraMove, nil),
			"camera[0]: camera.move: want a value, got null"},
		{"a null insert", blockCameraBoard, cameraEdit(0, FieldCameraInsert, nil),
			"camera[0]: camera.insert: want a value, got null"},
		{"a caption inserted into the camera", blockCameraBoard, cameraEdit(0, FieldCameraInsert, newEntry("caption", 0, 0)),
			"camera[0]: camera.insert: want a new camera move such as"},
		{"an insert without a kind", flowCaptionsBoard, captionEdit(0, FieldCaptionInsert, map[string]any{"at": moment(0, 0)}),
			"captions[0]: caption.insert: want a new caption such as"},
		{"an insert with an unknown key", flowCaptionsBoard,
			captionEdit(0, FieldCaptionInsert, map[string]any{"kind": "caption", "at": moment(0, 0), "text": "x"}),
			"captions[0]: caption.insert: want a new caption such as"},
		{"an insert with a bad at", flowCaptionsBoard,
			captionEdit(0, FieldCaptionInsert, map[string]any{"kind": "caption", "at": 5}),
			"captions[0]: caption.insert: at: want a moment"},
		{"a list field on a beat", blockCameraBoard, beatEdit(1, FieldCameraRemove, nil),
			"beat 1: camera.remove: want a camera owner"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := Apply([]byte(tc.board), listEditPath, tc.edit)

			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Errorf("Apply(%+v) = %v, want an error containing %q", tc.edit, err, tc.message)
			}
		})
	}
}
