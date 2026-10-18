package render

import (
	"errors"

	"github.com/airiclenz/photon/internal/caption"
	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/compose"
	"github.com/airiclenz/photon/internal/storyboard"
)

// CheckCaptions lays loaded's captions out as a render would, without a take: on the shipped
// frame of a rasterizer built over the config's terminal fonts and theme at the frame's
// [rasterGeometry], in the config's caption faces. A caption that does not fit — its box larger
// than the frame — is reported as a *[storyboard.ValidationError] under the storyboard's path
// whose one problem is that caption's *[storyboard.LocatedError], the first such caption only.
// A storyboard without captions is nil at once and loads no font. It fails too when a font
// cannot be loaded or the geometry is refused.
func CheckCaptions(loaded *clip.Clip) error {
	board := loaded.Storyboard
	if len(board.Captions) == 0 {
		return nil
	}
	rasterizer, err := newRasterizer(loaded)
	if err != nil {
		return err
	}
	captionFaces, err := loadCaptionFaces(loaded)
	if err != nil {
		return err
	}
	out, unit, err := compose.Output(cellLayout(board.Frame, rasterizer), board.Frame.Width)
	if err != nil {
		return err
	}
	if _, err := caption.New(board, captionFaces, out, unit); err != nil {
		return asProblem(board.Path, err)
	}
	return nil
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
