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

func TestBuild_Reuse_KeepsTheDraftRastersWhileTheDraftGeometryHolds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		storyboard   string
		wantsRedraws bool
	}{
		{name: "scale 2 to 3", storyboard: rescaledStoryboard(3)},
		{name: "scale 2 to 1", storyboard: rescaledStoryboard(1)},
		{name: "another font size", storyboard: resizedStoryboard, wantsRedraws: true},
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
			second := newCountingCache()
			loaded := loadRenderClipOf(t, tc.storyboard)
			built, err := Build(loaded, takePath, recorded, BuildOptions{Cache: newLastRaster(), DraftCache: second, Reuse: previous})
			if err != nil {
				t.Fatalf("Build: %v", err)
			}

			drawEveryDraft(t, built, loaded)

			hasRedrawn := first.puts != drawn || second.puts != 0
			if hasRedrawn != tc.wantsRedraws {
				t.Errorf("redrew draft rasters: %v (first cache %d → %d puts, second %d), want %v",
					hasRedrawn, drawn, first.puts, second.puts, tc.wantsRedraws)
			}
		})
	}
}

func TestBuild_Reuse_TheDraftStageNeverTakesTheExactRasters(t *testing.T) {
	t.Parallel()
	takePath, recorded := loadRenderTake(t, 20)
	exact := newCountingCache()
	previous, err := Build(loadRenderClipOf(t, rescaledStoryboard(1)), takePath, recorded, BuildOptions{Cache: exact})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	draft := newCountingCache()

	built, err := Build(loadRenderClipOf(t, rescaledStoryboard(1)), takePath, recorded, BuildOptions{Cache: newLastRaster(), DraftCache: draft, Reuse: previous})

	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if built.draftRasterizer == previous.rasterizer || built.draftCache != RasterCache(draft) {
		t.Error("want a draft rasterizer of its own over the draft cache it was given")
	}
}
