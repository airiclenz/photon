// Package take holds the Take — one recording of a program in a terminal — and its file format.
// A take is everything the later steps read: `check` judges its snapshots, the schedule times
// its beats from its events, and the render draws its cells, so none of them goes back to the
// program or the pty.
//
// A take is full-grid snapshots stamped on the take's clock plus a typed log of what photon did
// along the way (beat starts, actions, resolved targets, clicks, and each drag's press, moves and
// release). On disk it is a gzip-compressed JSON-lines file whose first line names the format and
// its version; a foreign file, a newer version or a truncated file is refused with an error naming
// the file.
//
// It is a leaf: it imports only the standard library. Turning emulator cells into [Cell]s lives
// with the terminal, and resolving a palette index to RGB lives with the rasterizer.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - take.go: the [Take], its [Snapshot]s and [Cursor], its beat spans ([Take.BeatSpans]) and
//     its beats matched against the storyboard's ([Take.MatchBeats], [BeatError]).
//   - cell.go: one snapshot [Cell] and its [Color] (default, palette index or RGB).
//   - event.go: the typed [Event], its [EventKind]s, and the [Box] and [Point] it carries.
//   - file.go: the versioned take file ([Write], [Read], [Save], [Load]).
package take
