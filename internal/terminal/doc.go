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
// A take can start at the program's first paint instead of its launch ([Options.FromFirstPaint]),
// so a wrapper's output never reaches it; [Terminal.Settle] waits for the program to go quiet,
// [Terminal.Quit] types its quit keys and waits for it to leave, and [Terminal.Freeze] ends the
// take while the program winds down. The program leads its own session and process group, and
// every kill goes to that whole group, so a shell between the pty and the app
// (`sh -c '. env.sh && app'`) never outlives the take or shields the app from it. A launch the
// kernel refuses with EPERM is retried, and the last refusal reports which launch step was
// refused and which processes hold the pty (read from /proc on Linux, "unknown" elsewhere).
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
//   - lifecycle.go: first paint, [Terminal.Painted], [Terminal.Settle], [Terminal.Quit], [Terminal.Freeze].
//   - probe.go: the EPERM retry of a launch and the probe its last refusal reports.
//   - ttyowner_linux.go: the processes holding a pty as their controlling terminal, from /proc.
//   - ttyowner_other.go: the same off Linux, where the owner is unknown.
//   - terminal_windows.go: the Windows stand-in; [Start] there returns [ErrNoPTY].
package terminal
