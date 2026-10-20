// Package compose turns each rasterized frame of a clip into the shipped frame: it crops through
// the storyboard's camera, scales to the storyboard's `frame.width`, draws a cursor over the
// clicks and drags on top and the storyboard's captions over everything. [New] plans everything
// from the storyboard, the take, its schedule, the frame [Layout] and the caption faces;
// [Compositor.Compose] is then a pure function of the output time and the source frame.
//
// The crop comes from a camera of two tracks: a zoom track of the magnification and a position
// track of where it aims. The storyboard's `camera.zooms` go on the zoom track, each at its `at`
// on the output clock for its `over`; it rests at 1 and holds the factor its last zoom ended on
// between zooms. Each zoom eases the factor's exponent, so every frame scales by the same ratio
// at the same eased pace. The storyboard's `camera.positions` go on the position track, sorted
// by start: the camera aims at a position's point through its hold, at the first one's before
// it and at the last one's after it, and across the gap between two positions glides from the
// centre of the crop shown as the earlier ends to the centre of the crop shown as the later
// starts, each clamped at the factor of its moment; a gap of 0 is a cut. With no positions it
// aims at the frame's centre. Every zoom and glide runs by its ease: smootherstep (`smooth`, the
// default), straight (`linear`), t³ (`ease-in`) or 1−(1−t)³ (`ease-out`). A position's point is
// a cell's centre or the centre of a text target's box, resolved on the screen shown at the
// position's start only. The crop is clamped inside the frame on every frame. A zoomed frame is
// cleared to the background colour [New] is given before the crop is drawn, and the cursor is
// mapped through the same crop, so it follows the camera.
//
// The cursor is drawn, not recorded: a white dot in a dark outline that fades in, glides
// (smootherstep) from the previous point — the bottom row's centre before the first — to each
// click and each drag's press, and rings on the press. Through a drag it eases (smootherstep)
// from the press cell to the drag's final move over the span between them, sits at the release
// cell and rings again on the release, never on a move. It fades out after the beat's last press
// or release. Its size is fixed in design pixels, scaled only by the frame's raster scale and the
// downscale to the shipped width, never by the zoom. A click or drag inside a cut beat draws
// nothing.
//
// The captions are a [caption.Layer] that [New] builds from the storyboard and the caption
// faces it is given, only when the storyboard has captions; [Compositor.Compose] draws it last,
// on the shipped frame, so the camera neither moves nor scales a caption. [Output] is the shipped
// frame's size and design-pixel unit that [New] lays the captions out at, so a caption check
// without a take lays them out alike.
//
// It imports schedule, target, take, storyboard, caption and fonts. golang.org/x/image/draw
// does the Catmull-Rom scale and the zoom's affine transform.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - compose.go: [New], [Output], the [Compositor], its [Layout], cropping, the full view
//     ([Compositor.ComposeFull]), drawing the cursor and mapping between shipped or full-view
//     pixels and cells ([Compositor.Cell], [Compositor.Rect] and their full-view variants).
//   - camera.go: the camera's zoom track of moves and its position track, their factor and aim
//     at a time, the eases, and planning the storyboard's zooms and positions onto them,
//     resolving a position's text target on the screen shown at its start; the camera's [View]
//     over the full view and each position's aimed cell ([Compositor.PositionCell]).
//   - position.go: the position track of the camera's aims on the output clock, held at each
//     position and glided across the gaps between them.
//   - cursor.go: the cursor's track of clicks and drags, its position, opacity and ring,
//     painting it, and the smootherstep ease.
package compose
