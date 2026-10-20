package storyboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/airiclenz/photon/internal/yamledit"
)

// The list fields an [Edit] names: each adds, removes or reorders an entry of the storyboard's
// captions list, or adds or removes one of its camera's positions or zooms, the entry its owner's
// index names. Neither camera list is reordered: render places its entries by their starts.
const (
	FieldCaptionInsert  = "caption.insert"
	FieldCaptionRemove  = "caption.remove"
	FieldCaptionMove    = "caption.move"
	FieldPositionInsert = "position.insert"
	FieldPositionRemove = "position.remove"
	FieldZoomInsert     = "zoom.insert"
	FieldZoomRemove     = "zoom.remove"
)

// entryCaption is the kind of entry a list insert adds to the captions.
const entryCaption = "caption"

// The presets a new caption is written with: newCaptionText for newCaptionLength.
const (
	newCaptionText   = "Caption"
	newCaptionLength = 2 * time.Second
)

// The presets a new zoom is written with: a zoom to newZoomFactor over newZoomLength.
const (
	newZoomFactor = 1.5
	newZoomLength = 600 * time.Millisecond
)

// storyboardKeyOrder is the storyboard's top-level keys in its struct field order: a list key an
// insert creates goes after whichever of the keys before it the storyboard holds.
var storyboardKeyOrder = keyOrder(reflect.TypeFor[Storyboard]())

// cameraKeyOrder is the camera's keys in its struct field order: a camera list an insert creates
// goes after whichever of the keys before it the camera holds.
var cameraKeyOrder = keyOrder(reflect.TypeFor[Camera]())

// entryList is one of the storyboard's lists a list edit changes: the captions, or the camera's
// positions or zooms.
type entryList struct {
	// holder is the mapping that holds the list, the storyboard's top level or its camera, and
	// key the list's key in it.
	holder yamledit.Path
	key    string
	// order is the holder's key order, which places a list key an insert creates.
	order []string
	// noun names one entry in errors.
	noun string
	// length is how many entries the list holds.
	length int
	// newEntry reads an insert's value and spells the new entry as one flow mapping.
	newEntry func(value json.RawMessage) (string, error)
}

// captionList is board's captions as list edits change them.
func captionList(board *Storyboard) entryList {
	return entryList{
		key:      keyCaptions,
		order:    storyboardKeyOrder,
		noun:     "caption",
		length:   len(board.Captions),
		newEntry: newCaption,
	}
}

// positionList is board's camera positions as list edits change them.
func positionList(board *Storyboard) entryList {
	return entryList{
		holder:   yamledit.Path{keyCamera},
		key:      keyPositions,
		order:    cameraKeyOrder,
		noun:     "camera position",
		length:   len(board.Camera.Positions),
		newEntry: newPosition,
	}
}

// zoomList is board's camera zooms as list edits change them.
func zoomList(board *Storyboard) entryList {
	return entryList{
		holder:   yamledit.Path{keyCamera},
		key:      keyZooms,
		order:    cameraKeyOrder,
		noun:     "camera zoom",
		length:   len(board.Camera.Zooms),
		newEntry: newZoom,
	}
}

// path is where the list sits in the storyboard.
func (l entryList) path() yamledit.Path {
	return under(l.holder, l.key)
}

// captionEntryKinds are the kinds of entry an insert adds to the captions, in the order the page
// offers them.
var captionEntryKinds = []string{entryCaption}

// listFields are the list fields, which add, remove or reorder an entry rather than edit one.
var listFields = []string{
	FieldCaptionInsert, FieldCaptionRemove, FieldCaptionMove,
	FieldPositionInsert, FieldPositionRemove, FieldZoomInsert, FieldZoomRemove,
}

// isListField reports whether field adds, removes or reorders an entry rather than editing one.
func isListField(field string) bool {
	return slices.Contains(listFields, field)
}

// isInsertField reports whether field adds an entry.
func isInsertField(field string) bool {
	return field == FieldCaptionInsert || field == FieldPositionInsert || field == FieldZoomInsert
}

// isRemoveField reports whether field removes an entry, the edit a null value types.
func isRemoveField(field string) bool {
	return field == FieldCaptionRemove || field == FieldPositionRemove ||
		field == FieldZoomRemove
}

// editList applies field, a list field typed as value, to list at index in the storyboard s.doc
// holds: an insert takes an index from 0 through the list's length, a remove or move one of the
// list's entries.
func (s *splicer) editList(list entryList, index int, field string, value json.RawMessage) error {
	switch {
	case isInsertField(field):
		return s.insertEntry(list, index, value)
	case index < 0 || index >= list.length:
		return fmt.Errorf("the storyboard has no %s at index %d", list.noun, index)
	case isRemoveField(field):
		return s.removeEntry(list, index, value)
	default:
		return s.moveEntry(list, index, value)
	}
}

// wireEntry is a new caption as an [Edit] types it, {"kind": "caption", "at": {"beat": 0,
// "offset_ms": 1200}}: its kind and where it starts. Both keys are required.
type wireEntry struct {
	Kind *string         `json:"kind"`
	At   json.RawMessage `json:"at"`
}

// insertEntry adds the new entry value types to list so that it is the entry at index, written
// as one flow line. A list the storyboard leaves out, or holds empty, is written whole as a flow
// list holding the entry, after the keys that precede it; a camera list whose camera is absent,
// empty or null is written with the camera as one flow mapping holding the list, after the keys
// that precede the camera.
func (s *splicer) insertEntry(list entryList, index int, value json.RawMessage) error {
	if index < 0 || index > list.length {
		return fmt.Errorf(
			"want an index from 0 to %d for the new %s, got %d", list.length, list.noun, index)
	}
	text, err := list.newEntry(value)
	if err != nil {
		return err
	}
	if list.length > 0 {
		return s.doc.InsertItem(list.path(), index, text)
	}
	whole := "[" + text + "]"
	if len(list.holder) == 0 || s.holdsAnyKey(list.holder, list.order) {
		return s.doc.Upsert(list.holder, list.key, whole, list.order)
	}
	// An empty or null holder is rewritten whole: a null one has no text to splice a key into.
	if err := s.doc.DeleteIfPresent(list.holder); err != nil {
		return err
	}
	holder := "{" + list.key + ": " + whole + "}"
	return s.doc.Upsert(yamledit.Path{}, list.holder[0], holder, storyboardKeyOrder)
}

// holdsAnyKey reports whether the mapping at holder holds any of the keys order names: false
// for an absent, empty or null mapping.
func (s *splicer) holdsAnyKey(holder yamledit.Path, order []string) bool {
	isHeld := func(key string) bool { return s.doc.Has(under(holder, key)) }
	return slices.ContainsFunc(order, isHeld)
}

// newCaption reads a new caption typed as {"kind", "at"} and spells it as [entryFlow] does.
func newCaption(value json.RawMessage) (string, error) {
	at, err := decodeEntry(value)
	if err != nil {
		return "", err
	}
	return entryFlow(at), nil
}

// decodeEntry reads a new caption typed as {"kind", "at"}, refusing any other key, a missing one,
// a kind the captions do not take and an at that is not a moment.
func decodeEntry(value json.RawMessage) (Moment, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var wire wireEntry
	err := decoder.Decode(&wire)
	isWhole := err == nil && wire.Kind != nil && len(wire.At) > 0
	if !isWhole || !slices.Contains(captionEntryKinds, *wire.Kind) {
		return Moment{}, fmt.Errorf(
			`want a new caption such as {"kind": %q, "at": {"beat": 0, "offset_ms": 0}}, `+
				`its kind %s, got %s`,
			captionEntryKinds[0], strings.Join(captionEntryKinds, " or "), value)
	}
	at, err := decodeMoment(wire.At)
	if err != nil {
		return Moment{}, fmt.Errorf("at: %w", err)
	}
	return at, nil
}

// entryFlow spells a new caption starting at as one flow mapping, from the presets.
func entryFlow(at Moment) string {
	text := keyText + ": " + strconv.Quote(newCaptionText)
	return "{" + text + startFlow(at) + ", " + keyFor + ": " + newCaptionLength.String() + "}"
}

// startFlow spells a new entry's at as the flow mapping entry after its first key, `, at:
// 300ms`. A start at the clip's first frame writes nothing: an absent at already means it.
func startFlow(at Moment) string {
	if at == (Moment{}) {
		return ""
	}
	return ", " + keyAt + ": " + momentFlow(at)
}

// wirePosition is a new camera position as an [Edit] types it, {"at": {"beat": 0, "offset_ms":
// 1200}, "cell": {"x": 3, "y": 4}}: where it starts and the cell it aims at. Both keys are
// required.
type wirePosition struct {
	At   json.RawMessage `json:"at"`
	Cell json.RawMessage `json:"cell"`
}

// newPosition reads a new camera position typed as {"at", "cell"}, refusing any other key, a
// missing one, an at that is not a moment and a cell that is not one, and spells it as a point in
// time on its cell, {cell: {x: 3, y: 4}, at: 1200ms}, in the position's key order.
func newPosition(value json.RawMessage) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var wire wirePosition
	if err := decoder.Decode(&wire); err != nil || wire.At == nil || wire.Cell == nil {
		return "", fmt.Errorf(`want a new camera position such as {"at": {"beat": 0, `+
			`"offset_ms": 0}, "cell": {"x": 3, "y": 4}}, got %s`, value)
	}
	at, err := decodeMoment(wire.At)
	if err != nil {
		return "", fmt.Errorf("at: %w", err)
	}
	cell, err := decodeCell(wire.Cell)
	if err != nil {
		return "", fmt.Errorf("cell: %w", err)
	}
	return "{" + keyCell + ": " + cellFlow(cell) + startFlow(at) + "}", nil
}

// wireZoom is a new camera zoom as an [Edit] types it, {"at": {"beat": 0, "offset_ms": 1200}}:
// where it starts. The key is required.
type wireZoom struct {
	At json.RawMessage `json:"at"`
}

// newZoom reads a new camera zoom typed as {"at"}, refusing any other key, a missing one and an
// at that is not a moment, and spells it from the presets in the zoom's key order, {to: 1.5, at:
// 1200ms, over: 600ms}.
func newZoom(value json.RawMessage) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var wire wireZoom
	if err := decoder.Decode(&wire); err != nil || wire.At == nil {
		return "", fmt.Errorf(
			`want a new camera zoom such as {"at": {"beat": 0, "offset_ms": 0}}, got %s`, value)
	}
	at, err := decodeMoment(wire.At)
	if err != nil {
		return "", fmt.Errorf("at: %w", err)
	}
	factor := keyTo + ": " + formatDecimal(newZoomFactor)
	return "{" + factor + startFlow(at) + ", " + keyOver + ": " + newZoomLength.String() + "}", nil
}

// removeEntry removes the entry at index from list, value typed as null; removing the list's
// only entry deletes the list's key, and the camera's too when the camera holds no other list.
func (s *splicer) removeEntry(list entryList, index int, value json.RawMessage) error {
	if !isNull(value) {
		return fmt.Errorf("want null, got %s", value)
	}
	if list.length > 1 {
		return s.doc.DeleteItem(list.path(), index)
	}
	others := slices.DeleteFunc(slices.Clone(list.order), func(key string) bool {
		return key == list.key
	})
	if len(list.holder) > 0 && !s.holdsAnyKey(list.holder, others) {
		return s.doc.Delete(list.holder)
	}
	return s.doc.Delete(list.path())
}

// moveEntry moves the entry at index in list so that it becomes the entry at the destination
// index value types, a whole number naming one of the list's entries.
func (s *splicer) moveEntry(list entryList, index int, value json.RawMessage) error {
	var destination int
	err := json.Unmarshal(value, &destination)
	if err != nil || destination < 0 || destination >= list.length {
		return fmt.Errorf("want a destination index from 0 to %d, got %s", list.length-1, value)
	}
	return s.doc.MoveItem(list.path(), index, destination)
}
