package compose

import (
	"image"
	"image/color"
	"math"
	"time"

	"github.com/airiclenz/photon/internal/schedule"
	"github.com/airiclenz/photon/internal/take"
)

// The cursor's look, in design pixels — pixels of a frame rasterized at designScale; a frame at
// another scale, and the downscale to the shipped width, multiply them through
// [Compositor.unit]. The dot is white at cursorDotOpacity inside a dark outline; each click grows
// a white ring from the dot's diameter to ringEndDiameter while it fades from ringStartOpacity.
const (
	designScale      = 2
	cursorDiameter   = 28.0
	cursorOutline    = 2.0
	cursorDotOpacity = 0.55
	ringEndDiameter  = 72.0
	ringStroke       = 3.0
	ringStartOpacity = 0.80
)

// The cursor's timing, on the output clock: it fades in over cursorFadeIn at the previous click
// point, glides to each click over cursorGlide ending at the press, rings for ringPulse after it,
// and starts fading out — over cursorFadeOut — cursorLinger after the beat's last click.
const (
	cursorFadeIn  = 400 * time.Millisecond
	cursorGlide   = 900 * time.Millisecond
	ringPulse     = 500 * time.Millisecond
	cursorLinger  = 1500 * time.Millisecond
	cursorFadeOut = 400 * time.Millisecond
)

// cursorOutlineColor is the dot's near-black outline, which keeps the dot's edge legible where it
// crosses a light row (a selection, a diff line).
var cursorOutlineColor = color.RGBA{0x11, 0x11, 0x1b, 0xff}

// cursorColor is the dot's and the ring's fill.
var cursorColor = color.RGBA{0xff, 0xff, 0xff, 0xff}

// clickMark is one click the cursor makes: its press on the output clock, the beat it belongs
// to and the clicked cell's centre in source pixels.
type clickMark struct {
	At    time.Duration
	Beat  int
	Point pointF
}

// cursorTrack is the cursor's whole path: every kept click in output order and the home point
// the first glide starts from.
type cursorTrack struct {
	Marks []clickMark
	Home  pointF
}

// trackCursor collects the take's clicks onto the output clock. A click the schedule gives no
// output time — one inside a cut beat — is dropped: the clip never shows it.
func trackCursor(recorded *take.Take, clipSchedule *schedule.Schedule, layout Layout) cursorTrack {
	track := cursorTrack{Home: layout.home()}
	for _, event := range recorded.Events {
		if event.Kind != take.EventClick || event.Cell == nil {
			continue
		}
		at, isShown := clipSchedule.Output(event.At)
		if !isShown {
			continue
		}
		track.Marks = append(track.Marks, clickMark{
			At:    at,
			Beat:  event.Beat,
			Point: layout.cellCentre(event.Cell.X, event.Cell.Y),
		})
	}
	return track
}

// position is where the cursor sits at output time t: at the previous click point (home before
// the first) until the glide to the next click starts, eased along the segment during it. A
// glide starts cursorGlide before its press, or at the previous press when they are closer.
func (c cursorTrack) position(t time.Duration) pointF {
	from := c.Home
	previous := time.Duration(math.MinInt64)
	for _, mark := range c.Marks {
		start := max(mark.At-cursorGlide, previous)
		if t < start {
			return from
		}
		if t < mark.At {
			return lerp(from, mark.Point, easeInOut(float64(t-start)/float64(mark.At-start)))
		}
		from, previous = mark.Point, mark.At
	}
	return from
}

// opacity is the cursor's visibility at output time t, 0 to 1: per beat that clicks, it rises
// over cursorFadeIn ending where the first glide starts, holds, and falls over cursorFadeOut
// starting cursorLinger after the beat's last press. Overlapping beats keep the brighter one.
func (c cursorTrack) opacity(t time.Duration) float64 {
	visible := 0.0
	for first := 0; first < len(c.Marks); {
		last := first
		for last+1 < len(c.Marks) && c.Marks[last+1].Beat == c.Marks[first].Beat {
			last++
		}
		shown := c.Marks[first].At - cursorGlide
		gone := c.Marks[last].At + cursorLinger
		rise := clamp01(float64(t-(shown-cursorFadeIn)) / float64(cursorFadeIn))
		fall := clamp01(float64(gone+cursorFadeOut-t) / float64(cursorFadeOut))
		visible = max(visible, min(rise, fall))
		first = last + 1
	}
	return visible
}

// ring is one click's pulse at output time t: its diameter in design pixels, its opacity and
// its centre in source pixels. ok is false outside every pulse; the latest press wins.
func (c cursorTrack) ring(t time.Duration) (diameter, opacity float64, centre pointF, ok bool) {
	for index := len(c.Marks) - 1; index >= 0; index-- {
		mark := c.Marks[index]
		if t < mark.At || t >= mark.At+ringPulse {
			continue
		}
		progress := float64(t-mark.At) / float64(ringPulse)
		diameter = cursorDiameter + (ringEndDiameter-cursorDiameter)*progress
		return diameter, ringStartOpacity * (1 - progress), mark.Point, true
	}
	return 0, 0, pointF{}, false
}

// paintAnnulus blends colour at opacity over every pixel between the radii inner and outer
// around centre, anti-aliased over one pixel at both edges. An inner radius below zero fills a
// disc.
func paintAnnulus(dst *image.RGBA, centre pointF, inner, outer float64, colour color.RGBA, opacity float64) {
	if opacity <= 0 || outer <= 0 {
		return
	}
	bounds := image.Rect(
		int(math.Floor(centre.X-outer-1)), int(math.Floor(centre.Y-outer-1)),
		int(math.Ceil(centre.X+outer+1)), int(math.Ceil(centre.Y+outer+1)),
	).Intersect(dst.Bounds())
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			distance := math.Hypot(float64(x)+0.5-centre.X, float64(y)+0.5-centre.Y)
			coverage := clamp01(outer - distance + 0.5)
			if inner >= 0 {
				coverage = min(coverage, clamp01(distance-inner+0.5))
			}
			if coverage > 0 {
				blendPixel(dst, x, y, colour, coverage*opacity)
			}
		}
	}
}

// blendPixel lays colour over the pixel at (x, y) with the given alpha.
func blendPixel(dst *image.RGBA, x, y int, colour color.RGBA, alpha float64) {
	offset := dst.PixOffset(x, y)
	pixel := dst.Pix[offset : offset+4 : offset+4]
	for channel, value := range [3]uint8{colour.R, colour.G, colour.B} {
		pixel[channel] = uint8(math.Round(float64(pixel[channel])*(1-alpha) + float64(value)*alpha))
	}
	pixel[3] = 0xff
}

// lerp is the point share of the way from a to b.
func lerp(a, b pointF, share float64) pointF {
	return pointF{a.X + (b.X-a.X)*share, a.Y + (b.Y-a.Y)*share}
}
