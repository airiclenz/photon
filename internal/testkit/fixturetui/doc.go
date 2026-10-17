// Command fixturetui is the small terminal app photon's own tests and its hello example film. It
// stands in for a real program: it enters the alternate screen and raw mode, enables SGR mouse
// reports, asks the terminal for its DA1 answer (and goes on without one after a short wait), and
// draws a title, the list alpha/beta/gamma, an input row, a status row, a footer row and a
// hairline along the bottom row.
//
// Typed text echoes on the input row; enter shows "you typed: <text>" on the status row and a
// left click on a list item shows "selected: <item>". The first status reads "ready, terminal
// answered" or "ready, terminal silent", so a test sees whether the DA1 query was answered. When
// FIXTURE_INPUT_LOG names a file, every input chunk the fixture reads, less the DA1 answer, is
// appended to it as one Go-quoted line. ctrl+c, or the end of its input, quits it.
//
// Tests get its path from testkit.BuildFixtureTUI.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - main.go: the fixture: terminal setup, the DA1 query, input parsing and the screen.
package main
