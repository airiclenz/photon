// Package render turns a clip's take into the shipped GIF. [Run] lays each beat of the take onto
// its section's duration (package schedule), rasterizes every scheduled snapshot at the
// storyboard's frame times its scale (package raster, over the config's fonts and theme), composes
// the zooms and the cursor over the clicks and drags at frame.width (package compose), and streams
// the frames as raw RGBA through ffmpeg twice for a per-clip palette encode: `max_colors` entries,
// no dithering, `fps` frames a second. Pass 1 builds the clip's palette into
// `<work>/<clip>.palette.png`, pass 2 maps the frames onto it into the GIF, and the palette file is
// removed after, whether the render succeeded or not. The frames are composed afresh for each pass and
// none is kept, only the last rasterized snapshot, so memory stays flat however long the clip. Two concurrent
// renders of one clip share that palette file, so they must not overlap. When gifsicle is on PATH,
// a lossless `-O3` pass then shrinks the GIF in place.
//
// The GIF goes to the storyboard's ship path unless [Options] names another, its directory created
// when missing. Run then prints `<path>  <size>  <length>s` and one `beat N  <speed>×  <take> of
// take in <duration>` row per beat, or `beat N  cut`. A dry run prints the two ffmpeg command lines,
// the palette pass first, one per line, and runs nothing. A take recorded at another size than the
// storyboard's frame is refused: its cells are what the rasterizer lays out and its targets are
// resolved on.
//
// Every frame comes from one [Pipeline]: [Build] checks the take's frame size, schedules it and
// builds the rasterizer, the target resolver and the compositor, and the pipeline answers the
// frame, the screen, a cell or a cell box's pixels by output time, rasterized snapshots kept in
// the [RasterCache] it is given (a render keeps the last one).
//
// It imports clip, schedule, raster, compose, fonts, target, storyboard and take, and runs ffmpeg
// and gifsicle as programs.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - render.go: [Run], its [Options], the render plan (take, pipeline, palette and output
//     paths) and the summary.
//   - pipeline.go: [Pipeline] and [Build], the frame-size check, [ErrOutsideClip], the
//     [RasterCache] seam and the render's reuse-last cache.
//   - encode.go: the palette pass's and the GIF pass's ffmpeg command lines, streaming the frames
//     into each, and the gifsicle pass.
package render
