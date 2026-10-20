package storyboard

import (
	"errors"
	"strings"
	"testing"
)

// listEditPath is the name the list edit tests parse their storyboard under.
const listEditPath = "list.yaml"

// blockCaptionsBoard is editBaseBoard with a block captions list of two block captions.
const blockCaptionsBoard = editBaseBoard + `captions:
  # the opening caption
  - text: 'One'
    for: 500ms
  - text: 'Two'
    at: 600ms
    for: 300ms
`

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
		{"a caption inserted between block captions", blockCaptionsBoard,
			captionEdit(1, FieldCaptionInsert, newEntry("caption", 0, 300)),
			"  - text: 'Two'\n", "  - {text: \"Caption\", at: 300ms, for: 2s}\n  - text: 'Two'\n"},
		{"a caption appended to a flow list", flowCaptionsBoard,
			captionEdit(2, FieldCaptionInsert, newEntry("caption", 1, 300)),
			"for: 300ms}]", "for: 300ms}, {text: \"Caption\", at: {beat: 1, offset: 300ms}, for: 2s}]"},
		{"a caption inserted first in a block list", captionEditBoard,
			captionEdit(0, FieldCaptionInsert, newEntry("caption", 0, 0)),
			"captions:\n", "captions:\n  - {text: \"Caption\", for: 2s}\n"},
		{"a caption inserted into a flow list", flowCaptionsBoard,
			captionEdit(1, FieldCaptionInsert, newEntry("caption", 1, 0)),
			"for: 500ms}, ", "for: 500ms}, {text: \"Caption\", at: {beat: 1}, for: 2s}, "},
		{"a block caption removed", blockCaptionsBoard,
			captionEdit(1, FieldCaptionRemove, nil),
			"  - text: 'Two'\n    at: 600ms\n    for: 300ms\n", ""},
		{"a flow caption removed", flowCaptionsBoard,
			captionEdit(0, FieldCaptionRemove, nil),
			"[{text: 'One', for: 500ms}, ", "["},
		{"a block caption moved last", blockCaptionsBoard,
			captionEdit(0, FieldCaptionMove, 1),
			"  - text: 'One'\n    for: 500ms\n  - text: 'Two'\n    at: 600ms\n    for: 300ms\n",
			"  - text: 'Two'\n    at: 600ms\n    for: 300ms\n  - text: 'One'\n    for: 500ms\n"},
		{"a flow caption moved first", flowCaptionsBoard,
			captionEdit(1, FieldCaptionMove, 0),
			"[{text: 'One', for: 500ms}, {text: 'Two', at: 600ms, for: 300ms}]",
			"[{text: 'Two', at: 600ms, for: 300ms}, {text: 'One', for: 500ms}]"},
		{"a move to its own index", blockCaptionsBoard, captionEdit(1, FieldCaptionMove, 1), "", ""},
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
		{"captions after the beats", editBaseBoard,
			editBaseBoard + "captions: [{text: \"Caption\", for: 2s}]\n",
			captionEdit(0, FieldCaptionInsert, newEntry("caption", 0, 0))},
		{"captions after the camera", cameraEditBoard,
			cameraEditBoard + "captions: [{text: \"Caption\", at: 200ms, for: 2s}]\n",
			captionEdit(0, FieldCaptionInsert, newEntry("caption", 0, 200))},
		{"an empty captions list", editBaseBoard + "captions: []\n",
			editBaseBoard + "captions: [{text: \"Caption\", at: {beat: 1}, for: 2s}]\n",
			captionEdit(0, FieldCaptionInsert, newEntry("caption", 1, 0))},
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
			if len(board.Captions) == 0 {
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
		{"a block captions list", editBaseBoard + "captions:\n  - text: 'Hi'\n    for: 1s\n",
			captionEdit(0, FieldCaptionRemove, nil)},
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

func TestApply_InsertEndingPastTheClip_IsRefusedNamingIt(t *testing.T) {
	t.Parallel()

	_, _, err := Apply([]byte(flowCaptionsBoard), listEditPath, captionEdit(2, FieldCaptionInsert, newEntry("caption", 2, 1000)))

	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("apply = %v, want a *ValidationError", err)
	}
	for _, want := range []string{"captions[2]: caption.insert: ", "captions[2]: for: "} {
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
		{"an insert past the list's end", flowCaptionsBoard, captionEdit(3, FieldCaptionInsert, newEntry("caption", 0, 300)),
			"captions[3]: caption.insert: want an index from 0 to 2 for the new caption, got 3"},
		{"an insert at a negative index", flowCaptionsBoard, captionEdit(-1, FieldCaptionInsert, newEntry("caption", 0, 0)),
			"captions[-1]: caption.insert: want an index from 0 to 2 for the new caption, got -1"},
		{"a remove past the last caption", flowCaptionsBoard, captionEdit(2, FieldCaptionRemove, nil),
			"captions[2]: caption.remove: the storyboard has no caption at index 2"},
		{"a remove from an absent list", editBaseBoard, captionEdit(0, FieldCaptionRemove, nil),
			"captions[0]: caption.remove: the storyboard has no caption at index 0"},
		{"a move of a negative index", flowCaptionsBoard, captionEdit(-1, FieldCaptionMove, 0),
			"captions[-1]: caption.move: the storyboard has no caption at index -1"},
		{"a move past the last entry", flowCaptionsBoard, captionEdit(0, FieldCaptionMove, 2),
			"captions[0]: caption.move: want a destination index from 0 to 1, got 2"},
		{"a move to a fraction", flowCaptionsBoard, captionEdit(0, FieldCaptionMove, 0.5),
			"captions[0]: caption.move: want a destination index from 0 to 1, got 0.5"},
		{"a remove with a value", flowCaptionsBoard, captionEdit(0, FieldCaptionRemove, 0),
			"captions[0]: caption.remove: want null, got 0"},
		{"a null move", flowCaptionsBoard, captionEdit(0, FieldCaptionMove, nil),
			"captions[0]: caption.move: want a value, got null"},
		{"a null insert", flowCaptionsBoard, captionEdit(0, FieldCaptionInsert, nil),
			"captions[0]: caption.insert: want a value, got null"},
		{"an insert of another kind", flowCaptionsBoard, captionEdit(0, FieldCaptionInsert, newEntry("zoom", 0, 0)),
			"captions[0]: caption.insert: want a new caption such as"},
		{"an insert without a kind", flowCaptionsBoard, captionEdit(0, FieldCaptionInsert, map[string]any{"at": moment(0, 0)}),
			"captions[0]: caption.insert: want a new caption such as"},
		{"an insert with an unknown key", flowCaptionsBoard,
			captionEdit(0, FieldCaptionInsert, map[string]any{"kind": "caption", "at": moment(0, 0), "text": "x"}),
			"captions[0]: caption.insert: want a new caption such as"},
		{"an insert with a bad at", flowCaptionsBoard,
			captionEdit(0, FieldCaptionInsert, map[string]any{"kind": "caption", "at": 5}),
			"captions[0]: caption.insert: at: want a moment"},
		{"a list field on a beat", flowCaptionsBoard, beatEdit(1, FieldCaptionRemove, nil),
			"beat 1: caption.remove: want a caption owner"},
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

// newPositionValue is a position.insert value: a point in time at the moment beat and offsetMS
// name, on the cell x, y.
func newPositionValue(beat int, offsetMS int64, x, y int) map[string]any {
	return map[string]any{"at": moment(beat, offsetMS), "cell": cell(x, y)}
}

// newZoomValue is a zoom.insert value: a zoom at the moment beat and offsetMS name.
func newZoomValue(beat int, offsetMS int64) map[string]any {
	return map[string]any{"at": moment(beat, offsetMS)}
}

// positionsOnlyBoard is editBaseBoard with a camera of one block position and no zooms.
const positionsOnlyBoard = editBaseBoard + "camera:\n  positions:\n    - cell: {x: 3, y: 2}\n      at: 900ms\n"

// zoomsOnlyBoard is editBaseBoard with a camera of one flow zoom and no positions.
const zoomsOnlyBoard = editBaseBoard + "camera:\n  zooms: [{to: 2, at: 1s, over: 200ms}]\n"

func TestApply_CameraListEdit_ChangesOnlyTheListsText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		board    string
		edit     Edit
		old, new string
	}{
		{"a position inserted between positions", cameraEditBoard,
			positionEdit(1, FieldPositionInsert, newPositionValue(0, 850, 5, 1)),
			"    - {cell: {x: 3, y: 2}, at: 900ms}\n",
			"    - {cell: {x: 5, y: 1}, at: 850ms}\n    - {cell: {x: 3, y: 2}, at: 900ms}\n"},
		{"a position appended at a beat", cameraEditBoard,
			positionEdit(2, FieldPositionInsert, newPositionValue(2, 0, 0, 0)),
			"    - {cell: {x: 3, y: 2}, at: 900ms}\n",
			"    - {cell: {x: 3, y: 2}, at: 900ms}\n    - {cell: {x: 0, y: 0}, at: {beat: 2}}\n"},
		{"a position inserted first at the clip's start", positionsOnlyBoard,
			positionEdit(0, FieldPositionInsert, newPositionValue(0, 0, 1, 1)),
			"  positions:\n", "  positions:\n    - {cell: {x: 1, y: 1}}\n"},
		{"a zoom appended to a block list", cameraEditBoard,
			zoomEdit(3, FieldZoomInsert, newZoomValue(2, 300)),
			"    - {to: 1.2, over: 50ms}\n",
			"    - {to: 1.2, over: 50ms}\n    - {to: 1.5, at: {beat: 2, offset: 300ms}, over: 600ms}\n"},
		{"a zoom inserted first in a flow list", zoomsOnlyBoard,
			zoomEdit(0, FieldZoomInsert, newZoomValue(0, 0)),
			"[{to: 2", "[{to: 1.5, over: 600ms}, {to: 2"},
		{"a block position removed", cameraEditBoard,
			positionEdit(0, FieldPositionRemove, nil),
			"    - text: 'failing'\n      at: 200ms # after the zoom starts\n      hold: 600ms\n", ""},
		{"a flow zoom removed", cameraEditBoard,
			zoomEdit(2, FieldZoomRemove, nil),
			"    - {to: 1.2, over: 50ms}\n", ""},
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

func TestApply_CameraInsertIntoAnAbsentList_WritesAFlowList(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		board, want string
		edit        Edit
	}{
		{"a position into an absent camera", editBaseBoard,
			editBaseBoard + "camera: {positions: [{cell: {x: 3, y: 2}, at: 200ms}]}\n",
			positionEdit(0, FieldPositionInsert, newPositionValue(0, 200, 3, 2))},
		{"a zoom into an absent camera", editBaseBoard,
			editBaseBoard + "camera: {zooms: [{to: 1.5, at: {beat: 1}, over: 600ms}]}\n",
			zoomEdit(0, FieldZoomInsert, newZoomValue(1, 0))},
		{"a camera before the captions", captionEditBoard,
			strings.Replace(captionEditBoard, "captions:\n", "camera: {zooms: [{to: 1.5, over: 600ms}]}\ncaptions:\n", 1),
			zoomEdit(0, FieldZoomInsert, newZoomValue(0, 0))},
		{"a position into an empty camera", editBaseBoard + "camera: {}\n",
			editBaseBoard + "camera: {positions: [{cell: {x: 1, y: 0}}]}\n",
			positionEdit(0, FieldPositionInsert, newPositionValue(0, 0, 1, 0))},
		{"a zoom into a null camera", editBaseBoard + "camera:\n",
			editBaseBoard + "camera: {zooms: [{to: 1.5, over: 600ms}]}\n",
			zoomEdit(0, FieldZoomInsert, newZoomValue(0, 0))},
		{"positions before the zooms", zoomsOnlyBoard,
			strings.Replace(zoomsOnlyBoard, "  zooms:", "  positions: [{cell: {x: 1, y: 1}, at: 100ms}]\n  zooms:", 1),
			positionEdit(0, FieldPositionInsert, newPositionValue(0, 100, 1, 1))},
		{"zooms after the positions", positionsOnlyBoard,
			positionsOnlyBoard + "  zooms: [{to: 1.5, at: 100ms, over: 600ms}]\n",
			zoomEdit(0, FieldZoomInsert, newZoomValue(0, 100))},
		{"an empty positions list", editBaseBoard + "camera:\n  positions: []\n",
			editBaseBoard + "camera:\n  positions: [{cell: {x: 2, y: 3}, at: 1000ms}]\n",
			positionEdit(0, FieldPositionInsert, newPositionValue(0, 1000, 2, 3))},
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
			if len(board.Camera.Positions)+len(board.Camera.Zooms) == 0 {
				t.Errorf("Apply(%+v) returned a storyboard without the new entry", tc.edit)
			}
		})
	}
}

func TestApply_RemoveTheOnlyCameraEntry_DeletesTheListAndAnEmptiedCamera(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		board, want string
		edit        Edit
	}{
		{"the only position of a camera without zooms", positionsOnlyBoard, editBaseBoard,
			positionEdit(0, FieldPositionRemove, nil)},
		{"the only zoom of a camera without positions", zoomsOnlyBoard, editBaseBoard,
			zoomEdit(0, FieldZoomRemove, nil)},
		{"the only position beside zooms", editBaseBoard + "camera:\n  positions:\n    - {cell: {x: 1, y: 1}}\n  zooms: [{to: 2, at: 1s, over: 200ms}]\n",
			zoomsOnlyBoard, positionEdit(0, FieldPositionRemove, nil)},
		{"the only zoom of a flow camera beside positions", editBaseBoard + "camera: {positions: [{cell: {x: 1, y: 1}}], zooms: [{to: 2, over: 200ms}]}\n",
			editBaseBoard + "camera: {positions: [{cell: {x: 1, y: 1}}]}\n", zoomEdit(0, FieldZoomRemove, nil)},
		{"the only position of a flow camera", editBaseBoard + "camera: {positions: [{cell: {x: 1, y: 1}}]}\n",
			editBaseBoard, positionEdit(0, FieldPositionRemove, nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, _, err := Apply([]byte(tc.board), listEditPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if string(got) != tc.want {
				t.Errorf("Apply(%+v) =\n%s\nwant\n%s", tc.edit, got, tc.want)
			}
		})
	}
}

func TestApply_PositionInsertOverlappingAPosition_IsRefusedNamingBoth(t *testing.T) {
	t.Parallel()

	_, _, err := Apply([]byte(cameraEditBoard), listEditPath, positionEdit(1, FieldPositionInsert, newPositionValue(0, 500, 1, 1)))

	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("apply = %v, want a *ValidationError", err)
	}
	for _, want := range []string{"camera.positions[1]: position.insert: ", "overlaps camera.positions[0]"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("apply = %q, want it to contain %q", err, want)
		}
	}
}

func TestApply_CameraListEditOutOfRangeOrMistyped_IsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		board   string
		edit    Edit
		message string
	}{
		{"a position insert past the list's end", cameraEditBoard, positionEdit(3, FieldPositionInsert, newPositionValue(0, 0, 1, 1)),
			"camera.positions[3]: position.insert: want an index from 0 to 2 for the new camera position, got 3"},
		{"a zoom insert at a negative index", cameraEditBoard, zoomEdit(-1, FieldZoomInsert, newZoomValue(0, 0)),
			"camera.zooms[-1]: zoom.insert: want an index from 0 to 3 for the new camera zoom, got -1"},
		{"a position remove past the last position", cameraEditBoard, positionEdit(2, FieldPositionRemove, nil),
			"camera.positions[2]: position.remove: the storyboard has no camera position at index 2"},
		{"a zoom remove from an absent camera", editBaseBoard, zoomEdit(0, FieldZoomRemove, nil),
			"camera.zooms[0]: zoom.remove: the storyboard has no camera zoom at index 0"},
		{"a remove with a value", cameraEditBoard, zoomEdit(0, FieldZoomRemove, 0),
			"camera.zooms[0]: zoom.remove: want null, got 0"},
		{"a null insert", cameraEditBoard, positionEdit(0, FieldPositionInsert, nil),
			"camera.positions[0]: position.insert: want a value, got null"},
		{"a position insert without a cell", cameraEditBoard, positionEdit(0, FieldPositionInsert, newZoomValue(0, 0)),
			"camera.positions[0]: position.insert: want a new camera position such as"},
		{"a position insert with a target", cameraEditBoard,
			positionEdit(0, FieldPositionInsert, map[string]any{"at": moment(0, 0), "cell": cell(1, 1), "text": "x"}),
			"camera.positions[0]: position.insert: want a new camera position such as"},
		{"a position insert with a bad cell", cameraEditBoard,
			positionEdit(0, FieldPositionInsert, map[string]any{"at": moment(0, 0), "cell": cell(1, 1)["x"]}),
			"camera.positions[0]: position.insert: cell: want a cell such as"},
		{"a position insert with a bad at", cameraEditBoard,
			positionEdit(0, FieldPositionInsert, map[string]any{"at": 5, "cell": cell(1, 1)}),
			"camera.positions[0]: position.insert: at: want a moment"},
		{"a position insert off the grid", cameraEditBoard, positionEdit(2, FieldPositionInsert, newPositionValue(2, 0, 20, 0)),
			"camera.positions[2]: position.insert: refused: "},
		{"a zoom insert with a cell", cameraEditBoard, zoomEdit(0, FieldZoomInsert, newPositionValue(0, 0, 1, 1)),
			"camera.zooms[0]: zoom.insert: want a new camera zoom such as"},
		{"a zoom insert of a caption", cameraEditBoard, zoomEdit(0, FieldZoomInsert, newEntry("caption", 0, 0)),
			"camera.zooms[0]: zoom.insert: want a new camera zoom such as"},
		{"a zoom insert with a bad at", cameraEditBoard, zoomEdit(0, FieldZoomInsert, map[string]any{"at": "1s"}),
			"camera.zooms[0]: zoom.insert: at: want a moment"},
		{"a position field on a zoom owner", cameraEditBoard, zoomEdit(0, FieldPositionRemove, nil),
			"camera.zooms[0]: position.remove: want a position owner"},
		{"a position move", cameraEditBoard, positionEdit(0, "position.move", 1),
			"camera.positions[0]: position.move: unknown field"},
		{"a zoom move", cameraEditBoard, zoomEdit(0, "zoom.move", 1),
			"camera.zooms[0]: zoom.move: unknown field"},
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
