// Package raster turns a take's snapshots into pixels. [New] builds a [Rasterizer] over the
// embedded (or overriding) faces, the project's theme and the clip's [Geometry]; its
// [Rasterizer.Frame] paints one snapshot into an RGBA image the render encodes.
//
// The cell is sized from the face, not configured: a column is as wide as Source Code Pro's `M`
// advance at the font size, and a row is the font size times the line height, rounded to whole
// pixels. Each cell paints its background across its span, then its glyph, then its underline.
// Bold and italic pick Source Code Pro's own files (bold italic draws bold), faint keeps 60
// percent of the foreground over the background, and reverse swaps the two before faint applies.
//
// Colours resolve here, never in the take: the terminal's default foreground and background and
// palette entries 0-15 come from the config's theme, 16-255 from the standard xterm 256-colour
// palette (its colour cube and grey ramp computed in this package), and a truecolor value is
// itself.
//
// A rune Source Code Pro lacks is drawn from the first fallback face that has it; a fallback
// glyph wider than its cell is scaled down to fit, so it never spills into a neighbour.
// [Rasterizer.Resolves] reports whether any face draws a rune.
//
// It imports take, fonts and config. golang.org/x/image/font/opentype builds the glyph faces from
// the fonts package's parsed sfnt fonts.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - raster.go: [New], the [Rasterizer] and its [Geometry], cell painting and colour resolution.
//   - glyph.go: the face chain, cell metrics and the cache of glyph masks.
package raster
