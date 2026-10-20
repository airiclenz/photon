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
