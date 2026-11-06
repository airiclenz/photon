package caption

import (
	"fmt"
	"image"
	"image/color"
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

// textBlock is a caption's lines measured in its faces: each line's runs and advance, the
// block's pixel size, the distance from one baseline to the next and the first baseline's depth
// below the block's top.
type textBlock struct {
	faces       runFaces
	lines       []styledLine
	widths      []fixed.Int26_6
	size        image.Point
	lineAdvance float64
	baseline    float64
}

// newTextBlock measures the lines of runs' text at an em of em pixels, each run in the face of its
// weight, wrapping each hard line no wider than maxWidth pixels as [wrapLine] does. Each line takes
// the line height of the tallest face the runs are drawn in, its ascent and descent centred in it.
func newTextBlock(
	faces fonts.CaptionSet,
	em float64,
	runs []storyboard.Run,
	maxWidth fixed.Int26_6,
) (textBlock, error) {
	drawn, err := newRunFaces(faces, em)
	if err != nil {
		return textBlock{}, err
	}

	block := textBlock{faces: drawn}
	for _, hardLine := range hardLines(runs) {
		block.lines = append(block.lines, wrapLine(drawn, hardLine, maxWidth)...)
	}
	var widest fixed.Int26_6
	for _, line := range block.lines {
		width := drawn.measure(line)
		block.widths = append(block.widths, width)
		widest = max(widest, width)
	}
	ascent, descent, height := drawn.metrics(runs)
	block.lineAdvance = height
	block.baseline = (block.lineAdvance-(ascent+descent))/2 + ascent
	block.size = image.Pt(
		int(math.Ceil(toFloat(widest))),
		int(math.Ceil(block.lineAdvance*float64(len(block.lines)))),
	)
	return block, nil
}

// runFaces are a caption's two faces at one em: a run is drawn in the one of its weight.
type runFaces struct {
	regular, bold font.Face
}

// newRunFaces opens the regular and bold caption faces at an em of em pixels.
func newRunFaces(faces fonts.CaptionSet, em float64) (runFaces, error) {
	regular, err := openFace(faces.Regular, em)
	if err != nil {
		return runFaces{}, err
	}
	bold, err := openFace(faces.Bold, em)
	if err != nil {
		return runFaces{}, err
	}
	return runFaces{regular: regular, bold: bold}, nil
}

// openFace is face at an em of em pixels, unhinted.
func openFace(face fonts.Face, em float64) (font.Face, error) {
	drawn, err := opentype.NewFace(face.Font, &opentype.FaceOptions{
		Size:    em,
		DPI:     pixelsPerPoint,
		Hinting: font.HintingNone,
	})
	if err != nil {
		return nil, fmt.Errorf("face %s at %g pixels: %w", face.Name, em, err)
	}
	return drawn, nil
}

// of is the face a run of weight is drawn in.
func (f runFaces) of(weight storyboard.Weight) font.Face {
	if weight == storyboard.WeightBold {
		return f.bold
	}
	return f.regular
}

// measure is line's advance, each run measured in its own face; a single run measures exactly
// as its text does.
func (f runFaces) measure(line styledLine) fixed.Int26_6 {
	var width fixed.Int26_6
	for _, run := range line.runs {
		width += font.MeasureString(f.of(run.Weight), run.Text)
	}
	return width
}

// metrics are the largest ascent, descent and line height, in pixels, among the faces runs are
// drawn in, so a line of mixed weights fits its tallest face.
func (f runFaces) metrics(runs []storyboard.Run) (ascent, descent, height float64) {
	for _, run := range runs {
		metrics := f.of(run.Weight).Metrics()
		ascent = max(ascent, toFloat(metrics.Ascent))
		descent = max(descent, toFloat(metrics.Descent))
		height = max(height, toFloat(metrics.Height))
	}
	return ascent, descent, height
}

// styledLine is one line of a caption's text with the runs it is drawn in: the runs' texts, in
// order, make up text, and none of them is empty.
type styledLine struct {
	text string
	runs []storyboard.Run
}

// hardLines splits runs into the caption's hard lines at each \n; a run that crosses a newline
// goes on both lines, in its own style on each.
func hardLines(runs []storyboard.Run) []styledLine {
	lines := []styledLine{{}}
	for _, run := range runs {
		for index, part := range strings.Split(run.Text, "\n") {
			if index > 0 {
				lines = append(lines, styledLine{})
			}
			lines[len(lines)-1].add(run, part)
		}
	}
	return lines
}

// add appends text to the line in run's style; empty text adds nothing.
func (l *styledLine) add(run storyboard.Run, text string) {
	if text == "" {
		return
	}
	run.Text = text
	l.text += text
	l.runs = append(l.runs, run)
}

// slice is the stretch of the line from byte offset start to end, each run cut to it.
func (l styledLine) slice(start, end int) styledLine {
	var sliced styledLine
	runStart := 0
	for _, run := range l.runs {
		runEnd := runStart + len(run.Text)
		if from, to := max(start, runStart), min(end, runEnd); from < to {
			sliced.add(run, run.Text[from-runStart:to-runStart])
		}
		runStart = runEnd
	}
	return sliced
}

// wrapLine splits one hard line of caption text into lines no wider than maxWidth pixels in
// faces. A line that fits comes back unchanged. One that does not is broken greedily at spaces,
// whichever run they fall in, each break dropping the whole run of spaces it falls on, so no
// line it yields ends in a space and none but the first starts with one; a word wider than
// maxWidth stays whole, alone on its line, and a run of leading spaces stays with the word that
// follows it. Each line it yields keeps the styles of the runs it cuts.
func wrapLine(faces runFaces, line styledLine, maxWidth fixed.Int26_6) []styledLine {
	if faces.measure(line) <= maxWidth {
		return []styledLine{line}
	}

	// The line so far is text[start:end]; it always ends where a word ends.
	text := line.text
	words := strings.Split(text, " ")
	var wrapped []styledLine
	start, end := 0, len(words[0])
	for _, word := range words[1:] {
		wordStart := end + 1
		wordEnd := wordStart + len(word)
		switch {
		case start == end && len(wrapped) > 0:
			// The last break fell inside a run of spaces; the rest of the run goes with it.
			start = wordStart
		case strings.TrimSpace(text[start:end]) == "" ||
			faces.measure(line.slice(start, wordEnd)) <= maxWidth:
			// The word fits on the line so far, the space before it too.
		default:
			wrapped = append(wrapped, line.slice(start, trimmedEnd(text, start, end)))
			start = wordStart
		}
		end = wordEnd
	}
	end = trimmedEnd(text, start, end)
	if start == end && len(wrapped) > 0 {
		// The line ended in the run of spaces the last break fell on.
		return wrapped
	}
	return append(wrapped, line.slice(start, end))
}

// trimmedEnd is where text[start:end] ends with its trailing spaces dropped.
func trimmedEnd(text string, start, end int) int {
	return start + len(strings.TrimRight(text[start:end], " "))
}

// maxTextWidth is how wide a caption's text may be, in shipped pixels, on a frame frameWidth
// pixels wide where one design pixel spans unit pixels: the frame's width less an edge margin on
// each side, narrowed to width design pixels when width is set (above 0), less the box's padding
// on each side.
func maxTextWidth(width, frameWidth int, unit float64) fixed.Int26_6 {
	boxWidth := float64(frameWidth)/unit - 2*edgeMargin
	if width > 0 {
		boxWidth = min(boxWidth, float64(width))
	}
	return fromFloat((boxWidth - 2*boxPaddingX) * unit)
}

// padding is the box's padding around its text in shipped pixels.
func padding(unit float64) image.Point {
	return image.Pt(int(math.Round(boxPaddingX*unit)), int(math.Round(boxPaddingY*unit)))
}

// boxRect sizes the caption's box around block and places it in a frame of size out: at
// position when the caption sets one, else by place and align a margin in from the edges and
// moved by offset. The box is then clamped inside the frame; a box larger than the frame is an
// error.
func boxRect(
	caption storyboard.Caption,
	block textBlock,
	out image.Point,
	unit float64,
) (image.Rectangle, error) {
	size := block.size.Add(padding(unit).Mul(2))
	if size.X > out.X || size.Y > out.Y {
		return image.Rectangle{}, fmt.Errorf(
			"the caption's box is %dx%d pixels, larger than the %dx%d frame",
			size.X, size.Y, out.X, out.Y)
	}

	var x, y float64
	if caption.Position != nil {
		x, y = float64(caption.Position.X)*unit, float64(caption.Position.Y)*unit
	} else {
		x = along(caption.Align == storyboard.AlignLeft, caption.Align == storyboard.AlignRight,
			size.X, out.X, unit)
		y = along(caption.Place == storyboard.PlaceTop, caption.Place == storyboard.PlaceBottom,
			size.Y, out.Y, unit)
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

// textMask is a caption's text in one colour: a coverage mask whose bounds are the caption's
// box.
type textMask struct {
	mask  *image.Alpha
	color color.RGBA
}

// masks rasterizes the block's lines into one coverage mask per distinct run colour, in the
// order the colours first appear, each mask's bounds box: the text inset by the padding, each
// line aligned left, centred or right within the block by align, and each run drawn in the face
// of its weight where the run before it ends. A caption placed by position has no align, and its
// lines align left.
func (b textBlock) masks(box image.Rectangle, align storyboard.Align, unit float64) []textMask {
	var masks []textMask
	origin := box.Min.Add(padding(unit))
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
		dot := fixed.Point26_6{X: fixed.I(origin.X) + indent, Y: fromFloat(baseline)}
		for _, run := range line.runs {
			var mask *image.Alpha
			masks, mask = maskOf(masks, opaque(run.Color), box)
			drawer := font.Drawer{
				Dst:  mask,
				Src:  image.Opaque,
				Face: b.faces.of(run.Weight),
				Dot:  dot,
			}
			drawer.DrawString(run.Text)
			dot = drawer.Dot
		}
	}
	return masks
}

// maskOf is the mask of masks in colour c, appended as an empty mask with bounds box when masks
// has none in it yet.
func maskOf(masks []textMask, c color.RGBA, box image.Rectangle) ([]textMask, *image.Alpha) {
	for _, existing := range masks {
		if existing.color == c {
			return masks, existing.mask
		}
	}
	mask := image.NewAlpha(box)
	return append(masks, textMask{mask: mask, color: c}), mask
}

// toFloat is a 26.6 fixed-point length in pixels.
func toFloat(length fixed.Int26_6) float64 {
	return float64(length) / fixedOne
}

// fromFloat is a pixel length in 26.6 fixed point.
func fromFloat(length float64) fixed.Int26_6 {
	return fixed.Int26_6(math.Round(length * fixedOne))
}
