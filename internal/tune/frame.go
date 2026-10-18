package tune

import (
	"bytes"
	"fmt"
	"image/png"
	"time"
)

// Frame returns the PNG of the frame `photon render` composes at output time out, before the
// GIF's palette quantization, from the last pipeline that built in full: while an error stands,
// the preview keeps showing the storyboard as it was before it. A snapshot is rasterized once
// and kept in the pipeline's raster cache, bounded at [rasterCacheBytes]. It fails with
// [render.ErrOutsideClip] when out is not in [0, length) and when the take holds no snapshot to
// show.
//
// The pipeline is taken under the session's lock and the frame drawn outside it, so frames are
// served concurrently with each other and with edits.
func (s *Session) Frame(out time.Duration) ([]byte, error) {
	frame, err := s.goodPipeline().frames.FrameAt(out)
	if err != nil {
		return nil, err
	}
	var encoded bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&encoded, frame); err != nil {
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
