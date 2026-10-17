package compose

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"golang.org/x/image/draw"
	"golang.org/x/image/math/f64"

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

// home is where the very first cursor appears: the frame's centre column, on the last row.
func (l Layout) home() pointF {
	return pointF{
		X: float64(l.Size.X) / 2,
		Y: float64(l.Size.Y-l.Padding) - float64(l.LineHeight)/2,
	}
}

// Compositor turns each scheduled snapshot, rasterized, into a shipped frame: it pushes into
// the section's zoom target, scales to the shipped width and draws the cursor of the clicks and
// drags on top at constant size. Everything it needs is fixed at construction, so composing is a
// pure function of the output time and the source frame, and a Compositor is safe for
// concurrent use.
type Compositor struct {
	layout     Layout
	out        image.Point
	background color.RGBA
	zooms      []zoomPlan
	cursor     cursorTrack
}

// New plans the zooms and the cursor of a clip: board's frame gives the shipped width and its
// beats the zooms, recorded the clicks, the drags and the snapshots a zoom target resolves on
// (through resolver), clipSchedule the output clock, layout where cells sit in a source frame, and
// background the colour a zoomed frame is cleared to before the push is drawn. It fails on a
// layout with no size, a frame width that is not positive, and a zoomed beat whose target
// resolves on none of its snapshots.
func New(
	board *storyboard.Storyboard,
	recorded *take.Take,
	clipSchedule *schedule.Schedule,
	resolver target.Resolver,
	layout Layout,
	background color.RGBA,
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
	zooms, err := planner.plan(clipSchedule)
	if err != nil {
		return nil, err
	}
	return &Compositor{
		layout:     layout,
		out:        image.Pt(width, height),
		background: background,
		zooms:      zooms,
		cursor:     trackCursor(recorded, clipSchedule, layout),
	}, nil
}

// Size is the shipped frame's pixel size.
func (c *Compositor) Size() image.Point { return c.out }

// Compose builds the shipped frame at output time t from src, the rasterized snapshot the
// schedule shows then. src is only read.
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
	return dst
}

// crop is the source rectangle shown at output time t: the whole frame when nothing zooms, else
// a frame-shaped rectangle 1/factor its size centred on the target and clamped inside the frame.
func (c *Compositor) crop(t time.Duration) rectF {
	full := rectF{W: float64(c.layout.Size.X), H: float64(c.layout.Size.Y)}
	for _, zoom := range c.zooms {
		factor := zoom.factorAt(t)
		if factor <= 1 {
			continue
		}
		w, h := full.W/factor, full.H/factor
		return rectF{
			X: math.Min(math.Max(zoom.Centre.X-w/2, 0), full.W-w),
			Y: math.Min(math.Max(zoom.Centre.Y-h/2, 0), full.H-h),
			W: w,
			H: h,
		}
	}
	return full
}

// toOutput maps a source pixel through the crop onto the shipped frame.
func (c *Compositor) toOutput(p pointF, crop rectF) pointF {
	return pointF{
		X: (p.X - crop.X) * float64(c.out.X) / crop.W,
		Y: (p.Y - crop.Y) * float64(c.out.Y) / crop.H,
	}
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
