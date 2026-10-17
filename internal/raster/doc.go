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
//
// Box drawing (U+2500–257F) and block elements (U+2580–259F) never come from a face: they are
// drawn procedurally across the cell, so a line or block meets its neighbour's edge exactly. A
// light line is an eighth of the cell width (at least a pixel), a heavy one twice it, a double
// one two light lines a light line apart; dashes, rounded corners and diagonals are drawn in the
// light weight, block elements fill whole-pixel eighths, shades cover a quarter, half or three
// quarters of the ink. Straight lines and blocks are integer fills, the same on every
// architecture; rounded corners and diagonals are anti-aliased. Bold and italic do not change a
// procedural glyph. [Rasterizer.Resolves] reports whether a rune draws: procedurally, or from a
// face.
//
// It imports take, fonts and config. golang.org/x/image/font/opentype builds the glyph faces from
// the fonts package's parsed sfnt fonts; golang.org/x/image/vector fills the anti-aliased
// rounded corners and diagonals.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - raster.go: [New], the [Rasterizer] and its [Geometry], cell painting and colour resolution.
//   - glyph.go: the face chain, cell metrics and the cache of glyph masks.
//   - box.go: the procedural ranges and box drawing: arms, dashes, rounded corners, diagonals.
//   - block.go: block elements: halves, eighths, shades and quadrants.
package raster
