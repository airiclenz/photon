// Package rig records one take of a clip end to end, and runs a project's one-off setup.
//
// [Record] creates the work dir, runs the config's `reset`, starts the take's services under the
// chosen profile, and launches the program in a pty at the storyboard's frame: `sh -c
// '<command>'` in the launch dir, with no `exec` prefixed, so a compound command such as
// `. env.sh && exec app` works. The launch sees the operator's env less every variable a
// `launch.strip` glob matches, then `launch.env` with its ${PHOTON_*} references expanded, then
// PHOTON_WORK and every service's PHOTON_PORT_<NAME>. Once the program has painted, the beats run;
// then the take ends: the program is given time to fall idle, the take is frozen so nothing it
// paints on its way out is kept, the quit keys are typed and a program still running is killed
// with its whole process group. The take is written to the clip's take path, and — only when
// every beat passed — checked while the services are still up, after which the services stop and
// the profile's `on_success` runs. A take whose beats failed is still written, because it shows
// why, but is neither checked nor followed by `on_success`.
//
// [Setup] runs the config's `setup` once. Every command the rig runs itself (setup, reset,
// on_success) is `sh -c <command>` in the config's dir with PHOTON_WORK set to the work dir, its
// output going to the caller's writer.
//
// It imports clip, terminal, engine, service, check, take, and the config, storyboard, target
// and keys types those speak in.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - record.go: [Record]: reset, services, launch, first paint, beats, end of take, save, check,
//     on_success.
//   - setup.go: [Setup], the work dir, and running one of the rig's commands.
//   - env.go: the launch's env: the strip globs, `launch.env` expansion and photon's own variables.
package rig
