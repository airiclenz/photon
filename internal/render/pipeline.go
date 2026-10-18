package render

import (
	"errors"
	"fmt"
	"image"
	"slices"
	"time"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/compose"
	"github.com/airiclenz/photon/internal/fonts"
	"github.com/airiclenz/photon/internal/raster"
	"github.com/airiclenz/photon/internal/schedule"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
)

// ErrOutsideClip is a frame or a screen asked for at an output time outside [0, length) of the
// clip.
var ErrOutsideClip = errors.New("the time is outside the clip")

// RasterCache keeps rasterized snapshots by their index in the take, so a snapshot shown by
// several frames is rasterized once. A cache belongs to one pipeline's rasterizer and take. Get
// reports false for an index it does not hold; Put may keep or drop what it is given.
type RasterCache interface {
	Get(index int) (*image.RGBA, bool)
	Put(index int, frame *image.RGBA)
}

// Pipeline builds the shipped frames of one clip's take by output time: the schedule maps an
// output time onto the snapshot it shows, the rasterizer draws that snapshot at frame.scale and
// the compositor lays the camera, the cursor and the captions over it at frame.width. Every frame
// `photon render` encodes comes from [Pipeline.FrameAt].
//
// A pipeline [Build] could not finish holds its schedule and the error that stopped it: its
// frame methods return that error, [Pipeline.Cell] reports no cell, [Pipeline.Rect] the empty
// rectangle and [Pipeline.Size] the zero size. It is safe for concurrent use when its
// [RasterCache] is.
type Pipeline struct {
	// loaded is the clip the pipeline was built over: its frame and config decide what a later
	// [Build] may reuse.
	loaded       *clip.Clip
	recorded     *take.Take
	schedule     *schedule.Schedule
	rasterizer   *raster.Rasterizer
	compositor   *compose.Compositor
	cache        RasterCache
	captionFaces fonts.CaptionSet
	// err is the stage error that left the pipeline without a rasterizer or compositor.
	err error
}

// BuildOptions is what [Build] is given beside the clip and its take. Cache keeps the rasterized
// snapshots of a pipeline that builds its own rasterizer; it must not be nil. Reuse, when not
// nil, is an earlier pipeline whose parts the new one takes over where they come out the same:
// its rasterizer and its raster cache while the take, the frame's raster geometry, the config's
// terminal fonts and its theme match, and its caption faces while the config's caption fonts do.
// A reused cache is then shared by both pipelines, so it must be safe for both to use at once
// when they are.
type BuildOptions struct {
	Cache RasterCache
	Reuse *Pipeline
}

// Build checks that the take at takePath, recorded, was recorded at the storyboard's frame size,
// schedules it, and builds the rasterizer and the compositor over loaded's config: frames are
// rasterized at frame.scale — padding and font size multiplied through — so a zoom has pixels to
// draw on, and composed at frame.width, with zoom and pan targets resolved through the config's
// areas. Rasterized snapshots are kept in options.Cache, or in the reused pipeline's cache along
// with its rasterizer; the caption fonts are loaded only when options.Reuse holds none that match
// (see [BuildOptions]).
//
// It fails with a nil pipeline when the frame sizes differ or the take does not schedule. When a
// later stage fails — a font or an area that does not load, a target not on screen, a caption
// that does not lay out — it returns that error together with a partial pipeline whose
// [Pipeline.Schedule] is set. Errors are returned as their stage reports them, unwrapped.
func Build(
	loaded *clip.Clip,
	takePath string,
	recorded *take.Take,
	options BuildOptions,
) (*Pipeline, error) {
	if err := checkFrame(takePath, recorded, loaded.Storyboard.Frame); err != nil {
		return nil, err
	}
	clipSchedule, err := schedule.New(loaded.Storyboard, recorded)
	if err != nil {
		return nil, err
	}
	built := &Pipeline{loaded: loaded, recorded: recorded, schedule: clipSchedule, cache: options.Cache}
	if built.err = built.buildFrames(options.Reuse); built.err != nil {
		return built, built.err
	}
	return built, nil
}

// buildFrames builds the rasterizer, the caption faces, the target resolver and the compositor
// over the pipeline's clip, taking from previous what it can reuse, and keeps every stage or
// none.
func (p *Pipeline) buildFrames(previous *Pipeline) error {
	rasterizer, cache := p.rasterizer, p.cache
	if p.canReuseRasters(previous) {
		rasterizer, cache = previous.rasterizer, previous.cache
	} else {
		var err error
		if rasterizer, err = newRasterizer(p.loaded); err != nil {
			return err
		}
	}
	captionFaces, err := p.captionFacesFrom(previous)
	if err != nil {
		return err
	}
	resolver, err := target.New(p.loaded.Config.Areas)
	if err != nil {
		return err
	}
	compositor, err := newCompositor(p.loaded, p.recorded, p.schedule, rasterizer, resolver, captionFaces)
	if err != nil {
		return err
	}
	p.rasterizer, p.cache, p.captionFaces, p.compositor = rasterizer, cache, captionFaces, compositor
	return nil
}

// canReuseRasters says previous built in full over the same take with a rasterizer p's clip
// would build alike — the same raster geometry, terminal fonts and theme — so its rasterizer
// draws p's snapshots as p's own would and the rasters in its cache hold for p.
func (p *Pipeline) canReuseRasters(previous *Pipeline) bool {
	if previous == nil || previous.compositor == nil || previous.recorded != p.recorded {
		return false
	}
	was, is := previous.loaded, p.loaded
	return rasterGeometry(was.Storyboard.Frame) == rasterGeometry(is.Storyboard.Frame) &&
		was.Config.Fonts == is.Config.Fonts &&
		was.Config.Theme.FG == is.Config.Theme.FG &&
		was.Config.Theme.BG == is.Config.Theme.BG &&
		slices.Equal(was.Config.Theme.ANSI, is.Config.Theme.ANSI)
}

// captionFacesFrom returns previous's caption faces when it built in full over the same caption
// font files as p's config names, and loads them otherwise. The faces do not depend on the take
// or the frame, so a session's edits load them once.
func (p *Pipeline) captionFacesFrom(previous *Pipeline) (fonts.CaptionSet, error) {
	isReusable := previous != nil && previous.compositor != nil &&
		previous.loaded.Config.CaptionFont == p.loaded.Config.CaptionFont
	if isReusable {
		return previous.captionFaces, nil
	}
	return loadCaptionFaces(p.loaded)
}

// checkFrame refuses a take recorded at another size than the storyboard's frame: its cells are
// what the rasterizer lays out and its targets are resolved on.
func checkFrame(takePath string, recorded *take.Take, frame storyboard.Frame) error {
	if recorded.Cols == frame.Cols && recorded.Rows == frame.Rows {
		return nil
	}
	return fmt.Errorf(
		"%s was recorded at %d×%d cells; the storyboard's frame is %d×%d — re-record it",
		takePath, recorded.Cols, recorded.Rows, frame.Cols, frame.Rows,
	)
}

// Schedule is the output clock the pipeline was built on, set on a partial pipeline too.
func (p *Pipeline) Schedule() *schedule.Schedule { return p.schedule }

// FrameAt returns the shipped frame at output time out, before the GIF's palette quantization:
// the snapshot on screen then, rasterized or taken from the raster cache, composed at out. It
// fails with [ErrOutsideClip] when out is not in [0, length), when the take holds no snapshot to
// show, and with the build's error on a partial pipeline.
func (p *Pipeline) FrameAt(out time.Duration) (*image.RGBA, error) {
	index, err := p.screenIndexAt(out)
	if err != nil {
		return nil, err
	}
	source, ok := p.cache.Get(index)
	if !ok {
		source = p.rasterizer.Frame(p.recorded.Snapshots[index])
		p.cache.Put(index, source)
	}
	return p.compositor.Compose(out, source), nil
}

// ScreenAt returns the take's snapshot on screen at output time out. It fails as
// [Pipeline.FrameAt] does.
func (p *Pipeline) ScreenAt(out time.Duration) (take.Snapshot, error) {
	index, err := p.screenIndexAt(out)
	if err != nil {
		return take.Snapshot{}, err
	}
	return p.recorded.Snapshots[index], nil
}

// screenIndexAt returns the index in the take of the snapshot on screen at output time out.
func (p *Pipeline) screenIndexAt(out time.Duration) (int, error) {
	if p.err != nil {
		return -1, p.err
	}
	source, _, ok := p.schedule.Source(out)
	if !ok {
		return -1, fmt.Errorf("%w: %s is not in [0, %s)", ErrOutsideClip, out, p.schedule.Length)
	}
	index := p.recorded.ScreenAt(source)
	if index < 0 {
		return -1, fmt.Errorf("the take holds no snapshot to show at %s", source)
	}
	return index, nil
}

// Cell returns the terminal cell under pixel point of the shipped frame at output time out, as
// [compose.Compositor.Cell] maps it through the camera at out. ok is false on a partial pipeline.
func (p *Pipeline) Cell(out time.Duration, point image.Point) (col, row int, ok bool) {
	if p.compositor == nil {
		return 0, 0, false
	}
	return p.compositor.Cell(out, point)
}

// Rect returns the pixel rectangle of the shipped frame at output time out that the cell box
// covers, as [compose.Compositor.Rect] maps it; the empty rectangle on a partial pipeline.
func (p *Pipeline) Rect(out time.Duration, box take.Box) image.Rectangle {
	if p.compositor == nil {
		return image.Rectangle{}
	}
	return p.compositor.Rect(out, box)
}

// Size is the shipped frame's pixel size; the zero size on a partial pipeline.
func (p *Pipeline) Size() image.Point {
	if p.compositor == nil {
		return image.Point{}
	}
	return p.compositor.Size()
}

// lastRaster is the [RasterCache] a render streams through: the frames come in output order, so
// consecutive frames showing one snapshot share its raster, and only the last raster put is kept.
// It is not safe for concurrent use.
type lastRaster struct {
	index  int
	raster *image.RGBA
}

// newLastRaster returns a lastRaster holding nothing.
func newLastRaster() *lastRaster { return &lastRaster{index: -1} }

// Get returns the kept raster when it is the snapshot at index's.
func (c *lastRaster) Get(index int) (*image.RGBA, bool) {
	if c.raster == nil || index != c.index {
		return nil, false
	}
	return c.raster, true
}

// Put keeps frame as the snapshot at index's, in place of the raster kept before.
func (c *lastRaster) Put(index int, frame *image.RGBA) { c.index, c.raster = index, frame }
