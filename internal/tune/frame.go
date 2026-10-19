package tune

import (
	"bytes"
	"fmt"
	"image"
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
