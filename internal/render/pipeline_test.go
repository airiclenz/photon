package render

import (
	"bytes"
	"context"
	"errors"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/schedule"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
)

// zoomedCaptionedStoryboard is captionedStoryboard with a camera zoom over beat 1, so the parity
// check covers the camera, the captions and their fades.
const zoomedCaptionedStoryboard = captionedStoryboard + `camera:
  - zoom: {to: 2, at: 100ms, over: 300ms}
`

// lostZoomStoryboard is renderStoryboard with beat 1 squeezed into 300ms — its 2s of take play
// above 6×, which the schedule warns about — and beat 2 zooming to a text no screen shows.
const lostZoomStoryboard = `clip: rendered
ship: out/rendered.gif
frame: {cols: 20, rows: 4, padding: 4, font_size: 10, line_height: 1.2, scale: 2, width: 160, fps: 10, max_colors: 16}
beats:
  - id: 1
    title: typing
    duration: 300ms
  - id: 2
    title: hold
    duration: 1500ms
    zoom:
      target: {text: 'nowhere'}
      factor: 1.5
`

// loadRenderTake writes the render tests' take of cols columns and loads it back, returning its
// path too.
func loadRenderTake(t *testing.T, cols int) (string, *take.Take) {
	t.Helper()
	takePath := saveRenderTake(t, t.TempDir(), cols)
	recorded, err := take.Load(takePath)
	if err != nil {
		t.Fatalf("take.Load: %v", err)
	}
	return takePath, recorded
}

// oracleFrames builds every scheduled frame of loaded over recorded without the pipeline: its own
// schedule, rasterizer and compositor, each frame's snapshot rasterized afresh.
func oracleFrames(t *testing.T, loaded *clip.Clip, recorded *take.Take) ([]schedule.Frame, []*image.RGBA) {
	t.Helper()
	clipSchedule, err := schedule.New(loaded.Storyboard, recorded)
	if err != nil {
		t.Fatalf("schedule.New: %v", err)
	}
	rasterizer, err := NewRasterizer(loaded)
	if err != nil {
		t.Fatalf("NewRasterizer: %v", err)
	}
	compositor, err := NewCompositor(loaded, recorded, clipSchedule, rasterizer)
	if err != nil {
		t.Fatalf("NewCompositor: %v", err)
	}
	frames := clipSchedule.Frames(loaded.Storyboard.Frame.FPS)
	composed := make([]*image.RGBA, 0, len(frames))
	for _, frame := range frames {
		index := recorded.ScreenAt(frame.Source)
		composed = append(composed, compositor.Compose(frame.Out, rasterizer.Frame(recorded.Snapshots[index])))
	}
	return frames, composed
}

func TestPipeline_FrameAt_EveryScheduledFrame_EqualsThePipelineFreeOracle(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClipOf(t, zoomedCaptionedStoryboard)
	takePath, recorded := loadRenderTake(t, 20)
	frames, want := oracleFrames(t, loaded, recorded)

	built, err := Build(loaded, takePath, recorded, newLastRaster())

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(frames) == 0 {
		t.Fatal("the oracle scheduled no frames")
	}
	for index, frame := range frames {
		got, err := built.FrameAt(frame.Out)
		if err != nil {
			t.Fatalf("FrameAt(%s): %v", frame.Out, err)
		}
		if got.Rect != want[index].Rect || !bytes.Equal(got.Pix, want[index].Pix) {
			t.Errorf("FrameAt(%s) differs from the oracle's frame", frame.Out)
		}
	}
}

func TestPipeline_ScreenAt_ShowsTheScheduledSnapshot(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClip(t)
	takePath, recorded := loadRenderTake(t, 20)
	built, err := Build(loaded, takePath, recorded, newLastRaster())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, frame := range built.Schedule().Frames(loaded.Storyboard.Frame.FPS) {
		got, err := built.ScreenAt(frame.Out)

		if err != nil {
			t.Fatalf("ScreenAt(%s): %v", frame.Out, err)
		}
		if want := recorded.Snapshots[recorded.ScreenAt(frame.Source)]; got.At != want.At {
			t.Errorf("ScreenAt(%s): want the snapshot at %s, got the one at %s", frame.Out, want.At, got.At)
		}
	}
}

func TestPipeline_OutsideTheClip_IsRefused(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClip(t)
	takePath, recorded := loadRenderTake(t, 20)
	built, err := Build(loaded, takePath, recorded, newLastRaster())
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, out := range []time.Duration{-time.Millisecond, built.Schedule().Length} {
		_, frameErr := built.FrameAt(out)
		_, screenErr := built.ScreenAt(out)

		if !errors.Is(frameErr, ErrOutsideClip) {
			t.Errorf("FrameAt(%s): want ErrOutsideClip, got %v", out, frameErr)
		}
		if !errors.Is(screenErr, ErrOutsideClip) {
			t.Errorf("ScreenAt(%s): want ErrOutsideClip, got %v", out, screenErr)
		}
	}
}

func TestBuild_ATakeOfAnotherFrame_IsRefusedWithoutAPipeline(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClip(t)
	takePath, recorded := loadRenderTake(t, 30)

	built, err := Build(loaded, takePath, recorded, newLastRaster())

	if built != nil {
		t.Error("want no pipeline for a take of another frame")
	}
	want := takePath + " was recorded at 30×4 cells; the storyboard's frame is 20×4 — re-record it"
	if err == nil || err.Error() != want {
		t.Fatalf("want %q, got %v", want, err)
	}
}

func TestBuild_AZoomTargetNotOnScreen_LeavesAPartialPipeline(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClipOf(t, lostZoomStoryboard)
	takePath, recorded := loadRenderTake(t, 20)

	built, err := Build(loaded, takePath, recorded, newLastRaster())

	if !errors.Is(err, target.ErrNotOnScreen) {
		t.Fatalf("want the zoom target not on screen, got %v", err)
	}
	if built == nil || built.Schedule() == nil {
		t.Fatal("want a partial pipeline holding its schedule")
	}
	if _, frameErr := built.FrameAt(0); !errors.Is(frameErr, err) {
		t.Errorf("FrameAt: want the build's error, got %v", frameErr)
	}
	if _, screenErr := built.ScreenAt(0); !errors.Is(screenErr, err) {
		t.Errorf("ScreenAt: want the build's error, got %v", screenErr)
	}
	if _, _, ok := built.Cell(0, image.Point{}); ok {
		t.Error("Cell: want no cell on a partial pipeline")
	}
	if size := built.Size(); size != (image.Point{}) {
		t.Errorf("Size: want zero on a partial pipeline, got %v", size)
	}
}

func TestRun_ScheduleWarnings_PrintBeforeAComposeError(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClipOf(t, lostZoomStoryboard)
	options := Options{Take: saveRenderTake(t, t.TempDir(), 20), DryRun: true}
	var stderr bytes.Buffer

	err := Run(context.Background(), loaded, options, &bytes.Buffer{}, &stderr)

	if !errors.Is(err, target.ErrNotOnScreen) {
		t.Fatalf("want the zoom target not on screen, got %v", err)
	}
	if !strings.HasPrefix(stderr.String(), "warning: beat 1 plays at ") {
		t.Errorf("want beat 1's speed warning on stderr, got %q", stderr.String())
	}
}
