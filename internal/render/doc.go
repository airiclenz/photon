// Package render turns a clip's take into the shipped GIF. [Run] lays each beat of the take onto
// its section's duration (package schedule), rasterizes every scheduled snapshot at the
// storyboard's frame times its scale (package raster, over the config's fonts and theme), composes
// the zooms and the cursor over the clicks and drags at frame.width (package compose), and streams
// the frames as raw RGBA into ffmpeg for a per-clip palette encode: `max_colors` entries, no
// dithering, `fps` frames a second. When gifsicle is on PATH, a lossless `-O3` pass then shrinks
// the GIF in place.
//
// The GIF goes to the storyboard's ship path unless [Options] names another, its directory created
// when missing. Run then prints `<path>  <size>  <length>s` and one `beat N  <speed>×  <take> of
// take in <duration>` row per beat, or `beat N  cut`. A dry run prints the ffmpeg command line and
// runs nothing. A take recorded at another size than the storyboard's frame is refused: its cells
// are what the rasterizer lays out and its targets are resolved on.
//
// It imports clip, schedule, raster, compose, fonts, target, storyboard and take, and runs ffmpeg
// and gifsicle as programs.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - render.go: [Run], its [Options], the render plan (take, schedule, rasterizer, compositor)
//     and the summary.
//   - encode.go: the ffmpeg command line, streaming the frames into it, and the gifsicle pass.
package render
