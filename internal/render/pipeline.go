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
// several frames is rasterized once. A cache belongs to one rasterizer and take: a pipeline's
// exact rasterizer keeps its rasters in [BuildOptions.Cache] and its draft rasterizer in
// [BuildOptions.DraftCache]. Get reports false for an index it does not hold; Put may keep or drop
// what it is given.
type RasterCache interface {
	Get(index int) (*image.RGBA, bool)
	Put(index int, frame *image.RGBA)
}

// Pipeline builds the shipped frames of one clip's take by output time: the schedule maps an
// output time onto the snapshot it shows, the rasterizer draws that snapshot at frame.scale and
// the compositor lays the camera, the cursor and the captions over it at frame.width. Every frame
// `photon render` encodes comes from [Pipeline.FrameAt]. Built with a draft cache, it also
// draws draft frames ([Pipeline.DraftAt]) from a second rasterizer drawing at scale 1, padding
// and font size unscaled, its rasters kept in the draft cache; `photon render` draws none.
//
// A pipeline [Build] could not finish holds its schedule and the error that stopped it: its
// frame methods return that error, [Pipeline.Cell] reports no cell, [Pipeline.Rect] the empty
// rectangle, [Pipeline.Size] the zero size, [Pipeline.Unit] 0 and [Pipeline.CaptionBoxes] none.
// It is safe for concurrent use when its raster cache and its draft cache are.
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

	// draftRasterizer draws the snapshots of draft frames at scale 1 into draftCache, and is nil
	// without a draft cache. At frame.scale 1 rasterizer and cache may be these same two.
	draftRasterizer *raster.Rasterizer
	draftCache      RasterCache

	// err is the stage error that left the pipeline without a rasterizer or compositor.
	err error
}

// BuildOptions is what [Build] is given beside the clip and its take. Cache keeps the rasterized
// snapshots of a pipeline that builds its own rasterizer; it must not be nil. DraftCache keeps
// the snapshots its draft rasterizer draws at scale 1; when nil the pipeline builds no draft
// rasterizer and [Pipeline.DraftAt] returns the exact frame.
//
// Reuse, when not nil, is an earlier pipeline whose parts the new one takes over where they come
// out the same. Its draft rasterizer and draft cache come first, while the take, the frame's
// draft geometry, the config's terminal fonts and its theme match — a change of frame.scale alone
// keeps them — and only when the new pipeline has a draft cache. Its rasterizer and its raster
// cache come next, while the take, the frame's raster geometry, the terminal fonts and the theme
// match; failing that, at frame.scale 1 the new pipeline's exact stage takes its own draft
// rasterizer and draft cache, whose rasters are then exact, while the draft stage never takes an
// exact rasterizer or its cache. Its caption faces carry over while the config's caption fonts
// match. A reused cache is then shared by both pipelines, so it must be safe for both to use at
// once when they are.
type BuildOptions struct {
	Cache      RasterCache
	DraftCache RasterCache
	Reuse      *Pipeline
}

// Build checks that the take at takePath, recorded, was recorded at the storyboard's frame size,
// schedules it, and builds the rasterizer and the compositor over loaded's config: frames are
// rasterized at frame.scale — padding and font size multiplied through — so a zoom has pixels to
// draw on, and composed at frame.width, with zoom and pan targets resolved through the config's
// areas. Rasterized snapshots are kept in options.Cache, or in the reused pipeline's cache along
// with its rasterizer; with options.DraftCache set, a draft rasterizer draws at scale 1 into it,
// or the reused pipeline's draft rasterizer and draft cache are taken over; the caption fonts are
// loaded only when options.Reuse holds none that match (see [BuildOptions]).
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
	built := &Pipeline{
		loaded: loaded, recorded: recorded, schedule: clipSchedule,
		cache: options.Cache, draftCache: options.DraftCache,
	}
	if built.err = built.buildFrames(options.Reuse); built.err != nil {
		return built, built.err
	}
	return built, nil
}

// buildFrames builds the draft rasterizer, the rasterizer, the caption faces, the target
// resolver and the compositor over the pipeline's clip, taking from previous what it can reuse,
// and keeps every stage or none.
func (p *Pipeline) buildFrames(previous *Pipeline) error {
	draftRasterizer, draftCache, err := p.draftRastersFrom(previous)
	if err != nil {
		return err
	}
	rasterizer, cache, err := p.rastersFrom(previous, draftRasterizer, draftCache)
	if err != nil {
		return err
	}
	captionFaces, err := p.captionFacesFrom(previous)
	if err != nil {
		return err
	}
	resolver, err := target.New(p.loaded.Config.Areas)
	if err != nil {
		return err
	}
	compositor, err := newCompositor(
		p.loaded, p.recorded, p.schedule, rasterizer, resolver, captionFaces)
	if err != nil {
		return err
	}
	p.rasterizer, p.cache = rasterizer, cache
	p.draftRasterizer, p.draftCache = draftRasterizer, draftCache
	p.captionFaces, p.compositor = captionFaces, compositor
	return nil
}

// draftRastersFrom returns the draft rasterizer and the cache its rasters are kept in: none
// without a draft cache, previous's while they hold for p, and a rasterizer of its own over p's
// draft cache otherwise.
func (p *Pipeline) draftRastersFrom(previous *Pipeline) (*raster.Rasterizer, RasterCache, error) {
	if p.draftCache == nil {
		return nil, nil, nil
	}
	if p.canReuseDraftRasters(previous) {
		return previous.draftRasterizer, previous.draftCache, nil
	}
	rasterizer, err := newDraftRasterizer(p.loaded)
	if err != nil {
		return nil, nil, err
	}
	return rasterizer, p.draftCache, nil
}

// rastersFrom returns the exact rasterizer and the cache its rasters are kept in: previous's
// while they hold for p; at frame.scale 1 the draft rasterizer and draft cache when p has them,
// since a draft is then drawn at the exact geometry; and a rasterizer of its own over p's cache
// otherwise.
func (p *Pipeline) rastersFrom(
	previous *Pipeline,
	draftRasterizer *raster.Rasterizer,
	draftCache RasterCache,
) (*raster.Rasterizer, RasterCache, error) {
	if p.canReuseRasters(previous) {
		return previous.rasterizer, previous.cache, nil
	}
	if draftRasterizer != nil && p.loaded.Storyboard.Frame.Scale == draftScale {
		return draftRasterizer, draftCache, nil
	}
	rasterizer, err := newRasterizer(p.loaded)
	if err != nil {
		return nil, nil, err
	}
	return rasterizer, p.cache, nil
}

// canReuseRasters says previous built in full over the same take with a rasterizer p's clip
// would build alike — the same raster geometry, terminal fonts and theme — so its rasterizer
// draws p's snapshots as p's own would and the rasters in its cache hold for p.
func (p *Pipeline) canReuseRasters(previous *Pipeline) bool {
	return p.drawsAlike(previous, rasterGeometry)
}

// canReuseDraftRasters says previous holds a draft rasterizer p's clip would build alike — over
// the same take, draft geometry, terminal fonts and theme — so the rasters in its draft cache
// hold for p's drafts.
func (p *Pipeline) canReuseDraftRasters(previous *Pipeline) bool {
	return previous != nil && previous.draftRasterizer != nil &&
		p.drawsAlike(previous, draftGeometry)
}

// drawsAlike says previous built in full over p's take, and a rasterizer built over p's clip at
// geometry draws as one built over previous's would: the same geometry, terminal fonts and theme.
func (p *Pipeline) drawsAlike(
	previous *Pipeline,
	geometry func(storyboard.Frame) raster.Geometry,
) bool {
	if previous == nil || previous.compositor == nil || previous.recorded != p.recorded {
		return false
	}
	was, is := previous.loaded, p.loaded
	return geometry(was.Storyboard.Frame) == geometry(is.Storyboard.Frame) &&
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
	return p.compositor.Compose(out, rasterAt(p.recorded, index, p.rasterizer, p.cache)), nil
}

// DraftAt returns a draft of the frame at output time out, of [Pipeline.Size] and with the same
// cursor and captions as [Pipeline.FrameAt]'s: the snapshot on screen then, rasterized at scale 1
// or taken from the draft cache, composed at out by [compose.Compositor.ComposeDraft]. Built
// without a draft cache it returns [Pipeline.FrameAt]'s frame. It fails as [Pipeline.FrameAt]
// does.
func (p *Pipeline) DraftAt(out time.Duration) (*image.RGBA, error) {
	if p.draftRasterizer == nil {
		return p.FrameAt(out)
	}
	index, err := p.screenIndexAt(out)
	if err != nil {
		return nil, err
	}
	source := rasterAt(p.recorded, index, p.draftRasterizer, p.draftCache)
	return p.compositor.ComposeDraft(out, source), nil
}

// rasterAt returns the snapshot at index of recorded drawn by rasterizer: taken from cache when
// it holds it, rasterized and put there otherwise.
func rasterAt(
	recorded *take.Take,
	index int,
	rasterizer *raster.Rasterizer,
	cache RasterCache,
) *image.RGBA {
	if source, ok := cache.Get(index); ok {
		return source
	}
	source := rasterizer.Frame(recorded.Snapshots[index])
	cache.Put(index, source)
	return source
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

// Path returns the points the via pan at index of the storyboard's camera list passes through, in
// pixels of the shipped frame at output time out, as [compose.Compositor.Path] maps them. ok is
// false when the camera move at index is not a via pan, and on a partial pipeline.
func (p *Pipeline) Path(out time.Duration, index int) (points []image.Point, ok bool) {
	if p.compositor == nil {
		return nil, false
	}
	return p.compositor.Path(out, index)
}

// Size is the shipped frame's pixel size; the zero size on a partial pipeline.
func (p *Pipeline) Size() image.Point {
	if p.compositor == nil {
		return image.Point{}
	}
	return p.compositor.Size()
}

// Unit is how many shipped pixels one design pixel spans, as [compose.Compositor.Unit] reports
// it; 0 on a partial pipeline.
func (p *Pipeline) Unit() float64 {
	if p.compositor == nil {
		return 0
	}
	return p.compositor.Unit()
}

// CaptionBoxes returns the box each of the storyboard's captions fills on the shipped frame, in
// storyboard order and in shipped pixels, as [compose.Compositor.CaptionBoxes] lays them out; nil
// on a partial pipeline and for a storyboard with no captions.
func (p *Pipeline) CaptionBoxes() []image.Rectangle {
	if p.compositor == nil {
		return nil
	}
	return p.compositor.CaptionBoxes()
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
