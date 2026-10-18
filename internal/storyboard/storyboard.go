package storyboard

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Storyboard is one clip's storyboard: the single source of a clip. The header names the clip,
// where its GIF ships and the terminal frame; each beat says what photon does (Do), how long the
// beat plays in the clip (Duration, Hold, Cut, Zoom) and what a keeper take must show (Expect).
// Expect at the top level holds what the take as a whole must show, Camera the camera moves
// placed on the clip's output clock, and Captions the text drawn over the composed frame on that
// clock.
//
// [Load] and [Parse] are the only ways to get one: a Storyboard in hand has passed validation,
// its Ship is resolved against the file's directory, its zoom ramps, camera eases and captions
// carry their defaults, and its camera moves and captions carry their resolved places on the
// output clock and, for a caption, its colours.
type Storyboard struct {
	// Path is the storyboard file Load read or Parse was handed, as the caller named it.
	Path string `yaml:"-"`
	Clip string `yaml:"clip"`
	// Ship is written relative to the storyboard's own directory and resolved by Load, so a
	// caller in any working directory finds it.
	Ship     string       `yaml:"ship"`
	Frame    Frame        `yaml:"frame"`
	Expect   []Expect     `yaml:"expect"`
	Beats    []Beat       `yaml:"beats"`
	Camera   []CameraMove `yaml:"camera"`
	Captions []Caption    `yaml:"captions"`
}

// Frame is the recorded terminal and the shipped geometry: the pty's Cols × Rows, the
// rasterizer's Padding (pixels), FontSize (points) and LineHeight (a multiple of the font size),
// the Scale frames are rasterized at, the shipped GIF's Width in pixels, its FPS and its palette
// size. Pixel size is derived from the font metrics by the rasterizer, not here.
type Frame struct {
	Cols       int     `yaml:"cols"`
	Rows       int     `yaml:"rows"`
	Padding    int     `yaml:"padding"`
	FontSize   float64 `yaml:"font_size"`
	LineHeight float64 `yaml:"line_height"`
	Scale      int     `yaml:"scale"`
	Width      int     `yaml:"width"`
	FPS        int     `yaml:"fps"`
	MaxColors  int     `yaml:"max_colors"`
}

// Load reads and validates the storyboard at path, exactly as [Parse] does with the file's bytes.
// A file that cannot be read is an error wrapping the read's own, so errors.Is reaches
// fs.ErrNotExist.
func Load(path string) (*Storyboard, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read storyboard: %w", err)
	}
	return Parse(raw, path)
}

// Parse decodes and validates a storyboard from data, the contents of the file at path. path
// names the storyboard in errors, becomes its Path and is the base a relative Ship resolves
// against; Parse never reads it. The decode is strict — an unknown field is an error — and every
// validation rule the storyboard breaks is reported at once, in a *[ValidationError], so one lint
// pass fixes the file in one round. An area name is checked for its spelling only; whether the
// config declares it is the caller's cross-check.
func Parse(data []byte, path string) (*Storyboard, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var board Storyboard
	// An empty file decodes to io.EOF; it is an empty storyboard, which validation then reports
	// field by field rather than as a bare end-of-file.
	if err := decoder.Decode(&board); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	board.Path = path
	if board.Ship != "" && !filepath.IsAbs(board.Ship) {
		board.Ship = filepath.Join(filepath.Dir(path), board.Ship)
	}
	if problems := board.validate(); len(problems) > 0 {
		return nil, &ValidationError{Path: path, Problems: problems}
	}
	board.applyDefaults()
	return &board, nil
}

// BeatIDs returns the ids of the storyboard's beats in storyboard order, its cut beats included.
func (s *Storyboard) BeatIDs() []int {
	ids := make([]int, 0, len(s.Beats))
	for _, beat := range s.Beats {
		ids = append(ids, beat.ID)
	}
	return ids
}

// applyDefaults fills what a valid storyboard may leave out: a zoom ramp left at zero runs
// [DefaultZoomRamp], a camera move without an ease eases [EaseSmooth], and a caption takes the
// defaults [Caption] lists.
func (s *Storyboard) applyDefaults() {
	for index := range s.Captions {
		s.Captions[index].applyDefaults(s.Frame.FontSize)
	}
	for _, move := range s.Camera {
		switch {
		case move.Zoom != nil && move.Zoom.Ease == "":
			move.Zoom.Ease = EaseSmooth
		case move.Pan != nil && move.Pan.Ease == "":
			move.Pan.Ease = EaseSmooth
		}
	}
	for index := range s.Beats {
		zoom := s.Beats[index].Zoom
		if zoom == nil {
			continue
		}
		if zoom.In == 0 {
			zoom.In = DefaultZoomRamp
		}
		if zoom.Out == 0 {
			zoom.Out = DefaultZoomRamp
		}
	}
}

// ValidationError lists every rule a storyboard breaks. A problem that belongs to one beat,
// camera move or caption is a *[LocatedError]; [OwnerOf] names any problem's owner.
type ValidationError struct {
	Path     string
	Problems []error
}

// Error joins the problems one per line, under the file's path.
func (e *ValidationError) Error() string {
	lines := make([]string, 0, len(e.Problems)+1)
	lines = append(lines, fmt.Sprintf("%s: %d problem(s)", e.Path, len(e.Problems)))
	for _, problem := range e.Problems {
		lines = append(lines, "  "+problem.Error())
	}
	return strings.Join(lines, "\n")
}

// Unwrap exposes the individual problems to errors.Is and errors.As.
func (e *ValidationError) Unwrap() []error { return e.Problems }
