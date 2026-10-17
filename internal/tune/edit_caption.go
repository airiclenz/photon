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

// The caption fields an Edit names; each edits the caption at the Edit's Index.
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

// editCaption applies a caption field's edit to the caption at edit.Index in board, the
// storyboard s.doc holds.
func (s *splicer) editCaption(board *storyboard.Storyboard, edit Edit) error {
	if edit.Index < 0 || edit.Index >= len(board.Captions) {
		return fmt.Errorf("the storyboard has no caption at index %d", edit.Index)
	}
	caption := yamledit.Path{keyCaptions, strconv.Itoa(edit.Index)}

	switch edit.Field {
	case fieldCaptionAt:
		return s.setMoment(caption, captionKeyOrder, edit.Value)
	case fieldCaptionFor:
		return s.setCaptionFor(caption, edit.Value)
	case fieldCaptionUntil:
		return s.setCaptionUntil(caption, edit.Value)
	case fieldCaptionText:
		return s.setCaptionText(caption, edit.Value)
	case fieldCaptionPlace:
		return s.setPlacement(caption, keyPlace, edit.Value, placeSpellings, string(storyboard.PlaceBottom))
	default:
		return s.setPlacement(caption, keyAlign, edit.Value, alignSpellings, string(storyboard.AlignMiddle))
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
	return s.deleteKey(under(caption, keyUntil))
}

// setCaptionUntil replaces the caption's whole until, whatever its form, with the moment value
// spells, and deletes its for, since a caption ends by exactly one of the two.
func (s *splicer) setCaptionUntil(caption yamledit.Path, value string) error {
	text, _, err := parseMoment(value)
	if err != nil {
		return err
	}
	until := under(caption, keyUntil)
	if s.lookup(until) == nil {
		err = s.doc.Insert(caption, keyUntil, text, keysBefore(captionKeyOrder, keyUntil))
	} else {
		err = s.doc.Replace(until, text)
	}
	if err != nil {
		return err
	}
	return s.deleteKey(under(caption, keyFor))
}

// setCaptionText writes value as the caption's text: one double-quoted line, its line breaks
// escaped as \n, in place of whatever form the old text took, a block scalar too.
func (s *splicer) setCaptionText(caption yamledit.Path, value string) error {
	quoted := strconv.Quote(value)
	text := under(caption, keyText)
	if s.lookup(text) == nil {
		return s.doc.Insert(caption, keyText, quoted, nil)
	}
	return s.doc.Replace(text, quoted)
}

// setPlacement writes the caption's place or align key, one of spellings, or deletes the key
// for absent, the value it takes when left out.
func (s *splicer) setPlacement(caption yamledit.Path, key, value string, spellings []string, absent string) error {
	if !slices.Contains(spellings, value) {
		last := len(spellings) - 1
		return fmt.Errorf("want %s or %s, got %q", strings.Join(spellings[:last], ", "), spellings[last], value)
	}
	if value == absent {
		return s.deleteKey(under(caption, key))
	}
	return s.writeScalar(caption, key, captionKeyOrder, value)
}
