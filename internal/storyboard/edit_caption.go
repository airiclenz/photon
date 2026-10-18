package storyboard

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/airiclenz/photon/internal/yamledit"
)

// The caption fields an [Edit] names; each edits the caption its owner's index names.
const (
	FieldCaptionAt    = "caption.at"
	FieldCaptionFor   = "caption.for"
	FieldCaptionUntil = "caption.until"
	FieldCaptionText  = "caption.text"
	FieldCaptionPlace = "caption.place"
	FieldCaptionAlign = "caption.align"
)

// captionFieldPrefix starts the name of every caption field.
const captionFieldPrefix = "caption."

// The storyboard keys a caption edit addresses, past the at key a camera move shares.
const (
	keyCaptions = "captions"
	keyText     = "text"
	keyFor      = "for"
	keyUntil    = "until"
	keyPlace    = "place"
	keyAlign    = "align"
)

// captionKeyOrder is a caption's keys in its struct field order: a new key goes after whichever
// of the keys before it the caption holds.
var captionKeyOrder = keyOrder(reflect.TypeFor[Caption]())

// The spellings of a caption's place and align, as their errors offer them.
var (
	placeSpellings = []string{string(PlaceTop), string(PlaceMiddle), string(PlaceBottom)}
	alignSpellings = []string{string(AlignLeft), string(AlignMiddle), string(AlignRight)}
)

// isCaptionField reports whether field edits a caption rather than a beat, the frame or a
// camera move.
func isCaptionField(field string) bool {
	return strings.HasPrefix(field, captionFieldPrefix)
}

// editCaption sets field, typed as value, on the caption at index in board, the storyboard s.doc
// holds — its timing, text, place and align here, its offset and position by
// [splicer.setCaptionOffset] and [splicer.setCaptionPosition], its font, colours, box and fade by
// [splicer.setCaptionStyle] — or adds, removes or reorders a caption for a list field.
func (s *splicer) editCaption(
	board *Storyboard, index int, field string, value json.RawMessage,
) error {
	if isListField(field) {
		return s.editList(board, captionList(board), index, field, value)
	}
	if index < 0 || index >= len(board.Captions) {
		return fmt.Errorf("the storyboard has no caption at index %d", index)
	}
	caption := yamledit.Path{keyCaptions, strconv.Itoa(index)}

	switch field {
	case FieldCaptionAt:
		return s.setMoment(caption, captionKeyOrder, value)
	case FieldCaptionFor:
		return s.setCaptionFor(caption, value)
	case FieldCaptionUntil:
		return s.setCaptionUntil(caption, board.Captions[index], value)
	case FieldCaptionText:
		return s.setCaptionText(caption, value)
	case FieldCaptionPlace:
		return s.setPlacement(caption, keyPlace, value, placeSpellings, string(PlaceBottom))
	case FieldCaptionAlign:
		return s.setPlacement(caption, keyAlign, value, alignSpellings, string(AlignMiddle))
	case FieldCaptionOffset:
		return s.setCaptionOffset(caption, value)
	case FieldCaptionPosition:
		return s.setCaptionPosition(caption, value)
	case FieldCaptionFontSize, FieldCaptionFontWeight, FieldCaptionColor, FieldCaptionBoxColor,
		FieldCaptionBoxOpacity, FieldCaptionFadeIn, FieldCaptionFadeOut:
		return s.setCaptionStyle(caption, board.Frame.FontSize, field, value)
	default:
		return fmt.Errorf("unknown caption field %s", field)
	}
}

// setCaptionFor writes the caption's length and deletes its until, since a caption ends by
// exactly one of the two. A length of 0 is refused: it would leave the caption no end.
func (s *splicer) setCaptionFor(caption yamledit.Path, value json.RawMessage) error {
	length, err := decodeMilliseconds(value)
	if err != nil {
		return err
	}
	if length == 0 {
		return errors.New("want a length longer than 0ms")
	}
	return s.writeCaptionFor(caption, length)
}

// writeCaptionFor writes length as the caption's for and deletes its until.
func (s *splicer) writeCaptionFor(caption yamledit.Path, length time.Duration) error {
	if err := s.writeDuration(caption, keyFor, captionKeyOrder, 0, length); err != nil {
		return err
	}
	return s.doc.DeleteIfPresent(under(caption, keyUntil))
}

// setCaptionUntil replaces the caption's whole until, whatever its form, with the moment value
// types, and deletes its for, since a caption ends by exactly one of the two. A null clears the
// until: the caption then ends after a for of the length it shows now, the same stretch; a
// caption without an until is left as it is.
func (s *splicer) setCaptionUntil(
	caption yamledit.Path, shown Caption, value json.RawMessage,
) error {
	if isNull(value) {
		if shown.Until == nil {
			return nil
		}
		return s.writeCaptionFor(caption, (shown.Stop() - shown.Start()).Round(time.Millisecond))
	}
	moment, err := decodeMoment(value)
	if err != nil {
		return err
	}
	err = s.replaceOrInsert(caption, keyUntil, momentFlow(moment), captionKeyOrder)
	if err != nil {
		return err
	}
	return s.doc.DeleteIfPresent(under(caption, keyFor))
}

// setCaptionText writes value, typed as a JSON string, as the caption's text: one double-quoted
// line, its line breaks escaped as \n, in place of whatever form the old text took, a block
// scalar too.
func (s *splicer) setCaptionText(caption yamledit.Path, value json.RawMessage) error {
	text, err := decodeText(value)
	if err != nil {
		return err
	}
	return s.replaceOrInsert(caption, keyText, strconv.Quote(text), captionKeyOrder)
}

// setPlacement writes the caption's place or align key, value typed as a JSON string holding one
// of spellings, or deletes the key for absent, the value it takes when left out; either way it
// deletes the caption's position, which comes instead of place and align.
func (s *splicer) setPlacement(
	caption yamledit.Path,
	key string,
	value json.RawMessage,
	spellings []string,
	absent string,
) error {
	spelling, err := decodeSpelling(value, spellings)
	if err != nil {
		return err
	}
	if err := s.doc.DeleteIfPresent(under(caption, keyPosition)); err != nil {
		return err
	}
	if spelling == absent {
		return s.doc.DeleteIfPresent(under(caption, key))
	}
	return s.doc.Upsert(caption, key, spelling, captionKeyOrder)
}

// decodeSpelling reads a JSON string holding one of spellings.
func decodeSpelling(value json.RawMessage, spellings []string) (string, error) {
	var spelling string
	err := json.Unmarshal(value, &spelling)
	if err != nil || !slices.Contains(spellings, spelling) {
		last := len(spellings) - 1
		leading := strings.Join(spellings[:last], ", ")
		return "", fmt.Errorf("want %s or %s, got %s", leading, spellings[last], value)
	}
	return spelling, nil
}

// decodeText reads a caption's text, typed as a JSON string.
func decodeText(value json.RawMessage) (string, error) {
	var text string
	if err := json.Unmarshal(value, &text); err != nil {
		return "", fmt.Errorf("want the field's text as a JSON string, got %s", value)
	}
	return text, nil
}
