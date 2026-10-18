package tune

import (
	"bytes"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/render"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

// frameAt returns the session's frame at out, decoded.
func (f sessionFixture) frameAt(t *testing.T, out time.Duration) *image.RGBA {
	t.Helper()
	encoded, err := f.session.Frame(out)
	if err != nil {
		t.Fatalf("Frame(%s): %v", out, err)
	}
	decoded, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("decode the frame at %s: %v", out, err)
	}
	frame := image.NewRGBA(decoded.Bounds())
	draw.Draw(frame, frame.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	return frame
}

// renderedFrame builds the frame at out from the files on disk alone, through the pipeline
// `photon render` encodes from.
func (f sessionFixture) renderedFrame(t *testing.T, out time.Duration) *image.RGBA {
	t.Helper()
	loaded, err := clip.Load(f.boardPath, "", t.TempDir())
	if err != nil {
		t.Fatalf("clip.Load: %v", err)
	}
	recorded, err := take.Load(f.takePath)
	if err != nil {
		t.Fatalf("take.Load: %v", err)
	}
	built, err := render.Build(loaded, f.takePath, recorded, render.BuildOptions{Cache: newRasterCache(rasterCacheBytes)})
	if err != nil {
		t.Fatalf("render.Build: %v", err)
	}
	frame, err := built.FrameAt(out)
	if err != nil {
		t.Fatalf("FrameAt(%s): %v", out, err)
	}
	return frame
}

// assertSameFrame fails the test unless got and want hold the same pixels.
func assertSameFrame(t *testing.T, got, want *image.RGBA) {
	t.Helper()
	if got.Bounds() != want.Bounds() {
		t.Fatalf("frame bounds = %v, want %v", got.Bounds(), want.Bounds())
	}
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("frame pixels differ from the frame built from the files on disk")
	}
}

func TestSession_Frame_AfterEditMatchesTheRenderOfTheSavedFile(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		edit Edit
		// out is an output time whose frame the edit changes.
		out time.Duration
	}{
		{"duration", beatEdit(1, storyboard.FieldDuration, 2000), 500 * time.Millisecond},
		{"hold", beatEdit(1, storyboard.FieldHold, 600), 500 * time.Millisecond},
		{"cut", beatEdit(1, storyboard.FieldCut, true), 500 * time.Millisecond},
		{"zoom ramp", beatEdit(2, storyboard.FieldZoomIn, 200), 1200 * time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fixture := newSessionFixture(t)
			before := fixture.frameAt(t, tc.out)

			fixture.apply(t, tc.edit)
			if err := fixture.session.Save(); err != nil {
				t.Fatalf("Save: %v", err)
			}
			after := fixture.frameAt(t, tc.out)

			assertSameFrame(t, after, fixture.renderedFrame(t, tc.out))
			if bytes.Equal(after.Pix, before.Pix) {
				t.Error("the frame is unchanged by the edit")
			}
		})
	}
}

func TestSession_Frame_OutsideTheClip_IsRefused(t *testing.T) {
	t.Parallel()
	fixture := newSessionFixture(t)
	length := time.Duration(fixture.session.State().LengthMS) * time.Millisecond

	for _, out := range []time.Duration{-time.Millisecond, length} {
		_, err := fixture.session.Frame(out)

		if !errors.Is(err, render.ErrOutsideClip) {
			t.Errorf("Frame(%s) error = %v, want ErrOutsideClip", out, err)
		}
	}
}

func TestSession_Frame_RewrittenTake_ShowsTheNewTake(t *testing.T) {
	t.Parallel()
	fixture := newSessionFixture(t)
	out := 500 * time.Millisecond
	before := fixture.frameAt(t, out)

	recorded, err := take.Load(fixture.takePath)
	if err != nil {
		t.Fatalf("take.Load: %v", err)
	}
	recorded.Snapshots[1] = lineSnapshot(time.Second, "> tests are passing")
	if err := take.Save(fixture.takePath, recorded); err != nil {
		t.Fatalf("take.Save: %v", err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(fixture.takePath, later, later); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	if err := fixture.session.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	after := fixture.frameAt(t, out)

	assertSameFrame(t, after, fixture.renderedFrame(t, out))
	if bytes.Equal(after.Pix, before.Pix) {
		t.Error("the frame still shows the take as it was before the rewrite")
	}
}

func TestSession_FrameAndApply_RunInParallel(t *testing.T) {
	t.Parallel()
	fixture := newSessionFixture(t)
	const rounds = 10

	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		durations := []int{1200, 1000}
		for index := range rounds {
			edit := beatEdit(1, storyboard.FieldDuration, durations[index%2])
			if err := fixture.session.Apply(edit); err != nil {
				t.Errorf("Apply(%+v): %v", edit, err)
			}
		}
	}()
	go func() {
		defer wait.Done()
		for range rounds {
			if _, err := fixture.session.Frame(500 * time.Millisecond); err != nil {
				t.Errorf("Frame: %v", err)
			}
		}
	}()
	wait.Wait()
}

func TestRasterCache_PastCapacity_EvictsTheLeastRecentlyUsed(t *testing.T) {
	t.Parallel()
	// Each raster is 2×2 pixels, 16 bytes: the cache keeps two.
	raster := func() *image.RGBA { return image.NewRGBA(image.Rect(0, 0, 2, 2)) }
	cache := newRasterCache(32)
	cache.Put(0, raster())
	cache.Put(1, raster())
	cache.Get(0)

	cache.Put(2, raster())

	for index, want := range map[int]bool{0: true, 1: false, 2: true} {
		if _, ok := cache.Get(index); ok != want {
			t.Errorf("Get(%d) kept = %v, want %v", index, ok, want)
		}
	}
}

func TestRasterCache_RasterLargerThanCapacity_IsNotKept(t *testing.T) {
	t.Parallel()
	cache := newRasterCache(8)

	cache.Put(0, image.NewRGBA(image.Rect(0, 0, 2, 2)))

	if _, ok := cache.Get(0); ok {
		t.Error("a raster larger than the capacity was kept")
	}
}
