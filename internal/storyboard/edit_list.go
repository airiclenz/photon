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
// captions list, the entry its owner's index names.
const (
	FieldCaptionInsert = "caption.insert"
	FieldCaptionRemove = "caption.remove"
	FieldCaptionMove   = "caption.move"
)

// entryCaption is the kind of entry a list insert adds to the captions.
const entryCaption = "caption"

// The presets a new caption is written with: newCaptionText for newCaptionLength.
const (
	newCaptionText   = "Caption"
	newCaptionLength = 2 * time.Second
)

// storyboardKeyOrder is the storyboard's top-level keys in its struct field order: a list key an
// insert creates goes after whichever of the keys before it the storyboard holds.
var storyboardKeyOrder = keyOrder(reflect.TypeFor[Storyboard]())

// entryList is one of the storyboard's lists a list edit changes: the captions.
type entryList struct {
	// key is the list's storyboard key.
	key string
	// noun names one entry in errors.
	noun string
	// length is how many entries the list holds.
	length int
	// kinds are the kinds of entry an insert adds to the list.
	kinds []string
}

// captionList is board's captions as list edits change them.
func captionList(board *Storyboard) entryList {
	return entryList{
		key:    keyCaptions,
		noun:   "caption",
		length: len(board.Captions),
		kinds:  captionEntryKinds,
	}
}

// captionEntryKinds are the kinds of entry an insert adds to the captions, in the order the page
// offers them.
var captionEntryKinds = []string{entryCaption}

// listFields are the list fields, which add, remove or reorder an entry rather than edit one.
var listFields = []string{FieldCaptionInsert, FieldCaptionRemove, FieldCaptionMove}

// isListField reports whether field adds, removes or reorders an entry rather than editing one.
func isListField(field string) bool {
	return slices.Contains(listFields, field)
}

// isRemoveField reports whether field removes an entry, the edit a null value types.
func isRemoveField(field string) bool {
	return field == FieldCaptionRemove
}

// editList applies field, a list field typed as value, to list at index in the storyboard s.doc
// holds: an insert takes an index from 0 through the list's length, a remove or move one of the
// list's entries.
func (s *splicer) editList(list entryList, index int, field string, value json.RawMessage) error {
	switch {
	case field == FieldCaptionInsert:
		return s.insertEntry(list, index, value)
	case index < 0 || index >= list.length:
		return fmt.Errorf("the storyboard has no %s at index %d", list.noun, index)
	case isRemoveField(field):
		return s.removeEntry(list, index, value)
	default:
		return s.moveEntry(list, index, value)
	}
}

// wireEntry is a new entry as an [Edit] types it, {"kind": "caption", "at": {"beat": 0,
// "offset_ms": 1200}}: its kind and where it starts. Both keys are required.
type wireEntry struct {
	Kind *string         `json:"kind"`
	At   json.RawMessage `json:"at"`
}

// insertEntry adds the new entry value types to list so that it is the entry at index, written
// from the presets as one flow line. A list the storyboard leaves out, or holds empty, is written
// whole as a flow list holding the entry, after the keys that precede it.
func (s *splicer) insertEntry(list entryList, index int, value json.RawMessage) error {
	if index < 0 || index > list.length {
		return fmt.Errorf(
			"want an index from 0 to %d for the new %s, got %d", list.length, list.noun, index)
	}
	at, err := decodeEntry(list, value)
	if err != nil {
		return err
	}
	text := entryFlow(at)
	if list.length == 0 {
		return s.doc.Upsert(yamledit.Path{}, list.key, "["+text+"]", storyboardKeyOrder)
	}
	return s.doc.InsertItem(yamledit.Path{list.key}, index, text)
}

// decodeEntry reads a new entry typed as {"kind", "at"}, refusing any other key, a missing one,
// a kind list does not take and an at that is not a moment.
func decodeEntry(list entryList, value json.RawMessage) (Moment, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var wire wireEntry
	err := decoder.Decode(&wire)
	isWhole := err == nil && wire.Kind != nil && len(wire.At) > 0
	if !isWhole || !slices.Contains(list.kinds, *wire.Kind) {
		return Moment{}, fmt.Errorf(
			`want a new %s such as {"kind": %q, "at": {"beat": 0, "offset_ms": 0}}, `+
				`its kind %s, got %s`,
			list.noun, list.kinds[0], strings.Join(list.kinds, " or "), value)
	}
	at, err := decodeMoment(wire.At)
	if err != nil {
		return Moment{}, fmt.Errorf("at: %w", err)
	}
	return at, nil
}

// entryFlow spells a new caption starting at as one flow mapping, from the presets. A start at
// the clip's first frame writes no at, which already means it.
func entryFlow(at Moment) string {
	start := ""
	if at != (Moment{}) {
		start = ", " + keyAt + ": " + momentFlow(at)
	}
	text := keyText + ": " + strconv.Quote(newCaptionText)
	return "{" + text + start + ", " + keyFor + ": " + newCaptionLength.String() + "}"
}

// removeEntry removes the entry at index from list, value typed as null; removing the list's
// only entry deletes the list's key.
func (s *splicer) removeEntry(list entryList, index int, value json.RawMessage) error {
	if !isNull(value) {
		return fmt.Errorf("want null, got %s", value)
	}
	if list.length == 1 {
		return s.doc.Delete(yamledit.Path{list.key})
	}
	return s.doc.DeleteItem(yamledit.Path{list.key}, index)
}

// moveEntry moves the entry at index in list so that it becomes the entry at the destination
// index value types, a whole number naming one of the list's entries.
func (s *splicer) moveEntry(list entryList, index int, value json.RawMessage) error {
	var destination int
	err := json.Unmarshal(value, &destination)
	if err != nil || destination < 0 || destination >= list.length {
		return fmt.Errorf("want a destination index from 0 to %d, got %s", list.length-1, value)
	}
	return s.doc.MoveItem(yamledit.Path{list.key}, index, destination)
}
