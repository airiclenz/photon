package caption

import (
	"fmt"
	"image"
	"math"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/airiclenz/photon/internal/fonts"
	"github.com/airiclenz/photon/internal/storyboard"
)

// designScale is the raster scale design pixels are measured at, the compositor's: a frame
// rasterized at it draws the terminal's text at an em of frame.font_size × designScale pixels.
const designScale = 2

// The caption box's spacing, in design pixels: the gap a placed box keeps from the frame's edges
// and the padding between the box's edges and its text.
const (
	edgeMargin  = 24
	boxPaddingX = 16
	boxPaddingY = 8
)

// pixelsPerPoint makes a face's Size a size in pixels: at 72 DPI one point is one pixel.
const pixelsPerPoint = 72

// fixedOne is one pixel in 26.6 fixed point.
const fixedOne = 64

// textBlock is a caption's lines measured in one face: each line's advance, the block's pixel
// size, the distance from one baseline to the next and the first baseline's depth below the
// block's top.
type textBlock struct {
	face        font.Face
	lines       []string
	widths      []fixed.Int26_6
	size        image.Point
	lineAdvance float64
	baseline    float64
}

// newTextBlock measures text's lines in face at an em of em pixels. Each line takes the face's
// line height, its ascent and descent centred in it.
func newTextBlock(face fonts.Face, em float64, text string) (textBlock, error) {
	drawn, err := opentype.NewFace(face.Font, &opentype.FaceOptions{
		Size:    em,
		DPI:     pixelsPerPoint,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return textBlock{}, fmt.Errorf("face %s at %g pixels: %w", face.Name, em, err)
	}

	block := textBlock{face: drawn, lines: strings.Split(text, "\n")}
	var widest fixed.Int26_6
	for _, line := range block.lines {
		width := font.MeasureString(drawn, line)
		block.widths = append(block.widths, width)
		widest = max(widest, width)
	}
	metrics := drawn.Metrics()
	ascent, descent := toFloat(metrics.Ascent), toFloat(metrics.Descent)
	block.lineAdvance = toFloat(metrics.Height)
	block.baseline = (block.lineAdvance-(ascent+descent))/2 + ascent
	block.size = image.Pt(
		int(math.Ceil(toFloat(widest))),
		int(math.Ceil(block.lineAdvance*float64(len(block.lines)))),
	)
	return block, nil
}

// padding is the box's padding around its text in shipped pixels.
func padding(unit float64) image.Point {
	return image.Pt(int(math.Round(boxPaddingX*unit)), int(math.Round(boxPaddingY*unit)))
}

// boxRect sizes the caption's box around block and places it in a frame of size out: at
// position when the caption sets one, else by place and align a margin in from the edges and
// moved by offset. The box is then clamped inside the frame; a box larger than the frame is an
// error.
func boxRect(caption storyboard.Caption, block textBlock, out image.Point, unit float64) (image.Rectangle, error) {
	size := block.size.Add(padding(unit).Mul(2))
	if size.X > out.X || size.Y > out.Y {
		return image.Rectangle{}, fmt.Errorf("the caption's box is %dx%d pixels, larger than the %dx%d frame",
			size.X, size.Y, out.X, out.Y)
	}

	var x, y float64
	if caption.Position != nil {
		x, y = float64(caption.Position.X)*unit, float64(caption.Position.Y)*unit
	} else {
		x = along(caption.Align == storyboard.AlignLeft, caption.Align == storyboard.AlignRight, size.X, out.X, unit)
		y = along(caption.Place == storyboard.PlaceTop, caption.Place == storyboard.PlaceBottom, size.Y, out.Y, unit)
		x += float64(caption.Offset.X) * unit
		y += float64(caption.Offset.Y) * unit
	}
	topLeft := image.Pt(
		min(max(int(math.Round(x)), 0), out.X-size.X),
		min(max(int(math.Round(y)), 0), out.Y-size.Y),
	)
	return image.Rectangle{Min: topLeft, Max: topLeft.Add(size)}, nil
}

// along places a box length long on a frame axis span long: a margin in from the start edge or
// the end edge, or centred when neither.
func along(isStart, isEnd bool, length, span int, unit float64) float64 {
	margin := edgeMargin * unit
	switch {
	case isStart:
		return margin
	case isEnd:
		return float64(span-length) - margin
	default:
		return float64(span-length) / 2
	}
}

// mask rasterizes the block's lines into a coverage mask whose bounds are box, the text inset by
// the padding and each line aligned left, centred or right within the block by align. A caption
// placed by position has no align, and its lines align left.
func (b textBlock) mask(box image.Rectangle, align storyboard.Align, unit float64) *image.Alpha {
	mask := image.NewAlpha(box)
	origin := box.Min.Add(padding(unit))
	drawer := font.Drawer{Dst: mask, Src: image.Opaque, Face: b.face}
	for index, line := range b.lines {
		slack := fixed.I(b.size.X) - b.widths[index]
		var indent fixed.Int26_6
		switch align {
		case storyboard.AlignMiddle:
			indent = slack / 2
		case storyboard.AlignRight:
			indent = slack
		}
		baseline := float64(origin.Y) + b.baseline + float64(index)*b.lineAdvance
		drawer.Dot = fixed.Point26_6{X: fixed.I(origin.X) + indent, Y: fromFloat(baseline)}
		drawer.DrawString(line)
	}
	return mask
}

// toFloat is a 26.6 fixed-point length in pixels.
func toFloat(length fixed.Int26_6) float64 {
	return float64(length) / fixedOne
}

// fromFloat is a pixel length in 26.6 fixed point.
func fromFloat(length float64) fixed.Int26_6 {
	return fixed.Int26_6(math.Round(length * fixedOne))
}
