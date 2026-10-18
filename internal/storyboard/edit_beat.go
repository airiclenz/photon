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

// The storyboard keys a beat or frame edit addresses directly; a field's own key is its name
// past the dot.
const (
	keyBeats  = "beats"
	keyFrame  = "frame"
	keyCut    = "cut"
	keyZoom   = "zoom"
	keyTarget = "target"
	keyFactor = "factor"
)

// valueTrue is the YAML text of cut switched on.
const valueTrue = "true"

// nthLastWord spells the last match's nth, in YAML and on the wire alike.
const nthLastWord = "last"

// beatKeyOrder, zoomKeyOrder and frameKeyOrder are a beat's, a zoom's and the frame's keys in
// their struct field order, which a new key is inserted by.
var (
	beatKeyOrder  = keyOrder(reflect.TypeFor[Beat]())
	zoomKeyOrder  = keyOrder(reflect.TypeFor[Zoom]())
	frameKeyOrder = keyOrder(reflect.TypeFor[Frame]())
)

// editFrame sets frame.width or frame.fps.
func (s *splicer) editFrame(edit Edit) error {
	if edit.Owner.Index != 0 {
		return fmt.Errorf("want the clip at index 0, got index %d", edit.Owner.Index)
	}
	count, err := decodeCount(edit.Value)
	if err != nil {
		return err
	}
	key := strings.TrimPrefix(edit.Field, keyFrame+".")
	return s.doc.Upsert(yamledit.Path{keyFrame}, key, strconv.Itoa(count), frameKeyOrder)
}

// editBeat applies a beat field's edit to the beat at beat; hasZoom says whether it zooms.
func (s *splicer) editBeat(beat yamledit.Path, hasZoom bool, edit Edit) error {
	switch edit.Field {
	case FieldDuration, FieldHold:
		length, err := decodeMilliseconds(edit.Value)
		if err != nil {
			return err
		}
		return s.writeDuration(beat, edit.Field, beatKeyOrder, 0, length)
	case FieldCut:
		return s.setCut(beat, edit.Value)
	case FieldZoom:
		return s.addOrRemoveZoom(beat, hasZoom, edit.Value)
	}

	if !hasZoom {
		return errors.New("the beat has no zoom; add one first")
	}
	zoom := under(beat, keyZoom)
	switch edit.Field {
	case FieldZoomTarget:
		return s.replaceTarget(beat, edit.Value)
	case FieldZoomFactor:
		return s.setFactor(zoom, edit.Value)
	default:
		return s.setRamp(zoom, strings.TrimPrefix(edit.Field, keyZoom+"."), edit.Value)
	}
}

// setRamp writes a zoom's in or out ramp. A ramp of 0 is refused: an absent ramp already runs
// the default, and an explicit 0 would cut the push in or out.
func (s *splicer) setRamp(zoom yamledit.Path, key string, value json.RawMessage) error {
	length, err := decodeMilliseconds(value)
	if err != nil {
		return err
	}
	if length == 0 {
		return fmt.Errorf(
			"want a ramp longer than 0ms; a ramp left out runs the %s default", DefaultZoomRamp)
	}
	return s.writeDuration(zoom, key, zoomKeyOrder, DefaultZoomRamp, length)
}

// setCut writes `cut: true` or deletes the key for false.
func (s *splicer) setCut(beat yamledit.Path, value json.RawMessage) error {
	isCut, err := decodeBool(value)
	if err != nil {
		return err
	}
	if isCut {
		return s.doc.Upsert(beat, keyCut, valueTrue, beatKeyOrder)
	}
	return s.doc.DeleteIfPresent(under(beat, keyCut))
}

// setFactor writes a zoom's factor, or deletes the key for 0, auto.
func (s *splicer) setFactor(zoom yamledit.Path, value json.RawMessage) error {
	factor, err := decodeFactor(value)
	if err != nil {
		return err
	}
	if factor == 0 {
		return s.doc.DeleteIfPresent(under(zoom, keyFactor))
	}
	return s.doc.Upsert(zoom, keyFactor, strconv.FormatFloat(factor, 'f', -1, 64), zoomKeyOrder)
}

// addOrRemoveZoom adds a zoom onto the target value holds, or removes the beat's zoom when
// value is null.
func (s *splicer) addOrRemoveZoom(beat yamledit.Path, hasZoom bool, value json.RawMessage) error {
	if isNull(value) {
		if !hasZoom {
			return errors.New("the beat has no zoom to remove")
		}
		return s.doc.Delete(under(beat, keyZoom))
	}
	if hasZoom {
		return errors.New("the beat already zooms; edit its target instead")
	}
	target, err := decodeTarget(value)
	if err != nil {
		return err
	}
	return s.doc.Upsert(beat, keyZoom, zoomOnto(target.YAMLFlow()), beatKeyOrder)
}

// replaceTarget swaps the zoom's target for the one value holds, in its place. A zoom holding
// nothing but its target is replaced whole by a flow zoom onto the new target, the form a zoom
// is added in.
func (s *splicer) replaceTarget(beat yamledit.Path, value json.RawMessage) error {
	decoded, err := decodeTarget(value)
	if err != nil {
		return err
	}
	target := decoded.YAMLFlow()
	zoom := under(beat, keyZoom)
	isOnlyTarget := true
	for _, key := range zoomKeyOrder {
		isOnlyTarget = isOnlyTarget && (key == keyTarget || !s.doc.Has(under(zoom, key)))
	}
	if isOnlyTarget {
		return s.doc.Replace(zoom, zoomOnto(target))
	}
	return s.doc.Replace(under(zoom, keyTarget), target)
}

// zoomOnto spells a zoom onto target, a target's flow text, with every other key left out.
func zoomOnto(target string) string {
	return "{" + keyTarget + ": " + target + "}"
}

// YAMLFlow writes t as a YAML flow mapping, the one spelling of a target that photon tune's pick
// snippets and a zoom or zoom.target [Edit] write: the last match is nth: last, and an nth of
// first and an empty area are left out.
func (t Target) YAMLFlow() string {
	fields := []string{"text: " + yamledit.Quote(t.Text)}
	switch {
	case t.Nth == NthLast:
		fields = append(fields, "nth: "+nthLastWord)
	case t.Nth > NthFirst:
		fields = append(fields, "nth: "+strconv.Itoa(int(t.Nth)))
	}
	if t.Area != "" {
		fields = append(fields, "area: "+yamledit.Quote(t.Area))
	}
	return "{" + strings.Join(fields, ", ") + "}"
}

// decodeTarget reads a target typed as a pick answers it, {"text", "nth", "area"}, refusing any
// other key.
func decodeTarget(value json.RawMessage) (Target, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var target Target
	if err := decoder.Decode(&target); err != nil {
		return Target{}, fmt.Errorf(
			`want a target such as {"text": "ok", "nth": 2, "area": "list"}, `+
				`got %s: %w`, value, err)
	}
	return target, nil
}

// decodeCount reads a frame field typed as a whole JSON number.
func decodeCount(value json.RawMessage) (int, error) {
	var count int
	err := json.Unmarshal(value, &count)
	if err != nil || count < 0 {
		return 0, fmt.Errorf("want a whole number, got %s", value)
	}
	return count, nil
}

// decodeBool reads cut typed as a JSON bool.
func decodeBool(value json.RawMessage) (bool, error) {
	var isOn bool
	if err := json.Unmarshal(value, &isOn); err != nil {
		return false, fmt.Errorf("want true or false, got %s", value)
	}
	return isOn, nil
}

// decodeFactor reads a zoom factor typed as a JSON number, 0 for auto.
func decodeFactor(value json.RawMessage) (float64, error) {
	var factor float64
	err := json.Unmarshal(value, &factor)
	if err != nil || factor < 0 {
		return 0, fmt.Errorf("want a number above 0, or 0 for auto, got %s", value)
	}
	return factor, nil
}
