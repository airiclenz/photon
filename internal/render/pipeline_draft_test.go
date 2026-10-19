package render

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/raster"
)

// countingCache is a RasterCache that keeps every raster it is given and counts the puts, so a
// test can tell a snapshot drawn afresh from one taken from the cache.
type countingCache struct {
	rasters map[int]*image.RGBA
	puts    int
}

// newCountingCache returns a countingCache holding nothing.
func newCountingCache() *countingCache { return &countingCache{rasters: map[int]*image.RGBA{}} }

// Get returns the raster kept for index.
func (c *countingCache) Get(index int) (*image.RGBA, bool) {
	raster, ok := c.rasters[index]
	return raster, ok
}

// Put keeps frame for index and counts the put.
func (c *countingCache) Put(index int, frame *image.RGBA) {
	c.rasters[index] = frame
	c.puts++
}

// rescaledStoryboard is renderStoryboard at frame.scale scale, all else alike.
func rescaledStoryboard(scale int) string {
	return strings.Replace(renderStoryboard, "scale: 2", fmt.Sprintf("scale: %d", scale), 1)
}

// drawEveryDraft draws the draft of every frame built schedules at loaded's fps.
func drawEveryDraft(t *testing.T, built *Pipeline, loaded *clip.Clip) {
	t.Helper()
	for _, frame := range built.Schedule().Frames(loaded.Storyboard.Frame.FPS) {
		if _, err := built.DraftAt(frame.Out); err != nil {
			t.Fatalf("DraftAt(%s): %v", frame.Out, err)
		}
	}
}

func TestPipeline_DraftAt_EveryScheduledFrame_IsOfTheFrameSize(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClipOf(t, zoomedCaptionedStoryboard)
	takePath, recorded := loadRenderTake(t, 20)
	built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: newLastRaster(), DraftCache: newLastRaster()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	frames := built.Schedule().Frames(loaded.Storyboard.Frame.FPS)
	if len(frames) == 0 {
		t.Fatal("the schedule holds no frames")
	}

	for _, frame := range frames {
		got, err := built.DraftAt(frame.Out)

		if err != nil {
			t.Fatalf("DraftAt(%s): %v", frame.Out, err)
		}
		if got.Rect.Size() != built.Size() {
			t.Errorf("DraftAt(%s) is %v, want the frame size %v", frame.Out, got.Rect.Size(), built.Size())
		}
	}
}

func TestPipeline_DraftAt_OutsideTheClip_IsRefused(t *testing.T) {
	t.Parallel()
	takePath, recorded := loadRenderTake(t, 20)
	built, err := Build(loadRenderClip(t), takePath, recorded, BuildOptions{Cache: newLastRaster(), DraftCache: newLastRaster()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, out := range []time.Duration{-time.Millisecond, built.Schedule().Length} {
		_, err := built.DraftAt(out)

		if !errors.Is(err, ErrOutsideClip) {
			t.Errorf("DraftAt(%s): want ErrOutsideClip, got %v", out, err)
		}
	}
}

func TestPipeline_DraftAt_WithoutADraftCache_IsTheExactFrame(t *testing.T) {
	t.Parallel()
	loaded := loadRenderClipOf(t, zoomedCaptionedStoryboard)
	takePath, recorded := loadRenderTake(t, 20)
	built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: newLastRaster()})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, frame := range built.Schedule().Frames(loaded.Storyboard.Frame.FPS) {
		draft, draftErr := built.DraftAt(frame.Out)
		exact, exactErr := built.FrameAt(frame.Out)

		if draftErr != nil || exactErr != nil {
			t.Fatalf("at %s: DraftAt: %v, FrameAt: %v", frame.Out, draftErr, exactErr)
		}
		if draft.Rect != exact.Rect || !bytes.Equal(draft.Pix, exact.Pix) {
			t.Errorf("DraftAt(%s) differs from FrameAt's frame", frame.Out)
		}
	}
}

// TestBuild_BothRasterizers_LoadTheTerminalFontsOnce counts the process-wide terminal-font loads,
// so it runs before the parallel tests are let go.
func TestBuild_BothRasterizers_LoadTheTerminalFontsOnce(t *testing.T) {
	loaded := loadRenderClip(t)
	takePath, recorded := loadRenderTake(t, 20)
	before := terminalLoads.Load()

	built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: newLastRaster(), DraftCache: newLastRaster()})

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if built.draftRasterizer == nil || built.draftRasterizer == built.rasterizer {
		t.Fatal("want a draft rasterizer of its own beside the exact one at frame.scale 2")
	}
	if loads := terminalLoads.Load() - before; loads != 1 {
		t.Errorf("a build of both rasterizers loaded the terminal fonts %d times, want once", loads)
	}
}

func TestBuild_WithoutADraftCache_HoldsNoDraftRasterizer(t *testing.T) {
	t.Parallel()
	takePath, recorded := loadRenderTake(t, 20)

	built, err := Build(loadRenderClip(t), takePath, recorded, BuildOptions{Cache: newLastRaster()})

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if built.draftRasterizer != nil {
		t.Error("want no draft rasterizer without a draft cache")
	}
}

// benchmarkFrameStoryboard is renderStoryboard at the draft benchmark's frame: 135×44 cells at
// font size 30 and scale 2, shipped 1250 pixels wide.
var benchmarkFrameStoryboard = strings.Replace(renderStoryboard,
	"{cols: 20, rows: 4, padding: 4, font_size: 10, line_height: 1.2, scale: 2, width: 160,",
	"{cols: 135, rows: 44, padding: 4, font_size: 30, line_height: 1.2, scale: 2, width: 1250,", 1)

// narrowedStoryboard is renderStoryboard shipped 128 pixels wide: its draft geometry is
// rescaledStoryboard(1)'s raster geometry, while its own raster geometry is at scale 2.
var narrowedStoryboard = strings.Replace(renderStoryboard, "width: 160", "width: 128", 1)

func TestDraftGeometry_ScaledToTheFrameWidth_FitsIt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		storyboard  string
		wantAtLeast int
	}{
		{name: "the benchmark frame", storyboard: benchmarkFrameStoryboard, wantAtLeast: 1200},
		// renderStoryboard's raster scaled by width alone, rounded, is 170 pixels wide.
		{name: "an overshooting cell width", storyboard: renderStoryboard, wantAtLeast: 144},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			loaded := loadRenderClipOf(t, tc.storyboard)
			faces, err := loadTerminalFaces(loaded)
			if err != nil {
				t.Fatalf("loadTerminalFaces: %v", err)
			}
			exact, err := newRasterizer(loaded)
			if err != nil {
				t.Fatalf("newRasterizer: %v", err)
			}

			geometry, err := draftGeometry(loaded, faces, exact)

			if err != nil {
				t.Fatalf("draftGeometry: %v", err)
			}
			draft, err := raster.New(faces, loaded.Config.Theme, geometry)
			if err != nil {
				t.Fatalf("raster.New: %v", err)
			}
			width := loaded.Storyboard.Frame.Width
			if got := draft.Size().X; got > width || got < tc.wantAtLeast {
				t.Errorf("the draft raster is %d pixels wide, want %d to %d", got, tc.wantAtLeast, width)
			}
		})
	}
}

func TestBuild_DraftStage_IsTheExactStageOnlyWhenTheExactRasterFits(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		storyboard    string
		wantsExactOne bool
	}{
		{name: "scale 1 within the width", storyboard: rescaledStoryboard(1), wantsExactOne: true},
		{name: "scale 2 wider than the width", storyboard: renderStoryboard},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			takePath, recorded := loadRenderTake(t, 20)
			exact, draft := newCountingCache(), newCountingCache()

			built, err := Build(loadRenderClipOf(t, tc.storyboard), takePath, recorded, BuildOptions{Cache: exact, DraftCache: draft})

			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			isExactStage := built.draftRasterizer == built.rasterizer && built.draftCache == RasterCache(exact)
			isOwnStage := built.draftRasterizer != built.rasterizer && built.draftCache == RasterCache(draft)
			if tc.wantsExactOne && !isExactStage {
				t.Error("want the exact rasterizer and its cache as the draft stage")
			}
			if !tc.wantsExactOne && !isOwnStage {
				t.Error("want a draft rasterizer of its own over the draft cache it was given")
			}
		})
	}
}

func TestBuild_Reuse_KeepsTheDraftRastersWhileTheDraftGeometryHolds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		storyboard string
		wantsKept  bool
	}{
		{name: "the same clip", storyboard: renderStoryboard, wantsKept: true},
		// At scale 3 the exact raster is 384 pixels wide; scaled to 160 its draft geometry is
		// scale 2's.
		{name: "scale 2 to 3", storyboard: rescaledStoryboard(3), wantsKept: true},
		{name: "scale 2 to 1", storyboard: rescaledStoryboard(1)},
		{name: "another width", storyboard: rewidthedStoryboard},
		{name: "another font size", storyboard: resizedStoryboard},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			takePath, recorded := loadRenderTake(t, 20)
			first := newCountingCache()
			previousClip := loadRenderClip(t)
			previous, err := Build(previousClip, takePath, recorded, BuildOptions{Cache: newLastRaster(), DraftCache: first})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			drawEveryDraft(t, previous, previousClip)
			drawn := first.puts
			if drawn == 0 {
				t.Fatal("the first pipeline drew no draft raster")
			}
			loaded := loadRenderClipOf(t, tc.storyboard)
			built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: newLastRaster(), DraftCache: newCountingCache(), Reuse: previous})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}

			drawEveryDraft(t, built, loaded)

			isKept := built.draftRasterizer == previous.draftRasterizer && built.draftCache == RasterCache(first)
			if isKept != tc.wantsKept {
				t.Errorf("kept the draft rasterizer and its cache: %v, want %v", isKept, tc.wantsKept)
			}
			if tc.wantsKept && first.puts != drawn {
				t.Errorf("redrew %d kept draft rasters, want none", first.puts-drawn)
			}
		})
	}
}

func TestBuild_Reuse_ASharedDraftStage_CarriesOverOnlyWithItsExactStage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		storyboard string
		wantsKept  bool
	}{
		{name: "the same exact stage", storyboard: rescaledStoryboard(1), wantsKept: true},
		{name: "another exact geometry", storyboard: narrowedStoryboard},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			takePath, recorded := loadRenderTake(t, 20)
			exact := newCountingCache()
			previous, err := Build(loadRenderClipOf(t, rescaledStoryboard(1)), takePath, recorded, BuildOptions{Cache: exact, DraftCache: newCountingCache()})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if previous.draftRasterizer != previous.rasterizer {
				t.Fatal("want the previous pipeline's draft stage to be its exact stage")
			}
			draft := newCountingCache()

			built, err := Build(loadRenderClipOf(t, tc.storyboard), takePath, recorded, BuildOptions{Cache: newLastRaster(), DraftCache: draft, Reuse: previous})

			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if built.draftGeometry != previous.draftGeometry {
				t.Fatalf("draft geometry %+v, want the previous one %+v", built.draftGeometry, previous.draftGeometry)
			}
			isKept := built.draftRasterizer == previous.rasterizer && built.draftCache == RasterCache(exact)
			if isKept != tc.wantsKept {
				t.Errorf("kept the previous exact rasterizer and cache as the draft stage: %v, want %v", isKept, tc.wantsKept)
			}
			if !tc.wantsKept && built.draftCache != RasterCache(draft) {
				t.Error("want a draft rasterizer of its own over the draft cache it was given")
			}
		})
	}
}
