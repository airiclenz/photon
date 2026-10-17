// Package check judges a take against its storyboard's expects: [Judge] returns one PASS or FAIL
// [Row] per expect — each beat's in storyboard order, then the take-level ones — and
// [WriteTable] prints them as the `beat N | PASS | detail` table `photon check` shows.
//
// A `seen` expect is a regex matched on the text an area shows (the whole screen without one),
// on the screens shown during its beat, or on any screen of the take for a take-level expect;
// areas are read through the target resolver, so check agrees with clicks, drags and zooms on
// what an area is. A `run` expect is how a project checks what photon cannot see: a command run by
// `sh -c` in the work dir with PHOTON_WORK, PHOTON_TAKE and PHOTON_BEAT in its env, passing on
// exit 0.
//
// It imports take, target and storyboard.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - check.go: [Judge], [Row], [Verdict], [Options], [WriteTable], [Failed]; the beats' screens
//     and the `seen` expect.
//   - run.go: the `run` expect: its command, dir and env.
package check
