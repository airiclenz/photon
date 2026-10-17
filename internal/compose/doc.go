// Package compose turns each rasterized frame of a clip into the shipped frame: it pushes into
// the beat's zoom target, scales to the storyboard's `frame.width` and draws a click cursor on
// top. [New] plans everything from the storyboard, the take, its schedule and the frame
// [Layout]; [Compositor.Compose] is then a pure function of the output time and the source frame.
//
// A zoomed beat pushes in over its `in` ramp from the start of its section, holds, and eases back
// out over its `out` ramp before the section ends; ramps that together outrun the section shrink
// in proportion. Each ramp runs smootherstep on the factor's exponent, so every frame scales by
// the same ratio at the same eased pace. With no `factor`, the zoom fits the target: the target
// box plus two cells either side fills 70 percent of the frame width, clamped to [1.25, 2.5]. The
// zoom box is the one the take logged for a click of the same beat on the identical target, else
// the target resolved on the latest snapshot inside the beat's span that shows it, walking back
// to the snapshot in effect at the beat's start. A zoomed frame is cleared to the background
// colour [New] is given before the push is drawn.
//
// The cursor is drawn, not recorded: a white dot in a dark outline that fades in, glides
// (smootherstep) from the previous click point — the bottom row's centre before the first — to
// each click, rings on the press, and fades out after the beat's last click. Its size is fixed in
// design pixels, scaled only by the frame's raster scale and the downscale to the shipped width,
// never by the zoom. A click inside a cut beat draws nothing.
//
// It imports schedule, target, take and storyboard. golang.org/x/image/draw does the
// Catmull-Rom scale and the zoom's affine transform.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - compose.go: [New], the [Compositor], its [Layout], cropping and drawing the cursor.
//   - zoom.go: zoom plans, the zoom box, the auto-fit factor and the smootherstep ease.
//   - cursor.go: the cursor's track of clicks, its position, opacity and ring, and painting it.
package compose
