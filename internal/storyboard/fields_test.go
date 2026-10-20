package storyboard

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// everyFormPath is the storyboard of every form the field description tests read.
const everyFormPath = "testdata/every-form.yaml"

// readEveryForm returns the every-form storyboard's source and the storyboard it parses to.
func readEveryForm(t *testing.T) ([]byte, *Storyboard) {
	t.Helper()
	src, err := os.ReadFile(everyFormPath)
	if err != nil {
		t.Fatalf("read %s: %v", everyFormPath, err)
	}
	board, err := Parse(src, everyFormPath)
	if err != nil {
		t.Fatalf("parse %s: %v", everyFormPath, err)
	}
	return src, board
}

// fieldKey names one field of one owner.
type fieldKey struct {
	owner Owner
	name  string
}

// clipOwner is the clip, the owner of the frame fields.
var clipOwner = Owner{Kind: OwnerClip}

// beatOwner is the beat with id.
func beatOwner(id int) Owner { return Owner{Kind: OwnerBeat, Index: id} }

// positionOwner is the camera position at index.
func positionOwner(index int) Owner { return Owner{Kind: OwnerPosition, Index: index} }

// zoomOwner is the camera zoom at index.
func zoomOwner(index int) Owner { return Owner{Kind: OwnerZoom, Index: index} }

// textOwner is the caption at index.
func textOwner(index int) Owner { return Owner{Kind: OwnerCaption, Index: index} }

// defaultRefusals are the fields of the every-form storyboard whose Default Apply refuses, each
// with a valid sample of its kind for its owner: a position's cell and target default to null,
// which Apply refuses, a position or zoom moved to the clip's start overlaps camera.positions[0]
// or camera.zooms[0], a zoom inserted at the clip's start overlaps camera.zooms[0], and a zoom of
// no length or factor 0 is invalid.
var defaultRefusals = map[fieldKey]json.RawMessage{
	{clipOwner, FieldFrameWidth}:            typedValue(300),
	{clipOwner, FieldFrameFPS}:              typedValue(12),
	{clipOwner, FieldFrameFontSize}:         typedValue(16),
	{clipOwner, FieldFrameLineHeight}:       typedValue(1.3),
	{clipOwner, FieldFrameScale}:            typedValue(3),
	{beatOwner(1), FieldDuration}:           typedValue(2500),
	{beatOwner(2), FieldDuration}:           typedValue(5000),
	{beatOwner(3), FieldDuration}:           typedValue(7000),
	{beatOwner(4), FieldCut}:                typedValue(true),
	{positionOwner(1), FieldPositionAt}:     moment(2, 0),
	{positionOwner(2), FieldPositionAt}:     moment(0, 600),
	{positionOwner(0), FieldPositionCell}:   typedValue(cell(1, 1)),
	{positionOwner(1), FieldPositionCell}:   typedValue(cell(1, 1)),
	{positionOwner(2), FieldPositionCell}:   typedValue(cell(1, 1)),
	{positionOwner(3), FieldPositionCell}:   typedValue(cell(1, 1)),
	{positionOwner(0), FieldPositionTarget}: typedValue(map[string]any{"text": "Items"}),
	{positionOwner(1), FieldPositionTarget}: typedValue(map[string]any{"text": "Items"}),
	{positionOwner(2), FieldPositionTarget}: typedValue(map[string]any{"text": "Items"}),
	{positionOwner(3), FieldPositionTarget}: typedValue(map[string]any{"text": "Items"}),
	{zoomOwner(1), FieldZoomAt}:             moment(1, 1000),
	{zoomOwner(0), FieldZoomInsert}:         typedValue(map[string]any{"at": moment(0, 2000)}),
	{zoomOwner(0), FieldZoomOver}:           typedValue(100),
	{zoomOwner(1), FieldZoomOver}:           typedValue(100),
	{zoomOwner(2), FieldZoomOver}:           typedValue(100),
	{zoomOwner(0), FieldZoomTo}:             typedValue(2),
	{zoomOwner(1), FieldZoomTo}:             typedValue(2),
	{zoomOwner(2), FieldZoomTo}:             typedValue(2),
	{textOwner(0), FieldCaptionFor}:         typedValue(1000),
	{textOwner(1), FieldCaptionFor}:         typedValue(1000),
	{textOwner(2), FieldCaptionFor}:         typedValue(1000),
	{textOwner(0), FieldCaptionText}:        typedValue("Hello"),
	{textOwner(1), FieldCaptionText}:        typedValue("Hello\nthere"),
	{textOwner(2), FieldCaptionText}:        typedValue("Bye"),
}

func TestFields_EveryEditableField_RoundTripsThroughApply(t *testing.T) {
	t.Parallel()
	src, board := readEveryForm(t)
	described := map[fieldKey]bool{}

	for _, field := range Fields(board) {
		if field.IsReadOnly {
			continue
		}
		key := fieldKey{field.Owner, field.Name}
		described[key] = true
		t.Run(editName(Edit{Owner: field.Owner, Field: field.Name}), func(t *testing.T) {
			t.Parallel()
			_, _, err := Apply(src, everyFormPath, Edit{Owner: field.Owner, Field: field.Name, Value: field.Default})
			sample, isRefused := defaultRefusals[key]
			switch {
			case !isRefused && err != nil:
				t.Fatalf("Apply of the default %s: %v", field.Default, err)
			case isRefused && err == nil:
				t.Fatalf("Apply of the default %s was accepted, want it refused", field.Default)
			case !isRefused || sample == nil:
				return
			}
			if _, _, err := Apply(src, everyFormPath, Edit{Owner: field.Owner, Field: field.Name, Value: sample}); err != nil {
				t.Errorf("Apply of the sample %s: %v", sample, err)
			}
		})
	}

	for key := range defaultRefusals {
		if !described[key] {
			t.Errorf("the refusal %v %s names no editable field", key.owner, key.name)
		}
	}
}

func TestFields_NameEveryEditField(t *testing.T) {
	t.Parallel()
	_, board := readEveryForm(t)
	var names []string
	for _, field := range Fields(board) {
		if !field.IsReadOnly {
			names = append(names, field.Name)
		}
	}

	for _, name := range EditFields() {
		if !slices.Contains(names, name) {
			t.Errorf("Fields describes no %s, which Apply accepts", name)
		}
	}
	for _, name := range names {
		if !slices.Contains(EditFields(), name) {
			t.Errorf("Fields describes %s as editable, which Apply does not accept", name)
		}
	}
}

func TestFields_DescribeEachOwnersFields(t *testing.T) {
	t.Parallel()
	_, board := readEveryForm(t)
	byOwner := map[Owner][]string{}
	described := map[fieldKey]Field{}
	for _, field := range Fields(board) {
		byOwner[field.Owner] = append(byOwner[field.Owner], field.Name)
		described[fieldKey{field.Owner, field.Name}] = field
	}

	owners := []struct {
		owner Owner
		want  []string
	}{
		{clipOwner, []string{"cols", "rows", FieldFramePadding, FieldFrameFontSize, FieldFrameLineHeight, FieldFrameScale, FieldFrameWidth, FieldFrameFPS, "max_colors"}},
		{beatOwner(1), []string{FieldDuration, FieldHold, FieldCut}},
		{beatOwner(2), []string{FieldDuration, FieldHold, FieldCut}},
		{positionOwner(0), []string{FieldPositionAt, FieldPositionHold, FieldPositionSpan, FieldPositionEase, FieldPositionCell, FieldPositionTarget, FieldPositionRemove, FieldPositionInsert}},
		{positionOwner(3), []string{FieldPositionAt, FieldPositionHold, FieldPositionSpan, FieldPositionTransition, FieldPositionEase, FieldPositionCell, FieldPositionTarget, FieldPositionRemove}},
		{zoomOwner(0), []string{FieldZoomAt, FieldZoomOver, FieldZoomEase, FieldZoomTo, FieldZoomRemove, FieldZoomInsert}},
		{zoomOwner(2), []string{FieldZoomAt, FieldZoomOver, FieldZoomEase, FieldZoomTo, FieldZoomRemove}},
		{textOwner(0), []string{FieldCaptionAt, FieldCaptionFor, FieldCaptionUntil, FieldCaptionShift, FieldCaptionText, FieldCaptionPlace, FieldCaptionAlign, FieldCaptionOffset, FieldCaptionPosition, FieldCaptionFontSize, FieldCaptionFontWeight, FieldCaptionColor, FieldCaptionBoxColor, FieldCaptionBoxOpacity, FieldCaptionFadeIn, FieldCaptionFadeOut, FieldCaptionRemove, FieldCaptionMove, FieldCaptionInsert}},
		{textOwner(2), []string{FieldCaptionAt, FieldCaptionFor, FieldCaptionUntil, FieldCaptionShift, FieldCaptionText, FieldCaptionPlace, FieldCaptionAlign, FieldCaptionOffset, FieldCaptionPosition, FieldCaptionFontSize, FieldCaptionFontWeight, FieldCaptionColor, FieldCaptionBoxColor, FieldCaptionBoxOpacity, FieldCaptionFadeIn, FieldCaptionFadeOut, FieldCaptionRemove, FieldCaptionMove}},
	}
	for _, test := range owners {
		if got := byOwner[test.owner]; !slices.Equal(got, test.want) {
			t.Errorf("%v fields %q, want %q", test.owner, got, test.want)
		}
	}

	fields := []struct {
		key        fieldKey
		kind       FieldKind
		choices    []string
		defaults   string
		isReadOnly bool
	}{
		{fieldKey{clipOwner, "cols"}, KindCount, nil, "0", true},
		{fieldKey{clipOwner, FieldFrameLineHeight}, KindDecimal, nil, "0", false},
		{fieldKey{clipOwner, FieldFramePadding}, KindCount, nil, "0", false},
		{fieldKey{clipOwner, "max_colors"}, KindCount, nil, "0", true},
		{fieldKey{clipOwner, FieldFrameFPS}, KindCount, nil, "0", false},
		{fieldKey{beatOwner(1), FieldCut}, KindBool, nil, "false", false},
		{fieldKey{positionOwner(0), FieldPositionAt}, KindMoment, nil, `{"beat":0,"offset_ms":0}`, false},
		{fieldKey{positionOwner(0), FieldPositionHold}, KindDuration, nil, "0", false},
		{fieldKey{positionOwner(1), FieldPositionSpan}, KindSpan, nil, `{"at":{"beat":0,"offset_ms":0},"hold_ms":0}`, false},
		{fieldKey{positionOwner(2), FieldPositionTransition}, KindTransition, nil, `{"start_ms":500,"end_ms":600}`, false},
		{fieldKey{positionOwner(3), FieldPositionTransition}, KindTransition, nil, `{"start_ms":900,"end_ms":1000}`, false},
		{fieldKey{positionOwner(1), FieldPositionTransition}, KindTransition, nil, `{"start_ms":1000,"end_ms":2000}`, false},
		{fieldKey{positionOwner(2), FieldPositionEase}, KindEnum, []string{"smooth", "linear", "ease-in", "ease-out"}, `"smooth"`, false},
		{fieldKey{positionOwner(2), FieldPositionCell}, KindCell, nil, "null", false},
		{fieldKey{positionOwner(3), FieldPositionTarget}, KindTarget, nil, "null", false},
		{fieldKey{zoomOwner(0), FieldZoomAt}, KindMoment, nil, `{"beat":0,"offset_ms":0}`, false},
		{fieldKey{zoomOwner(1), FieldZoomOver}, KindDuration, nil, "0", false},
		{fieldKey{zoomOwner(1), FieldZoomEase}, KindEnum, []string{"smooth", "linear", "ease-in", "ease-out"}, `"smooth"`, false},
		{fieldKey{zoomOwner(2), FieldZoomTo}, KindDecimal, nil, "0", false},
		{fieldKey{positionOwner(0), FieldPositionInsert}, KindNewPosition, nil, `{"at":{"beat":0,"offset_ms":0},"cell":{"x":40,"y":12}}`, false},
		{fieldKey{positionOwner(3), FieldPositionRemove}, KindRemoval, nil, "null", false},
		{fieldKey{zoomOwner(0), FieldZoomInsert}, KindNewZoom, nil, `{"at":{"beat":0,"offset_ms":0}}`, false},
		{fieldKey{zoomOwner(2), FieldZoomRemove}, KindRemoval, nil, "null", false},
		{fieldKey{textOwner(0), FieldCaptionUntil}, KindMoment, nil, "null", false},
		{fieldKey{textOwner(0), FieldCaptionShift}, KindCount, nil, "0", false},
		{fieldKey{textOwner(0), FieldCaptionText}, KindText, nil, `""`, false},
		{fieldKey{textOwner(0), FieldCaptionPlace}, KindEnum, []string{"top", "middle", "bottom"}, `"bottom"`, false},
		{fieldKey{textOwner(0), FieldCaptionAlign}, KindEnum, []string{"left", "middle", "right"}, `"middle"`, false},
		{fieldKey{textOwner(0), FieldCaptionOffset}, KindPixels, nil, `{"x":0,"y":0}`, false},
		{fieldKey{textOwner(1), FieldCaptionPosition}, KindPixels, nil, "null", false},
		{fieldKey{textOwner(1), FieldCaptionFontSize}, KindDecimal, nil, "14", false},
		{fieldKey{textOwner(1), FieldCaptionFontWeight}, KindEnum, []string{"regular", "bold"}, `"regular"`, false},
		{fieldKey{textOwner(1), FieldCaptionColor}, KindColor, nil, `"#ffffff"`, false},
		{fieldKey{textOwner(1), FieldCaptionBoxColor}, KindColor, nil, `"#000000"`, false},
		{fieldKey{textOwner(1), FieldCaptionBoxOpacity}, KindFraction, nil, "0.6", false},
		{fieldKey{textOwner(1), FieldCaptionFadeIn}, KindDuration, nil, "0", false},
		{fieldKey{textOwner(1), FieldCaptionFadeOut}, KindDuration, nil, "0", false},
		{fieldKey{textOwner(0), FieldCaptionInsert}, KindEntry, []string{"caption"}, `{"kind":"caption","at":{"beat":0,"offset_ms":0}}`, false},
		{fieldKey{textOwner(2), FieldCaptionRemove}, KindRemoval, nil, "null", false},
		{fieldKey{textOwner(2), FieldCaptionMove}, KindCount, nil, "2", false},
	}
	for _, test := range fields {
		field := described[test.key]
		isWanted := field.Kind == test.kind && slices.Equal(field.Choices, test.choices) &&
			string(field.Default) == test.defaults && field.IsReadOnly == test.isReadOnly
		if !isWanted {
			t.Errorf("%v %s: %+v, want kind %s, choices %q, default %s, read-only %t",
				test.key.owner, test.key.name, field, test.kind, test.choices, test.defaults, test.isReadOnly)
		}
	}
}

func TestField_JSONKeys(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(Field{Default: json.RawMessage("0")})
	if err != nil {
		t.Fatalf("marshal a Field: %v", err)
	}
	var keys map[string]any
	if err := json.Unmarshal(encoded, &keys); err != nil {
		t.Fatalf("unmarshal a Field: %v", err)
	}

	for _, key := range []string{"owner", "name", "kind", "choices", "default", "read_only"} {
		if _, ok := keys[key]; !ok {
			t.Errorf("a Field encodes no %s key: %s", key, encoded)
		}
	}
	if len(keys) != 6 {
		t.Errorf("a Field encodes %d keys, want 6: %s", len(keys), encoded)
	}
}
