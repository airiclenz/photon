package storyboard

import (
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// FieldKind is the kind of value a [Field] holds, which says how an [Edit] types it.
type FieldKind string

// The spellings of FieldKind.
const (
	// KindDuration is a whole number of milliseconds.
	KindDuration FieldKind = "duration"
	// KindCount is a whole number.
	KindCount FieldKind = "count"
	// KindDecimal is a number.
	KindDecimal FieldKind = "decimal"
	// KindBool is true or false.
	KindBool FieldKind = "bool"
	// KindEnum is one of the field's Choices.
	KindEnum FieldKind = "enum"
	// KindText is a string.
	KindText FieldKind = "text"
	// KindMoment is a moment on the clip's output clock, {"beat": 3, "offset_ms": 200}.
	KindMoment FieldKind = "moment"
	// KindTarget is an on-screen target, {"text": "ok", "nth": 2, "area": "list"}.
	KindTarget FieldKind = "target"
	// KindCell is a cell of the terminal grid, {"x": 3, "y": 4}: column X and row Y, from 0.
	KindCell FieldKind = "cell"
	// KindSpan is a camera position's start and hold at once, {"at": {"beat": 3, "offset_ms":
	// 200}, "hold_ms": 600}.
	KindSpan FieldKind = "span"
	// KindTransition is the edges of a camera transition on the output clock, {"start_ms":
	// 1200, "end_ms": 1800}: from the end of one position to the start of the next.
	KindTransition FieldKind = "transition"
	// KindEntry is a new caption, {"kind": "caption", "at": {"beat": 0, "offset_ms": 0}}, its
	// kind one of the field's Choices.
	KindEntry FieldKind = "entry"
	// KindNewPosition is a new camera position, {"at": {"beat": 0, "offset_ms": 0}, "cell":
	// {"x": 3, "y": 4}}: a point in time on a cell.
	KindNewPosition FieldKind = "new-position"
	// KindNewZoom is a new camera zoom, {"at": {"beat": 0, "offset_ms": 0}}.
	KindNewZoom FieldKind = "new-zoom"
	// KindRemoval is null, which removes the owner.
	KindRemoval FieldKind = "removal"
	// KindColor is a colour spelled #rrggbb, "#ffcc00".
	KindColor FieldKind = "color"
	// KindFraction is a number from 0 to 1.
	KindFraction FieldKind = "fraction"
	// KindPixels is a pair of whole lengths in design pixels, {"x": 8, "y": -4}: X to the right
	// and Y down.
	KindPixels FieldKind = "pixels"
)

// Field describes one field of a storyboard's owner — the clip, a beat, a camera position, a
// camera zoom or a caption — as photon tune's page shows it: its Name, the kind of value it
// holds, the Choices of an enum, and its Default, the value it holds when the storyboard leaves
// its key out (the zero value, or what [Load] fills in), typed as an [Edit] types it; null is a
// field an absent key leaves unset, a position's cell and target among them. A list field, which
// adds, removes or reorders an entry and has no key, defaults to a value it takes: a caption
// insert's entry of its first choice at the clip's start, a position insert's point at the clip's
// start on the frame's centre cell, a zoom insert's at the clip's start, a remove's null, a move's
// own index. A caption's
// caption.shift, which has no key either, defaults to 0, the shift that changes nothing, and a
// position's position.span, which sets two keys, to what both mean when absent: the clip's start
// and a hold of 0. A position's position.transition, which has no key either, defaults to the
// transition into it as it stands, the edges that change nothing; only a position with another
// starting before it has one. An editable field's Name is the field an [Edit] names; a read-only
// field is one of the frame's keys photon tune shows but does not edit — cols, rows or
// max_colors — named by that key alone.
type Field struct {
	Owner      Owner           `json:"owner"`
	Name       string          `json:"name"`
	Kind       FieldKind       `json:"kind"`
	Choices    []string        `json:"choices"`
	Default    json.RawMessage `json:"default"`
	IsReadOnly bool            `json:"read_only"`
}

// frameFieldPrefix starts the name of the frame fields an [Edit] names.
const frameFieldPrefix = "frame."

// The defaults a field's absent key means, typed as an [Edit] types them: zero, false,
// no text, a moment at the clip's first frame, no offset, and a span at the clip's first frame
// holding no time.
const (
	defaultZero      = "0"
	defaultFalse     = "false"
	defaultText      = `""`
	defaultClipStart = `{"beat":0,"offset_ms":0}`
	defaultNoOffset  = `{"x":0,"y":0}`
	defaultSpan      = `{"at":` + defaultClipStart + `,"hold_ms":0}`
)

// newEntryDefault is the default of a list's insert: an entry of kind at the clip's start.
func newEntryDefault(kind string) json.RawMessage {
	return raw(`{"kind":` + strconv.Quote(kind) + `,"at":` + defaultClipStart + `}`)
}

// The choices of the enum fields, in the order the page offers them.
var (
	placeChoices  = []string{string(PlaceTop), string(PlaceMiddle), string(PlaceBottom)}
	alignChoices  = []string{string(AlignLeft), string(AlignMiddle), string(AlignRight)}
	weightChoices = []string{string(WeightRegular), string(WeightBold)}
	easeChoices   = []string{
		string(EaseSmooth), string(EaseLinear), string(EaseIn), string(EaseOut),
	}
)

// Fields describes every field of board's owners, the editable and the read-only: the clip's
// frame fields in the frame's key order, then each beat's, camera position's, camera zoom's and
// caption's in storyboard order, each list's insert after its entries', owned by index 0,
// whether or not the list holds an entry. The frame's cols and rows are read-only because they
// size the terminal the take was recorded in, so a change needs a new `photon record`, and its
// max_colors because the page draws its preview before the palette is quantized, so the preview
// could not show a change.
func Fields(board *Storyboard) []Field {
	fields := clipFields()
	for _, beat := range board.Beats {
		fields = append(fields, beatFields(beat)...)
	}
	for index := range board.Camera.Positions {
		fields = append(fields, positionFields(board.Camera.Positions, index)...)
	}
	fields = append(fields, positionInsertField(board.Frame))
	for index := range board.Camera.Zooms {
		fields = append(fields, zoomFields(index)...)
	}
	fields = append(fields, zoomInsertField())
	for index := range board.Captions {
		fields = append(fields, captionFields(index, board.Frame.FontSize)...)
	}
	return append(fields, insertField(OwnerCaption, FieldCaptionInsert, captionEntryKinds))
}

// insertField describes a list's insert, name, owned by the list's first index, its choices
// kinds.
func insertField(kind OwnerKind, name string, kinds []string) Field {
	return Field{
		Owner:   Owner{Kind: kind},
		Name:    name,
		Kind:    KindEntry,
		Choices: slices.Clone(kinds),
		Default: newEntryDefault(kinds[0]),
	}
}

// positionInsertField describes the camera positions' insert, owned by the list's first index:
// a point in time at the clip's start on frame's centre cell.
func positionInsertField(frame Frame) Field {
	centre := cellJSON(Cell{X: frame.Cols / 2, Y: frame.Rows / 2})
	return Field{
		Owner:   Owner{Kind: OwnerPosition},
		Name:    FieldPositionInsert,
		Kind:    KindNewPosition,
		Default: raw(`{"at":` + defaultClipStart + `,"cell":` + centre + `}`),
	}
}

// zoomInsertField describes the camera zooms' insert, owned by the list's first index: a zoom at
// the clip's start.
func zoomInsertField() Field {
	return Field{
		Owner:   Owner{Kind: OwnerZoom},
		Name:    FieldZoomInsert,
		Kind:    KindNewZoom,
		Default: raw(`{"at":` + defaultClipStart + `}`),
	}
}

// cellJSON spells cell as an [Edit] types it, {"x":3,"y":4}.
func cellJSON(cell Cell) string {
	return `{"x":` + strconv.Itoa(cell.X) + `,"y":` + strconv.Itoa(cell.Y) + `}`
}

// listEntryFields describes the remove and move of the entry at index of owner's kind, named
// remove and move.
func listEntryFields(owner Owner, remove, move string) []Field {
	return []Field{
		removeField(owner, remove),
		{Owner: owner, Name: move, Kind: KindCount, Default: raw(strconv.Itoa(owner.Index))},
	}
}

// removeField describes the remove of owner, named name: null, the value it takes.
func removeField(owner Owner, name string) Field {
	return Field{Owner: owner, Name: name, Kind: KindRemoval, Default: raw(jsonNull)}
}

// clipFields describes the frame's fields in its field order, each keyed as its yaml tag names it:
// frame.padding, frame.font_size, frame.line_height, frame.scale, frame.width and frame.fps,
// which an [Edit] names, and the rest read-only under their bare key.
// A floating-point field is a decimal and every other a count; each defaults to zero.
func clipFields() []Field {
	frame := reflect.TypeFor[Frame]()
	owner := Owner{Kind: OwnerClip}
	fields := make([]Field, 0, frame.NumField())
	for goField := range frame.Fields() {
		key, _, _ := strings.Cut(goField.Tag.Get(yamlTag), tagSeparator)
		described := Field{
			Owner: owner, Name: frameFieldPrefix + key, Kind: KindCount, Default: raw(defaultZero),
		}
		if kind := goField.Type.Kind(); kind == reflect.Float32 || kind == reflect.Float64 {
			described.Kind = KindDecimal
		}
		if !slices.Contains(editFields, described.Name) {
			described.Name, described.IsReadOnly = key, true
		}
		fields = append(fields, described)
	}
	return fields
}

// beatFields describes beat's fields: its timing and its cut.
func beatFields(beat Beat) []Field {
	owner := Owner{Kind: OwnerBeat, Index: beat.ID}
	return []Field{
		{Owner: owner, Name: FieldDuration, Kind: KindDuration, Default: raw(defaultZero)},
		{Owner: owner, Name: FieldHold, Kind: KindDuration, Default: raw(defaultZero)},
		{Owner: owner, Name: FieldCut, Kind: KindBool, Default: raw(defaultFalse)},
	}
}

// positionFields describes the fields of the camera position at index in positions: its start,
// its hold, both at once, the transition into it unless it is the first by start, its ease,
// where it aims — a cell or a text target — and its remove.
func positionFields(positions []Position, index int) []Field {
	owner := Owner{Kind: OwnerPosition, Index: index}
	fields := []Field{
		{Owner: owner, Name: FieldPositionAt, Kind: KindMoment, Default: raw(defaultClipStart)},
		{Owner: owner, Name: FieldPositionHold, Kind: KindDuration, Default: raw(defaultZero)},
		{Owner: owner, Name: FieldPositionSpan, Kind: KindSpan, Default: raw(defaultSpan)},
	}
	if edges, hasTransition := currentTransition(positions, index); hasTransition {
		fields = append(fields, Field{
			Owner: owner, Name: FieldPositionTransition, Kind: KindTransition,
			Default: raw(transitionJSON(edges)),
		})
	}
	return append(fields,
		easeField(owner, FieldPositionEase),
		Field{Owner: owner, Name: FieldPositionCell, Kind: KindCell, Default: raw(jsonNull)},
		Field{Owner: owner, Name: FieldPositionTarget, Kind: KindTarget, Default: raw(jsonNull)},
		removeField(owner, FieldPositionRemove),
	)
}

// zoomFields describes the fields of the camera zoom at index: its start, its length, its ease,
// its factor and its remove.
func zoomFields(index int) []Field {
	owner := Owner{Kind: OwnerZoom, Index: index}
	return []Field{
		{Owner: owner, Name: FieldZoomAt, Kind: KindMoment, Default: raw(defaultClipStart)},
		{Owner: owner, Name: FieldZoomOver, Kind: KindDuration, Default: raw(defaultZero)},
		easeField(owner, FieldZoomEase),
		{Owner: owner, Name: FieldZoomTo, Kind: KindDecimal, Default: raw(defaultZero)},
		removeField(owner, FieldZoomRemove),
	}
}

// easeField describes owner's ease, name: an enum of the four eases, smooth by default.
func easeField(owner Owner, name string) Field {
	return Field{
		Owner: owner, Name: name, Kind: KindEnum,
		Choices: slices.Clone(easeChoices), Default: quoted(string(EaseSmooth)),
	}
}

// captionFields describes the fields of the caption at index: its start, its end — a for or an
// until — the shift of both, its text, where it sits — its place, align and offset, or its
// position — its font, its size defaulting to frameFontSize, its colours, its box and its fade,
// and its remove and move.
func captionFields(index int, frameFontSize float64) []Field {
	owner := Owner{Kind: OwnerCaption, Index: index}
	fields := []Field{
		{Owner: owner, Name: FieldCaptionAt, Kind: KindMoment, Default: raw(defaultClipStart)},
		{Owner: owner, Name: FieldCaptionFor, Kind: KindDuration, Default: raw(defaultZero)},
		{Owner: owner, Name: FieldCaptionUntil, Kind: KindMoment, Default: raw(jsonNull)},
		{Owner: owner, Name: FieldCaptionShift, Kind: KindCount, Default: raw(defaultZero)},
		{Owner: owner, Name: FieldCaptionText, Kind: KindText, Default: raw(defaultText)},
		{
			Owner: owner, Name: FieldCaptionPlace, Kind: KindEnum,
			Choices: slices.Clone(placeChoices), Default: quoted(string(PlaceBottom)),
		},
		{
			Owner: owner, Name: FieldCaptionAlign, Kind: KindEnum,
			Choices: slices.Clone(alignChoices), Default: quoted(string(AlignMiddle)),
		},
		{Owner: owner, Name: FieldCaptionOffset, Kind: KindPixels, Default: raw(defaultNoOffset)},
		{Owner: owner, Name: FieldCaptionPosition, Kind: KindPixels, Default: raw(jsonNull)},
		{
			Owner: owner, Name: FieldCaptionFontSize, Kind: KindDecimal,
			Default: raw(formatDecimal(frameFontSize)),
		},
		{
			Owner: owner, Name: FieldCaptionFontWeight, Kind: KindEnum,
			Choices: slices.Clone(weightChoices), Default: quoted(string(WeightRegular)),
		},
		{
			Owner: owner, Name: FieldCaptionColor, Kind: KindColor,
			Default: quoted(defaultCaptionColor),
		},
		{
			Owner: owner, Name: FieldCaptionBoxColor, Kind: KindColor,
			Default: quoted(defaultBoxColor),
		},
		{
			Owner: owner, Name: FieldCaptionBoxOpacity, Kind: KindFraction,
			Default: raw(formatDecimal(defaultBoxOpacity)),
		},
		{Owner: owner, Name: FieldCaptionFadeIn, Kind: KindDuration, Default: raw(defaultZero)},
		{Owner: owner, Name: FieldCaptionFadeOut, Kind: KindDuration, Default: raw(defaultZero)},
	}
	return append(fields, listEntryFields(owner, FieldCaptionRemove, FieldCaptionMove)...)
}

// raw is text, a JSON value, in bytes of its own, so no two fields share a Default.
func raw(text string) json.RawMessage {
	return json.RawMessage(text)
}

// quoted is text as a JSON string.
func quoted(text string) json.RawMessage {
	return json.RawMessage(strconv.Quote(text))
}
