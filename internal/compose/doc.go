// Package compose turns each rasterized frame of a clip into the shipped frame: it pushes into
// the beat's zoom target, scales to the storyboard's `frame.width` and draws a cursor over the
// clicks and drags on top. [New] plans everything from the storyboard, the take, its schedule and
// the frame [Layout]; [Compositor.Compose] is then a pure function of the output time and the
// source frame.
//
// The crop comes from a camera of two tracks, each holding its last state between moves: a zoom
// track of the magnification, at rest at 1, and a pan track of the crop's centre, at rest at the
// frame's centre. A zoomed beat is planned as moves on both: its section's start aims the pan
// track at the zoom box's centre, the zoom pushes in over its `in` ramp from there, holds, and
// eases back out over its `out` ramp, ending as the section ends; ramps that together outrun the
// section shrink in proportion. Each zoom move runs smootherstep on the factor's exponent, so
// every frame scales by the same ratio at the same eased pace. A pan move carries the centre
// along a centripetal Catmull-Rom path from where the track holds it, through any waypoints, to
// its end, walked by arc length, so it turns without a corner at a waypoint; its progress runs
// smootherstep, or straight when its ease is linear. With no `factor`, the zoom fits the target: the target
// box plus two cells either side fills 70 percent of the frame width, clamped to [1.25, 2.5]. The
// zoom box is the one the take logged for a click of the same beat on the identical target, or
// for a drag of the same beat whose `from` or `to` target is identical, else
// the target resolved on the latest snapshot inside the beat's span that shows it, walking back
// to the snapshot in effect at the beat's start. A zoomed frame is cleared to the background
// colour [New] is given before the push is drawn.
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
// It imports schedule, target, take and storyboard. golang.org/x/image/draw does the
// Catmull-Rom scale and the zoom's affine transform.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - compose.go: [New], the [Compositor], its [Layout], cropping, drawing the cursor and mapping
//     between shipped pixels and cells ([Compositor.Cell], [Compositor.Rect]).
//   - camera.go: the camera's zoom and pan tracks of moves, and their factor and centre at a time.
//   - path.go: a pan's smooth path through its points, walked by arc length.
//   - zoom.go: planning a beat's zoom onto the camera, the zoom box, the auto-fit factor and the
//     smootherstep ease.
//   - cursor.go: the cursor's track of clicks and drags, its position, opacity and ring, and
//     painting it.
package compose
