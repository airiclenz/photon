// Package compose turns each rasterized frame of a clip into the shipped frame: it crops through
// the storyboard's camera, scales to the storyboard's `frame.width`, draws a cursor over the
// clicks and drags on top and the storyboard's captions over everything. [New] plans everything
// from the storyboard, the take, its schedule, the frame [Layout] and the caption faces;
// [Compositor.Compose] is then a pure function of the output time and the source frame.
//
// The crop comes from a camera of two tracks, each holding its last state between moves: a zoom
// track of the magnification, at rest at 1, and a pan track of the crop's centre, at rest at the
// frame's centre. The storyboard's `camera` moves are planned onto them, each at its `at` on the
// output clock for its `over`: a zoom move goes on the zoom track alone and eases to its factor,
// and a pan move is the only move of the pan track. Each zoom move runs smootherstep on the
// factor's exponent, so every frame scales by the same ratio at the same eased pace. A pan move
// carries the centre along a centripetal Catmull-Rom path from the centre the crop shows at its
// start — the frame's centre at a factor of 1, else the held centre clamped as the crop clamps
// it — through any waypoints, to its end, walked by arc length, so it turns without a corner at a
// waypoint; its progress runs smootherstep, or straight when its ease is linear. A pan's points
// are a cell's centre or a target's box centre, resolved on the screen shown at the move's end
// and walking back to the one shown at its start. A zoomed frame is cleared to the background
// colour [New] is given before the crop is drawn, and the cursor is mapped through the same crop,
// so it follows the camera.
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
// on the shipped frame, so no zoom or pan moves or scales a caption. [Output] is the shipped
// frame's size and design-pixel unit that [New] lays the captions out at, so a caption check
// without a take lays them out alike.
//
// It imports schedule, target, take, storyboard, caption and fonts. golang.org/x/image/draw
// does the Catmull-Rom scale and the zoom's affine transform.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - compose.go: [New], [Output], the [Compositor], its [Layout], cropping, drawing the cursor
//     and mapping between shipped pixels and cells ([Compositor.Cell], [Compositor.Rect]) and a
//     via pan's points onto the shipped frame ([Compositor.Path]).
//   - camera.go: the camera's zoom and pan tracks of moves, their factor and centre at a time,
//     planning the storyboard's camera moves onto them, a via pan's points kept by its index,
//     and resolving a target on the screens a stretch of the take shows.
//   - path.go: a pan's smooth path through its points, walked by arc length.
//   - cursor.go: the cursor's track of clicks and drags, its position, opacity and ring,
//     painting it, and the smootherstep ease.
package compose
