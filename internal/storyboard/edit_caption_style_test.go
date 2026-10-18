package storyboard

import (
	"reflect"
	"strings"
	"testing"
)

// captionStyleBoard is editBaseBoard, its frame.font_size 10, with captions styled in block and
// flow mappings: a block caption with block font, box and fade, a flow caption with flow ones, a
// block and a flow caption without style, and a block caption whose fade holds one key.
const captionStyleBoard = editBaseBoard + `captions:
  - text: Block
    at: 100ms
    for: 1s
    font:
      size: 18
      weight: bold
    color: '#FFCC00'
    box:
      color: '#202020'
      opacity: 0.8
    fade:
      in: 200ms # eased in
      out: 0.3s
  - {text: 'Flow', at: 1200ms, for: 800ms, font: {size: 12}, box: {opacity: 0.4}, fade: {in: 100ms}}
  - text: Plain
    at: 2s
    for: 400ms
  - {text: 'Bare', at: 300ms, for: 500ms}
  - text: Single
    at: 1500ms
    for: 500ms
    fade:
      out: 100ms
`

func TestApply_CaptionStyleEdit_ChangesOnlyTheEditedText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		edit     Edit
		old, new string
	}{
		{"a block size is changed", captionEdit(0, FieldCaptionFontSize, 20), "size: 18", "size: 20"},
		{"a size at frame.font_size deletes its key", captionEdit(0, FieldCaptionFontSize, 10),
			"      size: 18\n", ""},
		{"a flow size at frame.font_size deletes the emptied font", captionEdit(1, FieldCaptionFontSize, 10),
			", font: {size: 12}", ""},
		{"a size creates an absent block font", captionEdit(2, FieldCaptionFontSize, 16.5),
			"    for: 400ms\n", "    for: 400ms\n    font: {size: 16.5}\n"},
		{"regular deletes a weight", captionEdit(0, FieldCaptionFontWeight, "regular"), "      weight: bold\n", ""},
		{"a weight goes after a flow size", captionEdit(1, FieldCaptionFontWeight, "bold"),
			"font: {size: 12}", "font: {size: 12, weight: bold}"},
		{"a colour is changed, quoted", captionEdit(0, FieldCaptionColor, "#00ff00"),
			"color: '#FFCC00'", "color: '#00ff00'"},
		{"white deletes a colour", captionEdit(0, FieldCaptionColor, "#ffffff"), "    color: '#FFCC00'\n", ""},
		{"white in capitals deletes a colour", captionEdit(0, FieldCaptionColor, "#FFFFFF"),
			"    color: '#FFCC00'\n", ""},
		{"a colour goes after a flow font", captionEdit(1, FieldCaptionColor, "#00ff00"),
			"font: {size: 12}, box", "font: {size: 12}, color: '#00ff00', box"},
		{"a block box colour is changed", captionEdit(0, FieldCaptionBoxColor, "#112233"),
			"color: '#202020'", "color: '#112233'"},
		{"black deletes a box colour", captionEdit(0, FieldCaptionBoxColor, "#000000"),
			"      color: '#202020'\n", ""},
		{"a box colour goes before a flow opacity", captionEdit(1, FieldCaptionBoxColor, "#112233"),
			"box: {opacity: 0.4}", "box: {color: '#112233', opacity: 0.4}"},
		{"a box colour creates an absent flow box", captionEdit(3, FieldCaptionBoxColor, "#112233"),
			"for: 500ms}", "for: 500ms, box: {color: '#112233'}}"},
		{"a block opacity is changed", captionEdit(0, FieldCaptionBoxOpacity, 1), "opacity: 0.8", "opacity: 1"},
		{"0.6 deletes an opacity", captionEdit(0, FieldCaptionBoxOpacity, 0.6), "      opacity: 0.8\n", ""},
		{"0.6 deletes the emptied flow box", captionEdit(1, FieldCaptionBoxOpacity, 0.6), ", box: {opacity: 0.4}", ""},
		{"an opacity creates an absent block box", captionEdit(2, FieldCaptionBoxOpacity, 0),
			"    for: 400ms\n", "    for: 400ms\n    box: {opacity: 0}\n"},
		{"a fade in keeps its unit and comment", captionEdit(0, FieldCaptionFadeIn, 250),
			"in: 200ms # eased in", "in: 250ms # eased in"},
		{"a fade out keeps its seconds", captionEdit(0, FieldCaptionFadeOut, 450), "out: 0.3s", "out: 0.45s"},
		{"0 deletes a fade in", captionEdit(0, FieldCaptionFadeIn, 0), "      in: 200ms # eased in\n", ""},
		{"0 deletes the emptied flow fade", captionEdit(1, FieldCaptionFadeIn, 0), ", fade: {in: 100ms}", ""},
		{"0 deletes the emptied block fade", captionEdit(4, FieldCaptionFadeOut, 0),
			"    fade:\n      out: 100ms\n", ""},
		{"a fade out goes after a flow fade in", captionEdit(1, FieldCaptionFadeOut, 50),
			"fade: {in: 100ms}", "fade: {in: 100ms, out: 50ms}"},
		{"a fade creates an absent flow fade", captionEdit(3, FieldCaptionFadeIn, 100),
			"for: 500ms}", "for: 500ms, fade: {in: 100ms}}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if count := strings.Count(captionStyleBoard, tc.old); count != 1 {
				t.Fatalf("the board holds %q %d times, want once", tc.old, count)
			}
			want := strings.Replace(captionStyleBoard, tc.old, tc.new, 1)

			got, _, err := Apply([]byte(captionStyleBoard), captionEditPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if string(got) != want {
				t.Errorf("Apply(%+v) =\n%s\nwant\n%s", tc.edit, got, want)
			}
		})
	}
}

func TestApply_CaptionStyleEdit_ReadsBackAsTheValue(t *testing.T) {
	t.Parallel()

	_, board, err := Apply([]byte(captionStyleBoard), captionEditPath, captionEdit(3, FieldCaptionColor, "#102030"))

	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got, want := board.Captions[3].TextColor(), (RGB{R: 0x10, G: 0x20, B: 0x30}); got != want {
		t.Errorf("the caption's text colour is %+v, want %+v", got, want)
	}
}

func TestApply_CaptionStyleEditToWhatAnAbsentKeyMeans_LeavesTheSourceAsItIs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		edit Edit
	}{
		{"frame.font_size on a caption without font", captionEdit(2, FieldCaptionFontSize, 10)},
		{"regular on a caption without font", captionEdit(3, FieldCaptionFontWeight, "regular")},
		{"white on a caption without colour", captionEdit(2, FieldCaptionColor, "#FFFFFF")},
		{"black on a caption without box", captionEdit(3, FieldCaptionBoxColor, "#000000")},
		{"0.6 on a caption without box", captionEdit(2, FieldCaptionBoxOpacity, 0.6)},
		{"0 on a caption without fade", captionEdit(3, FieldCaptionFadeIn, 0)},
		{"0 on a fade without the key", captionEdit(4, FieldCaptionFadeIn, 0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, _, err := Apply([]byte(captionStyleBoard), captionEditPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if string(got) != captionStyleBoard {
				t.Errorf("Apply(%+v) changed the source:\n%s", tc.edit, got)
			}
		})
	}
}

func TestApply_CaptionStyleEditOnAnEmptyMapping(t *testing.T) {
	t.Parallel()
	const emptyFontBoard = editBaseBoard + `captions:
  - text: Empty
    at: 100ms
    for: 1s
    font:
`
	cases := []struct {
		name string
		edit Edit
		want string
	}{
		{"a size replaces it", captionEdit(0, FieldCaptionFontSize, 20), "    font: {size: 20}\n"},
		{"a default deletes it", captionEdit(0, FieldCaptionFontSize, 10), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := strings.Replace(emptyFontBoard, "    font:\n", tc.want, 1)

			got, _, err := Apply([]byte(emptyFontBoard), captionEditPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if string(got) != want {
				t.Errorf("Apply(%+v) =\n%s\nwant\n%s", tc.edit, got, want)
			}
		})
	}
}

func TestApply_CaptionStyleEdit_IsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		edit    Edit
		message string
	}{
		{"a size past the caption bounds", captionEdit(0, FieldCaptionFontSize, 80),
			"font.size: want in [6, 72], got 80"},
		{"a size of 0", captionEdit(0, FieldCaptionFontSize, 0),
			"captions[0]: caption.font.size: want a size above 0"},
		{"a size as text", captionEdit(0, FieldCaptionFontSize, "18"),
			"captions[0]: caption.font.size: want a size above 0"},
		{"an opacity past 1", captionEdit(1, FieldCaptionBoxOpacity, 1.5), "box.opacity: want in [0, 1], got 1.5"},
		{"an opacity as text", captionEdit(1, FieldCaptionBoxOpacity, "0.5"),
			"captions[1]: caption.box.opacity: want an opacity from 0 to 1"},
		{"a fade longer than the caption", captionEdit(1, FieldCaptionFadeIn, 900),
			"fade: want in + out within the caption's length 800ms"},
		{"a negative fade", captionEdit(1, FieldCaptionFadeOut, -100),
			"captions[1]: caption.fade.out: want a whole number of milliseconds"},
		{"an unknown weight", captionEdit(0, FieldCaptionFontWeight, "heavy"),
			`captions[0]: caption.font.weight: want regular or bold, got "heavy"`},
		{"a colour not spelled #rrggbb", captionEdit(0, FieldCaptionColor, "red"), `color: want #rrggbb, got "red"`},
		{"a box colour as a number", captionEdit(0, FieldCaptionBoxColor, 0),
			"captions[0]: caption.box.color: want a colour"},
		{"a null colour", captionEdit(0, FieldCaptionColor, nil), "captions[0]: caption.color: want a value, got null"},
		{"a caption that is not there", captionEdit(9, FieldCaptionFadeIn, 100),
			"captions[9]: caption.fade.in: the storyboard has no caption at index 9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := Apply([]byte(captionStyleBoard), captionEditPath, tc.edit)

			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Errorf("Apply(%+v) = %v, want an error containing %q", tc.edit, err, tc.message)
			}
		})
	}
}

// captionPlacementBoard is editBaseBoard with captions placed in block and flow mappings: a
// block caption with place, align and a block offset, a flow caption with align and an offset,
// a block and a flow caption set by position, and a flow caption with a place alone.
const captionPlacementBoard = editBaseBoard + `captions:
  - text: Placed
    at: 100ms
    for: 1s
    place: top
    align: left
    offset:
      x: 8
      y: -4
    color: '#ffcc00'
  - {text: 'Nudged', at: 1200ms, for: 800ms, align: right, offset: {x: 2, y: 3}}
  - text: Pinned
    at: 2s
    for: 400ms
    position: {x: 40, y: 300} # top-left
  - {text: 'Fixed', at: 300ms, for: 500ms, position: {x: 10, y: 20}, color: '#ffcc00'}
  - {text: 'Middle', at: 1500ms, for: 300ms, place: middle, color: '#ffcc00'}
`

// pixels is design pixels as an [Edit] types them.
func pixels(x, y int) map[string]int {
	return map[string]int{"x": x, "y": y}
}

// swap is one change an edit makes to a storyboard's text: old, found once, becomes new.
type swap struct {
	old, new string
}

func TestApply_CaptionPlacementEdit_ChangesOnlyTheEditedText(t *testing.T) {
	t.Parallel()
	positionLine := "    position: {x: 40, y: 300} # top-left\n"
	cases := []struct {
		name  string
		edit  Edit
		swaps []swap
	}{
		{"a block offset becomes a flow one", captionEdit(0, FieldCaptionOffset, pixels(5, 6)),
			[]swap{{"    offset:\n      x: 8\n      y: -4\n", "    offset: {x: 5, y: 6}\n"}}},
		{"a flow offset is changed", captionEdit(1, FieldCaptionOffset, pixels(-2, 0)),
			[]swap{{"offset: {x: 2, y: 3}", "offset: {x: -2, y: 0}"}}},
		{"0, 0 deletes a block offset", captionEdit(0, FieldCaptionOffset, pixels(0, 0)),
			[]swap{{"    offset:\n      x: 8\n      y: -4\n", ""}}},
		{"0, 0 deletes a flow offset", captionEdit(1, FieldCaptionOffset, pixels(0, 0)),
			[]swap{{", offset: {x: 2, y: 3}", ""}}},
		{"an offset goes after a flow place", captionEdit(4, FieldCaptionOffset, pixels(1, 2)),
			[]swap{{"place: middle, color", "place: middle, offset: {x: 1, y: 2}, color"}}},
		{"an offset replaces a block position", captionEdit(2, FieldCaptionOffset, pixels(1, -1)),
			[]swap{{positionLine, "    offset: {x: 1, y: -1}\n"}}},
		{"0, 0 deletes a flow position", captionEdit(3, FieldCaptionOffset, pixels(0, 0)),
			[]swap{{", position: {x: 10, y: 20}", ""}}},
		{"a place replaces a block position", captionEdit(2, FieldCaptionPlace, "top"),
			[]swap{{positionLine, "    place: top\n"}}},
		{"bottom deletes a flow position", captionEdit(3, FieldCaptionPlace, "bottom"),
			[]swap{{", position: {x: 10, y: 20}", ""}}},
		{"an align replaces a flow position", captionEdit(3, FieldCaptionAlign, "right"),
			[]swap{{", position: {x: 10, y: 20}", ", align: right"}}},
		{"middle deletes a block position", captionEdit(2, FieldCaptionAlign, "middle"),
			[]swap{{positionLine, ""}}},
		{"a position replaces a block place, align and offset", captionEdit(0, FieldCaptionPosition, pixels(40, 50)),
			[]swap{{"    place: top\n    align: left\n    offset:\n      x: 8\n      y: -4\n", "    position: {x: 40, y: 50}\n"}}},
		{"a position replaces a flow align and offset", captionEdit(1, FieldCaptionPosition, pixels(4, 5)),
			[]swap{{", align: right, offset: {x: 2, y: 3}", ", position: {x: 4, y: 5}"}}},
		{"a position replaces a flow place", captionEdit(4, FieldCaptionPosition, pixels(1, 2)),
			[]swap{{", place: middle", ", position: {x: 1, y: 2}"}}},
		{"a block position is changed, keeping its comment", captionEdit(2, FieldCaptionPosition, pixels(0, 0)),
			[]swap{{"{x: 40, y: 300} # top-left", "{x: 0, y: 0} # top-left"}}},
		{"a flow position is changed", captionEdit(3, FieldCaptionPosition, pixels(-5, 20)),
			[]swap{{"{x: 10, y: 20}", "{x: -5, y: 20}"}}},
		{"null deletes a block position", captionEdit(2, FieldCaptionPosition, nil), []swap{{positionLine, ""}}},
		{"null deletes a flow position", captionEdit(3, FieldCaptionPosition, nil),
			[]swap{{", position: {x: 10, y: 20}", ""}}},
		{"null on a caption without position leaves it as it is", captionEdit(0, FieldCaptionPosition, nil), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := captionPlacementBoard
			for _, change := range tc.swaps {
				if count := strings.Count(want, change.old); count != 1 {
					t.Fatalf("the board holds %q %d times, want once", change.old, count)
				}
				want = strings.Replace(want, change.old, change.new, 1)
			}

			got, _, err := Apply([]byte(captionPlacementBoard), captionEditPath, tc.edit)

			if err != nil {
				t.Fatalf("Apply(%+v): %v", tc.edit, err)
			}
			if string(got) != want {
				t.Errorf("Apply(%+v) =\n%s\nwant\n%s", tc.edit, got, want)
			}
		})
	}
}

// placement is where a loaded caption sits: its place, align, offset and position.
type placement struct {
	place    Place
	align    Align
	offset   Pixels
	position *Pixels
}

// placementOf reads caption's placement.
func placementOf(caption Caption) placement {
	return placement{caption.Place, caption.Align, caption.Offset, caption.Position}
}

func TestApply_CaptionPlacementEdits_SwitchToPositionAndBack(t *testing.T) {
	t.Parallel()
	positioned := func(x, y int) placement { return placement{position: &Pixels{X: x, Y: y}} }
	cases := []struct {
		name  string
		edits []Edit
		index int
		want  placement
	}{
		{"a block caption to position", []Edit{captionEdit(0, FieldCaptionPosition, pixels(40, 50))}, 0,
			positioned(40, 50)},
		{"a flow caption to position", []Edit{captionEdit(1, FieldCaptionPosition, pixels(4, 5))}, 1,
			positioned(4, 5)},
		{"a block caption to position and back by null", []Edit{
			captionEdit(0, FieldCaptionPosition, pixels(40, 50)), captionEdit(0, FieldCaptionPosition, nil),
		}, 0, placement{place: PlaceBottom, align: AlignMiddle}},
		{"a flow caption to position and back by a place", []Edit{
			captionEdit(1, FieldCaptionPosition, pixels(4, 5)), captionEdit(1, FieldCaptionPlace, "top"),
		}, 1, placement{place: PlaceTop, align: AlignMiddle}},
		{"a positioned block caption to an offset", []Edit{captionEdit(2, FieldCaptionOffset, pixels(3, 4))}, 2,
			placement{place: PlaceBottom, align: AlignMiddle, offset: Pixels{X: 3, Y: 4}}},
		{"a positioned flow caption to an align and back by a position", []Edit{
			captionEdit(3, FieldCaptionAlign, "left"), captionEdit(3, FieldCaptionPosition, pixels(7, 8)),
		}, 3, positioned(7, 8)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := []byte(captionPlacementBoard)
			var board *Storyboard
			var err error

			for _, edit := range tc.edits {
				if src, board, err = Apply(src, captionEditPath, edit); err != nil {
					t.Fatalf("Apply(%+v): %v", edit, err)
				}
			}

			if got := placementOf(board.Captions[tc.index]); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("captions[%d] placed %+v, want %+v", tc.index, got, tc.want)
			}
		})
	}
}

func TestApply_CaptionPlacementEdit_IsRefused(t *testing.T) {
	t.Parallel()
	const wantPixels = "want whole design pixels"
	cases := []struct {
		name    string
		edit    Edit
		message string
	}{
		{"an offset as text", captionEdit(0, FieldCaptionOffset, "8, -4"), "captions[0]: caption.offset: " + wantPixels},
		{"an offset without y", captionEdit(0, FieldCaptionOffset, map[string]int{"x": 1}),
			"captions[0]: caption.offset: " + wantPixels},
		{"an offset of a fraction", captionEdit(0, FieldCaptionOffset, map[string]float64{"x": 1.5, "y": 0}),
			"captions[0]: caption.offset: " + wantPixels},
		{"an offset with another key", captionEdit(0, FieldCaptionOffset, map[string]int{"x": 1, "y": 2, "z": 3}),
			"captions[0]: caption.offset: " + wantPixels},
		{"a position as a number", captionEdit(2, FieldCaptionPosition, 40), "captions[2]: caption.position: " + wantPixels},
		{"a position on a caption that is not there", captionEdit(9, FieldCaptionPosition, pixels(1, 1)),
			"captions[9]: caption.position: the storyboard has no caption at index 9"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, _, err := Apply([]byte(captionPlacementBoard), captionEditPath, tc.edit)

			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Errorf("Apply(%+v) = %v, want an error containing %q", tc.edit, err, tc.message)
			}
		})
	}
}
