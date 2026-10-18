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
	// KindPoint is a target or a cell of the terminal grid, {"cell": {"x": 3, "y": 4}}.
	KindPoint FieldKind = "point"
	// KindEntry is a new camera move or caption, {"kind": "zoom", "at": {"beat": 0,
	// "offset_ms": 0}}, its kind one of the field's Choices.
	KindEntry FieldKind = "entry"
	// KindRemoval is null, which removes the owner.
	KindRemoval FieldKind = "removal"
	// KindColor is a colour spelled #rrggbb, "#ffcc00".
	KindColor FieldKind = "color"
	// KindFraction is a number from 0 to 1.
	KindFraction FieldKind = "fraction"
	// KindPixels is a pair of whole lengths in design pixels, {"x": 8, "y": -4}: X to the right
	// and Y down.
	KindPixels FieldKind = "pixels"
	// KindWaypoint is a waypoint of a pan's via path, {"index": 1, "point": {"cell": {"x": 3,
	// "y": 4}}}: its index and its point, a target or a cell.
	KindWaypoint FieldKind = "waypoint"
	// KindWaypointRemoval is a waypoint's index, {"index": 1}, which removes that waypoint.
	KindWaypointRemoval FieldKind = "waypoint_removal"
)

// Field describes one field of a storyboard's owner — the clip, a beat, a camera move or a
// caption — as photon tune's page shows it: its Name, the kind of value it holds, the Choices of
// an enum, and its Default, the value it holds when the storyboard leaves its key out (the zero
// value, or what [Load] fills in), typed as an [Edit] types it; null is a field an absent key
// leaves unset. A list field, which adds, removes or reorders an entry and has no key, defaults to
// a value it takes: an insert's entry of its first choice at the clip's start, a remove's null,
// a move's own index. A waypoint field, which sets, adds or removes the waypoint its value
// indexes, defaults to null, and a pan's camera.path, which has no key either, to the path the
// pan follows. An editable field's Name is the field an [Edit] names; a read-only field is one of
// the frame's keys photon tune shows but does not edit, named by that key alone.
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
// no text, a target with no text, a moment at the clip's first frame, and no offset.
const (
	defaultZero      = "0"
	defaultFalse     = "false"
	defaultText      = `""`
	defaultTarget    = `{"text":""}`
	defaultClipStart = `{"beat":0,"offset_ms":0}`
	defaultNoOffset  = `{"x":0,"y":0}`
)

// newEntryDefault is the default of a list's insert: an entry of kind at the clip's start.
func newEntryDefault(kind string) json.RawMessage {
	return raw(`{"kind":` + strconv.Quote(kind) + `,"at":` + defaultClipStart + `}`)
}

// The choices of the enum fields, in the order the page offers them.
var (
	easeChoices   = []string{string(EaseSmooth), string(EaseLinear)}
	placeChoices  = []string{string(PlaceTop), string(PlaceMiddle), string(PlaceBottom)}
	alignChoices  = []string{string(AlignLeft), string(AlignMiddle), string(AlignRight)}
	weightChoices = []string{string(WeightRegular), string(WeightBold)}
)

// Fields describes every field of board's owners, the editable and the read-only: the clip's
// frame fields in the frame's key order, then each beat's, camera move's and caption's in
// storyboard order, the camera's and the captions' insert after their entries', owned by index 0,
// whether or not the list holds an entry. A beat lists its zoom's fields only when it zooms, a
// camera move's camera.to is a decimal on a zoom move and a point on a pan, and a pan lists its
// camera.path, a via pan its waypoint fields too.
func Fields(board *Storyboard) []Field {
	fields := clipFields()
	for _, beat := range board.Beats {
		fields = append(fields, beatFields(beat)...)
	}
	for index, move := range board.Camera {
		fields = append(fields, cameraFields(index, move)...)
	}
	fields = append(fields, insertField(OwnerCamera, FieldCameraInsert, cameraEntryKinds))
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

// listEntryFields describes the remove and move of the entry at index of owner's kind, named
// remove and move.
func listEntryFields(owner Owner, remove, move string) []Field {
	return []Field{
		{Owner: owner, Name: remove, Kind: KindRemoval, Default: raw(jsonNull)},
		{Owner: owner, Name: move, Kind: KindCount, Default: raw(strconv.Itoa(owner.Index))},
	}
}

// clipFields describes the frame's fields in its field order, each keyed as its yaml tag names it:
// frame.width and frame.fps, which an [Edit] names, and the rest read-only under their bare key.
// A floating-point field is a decimal and every other a count; each defaults to zero.
func clipFields() []Field {
	frame := reflect.TypeFor[Frame]()
	owner := Owner{Kind: OwnerClip}
	fields := make([]Field, 0, frame.NumField())
	for goField := range frame.Fields() {
		key, _, _ := strings.Cut(goField.Tag.Get(yamlTag), tagSeparator)
		described := Field{Owner: owner, Name: frameFieldPrefix + key, Kind: KindCount, Default: raw(defaultZero)}
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

// beatFields describes beat's fields: its timing, its cut and its zoom, with the zoom's own
// fields when it zooms. An absent zoom ramp runs [DefaultZoomRamp].
func beatFields(beat Beat) []Field {
	owner := Owner{Kind: OwnerBeat, Index: beat.ID}
	fields := []Field{
		{Owner: owner, Name: FieldDuration, Kind: KindDuration, Default: raw(defaultZero)},
		{Owner: owner, Name: FieldHold, Kind: KindDuration, Default: raw(defaultZero)},
		{Owner: owner, Name: FieldCut, Kind: KindBool, Default: raw(defaultFalse)},
		{Owner: owner, Name: FieldZoom, Kind: KindTarget, Default: raw(jsonNull)},
	}
	if beat.Zoom == nil {
		return fields
	}
	ramp := strconv.FormatInt(DefaultZoomRamp.Milliseconds(), 10)
	return append(fields,
		Field{Owner: owner, Name: FieldZoomTarget, Kind: KindTarget, Default: raw(defaultTarget)},
		Field{Owner: owner, Name: FieldZoomFactor, Kind: KindDecimal, Default: raw(defaultZero)},
		Field{Owner: owner, Name: FieldZoomIn, Kind: KindDuration, Default: raw(ramp)},
		Field{Owner: owner, Name: FieldZoomOut, Kind: KindDuration, Default: raw(ramp)},
	)
}

// cameraFields describes the fields of move, the camera move at index: its start, length and
// ease, its destination — a zoom move's factor, or a pan's target or cell, which a via path has
// none of — a pan's path and a via path's waypoints, and its remove and move.
func cameraFields(index int, move CameraMove) []Field {
	owner := Owner{Kind: OwnerCamera, Index: index}
	to := Field{Owner: owner, Name: FieldCameraTo, Kind: KindDecimal, Default: raw(defaultZero)}
	if move.Pan != nil {
		to.Kind, to.Default = KindPoint, raw(jsonNull)
	}
	fields := []Field{
		{Owner: owner, Name: FieldCameraAt, Kind: KindMoment, Default: raw(defaultClipStart)},
		{Owner: owner, Name: FieldCameraOver, Kind: KindDuration, Default: raw(defaultZero)},
		{Owner: owner, Name: FieldCameraEase, Kind: KindEnum, Choices: slices.Clone(easeChoices), Default: quoted(string(EaseSmooth))},
		to,
	}
	if move.Pan != nil {
		fields = append(fields, pathFields(owner, *move.Pan)...)
	}
	return append(fields, listEntryFields(owner, FieldCameraRemove, FieldCameraMove)...)
}

// pathFields describes the path of pan, the move owner names: its camera.path, defaulting to the
// path it follows, and on a via path the set, insert and remove of a waypoint, each defaulting
// to null.
func pathFields(owner Owner, pan PanMove) []Field {
	path := pathTo
	if pan.Via != nil {
		path = pathVia
	}
	fields := []Field{{
		Owner:   owner,
		Name:    FieldCameraPath,
		Kind:    KindEnum,
		Choices: slices.Clone(pathChoices),
		Default: quoted(path),
	}}
	if pan.Via == nil {
		return fields
	}
	return append(fields,
		Field{Owner: owner, Name: FieldCameraVia, Kind: KindWaypoint, Default: raw(jsonNull)},
		Field{Owner: owner, Name: FieldCameraViaInsert, Kind: KindWaypoint, Default: raw(jsonNull)},
		Field{
			Owner:   owner,
			Name:    FieldCameraViaRemove,
			Kind:    KindWaypointRemoval,
			Default: raw(jsonNull),
		},
	)
}

// captionFields describes the fields of the caption at index: its start, its end — a for or an
// until — its text, where it sits — its place, align and offset, or its position — its font, its
// size defaulting to frameFontSize, its colours, its box and its fade, and its remove and move.
func captionFields(index int, frameFontSize float64) []Field {
	owner := Owner{Kind: OwnerCaption, Index: index}
	fields := []Field{
		{Owner: owner, Name: FieldCaptionAt, Kind: KindMoment, Default: raw(defaultClipStart)},
		{Owner: owner, Name: FieldCaptionFor, Kind: KindDuration, Default: raw(defaultZero)},
		{Owner: owner, Name: FieldCaptionUntil, Kind: KindMoment, Default: raw(jsonNull)},
		{Owner: owner, Name: FieldCaptionText, Kind: KindText, Default: raw(defaultText)},
		{Owner: owner, Name: FieldCaptionPlace, Kind: KindEnum, Choices: slices.Clone(placeChoices), Default: quoted(string(PlaceBottom))},
		{Owner: owner, Name: FieldCaptionAlign, Kind: KindEnum, Choices: slices.Clone(alignChoices), Default: quoted(string(AlignMiddle))},
		{Owner: owner, Name: FieldCaptionOffset, Kind: KindPixels, Default: raw(defaultNoOffset)},
		{Owner: owner, Name: FieldCaptionPosition, Kind: KindPixels, Default: raw(jsonNull)},
		{Owner: owner, Name: FieldCaptionFontSize, Kind: KindDecimal, Default: raw(formatDecimal(frameFontSize))},
		{Owner: owner, Name: FieldCaptionFontWeight, Kind: KindEnum, Choices: slices.Clone(weightChoices), Default: quoted(string(WeightRegular))},
		{Owner: owner, Name: FieldCaptionColor, Kind: KindColor, Default: quoted(defaultCaptionColor)},
		{Owner: owner, Name: FieldCaptionBoxColor, Kind: KindColor, Default: quoted(defaultBoxColor)},
		{Owner: owner, Name: FieldCaptionBoxOpacity, Kind: KindFraction, Default: raw(formatDecimal(defaultBoxOpacity))},
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
