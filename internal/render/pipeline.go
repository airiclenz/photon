package render

import (
	"errors"
	"fmt"
	"image"
	"slices"
	"sync"
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
// draws draft frames ([Pipeline.DraftAt]) from a draft rasterizer drawing at about the shipped
// frame's size, at most frame.width wide, its rasters kept in the draft cache; when the exact
// rasters are no wider than frame.width the draft stage is the exact one. `photon render` draws
// none.
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

	// draftRasterizer draws the snapshots of draft frames at draftGeometry into draftCache, and
	// is nil without a draft cache. When draftGeometry is the raster geometry these two are
	// rasterizer and cache.
	draftRasterizer *raster.Rasterizer
	draftCache      RasterCache
	draftGeometry   raster.Geometry

	// err is the stage error that left the pipeline without a rasterizer or compositor.
	err error
}

// BuildOptions is what [Build] is given beside the clip and its take. Cache keeps the rasterized
// snapshots of a pipeline that builds its own rasterizer; it must not be nil. DraftCache keeps
// the snapshots its draft rasterizer draws at the shipped frame's size; when nil the pipeline
// builds no draft rasterizer and [Pipeline.DraftAt] returns the exact frame. When the exact
// rasters are no wider than frame.width, the draft geometry is the exact one and the draft stage
// is the exact stage, its rasterizer and Cache, so DraftCache is left unused.
//
// Reuse, when not nil, is an earlier pipeline whose parts the new one takes over where they come
// out the same. Its rasterizer and its raster cache come first, while the take, the frame's raster
// geometry, the config's terminal fonts and its theme match. Its draft rasterizer and draft cache
// come next, only when the new pipeline has a draft cache and its draft geometry is not its raster
// geometry — the new exact stage is the draft stage then, whatever draft stage Reuse held — while
// the take, the draft geometry, the terminal fonts and the theme match, and — when its draft stage
// was its exact stage — only along with that exact stage, so a draft stage never binds an exact
// cache the new exact stage does not share. Its caption faces carry over while the config's
// caption fonts match. A reused cache is then shared by both pipelines, so it must be safe for
// both to use at once when they are.
type BuildOptions struct {
	Cache      RasterCache
	DraftCache RasterCache
	Reuse      *Pipeline
}

// Build checks that the take at takePath, recorded, was recorded at the storyboard's frame size,
// schedules it, and builds the rasterizer and the compositor over loaded's config: frames are
// rasterized at frame.scale — padding and font size multiplied through — so a zoom has pixels to
// draw on, and composed at frame.width, with the camera positions' text targets resolved through
// the config's areas. Rasterized snapshots are kept in options.Cache, or in the reused pipeline's
// cache along with its rasterizer; with options.DraftCache set, the exact stage stands in when it
// already draws at the shipped frame's size, and otherwise the reused pipeline's draft rasterizer
// and draft cache are taken over or a draft rasterizer draws at that size into it; the caption
// fonts are loaded only when options.Reuse holds none that match (see [BuildOptions]).
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

// buildFrames builds the rasterizer, the draft rasterizer from it, the caption faces, the target
// resolver and the compositor over the pipeline's clip, taking from previous what it can reuse,
// and keeps every stage or none. The two rasterizers share one load of the terminal fonts.
func (p *Pipeline) buildFrames(previous *Pipeline) error {
	faces := sync.OnceValues(func() (fonts.Set, error) { return loadTerminalFaces(p.loaded) })
	rasterizer, cache, err := p.rastersFrom(previous, faces)
	if err != nil {
		return err
	}
	draftAt, err := p.draftGeometryFrom(previous, faces, rasterizer)
	if err != nil {
		return err
	}
	draftRasterizer, draftCache, err := p.draftRastersFrom(
		previous, faces, draftAt, rasterizer, cache)
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
	p.draftRasterizer, p.draftCache, p.draftGeometry = draftRasterizer, draftCache, draftAt
	p.captionFaces, p.compositor = captionFaces, compositor
	return nil
}

// rastersFrom returns the exact rasterizer and the cache its rasters are kept in: previous's
// while they hold for p, and otherwise a rasterizer of its own over the faces faces loads, with
// p's cache.
func (p *Pipeline) rastersFrom(
	previous *Pipeline,
	faces func() (fonts.Set, error),
) (*raster.Rasterizer, RasterCache, error) {
	if p.canReuseRasters(previous) {
		return previous.rasterizer, previous.cache, nil
	}
	set, err := faces()
	if err != nil {
		return nil, nil, err
	}
	rasterizer, err := newRasterizerAt(p.loaded, set, rasterGeometry(p.loaded.Storyboard.Frame))
	if err != nil {
		return nil, nil, err
	}
	return rasterizer, p.cache, nil
}

// draftGeometryFrom returns the geometry p's drafts are rasterized at, as [draftGeometry] derives
// it from p's exact rasterizer, and the zero geometry without a draft cache. When p took over
// previous's exact rasterizer and ships at previous's frame.width, it is previous's draft
// geometry, so such a rebuild loads no terminal font.
func (p *Pipeline) draftGeometryFrom(
	previous *Pipeline,
	faces func() (fonts.Set, error),
	rasterizer *raster.Rasterizer,
) (raster.Geometry, error) {
	if p.draftCache == nil {
		return raster.Geometry{}, nil
	}
	isUnchanged := previous != nil && previous.draftRasterizer != nil &&
		previous.rasterizer == rasterizer &&
		previous.loaded.Storyboard.Frame.Width == p.loaded.Storyboard.Frame.Width
	if isUnchanged {
		return previous.draftGeometry, nil
	}
	set, err := faces()
	if err != nil {
		return raster.Geometry{}, err
	}
	return draftGeometry(p.loaded, set, rasterizer)
}

// draftRastersFrom returns the draft rasterizer and the cache its rasters are kept in: none
// without a draft cache; p's exact rasterizer and cache when geometry is the raster geometry,
// whatever draft stage previous holds; previous's while they hold for p at geometry; and
// otherwise a rasterizer of its own at geometry over the faces faces loads, with p's draft cache.
func (p *Pipeline) draftRastersFrom(
	previous *Pipeline,
	faces func() (fonts.Set, error),
	geometry raster.Geometry,
	rasterizer *raster.Rasterizer,
	cache RasterCache,
) (*raster.Rasterizer, RasterCache, error) {
	if p.draftCache == nil {
		return nil, nil, nil
	}
	if geometry == rasterGeometry(p.loaded.Storyboard.Frame) {
		return rasterizer, cache, nil
	}
	if p.canReuseDraftRasters(previous, geometry, rasterizer) {
		return previous.draftRasterizer, previous.draftCache, nil
	}
	set, err := faces()
	if err != nil {
		return nil, nil, err
	}
	draftRasterizer, err := newRasterizerAt(p.loaded, set, geometry)
	if err != nil {
		return nil, nil, err
	}
	return draftRasterizer, p.draftCache, nil
}

// canReuseRasters says previous built in full over the same take with a rasterizer p's clip
// would build alike — the same raster geometry, terminal fonts and theme — so its rasterizer
// draws p's snapshots as p's own would and the rasters in its cache hold for p.
func (p *Pipeline) canReuseRasters(previous *Pipeline) bool {
	if !p.drawsAlike(previous) {
		return false
	}
	return rasterGeometry(previous.loaded.Storyboard.Frame) ==
		rasterGeometry(p.loaded.Storyboard.Frame)
}

// canReuseDraftRasters says previous holds a draft rasterizer p would build alike at geometry —
// over the same take, draft geometry, terminal fonts and theme — so the rasters in its draft
// cache hold for p's drafts. p asks only for a geometry that is not its raster geometry, its
// exact stage standing in otherwise. A draft stage that was previous's exact stage is reused only
// along with that exact stage, as p's rasterizer, so p never keeps drafts in an exact cache its
// own exact stage does not use.
func (p *Pipeline) canReuseDraftRasters(
	previous *Pipeline,
	geometry raster.Geometry,
	rasterizer *raster.Rasterizer,
) bool {
	if previous == nil || previous.draftRasterizer == nil || !p.drawsAlike(previous) {
		return false
	}
	isSharedStage := previous.draftRasterizer == previous.rasterizer
	return previous.draftGeometry == geometry &&
		(!isSharedStage || previous.rasterizer == rasterizer)
}

// drawsAlike says previous built in full over p's take, and a rasterizer built over p's clip
// draws as one built over previous's would at the same geometry: the same terminal fonts and
// theme.
func (p *Pipeline) drawsAlike(previous *Pipeline) bool {
	if previous == nil || previous.compositor == nil || previous.recorded != p.recorded {
		return false
	}
	was, is := previous.loaded, p.loaded
	return was.Config.Fonts == is.Config.Fonts &&
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
// cursor and captions as [Pipeline.FrameAt]'s: the snapshot on screen then, rasterized at the
// draft geometry or taken from the draft cache, composed at out by
// [compose.Compositor.ComposeDraft]. Built
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
