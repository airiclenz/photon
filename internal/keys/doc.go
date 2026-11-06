// Package keys spells keystrokes and mouse clicks the only way a terminal can: as the bytes it
// sends. A storyboard's `key` action and the config's end keys name a key; this package turns
// the name into the xterm bytes a real terminal would write, so the filmed program reads them
// through its own input parser exactly as it would from a person at the keyboard. A click is the
// SGR (mode 1006) mouse report for a press and its release at a cell, of the left button or any
// other with any modifier keys held; a drag adds the held-button motion reports between them.
// A [Pointer] is one such press, move or release at a cell, the form a pointer event takes before
// a source encodes it; [Pointer.Report] is its SGR report.
//
// It is a leaf: it imports nothing from photon.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - keys.go: the key-name table ([Lookup], [Names]), the SGR mouse reports ([Click],
//     [Release], and [MousePress], [MouseMotion], [MouseRelease] for any [Mouse] button and
//     modifiers), the [Pointer] event and its report, and the storyboard's mouse spellings
//     ([ParseButton], [ParseMods]).
package keys
