package caption

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
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
// spans unit shipped pixels (the compositor's unit). It draws each caption's text from faces at
// an em of its font size × the design scale × unit pixels — the shipped em of the terminal's text
// at that font size — sizes its box around the text, places the box by place, align and offset or
// by position, clamps it inside the frame and rasterizes the text, so [Layer.Draw] only
// composites, showing each caption over the start and stop it carries in the colours it carries.
//
// board must be one [storyboard.Parse] returned, so the captions carry their defaults and their
// resolved start, stop and colours; a caption built as a struct literal never shows. New fails
// when out or unit is not positive, a face is missing, or a caption's text cannot be laid out or
// its box is wider or taller than the frame; a caption's error names the caption and is filed
// under it as a [storyboard.LocatedError].
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
		laid, err := place(caption, faces, out, unit)
		if err != nil {
			return nil, fmt.Errorf("caption: %w", storyboard.AtCaption(index, err))
		}
		layer.captions = append(layer.captions, laid)
	}
	return layer, nil
}

// place lays out and rasterizes one caption, keeping the start, stop and colours Parse resolved.
func place(caption storyboard.Caption, faces fonts.CaptionSet, out image.Point, unit float64) (placed, error) {
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
		start:      caption.Start(),
		stop:       caption.Stop(),
		fade:       caption.Fade,
		box:        box,
		boxColor:   opaque(caption.BoxColor()),
		boxOpacity: boxOpacity,
		text:       block.mask(box, caption.Align, unit),
		textColor:  opaque(caption.TextColor()),
	}, nil
}

// opaque is the storyboard colour c as an opaque RGBA.
func opaque(c storyboard.RGB) color.RGBA {
	return color.RGBA{R: c.R, G: c.G, B: c.B, A: maxAlpha}
}

// Boxes returns the box each caption fills, in storyboard order and in shipped pixels: the
// rectangle [Layer.Draw] paints the caption's box and text in. The slice is the caller's own.
func (l *Layer) Boxes() []image.Rectangle {
	boxes := make([]image.Rectangle, 0, len(l.captions))
	for _, caption := range l.captions {
		boxes = append(boxes, caption.box)
	}
	return boxes
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
