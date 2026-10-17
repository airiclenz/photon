package compose

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"golang.org/x/image/draw"
	"golang.org/x/image/math/f64"

	"github.com/airiclenz/photon/internal/caption"
	"github.com/airiclenz/photon/internal/fonts"
	"github.com/airiclenz/photon/internal/schedule"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
)

// Layout is where the terminal's cells sit in a rasterized frame: the frame's pixel Size, the
// Padding around the grid, one cell's pixel size and the Scale the frame was rasterized at (0
// reads as the design scale, 2).
type Layout struct {
	Size       image.Point
	Padding    int
	CellWidth  int
	LineHeight int
	Scale      int
}

// pointF is a fractional pixel position.
type pointF struct{ X, Y float64 }

// rectF is a fractional pixel rectangle: its top-left corner and its size.
type rectF struct{ X, Y, W, H float64 }

// centre is the rectangle's middle.
func (r rectF) centre() pointF { return pointF{r.X + r.W/2, r.Y + r.H/2} }

// boxRect is the pixel rectangle a cell box covers.
func (l Layout) boxRect(b take.Box) rectF {
	return rectF{
		X: float64(l.Padding + b.X*l.CellWidth),
		Y: float64(l.Padding + b.Y*l.LineHeight),
		W: float64(b.W * l.CellWidth),
		H: float64(b.H * l.LineHeight),
	}
}

// cellCentre is the pixel centre of the cell at column x, row y.
func (l Layout) cellCentre(x, y int) pointF {
	return pointF{
		X: float64(l.Padding) + (float64(x)+0.5)*float64(l.CellWidth),
		Y: float64(l.Padding) + (float64(y)+0.5)*float64(l.LineHeight),
	}
}

// grid is the number of whole columns and rows of cells the frame holds inside its padding.
func (l Layout) grid() image.Point {
	return image.Pt((l.Size.X-2*l.Padding)/l.CellWidth, (l.Size.Y-2*l.Padding)/l.LineHeight)
}

// home is where the very first cursor appears: the frame's centre column, on the last row.
func (l Layout) home() pointF {
	return pointF{
		X: float64(l.Size.X) / 2,
		Y: float64(l.Size.Y-l.Padding) - float64(l.LineHeight)/2,
	}
}

// Compositor turns each scheduled snapshot, rasterized, into a shipped frame: it pushes into
// the section's zoom target, scales to the shipped width, draws the cursor of the clicks and
// drags on top at constant size and the storyboard's captions over everything. Everything it needs is fixed at construction, so composing is a
// pure function of the output time and the source frame, and a Compositor is safe for
// concurrent use.
type Compositor struct {
	layout     Layout
	out        image.Point
	background color.RGBA
	camera     camera
	cursor     cursorTrack
	// captions is the storyboard's captions laid out for the shipped frame; nil when it has none.
	captions *caption.Layer
}

// New plans the camera and the cursor of a clip: board's frame gives the shipped width, its
// beats the zooms and its camera the moves beside them, recorded the clicks, the drags and the
// snapshots a zoom or pan target resolves on (through resolver), clipSchedule the output clock,
// layout where cells sit in a source frame, background the colour a zoomed frame is cleared to
// before the push is drawn, and captionFaces the faces board's captions are drawn in (unread when
// it has none, so it may then be the zero set). It fails on a layout with no size, a frame width
// that is not positive, a zoomed beat whose target resolves on none of its snapshots, a camera
// move whose start is not on the clip's clock, a pan target no screen of its move shows, and a
// caption [caption.New] refuses.
func New(
	board *storyboard.Storyboard,
	recorded *take.Take,
	clipSchedule *schedule.Schedule,
	resolver target.Resolver,
	layout Layout,
	background color.RGBA,
	captionFaces fonts.CaptionSet,
) (*Compositor, error) {
	if layout.Size.X <= 0 || layout.Size.Y <= 0 || layout.CellWidth <= 0 || layout.LineHeight <= 0 {
		return nil, fmt.Errorf("compose: invalid cell layout %+v", layout)
	}
	width := board.Frame.Width
	if width <= 0 {
		return nil, fmt.Errorf("compose: frame.width: want >0, got %d", width)
	}
	height := max(1, int(math.Round(float64(layout.Size.Y)*float64(width)/float64(layout.Size.X))))

	planner := zoomPlanner{board: board, recorded: recorded, resolver: resolver, layout: layout}
	clipCamera, err := planner.plan(clipSchedule)
	if err != nil {
		return nil, err
	}
	compositor := &Compositor{
		layout:     layout,
		out:        image.Pt(width, height),
		background: background,
		camera:     clipCamera,
		cursor:     trackCursor(recorded, clipSchedule, layout),
	}
	if len(board.Captions) > 0 {
		if compositor.captions, err = caption.New(board, captionFaces, compositor.out, compositor.unit()); err != nil {
			return nil, err
		}
	}
	return compositor, nil
}

// Size is the shipped frame's pixel size.
func (c *Compositor) Size() image.Point { return c.out }

// Compose builds the shipped frame at output time t from src, the rasterized snapshot the
// schedule shows then: the camera's crop scaled to the shipped width, the cursor over it and the
// captions showing at t last, in output space, so no zoom or pan moves or scales them. src is
// only read.
func (c *Compositor) Compose(t time.Duration, src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(image.Rectangle{Max: c.out})
	crop := c.crop(t)
	if crop.W >= float64(c.layout.Size.X) {
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	} else {
		draw.Draw(dst, dst.Bounds(), &image.Uniform{C: c.background}, image.Point{}, draw.Src)
		scaleX, scaleY := float64(c.out.X)/crop.W, float64(c.out.Y)/crop.H
		transform := f64.Aff3{scaleX, 0, -crop.X * scaleX, 0, scaleY, -crop.Y * scaleY}
		draw.CatmullRom.Transform(dst, transform, src, src.Bounds(), draw.Src, nil)
	}
	c.drawCursor(dst, t, crop)
	if c.captions != nil {
		c.captions.Draw(dst, t)
	}
	return dst
}

// crop is the source rectangle the camera shows at output time t: the whole frame at a factor of
// 1, else a frame-shaped rectangle 1/factor its size centred on the camera's centre and clamped
// inside the frame.
func (c *Compositor) crop(t time.Duration) rectF {
	full := rectF{W: float64(c.layout.Size.X), H: float64(c.layout.Size.Y)}
	factor := c.camera.factorAt(t)
	if factor <= 1 {
		return full
	}
	centre := c.camera.centreAt(t)
	w, h := full.W/factor, full.H/factor
	return rectF{
		X: math.Min(math.Max(centre.X-w/2, 0), full.W-w),
		Y: math.Min(math.Max(centre.Y-h/2, 0), full.H-h),
		W: w,
		H: h,
	}
}

// toOutput maps a source pixel through the crop onto the shipped frame.
func (c *Compositor) toOutput(p pointF, crop rectF) pointF {
	return pointF{
		X: (p.X - crop.X) * float64(c.out.X) / crop.W,
		Y: (p.Y - crop.Y) * float64(c.out.Y) / crop.H,
	}
}

// fromOutput maps a pixel of the shipped frame back through the crop onto the source frame: the
// inverse of [Compositor.toOutput].
func (c *Compositor) fromOutput(p pointF, crop rectF) pointF {
	return pointF{
		X: crop.X + p.X*crop.W/float64(c.out.X),
		Y: crop.Y + p.Y*crop.H/float64(c.out.Y),
	}
}

// Cell returns the terminal cell under p, a pixel of the shipped frame at output time t: the
// pixel's centre is mapped back through the scale to the shipped width and the zoom's crop at t,
// so a pick on a zoomed frame lands on the cell the frame shows there. ok is false when p lies
// outside the shipped frame or its centre falls on the padding around the grid; col and row are
// then the nearest cell, clamped into the grid, so a drag that runs off the grid still spans the
// cells up to its edge.
func (c *Compositor) Cell(t time.Duration, p image.Point) (col, row int, ok bool) {
	source := c.fromOutput(pointF{X: float64(p.X) + 0.5, Y: float64(p.Y) + 0.5}, c.crop(t))
	column := math.Floor((source.X - float64(c.layout.Padding)) / float64(c.layout.CellWidth))
	line := math.Floor((source.Y - float64(c.layout.Padding)) / float64(c.layout.LineHeight))
	grid := c.layout.grid()
	isOnGrid := column >= 0 && line >= 0 && int(column) < grid.X && int(line) < grid.Y
	ok = p.In(image.Rectangle{Max: c.out}) && isOnGrid
	col = min(max(int(column), 0), grid.X-1)
	row = min(max(int(line), 0), grid.Y-1)
	return col, row, ok
}

// Rect returns the pixel rectangle of the shipped frame at output time t that the cell box b
// covers, through the zoom's crop at t and the scale to the shipped width, rounded outwards to
// whole pixels and clipped to the frame; a box the crop leaves out entirely is the empty
// rectangle.
func (c *Compositor) Rect(t time.Duration, b take.Box) image.Rectangle {
	crop := c.crop(t)
	covered := c.layout.boxRect(b)
	topLeft := c.toOutput(pointF{X: covered.X, Y: covered.Y}, crop)
	bottomRight := c.toOutput(pointF{X: covered.X + covered.W, Y: covered.Y + covered.H}, crop)
	rect := image.Rect(
		int(math.Floor(topLeft.X)),
		int(math.Floor(topLeft.Y)),
		int(math.Ceil(bottomRight.X)),
		int(math.Ceil(bottomRight.Y)),
	)
	return rect.Intersect(image.Rectangle{Max: c.out})
}

// unit is how many shipped pixels one design pixel spans: the source frame's scale against
// designScale, times the downscale to the shipped width. The zoom never enters it — the cursor
// keeps its size however far the frame pushes in.
func (c *Compositor) unit() float64 {
	scale := c.layout.Scale
	if scale <= 0 {
		scale = designScale
	}
	return float64(scale) / designScale * float64(c.out.X) / float64(c.layout.Size.X)
}

// drawCursor draws the ring of a click, press or release and then the dot at output time t, each
// positioned through the crop and sized in design pixels.
func (c *Compositor) drawCursor(dst *image.RGBA, t time.Duration, crop rectF) {
	unit := c.unit()
	if diameter, opacity, centre, ok := c.cursor.ring(t); ok {
		outer := diameter / 2 * unit
		paintAnnulus(dst, c.toOutput(centre, crop), outer-ringStroke*unit, outer, cursorColor, opacity)
	}
	opacity := c.cursor.opacity(t)
	if opacity <= 0 {
		return
	}
	centre := c.toOutput(c.cursor.position(t), crop)
	radius := cursorDiameter / 2 * unit
	inner := radius - cursorOutline*unit
	paintAnnulus(dst, centre, inner, radius, cursorOutlineColor, opacity)
	paintAnnulus(dst, centre, -1, inner, cursorColor, cursorDotOpacity*opacity)
}
