// Command photon records a terminal app's demo clip from a storyboard, checks the take, and
// renders the shipped GIF. This package holds the Cobra commands and nothing else: each parses
// its flags, calls one internal package, and maps the error to an exit code.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - main.go: the entry point, and execute, which prints every error and returns the exit status.
//   - root.go: the root command.
//   - lint.go: `photon lint`, which loads a clip and reports every problem or ok.
//   - setup.go: `photon setup`, which runs the config's setup once to build the rig.
//   - record.go: `photon record`, which records one take, checks it and prints the table.
//   - pty_unix.go, pty_windows.go: whether record and setup can run here; on Windows they refuse
//     with the terminal's "needs a unix pty" error before touching the rig.
//   - check.go: `photon check`, which judges a take against its storyboard's expects and prints
//     the table.
//   - render.go: `photon render`, which renders a take to the shipped GIF and prints its summary.
//   - version.go: `photon version`, which prints the version the root package embeds from VERSION.
//   - exit.go: the exit codes, and the wrapping that tells a failed run from a bad command line.
package main
