package render

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/compose"
	"github.com/airiclenz/photon/internal/config"
	"github.com/airiclenz/photon/internal/fonts"
	"github.com/airiclenz/photon/internal/pixels"
	"github.com/airiclenz/photon/internal/raster"
	"github.com/airiclenz/photon/internal/schedule"
	"github.com/airiclenz/photon/internal/storyboard"
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
	return oracleFramesOf(t, loaded, recorded, func(rasterizer *raster.Rasterizer, index int) *image.RGBA {
		return rasterizer.Frame(recorded.Snapshots[index])
	})
}

// oracleFramesOf is [oracleFrames] with each frame's snapshot drawn afresh by draw, given the
// oracle's own rasterizer and the snapshot's index in recorded.
func oracleFramesOf(
	t *testing.T,
	loaded *clip.Clip,
	recorded *take.Take,
	draw func(rasterizer *raster.Rasterizer, index int) *image.RGBA,
) ([]schedule.Frame, []*image.RGBA) {
	t.Helper()
	rasterizer, err := newRasterizer(loaded)
	if err != nil {
		t.Fatalf("newRasterizer: %v", err)
	}
	layout := cellLayout(loaded.Storyboard.Frame, recorded, rasterizer)
	compositor, clipSchedule := oracleCompositor(t, loaded, recorded, layout)
	return composeOracle(loaded, recorded, clipSchedule, compositor, func(index int) *image.RGBA {
		return draw(rasterizer, index)
	})
}

// oracleCompositor is the oracles' compositor of loaded over recorded at layout and the schedule
// of its own it is planned on.
func oracleCompositor(
	t *testing.T,
	loaded *clip.Clip,
	recorded *take.Take,
	layout compose.Layout,
) (*compose.Compositor, *schedule.Schedule) {
	t.Helper()
	clipSchedule, err := schedule.New(loaded.Storyboard, recorded)
	if err != nil {
		t.Fatalf("schedule.New: %v", err)
	}
	resolver, err := target.New(loaded.Config.Areas)
	if err != nil {
		t.Fatalf("target.New: %v", err)
	}
	captionFaces, err := fonts.LoadCaption(loaded.Config.CaptionFont.Regular, loaded.Config.CaptionFont.Bold)
	if err != nil {
		t.Fatalf("fonts.LoadCaption: %v", err)
	}
	background := loaded.Config.Theme.BG.RGBA()
	compositor, err := compose.New(
		loaded.Storyboard, recorded, clipSchedule, resolver, layout, background, captionFaces)
	if err != nil {
		t.Fatalf("compose.New: %v", err)
	}
	return compositor, clipSchedule
}

// composeOracle composes every frame clipSchedule schedules for loaded with compositor, each
// frame's snapshot drawn afresh by draw from its index in recorded.
func composeOracle(
	loaded *clip.Clip,
	recorded *take.Take,
	clipSchedule *schedule.Schedule,
	compositor *compose.Compositor,
	draw func(index int) *image.RGBA,
) ([]schedule.Frame, []*image.RGBA) {
	frames := clipSchedule.Frames(loaded.Storyboard.Frame.FPS)
	composed := make([]*image.RGBA, 0, len(frames))
	for _, frame := range frames {
		index := recorded.ScreenAt(frame.Source)
		composed = append(composed, compositor.Compose(frame.Out, draw(index)))
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
	if !errors.Is(err, ErrTakeSize) {
		t.Errorf("want the refusal to wrap ErrTakeSize, got %v", err)
	}
}

func TestCheckFrame_ATakeOfTheFramesSize_Passes(t *testing.T) {
	t.Parallel()
	_, imageTake, _ := loadImageTake(t)
	terminalFrame := loadRenderClip(t).Storyboard.Frame
	tests := []struct {
		name     string
		recorded *take.Take
		frame    storyboard.Frame
	}{
		{name: "a terminal take", recorded: terminalTake(20, 4), frame: terminalFrame},
		{name: "an image take of a terminal frame", recorded: imageTake, frame: terminalFrame},
		{name: "a page take", recorded: pageTake(160, 100, 2), frame: viewportFrame(160, 100, 2)},
		{
			name: "a page take at the default device scale", recorded: pageTake(160, 100, 1),
			frame: viewportFrame(160, 100, 0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := CheckFrame("clip.take", tt.recorded, tt.frame)

			if err != nil {
				t.Errorf("CheckFrame: %v, want nil", err)
			}
		})
	}
}

func TestCheckFrame_ATakeOfAnotherSize_IsRefusedNamingTheKeys(t *testing.T) {
	t.Parallel()
	terminalFrame := loadRenderClip(t).Storyboard.Frame
	tests := []struct {
		name     string
		recorded *take.Take
		frame    storyboard.Frame
		want     string
	}{
		{
			name: "a terminal take of other cols", recorded: terminalTake(30, 4), frame: terminalFrame,
			want: "clip.take was recorded at 30×4 cells; the storyboard's frame is 20×4 — re-record it",
		},
		{
			name: "a page take of another viewport", recorded: pageTake(120, 100, 2),
			frame: viewportFrame(160, 100, 2),
			want: "clip.take was recorded at a 120×100 viewport at device scale 2; the storyboard's " +
				"frame.viewport is 160×100 at frame.device_scale 2 — re-record it",
		},
		{
			name: "a page take at another device scale", recorded: pageTake(160, 100, 1.5),
			frame: viewportFrame(160, 100, 2),
			want: "clip.take was recorded at a 160×100 viewport at device scale 1.5; the " +
				"storyboard's frame.viewport is 160×100 at frame.device_scale 2 — re-record it",
		},
		{
			name: "a page take under a frame without a viewport", recorded: pageTake(20, 4, 1),
			frame: terminalFrame,
			want: "clip.take was recorded at a 20×4 viewport at device scale 1; the storyboard's " +
				"frame sets no frame.viewport — re-record it",
		},
		{
			name: "a terminal take under a frame with a viewport", recorded: terminalTake(160, 100),
			frame: viewportFrame(160, 100, 1),
			want: "clip.take was recorded at 160×100 terminal cells; the storyboard's " +
				"frame.viewport is 160×100 at frame.device_scale 1 — re-record it",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := CheckFrame("clip.take", tt.recorded, tt.frame)

			if err == nil || err.Error() != tt.want {
				t.Fatalf("CheckFrame: want %q, got %v", tt.want, err)
			}
			if !errors.Is(err, ErrTakeSize) {
				t.Errorf("want the refusal to wrap ErrTakeSize, got %v", err)
			}
		})
	}
}

// terminalTake is a take recorded in a cols by rows terminal: no cell size.
func terminalTake(cols, rows int) *take.Take {
	return &take.Take{Cols: cols, Rows: rows, FPS: 10}
}

// pageTake is a take recorded from a cols by rows CSS-pixel viewport at deviceScale device
// pixels per CSS pixel: one cell to a CSS pixel.
func pageTake(cols, rows int, deviceScale float64) *take.Take {
	return &take.Take{
		Cols: cols, Rows: rows, FPS: 10,
		Cell: take.CellSize{W: deviceScale, H: deviceScale},
	}
}

// viewportFrame is a frame filming a width by height CSS-pixel page at deviceScale, zero for
// unset.
func viewportFrame(width, height int, deviceScale float64) storyboard.Frame {
	return storyboard.Frame{
		Viewport:    storyboard.Viewport{Width: width, Height: height},
		DeviceScale: deviceScale,
		Scale:       2, Width: width, FPS: 10, MaxColors: 16,
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
	if _, fullErr := built.FullFrameAt(0); !errors.Is(fullErr, err) {
		t.Errorf("FullFrameAt: want the build's error, got %v", fullErr)
	}
	if _, _, ok := built.FullViewCell(image.Point{}); ok {
		t.Error("FullViewCell: want no cell on a partial pipeline")
	}
	if rect := built.FullViewRect(take.Box{W: 1, H: 1}); !rect.Empty() {
		t.Errorf("FullViewRect: want the empty rectangle on a partial pipeline, got %v", rect)
	}
	if _, ok := built.ViewAt(0); ok {
		t.Error("ViewAt: want no view on a partial pipeline")
	}
	if _, _, ok := built.PositionCell(0); ok {
		t.Error("PositionCell: want no cell on a partial pipeline")
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

// aimedZoomedStoryboard is zoomedCaptionedStoryboard with the camera aimed at cell (15, 2) from
// the start, so it is pushed in 2× on that cell from 400ms.
const aimedZoomedStoryboard = zoomedCaptionedStoryboard + `  positions:
    - {cell: {x: 15, y: 2}, at: 0s}
`

func TestPipeline_FullFrameAt_IsTheFrameComposedAtAFactorOfOne(t *testing.T) {
	t.Parallel()
	takePath, recorded := loadRenderTake(t, 20)
	zoomed, err := Build(loadRenderClipOf(t, aimedZoomedStoryboard), takePath, recorded, BuildOptions{Cache: newLastRaster()})
	if err != nil {
		t.Fatalf("Build zoomed: %v", err)
	}
	unzoomed, err := Build(loadRenderClipOf(t, captionedStoryboard), takePath, recorded, BuildOptions{Cache: newLastRaster()})
	if err != nil {
		t.Fatalf("Build unzoomed: %v", err)
	}
	// At 500ms the zoomed clip is pushed in 2× and caption one shows.
	const at = 500 * time.Millisecond

	got, err := zoomed.FullFrameAt(at)

	if err != nil {
		t.Fatalf("FullFrameAt: %v", err)
	}
	want, err := unzoomed.FrameAt(at)
	if err != nil {
		t.Fatalf("FrameAt unzoomed: %v", err)
	}
	if got.Rect != want.Rect || !bytes.Equal(got.Pix, want.Pix) {
		t.Error("FullFrameAt: want the unzoomed clip's frame, got different pixels")
	}
	if _, err := zoomed.FullFrameAt(wantLength); !errors.Is(err, ErrOutsideClip) {
		t.Errorf("FullFrameAt(length): want ErrOutsideClip, got %v", err)
	}
}

func TestPipeline_TheCameraView_PassesTheCompositorThrough(t *testing.T) {
	t.Parallel()
	takePath, recorded := loadRenderTake(t, 20)
	built, err := Build(loadRenderClipOf(t, aimedZoomedStoryboard), takePath, recorded, BuildOptions{Cache: newLastRaster()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	view, ok := built.ViewAt(500 * time.Millisecond)

	if !ok {
		t.Fatal("ViewAt: want a view")
	}
	if want := image.Pt(15, 2); view.Centre != want {
		t.Errorf("ViewAt.Centre: want the aimed cell %v, got %v", want, view.Centre)
	}
	if half := float64(built.Size().X) / 2; view.Crop.W < half-1e-9 || view.Crop.W > half+1e-9 {
		t.Errorf("ViewAt.Crop.W: want half the full view's %d pixels at 2×, got %g", built.Size().X, view.Crop.W)
	}
	if col, row, ok := built.PositionCell(0); !ok || col != 15 || row != 2 {
		t.Errorf("PositionCell(0): want (15, 2, true), got (%d, %d, %t)", col, row, ok)
	}
	// At 0s the camera is still at 1×, so the full view maps cells as the shipped frame does.
	box := take.Box{X: 3, Y: 1, W: 4, H: 2}
	if got, want := built.FullViewRect(box), built.Rect(0, box); got != want || got.Empty() {
		t.Errorf("FullViewRect(%+v): want the unzoomed %v, got %v", box, want, got)
	}
	pixel := image.Pt(40, 20)
	wantCol, wantRow, wantOK := built.Cell(0, pixel)
	if col, row, ok := built.FullViewCell(pixel); col != wantCol || row != wantRow || ok != wantOK {
		t.Errorf("FullViewCell(%v): want (%d, %d, %t), got (%d, %d, %t)", pixel, wantCol, wantRow, wantOK, col, row, ok)
	}
}

// imageFrameSize is the pixel size of the frames in imageTake's image layer.
var imageFrameSize = image.Pt(48, 24)

// imageFrame is an opaque frame of imageFrameSize filled with fill, with a block of mark at
// (x, 4) to (x+6, 10).
func imageFrame(fill, mark color.RGBA, x int) *image.RGBA {
	frame := image.NewRGBA(image.Rectangle{Max: imageFrameSize})
	for y := range imageFrameSize.Y {
		for column := range imageFrameSize.X {
			isMark := column >= x && column < x+6 && y >= 4 && y < 10
			if isMark {
				frame.SetRGBA(column, y, mark)
			} else {
				frame.SetRGBA(column, y, fill)
			}
		}
	}
	return frame
}

// saveImageTake writes into dir a take of the render tests' grid with an image layer and returns
// its path and the frames its layer was encoded from: a key frame at 0, a patch at 1s that moves
// the block, and a key frame at 3s, two seconds after the first. Beat 1 runs from 0 and beat 2
// from 2s.
func saveImageTake(t *testing.T, dir string) (string, []*image.RGBA) {
	t.Helper()
	ink := color.RGBA{R: 0xe0, G: 0x40, B: 0x20, A: 0xff}
	ats := []time.Duration{0, sec, 3 * sec}
	frames := []*image.RGBA{
		imageFrame(color.RGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xff}, ink, 2),
		imageFrame(color.RGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xff}, ink, 10),
		imageFrame(color.RGBA{R: 0x30, G: 0x30, B: 0x10, A: 0xff}, ink, 20),
	}
	recorded := &take.Take{
		Cols: 20, Rows: 4, FPS: 10,
		Events: []take.Event{
			{At: 0, Kind: take.EventBeatStart, Beat: 1},
			{At: 2 * sec, Kind: take.EventBeatStart, Beat: 2},
		},
	}
	var encoder pixels.Encoder
	for index, frame := range frames {
		entry, err := encoder.Encode(ats[index], frame)
		if err != nil {
			t.Fatalf("Encode the frame at %s: %v", ats[index], err)
		}
		recorded.Snapshots = append(recorded.Snapshots, take.Snapshot{At: ats[index], Image: entry})
	}
	if recorded.Snapshots[1].Image.Key {
		t.Fatal("the frame at 1s encoded as a key frame, not a patch")
	}
	path := filepath.Join(dir, "rendered.take")
	if err := take.Save(path, recorded); err != nil {
		t.Fatalf("take.Save: %v", err)
	}
	return path, frames
}

// loadImageTake writes saveImageTake's take and loads it back, returning its path and the frames
// its layer was encoded from too.
func loadImageTake(t *testing.T) (string, *take.Take, []*image.RGBA) {
	t.Helper()
	takePath, frames := saveImageTake(t, t.TempDir())
	recorded, err := take.Load(takePath)
	if err != nil {
		t.Fatalf("take.Load: %v", err)
	}
	return takePath, recorded, frames
}

// imageOracleFrames is [oracleFrames] for a take with an image layer: each frame's snapshot is
// the layer's frame there, decoded afresh.
func imageOracleFrames(t *testing.T, loaded *clip.Clip, recorded *take.Take) ([]schedule.Frame, []*image.RGBA) {
	t.Helper()
	return oracleFramesOf(t, loaded, recorded, func(_ *raster.Rasterizer, index int) *image.RGBA {
		frame, err := pixels.NewDecoder(recorded).Frame(index)
		if err != nil {
			t.Fatalf("Decoder.Frame(%d): %v", index, err)
		}
		return frame
	})
}

// keepNothing is a RasterCache that keeps nothing, so every frame draws its snapshot afresh.
type keepNothing struct{}

// Get reports that no raster is kept.
func (keepNothing) Get(int) (*image.RGBA, bool) { return nil, false }

// Put drops frame.
func (keepNothing) Put(int, *image.RGBA) {}

func TestPipeline_RasterAt_AnImageTake_IsTheEncodersInputFrame(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClip(t)
	takePath, recorded, frames := loadImageTake(t)
	cache := newCountingCache()
	built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: cache})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, index := range []int{2, 0, 1, 2} {
		got, err := built.rasterAt(index, built.rasterizer, built.cache)

		if err != nil {
			t.Fatalf("rasterAt(%d): %v", index, err)
		}
		if got.Rect != frames[index].Rect || !bytes.Equal(got.Pix, frames[index].Pix) {
			t.Errorf("rasterAt(%d) is not the frame the layer was encoded from", index)
		}
	}
	if cache.puts != len(frames) {
		t.Errorf("the cache got %d puts, want one per snapshot, %d", cache.puts, len(frames))
	}
	for index, kept := range cache.rasters {
		if !bytes.Equal(kept.Pix, frames[index].Pix) {
			t.Errorf("the cached frame of snapshot %d changed after it was kept", index)
		}
	}
}

func TestPipeline_FrameAt_AnImageTake_EqualsTheOracleComposingTheDecodedImage(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClipOf(t, zoomedCaptionedStoryboard)
	takePath, recorded, _ := loadImageTake(t)
	frames, want := imageOracleFrames(t, loaded, recorded)

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

func TestPipeline_FrameAt_AnImageTakeDrawnConcurrently_EqualsTheOracle(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClip(t)
	takePath, recorded, _ := loadImageTake(t)
	frames, want := imageOracleFrames(t, loaded, recorded)
	built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: keepNothing{}})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	const workers = 4
	errs := make(chan error, workers*len(frames))
	var wg sync.WaitGroup

	for worker := range workers {
		wg.Go(func() {
			for step := range frames {
				index := (step*(worker+1) + worker) % len(frames)
				got, err := built.FrameAt(frames[index].Out)
				if err != nil {
					errs <- err
					continue
				}
				if !bytes.Equal(got.Pix, want[index].Pix) {
					errs <- errors.New("FrameAt(" + frames[index].Out.String() + ") differs from the oracle's frame")
				}
			}
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

// pageCameraStoryboard is pageCaptionedStoryboard with a camera that zooms to 2× over beat 1
// and aims at the CSS pixel (120, 60) from beat 2, so the url parity check covers the camera, the
// zoom and a caption.
const pageCameraStoryboard = pageCaptionedStoryboard + `camera:
  positions:
    - {cell: {x: 120, y: 60}, at: {beat: 2}}
  zooms:
    - {to: 2, at: 100ms, over: 300ms}
`

// pageFrameSize is the pixel size of the frames in savePageTake's image layer: pageStoryboard's
// 160×100 viewport at device scale 2.
var pageFrameSize = image.Pt(320, 200)

// pageFrame is an opaque frame of pageFrameSize filled with fill, with a block of mark at
// (x, 40) to (x+60, 100).
func pageFrame(fill, mark color.RGBA, x int) *image.RGBA {
	frame := image.NewRGBA(image.Rectangle{Max: pageFrameSize})
	for y := range pageFrameSize.Y {
		for column := range pageFrameSize.X {
			isMark := column >= x && column < x+60 && y >= 40 && y < 100
			if isMark {
				frame.SetRGBA(column, y, mark)
			} else {
				frame.SetRGBA(column, y, fill)
			}
		}
	}
	return frame
}

// savePageTake writes into dir a take recorded from pageStoryboard's page and returns its path:
// a 160×100 viewport at device scale 2 with an image layer of a key frame at 0, a patch at 1s
// that moves the block, and a key frame at 3s. Beat 1 runs from 0 and beat 2 from 2s.
func savePageTake(t *testing.T, dir string) string {
	t.Helper()
	ink := color.RGBA{R: 0xe0, G: 0x40, B: 0x20, A: 0xff}
	ats := []time.Duration{0, sec, 3 * sec}
	frames := []*image.RGBA{
		pageFrame(color.RGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xff}, ink, 20),
		pageFrame(color.RGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xff}, ink, 120),
		pageFrame(color.RGBA{R: 0x30, G: 0x30, B: 0x10, A: 0xff}, ink, 220),
	}
	recorded := &take.Take{
		Cols: 160, Rows: 100, FPS: 10, Cell: take.CellSize{W: 2, H: 2},
		Events: []take.Event{
			{At: 0, Kind: take.EventBeatStart, Beat: 1},
			{At: 2 * sec, Kind: take.EventBeatStart, Beat: 2},
		},
	}
	var encoder pixels.Encoder
	for index, frame := range frames {
		entry, err := encoder.Encode(ats[index], frame)
		if err != nil {
			t.Fatalf("Encode the frame at %s: %v", ats[index], err)
		}
		recorded.Snapshots = append(recorded.Snapshots, take.Snapshot{At: ats[index], Image: entry})
	}
	if recorded.Snapshots[1].Image.Key {
		t.Fatal("the frame at 1s encoded as a key frame, not a patch")
	}
	path := filepath.Join(dir, "rendered.take")
	if err := take.Save(path, recorded); err != nil {
		t.Fatalf("take.Save: %v", err)
	}
	return path
}

// loadPageTake writes savePageTake's take and loads it back, returning its path too.
func loadPageTake(t *testing.T) (string, *take.Take) {
	t.Helper()
	takePath := savePageTake(t, t.TempDir())
	recorded, err := take.Load(takePath)
	if err != nil {
		t.Fatalf("take.Load: %v", err)
	}
	return takePath, recorded
}

// pageOracleFrames builds every scheduled frame of a url-launch clip over recorded without the
// pipeline and without a rasterizer: a compositor of its own over the frame's pageLayout whose
// unit is pageOutput's, each frame's snapshot the image layer's frame there, decoded afresh.
func pageOracleFrames(t *testing.T, loaded *clip.Clip, recorded *take.Take) ([]schedule.Frame, []*image.RGBA) {
	t.Helper()
	frame := loaded.Storyboard.Frame
	layout := pageLayout(frame)
	layout.DeviceScale = frame.Scaling()
	compositor, clipSchedule := oracleCompositor(t, loaded, recorded, layout)
	size, unit, err := pageOutput(frame)
	if err != nil {
		t.Fatalf("pageOutput: %v", err)
	}
	if compositor.Size() != size || compositor.Unit() != unit {
		t.Fatalf("the oracle composes at %v, unit %g; pageOutput is %v, unit %g",
			compositor.Size(), compositor.Unit(), size, unit)
	}
	return composeOracle(loaded, recorded, clipSchedule, compositor, func(index int) *image.RGBA {
		decoded, err := pixels.NewDecoder(recorded).Frame(index)
		if err != nil {
			t.Fatalf("Decoder.Frame(%d): %v", index, err)
		}
		return decoded
	})
}

func TestPipeline_FrameAt_AURLClip_EqualsThePageOracle(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClipWith(t, pageConfig, pageCameraStoryboard)
	takePath, recorded := loadPageTake(t)
	frames, want := pageOracleFrames(t, loaded, recorded)

	built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: newLastRaster()})

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(frames) == 0 {
		t.Fatal("the oracle scheduled no frames")
	}
	size, unit, _ := pageOutput(loaded.Storyboard.Frame)
	if built.Size() != size || built.Unit() != unit {
		t.Errorf("the pipeline ships %v at unit %g, want pageOutput's %v at unit %g",
			built.Size(), built.Unit(), size, unit)
	}
	for index, frame := range frames {
		got, err := built.FrameAt(frame.Out)
		if err != nil {
			t.Fatalf("FrameAt(%s): %v", frame.Out, err)
		}
		if got.Rect != want[index].Rect || !bytes.Equal(got.Pix, want[index].Pix) {
			t.Errorf("FrameAt(%s) differs from the page oracle's frame", frame.Out)
		}
	}
}

func TestPipeline_DraftAt_AURLClip_DrawsThroughTheDraftCache(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClipWith(t, pageConfig, pageCameraStoryboard)
	takePath, recorded := loadPageTake(t)
	exact, draft := newCountingCache(), newCountingCache()
	built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: exact, DraftCache: draft})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	const out = 300 * time.Millisecond
	decoded, err := pixels.NewDecoder(recorded).Frame(recorded.ScreenAt(600 * time.Millisecond))
	if err != nil {
		t.Fatalf("Decoder.Frame: %v", err)
	}

	got, err := built.DraftAt(out)
	if err != nil {
		t.Fatalf("DraftAt(%s): %v", out, err)
	}
	full, err := built.FullDraftAt(out)

	if err != nil {
		t.Fatalf("FullDraftAt(%s): %v", out, err)
	}
	if draft.puts != 1 || exact.puts != 0 {
		t.Errorf("puts: draft cache %d, exact cache %d; want the one snapshot in the draft cache only",
			draft.puts, exact.puts)
	}
	if want := built.compositor.ComposeDraft(out, decoded); !bytes.Equal(got.Pix, want.Pix) {
		t.Errorf("DraftAt(%s) is not the draft composition of the image layer's frame", out)
	}
	if want := built.compositor.ComposeFullDraft(out, decoded); !bytes.Equal(full.Pix, want.Pix) {
		t.Errorf("FullDraftAt(%s) is not the full draft composition of the image layer's frame", out)
	}
}

// TestBuild_AURLClip_LoadsNoTerminalFont counts the process-wide terminal-font loads, so it runs
// before the parallel tests are let go.
func TestBuild_AURLClip_LoadsNoTerminalFont(t *testing.T) {
	loaded := loadRenderClipWith(t, pageConfig, pageCameraStoryboard)
	takePath, recorded := loadPageTake(t)
	before := terminalLoads.Load()

	built, err := Build(loaded, takePath, recorded, BuildOptions{
		Cache: newCountingCache(), DraftCache: newCountingCache(),
	})

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if loads := terminalLoads.Load() - before; loads != 0 {
		t.Errorf("building a url clip loaded the terminal fonts %d times, want none", loads)
	}
	if built.rasterizer != nil || built.draftRasterizer != nil {
		t.Error("building a url clip built a rasterizer")
	}
}

func TestBuild_AURLClipReusingAURLPipeline_TakesOverItsImageLayerAndCaptionFaces(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClipWith(t, pageConfig, pageCameraStoryboard)
	takePath, recorded := loadPageTake(t)
	previous, err := Build(loaded, takePath, recorded, BuildOptions{Cache: newCountingCache()})
	if err != nil {
		t.Fatalf("Build the previous pipeline: %v", err)
	}
	frames, want := pageOracleFrames(t, loaded, recorded)

	built, err := Build(loaded, takePath, recorded, BuildOptions{
		Cache: newCountingCache(), DraftCache: newCountingCache(), Reuse: previous,
	})

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if built.images != previous.images || built.rasterizer != nil {
		t.Error("the rebuild did not take over the image layer alone")
	}
	got, err := built.FrameAt(frames[len(frames)-1].Out)
	if err != nil {
		t.Fatalf("FrameAt: %v", err)
	}
	if !bytes.Equal(got.Pix, want[len(want)-1].Pix) {
		t.Error("the rebuilt pipeline's last frame differs from the page oracle's")
	}
}

func TestBuild_AURLClipReusingAURLPipeline_TakesOverItsPageFrameCaches(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClipWith(t, pageConfig, pageCameraStoryboard)
	takePath, recorded := loadPageTake(t)
	previous, err := Build(loaded, takePath, recorded, BuildOptions{
		Cache: newCountingCache(), DraftCache: newCountingCache(),
	})
	if err != nil {
		t.Fatalf("Build the previous pipeline: %v", err)
	}
	frames, want := pageOracleFrames(t, loaded, recorded)
	last := frames[len(frames)-1].Out
	if _, err := previous.FrameAt(last); err != nil {
		t.Fatalf("FrameAt through the previous pipeline: %v", err)
	}
	if _, err := previous.DraftAt(last); err != nil {
		t.Fatalf("DraftAt through the previous pipeline: %v", err)
	}
	exact, draft := previous.cache.(*countingCache), previous.draftCache.(*countingCache)
	exactPuts, draftPuts := exact.puts, draft.puts
	givenExact, givenDraft := newCountingCache(), newCountingCache()

	built, err := Build(loaded, takePath, recorded, BuildOptions{
		Cache: givenExact, DraftCache: givenDraft, Reuse: previous,
	})

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if built.cache != previous.cache || built.draftCache != previous.draftCache {
		t.Fatal("the rebuild did not take over the previous pipeline's exact and draft caches")
	}
	got, err := built.FrameAt(last)
	if err != nil {
		t.Fatalf("FrameAt: %v", err)
	}
	if _, err := built.DraftAt(last); err != nil {
		t.Fatalf("DraftAt: %v", err)
	}
	if exact.puts != exactPuts || draft.puts != draftPuts || givenExact.puts+givenDraft.puts != 0 {
		t.Errorf("redrawing a drawn frame put %d exact and %d draft frames, "+
			"the given caches %d and %d; want none",
			exact.puts-exactPuts, draft.puts-draftPuts, givenExact.puts, givenDraft.puts)
	}
	if !bytes.Equal(got.Pix, want[len(want)-1].Pix) {
		t.Error("the rebuilt pipeline's last frame differs from the page oracle's")
	}
}

func TestBuild_AURLClipReusingAPipelineWithoutADraftCache_KeepsTheGivenDraftCache(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClipWith(t, pageConfig, pageCameraStoryboard)
	takePath, recorded := loadPageTake(t)
	previous, err := Build(loaded, takePath, recorded, BuildOptions{Cache: newCountingCache()})
	if err != nil {
		t.Fatalf("Build the previous pipeline: %v", err)
	}
	givenDraft := newCountingCache()

	built, err := Build(loaded, takePath, recorded, BuildOptions{
		Cache: newCountingCache(), DraftCache: givenDraft, Reuse: previous,
	})

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if built.cache != previous.cache {
		t.Error("the rebuild did not take over the previous pipeline's exact cache")
	}
	if built.draftCache != RasterCache(givenDraft) {
		t.Error("the rebuild did not keep the draft cache it was given")
	}
}

func TestBuild_AURLClipReusingAPipelineThatDoesNotHold_KeepsTheGivenCaches(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClipWith(t, pageConfig, pageCameraStoryboard)
	takePath, recorded := loadPageTake(t)
	_, otherTake := loadPageTake(t)
	otherPrevious, err := Build(loaded, takePath, otherTake, BuildOptions{
		Cache: newCountingCache(), DraftCache: newCountingCache(),
	})
	if err != nil {
		t.Fatalf("Build the pipeline over another take: %v", err)
	}
	lost := loadRenderClipWith(t, pageConfig, pageStoryboard+`camera:
  positions:
    - {text: 'nowhere', at: {beat: 2}}
`)
	partial, err := Build(lost, takePath, recorded, BuildOptions{
		Cache: newCountingCache(), DraftCache: newCountingCache(),
	})
	if !errors.Is(err, target.ErrNotOnScreen) || partial == nil {
		t.Fatalf("Build the partial pipeline: %v, want a target not on screen and a partial one", err)
	}
	frames, want := pageOracleFrames(t, loaded, recorded)

	previousOf := map[string]*Pipeline{"another take": otherPrevious, "partial": partial}
	for name, previous := range previousOf {
		givenExact, givenDraft := newCountingCache(), newCountingCache()
		built, err := Build(loaded, takePath, recorded, BuildOptions{
			Cache: givenExact, DraftCache: givenDraft, Reuse: previous,
		})
		if err != nil {
			t.Fatalf("%s: Build: %v", name, err)
		}
		if built.cache != RasterCache(givenExact) || built.draftCache != RasterCache(givenDraft) {
			t.Errorf("%s: the rebuild did not keep the caches it was given", name)
		}
		got, err := built.FrameAt(frames[len(frames)-1].Out)
		if err != nil {
			t.Fatalf("%s: FrameAt: %v", name, err)
		}
		if !bytes.Equal(got.Pix, want[len(want)-1].Pix) {
			t.Errorf("%s: the rebuilt pipeline's last frame differs from the page oracle's", name)
		}
	}
}

func TestBuild_AURLClipWithoutAnImageLayer_IsRefused(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClipWith(t, pageConfig, pageStoryboard)
	recorded := &take.Take{
		Cols: 160, Rows: 100, FPS: 10, Cell: take.CellSize{W: 2, H: 2},
		Snapshots: []take.Snapshot{textSnapshot(0, "page")},
		Events: []take.Event{
			{At: 0, Kind: take.EventBeatStart, Beat: 1},
			{At: 2 * sec, Kind: take.EventBeatStart, Beat: 2},
		},
	}

	built, err := Build(loaded, "rendered.take", recorded, BuildOptions{Cache: newCountingCache()})

	if !errors.Is(err, errNoImageLayer) {
		t.Fatalf("Build: %v, want errNoImageLayer", err)
	}
	if _, err := built.FrameAt(0); !errors.Is(err, errNoImageLayer) {
		t.Errorf("FrameAt on the partial pipeline: %v, want errNoImageLayer", err)
	}
}
