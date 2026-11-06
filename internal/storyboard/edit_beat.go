package storyboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/airiclenz/photon/internal/yamledit"
)

// The storyboard keys a beat or frame edit addresses directly; a field's own key is its name
// past the dot.
const (
	keyBeats       = "beats"
	keyFrame       = "frame"
	keyCut         = "cut"
	keyViewport    = "viewport"
	keyDeviceScale = "device_scale"
)

// valueTrue is the YAML text of cut switched on.
const valueTrue = "true"

// nthLastWord spells the last match's nth, in YAML and on the wire alike.
const nthLastWord = "last"

// beatKeyOrder and frameKeyOrder are a beat's and the frame's keys in their struct field order,
// which a new key is inserted by.
var (
	beatKeyOrder  = keyOrder(reflect.TypeFor[Beat]())
	frameKeyOrder = keyOrder(reflect.TypeFor[Frame]())
)

// editFrame sets one of the frame's edit fields: frame.font_size or frame.line_height, a decimal,
// frame.cols, frame.rows, frame.padding, frame.scale, frame.width, frame.fps or
// frame.max_colors, a whole number, or frame.viewport or frame.device_scale.
func (s *splicer) editFrame(edit Edit) error {
	if edit.Owner.Index != 0 {
		return fmt.Errorf("want the clip at index 0, got index %d", edit.Owner.Index)
	}
	switch edit.Field {
	case FieldFrameViewport:
		return s.setViewport(edit.Value)
	case FieldFrameDeviceScale:
		return s.setDeviceScale(edit.Value)
	}
	var text string
	if edit.Field == FieldFrameFontSize || edit.Field == FieldFrameLineHeight {
		number, err := decodeFrameDecimal(edit.Value)
		if err != nil {
			return err
		}
		text = formatDecimal(number)
	} else {
		count, err := decodeCount(edit.Value)
		if err != nil {
			return err
		}
		text = strconv.Itoa(count)
	}
	key := strings.TrimPrefix(edit.Field, keyFrame+".")
	return s.doc.Upsert(yamledit.Path{keyFrame}, key, text, frameKeyOrder)
}

// setViewport writes frame.viewport as a {width: 1280, height: 800} flow mapping in place of its
// whole node, or deletes the key for null.
func (s *splicer) setViewport(value json.RawMessage) error {
	path := under(yamledit.Path{keyFrame}, keyViewport)
	if isNull(value) {
		return s.doc.DeleteIfPresent(path)
	}
	viewport, err := decodeViewport(value)
	if err != nil {
		return err
	}
	return s.replaceOrInsert(yamledit.Path{keyFrame}, keyViewport, viewport.flow(), frameKeyOrder)
}

// setDeviceScale writes frame.device_scale, or deletes the key for [DefaultDeviceScale], the
// scale an absent key means.
func (s *splicer) setDeviceScale(value json.RawMessage) error {
	scale, err := decodeFrameDecimal(value)
	if err != nil {
		return err
	}
	if scale == DefaultDeviceScale {
		return s.doc.DeleteIfPresent(under(yamledit.Path{keyFrame}, keyDeviceScale))
	}
	return s.doc.Upsert(yamledit.Path{keyFrame}, keyDeviceScale, formatDecimal(scale),
		frameKeyOrder)
}

// flow spells the viewport as the storyboard writes one, a {width: 1280, height: 800} flow
// mapping.
func (v Viewport) flow() string {
	return "{width: " + strconv.Itoa(v.Width) + ", height: " + strconv.Itoa(v.Height) + "}"
}

// wireViewport is a viewport as an [Edit] types it; a nil field is one the value left out.
type wireViewport struct {
	Width  *int `json:"width"`
	Height *int `json:"height"`
}

// decodeViewport reads frame.viewport typed as {"width": 1280, "height": 800}, both whole numbers
// of at least 1, refusing any other key.
func decodeViewport(value json.RawMessage) (Viewport, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var wire wireViewport
	err := decoder.Decode(&wire)
	if err != nil || wire.Width == nil || wire.Height == nil ||
		*wire.Width < 1 || *wire.Height < 1 {
		return Viewport{}, fmt.Errorf(
			`want a viewport in CSS pixels such as {"width": 1280, "height": 800}, got %s`, value)
	}
	return Viewport{Width: *wire.Width, Height: *wire.Height}, nil
}

// editBeat applies a beat field's edit, its duration, hold or cut, to the beat at beat.
func (s *splicer) editBeat(beat yamledit.Path, edit Edit) error {
	if edit.Field == FieldCut {
		return s.setCut(beat, edit.Value)
	}
	length, err := decodeMilliseconds(edit.Value)
	if err != nil {
		return err
	}
	return s.writeDuration(beat, edit.Field, beatKeyOrder, 0, length)
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

// YAMLFlow writes t as a YAML flow mapping, the one spelling of a target that photon tune's pick
// snippets write: the last match is nth: last, and an nth of first and an empty area are left
// out.
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

// decodeCount reads a frame field typed as a whole JSON number.
func decodeCount(value json.RawMessage) (int, error) {
	var count int
	err := json.Unmarshal(value, &count)
	if err != nil || count < 0 {
		return 0, fmt.Errorf("want a whole number, got %s", value)
	}
	return count, nil
}

// decodeFrameDecimal reads frame.font_size or frame.line_height, typed as a JSON number above 0;
// whether the font size lies in the caption bounds is the storyboard's rule to check.
func decodeFrameDecimal(value json.RawMessage) (float64, error) {
	var number float64
	if err := json.Unmarshal(value, &number); err != nil || number <= 0 {
		return 0, fmt.Errorf("want a number above 0, got %s", value)
	}
	return number, nil
}

// decodeBool reads cut typed as a JSON bool.
func decodeBool(value json.RawMessage) (bool, error) {
	var isOn bool
	if err := json.Unmarshal(value, &isOn); err != nil {
		return false, fmt.Errorf("want true or false, got %s", value)
	}
	return isOn, nil
}
