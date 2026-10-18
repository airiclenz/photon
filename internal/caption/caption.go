package caption

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
	"time"

	"github.com/airiclenz/photon/internal/fonts"
	"github.com/airiclenz/photon/internal/storyboard"
)

// maxAlpha is a fully opaque 8-bit alpha.
const maxAlpha = 255

// Layer is a storyboard's captions, laid out for one shipped frame size and rasterized, ready to
// be drawn onto any frame of the clip. It is immutable after [New], so one Layer is safe for
// concurrent use.
type Layer struct {
	captions []placed
}

// placed is one caption ready to draw: its span on the output clock and its fades, the box it
// fills in shipped pixels, the box's colour and opacity, and its text as a coverage mask whose
// bounds are the box itself, in the text's colour.
type placed struct {
	start, stop time.Duration
	fade        storyboard.Fade

	box        image.Rectangle
	boxColor   color.RGBA
	boxOpacity float64

	text      *image.Alpha
	textColor color.RGBA
}

// New lays out board's captions on a shipped frame out pixels in size, where one design pixel
// spans unit shipped pixels (the compositor's unit). It resolves each caption's start and end on
// the output clock, draws its text from faces at an em of its font size × the design scale × unit
// pixels — the shipped em of the terminal's text at that font size — sizes its box around the
// text, places the box by place, align and offset or by position, clamps it inside the frame and
// rasterizes the text, so [Layer.Draw] only composites.
//
// board must be one [storyboard.Parse] returned, so the captions carry their defaults. New fails
// when out or unit is not positive, a face is missing, a moment does not resolve, a colour is not
// #rrggbb, or a caption's box is wider or taller than the frame; the error names the caption and
// is filed under it as a [storyboard.LocatedError].
func New(board *storyboard.Storyboard, faces fonts.CaptionSet, out image.Point, unit float64) (*Layer, error) {
	if out.X <= 0 || out.Y <= 0 {
		return nil, fmt.Errorf("caption: want a frame of positive size, got %dx%d", out.X, out.Y)
	}
	if unit <= 0 {
		return nil, fmt.Errorf("caption: want a positive unit, got %g", unit)
	}
	if faces.Regular.Font == nil || faces.Bold.Font == nil {
		return nil, errors.New("caption: the caption face set is missing a face")
	}

	layer := &Layer{captions: make([]placed, 0, len(board.Captions))}
	for index, caption := range board.Captions {
		laid, err := place(board, caption, faces, out, unit)
		if err != nil {
			return nil, fmt.Errorf("caption: %w", storyboard.AtCaption(index, err))
		}
		layer.captions = append(layer.captions, laid)
	}
	return layer, nil
}

// place resolves, lays out and rasterizes one caption.
func place(
	board *storyboard.Storyboard,
	caption storyboard.Caption,
	faces fonts.CaptionSet,
	out image.Point,
	unit float64,
) (placed, error) {
	start, stop, err := span(board, caption)
	if err != nil {
		return placed{}, err
	}
	textColor, err := parseColor(caption.Color)
	if err != nil {
		return placed{}, fmt.Errorf("color: %w", err)
	}
	boxColor, err := parseColor(caption.Box.Color)
	if err != nil {
		return placed{}, fmt.Errorf("box.color: %w", err)
	}
	face := faces.Regular
	if caption.Font.Weight == storyboard.WeightBold {
		face = faces.Bold
	}
	block, err := newTextBlock(face, caption.Font.Size*designScale*unit, caption.Text)
	if err != nil {
		return placed{}, err
	}
	box, err := boxRect(caption, block, out, unit)
	if err != nil {
		return placed{}, err
	}

	var boxOpacity float64
	if caption.Box.Opacity != nil {
		boxOpacity = *caption.Box.Opacity
	}
	return placed{
		start:      start,
		stop:       stop,
		fade:       caption.Fade,
		box:        box,
		boxColor:   boxColor,
		boxOpacity: boxOpacity,
		text:       block.mask(box, caption.Align, unit),
		textColor:  textColor,
	}, nil
}

// span resolves the caption's start and its end on board's output clock: its at, and its at
// plus for or its until.
func span(board *storyboard.Storyboard, caption storyboard.Caption) (start, stop time.Duration, err error) {
	start, err = board.ClipTime(caption.At)
	if err != nil {
		return 0, 0, fmt.Errorf("at: %w", err)
	}
	if caption.Until == nil {
		return start, start + caption.For, nil
	}
	stop, err = board.ClipTime(*caption.Until)
	if err != nil {
		return 0, 0, fmt.Errorf("until: %w", err)
	}
	return start, stop, nil
}

// parseColor reads a #rrggbb colour as an opaque RGBA.
func parseColor(spelled string) (color.RGBA, error) {
	const hexDigits = 6
	if len(spelled) != hexDigits+1 || spelled[0] != '#' {
		return color.RGBA{}, fmt.Errorf("want #rrggbb, got %q", spelled)
	}
	value, err := strconv.ParseUint(spelled[1:], 16, 32)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("want #rrggbb, got %q", spelled)
	}
	return color.RGBA{R: uint8(value >> 16), G: uint8(value >> 8), B: uint8(value), A: maxAlpha}, nil
}

// Draw paints every caption showing at output time t onto dst: first its box, at the box's
// opacity × the caption's fade, then its text at the fade. dst must be a frame of the size the
// Layer was built for; Draw writes only dst, so calls on distinct frames may run concurrently.
func (l *Layer) Draw(dst *image.RGBA, t time.Duration) {
	for _, caption := range l.captions {
		fade := caption.opacity(t)
		if fade <= 0 {
			continue
		}
		box := &image.Uniform{C: withAlpha(caption.boxColor, caption.boxOpacity*fade)}
		draw.Draw(dst, caption.box, box, image.Point{}, draw.Over)
		text := &image.Uniform{C: withAlpha(caption.textColor, fade)}
		draw.DrawMask(dst, caption.box, text, image.Point{}, caption.text, caption.box.Min, draw.Over)
	}
}

// opacity is the caption's fade at output time t: 0 outside [start, stop), rising linearly over
// the fade-in from the start, falling linearly over the fade-out to the stop, and 1 between.
func (p placed) opacity(t time.Duration) float64 {
	if t < p.start || t >= p.stop {
		return 0
	}
	opacity := 1.0
	if elapsed := t - p.start; elapsed < p.fade.In {
		opacity = float64(elapsed) / float64(p.fade.In)
	}
	if remaining := p.stop - t; remaining < p.fade.Out {
		opacity = math.Min(opacity, float64(remaining)/float64(p.fade.Out))
	}
	return opacity
}

// withAlpha is c at opacity, as a non-premultiplied colour.
func withAlpha(c color.RGBA, opacity float64) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: uint8(math.Round(maxAlpha * opacity))}
}
