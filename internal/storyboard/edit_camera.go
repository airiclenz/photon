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

// The camera position fields an [Edit] names; each edits the position its owner's index names.
const (
	FieldPositionAt     = "position.at"
	FieldPositionHold   = "position.hold"
	FieldPositionSpan   = "position.span"
	FieldPositionEase   = "position.ease"
	FieldPositionCell   = "position.cell"
	FieldPositionTarget = "position.target"
)

// The camera zoom fields an [Edit] names; each edits the zoom its owner's index names.
const (
	FieldZoomAt   = "zoom.at"
	FieldZoomOver = "zoom.over"
	FieldZoomEase = "zoom.ease"
	FieldZoomTo   = "zoom.to"
)

// The prefixes that start the name of every position field and of every zoom field.
const (
	positionFieldPrefix = "position."
	zoomFieldPrefix     = "zoom."
)

// The storyboard keys a camera edit addresses, past the at key [keyAt] and the text key
// [keyText].
const (
	keyCamera    = "camera"
	keyPositions = "positions"
	keyZooms     = "zooms"
	keyHold      = "hold"
	keyNth       = "nth"
	keyArea      = "area"
	keyCell      = "cell"
	keyEase      = "ease"
	keyOver      = "over"
	keyTo        = "to"
)

// positionKeyOrder is a position's keys in its struct field order: its point's — the target's
// text, nth and area, then cell — ahead of its at, hold and ease, as Point leads Position.
// keyOrder leaves an inline field out, so the order is put together here. zoomKeyOrder is a
// zoom's keys in its struct field order.
var (
	positionKeyOrder = slices.Concat(
		keyOrder(reflect.TypeFor[Target]()),
		keyOrder(reflect.TypeFor[Point]()),
		keyOrder(reflect.TypeFor[Position]()),
	)
	zoomKeyOrder = keyOrder(reflect.TypeFor[ZoomMove]())
)

// targetKeys are the keys of a position's text target, which its cell comes instead of.
var targetKeys = []string{keyText, keyNth, keyArea}

// isPositionField reports whether field edits a camera position.
func isPositionField(field string) bool {
	return strings.HasPrefix(field, positionFieldPrefix)
}

// isZoomField reports whether field edits a camera zoom.
func isZoomField(field string) bool {
	return strings.HasPrefix(field, zoomFieldPrefix)
}

// editPosition sets field, typed as value, on the camera position at index in board, the
// storyboard s.doc holds: its start, hold, both at once, ease, cell or target.
func (s *splicer) editPosition(
	board *Storyboard, index int, field string, value json.RawMessage,
) error {
	if index < 0 || index >= len(board.Camera.Positions) {
		return fmt.Errorf("the storyboard has no camera position at index %d", index)
	}
	position := yamledit.Path{keyCamera, keyPositions, strconv.Itoa(index)}

	switch field {
	case FieldPositionAt:
		return s.setMoment(position, positionKeyOrder, value)
	case FieldPositionHold:
		hold, err := decodeMilliseconds(value)
		if err != nil {
			return err
		}
		return s.writeHold(position, hold)
	case FieldPositionSpan:
		return s.setPositionSpan(position, value)
	case FieldPositionEase:
		return s.setEase(position, positionKeyOrder, value)
	case FieldPositionCell:
		return s.setPositionCell(position, value)
	case FieldPositionTarget:
		return s.setPositionTarget(position, value)
	default:
		return fmt.Errorf("unknown position field %s", field)
	}
}

// editZoom sets field, typed as value, on the camera zoom at index in board, the storyboard
// s.doc holds: its start, length, ease or factor.
func (s *splicer) editZoom(
	board *Storyboard, index int, field string, value json.RawMessage,
) error {
	if index < 0 || index >= len(board.Camera.Zooms) {
		return fmt.Errorf("the storyboard has no camera zoom at index %d", index)
	}
	zoom := yamledit.Path{keyCamera, keyZooms, strconv.Itoa(index)}

	switch field {
	case FieldZoomAt:
		return s.setMoment(zoom, zoomKeyOrder, value)
	case FieldZoomOver:
		length, err := decodeMilliseconds(value)
		if err != nil {
			return err
		}
		return s.writeDuration(zoom, keyOver, zoomKeyOrder, 0, length)
	case FieldZoomEase:
		return s.setEase(zoom, zoomKeyOrder, value)
	case FieldZoomTo:
		factor, err := decodeZoomFactor(value)
		if err != nil {
			return err
		}
		return s.doc.Upsert(zoom, keyTo, formatDecimal(factor), zoomKeyOrder)
	default:
		return fmt.Errorf("unknown zoom field %s", field)
	}
}

// writeHold writes hold as the position's hold, or deletes the key for 0, the hold an absent key
// means.
func (s *splicer) writeHold(position yamledit.Path, hold time.Duration) error {
	if hold == 0 {
		return s.doc.DeleteIfPresent(under(position, keyHold))
	}
	return s.writeDuration(position, keyHold, positionKeyOrder, 0, hold)
}

// wireSpan is a position's start and hold as an [Edit] types them at once, {"at": {"beat": 3,
// "offset_ms": 200}, "hold_ms": 600}. Both keys are required.
type wireSpan struct {
	At     json.RawMessage `json:"at"`
	HoldMS json.RawMessage `json:"hold_ms"`
}

// setPositionSpan writes the position's at and hold, typed together as value, as position.at
// and position.hold write them, once both check out.
func (s *splicer) setPositionSpan(position yamledit.Path, value json.RawMessage) error {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var wire wireSpan
	if err := decoder.Decode(&wire); err != nil || wire.At == nil || wire.HoldMS == nil {
		return fmt.Errorf(`want a span such as {"at": {"beat": 3, "offset_ms": 200}, `+
			`"hold_ms": 600}, got %s`, value)
	}
	if _, err := decodeMoment(wire.At); err != nil {
		return err
	}
	hold, err := decodeMilliseconds(wire.HoldMS)
	if err != nil {
		return err
	}
	if err := s.setMoment(position, positionKeyOrder, wire.At); err != nil {
		return err
	}
	return s.writeHold(position, hold)
}

// setEase writes the ease of mapping, a position or a zoom whose keys follow order, typed as a
// JSON string holding one of the four spellings, or deletes the key for smooth, the ease an
// absent key means.
func (s *splicer) setEase(mapping yamledit.Path, order []string, value json.RawMessage) error {
	spelling, err := decodeSpelling(value, easeChoices)
	if err != nil {
		return err
	}
	if Ease(spelling) == EaseSmooth {
		return s.doc.DeleteIfPresent(under(mapping, keyEase))
	}
	return s.doc.Upsert(mapping, keyEase, spelling, order)
}

// setPositionCell writes the position's cell, typed as {"x", "y"}, as a {x: 3, y: 4} flow
// mapping in place of its whole node, then deletes its text, nth and area, which the cell comes
// instead of. The cell goes in first: a block sequence item's only key cannot be deleted.
func (s *splicer) setPositionCell(position yamledit.Path, value json.RawMessage) error {
	cell, err := decodeCell(value)
	if err != nil {
		return err
	}
	flow := "{x: " + strconv.Itoa(cell.X) + ", y: " + strconv.Itoa(cell.Y) + "}"
	if err := s.replaceOrInsert(position, keyCell, flow, positionKeyOrder); err != nil {
		return err
	}
	for _, key := range targetKeys {
		if err := s.doc.DeleteIfPresent(under(position, key)); err != nil {
			return err
		}
	}
	return nil
}

// setPositionTarget writes the position's text target, typed as a pick answers it: its text
// quoted, its nth and area as [Target.YAMLFlow] spells them, deleting the nth for the first match
// and the area for none, then deletes its cell, which the target comes instead of. The text goes
// in first: a block sequence item's only key cannot be deleted.
func (s *splicer) setPositionTarget(position yamledit.Path, value json.RawMessage) error {
	target, err := decodeTarget(value)
	if err != nil {
		return err
	}
	nth := ""
	switch {
	case target.Nth == NthLast:
		nth = nthLastWord
	case target.Nth > NthFirst:
		nth = strconv.Itoa(int(target.Nth))
	}
	area := ""
	if target.Area != "" {
		area = yamledit.Quote(target.Area)
	}

	keys := map[string]string{keyText: yamledit.Quote(target.Text), keyNth: nth, keyArea: area}
	for _, key := range targetKeys {
		if err := s.writeTargetKey(position, key, keys[key]); err != nil {
			return err
		}
	}
	return s.doc.DeleteIfPresent(under(position, keyCell))
}

// writeTargetKey writes text as the position's key, or deletes the key when text is empty, the
// value an absent key means.
func (s *splicer) writeTargetKey(position yamledit.Path, key, text string) error {
	if text == "" {
		return s.doc.DeleteIfPresent(under(position, key))
	}
	return s.replaceOrInsert(position, key, text, positionKeyOrder)
}

// wireCell is a cell of the terminal grid as an [Edit] types it, {"x": 3, "y": 4}: column X and
// row Y, from 0. Both keys are required.
type wireCell struct {
	X *int `json:"x"`
	Y *int `json:"y"`
}

// decodeCell reads a cell typed as {"x", "y"}, refusing any other key, a missing one and a value
// that is not a whole number; whether the cell lies inside the frame is the storyboard's rule to
// check.
func decodeCell(value json.RawMessage) (Cell, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var wire wireCell
	if err := decoder.Decode(&wire); err != nil || wire.X == nil || wire.Y == nil {
		return Cell{}, fmt.Errorf(`want a cell such as {"x": 3, "y": 4}, got %s`, value)
	}
	return Cell{X: *wire.X, Y: *wire.Y}, nil
}

// decodeTarget reads a text target typed as a pick answers it, {"text": "ok", "nth": 2, "area":
// "list"}, its nth and area optional, refusing any other key, an nth [Nth.UnmarshalJSON] refuses
// and an empty text.
func decodeTarget(value json.RawMessage) (Target, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var target Target
	if err := decoder.Decode(&target); err != nil || target.Text == "" {
		return Target{}, fmt.Errorf(
			`want a target such as {"text": "ok", "nth": 2, "area": "list"}, got %s`, value)
	}
	return target, nil
}

// decodeZoomFactor reads a zoom's factor, typed as a JSON number; whether it lies in the zoom
// bounds is the storyboard's rule to check.
func decodeZoomFactor(value json.RawMessage) (float64, error) {
	var factor float64
	if err := json.Unmarshal(value, &factor); err != nil {
		return 0, fmt.Errorf("want a factor such as 1.5, got %s", value)
	}
	return factor, nil
}
