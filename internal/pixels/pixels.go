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

// patchGap is the fewest unchanged rows, or unchanged columns, that part two changes of a frame
// into separate patches. Changes closer than that share one rectangle: a patch's own PNG costs
// more than the few unchanged pixels between them.
const patchGap = 32

// ErrMalformed is wrapped by every error for an image layer a frame cannot be rebuilt from: no
// key frame before a snapshot, a key frame that is not one patch at (0, 0), a patch outside its
// frame, or bytes that are not the PNG a patch says they are.
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
// a patch entry holding one rectangle for each far-apart change since the previous frame, or nil
// when nothing changed, in which case the caller keeps no snapshot for the frame. The encoder
// copies what it keeps, so the caller may reuse frame's buffer for the next frame.
func (e *Encoder) Encode(at time.Duration, frame *image.RGBA) (*take.ImageEntry, error) {
	bounds := frame.Bounds()
	if bounds.Empty() {
		return nil, fmt.Errorf("encode the frame at %s: %w", at, errEmptyFrame)
	}
	if e.prev == nil || e.prev.Rect.Size() != bounds.Size() {
		return e.key(at, frame)
	}
	changed := e.changed(frame)
	if len(changed) == 0 {
		return nil, nil
	}
	area := 0
	for _, rect := range changed {
		area += rect.Dx() * rect.Dy()
	}
	if at-e.lastKey >= keyInterval || 2*area > bounds.Dx()*bounds.Dy() {
		return e.key(at, frame)
	}

	patches := make([]take.Patch, 0, len(changed))
	for _, rect := range changed {
		width := rect.Dx()
		for y := rect.Min.Y; y < rect.Max.Y; y++ {
			start := e.prev.PixOffset(rect.Min.X, y)
			copy(e.prev.Pix[start:start+width*bytesPerPixel], row(frame, rect.Min.X, y, width))
		}
		encoded, err := encodePNG(e.prev, rect)
		if err != nil {
			return nil, fmt.Errorf("encode the patch at %s: %w", at, err)
		}
		patches = append(patches, take.Patch{Rect: box(rect), PNG: encoded})
	}
	return &take.ImageEntry{Patches: patches}, nil
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
	patch := take.Patch{Rect: box(e.prev.Rect), PNG: encoded}
	return &take.ImageEntry{Key: true, Patches: []take.Patch{patch}}, nil
}

// changed returns the rectangles, in the frame's own pixels from (0, 0), that together hold every
// pixel where frame differs from the previous frame of the same size, and none when no pixel
// does. The changed rows split into bands at every run of at least patchGap unchanged rows, each
// band splits into spans at every run of at least patchGap unchanged columns, and each span's
// rectangle is the smallest one holding its changed pixels. The rectangles do not overlap and
// run top to bottom, then left to right.
func (e *Encoder) changed(frame *image.RGBA) []image.Rectangle {
	size := e.prev.Rect.Size()
	rowChanged := make([]bool, size.Y)
	for y := range size.Y {
		rowChanged[y] = !bytes.Equal(row(frame, 0, y, size.X), row(e.prev, 0, y, size.X))
	}
	var rects []image.Rectangle
	for _, band := range runs(rowChanged) {
		rects = append(rects, e.spans(frame, rowChanged, band)...)
	}
	return rects
}

// spans returns the rectangles of the changed pixels in band, a run of frame's rows, one for
// each run of changed columns; rowChanged says which of frame's rows differ at all.
func (e *Encoder) spans(frame *image.RGBA, rowChanged []bool, band run) []image.Rectangle {
	width := e.prev.Rect.Dx()
	columnChanged := make([]bool, width)
	for y := band.start; y < band.end; y++ {
		if !rowChanged[y] {
			continue
		}
		now, before := row(frame, 0, y, width), row(e.prev, 0, y, width)
		for x := range width {
			if !columnChanged[x] && !samePixel(now, before, x) {
				columnChanged[x] = true
			}
		}
	}

	var rects []image.Rectangle
	for _, span := range runs(columnChanged) {
		differs := func(y int) bool {
			return rowChanged[y] && !bytes.Equal(row(frame, span.start, y, span.end-span.start),
				row(e.prev, span.start, y, span.end-span.start))
		}
		top := band.start
		for !differs(top) {
			top++
		}
		bottom := band.end - 1
		for !differs(bottom) {
			bottom--
		}
		rects = append(rects, image.Rect(span.start, top, span.end, bottom+1))
	}
	return rects
}

// run is the rows or columns [start, end).
type run struct {
	start, end int
}

// runs returns the runs of changed's true indices, joining two runs fewer than patchGap apart;
// each run starts and ends at a true index.
func runs(changed []bool) []run {
	var found []run
	for index, isChanged := range changed {
		if !isChanged {
			continue
		}
		if last := len(found) - 1; last >= 0 && index-found[last].end < patchGap {
			found[last].end = index + 1
			continue
		}
		found = append(found, run{start: index, end: index + 1})
	}
	return found
}

// Decoder rebuilds the frames of a take's image layer. It keeps the frame it built last, so
// reading snapshots in order costs one entry each, and reading any other one costs at most the
// entries since its key frame.
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

// decodeKey returns the frame a key frame entry holds: its one patch, at (0, 0), whose size is
// the frame's.
func decodeKey(entry *take.ImageEntry) (*image.RGBA, error) {
	if len(entry.Patches) != 1 {
		return nil, fmt.Errorf("a key frame of %d patches, not one: %w", len(entry.Patches),
			ErrMalformed)
	}
	whole := entry.Patches[0]
	if whole.Rect.X != 0 || whole.Rect.Y != 0 {
		return nil, fmt.Errorf("a key frame at (%d, %d), not (0, 0): %w",
			whole.Rect.X, whole.Rect.Y, ErrMalformed)
	}
	return decodePNG(whole)
}

// applyPatch writes each of a patch entry's patches over its rectangle of frame, in order.
func applyPatch(frame *image.RGBA, entry *take.ImageEntry) error {
	for _, patch := range entry.Patches {
		rect := image.Rect(patch.Rect.X, patch.Rect.Y,
			patch.Rect.X+patch.Rect.W, patch.Rect.Y+patch.Rect.H)
		if !rect.In(frame.Rect) {
			return fmt.Errorf("a patch at %v outside the %v frame: %w", rect, frame.Rect.Size(),
				ErrMalformed)
		}
		decoded, err := decodePNG(patch)
		if err != nil {
			return err
		}
		for y := range rect.Dy() {
			start := frame.PixOffset(rect.Min.X, rect.Min.Y+y)
			copy(frame.Pix[start:start+rect.Dx()*bytesPerPixel], row(decoded, 0, y, rect.Dx()))
		}
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

// decodePNG returns the pixels a patch's PNG holds, with the bytes [encodePNG] wrote, checking
// that its size is the patch's rectangle's.
func decodePNG(patch take.Patch) (*image.RGBA, error) {
	decoded, err := png.Decode(bytes.NewReader(patch.PNG))
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
	if size := frame.Rect.Size(); size.X != patch.Rect.W || size.Y != patch.Rect.H {
		return nil, fmt.Errorf("a %v PNG for a %dx%d rectangle: %w", size, patch.Rect.W,
			patch.Rect.H, ErrMalformed)
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
