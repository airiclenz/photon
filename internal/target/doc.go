// Package target finds things on a snapshot: where a storyboard target sits, which rows an area
// covers, and the text an area shows. It is the only place targets and areas are resolved — the
// engine aims clicks, drags and waits with it, compose aims pans with it and check reads `seen`
// expects through it — so all of them agree on what a pattern and an area mean.
//
// A snapshot is read row by row as plain text, trailing blanks trimmed, a wide grapheme counted
// once. A regex is matched against one row at a time and never spans rows. An area is a band of
// rows whose edges are the config's row anchors, found afresh on every snapshot, so an area
// follows the app's chrome as it moves. Every failure names the pattern and the area it was
// searched in; one that depends only on what the screen shows wraps [ErrNotOnScreen], so a
// caller can wait for the screen to change rather than give up.
//
// It imports take, storyboard and config, and nothing that runs a program.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - resolver.go: the [Resolver] ([New], [Resolver.Target], [Resolver.Rows], [Resolver.Text]),
//     a snapshot's rows as text and [ErrNotOnScreen].
//   - anchor.go: the config's areas and row anchors, compiled, and how they pick rows.
package target
