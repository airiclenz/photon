// Package engine performs a storyboard's beats against a running program: it types (humanized
// from a fixed, seeded profile, or flat), presses named keys, clicks on-screen targets with real
// SGR mouse press and release reports, drags from one target to another target or an offset
// with a press, eased held-button motion reports and a release, waits on the screen and pauses.
// Every beat start, action start and end, resolved target, clicked cell and drag's pressed,
// moved-onto and released cell goes into the take's event log, where check and render read
// them. A failing action ends the run with a [BeatError] naming the beat and the
// action.
//
// The engine drives the program through its own [Terminal] interface, which the pty terminal
// and a test fake both satisfy; it never imports the terminal package. Targets are resolved by
// the target package, the one resolver photon has.
//
// It imports take, target, storyboard and keys.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - engine.go: the [Terminal] interface, the [Engine] ([New], [Engine.Run]) and [BeatError].
//   - action.go: performing each action form, and the engine's pacing gaps.
//   - drag.go: performing a drag, and its eased path.
//   - typing.go: the humanized-typing profile and its seeded MINSTD generator.
package engine
