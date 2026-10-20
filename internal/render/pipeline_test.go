package render

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/config"
	"github.com/airiclenz/photon/internal/fonts"
	"github.com/airiclenz/photon/internal/schedule"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
)

// zoomedCaptionedStoryboard is captionedStoryboard with a camera zoom over beat 1, so the parity
// check covers the camera, the captions and their fades.
const zoomedCaptionedStoryboard = captionedStoryboard + `camera:
  zooms:
    - {to: 2, at: 100ms, over: 300ms}
`

// lostZoomStoryboard is renderStoryboard with beat 1 squeezed into 300ms — its 2s of take play
// above 6×, which the schedule warns about — and a camera position at beat 2 on a text no screen
// shows.
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
camera:
  positions:
    - {text: 'nowhere', at: {beat: 2}}
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
	rasterizer, err := newRasterizer(loaded)
	if err != nil {
		t.Fatalf("newRasterizer: %v", err)
	}
	resolver, err := target.New(loaded.Config.Areas)
	if err != nil {
		t.Fatalf("target.New: %v", err)
	}
	captionFaces, err := fonts.LoadCaption(loaded.Config.CaptionFont.Regular, loaded.Config.CaptionFont.Bold)
	if err != nil {
		t.Fatalf("fonts.LoadCaption: %v", err)
	}
	compositor, err := newCompositor(loaded, recorded, clipSchedule, rasterizer, resolver, captionFaces)
	if err != nil {
		t.Fatalf("newCompositor: %v", err)
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

	built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: newLastRaster()})

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
	built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: newLastRaster()})
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
	built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: newLastRaster()})
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

	built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: newLastRaster()})

	if built != nil {
		t.Error("want no pipeline for a take of another frame")
	}
	want := takePath + " was recorded at 30×4 cells; the storyboard's frame is 20×4 — re-record it"
	if err == nil || err.Error() != want {
		t.Fatalf("want %q, got %v", want, err)
	}
}

func TestBuild_APositionTargetNotOnScreen_LeavesAPartialPipeline(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClipOf(t, lostZoomStoryboard)
	takePath, recorded := loadRenderTake(t, 20)

	built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: newLastRaster()})

	if !errors.Is(err, target.ErrNotOnScreen) || !strings.Contains(err.Error(), "camera.positions[0]: ") {
		t.Fatalf("want camera.positions[0]'s target not on screen, got %v", err)
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
	if unit := built.Unit(); unit != 0 {
		t.Errorf("Unit: want 0 on a partial pipeline, got %g", unit)
	}
	if boxes := built.CaptionBoxes(); boxes != nil {
		t.Errorf("CaptionBoxes: want none on a partial pipeline, got %v", boxes)
	}
}

// resizedStoryboard is renderStoryboard at another font size: another raster geometry.
const resizedStoryboard = `clip: rendered
ship: out/rendered.gif
frame: {cols: 20, rows: 4, padding: 4, font_size: 12, line_height: 1.2, scale: 2, width: 160, fps: 10, max_colors: 16}
beats:
  - id: 1
    title: typing
    duration: 1s
  - id: 2
    title: hold
    duration: 1500ms
`

// rewidthedStoryboard is renderStoryboard shipped at another width, which only the compositor
// applies.
const rewidthedStoryboard = `clip: rendered
ship: out/rendered.gif
frame: {cols: 20, rows: 4, padding: 4, font_size: 10, line_height: 1.2, scale: 2, width: 120, fps: 10, max_colors: 16}
beats:
  - id: 1
    title: typing
    duration: 1s
  - id: 2
    title: hold
    duration: 1500ms
`

func TestBuild_Reuse_SharesTheRastersOnlyWhileTheyHold(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		storyboard   string
		isOtherTake  bool
		reconfigure  func(t *testing.T, project *config.Config)
		wantsRasters bool
	}{
		{name: "the same clip", storyboard: renderStoryboard, wantsRasters: true},
		{name: "another width", storyboard: rewidthedStoryboard, wantsRasters: true},
		{name: "another raster geometry", storyboard: resizedStoryboard},
		{name: "another take", storyboard: renderStoryboard, isOtherTake: true},
		{
			name:       "another fonts dir",
			storyboard: renderStoryboard,
			reconfigure: func(t *testing.T, project *config.Config) {
				project.Fonts = copyFontFiles(t, terminalFontFiles...)
			},
		},
		{
			name:       "another foreground colour",
			storyboard: renderStoryboard,
			reconfigure: func(_ *testing.T, project *config.Config) {
				project.Theme.FG = "#123456"
			},
		},
		{
			name:       "another ANSI colour",
			storyboard: renderStoryboard,
			reconfigure: func(_ *testing.T, project *config.Config) {
				project.Theme.ANSI[1] = "#123456"
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			takePath, recorded := loadRenderTake(t, 20)
			previous, err := Build(loadRenderClip(t), takePath, recorded, BuildOptions{Cache: newLastRaster()})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if tc.isOtherTake {
				takePath, recorded = loadRenderTake(t, 20)
			}
			cache := newLastRaster()
			loaded := loadRenderClipOf(t, tc.storyboard)
			if tc.reconfigure != nil {
				tc.reconfigure(t, loaded.Config)
			}

			built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: cache, Reuse: previous})

			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			isSharingRasters := built.rasterizer == previous.rasterizer && built.cache == previous.cache
			if isSharingRasters != tc.wantsRasters {
				t.Errorf("shares the rasterizer and its cache: %v, want %v", isSharingRasters, tc.wantsRasters)
			}
			if !tc.wantsRasters && (built.rasterizer == previous.rasterizer || built.cache != RasterCache(cache)) {
				t.Error("want a rasterizer of its own over the cache it was given")
			}
			if built.captionFaces.Regular.Font != previous.captionFaces.Regular.Font {
				t.Error("want the previous pipeline's caption faces")
			}
		})
	}
}

// TestBuild_TenReusedBuilds_LoadCaptionFontsOnce counts the process-wide caption loads, so it
// runs before the parallel tests are let go.
func TestBuild_TenReusedBuilds_LoadCaptionFontsOnce(t *testing.T) {
	const builds = 10
	loaded := loadRenderClipOf(t, captionedStoryboard)
	takePath, recorded := loadRenderTake(t, 20)
	before := captionLoads.Load()

	var previous *Pipeline
	for range builds {
		built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: newLastRaster(), Reuse: previous})
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		previous = built
	}

	if loads := captionLoads.Load() - before; loads != 1 {
		t.Errorf("%d builds loaded the caption fonts %d times, want once", builds, loads)
	}
}

// TestBuild_AnotherCaptionFont_LoadsTheCaptionFacesAfresh counts the process-wide caption loads,
// so it runs before the parallel tests are let go.
func TestBuild_AnotherCaptionFont_LoadsTheCaptionFacesAfresh(t *testing.T) {
	takePath, recorded := loadRenderTake(t, 20)
	previous, err := Build(loadRenderClipOf(t, captionedStoryboard), takePath, recorded, BuildOptions{Cache: newLastRaster()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	loaded := loadRenderClipOf(t, captionedStoryboard)
	captionDir := copyFontFiles(t, "SourceSans3-Regular.ttf", "SourceSans3-Bold.ttf")
	loaded.Config.CaptionFont = config.CaptionFont{
		Regular: filepath.Join(captionDir, "SourceSans3-Regular.ttf"),
		Bold:    filepath.Join(captionDir, "SourceSans3-Bold.ttf"),
	}
	before := captionLoads.Load()

	built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: newLastRaster(), Reuse: previous})

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if loads := captionLoads.Load() - before; loads != 1 {
		t.Errorf("the build loaded the caption fonts %d times, want once", loads)
	}
	if built.captionFaces.Regular.Font == previous.captionFaces.Regular.Font {
		t.Error("want caption faces of its own, not the previous pipeline's")
	}
}

func TestRun_ScheduleWarnings_PrintBeforeAComposeError(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClipOf(t, lostZoomStoryboard)
	options := Options{Take: saveRenderTake(t, t.TempDir(), 20), DryRun: true}
	var stderr bytes.Buffer

	err := Run(context.Background(), loaded, options, &bytes.Buffer{}, &stderr)

	if !errors.Is(err, target.ErrNotOnScreen) {
		t.Fatalf("want the position's target not on screen, got %v", err)
	}
	if !strings.HasPrefix(stderr.String(), "warning: beat 1 plays at ") {
		t.Errorf("want beat 1's speed warning on stderr, got %q", stderr.String())
	}
}

func TestPipeline_CaptionBoxes_AreWhereTheFrameShowsTheBoxes(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClipOf(t, captionedStoryboard)
	takePath, recorded := loadRenderTake(t, 20)
	built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: newLastRaster()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// Caption one shows in an opaque red box over [100ms, 800ms).
	const shown = 200 * time.Millisecond
	red := color.RGBA{R: 0xff, A: 0xff}

	boxes := built.CaptionBoxes()

	if len(boxes) != 2 {
		t.Fatalf("CaptionBoxes() = %v, want one box per caption", boxes)
	}
	if built.Unit() <= 0 {
		t.Errorf("Unit() = %g, want a positive unit", built.Unit())
	}
	frame, err := built.FrameAt(shown)
	if err != nil {
		t.Fatalf("FrameAt: %v", err)
	}
	box := boxes[0]
	for _, corner := range []image.Point{box.Min, box.Max.Sub(image.Pt(1, 1))} {
		if got := frame.RGBAAt(corner.X, corner.Y); got != red {
			t.Errorf("pixel %v of the frame at %s is %v, want the caption's red box", corner, shown, got)
		}
	}
}
