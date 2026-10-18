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

// The caption style fields an [Edit] names: a caption's font, its colours, its box and its fade.
const (
	FieldCaptionFontSize   = "caption.font.size"
	FieldCaptionFontWeight = "caption.font.weight"
	FieldCaptionColor      = "caption.color"
	FieldCaptionBoxColor   = "caption.box.color"
	FieldCaptionBoxOpacity = "caption.box.opacity"
	FieldCaptionFadeIn     = "caption.fade.in"
	FieldCaptionFadeOut    = "caption.fade.out"
)

// The caption placement fields an [Edit] names past place and align: the caption's offset from
// where those put it, and its position, which comes instead of all three.
const (
	FieldCaptionOffset   = "caption.offset"
	FieldCaptionPosition = "caption.position"
)

// The storyboard keys a caption style edit addresses: the caption's nested font, box and fade
// mappings and the keys in them, and its own color key, which the box shares.
const (
	keyFont    = "font"
	keySize    = "size"
	keyWeight  = "weight"
	keyColor   = "color"
	keyBox     = "box"
	keyOpacity = "opacity"
	keyFade    = "fade"
	keyIn      = "in"
	keyOut     = "out"
)

// The storyboard keys of a caption's offset and position.
const (
	keyOffset   = "offset"
	keyPosition = "position"
)

// positionReplaced is the caption keys a position comes instead of.
var positionReplaced = []string{keyPlace, keyAlign, keyOffset}

// The key orders of a caption's nested style mappings, in their struct field order.
var (
	fontKeyOrder = keyOrder(reflect.TypeFor[CaptionFont]())
	boxKeyOrder  = keyOrder(reflect.TypeFor[CaptionBox]())
	fadeKeyOrder = keyOrder(reflect.TypeFor[Fade]())
)

// styleSite is where a caption style field's key sits in its caption: in the nested mapping
// nested, whose keys follow order, or, with nested empty, in the caption itself.
type styleSite struct {
	nested string
	key    string
	order  []string
}

// styleSites places each caption style field's key.
var styleSites = map[string]styleSite{
	FieldCaptionFontSize:   {nested: keyFont, key: keySize, order: fontKeyOrder},
	FieldCaptionFontWeight: {nested: keyFont, key: keyWeight, order: fontKeyOrder},
	FieldCaptionColor:      {key: keyColor, order: captionKeyOrder},
	FieldCaptionBoxColor:   {nested: keyBox, key: keyColor, order: boxKeyOrder},
	FieldCaptionBoxOpacity: {nested: keyBox, key: keyOpacity, order: boxKeyOrder},
	FieldCaptionFadeIn:     {nested: keyFade, key: keyIn, order: fadeKeyOrder},
	FieldCaptionFadeOut:    {nested: keyFade, key: keyOut, order: fadeKeyOrder},
}

// setCaptionStyle sets field, a caption style field typed as value, on caption; frameFontSize is
// the frame's font size, the size a caption's font defaults to. A value equal to the field's
// default — the one [Load] fills in, a colour compared case-insensitively — deletes its key and
// then its font, box or fade mapping once it holds no key; any other value is written, a colour
// quoted, into that mapping, which is created as a one-key flow mapping when the caption lacks
// it.
func (s *splicer) setCaptionStyle(
	caption yamledit.Path,
	frameFontSize float64,
	field string,
	value json.RawMessage,
) error {
	site := styleSites[field]
	switch field {
	case FieldCaptionFontSize:
		size, err := decodeCaptionSize(value)
		if err != nil {
			return err
		}
		return s.writeStyle(caption, site, formatDecimal(size), size == frameFontSize)
	case FieldCaptionFontWeight:
		weight, err := decodeSpelling(value, weightChoices)
		if err != nil {
			return err
		}
		return s.writeStyle(caption, site, weight, weight == string(WeightRegular))
	case FieldCaptionColor, FieldCaptionBoxColor:
		return s.setStyleColor(caption, site, field, value)
	case FieldCaptionBoxOpacity:
		opacity, err := decodeOpacity(value)
		if err != nil {
			return err
		}
		return s.writeStyle(caption, site, formatDecimal(opacity), opacity == defaultBoxOpacity)
	default:
		return s.setStyleFade(caption, site, value)
	}
}

// setStyleColor writes the caption's text colour or its box's colour, value typed as a JSON
// string, quoted: a bare # starts a YAML comment.
func (s *splicer) setStyleColor(
	caption yamledit.Path,
	site styleSite,
	field string,
	value json.RawMessage,
) error {
	var color string
	if err := json.Unmarshal(value, &color); err != nil {
		return fmt.Errorf("want a colour such as \"#ffcc00\", got %s", value)
	}
	absent := defaultCaptionColor
	if field == FieldCaptionBoxColor {
		absent = defaultBoxColor
	}
	return s.writeStyle(caption, site, yamledit.Quote(color), strings.EqualFold(color, absent))
}

// setStyleFade writes the caption's fade in or out, a whole number of milliseconds; a present key
// keeps its unit.
func (s *splicer) setStyleFade(caption yamledit.Path, site styleSite, value json.RawMessage) error {
	length, err := decodeMilliseconds(value)
	if err != nil {
		return err
	}
	fade := under(caption, site.nested)
	if length == 0 || !s.isMapping(fade) {
		return s.writeStyle(caption, site, length.String(), length == 0)
	}
	return s.writeDuration(fade, site.key, site.order, 0, length)
}

// writeStyle writes text, a value's one line of YAML, as the site's key in caption, or, when
// isDefault, deletes the key and then a nested mapping left holding none.
func (s *splicer) writeStyle(
	caption yamledit.Path,
	site styleSite,
	text string,
	isDefault bool,
) error {
	if site.nested == "" {
		if isDefault {
			return s.doc.DeleteIfPresent(under(caption, site.key))
		}
		return s.doc.Upsert(caption, site.key, text, site.order)
	}

	mapping := under(caption, site.nested)
	switch {
	case isDefault && !s.isMapping(mapping):
		return s.doc.DeleteIfPresent(mapping)
	case isDefault:
		return s.deleteNestedKey(mapping, site)
	case !s.isMapping(mapping):
		return s.createNested(caption, site, text)
	default:
		return s.doc.Upsert(mapping, site.key, text, site.order)
	}
}

// createNested writes the site's nested mapping into caption as a one-key flow mapping holding
// text, in place of the null an empty `font:` holds: [yamledit.Document.Replace] cannot splice
// over a value with no text, so that key is deleted and inserted anew.
func (s *splicer) createNested(caption yamledit.Path, site styleSite, text string) error {
	if err := s.doc.DeleteIfPresent(under(caption, site.nested)); err != nil {
		return err
	}
	return s.doc.Upsert(caption, site.nested, "{"+site.key+": "+text+"}", captionKeyOrder)
}

// deleteNestedKey deletes the site's key from mapping, a caption's nested style mapping, and the
// mapping itself once it holds none of its keys.
func (s *splicer) deleteNestedKey(mapping yamledit.Path, site styleSite) error {
	if err := s.doc.DeleteIfPresent(under(mapping, site.key)); err != nil {
		return err
	}
	for _, key := range site.order {
		if s.doc.Has(under(mapping, key)) {
			return nil
		}
	}
	return s.doc.Delete(mapping)
}

// isMapping reports whether path holds a mapping: present and not a scalar, so not the null an
// empty `font:` holds.
func (s *splicer) isMapping(path yamledit.Path) bool {
	_, isScalar := s.doc.Scalar(path)
	return s.doc.Has(path) && !isScalar
}

// setCaptionOffset writes the caption's offset, value typed as design pixels, or deletes the key
// for {0, 0}, the offset it takes when left out; either way it deletes the caption's position,
// which comes instead of the offset.
func (s *splicer) setCaptionOffset(caption yamledit.Path, value json.RawMessage) error {
	offset, err := decodePixels(value)
	if err != nil {
		return err
	}
	if err := s.doc.DeleteIfPresent(under(caption, keyPosition)); err != nil {
		return err
	}
	if offset == (Pixels{}) {
		return s.doc.DeleteIfPresent(under(caption, keyOffset))
	}
	return s.writePixels(caption, keyOffset, offset)
}

// setCaptionPosition writes the caption's position, value typed as design pixels, and deletes
// its place, align and offset, which the position comes instead of. A null deletes the position:
// the caption then sits where an absent place and align put it, bottom middle.
func (s *splicer) setCaptionPosition(caption yamledit.Path, value json.RawMessage) error {
	if isNull(value) {
		return s.doc.DeleteIfPresent(under(caption, keyPosition))
	}
	position, err := decodePixels(value)
	if err != nil {
		return err
	}
	for _, key := range positionReplaced {
		if err := s.doc.DeleteIfPresent(under(caption, key)); err != nil {
			return err
		}
	}
	return s.writePixels(caption, keyPosition, position)
}

// writePixels writes pixels as the caption's key, a {x: 8, y: -4} flow mapping in place of
// whatever form the key held, or after the keys that precede it when the key is absent. A key
// holding no text, the null an empty `offset:` holds, is deleted and inserted anew:
// [yamledit.Document.Replace] cannot splice over it.
func (s *splicer) writePixels(caption yamledit.Path, key string, pixels Pixels) error {
	path := under(caption, key)
	if !s.isMapping(path) {
		if err := s.doc.DeleteIfPresent(path); err != nil {
			return err
		}
	}
	flow := "{x: " + strconv.Itoa(pixels.X) + ", y: " + strconv.Itoa(pixels.Y) + "}"
	return s.replaceOrInsert(caption, key, flow, captionKeyOrder)
}

// wirePixels is a caption's offset or position as an [Edit] types it, {"x": 8, "y": -4}: whole
// design pixels, X to the right and Y down. Both keys are required.
type wirePixels struct {
	X *int `json:"x"`
	Y *int `json:"y"`
}

// decodePixels reads design pixels typed as {"x", "y"}, refusing any other key, a missing one
// and a length that is not a whole number.
func decodePixels(value json.RawMessage) (Pixels, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var wire wirePixels
	if err := decoder.Decode(&wire); err != nil || wire.X == nil || wire.Y == nil {
		return Pixels{}, fmt.Errorf(
			`want whole design pixels such as {"x": 8, "y": -4}, got %s`, value)
	}
	return Pixels{X: *wire.X, Y: *wire.Y}, nil
}

// decodeCaptionSize reads a caption's font size, typed as a JSON number above 0; whether it lies
// in the caption bounds is the storyboard's rule to check.
func decodeCaptionSize(value json.RawMessage) (float64, error) {
	var size float64
	if err := json.Unmarshal(value, &size); err != nil || size <= 0 {
		return 0, fmt.Errorf("want a size above 0 such as 18, got %s", value)
	}
	return size, nil
}

// decodeOpacity reads a caption box's opacity, typed as a JSON number; whether it lies in [0, 1]
// is the storyboard's rule to check.
func decodeOpacity(value json.RawMessage) (float64, error) {
	var opacity float64
	if err := json.Unmarshal(value, &opacity); err != nil {
		return 0, fmt.Errorf("want an opacity from 0 to 1 such as 0.6, got %s", value)
	}
	return opacity, nil
}

// formatDecimal spells number in the fewest digits that read back as it: 18, 0.6.
func formatDecimal(number float64) string {
	return strconv.FormatFloat(number, 'f', -1, 64)
}
