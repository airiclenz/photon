package storyboard

import (
	"cmp"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Caption is a line or more of text drawn over the composed frame, placed on the clip's output
// clock. It sits in output space: a zoom or pan does not move or scale it.
//
// Text holds its lines, separated by \n; [Load] trims one trailing newline, so a YAML block
// scalar reads as written. The caption starts At and ends after For or at Until, exactly one of
// the two. It is placed by Place and Align, nudged by Offset, or set by Position — the box's
// top-left corner — instead of all three. [Load] fills what a valid caption leaves out: Font's
// size is frame.font_size and its weight regular, Place is bottom and Align middle (unless
// Position is set, when both stay empty), Color is white and Box is black at 0.6 opacity.
//
// The authored fields stay as written, defaults filled; [Load] also resolves the caption onto the
// clip's output clock and its colours, which [Caption.Start], [Caption.Stop],
// [Caption.TextColor] and [Caption.BoxColor] report. A Caption built as a struct literal rather
// than loaded reports zero for all four.
type Caption struct {
	Text     string        `yaml:"text"`
	At       Moment        `yaml:"at"`
	For      time.Duration `yaml:"for"`
	Until    *Moment       `yaml:"until"`
	Font     CaptionFont   `yaml:"font"`
	Place    Place         `yaml:"place"`
	Align    Align         `yaml:"align"`
	Offset   Pixels        `yaml:"offset"`
	Position *Pixels       `yaml:"position"`
	Color    string        `yaml:"color"`
	Box      CaptionBox    `yaml:"box"`
	Fade     Fade          `yaml:"fade"`

	start, stop         time.Duration
	textColor, boxColor RGB
}

// Start is where the caption starts on the clip's output clock: its At, resolved.
func (c Caption) Start() time.Duration { return c.start }

// Stop is where the caption ends on the clip's output clock: its start plus For, or its Until
// resolved.
func (c Caption) Stop() time.Duration { return c.stop }

// TextColor is the colour of the caption's text: its Color, parsed.
func (c Caption) TextColor() RGB { return c.textColor }

// BoxColor is the colour of the box behind the caption's text: its Box.Color, parsed.
func (c Caption) BoxColor() RGB { return c.boxColor }

// RGB is an opaque colour, one byte per channel.
type RGB struct {
	R, G, B uint8
}

// CaptionFont is a caption's face: its Size, in the unit of frame.font_size, and its Weight.
type CaptionFont struct {
	Size   float64 `yaml:"size"`
	Weight Weight  `yaml:"weight"`
}

// Weight is which caption face a caption is drawn in.
type Weight string

// The spellings of Weight.
const (
	WeightRegular Weight = "regular"
	WeightBold    Weight = "bold"
)

// Place is where a caption sits vertically in the frame.
type Place string

// The spellings of Place.
const (
	PlaceTop    Place = "top"
	PlaceMiddle Place = "middle"
	PlaceBottom Place = "bottom"
)

// Align is where a caption sits horizontally in the frame.
type Align string

// The spellings of Align.
const (
	AlignLeft   Align = "left"
	AlignMiddle Align = "middle"
	AlignRight  Align = "right"
)

// Pixels is a pair of lengths in design pixels — pixels of a frame rasterized at the
// compositor's design scale: X to the right and Y down.
type Pixels struct {
	X int `yaml:"x"`
	Y int `yaml:"y"`
}

// CaptionBox is the background drawn behind a caption's text: its Color and its Opacity, from
// 0 (no box) to 1. [Load] sets an absent Opacity, so a valid storyboard's is never nil.
type CaptionBox struct {
	Color   string   `yaml:"color"`
	Opacity *float64 `yaml:"opacity"`
}

// Fade is how long a caption takes to fade in after its start and out before its end, linear in
// opacity. Zero cuts it in or out.
type Fade struct {
	In  time.Duration `yaml:"in"`
	Out time.Duration `yaml:"out"`
}

// Caption size bounds, in the unit of frame.font_size, which bound frame.font_size too: below 6
// the text is unreadable in a GIF, beyond 72 one line fills the frame.
const (
	minCaptionSize = 6
	maxCaptionSize = 72
)

// The colours a caption leaves out.
const (
	defaultCaptionColor = "#ffffff"
	defaultBoxColor     = "#000000"
	defaultBoxOpacity   = 0.6
)

// Box opacity bounds.
const (
	minBoxOpacity = 0
	maxBoxOpacity = 1
)

// hexColor is the spelling of a caption colour: #rrggbb.
var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// validateCaptions checks every caption's text, font, placement, colours, timing and fades, and
// keeps what it resolves. Problems are filed under captions[i].
func (s *Storyboard) validateCaptions(collect collector) {
	clock := s.Clock()
	for index := range s.Captions {
		s.Captions[index].validate(s, clock, collect.at(Owner{Kind: OwnerCaption, Index: index}))
	}
}

// validate applies the caption's rules, resolving its times on clock, board's output clock, and
// its colours, a colour left out resolving to its default. What fails to resolve stays zero.
func (c *Caption) validate(board *Storyboard, clock OutputClock, report reporter) {
	if strings.TrimSuffix(c.Text, "\n") == "" {
		report("text: missing")
	}
	c.Font.validate(board.Frame.FontSize, report)
	c.validatePlacement(report)
	c.textColor = validateColor("color", cmp.Or(c.Color, defaultCaptionColor), report)
	c.boxColor = c.Box.validate(report)
	start, stop, isTimed := c.validateTiming(clock, report)
	if isTimed {
		c.start, c.stop = start, stop
	}
	c.Fade.validate(stop-start, isTimed, report)
}

// validate checks the font's weight and that its size — its own, or frame.font_size when it has
// none — lies in the caption bounds. A missing frame.font_size is the header's problem, not the
// caption's.
func (f CaptionFont) validate(frameFontSize float64, report reporter) {
	if f.Weight != "" && f.Weight != WeightRegular && f.Weight != WeightBold {
		report("font.weight: want %s or %s, got %q", WeightRegular, WeightBold, f.Weight)
	}
	size, source := f.Size, ""
	if size == 0 {
		if frameFontSize <= 0 {
			return
		}
		size, source = frameFontSize, " from frame.font_size"
	}
	if size < minCaptionSize || size > maxCaptionSize {
		report("font.size: want in [%d, %d], got %g%s",
			minCaptionSize, maxCaptionSize, size, source)
	}
}

// validatePlacement checks the place and align spellings and that a position comes instead of
// them and the offset, not with them.
func (c Caption) validatePlacement(report reporter) {
	if c.Position != nil && (c.Place != "" || c.Align != "" || c.Offset != (Pixels{})) {
		report("position: want it instead of place, align and offset, not with them")
	}
	switch c.Place {
	case "", PlaceTop, PlaceMiddle, PlaceBottom:
	default:
		report("place: want %s, %s or %s, got %q", PlaceTop, PlaceMiddle, PlaceBottom, c.Place)
	}
	switch c.Align {
	case "", AlignLeft, AlignMiddle, AlignRight:
	default:
		report("align: want %s, %s or %s, got %q", AlignLeft, AlignMiddle, AlignRight, c.Align)
	}
}

// validate checks the box's colour and that its opacity lies in [0, 1], and returns the colour
// resolved, the default when the box leaves it out.
func (b CaptionBox) validate(report reporter) RGB {
	resolved := validateColor("box.color", cmp.Or(b.Color, defaultBoxColor), report)
	if b.Opacity != nil && (*b.Opacity < minBoxOpacity || *b.Opacity > maxBoxOpacity) {
		report("box.opacity: want in [%d, %d], got %g", minBoxOpacity, maxBoxOpacity, *b.Opacity)
	}
	return resolved
}

// validateTiming checks that the caption sets exactly one end, that it starts inside the clip
// and that it ends after its start and by the clip's end, and returns where it starts and stops
// on clock. isTimed is false when any of that fails, so there is no stretch to fit the fades in.
func (c Caption) validateTiming(
	clock OutputClock, report reporter,
) (start, stop time.Duration, isTimed bool) {
	hasEnd := true
	if (c.For == 0) == (c.Until == nil) {
		report("want exactly one of for or until")
		hasEnd = false
	}
	if c.For < 0 {
		report("for: want >0, got %s", c.For)
		hasEnd = false
	}
	start, err := clock.Resolve(c.At)
	if err != nil {
		report("at: %v", err)
		return 0, 0, false
	}
	clipLength := clock.Length()
	if start < 0 || start >= clipLength {
		report("at: want a start in [0s, %s), got %s", clipLength, start)
		return 0, 0, false
	}
	if !hasEnd {
		return 0, 0, false
	}
	stop, field := start+c.For, "for"
	if c.Until != nil {
		field = "until"
		if stop, err = clock.Resolve(*c.Until); err != nil {
			report("until: %v", err)
			return 0, 0, false
		}
		if stop <= start {
			report("until: want a stop after the start %s, got %s", start, stop)
			return 0, 0, false
		}
	}
	if stop > clipLength {
		report("%s: want the caption to end by the clip's end %s, got an end at %s",
			field, clipLength, stop)
		return 0, 0, false
	}
	return start, stop, true
}

// validate checks that neither fade is negative and, when the caption's length is known, that
// the two fit in it.
func (f Fade) validate(length time.Duration, isTimed bool, report reporter) {
	if f.In < 0 {
		report("fade.in: want ≥0, got %s", f.In)
	}
	if f.Out < 0 {
		report("fade.out: want ≥0, got %s", f.Out)
	}
	if isTimed && f.In >= 0 && f.Out >= 0 && f.In+f.Out > length {
		report("fade: want in + out within the caption's length %s, got %s", length, f.In+f.Out)
	}
}

// validateColor reports a colour not spelled #rrggbb, under the field's name, and returns it
// parsed; a colour it reports is zero.
func validateColor(field, color string, report reporter) RGB {
	value, err := strconv.ParseUint(strings.TrimPrefix(color, "#"), 16, 32)
	if !hexColor.MatchString(color) || err != nil {
		report("%s: want #rrggbb, got %q", field, color)
		return RGB{}
	}
	return RGB{R: uint8(value >> 16), G: uint8(value >> 8), B: uint8(value)}
}

// applyDefaults fills what a valid caption leaves out; frameFontSize is the frame's font size,
// the default caption size.
func (c *Caption) applyDefaults(frameFontSize float64) {
	c.Text = strings.TrimSuffix(c.Text, "\n")
	if c.Font.Size == 0 {
		c.Font.Size = frameFontSize
	}
	if c.Font.Weight == "" {
		c.Font.Weight = WeightRegular
	}
	if c.Position == nil {
		if c.Place == "" {
			c.Place = PlaceBottom
		}
		if c.Align == "" {
			c.Align = AlignMiddle
		}
	}
	if c.Color == "" {
		c.Color = defaultCaptionColor
	}
	if c.Box.Color == "" {
		c.Box.Color = defaultBoxColor
	}
	if c.Box.Opacity == nil {
		c.Box.Opacity = new(float64(defaultBoxOpacity))
	}
}
