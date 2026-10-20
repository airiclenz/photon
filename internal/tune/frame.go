package tune

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"time"

	"github.com/airiclenz/photon/internal/compose"
	"github.com/airiclenz/photon/internal/render"
)

// ErrNoPosition is wrapped by a camera view asked for the aim of a position the storyboard's
// camera.positions does not list.
var ErrNoPosition = errors.New("no such camera position")

// errNoCameraView is returned when the last good pipeline holds no compositor to answer the
// camera's view; a pipeline that built in full always holds one.
var errNoCameraView = errors.New("the preview holds no camera view")

// CameraView is where the camera looks at one output time, in pixels of the full view
// [Session.FullFrame] draws, and the cell the position asked for aims at.
type CameraView struct {
	// View is the camera's crop, the cell it centres on and where the grid's cells sit.
	View compose.View
	// Aim is the cell, column and row, the asked position aims at; nil when none was asked for.
	Aim *image.Point
}

// Frame returns the PNG of the frame `photon render` composes at output time out, before the
// GIF's palette quantization, from the last pipeline that built in full: while an error stands,
// the preview keeps showing the storyboard as it was before it. A snapshot is rasterized once
// and kept in the pipeline's raster cache, bounded at [rasterCacheBytes]. It fails with
// [render.ErrOutsideClip] when out is not in [0, length) and when the take holds no snapshot to
// show.
//
// The pipeline is taken under the session's lock and the frame drawn outside it, so frames are
// served concurrently with each other and with edits. The PNG is compressed at [png.BestSpeed].
func (s *Session) Frame(out time.Duration) ([]byte, error) {
	return encodeFrame(s.goodPipeline().frames.FrameAt, png.BestSpeed, out)
}

// DraftFrame returns the PNG of the draft frame at output time out, the preview's frame while it
// plays or scrubs: drawn from a raster at the shipped size with a fast scaler, of
// [Session.Frame]'s pixel size and with its cursor and captions, from the same last good
// pipeline. A snapshot's draft raster is drawn once and kept in the pipeline's draft raster
// cache, bounded at [draftCacheBytes]. The PNG is not compressed ([png.NoCompression]): encoding
// it is then far faster, and the page fetches it from this machine. It fails as [Session.Frame]
// does, and is served concurrently as it is.
func (s *Session) DraftFrame(out time.Duration) ([]byte, error) {
	return encodeFrame(s.goodPipeline().frames.DraftAt, png.NoCompression, out)
}

// FullFrame returns the PNG of the full view at output time out: the whole frame at
// [Session.Frame]'s pixel size, with its cursor and captions, so the camera's zoom and aim never
// enter; the page shows it while a camera position is edited. It is drawn from the same last good
// pipeline and raster cache, fails as [Session.Frame] does, and is served concurrently as it is.
func (s *Session) FullFrame(out time.Duration) ([]byte, error) {
	return encodeFrame(s.goodPipeline().frames.FullFrameAt, png.BestSpeed, out)
}

// Camera returns where the camera looks at output time out, from the last good pipeline, and,
// when position is not nil, the cell the position at that index in camera.positions aims at,
// both from the same pipeline. It fails with [render.ErrOutsideClip] when out is not in
// [0, length) and with [ErrNoPosition] when the storyboard lists no position at that index.
func (s *Session) Camera(out time.Duration, position *int) (CameraView, error) {
	frames := s.goodPipeline().frames
	view, ok := frames.ViewAt(out)
	if !ok {
		return CameraView{}, errNoCameraView
	}
	clipSchedule := frames.Schedule()
	if _, _, isInClip := clipSchedule.Source(out); !isInClip {
		return CameraView{}, fmt.Errorf(
			"%w: %s is not in [0, %s)", render.ErrOutsideClip, out, clipSchedule.Length,
		)
	}
	answer := CameraView{View: view}
	if position == nil {
		return answer, nil
	}
	col, row, ok := frames.PositionCell(*position)
	if !ok {
		return CameraView{}, fmt.Errorf(
			"%w: camera.positions has no index %d", ErrNoPosition, *position,
		)
	}
	answer.Aim = &image.Point{X: col, Y: row}
	return answer, nil
}

// goodPipeline returns a copy of the last pipeline that built in full, taken under the lock.
func (s *Session) goodPipeline() pipeline {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.good
}

// encodeFrame draws the frame at output time out with drawFrame and encodes it as a PNG at the
// compression level, fast rather than small: the page shows it once and drops it.
func encodeFrame(
	drawFrame func(time.Duration) (*image.RGBA, error),
	level png.CompressionLevel,
	out time.Duration,
) ([]byte, error) {
	frame, err := drawFrame(out)
	if err != nil {
		return nil, err
	}
	var encoded bytes.Buffer
	encoder := png.Encoder{CompressionLevel: level}
	if err := encoder.Encode(&encoded, frame); err != nil {
		return nil, fmt.Errorf("encode the frame at %s: %w", out, err)
	}
	return encoded.Bytes(), nil
}
