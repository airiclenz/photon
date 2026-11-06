package browser

import (
	"context"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/take"
)

// The recording the tests play frames into: a small viewport at device scale 2, kept at 10
// snapshots a second, so one frame interval is 100 ms.
const (
	recordCols  = 32
	recordRows  = 16
	recordScale = 2
	recordFPS   = 10
	interval    = 100 * time.Millisecond
)

// t0 is when every test's recording begins.
var t0 = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

func TestRecording_FirstFrameRebasesTheClock(t *testing.T) {
	rec := newRecording(recordCols, recordRows, recordScale, recordFPS, t0)
	rec.event(t0.Add(50*time.Millisecond), take.Event{Kind: take.EventBeatStart, Beat: 1})

	first := t0.Add(200 * time.Millisecond)
	playFrame(t, rec, first, picture(color.White), layer("hello"))
	rec.event(first.Add(30*time.Millisecond), take.Event{Kind: take.EventBeatStart, Beat: 2})

	got := rec.result()
	if !got.Started.Equal(first) {
		t.Errorf("Started = %v, want the first frame's arrival %v", got.Started, first)
	}
	if len(got.Snapshots) != 1 || got.Snapshots[0].At != 0 {
		t.Fatalf("snapshots = %v, want one at 0 s", ats(got))
	}
	if got.Snapshots[0].Image == nil || !got.Snapshots[0].Image.Key {
		t.Errorf("first snapshot's image = %+v, want a key frame", got.Snapshots[0].Image)
	}
	wantEvents := []time.Duration{0, 30 * time.Millisecond}
	for index, event := range got.Events {
		if event.At != wantEvents[index] {
			t.Errorf("event %d at %v, want %v", index, event.At, wantEvents[index])
		}
	}
	wantTake := take.Take{Cols: recordCols, Rows: recordRows, FPS: recordFPS,
		Cell: take.CellSize{W: recordScale, H: recordScale}}
	if got.Cols != wantTake.Cols || got.Rows != wantTake.Rows || got.FPS != wantTake.FPS ||
		got.Cell != wantTake.Cell {
		t.Errorf("take = %dx%d at %d fps, cell %+v; want %dx%d at %d fps, cell %+v",
			got.Cols, got.Rows, got.FPS, got.Cell,
			wantTake.Cols, wantTake.Rows, wantTake.FPS, wantTake.Cell)
	}
}

func TestRecording_KeepsAFrameOnlyWhenItChangesTheScreen(t *testing.T) {
	cases := []struct {
		name      string
		picture   *image.RGBA
		layer     textLayer
		wantSnaps int
		wantImage string
	}{
		{name: "same picture and text", picture: picture(color.White), layer: layer("hello"),
			wantSnaps: 1},
		{name: "same picture, new text", picture: picture(color.White), layer: layer("bye"),
			wantSnaps: 2, wantImage: "none"},
		{name: "new picture, same text", picture: marked(color.White), layer: layer("hello"),
			wantSnaps: 2, wantImage: "patch"},
		{name: "same text, new selector boxes", picture: picture(color.White),
			layer:     withBox(layer("hello"), take.Box{X: 1, Y: 2, W: 3, H: 4}),
			wantSnaps: 2, wantImage: "none"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := newRecording(recordCols, recordRows, recordScale, recordFPS, t0)
			playFrame(t, rec, t0, picture(color.White), layer("hello"))
			playFrame(t, rec, t0.Add(interval), tc.picture, tc.layer)

			got := rec.result()
			if len(got.Snapshots) != tc.wantSnaps {
				t.Fatalf("snapshots = %v, want %d", ats(got), tc.wantSnaps)
			}
			if tc.wantSnaps == 1 {
				return
			}
			second := got.Snapshots[1]
			if second.At != interval {
				t.Errorf("second snapshot at %v, want %v", second.At, interval)
			}
			if kind := imageKind(second.Image); kind != tc.wantImage {
				t.Errorf("second snapshot's image = %s, want %s", kind, tc.wantImage)
			}
			if !sameLayer(second, take.Snapshot{Runs: tc.layer.runs, Selectors: tc.layer.selectors}) {
				t.Errorf("second snapshot's text layer = %+v, want %+v", second.Runs, tc.layer.runs)
			}
		})
	}
}

func TestRecording_HoldsAFrameSoonerThanOneIntervalForTheBoundary(t *testing.T) {
	rec := newRecording(recordCols, recordRows, recordScale, recordFPS, t0)
	playFrame(t, rec, t0, picture(color.White), layer("one"))

	due, held, err := rec.frame(t0.Add(30*time.Millisecond),
		frame{picture: marked(color.Black), layer: layer("two")})
	if err != nil || !held || !due.Equal(t0.Add(interval)) {
		t.Fatalf("frame at 30 ms: due %v, held %t, err %v; want held for %v",
			due, held, err, t0.Add(interval))
	}
	// A newer frame inside the same interval replaces the held one.
	if _, held, err := rec.frame(t0.Add(60*time.Millisecond),
		frame{picture: marked(color.White), layer: layer("three")}); err != nil || !held {
		t.Fatalf("frame at 60 ms: held %t, err %v; want held", held, err)
	}
	if _, held, err := rec.flush(t0.Add(99 * time.Millisecond)); err != nil || !held {
		t.Fatalf("flush before the boundary: held %t, err %v; want still held", held, err)
	}
	if got := len(rec.result().Snapshots); got != 1 {
		t.Fatalf("before the boundary the take has %d snapshots, want 1", got)
	}
	if _, held, err := rec.flush(t0.Add(130 * time.Millisecond)); err != nil || held {
		t.Fatalf("flush past the boundary: held %t, err %v; want recorded", held, err)
	}

	got := rec.result()
	if len(got.Snapshots) != 2 {
		t.Fatalf("snapshots = %v, want 2", ats(got))
	}
	second := got.Snapshots[1]
	if second.At != interval || textOf(second) != "three" || imageKind(second.Image) != "patch" {
		t.Errorf("second snapshot = %v %q %s, want the newest frame at the boundary %v as a patch",
			second.At, textOf(second), imageKind(second.Image), interval)
	}
	if _, held, _ := rec.flush(t0.Add(time.Second)); held {
		t.Error("a second flush still holds a frame")
	}
}

func TestRecording_FreezeRecordsTheHeldFrameAndEndsTheTake(t *testing.T) {
	rec := newRecording(recordCols, recordRows, recordScale, recordFPS, t0)
	playFrame(t, rec, t0, picture(color.White), layer("one"))
	if _, _, err := rec.frame(t0.Add(40*time.Millisecond),
		frame{picture: picture(color.White), layer: layer("two")}); err != nil {
		t.Fatalf("frame: %v", err)
	}

	if err := rec.freeze(); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	playFrame(t, rec, t0.Add(time.Second), marked(color.Black), layer("after"))
	rec.event(t0.Add(time.Second), take.Event{Kind: take.EventBeatStart, Beat: 1})

	got := rec.result()
	if len(got.Snapshots) != 2 || got.Snapshots[1].At != interval ||
		textOf(got.Snapshots[1]) != "two" {
		t.Errorf("snapshots = %v, want the held frame recorded at %v and nothing after", ats(got),
			interval)
	}
	if len(got.Events) != 0 {
		t.Errorf("events = %+v, want none after the freeze", got.Events)
	}
}

func TestRecording_MatchRecordsTheScreenThatMatched(t *testing.T) {
	hasText := func(want string) func(take.Snapshot) bool {
		return func(s take.Snapshot) bool { return textOf(s) == want }
	}

	t.Run("before the first frame it reports and records nothing", func(t *testing.T) {
		rec := newRecording(recordCols, recordRows, recordScale, recordFPS, t0)
		matched, err := rec.match(t0.Add(time.Millisecond), layer("ready"), hasText("ready"))
		if err != nil || !matched || len(rec.result().Snapshots) != 0 {
			t.Errorf("match = %t, err %v, %d snapshots; want true and none recorded",
				matched, err, len(rec.result().Snapshots))
		}
	})

	t.Run("a miss records nothing", func(t *testing.T) {
		rec := newRecording(recordCols, recordRows, recordScale, recordFPS, t0)
		playFrame(t, rec, t0, picture(color.White), layer("busy"))
		matched, err := rec.match(t0.Add(time.Millisecond), layer("busy"), hasText("ready"))
		if err != nil || matched || len(rec.result().Snapshots) != 1 {
			t.Errorf("match = %t, err %v, %d snapshots; want false and one",
				matched, err, len(rec.result().Snapshots))
		}
	})

	t.Run("a match between intervals is recorded now with the held picture", func(t *testing.T) {
		rec := newRecording(recordCols, recordRows, recordScale, recordFPS, t0)
		playFrame(t, rec, t0, picture(color.White), layer("busy"))
		if _, _, err := rec.frame(t0.Add(10*time.Millisecond),
			frame{picture: marked(color.White), layer: layer("busy")}); err != nil {
			t.Fatalf("frame: %v", err)
		}
		at := 20 * time.Millisecond
		matched, err := rec.match(t0.Add(at), layer("ready"), hasText("ready"))
		if err != nil || !matched {
			t.Fatalf("match = %t, err %v; want true", matched, err)
		}
		if _, held, _ := rec.flush(t0.Add(time.Second)); held {
			t.Error("the match left the held frame held")
		}
		got := rec.result()
		if len(got.Snapshots) != 2 {
			t.Fatalf("snapshots = %v, want 2", ats(got))
		}
		second := got.Snapshots[1]
		if second.At != at || textOf(second) != "ready" || imageKind(second.Image) != "patch" {
			t.Errorf("matched snapshot = %v %q %s, want %v %q with the held picture's patch",
				second.At, textOf(second), imageKind(second.Image), at, "ready")
		}
	})

	t.Run("a match on the screen as recorded adds nothing", func(t *testing.T) {
		rec := newRecording(recordCols, recordRows, recordScale, recordFPS, t0)
		playFrame(t, rec, t0, picture(color.White), layer("ready"))
		matched, err := rec.match(t0.Add(time.Millisecond), layer("ready"), hasText("ready"))
		if err != nil || !matched || len(rec.result().Snapshots) != 1 {
			t.Errorf("match = %t, err %v, %d snapshots; want true and still one",
				matched, err, len(rec.result().Snapshots))
		}
	})
}

func TestSettle_ReportsQuietTimeoutCancelAndGone(t *testing.T) {
	// The page has been still an hour, but a quiet spell counts from Settle's call at the earliest.
	const spell, short, long = 20 * time.Millisecond, 50 * time.Millisecond, 5 * time.Second
	cases := []struct {
		name      string
		quiet     time.Duration
		timeout   time.Duration
		waiting   bool
		cancelled bool
		gone      bool
		want      bool
	}{
		{name: "still for quiet", quiet: spell, timeout: long, want: true},
		{name: "quiet longer than the timeout", quiet: short, timeout: spell, want: false},
		{name: "timeout before quiet", quiet: time.Hour, timeout: short, want: false},
		{name: "a frame not yet taken in", quiet: spell, timeout: short, waiting: true, want: false},
		{name: "cancelled", quiet: time.Hour, timeout: long, cancelled: true, want: false},
		{name: "page gone", quiet: time.Hour, timeout: long, gone: true, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			begun := time.Now().Add(-time.Hour)
			b := &Browser{
				rec:  newRecording(recordCols, recordRows, recordScale, recordFPS, begun),
				done: make(chan struct{}),
			}
			if tc.waiting {
				b.rec.arrived(begun.Add(time.Minute))
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			if tc.gone {
				close(b.done)
			}
			if got := b.Settle(ctx, tc.quiet, tc.timeout); got != tc.want {
				t.Errorf("Settle = %t, want %t", got, tc.want)
			}
		})
	}
}

// playFrame hands rec a frame of pic and l arriving at now and fails t on an error.
func playFrame(t *testing.T, rec *recording, now time.Time, pic *image.RGBA, l textLayer) {
	t.Helper()
	if _, _, err := rec.frame(now, frame{picture: pic, layer: l}); err != nil {
		t.Fatalf("frame at %v: %v", now.Sub(t0), err)
	}
}

// picture is a frame of the recording's size in device pixels, all c.
func picture(c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, recordCols*recordScale, recordRows*recordScale))
	for y := range img.Rect.Dy() {
		for x := range img.Rect.Dx() {
			img.Set(x, y, c)
		}
	}
	return img
}

// marked is picture(c) with a small red square in its top left corner.
func marked(c color.Color) *image.RGBA {
	img := picture(c)
	for y := range 4 {
		for x := range 4 {
			img.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	return img
}

// layer is a text layer of one run of text on the first line.
func layer(text string) textLayer {
	edges := make([]int, len(text)+1)
	for index := range edges {
		edges[index] = index * 8
	}
	return textLayer{runs: []take.TextRun{{
		Text: text, Box: take.Box{W: len(text) * 8, H: 16}, Edges: edges,
	}}}
}

// withBox is l with box as the one box of the selector "button".
func withBox(l textLayer, box take.Box) textLayer {
	l.selectors = map[string][]take.Box{"button": {box}}
	return l
}

// textOf is the text of a snapshot's runs, joined.
func textOf(s take.Snapshot) string {
	var text string
	for _, run := range s.Runs {
		text += run.Text
	}
	return text
}

// imageKind names an image entry: "key", "patch" or "none".
func imageKind(entry *take.ImageEntry) string {
	switch {
	case entry == nil:
		return "none"
	case entry.Key:
		return "key"
	}
	return "patch"
}

// ats lists a take's snapshot times, for a failure message.
func ats(t *take.Take) []time.Duration {
	times := make([]time.Duration, len(t.Snapshots))
	for index, snapshot := range t.Snapshots {
		times[index] = snapshot.At
	}
	return times
}
