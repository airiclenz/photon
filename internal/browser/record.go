package browser

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"iter"
	"maps"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/airiclenz/photon/internal/pixels"
	"github.com/airiclenz/photon/internal/take"
)

// textLayer is what the page showed as text at one moment: its runs and, per selector, the boxes
// of the elements it picked, as [Browser.ReadText] reads them.
type textLayer struct {
	runs      []take.TextRun
	selectors map[string][]take.Box
}

// frame is one screencast frame: its picture in device pixels and the text layer read with it.
type frame struct {
	picture *image.RGBA
	layer   textLayer
}

// startRecording starts the tab's screencast, in PNG at the viewport's size in device pixels, and
// the pumps that turn its frames into the take, then watches for the tab or the browser to go.
func (b *Browser) startRecording(options Options) error {
	b.rec = newRecording(options.Width, options.Height, options.DeviceScale, options.FPS,
		time.Now())
	b.done = make(chan struct{})
	detached, err := chromedp.Detached(b.tab)
	if err != nil {
		return fmt.Errorf("watch the tab: %w", err)
	}
	go func() {
		defer close(b.done)
		select {
		case <-detached:
		case <-b.tab.Done():
		}
	}()

	ctx, stop := context.WithCancel(b.tab)
	b.stopRecording = stop
	b.recordingDone = make(chan struct{})
	// Subscribed before the screencast starts, so its first frame is not missed.
	events := chromedp.Events(ctx, page.ScreencastFrame)
	_, err = chromedp.Call(ctx, page.StartScreencast, page.StartScreencastParams{
		Format:    page.StartScreencastFormatPng,
		MaxWidth:  devicePixels(options.Width, options.DeviceScale),
		MaxHeight: devicePixels(options.Height, options.DeviceScale),
	})
	if err != nil {
		stop()
		return fmt.Errorf("start the page's screencast: %w", err)
	}

	b.frameReady = make(chan struct{}, 1)
	var pumps sync.WaitGroup
	pumps.Go(func() { b.receive(ctx, events) })
	pumps.Go(func() { b.record(ctx) })
	go func() {
		pumps.Wait()
		close(b.recordingDone)
	}()
	return nil
}

// receive acknowledges every screencast frame as it arrives, so the browser sends the next, and
// leaves it for record as the newest frame, until ctx ends or the screencast does. A frame it
// cannot read ends the screencast's frames and is the recording's error.
func (b *Browser) receive(ctx context.Context, events iter.Seq2[page.EventScreencastFrame, error]) {
	for event, err := range events {
		if err != nil {
			if ctx.Err() == nil {
				b.noteError(err)
			}
			return
		}
		arrived := time.Now()
		b.rec.arrived(arrived)
		ack := page.ScreencastFrameAckParams{SessionID: event.SessionID}
		if _, err := chromedp.Call(ctx, page.ScreencastFrameAck, ack); err != nil {
			return
		}
		b.frameMu.Lock()
		b.nextFrame, b.nextArrived = &event, arrived
		b.frameMu.Unlock()
		select {
		case b.frameReady <- struct{}{}:
		default:
		}
	}
}

// record turns the newest frame received into the take, and records a frame held for its frame
// interval once the interval's boundary falls, until ctx ends. A frame that arrives while record
// is still busy with the one before replaces any other frame waiting, so a host slower than the
// page records the newest picture and not a backlog.
func (b *Browser) record(ctx context.Context) {
	boundary := time.NewTimer(0)
	boundary.Stop()
	defer boundary.Stop()
	for {
		var due time.Time
		var held bool
		var err error
		select {
		case <-ctx.Done():
			return
		case <-b.frameReady:
			due, held, err = b.recordNextFrame()
		case <-boundary.C:
			due, held, err = b.rec.flush(time.Now())
		}
		b.noteError(err)
		if held {
			boundary.Reset(time.Until(due))
		}
	}
}

// recordNextFrame hands the newest frame received, if one waits, to the recording, with the text
// layer read now. A picture that does not decode is the recording's error, and the frame keeps
// only its text layer.
func (b *Browser) recordNextFrame() (due time.Time, held bool, err error) {
	b.frameMu.Lock()
	event, arrived := b.nextFrame, b.nextArrived
	b.nextFrame = nil
	b.frameMu.Unlock()
	if event == nil {
		return time.Time{}, false, nil
	}
	// Read before the decode, so the text layer is as close to the picture as it can be.
	layer := b.readLayer()
	picture, decodeErr := decodeFrame(event.Data)
	b.noteError(decodeErr)
	return b.rec.frame(arrived, frame{picture: picture, layer: layer})
}

// decodeFrame decodes a screencast frame's PNG into an RGBA picture.
func decodeFrame(data []byte) (*image.RGBA, error) {
	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode a screencast frame: %w", err)
	}
	if picture, isRGBA := decoded.(*image.RGBA); isRGBA {
		return picture, nil
	}
	bounds := decoded.Bounds()
	picture := image.NewRGBA(image.Rectangle{Max: bounds.Size()})
	draw.Draw(picture, picture.Rect, decoded, bounds.Min, draw.Src)
	return picture, nil
}

// devicePixels is length CSS pixels in device pixels at scale, rounded up.
func devicePixels(length int, scale float64) int64 {
	return int64(math.Ceil(float64(length) * scale))
}

// recording is the take a browser builds from its screencast frames, and the clock it stamps
// them on. Every time it is handed is a wall-clock time, so the browser passes time.Now() and a
// test passes the times it plays.
//
// It keeps at most one snapshot per frame interval (1/fps) of the take's clock: a frame that
// arrives sooner than that after the last snapshot is held as pending, a newer pending frame
// replacing an older one, and recorded at the interval's boundary. So the last picture before the
// page goes still is kept, and the take's rate never passes its fps. A frame the take records
// only when its picture or its text layer differs from the last snapshot's.
type recording struct {
	mu       sync.Mutex
	take     *take.Take
	encoder  pixels.Encoder
	interval time.Duration
	// start is the wall-clock time the take's clock counts from: provisionally when the
	// recording began, then the first frame's arrival, to which the clock is rebased.
	start time.Time
	// awaitingFrame holds until the first frame arrives; no snapshot is recorded before it.
	awaitingFrame bool
	frozen        bool
	// lastAt is the take-clock time of the last snapshot recorded.
	lastAt time.Duration
	// pending is the newest frame held for the next interval boundary, nil when none is.
	pending *frame
	// newest is the text layer of the newest frame, recorded or not.
	newest textLayer
	// lastFrame is when the newest frame arrived, or start before any did; processed is when the
	// newest frame the recording has taken in arrived. A frame arrived and not yet taken in keeps
	// the page from counting as still.
	lastFrame time.Time
	processed time.Time
}

// newRecording returns a recording, begun at start, of a cols by rows CSS-pixel viewport at scale
// device pixels per CSS pixel, kept at most fps snapshots a second. fps must be positive.
func newRecording(cols, rows int, scale float64, fps int, start time.Time) *recording {
	return &recording{
		take: &take.Take{
			Cols: cols, Rows: rows, FPS: fps, Started: start.UTC().Round(0),
			Cell: take.CellSize{W: scale, H: scale},
		},
		interval:      time.Second / time.Duration(fps),
		start:         start,
		awaitingFrame: true,
		lastFrame:     start,
		processed:     start,
	}
}

// frame takes in f, which arrived at now; a nil picture keeps the last one's. The first frame
// restarts the take's clock at now and is recorded at 0 s, any event logged before it moving to
// 0 s rather than being dropped. A later frame is recorded at once when a frame interval has
// passed since the last snapshot, and held as pending otherwise; held reports whether it was, and
// due when the boundary it waits for falls, at which the caller calls [recording.flush]. After
// [recording.freeze] a frame only counts as the page's activity. An error is one encoding the
// frame's picture.
func (r *recording) frame(now time.Time, f frame) (due time.Time, held bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.processed = now
	if now.After(r.lastFrame) {
		r.lastFrame = now
	}
	r.newest = f.layer
	if r.frozen {
		return time.Time{}, false, nil
	}
	if r.awaitingFrame {
		r.rebaseLocked(now)
		return time.Time{}, false, r.recordLocked(0, &f)
	}
	at := now.Sub(r.start)
	if at >= r.lastAt+r.interval {
		r.pending = nil
		return time.Time{}, false, r.recordLocked(at, &f)
	}
	r.pending = &f
	return r.dueLocked(), true, nil
}

// flush records the pending frame at its boundary once now has reached it. When a frame is still
// held, because now falls short of its boundary, held reports so and due gives the boundary.
func (r *recording) flush(now time.Time) (due time.Time, held bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending == nil {
		return time.Time{}, false, nil
	}
	if now.Before(r.dueLocked()) {
		return r.dueLocked(), true, nil
	}
	return time.Time{}, false, r.recordPendingLocked()
}

// match reports whether match holds for layer, the page's text layer read at now, and when it
// does records it into the take, stamped now: the screen a wait saw is a screen the take holds,
// even between two frame intervals. Its picture is the pending frame's, which it takes over, or
// else the last one recorded. Before the first frame and after [recording.freeze] a match is
// reported and nothing is recorded. An error is one encoding the pending picture.
func (r *recording) match(
	now time.Time,
	layer textLayer,
	match func(take.Snapshot) bool,
) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !match(r.screenLocked(now, layer)) {
		return false, nil
	}
	if r.awaitingFrame || r.frozen {
		return true, nil
	}
	f := frame{layer: layer}
	if r.pending != nil {
		f.picture = r.pending.picture
		r.pending = nil
	}
	return true, r.recordLocked(now.Sub(r.start), &f)
}

// screen is layer as a snapshot stamped now on the take's clock, read and not recorded.
func (r *recording) screen(now time.Time, layer textLayer) take.Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.screenLocked(now, layer)
}

// latest is the text layer of the newest frame, empty before any arrived.
func (r *recording) latest() textLayer {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.newest
}

// event logs event into the take, stamped now on the take's clock: whatever At it carries is
// replaced. After [recording.freeze] it is dropped.
func (r *recording) event(now time.Time, event take.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return
	}
	event.At = now.Sub(r.start)
	r.take.Events = append(r.take.Events, event)
}

// freeze ends the take where it stands: a pending frame is recorded at its boundary, and from
// then on no snapshot or event reaches the take. A second freeze does nothing.
func (r *recording) freeze() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return nil
	}
	r.frozen = true
	if r.pending == nil {
		return nil
	}
	return r.recordPendingLocked()
}

// arrived notes that a frame arrived at now, ahead of [recording.frame] taking it in.
func (r *recording) arrived(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastFrame = now
}

// isFrozen reports whether [recording.freeze] has ended the take.
func (r *recording) isFrozen() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.frozen
}

// quiet is how long before now the newest frame arrived, or the recording began when none has;
// it is zero while a frame that arrived waits to be taken in.
func (r *recording) quiet(now time.Time) time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.processed.Before(r.lastFrame) {
		return 0
	}
	return now.Sub(r.lastFrame)
}

// result is the take as recorded so far. The caller reads it only once nothing records any more.
func (r *recording) result() *take.Take {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.take
}

// rebaseLocked restarts the take's clock at now, moving every event logged so far back by the
// shift and onto 0 s at the earliest. The caller holds mu.
func (r *recording) rebaseLocked(now time.Time) {
	shift := now.Sub(r.start)
	for index := range r.take.Events {
		r.take.Events[index].At = max(r.take.Events[index].At-shift, 0)
	}
	r.start = now
	r.take.Started = now.UTC().Round(0)
	r.awaitingFrame = false
}

// dueLocked is the wall-clock time of the boundary a pending frame waits for: one frame interval
// after the last snapshot. The caller holds mu.
func (r *recording) dueLocked() time.Time {
	return r.start.Add(r.lastAt + r.interval)
}

// recordPendingLocked records the pending frame at its boundary. The caller holds mu and has
// checked a frame is pending.
func (r *recording) recordPendingLocked() error {
	f := r.pending
	r.pending = nil
	return r.recordLocked(r.lastAt+r.interval, f)
}

// recordLocked adds f to the take as a snapshot at at, unless its picture and text layer are the
// last snapshot's: a take holds a screen once, from the moment it first appeared. A frame without
// a picture keeps the last snapshot's. The caller holds mu.
func (r *recording) recordLocked(at time.Duration, f *frame) error {
	var entry *take.ImageEntry
	if f.picture != nil {
		encoded, err := r.encoder.Encode(at, f.picture)
		if err != nil {
			return err
		}
		entry = encoded
	}
	snapshot := r.screenLocked(r.start.Add(at), f.layer)
	snapshot.Image = entry
	if n := len(r.take.Snapshots); n > 0 && entry == nil &&
		sameLayer(r.take.Snapshots[n-1], snapshot) {
		return nil
	}
	r.take.Snapshots = append(r.take.Snapshots, snapshot)
	r.lastAt = at
	return nil
}

// screenLocked is layer as a snapshot stamped now on the take's clock. The caller holds mu.
func (r *recording) screenLocked(now time.Time, layer textLayer) take.Snapshot {
	return take.Snapshot{At: now.Sub(r.start), Runs: layer.runs, Selectors: layer.selectors}
}

// sameLayer reports whether two snapshots hold the same text layer: the same runs and the same
// boxes for every selector. Their times and pictures do not count.
func sameLayer(a, b take.Snapshot) bool {
	return slices.EqualFunc(a.Runs, b.Runs, sameRun) &&
		maps.EqualFunc(a.Selectors, b.Selectors, slices.Equal[[]take.Box])
}

// sameRun reports whether two runs hold the same text over the same box with the same edges.
func sameRun(a, b take.TextRun) bool {
	return a.Text == b.Text && a.Box == b.Box && slices.Equal(a.Edges, b.Edges)
}
