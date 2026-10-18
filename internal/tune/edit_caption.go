package tune

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/yamledit"
)

// The caption fields an Edit names; each edits the caption its owner's index names.
const (
	fieldCaptionAt    = "caption.at"
	fieldCaptionFor   = "caption.for"
	fieldCaptionUntil = "caption.until"
	fieldCaptionText  = "caption.text"
	fieldCaptionPlace = "caption.place"
	fieldCaptionAlign = "caption.align"
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
var captionKeyOrder = []string{
	"text", "at", "for", "until", "font", "place", "align", "offset", "position", "color", "box", "fade",
}

// The spellings of a caption's place and align, as their errors offer them.
var (
	placeSpellings = []string{string(storyboard.PlaceTop), string(storyboard.PlaceMiddle), string(storyboard.PlaceBottom)}
	alignSpellings = []string{string(storyboard.AlignLeft), string(storyboard.AlignMiddle), string(storyboard.AlignRight)}
)

// isCaptionField reports whether field edits a caption rather than a beat, the frame or a
// camera move.
func isCaptionField(field string) bool {
	return strings.HasPrefix(field, captionFieldPrefix)
}

// editCaption sets field, spelled as value, on the caption at index in board, the storyboard
// s.doc holds.
func (s *splicer) editCaption(board *storyboard.Storyboard, index int, field, value string) error {
	if index < 0 || index >= len(board.Captions) {
		return fmt.Errorf("the storyboard has no caption at index %d", index)
	}
	caption := yamledit.Path{keyCaptions, strconv.Itoa(index)}

	switch field {
	case fieldCaptionAt:
		return s.setMoment(caption, captionKeyOrder, value)
	case fieldCaptionFor:
		return s.setCaptionFor(caption, value)
	case fieldCaptionUntil:
		return s.setCaptionUntil(caption, value)
	case fieldCaptionText:
		return s.setCaptionText(caption, value)
	case fieldCaptionPlace:
		return s.setPlacement(caption, keyPlace, value, placeSpellings, string(storyboard.PlaceBottom))
	default:
		return s.setPlacement(caption, keyAlign, value, alignSpellings, string(storyboard.AlignMiddle))
	}
}

// setCaptionFor writes the caption's length and deletes its until, since a caption ends by
// exactly one of the two. A length of 0 is refused: it would leave the caption no end.
func (s *splicer) setCaptionFor(caption yamledit.Path, value string) error {
	length, err := parseMilliseconds(value)
	if err != nil {
		return err
	}
	if length == 0 {
		return errors.New("want a length longer than 0ms")
	}
	if err := s.writeDuration(caption, keyFor, captionKeyOrder, 0, length); err != nil {
		return err
	}
	return s.doc.DeleteIfPresent(under(caption, keyUntil))
}

// setCaptionUntil replaces the caption's whole until, whatever its form, with the moment value
// spells, and deletes its for, since a caption ends by exactly one of the two.
func (s *splicer) setCaptionUntil(caption yamledit.Path, value string) error {
	text, _, err := parseMoment(value)
	if err != nil {
		return err
	}
	if err := s.replaceOrInsert(caption, keyUntil, text, captionKeyOrder); err != nil {
		return err
	}
	return s.doc.DeleteIfPresent(under(caption, keyFor))
}

// setCaptionText writes value as the caption's text: one double-quoted line, its line breaks
// escaped as \n, in place of whatever form the old text took, a block scalar too.
func (s *splicer) setCaptionText(caption yamledit.Path, value string) error {
	return s.replaceOrInsert(caption, keyText, strconv.Quote(value), captionKeyOrder)
}

// setPlacement writes the caption's place or align key, one of spellings, or deletes the key
// for absent, the value it takes when left out.
func (s *splicer) setPlacement(caption yamledit.Path, key, value string, spellings []string, absent string) error {
	if !slices.Contains(spellings, value) {
		last := len(spellings) - 1
		return fmt.Errorf("want %s or %s, got %q", strings.Join(spellings[:last], ", "), spellings[last], value)
	}
	if value == absent {
		return s.doc.DeleteIfPresent(under(caption, key))
	}
	return s.doc.Upsert(caption, key, value, captionKeyOrder)
}
