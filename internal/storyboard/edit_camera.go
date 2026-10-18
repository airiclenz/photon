package storyboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/airiclenz/photon/internal/yamledit"
)

// The camera move fields an [Edit] names; each edits the move its owner's index names.
const (
	FieldCameraAt   = "camera.at"
	FieldCameraOver = "camera.over"
	FieldCameraEase = "camera.ease"
	FieldCameraTo   = "camera.to"
)

// cameraFieldPrefix starts the name of every camera move field.
const cameraFieldPrefix = "camera."

// The storyboard keys a camera move edit addresses, past a zoom move's own keyZoom.
const (
	keyCamera = "camera"
	keyPan    = "pan"
	keyTo     = "to"
	keyAt     = "at"
	keyOver   = "over"
	keyEase   = "ease"
)

// zoomMoveKeyOrder and panMoveKeyOrder are a camera move's keys in its struct field order: a
// new key goes after whichever of the keys before it the move holds.
var (
	zoomMoveKeyOrder = keyOrder(reflect.TypeFor[ZoomMove]())
	panMoveKeyOrder  = keyOrder(reflect.TypeFor[PanMove]())
)

// isCameraField reports whether field edits a camera move rather than a beat, the frame or a
// caption.
func isCameraField(field string) bool {
	return strings.HasPrefix(field, cameraFieldPrefix)
}

// editCamera sets field, typed as value, on the move at index in board, the storyboard s.doc
// holds, edits a pan's via path for a via field, or adds, removes or reorders a move for a list
// field.
func (s *splicer) editCamera(board *Storyboard, index int, field string, value json.RawMessage) error {
	if isListField(field) {
		return s.editList(board, cameraList(board), index, field, value)
	}
	if index < 0 || index >= len(board.Camera) {
		return fmt.Errorf("the storyboard has no camera move at index %d", index)
	}
	camera := board.Camera[index]
	kind, order := keyZoom, zoomMoveKeyOrder
	if camera.Pan != nil {
		kind, order = keyPan, panMoveKeyOrder
	}
	move := yamledit.Path{keyCamera, strconv.Itoa(index), kind}
	if isViaField(field) {
		return s.editVia(board.Frame, camera.Pan, move, field, value)
	}

	switch field {
	case FieldCameraAt:
		return s.setMoment(move, order, value)
	case FieldCameraOver:
		length, err := decodeMilliseconds(value)
		if err != nil {
			return err
		}
		return s.writeDuration(move, keyOver, order, 0, length)
	case FieldCameraEase:
		return s.setEase(move, order, value)
	default:
		if camera.Zoom != nil {
			return s.setZoomTo(move, value)
		}
		return s.setPanTo(move, camera.Pan.Via != nil, value)
	}
}

// setEase writes `ease: linear`, or deletes the key for smooth, the ease an absent key means.
func (s *splicer) setEase(move yamledit.Path, order []string, value json.RawMessage) error {
	var ease Ease
	if err := json.Unmarshal(value, &ease); err == nil {
		switch ease {
		case EaseSmooth:
			return s.doc.DeleteIfPresent(under(move, keyEase))
		case EaseLinear:
			return s.doc.Upsert(move, keyEase, string(ease), order)
		}
	}
	return fmt.Errorf("want %s or %s, got %s", EaseSmooth, EaseLinear, value)
}

// setZoomTo writes a zoom move's factor.
func (s *splicer) setZoomTo(move yamledit.Path, value json.RawMessage) error {
	var factor float64
	if err := json.Unmarshal(value, &factor); err != nil {
		return fmt.Errorf("want a number such as 1.8, got %s", value)
	}
	return s.doc.Upsert(move, keyTo, strconv.FormatFloat(factor, 'f', -1, 64), zoomMoveKeyOrder)
}

// setPanTo replaces a straight pan's destination with the target or cell value types. A pan
// along a via path is refused: its waypoints are edited by camera.via, and camera.path turns it
// into a straight pan.
func (s *splicer) setPanTo(move yamledit.Path, isVia bool, value json.RawMessage) error {
	if isVia {
		return fmt.Errorf("the pan follows a via path: set its waypoints with %s, "+
			"or make it a straight pan with %s", FieldCameraVia, FieldCameraPath)
	}
	point, err := decodePoint(value)
	if err != nil {
		return err
	}
	return s.doc.Replace(under(move, keyTo), point)
}

// wireCell is a cell of the terminal grid as an [Edit] types it, {"x": 3, "y": 4}.
type wireCell struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// wirePoint is a pan's destination as an [Edit] types it: a target as a zoom.target takes it,
// {"text", "nth", "area"}, or a cell, {"cell": {"x": 3, "y": 4}} — one of the two.
type wirePoint struct {
	Target
	Cell *wireCell `json:"cell"`
}

// decodePoint reads a pan's target or cell, refusing any other key and both at once, and spells
// it as [pointFlow] does.
func decodePoint(value json.RawMessage) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var wire wirePoint
	err := decoder.Decode(&wire)
	if err == nil && wire.Cell != nil && wire.Target != (Target{}) {
		err = errors.New("a target and a cell at once")
	}
	if err != nil {
		return "", fmt.Errorf(`want a target such as {"text": "ok", "nth": 2, "area": "list"} `+
			`or a cell such as {"cell": {"x": 3, "y": 4}}, got %s: %w`, value, err)
	}
	point := Point{Target: wire.Target}
	if wire.Cell != nil {
		point = Point{Cell: &Cell{X: wire.Cell.X, Y: wire.Cell.Y}}
	}
	return pointFlow(point), nil
}

// pointFlow spells point as the YAML flow mapping a pan's to or waypoint is written as: a target
// as [Target.YAMLFlow] does, a cell as {cell: {x: 3, y: 4}}.
func pointFlow(point Point) string {
	if point.Cell == nil {
		return point.Target.YAMLFlow()
	}
	cell := "{x: " + strconv.Itoa(point.Cell.X) + ", y: " + strconv.Itoa(point.Cell.Y) + "}"
	return "{cell: " + cell + "}"
}

// centreCell is frame's centre cell, {x: cols/2, y: rows/2}: where a new pan aims, and the
// waypoint a straight pan made a via path passes first.
func centreCell(frame Frame) Point {
	return Point{Cell: &Cell{X: frame.Cols / 2, Y: frame.Rows / 2}}
}
