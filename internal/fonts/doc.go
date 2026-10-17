// Package fonts holds the faces a clip's frames are drawn with. Source Code Pro (regular, bold,
// italic) is the terminal face; Noto Sans Symbols 2, Noto Sans Math and Noto Sans Symbols are
// tried, in that order, for a rune it lacks. Source Sans 3 (regular, bold) is the caption face.
// All eight files are embedded, so a clip renders the same on every machine without a project
// vendoring fonts or the operator installing them, and the SIL Open Font License texts that cover
// them sit beside them: SourceCodePro-OFL.txt, NotoSansMath-OFL.txt, NotoSansSymbols-OFL.txt for
// both Symbols families, and SourceSans3-OFL.txt. Those texts must travel with the files.
//
// [Load] returns the parsed terminal [Set], from the embed or from a project's override dir (the
// config's `fonts`), which then has to hold all six terminal files under the same names.
// [LoadCaption] returns the parsed [CaptionSet], each style from the embed or from the file the
// config's `caption_font` names; the `fonts` dir does not reach it.
//
// It imports no photon package. golang.org/x/image/font/opentype parses the files into the
// sfnt fonts the raster builds its glyph faces from.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - fonts.go: [Load], [LoadCaption], the [Set], the [CaptionSet] and their [Face]s, the embed
//     and the file-to-licence tables.
package fonts
