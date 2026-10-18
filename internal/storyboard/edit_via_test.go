package storyboard

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// viaEditPath is the name the via edit tests parse their storyboard under.
const viaEditPath = "via.yaml"

// viaBoard is editBaseBoard, its frame's centre cell {x: 10, y: 2}, with a camera inside beat 1
// of a block via pan of three waypoints, one a block mapping, a flow via pan of three, a block and
// a flow straight pan and a zoom.
const viaBoard = editBaseBoard + `camera:
  - pan:
      via:
        - {cell: {x: 2, y: 1}}
        - text: 'failing'
          nth: last
        - {cell: {x: 5, y: 3}} # the end
      at: 100ms
      over: 300ms
  - pan: {via: [{text: 'ok'}, {cell: {x: 4, y: 0}}, {cell: {x: 6, y: 1}}], at: 400ms, over: 200ms}
  - pan:
      to: {cell: {x: 3, y: 2}}
      at: 600ms
      over: 100ms
  - pan: {to: {text: 'tests', nth: 2, area: 'list'}, at: 800ms, over: 100ms}
  - zoom: {to: 2, over: 100ms}
`

// waypoint is a camera.via or camera.via.insert value: point at index.
func waypoint(index int, point any) json.RawMessage {
	return typedValue(map[string]any{"index": index, "point": point})
}

// waypointIndex is a camera.via.remove value: the waypoint at index.
func waypointIndex(index int) json.RawMessage {
	return typedValue(map[string]int{"index": index})
}

// cellPoint is a cell as a waypoint's point types it.
func cellPoint(x, y int) map[string]any {
	return map[string]any{"cell": map[string]int{"x": x, "y": y}}
}

// textPoint is a target as a waypoint's point types it, matching text.
func textPoint(text string) map[string]any {
	return map[string]any{"text": text}
}

// applyVia applies edit to viaBoard and fails the test on an error.
func applyVia(t *testing.T, src string, edit Edit) string {
	t.Helper()
	got, _, err := Apply([]byte(src), viaEditPath, edit)
	if err != nil {
		t.Fatalf("Apply(%+v): %v", edit, err)
	}
	return string(got)
}

func TestApply_ViaEdit_ChangesOnlyTheEditedText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		edit     Edit
		old, new string
	}{
		{"a block cell waypoint becomes a target", cameraEdit(0, FieldCameraVia, waypoint(0, textPoint("ok"))),
			"        - {cell: {x: 2, y: 1}}\n", "        - {text: 'ok'}\n"},
		{"a block mapping waypoint becomes a cell", cameraEdit(0, FieldCameraVia, waypoint(1, cellPoint(1, 1))),
			"        - text: 'failing'\n          nth: last\n", "        - {cell: {x: 1, y: 1}}\n"},
		{"a waypoint keeps its comment", cameraEdit(0, FieldCameraVia, waypoint(2, textPoint("x"))),
			"- {cell: {x: 5, y: 3}} # the end", "- {text: 'x'} # the end"},
		{"a flow cell waypoint becomes a target", cameraEdit(1, FieldCameraVia,
			waypoint(1, map[string]any{"text": "go", "nth": "last", "area": "list"})),
			"{cell: {x: 4, y: 0}}", "{text: 'go', nth: last, area: 'list'}"},
		{"a waypoint inserted first in a block path", cameraEdit(0, FieldCameraViaInsert, waypoint(0, cellPoint(0, 0))),
			"      via:\n", "      via:\n        - {cell: {x: 0, y: 0}}\n"},
		{"a waypoint appended to a block path", cameraEdit(0, FieldCameraViaInsert, waypoint(3, textPoint("end"))),
			"        - {cell: {x: 5, y: 3}} # the end\n", "        - {cell: {x: 5, y: 3}} # the end\n        - {text: 'end'}\n"},
		{"a waypoint inserted into a flow path", cameraEdit(1, FieldCameraViaInsert, waypoint(1, cellPoint(1, 2))),
			"[{text: 'ok'}, ", "[{text: 'ok'}, {cell: {x: 1, y: 2}}, "},
		{"a waypoint appended to a flow path", cameraEdit(1, FieldCameraViaInsert, waypoint(3, textPoint("end"))),
			"{cell: {x: 6, y: 1}}]", "{cell: {x: 6, y: 1}}, {text: 'end'}]"},
		{"a block mapping waypoint removed", cameraEdit(0, FieldCameraViaRemove, waypointIndex(1)),
			"        - text: 'failing'\n          nth: last\n", ""},
		{"a flow waypoint removed", cameraEdit(1, FieldCameraViaRemove, waypointIndex(0)),
			"[{text: 'ok'}, ", "["},
		{"a block straight pan becomes a via path", cameraEdit(2, FieldCameraPath, "via"),
			"      to: {cell: {x: 3, y: 2}}\n", "      via: [{cell: {x: 10, y: 2}}, {cell: {x: 3, y: 2}}]\n"},
		{"a flow straight pan becomes a via path", cameraEdit(3, FieldCameraPath, "via"),
			"{to: {text: 'tests', nth: 2, area: 'list'}, ",
			"{via: [{cell: {x: 10, y: 2}}, {text: 'tests', nth: 2, area: 'list'}], "},
		{"a block via path becomes straight to its last waypoint", cameraEdit(0, FieldCameraPath, "to"),
			"      via:\n        - {cell: {x: 2, y: 1}}\n        - text: 'failing'\n          nth: last\n" +
				"        - {cell: {x: 5, y: 3}} # the end\n",
			"      to: {cell: {x: 5, y: 3}}\n"},
		{"a flow via path becomes straight to its last waypoint", cameraEdit(1, FieldCameraPath, "to"),
			"{via: [{text: 'ok'}, {cell: {x: 4, y: 0}}, {cell: {x: 6, y: 1}}], ", "{to: {cell: {x: 6, y: 1}}, "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if count := strings.Count(viaBoard, tc.old); count != 1 {
				t.Fatalf("the board holds %q %d times, want once", tc.old, count)
			}
			want := strings.Replace(viaBoard, tc.old, tc.new, 1)

			got := applyVia(t, viaBoard, tc.edit)

			if got != want {
				t.Errorf("Apply(%+v) =\n%s\nwant\n%s", tc.edit, got, want)
			}
		})
	}
}

func TestApply_ViaEdit_ReturnsTheStoryboardTheBytesDecodeTo(t *testing.T) {
	t.Parallel()

	_, board, err := Apply([]byte(viaBoard), viaEditPath, cameraEdit(2, FieldCameraPath, "via"))

	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	pan := board.Camera[2].Pan
	if pan.To != nil || len(pan.Via) != 2 {
		t.Fatalf("camera[2] pan = %+v, want a via path of two waypoints and no to", pan)
	}
	if first := pan.Via[0].Cell; first == nil || *first != (Cell{X: 10, Y: 2}) {
		t.Errorf("the first waypoint is %+v, want the frame's centre cell {10 2}", pan.Via[0])
	}
	if last := pan.Via[1].Cell; last == nil || *last != (Cell{X: 3, Y: 2}) {
		t.Errorf("the last waypoint is %+v, want the old to {3 2}", pan.Via[1])
	}
}

func TestApply_ViaEditThenItsInverse_LeavesTheSourceAsItWas(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		edit, inverse Edit
	}{
		{"a block straight pan made a via path and back", cameraEdit(2, FieldCameraPath, "via"),
			cameraEdit(2, FieldCameraPath, "to")},
		{"a flow straight pan made a via path and back", cameraEdit(3, FieldCameraPath, "via"),
			cameraEdit(3, FieldCameraPath, "to")},
		{"a block waypoint set and set back", cameraEdit(0, FieldCameraVia, waypoint(0, textPoint("ok"))),
			cameraEdit(0, FieldCameraVia, waypoint(0, cellPoint(2, 1)))},
		{"a flow waypoint set and set back", cameraEdit(1, FieldCameraVia, waypoint(2, cellPoint(1, 1))),
			cameraEdit(1, FieldCameraVia, waypoint(2, cellPoint(6, 1)))},
		{"a block waypoint inserted and removed", cameraEdit(0, FieldCameraViaInsert, waypoint(1, cellPoint(1, 1))),
			cameraEdit(0, FieldCameraViaRemove, waypointIndex(1))},
		{"a flow waypoint removed and inserted", cameraEdit(1, FieldCameraViaRemove, waypointIndex(0)),
			cameraEdit(1, FieldCameraViaInsert, waypoint(0, textPoint("ok")))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			edited := applyVia(t, viaBoard, tc.edit)
			got := applyVia(t, edited, tc.inverse)

			if edited == viaBoard {
				t.Fatalf("Apply(%+v) left the source as it is", tc.edit)
			}
			if got != viaBoard {
				t.Errorf("Apply(%+v) then Apply(%+v) =\n%s\nwant\n%s", tc.edit, tc.inverse, got, viaBoard)
			}
		})
	}
}

func TestApply_PathThePanFollows_LeavesTheSourceAsItIs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		edit Edit
	}{
		{"via on a via path", cameraEdit(0, FieldCameraPath, "via")},
		{"to on a straight pan", cameraEdit(3, FieldCameraPath, "to")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := applyVia(t, viaBoard, tc.edit); got != viaBoard {
				t.Errorf("Apply(%+v) changed the source:\n%s", tc.edit, got)
			}
		})
	}
}

func TestApply_ViaEditFailingValidation_IsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		board   string
		edit    Edit
		message string
	}{
		{"a remove leaving one waypoint", cameraCaptionsBoard, cameraEdit(1, FieldCameraViaRemove, waypointIndex(0)),
			"want at least 2 points, got 1"},
		{"a waypoint outside the frame", viaBoard, cameraEdit(0, FieldCameraVia, waypoint(0, cellPoint(30, 0))),
			"pan.via[0].cell: want x in [0, 20)"},
		{"an inserted waypoint outside the frame", viaBoard, cameraEdit(1, FieldCameraViaInsert, waypoint(0, cellPoint(0, 9))),
			"pan.via[0].cell: want x in [0, 20) and y in [0, 4)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := Apply([]byte(tc.board), viaEditPath, tc.edit)

			var validation *ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("Apply(%+v) = %v, want a *ValidationError", tc.edit, err)
			}
			if !strings.Contains(err.Error(), tc.message) {
				t.Errorf("Apply(%+v) = %q, want it to contain %q", tc.edit, err, tc.message)
			}
		})
	}
}

func TestApply_ViaEditOnWhatIsNotThere_IsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		edit    Edit
		message string
	}{
		{"a waypoint of a straight pan", cameraEdit(2, FieldCameraVia, waypoint(0, cellPoint(1, 1))),
			"camera[2]: camera.via: the pan moves straight to its to; make it a via path with camera.path first"},
		{"an insert into a straight pan", cameraEdit(3, FieldCameraViaInsert, waypoint(0, cellPoint(1, 1))),
			"camera[3]: camera.via.insert: the pan moves straight to its to"},
		{"a remove from a straight pan", cameraEdit(3, FieldCameraViaRemove, waypointIndex(0)),
			"camera[3]: camera.via.remove: the pan moves straight to its to"},
		{"the path of a zoom", cameraEdit(4, FieldCameraPath, "via"),
			"camera[4]: camera.path: a zoom move has no path"},
		{"a waypoint of a zoom", cameraEdit(4, FieldCameraVia, waypoint(0, cellPoint(1, 1))),
			"camera[4]: camera.via: a zoom move has no path"},
		{"a waypoint past the last", cameraEdit(0, FieldCameraVia, waypoint(3, cellPoint(1, 1))),
			"camera[0]: camera.via: the pan's via path has no waypoint at index 3"},
		{"a negative waypoint index", cameraEdit(1, FieldCameraViaRemove, waypointIndex(-1)),
			"camera[1]: camera.via.remove: the pan's via path has no waypoint at index -1"},
		{"a remove past the last", cameraEdit(1, FieldCameraViaRemove, waypointIndex(3)),
			"camera[1]: camera.via.remove: the pan's via path has no waypoint at index 3"},
		{"an insert past the end", cameraEdit(0, FieldCameraViaInsert, waypoint(4, cellPoint(1, 1))),
			"camera[0]: camera.via.insert: want an index from 0 to 3 for the new waypoint, got 4"},
		{"a negative insert index", cameraEdit(0, FieldCameraViaInsert, waypoint(-1, cellPoint(1, 1))),
			"camera[0]: camera.via.insert: want an index from 0 to 3 for the new waypoint, got -1"},
		{"a move past the last", cameraEdit(9, FieldCameraPath, "via"),
			"camera[9]: camera.path: the storyboard has no camera move at index 9"},
		{"the destination of a via pan", cameraEdit(0, FieldCameraTo, cellPoint(1, 1)),
			"camera[0]: camera.to: the pan follows a via path: set its waypoints with camera.via"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := Apply([]byte(viaBoard), viaEditPath, tc.edit)

			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Errorf("Apply(%+v) = %v, want an error containing %q", tc.edit, err, tc.message)
			}
		})
	}
}

func TestApply_ViaWireSpellings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		edit       Edit
		isAccepted bool
	}{
		{"a waypoint", cameraEdit(0, FieldCameraVia, json.RawMessage(`{"index": 0, "point": {"cell": {"x": 1, "y": 1}}}`)), true},
		{"a waypoint without an index", cameraEdit(0, FieldCameraVia, json.RawMessage(`{"point": {"cell": {"x": 1, "y": 1}}}`)), false},
		{"a waypoint without a point", cameraEdit(0, FieldCameraVia, json.RawMessage(`{"index": 0}`)), false},
		{"a waypoint with an unknown key", cameraEdit(0, FieldCameraVia, json.RawMessage(`{"index": 0, "point": {"text": "ok"}, "bogus": 1}`)), false},
		{"a waypoint with a fractional index", cameraEdit(0, FieldCameraVia, json.RawMessage(`{"index": 0.5, "point": {"text": "ok"}}`)), false},
		{"a point with a target and a cell", cameraEdit(0, FieldCameraVia, json.RawMessage(`{"index": 0, "point": {"text": "ok", "cell": {"x": 1, "y": 1}}}`)), false},
		{"a point with an unknown key", cameraEdit(0, FieldCameraViaInsert, json.RawMessage(`{"index": 0, "point": {"spot": 1}}`)), false},
		{"a bare point", cameraEdit(0, FieldCameraViaInsert, json.RawMessage(`{"cell": {"x": 1, "y": 1}}`)), false},
		{"a remove", cameraEdit(0, FieldCameraViaRemove, json.RawMessage(`{"index": 0}`)), true},
		{"a remove with a point", cameraEdit(0, FieldCameraViaRemove, json.RawMessage(`{"index": 0, "point": {"text": "ok"}}`)), false},
		{"a bare remove index", cameraEdit(0, FieldCameraViaRemove, json.RawMessage(`0`)), false},
		{"a null remove", cameraEdit(0, FieldCameraViaRemove, nil), false},
		{"a null waypoint", cameraEdit(0, FieldCameraVia, nil), false},
		{"a path of to", cameraEdit(0, FieldCameraPath, json.RawMessage(`"to"`)), true},
		{"a path of another spelling", cameraEdit(0, FieldCameraPath, json.RawMessage(`"curve"`)), false},
		{"a path as a number", cameraEdit(0, FieldCameraPath, json.RawMessage(`1`)), false},
		{"a null path", cameraEdit(0, FieldCameraPath, nil), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := Apply([]byte(viaBoard), viaEditPath, tc.edit)

			if tc.isAccepted && err != nil {
				t.Errorf("Apply(%+v): %v", tc.edit, err)
			}
			if !tc.isAccepted && err == nil {
				t.Errorf("Apply(%+v) was accepted, want it refused", tc.edit)
			}
		})
	}
}
