// Package terminal runs a program under a real pseudo-terminal, emulates the screen it paints
// with charmbracelet/x/vt, and records what it shows into a [take.Take].
//
// Bytes the program writes go into the emulator; the replies the emulator produces to the
// program's queries (DA1, DECRQM, a cursor position report) go back to the program as input, so a
// program that asks the terminal who it is never hangs waiting; photon's own input (typed text, a
// key's sequence, a mouse report) goes in through [Terminal.Send]. A sampler copies the screen
// into the take no faster than the take's fps, and only when it changed: two snapshots in a row
// never show the same picture.
//
// The emulator-cell to [take.Cell] conversion lives here, so the take stays free of emulator
// types. On Windows there is no unix pty: [Start] returns [ErrNoPTY] and the rest of the package
// is a stand-in that only builds.
//
// It imports take and nothing above it.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - terminal.go: the [Terminal] and its [Options]: [Start], input, events, the pumps, [Terminal.Close].
//   - sample.go: the sampler, snapshots of the emulator and the collapse of identical grids.
//   - cell.go: an emulator cell and colour as a [take.Cell] and [take.Color]; a whole grid.
//   - margins.go: the scroll-margin clamp that keeps x/vt's line operations inside its buffer.
//   - terminal_windows.go: the Windows stand-in; [Start] there returns [ErrNoPTY].
package terminal
