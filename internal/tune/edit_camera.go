package tune

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/yamledit"
)

// The camera move fields an Edit names; each edits the move its owner's index names.
const (
	fieldCameraAt   = "camera.at"
	fieldCameraOver = "camera.over"
	fieldCameraEase = "camera.ease"
	fieldCameraTo   = "camera.to"
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
	zoomMoveKeyOrder = []string{"to", "at", "over", "ease"}
	panMoveKeyOrder  = []string{"to", "via", "at", "over", "ease"}
)

// isCameraField reports whether field edits a camera move rather than a beat or the frame.
func isCameraField(field string) bool {
	return strings.HasPrefix(field, cameraFieldPrefix)
}

// editCamera sets field, spelled as value, on the move at index in board, the storyboard s.doc
// holds.
func (s *splicer) editCamera(board *storyboard.Storyboard, index int, field, value string) error {
	if index < 0 || index >= len(board.Camera) {
		return fmt.Errorf("the storyboard has no camera move at index %d", index)
	}
	camera := board.Camera[index]
	kind, order := keyZoom, zoomMoveKeyOrder
	if camera.Pan != nil {
		kind, order = keyPan, panMoveKeyOrder
	}
	move := yamledit.Path{keyCamera, strconv.Itoa(index), kind}

	switch field {
	case fieldCameraAt:
		return s.setMoment(move, order, value)
	case fieldCameraOver:
		length, err := parseMilliseconds(value)
		if err != nil {
			return err
		}
		return s.writeDuration(move, keyOver, order, 0, length)
	case fieldCameraEase:
		return s.setEase(move, order, value)
	default:
		if camera.Zoom != nil {
			return s.setZoomTo(move, value)
		}
		return s.setPanTo(move, camera.Pan.Via != nil, value)
	}
}

// setMoment replaces a camera move's whole at, whatever its form, with the moment value spells.
// A clip start on a move without at writes nothing: an absent at already starts the move there.
func (s *splicer) setMoment(move yamledit.Path, order []string, value string) error {
	text, moment, err := parseMoment(value)
	if err != nil {
		return err
	}
	if moment == (storyboard.Moment{}) && !s.doc.Has(under(move, keyAt)) {
		return nil
	}
	return s.replaceOrInsert(move, keyAt, text, order)
}

// setEase writes `ease: linear`, or deletes the key for smooth, the ease an absent key means.
func (s *splicer) setEase(move yamledit.Path, order []string, value string) error {
	switch storyboard.Ease(value) {
	case storyboard.EaseSmooth:
		return s.doc.DeleteIfPresent(under(move, keyEase))
	case storyboard.EaseLinear:
		return s.doc.Upsert(move, keyEase, value, order)
	default:
		return fmt.Errorf("want %s or %s, got %q", storyboard.EaseSmooth, storyboard.EaseLinear, value)
	}
}

// setZoomTo writes a zoom move's factor.
func (s *splicer) setZoomTo(move yamledit.Path, value string) error {
	factor, err := parseDecimal(value)
	if err != nil {
		return err
	}
	return s.doc.Upsert(move, keyTo, strconv.FormatFloat(factor, 'f', -1, 64), zoomMoveKeyOrder)
}

// setPanTo replaces a straight pan's destination with the target or cell value spells. A pan
// along a via path is refused: its waypoints are edited in the storyboard itself.
func (s *splicer) setPanTo(move yamledit.Path, isVia bool, value string) error {
	if isVia {
		return errors.New("the pan follows a via path, which tune does not edit; edit it in the storyboard")
	}
	point, err := parsePoint(value)
	if err != nil {
		return err
	}
	return s.doc.Replace(under(move, keyTo), point)
}
