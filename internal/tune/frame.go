package tune

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"time"
)

// ErrOutsideClip is a frame asked for at an output time outside [0, length) of the clip.
var ErrOutsideClip = errors.New("the time is outside the clip")

// Frame returns the PNG of the frame `photon render` composes at output time out, before the
// GIF's palette quantization, from the last pipeline that built in full: while an error stands,
// the preview keeps showing the storyboard as it was before it. A snapshot is rasterized once
// and kept in the pipeline's raster cache, bounded at [rasterCacheBytes]. It fails with
// [ErrOutsideClip] when out is not in [0, length) and when the take holds no snapshot to show.
//
// The pipeline is taken under the session's lock and the frame drawn outside it, so frames are
// served concurrently with each other and with edits.
func (s *Session) Frame(out time.Duration) ([]byte, error) {
	shown := s.goodPipeline()

	source, _, ok := shown.schedule.Source(out)
	if !ok {
		return nil, fmt.Errorf("%w: %s is not in [0, %s)", ErrOutsideClip, out, shown.schedule.Length)
	}
	raster, err := shown.rasterAt(source)
	if err != nil {
		return nil, err
	}
	var encoded bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&encoded, shown.compositor.Compose(out, raster)); err != nil {
		return nil, fmt.Errorf("encode the frame at %s: %w", out, err)
	}
	return encoded.Bytes(), nil
}

// goodPipeline returns a copy of the last pipeline that built in full, taken under the lock.
func (s *Session) goodPipeline() pipeline {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.good
}

// rasterAt returns the rasterized snapshot on screen at source time t, from the pipeline's raster
// cache or drawn and kept there.
func (p pipeline) rasterAt(t time.Duration) (*image.RGBA, error) {
	index := p.recorded.ScreenAt(t)
	if index < 0 {
		return nil, fmt.Errorf("the take holds no snapshot to show at %s", t)
	}
	if raster, ok := p.rasters.get(index); ok {
		return raster, nil
	}
	raster := p.rasterizer.Frame(p.recorded.Snapshots[index])
	p.rasters.put(index, raster)
	return raster, nil
}
