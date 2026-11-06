package pixels

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"time"

	"github.com/airiclenz/photon/internal/take"
)

// keyInterval is the most of the take's clock that passes between two key frames, so seeking any
// snapshot decodes at most this stretch of the layer.
const keyInterval = 2 * time.Second

// bytesPerPixel is the size of one pixel in an [image.RGBA]'s Pix.
const bytesPerPixel = 4

// ErrMalformed is wrapped by every error for an image layer a frame cannot be rebuilt from: no
// key frame before a snapshot, a patch outside its frame, or bytes that are not the PNG an entry
// says they are.
var ErrMalformed = errors.New("malformed image layer")

// errEmptyFrame is the error for a frame with no pixels.
var errEmptyFrame = errors.New("the frame has no pixels")

// Encoder turns a capture's frames, in the order they were taken, into the take's image layer
// entries. It holds the frame before the current one, which it compares each new frame against.
// The zero value is ready to use.
type Encoder struct {
	prev    *image.RGBA
	lastKey time.Duration
}

// Encode returns the image layer entry for frame, taken at at on the take's clock: a key frame,
// a patch of the rectangle that changed since the previous frame, or nil when nothing changed, in
// which case the caller keeps no snapshot for the frame. The encoder copies what it keeps, so the
// caller may reuse frame's buffer for the next frame.
func (e *Encoder) Encode(at time.Duration, frame *image.RGBA) (*take.ImageEntry, error) {
	bounds := frame.Bounds()
	if bounds.Empty() {
		return nil, fmt.Errorf("encode the frame at %s: %w", at, errEmptyFrame)
	}
	if e.prev == nil || e.prev.Rect.Size() != bounds.Size() {
		return e.key(at, frame)
	}
	changed, isChanged := e.changed(frame)
	if !isChanged {
		return nil, nil
	}
	if at-e.lastKey >= keyInterval || 2*changed.Dx()*changed.Dy() > bounds.Dx()*bounds.Dy() {
		return e.key(at, frame)
	}

	width := changed.Dx()
	for y := changed.Min.Y; y < changed.Max.Y; y++ {
		start := e.prev.PixOffset(changed.Min.X, y)
		copy(e.prev.Pix[start:start+width*bytesPerPixel], row(frame, changed.Min.X, y, width))
	}
	encoded, err := encodePNG(e.prev, changed)
	if err != nil {
		return nil, fmt.Errorf("encode the patch at %s: %w", at, err)
	}
	return &take.ImageEntry{Rect: box(changed), PNG: encoded}, nil
}

// key makes frame the encoder's previous frame and returns it whole as a key frame.
func (e *Encoder) key(at time.Duration, frame *image.RGBA) (*take.ImageEntry, error) {
	size := frame.Bounds().Size()
	if e.prev == nil || e.prev.Rect.Size() != size {
		e.prev = image.NewRGBA(image.Rectangle{Max: size})
	}
	for y := range size.Y {
		start := e.prev.PixOffset(0, y)
		copy(e.prev.Pix[start:start+size.X*bytesPerPixel], row(frame, 0, y, size.X))
	}
	encoded, err := encodePNG(e.prev, e.prev.Rect)
	if err != nil {
		return nil, fmt.Errorf("encode the key frame at %s: %w", at, err)
	}
	e.lastKey = at
	return &take.ImageEntry{Key: true, Rect: box(e.prev.Rect), PNG: encoded}, nil
}

// changed returns the smallest rectangle, in the frame's own pixels from (0, 0), holding every
// pixel where frame differs from the previous frame of the same size, and false when none does.
func (e *Encoder) changed(frame *image.RGBA) (image.Rectangle, bool) {
	size := e.prev.Rect.Size()
	top, bottom := -1, -1
	for y := range size.Y {
		if !bytes.Equal(row(frame, 0, y, size.X), row(e.prev, 0, y, size.X)) {
			if top < 0 {
				top = y
			}
			bottom = y
		}
	}
	if top < 0 {
		return image.Rectangle{}, false
	}

	left, right := size.X, -1
	for y := top; y <= bottom; y++ {
		now, before := row(frame, 0, y, size.X), row(e.prev, 0, y, size.X)
		for x := 0; x < left; x++ {
			if !samePixel(now, before, x) {
				left = x
				break
			}
		}
		for x := size.X - 1; x > right; x-- {
			if !samePixel(now, before, x) {
				right = x
				break
			}
		}
	}
	return image.Rect(left, top, right+1, bottom+1), true
}

// Decoder rebuilds the frames of a take's image layer. It keeps the frame it built last, so
// reading snapshots in order costs one patch each, and reading any other one costs at most the
// patches since its key frame.
type Decoder struct {
	snapshots []take.Snapshot
	frame     *image.RGBA
	index     int
}

// NewDecoder returns a decoder over t's image layer.
func NewDecoder(t *take.Take) *Decoder {
	return &Decoder{snapshots: t.Snapshots, index: -1}
}

// Frame returns snapshot index's frame. A snapshot without an entry shows the frame before it.
// The image belongs to the decoder and the next call overwrites it: a caller that keeps a frame
// copies it. An error for a layer the frame cannot be rebuilt from wraps [ErrMalformed].
func (d *Decoder) Frame(index int) (*image.RGBA, error) {
	if index < 0 || index >= len(d.snapshots) {
		return nil, fmt.Errorf("no snapshot %d: the take has %d", index, len(d.snapshots))
	}
	key := index
	for key >= 0 && (d.snapshots[key].Image == nil || !d.snapshots[key].Image.Key) {
		key--
	}
	if key < 0 {
		return nil, fmt.Errorf("snapshot %d: no key frame at or before it: %w", index, ErrMalformed)
	}

	from := d.index + 1
	if d.frame == nil || d.index < key || d.index > index {
		d.index = -1
		frame, err := decodeKey(d.snapshots[key].Image)
		if err != nil {
			return nil, fmt.Errorf("snapshot %d: %w", key, err)
		}
		d.frame, from = frame, key+1
	}
	for i := from; i <= index; i++ {
		entry := d.snapshots[i].Image
		if entry == nil {
			continue
		}
		if err := applyPatch(d.frame, entry); err != nil {
			d.index = -1
			return nil, fmt.Errorf("snapshot %d: %w", i, err)
		}
	}
	d.index = index
	return d.frame, nil
}

// decodeKey returns the frame a key frame entry holds.
func decodeKey(entry *take.ImageEntry) (*image.RGBA, error) {
	if entry.Rect.X != 0 || entry.Rect.Y != 0 {
		return nil, fmt.Errorf("a key frame at (%d, %d), not (0, 0): %w",
			entry.Rect.X, entry.Rect.Y, ErrMalformed)
	}
	return decodePNG(entry)
}

// applyPatch writes a patch entry's pixels over its rectangle of frame.
func applyPatch(frame *image.RGBA, entry *take.ImageEntry) error {
	rect := image.Rect(entry.Rect.X, entry.Rect.Y,
		entry.Rect.X+entry.Rect.W, entry.Rect.Y+entry.Rect.H)
	if !rect.In(frame.Rect) {
		return fmt.Errorf("a patch at %v outside the %v frame: %w", rect, frame.Rect.Size(),
			ErrMalformed)
	}
	patch, err := decodePNG(entry)
	if err != nil {
		return err
	}
	for y := range rect.Dy() {
		start := frame.PixOffset(rect.Min.X, rect.Min.Y+y)
		copy(frame.Pix[start:start+rect.Dx()*bytesPerPixel], row(patch, 0, y, rect.Dx()))
	}
	return nil
}

// encodePNG returns the PNG of rect of frame, a frame whose bounds start at (0, 0). It writes the
// RGBA bytes verbatim, as a non-premultiplied image, so that [decodePNG] gets every byte back
// whatever its alpha; an opaque rectangle, where the two forms agree, is written without alpha.
func encodePNG(frame *image.RGBA, rect image.Rectangle) ([]byte, error) {
	verbatim := &image.NRGBA{
		Pix:    frame.Pix[frame.PixOffset(rect.Min.X, rect.Min.Y):],
		Stride: frame.Stride,
		Rect:   image.Rectangle{Max: rect.Size()},
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, verbatim); err != nil {
		return nil, err
	}
	return encoded.Bytes(), nil
}

// decodePNG returns the frame an entry's PNG holds, with the bytes [encodePNG] wrote, checking
// that its size is the entry's rectangle's.
func decodePNG(entry *take.ImageEntry) (*image.RGBA, error) {
	decoded, err := png.Decode(bytes.NewReader(entry.PNG))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrMalformed, err)
	}
	var frame *image.RGBA
	switch img := decoded.(type) {
	case *image.RGBA:
		frame = img
	case *image.NRGBA:
		frame = &image.RGBA{Pix: img.Pix, Stride: img.Stride, Rect: img.Rect}
	default:
		return nil, fmt.Errorf("a %T PNG, not RGBA: %w", decoded, ErrMalformed)
	}
	if size := frame.Rect.Size(); size.X != entry.Rect.W || size.Y != entry.Rect.H {
		return nil, fmt.Errorf("a %v PNG for a %dx%d rectangle: %w", size, entry.Rect.W,
			entry.Rect.H, ErrMalformed)
	}
	return frame, nil
}

// row returns the width pixels of img's row y from column x, both counted from img's top-left.
func row(img *image.RGBA, x, y, width int) []byte {
	start := img.PixOffset(img.Rect.Min.X+x, img.Rect.Min.Y+y)
	return img.Pix[start : start+width*bytesPerPixel]
}

// samePixel reports whether the rows a and b hold the same pixel at column x.
func samePixel(a, b []byte, x int) bool {
	start := x * bytesPerPixel
	return bytes.Equal(a[start:start+bytesPerPixel], b[start:start+bytesPerPixel])
}

// box is rect as a take [take.Box].
func box(rect image.Rectangle) take.Box {
	return take.Box{X: rect.Min.X, Y: rect.Min.Y, W: rect.Dx(), H: rect.Dy()}
}
