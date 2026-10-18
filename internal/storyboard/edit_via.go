package storyboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/airiclenz/photon/internal/yamledit"
)

// The via fields an [Edit] names: each edits the path of the pan its owner's index names —
// camera.via, camera.via.insert and camera.via.remove one waypoint of its via path, by the index
// their value names, and camera.path whether it moves straight to its to or along a via path.
const (
	FieldCameraVia       = "camera.via"
	FieldCameraViaInsert = "camera.via.insert"
	FieldCameraViaRemove = "camera.via.remove"
	FieldCameraPath      = "camera.path"
)

// keyVia is a pan's via path key.
const keyVia = "via"

// The paths a pan moves along, as camera.path types them: straight to its to, or along its via.
const (
	pathTo  = "to"
	pathVia = "via"
)

// pathChoices are the paths camera.path takes, in the order the page offers them.
var pathChoices = []string{pathTo, pathVia}

// viaFields are the via fields, which edit a pan's path rather than its timing.
var viaFields = []string{
	FieldCameraVia, FieldCameraViaInsert, FieldCameraViaRemove, FieldCameraPath,
}

// isViaField reports whether field edits a pan's path: a waypoint of its via path, or the path.
func isViaField(field string) bool {
	return slices.Contains(viaFields, field)
}

// wireWaypoint is a waypoint edit's value as an [Edit] types it, {"index": 1, "point": {"cell":
// {"x": 3, "y": 4}}}: the index of the waypoint it edits and, but for a remove, the point it
// writes there, a target or a cell as camera.to takes it.
type wireWaypoint struct {
	Index *int            `json:"index"`
	Point json.RawMessage `json:"point"`
}

// editVia applies field, a via field typed as value, to pan, the move at move in the storyboard
// s.doc holds, nil for a zoom move, which has no path; frame places the centre cell a straight
// pan made a via path passes first. A waypoint edit takes a via pan: a set or remove names one
// of its waypoints, an insert an index from 0 through their count.
func (s *splicer) editVia(
	frame Frame,
	pan *PanMove,
	move yamledit.Path,
	field string,
	value json.RawMessage,
) error {
	if pan == nil {
		return errors.New("a zoom move has no path; only a pan moves along one")
	}
	if field == FieldCameraPath {
		return s.setPath(frame, *pan, move, value)
	}
	if pan.Via == nil {
		return fmt.Errorf("the pan moves straight to its to; make it a via path with %s first",
			FieldCameraPath)
	}
	index, point, err := decodeWaypoint(field, value)
	if err != nil {
		return err
	}
	via := under(move, keyVia)
	switch {
	case field == FieldCameraViaInsert:
		if index < 0 || index > len(pan.Via) {
			return fmt.Errorf(
				"want an index from 0 to %d for the new waypoint, got %d", len(pan.Via), index)
		}
		return s.doc.InsertItem(via, index, point)
	case index < 0 || index >= len(pan.Via):
		return fmt.Errorf("the pan's via path has no waypoint at index %d", index)
	case field == FieldCameraViaRemove:
		return s.doc.DeleteItem(via, index)
	default:
		return s.doc.Replace(under(via, strconv.Itoa(index)), point)
	}
}

// decodeWaypoint reads a waypoint edit typed as {"index", "point"}, or a remove's {"index"},
// refusing any other key, a missing one and a point that is not a target or cell, and returns
// the index and the point spelled as [pointFlow] does, empty for a remove.
func decodeWaypoint(field string, value json.RawMessage) (int, string, error) {
	isRemove := field == FieldCameraViaRemove
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var wire wireWaypoint
	err := decoder.Decode(&wire)
	isWhole := err == nil && wire.Index != nil && (len(wire.Point) == 0) == isRemove
	switch {
	case !isWhole && isRemove:
		return 0, "", fmt.Errorf(`want a waypoint index such as {"index": 1}, got %s`, value)
	case !isWhole:
		return 0, "", fmt.Errorf(`want a waypoint such as {"index": 1, "point": `+
			`{"cell": {"x": 3, "y": 4}}}, got %s`, value)
	case isRemove:
		return *wire.Index, "", nil
	}
	point, err := decodePoint(wire.Point)
	if err != nil {
		return 0, "", fmt.Errorf("point: %w", err)
	}
	return *wire.Index, point, nil
}

// setPath makes pan, the move at move in frame, follow the path value types: a straight pan
// made a via path passes frame's centre cell on its way to its old to, and a via pan made
// straight moves to its last waypoint. The path the pan already follows writes nothing.
func (s *splicer) setPath(
	frame Frame,
	pan PanMove,
	move yamledit.Path,
	value json.RawMessage,
) error {
	var path string
	if err := json.Unmarshal(value, &path); err != nil || !slices.Contains(pathChoices, path) {
		return fmt.Errorf("want %s or %s, got %s", pathTo, pathVia, value)
	}
	switch {
	case path == pathVia && pan.To != nil:
		via := "[" + pointFlow(centreCell(frame)) + ", " + pointFlow(*pan.To) + "]"
		return s.swapPath(move, keyVia, via, keyTo)
	case path == pathTo && pan.Via != nil:
		return s.swapPath(move, keyTo, pointFlow(pan.Via[len(pan.Via)-1]), keyVia)
	default:
		return nil
	}
}

// swapPath writes `key: text` into the pan at move, in its key order, then deletes its old key.
func (s *splicer) swapPath(move yamledit.Path, key, text, old string) error {
	if err := s.doc.Upsert(move, key, text, panMoveKeyOrder); err != nil {
		return err
	}
	return s.doc.Delete(under(move, old))
}
