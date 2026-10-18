package storyboard

import (
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
