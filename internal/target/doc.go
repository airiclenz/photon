// Package target finds things on a snapshot: where a storyboard target sits, the box an area
// covers, and the text an area shows. It is the only place targets and areas are resolved — the
// engine aims clicks, drags and waits with it, compose aims camera positions with it and check
// reads `seen` expects through it — so all of them agree on what a pattern and an area mean.
//
// A snapshot is read through its text layer ([take.Snapshot.Text]): runs of plain text, each with
// its box on the grid, grouped into lines by their top edge. On a terminal a run is a row,
// trailing blanks trimmed, a wide grapheme counted once, and a line is that row. A regex is
// matched against one run at a time and never spans runs. An area is a band of lines whose edges
// are the config's anchors, found afresh on every snapshot, so an area follows the app's chrome
// as it moves; it holds what lies within its top and bottom, whatever the columns. Every failure
// names the pattern and the area it was searched in; one that depends only on what the screen
// shows wraps [ErrNotOnScreen], so a caller can wait for the screen to change rather than give
// up.
//
// It imports take, storyboard and config, and nothing that runs a program.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - resolver.go: the [Resolver] ([New], [Resolver.Target], [Resolver.Area], [Resolver.Text]),
//     a snapshot's text layer as lines and [ErrNotOnScreen].
//   - anchor.go: the config's areas and anchors, compiled, and how they pick lines.
package target
