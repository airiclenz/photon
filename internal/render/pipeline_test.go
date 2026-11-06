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
	"github.com/airiclenz/photon/internal/config"
	"github.com/airiclenz/photon/internal/fonts"
	"github.com/airiclenz/photon/internal/pixels"
	"github.com/airiclenz/photon/internal/raster"
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
		composed = append(composed, compositor.Compose(frame.Out, draw(rasterizer, index)))
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
