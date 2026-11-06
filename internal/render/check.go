package render

import (
	"errors"
	"image"
	"math"

	"github.com/airiclenz/photon/internal/caption"
	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/compose"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

// CheckCaptions lays loaded's captions out as a render would, without a take, in the config's
// caption faces. A command-launch clip lays them out on the shipped frame of a rasterizer built
// over the config's terminal fonts and theme at the frame's [rasterGeometry]; a url-launch clip
// on the shipped frame of its [pageOutput], with no rasterizer and no terminal font loaded, so a
// caption's font.size is in CSS pixels. A caption that does not fit — its box larger than the
// frame — is reported as a *[storyboard.ValidationError] under the storyboard's path whose one
// problem is that caption's *[storyboard.LocatedError], the first such caption only. A
// storyboard without captions is nil at once and loads no font. It fails too when a font cannot
// be loaded or the geometry is refused.
func CheckCaptions(loaded *clip.Clip) error {
	board := loaded.Storyboard
	if len(board.Captions) == 0 {
		return nil
	}
	out, unit, err := captionOutput(loaded)
	if err != nil {
		return err
	}
	captionFaces, err := loadCaptionFaces(loaded)
	if err != nil {
		return err
	}
	if _, err := caption.New(board, captionFaces, out, unit); err != nil {
		return asProblem(board.Path, err)
	}
	return nil
}

// captionOutput is the shipped frame and unit loaded's captions are laid out on: its
// [pageOutput] for a url-launch clip, else the [compose.Output] of the [cellLayout] of a
// rasterizer built at the frame's [rasterGeometry]. It fails when a terminal font cannot be
// loaded or the geometry is refused.
func captionOutput(loaded *clip.Clip) (image.Point, float64, error) {
	frame := loaded.Storyboard.Frame
	if loaded.Config.IsBrowser() {
		return pageOutput(frame)
	}
	rasterizer, err := newRasterizer(loaded)
	if err != nil {
		return image.Point{}, 0, err
	}
	return compose.Output(cellLayout(frame, nil, rasterizer), frame.Width)
}

// pageLayout is where the cells of a frame with a viewport sit in its frames: one cell per CSS
// pixel, frame.device_scale device pixels square, on a frame of the viewport in device pixels —
// each side rounded — times frame.scale, without padding. Layout.Scale is frame.scale; the device
// scale lives only in the cell size.
func pageLayout(frame storyboard.Frame) compose.Layout {
	deviceScale := frame.Scaling()
	return compose.Layout{
		Size: image.Pt(
			devicePixels(frame.Viewport.Width, deviceScale)*frame.Scale,
			devicePixels(frame.Viewport.Height, deviceScale)*frame.Scale,
		),
		Cell:  take.CellSize{W: deviceScale, H: deviceScale},
		Scale: frame.Scale,
	}
}

// devicePixels is cssPixels at deviceScale, rounded to whole device pixels.
func devicePixels(cssPixels int, deviceScale float64) int {
	return int(math.Round(float64(cssPixels) * deviceScale))
}

// pageOutput is the shipped frame and unit of a frame with a viewport: the [compose.Output] of
// its [pageLayout] at frame.width, the unit scaled by frame.device_scale, so a design pixel — a
// caption's CSS pixel — spans device_scale × frame.scale pixels of the frame before it is scaled
// to frame.width. It fails as [compose.Output] does.
func pageOutput(frame storyboard.Frame) (image.Point, float64, error) {
	out, unit, err := compose.Output(pageLayout(frame), frame.Width)
	if err != nil {
		return image.Point{}, 0, err
	}
	return out, unit * frame.Scaling(), nil
}

// asProblem files the caption problem in err under path the way a storyboard's own problems
// are reported, or returns err as it is when it names no caption.
func asProblem(path string, err error) error {
	var located *storyboard.LocatedError
	if !errors.As(err, &located) {
		return err
	}
	return &storyboard.ValidationError{Path: path, Problems: []error{located}}
}
