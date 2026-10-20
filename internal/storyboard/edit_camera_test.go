package storyboard

import (
	"errors"
	"strings"
	"testing"
)

// cameraEditPath is the name the camera edit tests parse their storyboard under.
const cameraEditPath = "camera.yaml"

// positionEdit is an Edit of the camera position at index, its value typed as value encodes.
func positionEdit(index int, field string, value any) Edit {
	owner := Owner{Kind: OwnerPosition, Index: index}
	return Edit{Owner: owner, Field: field, Value: typedValue(value)}
}

// zoomEdit is an Edit of the camera zoom at index, its value typed as value encodes.
func zoomEdit(index int, field string, value any) Edit {
	owner := Owner{Kind: OwnerZoom, Index: index}
	return Edit{Owner: owner, Field: field, Value: typedValue(value)}
}

// spanValue is a position.span value: its at a moment, beat 0 for a clip time, and its hold.
func spanValue(beat int, offsetMS, holdMS int64) map[string]any {
	return map[string]any{"at": moment(beat, offsetMS), "hold_ms": holdMS}
}

// cell is a position.cell value.
func cell(x, y int) map[string]int {
	return map[string]int{"x": x, "y": y}
}

func TestApply_CameraEdit_ChangesOnlyTheEditedText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		edit     Edit
		old, new string
	}{
		{"a block at keeps its comment", positionEdit(0, FieldPositionAt, moment(0, 100)),
			"at: 200ms # after", "at: 100ms # after"},
		{"a flow at moves to a beat", positionEdit(1, FieldPositionAt, moment(2, 100)),
			"at: 900ms}", "at: {beat: 2, offset: 100ms}}"},
		{"a hold keeps its unit", positionEdit(0, FieldPositionHold, 500), "hold: 600ms", "hold: 500ms"},
		{"a hold of 0 deletes the key", positionEdit(0, FieldPositionHold, 0), "      hold: 600ms\n", ""},
		{"a new hold goes after at", positionEdit(1, FieldPositionHold, 300),
			"at: 900ms}", "at: 900ms, hold: 300ms}"},
		{"a span writes at and hold", positionEdit(0, FieldPositionSpan, spanValue(0, 100, 500)),
			"at: 200ms # after the zoom starts\n      hold: 600ms",
			"at: 100ms # after the zoom starts\n      hold: 500ms"},
		{"a span of no hold deletes the key", positionEdit(0, FieldPositionSpan, spanValue(1, 300, 0)),
			"at: 200ms # after the zoom starts\n      hold: 600ms\n",
			"at: {beat: 1, offset: 300ms} # after the zoom starts\n"},
		{"a new block ease goes after hold", positionEdit(0, FieldPositionEase, "linear"),
			"      hold: 600ms\n", "      hold: 600ms\n      ease: linear\n"},
		{"a new flow ease goes after at", positionEdit(1, FieldPositionEase, "ease-in"),
			"at: 900ms}", "at: 900ms, ease: ease-in}"},
		{"a cell is set", positionEdit(1, FieldPositionCell, cell(5, 1)),
			"{cell: {x: 3, y: 2},", "{cell: {x: 5, y: 1},"},
		{"a cell replaces a block text", positionEdit(0, FieldPositionCell, cell(1, 1)),
			"    - text: 'failing'\n      at:", "    - cell: {x: 1, y: 1}\n      at:"},
		{"a target replaces a flow cell", positionEdit(1, FieldPositionTarget,
			map[string]any{"text": "ok", "nth": 2, "area": "list"}),
			"{cell: {x: 3, y: 2}, at: 900ms}", "{text: 'ok', nth: 2, area: 'list', at: 900ms}"},
		{"a target's text is set", positionEdit(0, FieldPositionTarget, map[string]any{"text": "it's"}),
			"text: 'failing'", "text: 'it''s'"},
		{"a target's last nth goes after its text", positionEdit(0, FieldPositionTarget,
			map[string]any{"text": "failing", "nth": "last"}),
			"text: 'failing'\n", "text: 'failing'\n      nth: last\n"},
		{"a block at becomes a clip time", zoomEdit(0, FieldZoomAt, moment(0, 150)),
			"      at:\n        beat: 1\n        offset: 100ms\n", "      at: 150ms\n"},
		{"a new at goes after to", zoomEdit(2, FieldZoomAt, moment(0, 1000)),
			"{to: 1.2, over: 50ms}", "{to: 1.2, at: 1000ms, over: 50ms}"},
		{"over keeps its unit", zoomEdit(1, FieldZoomOver, 350), "over: 400ms, ease", "over: 350ms, ease"},
		{"smooth deletes an ease", zoomEdit(0, FieldZoomEase, "smooth"), "      ease: linear\n", ""},
		{"a new ease goes after over", zoomEdit(2, FieldZoomEase, "ease-out"),
			"over: 50ms}", "over: 50ms, ease: ease-out}"},
		{"to is set", zoomEdit(1, FieldZoomTo, 2.5), "{to: 2,", "{to: 2.5,"},
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

func TestApply_PointEditOnAOneKeyPosition_WritesTheNewKeyFirst(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		position string
		edit     Edit
		want     string
	}{
		{"a cell replaces the only text", "    - text: ok\n",
			positionEdit(0, FieldPositionCell, cell(1, 1)), "    - cell: {x: 1, y: 1}\n"},
		{"a target replaces the only cell", "    - cell: {x: 1, y: 1}\n",
			positionEdit(0, FieldPositionTarget, map[string]any{"text": "ok"}), "    - text: 'ok'\n"},
		{"a target drops the nth and area it leaves out", "    - {text: ok, nth: 2, area: list}\n",
			positionEdit(0, FieldPositionTarget, map[string]any{"text": "ok"}), "    - {text: 'ok'}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			board := editBaseBoard + "camera:\n  positions:\n" + tc.position

			got, _, err := Apply([]byte(board), cameraEditPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if want := editBaseBoard + "camera:\n  positions:\n" + tc.want; string(got) != want {
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
		{"a hold of 0", positionEdit(1, FieldPositionHold, 0)},
		{"a position's smooth ease", positionEdit(1, FieldPositionEase, "smooth")},
		{"a span at the position's start and no hold", positionEdit(1, FieldPositionSpan, spanValue(0, 900, 0))},
		{"a zoom's smooth ease", zoomEdit(2, FieldZoomEase, "smooth")},
		{"a zoom's clip start", zoomEdit(2, FieldZoomAt, moment(0, 0))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, _, err := Apply([]byte(cameraEditBoard), cameraEditPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if string(got) != cameraEditBoard {
				t.Errorf("Apply(%+v) =\n%s\nwant the source as it is", tc.edit, got)
			}
		})
	}
}

func TestApply_CameraEditFailingValidation_IsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		edit    Edit
		message string
	}{
		{"a start inside the earlier position", positionEdit(1, FieldPositionAt, moment(0, 500)),
			"camera.positions[1]: overlaps camera.positions[0]"},
		{"a hold over the later position", positionEdit(0, FieldPositionHold, 800),
			"camera.positions[1]: overlaps camera.positions[0]"},
		{"a span over the later position", positionEdit(0, FieldPositionSpan, spanValue(0, 900, 100)),
			"camera.positions[1]: overlaps camera.positions[0]"},
		{"a hold past the clip's end", positionEdit(1, FieldPositionHold, 2000),
			"hold: want the position to end by the clip's end"},
		{"a cell outside the frame", positionEdit(1, FieldPositionCell, cell(20, 0)),
			"cell: want x in [0, 20)"},
		{"a start inside another zoom", zoomEdit(2, FieldZoomAt, moment(1, 600)),
			"camera.zooms[2]: overlaps camera.zooms[1]"},
		{"a factor past 3", zoomEdit(1, FieldZoomTo, 4), "to: want in [1, 3]"},
		{"no length", zoomEdit(1, FieldZoomOver, 0), "over: want >0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := Apply([]byte(cameraEditBoard), cameraEditPath, tc.edit)

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

func TestApply_CameraEditOnWhatIsNotThere_IsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		board   string
		edit    Edit
		message string
	}{
		{"an index past the last position", cameraEditBoard, positionEdit(2, FieldPositionHold, 100),
			"camera.positions[2]: position.hold: the storyboard has no camera position at index 2"},
		{"a negative position index", cameraEditBoard, positionEdit(-1, FieldPositionAt, moment(0, 0)),
			"camera.positions[-1]: position.at: the storyboard has no camera position at index -1"},
		{"an index past the last zoom", cameraEditBoard, zoomEdit(3, FieldZoomTo, 2),
			"camera.zooms[3]: zoom.to: the storyboard has no camera zoom at index 3"},
		{"a storyboard without a camera", editBaseBoard, zoomEdit(0, FieldZoomOver, 100),
			"camera.zooms[0]: zoom.over: the storyboard has no camera zoom at index 0"},
		{"a position field on a zoom", cameraEditBoard, zoomEdit(0, FieldPositionHold, 100),
			`camera.zooms[0]: position.hold: want a position owner, got "zoom"`},
		{"a zoom field on a position", cameraEditBoard, positionEdit(0, FieldZoomTo, 2),
			`camera.positions[0]: zoom.to: want a zoom owner, got "position"`},
		{"a zoom field on a caption", cameraEditBoard, captionEdit(0, FieldZoomEase, "linear"),
			`captions[0]: zoom.ease: want a zoom owner, got "caption"`},
		{"an unknown position field", cameraEditBoard, positionEdit(0, "position.to", 2),
			"camera.positions[0]: position.to: unknown field"},
		{"a null position at", cameraEditBoard, positionEdit(0, FieldPositionAt, nil),
			"camera.positions[0]: position.at: want a value, got null"},
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
		{"a moment", positionEdit(1, FieldPositionAt, moment(1, 950)), true},
		{"a moment without an offset", positionEdit(1, FieldPositionAt, map[string]int{"beat": 1}), false},
		{"a duration with a unit", positionEdit(1, FieldPositionAt, "900ms"), false},
		{"a hold in milliseconds", positionEdit(1, FieldPositionHold, 100), true},
		{"a hold as a string", positionEdit(1, FieldPositionHold, "100"), false},
		{"a negative hold", positionEdit(1, FieldPositionHold, -5), false},
		{"a fractional hold", positionEdit(1, FieldPositionHold, 10.5), false},
		{"a span", positionEdit(1, FieldPositionSpan, spanValue(0, 1000, 200)), true},
		{"a span without a hold", positionEdit(1, FieldPositionSpan, map[string]any{"at": moment(0, 1000)}), false},
		{"a span without an at", positionEdit(1, FieldPositionSpan, map[string]any{"hold_ms": 200}), false},
		{"a span with another key", positionEdit(1, FieldPositionSpan,
			map[string]any{"at": moment(0, 1000), "hold_ms": 200, "ease": "linear"}), false},
		{"a span of a negative hold", positionEdit(1, FieldPositionSpan, spanValue(0, 1000, -1)), false},
		{"a span of a malformed at", positionEdit(1, FieldPositionSpan,
			map[string]any{"at": map[string]int{"beat": 1}, "hold_ms": 200}), false},
		{"every ease", positionEdit(1, FieldPositionEase, "ease-out"), true},
		{"a capitalised ease", positionEdit(1, FieldPositionEase, "Linear"), false},
		{"an underscored ease", positionEdit(1, FieldPositionEase, "ease_in"), false},
		{"an ease as a number", zoomEdit(0, FieldZoomEase, 1), false},
		{"a cell", positionEdit(1, FieldPositionCell, cell(0, 0)), true},
		{"a cell without a row", positionEdit(1, FieldPositionCell, map[string]int{"x": 1}), false},
		{"a cell with another key", positionEdit(1, FieldPositionCell, map[string]int{"x": 1, "y": 1, "z": 0}), false},
		{"a cell as a point", positionEdit(1, FieldPositionCell, map[string]any{"cell": cell(1, 1)}), false},
		{"a fractional cell", positionEdit(1, FieldPositionCell, map[string]float64{"x": 1.5, "y": 1}), false},
		{"a target", positionEdit(1, FieldPositionTarget, map[string]any{"text": "ok", "nth": "last", "area": "list"}), true},
		{"a target without text", positionEdit(1, FieldPositionTarget, map[string]any{"text": ""}), false},
		{"a target's first nth", positionEdit(1, FieldPositionTarget, map[string]any{"text": "ok", "nth": "first"}), false},
		{"a target's nth of 0", positionEdit(1, FieldPositionTarget, map[string]any{"text": "ok", "nth": 0}), false},
		{"a target with a cell", positionEdit(1, FieldPositionTarget, map[string]any{"text": "ok", "cell": cell(1, 1)}), false},
		{"a target as a string", positionEdit(1, FieldPositionTarget, "ok"), false},
		{"a zoom over in milliseconds", zoomEdit(2, FieldZoomOver, 80), true},
		{"a negative zoom over", zoomEdit(2, FieldZoomOver, -1), false},
		{"a zoom factor", zoomEdit(2, FieldZoomTo, 2.5), true},
		{"a zoom factor as a string", zoomEdit(2, FieldZoomTo, "2"), false},
		{"a negative beat", zoomEdit(2, FieldZoomAt, moment(-1, 0)), false},
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
