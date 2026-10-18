package tune

import (
	"encoding/json"
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

// editCamera sets field, typed as value, on the move at index in board, the storyboard s.doc
// holds.
func (s *splicer) editCamera(
	board *storyboard.Storyboard,
	index int,
	field string,
	value json.RawMessage,
) error {
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
		return s.setMoveAt(move, order, value)
	case fieldCameraOver:
		length, err := decodeMilliseconds(value)
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

// setMoveAt replaces a camera move's whole at, whatever its form, with the moment value types.
func (s *splicer) setMoveAt(move yamledit.Path, order []string, value json.RawMessage) error {
	moment, err := decodeMoment(value)
	if err != nil {
		return err
	}
	return s.writeMoment(move, order, momentFlow(moment), moment)
}

// setMoment replaces a caption's whole at, whatever its form, with the moment value spells as
// one line of YAML.
func (s *splicer) setMoment(mapping yamledit.Path, order []string, value string) error {
	text, moment, err := parseMoment(value)
	if err != nil {
		return err
	}
	return s.writeMoment(mapping, order, text, moment)
}

// writeMoment puts text, moment's one line of YAML, in place of the whole at of mapping. A clip
// start on a mapping without at writes nothing: an absent at already starts it there.
func (s *splicer) writeMoment(
	mapping yamledit.Path,
	order []string,
	text string,
	moment storyboard.Moment,
) error {
	if moment == (storyboard.Moment{}) && !s.doc.Has(under(mapping, keyAt)) {
		return nil
	}
	return s.replaceOrInsert(mapping, keyAt, text, order)
}

// momentFlow spells moment as the storyboard writes one: a clip time in milliseconds (4500ms),
// or a beat's {beat: 3, offset: 200ms} with an offset of 0 left out.
func momentFlow(moment storyboard.Moment) string {
	offset := strconv.FormatInt(moment.Offset.Milliseconds(), 10) + "ms"
	if moment.Beat == 0 {
		return offset
	}
	beat := "beat: " + strconv.Itoa(moment.Beat)
	if moment.Offset == 0 {
		return "{" + beat + "}"
	}
	return "{" + beat + ", offset: " + offset + "}"
}

// setEase writes `ease: linear`, or deletes the key for smooth, the ease an absent key means.
func (s *splicer) setEase(move yamledit.Path, order []string, value json.RawMessage) error {
	var ease storyboard.Ease
	if err := json.Unmarshal(value, &ease); err == nil {
		switch ease {
		case storyboard.EaseSmooth:
			return s.doc.DeleteIfPresent(under(move, keyEase))
		case storyboard.EaseLinear:
			return s.doc.Upsert(move, keyEase, string(ease), order)
		}
	}
	return fmt.Errorf("want %s or %s, got %s", storyboard.EaseSmooth, storyboard.EaseLinear, value)
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
// along a via path is refused: its waypoints are edited in the storyboard itself.
func (s *splicer) setPanTo(move yamledit.Path, isVia bool, value json.RawMessage) error {
	if isVia {
		return errors.New("the pan follows a via path, which tune does not edit; edit it in the storyboard")
	}
	point, err := decodePoint(value)
	if err != nil {
		return err
	}
	return s.doc.Replace(under(move, keyTo), point)
}
