// Package pixels owns a take's image layer: it turns the frames a capture source grabs into the
// take's [take.ImageEntry] values and turns those back into frames at any snapshot.
//
// The layer is key frames and patches. The [Encoder] writes a key frame (the whole frame as a
// PNG) at the first frame, whenever the frame's size changes, when at least two seconds of the
// take's clock have passed since the last key frame, and when the rectangle that changed covers
// more than half the frame; any other changed frame is a PNG patch of the rectangle that changed.
// A frame identical to the one before it gets no entry. The [Decoder] seeks a snapshot by
// decoding the last key frame at or before it and applying the patches after, and it keeps the
// frame it last built, so reading the snapshots in order costs one patch each.
//
// Frames round-trip bit for bit: a PNG holds a frame's RGBA bytes verbatim, which for an opaque
// frame, as every screen capture is, is exactly the picture.
//
// It imports `take` and the standard library's image packages only: `take` carries the encoded
// bytes and stays free of image code.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - pixels.go: the [Encoder] that turns frames into image layer entries and the [Decoder]
//     that turns them back into the frame at any snapshot.
package pixels
