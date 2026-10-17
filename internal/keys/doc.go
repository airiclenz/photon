// Package keys spells keystrokes and mouse clicks the only way a terminal can: as the bytes it
// sends. A storyboard's `key` action and the config's end keys name a key; this package turns
// the name into the xterm bytes a real terminal would write, so the filmed program reads them
// through its own input parser exactly as it would from a person at the keyboard. A click is the
// SGR (mode 1006) mouse report for a left-button press and its release at a cell.
//
// It is a leaf: it imports nothing from photon.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - keys.go: the key-name table ([Lookup], [Names]) and the SGR mouse reports ([Click],
//     [Release]).
package keys
