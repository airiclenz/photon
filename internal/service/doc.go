// Package service runs a take's services: the helper processes a config declares, such as a fake
// backend the filmed program talks to. [Start] starts them in name order under the chosen
// profile, each by `sh -c` in the config's dir with its own process group, gives each a port
// (fixed, or a free one the kernel picks), waits until each is ready (its port accepting a
// connection, or a line of its output matching) within its timeout, and logs each one's output to
// `<work>/<name>.log`. The returned [Set] lists the ports for every later command ([Set.Env]) and
// stops the services ([Set.Stop]): SIGTERM to each group, then a kill after a timeout.
//
// A service's env holds PHOTON_WORK, PHOTON_PORT (its own port) and the PHOTON_PORT_<NAME> of
// every service started before it; its config env values may name those as ${PHOTON_*}, which
// [ExpandEnv] expands, the one place photon expands anything.
//
// It imports only config.
//
// The file map:
//   - doc.go: this package comment and file map.
//   - service.go: [Start], the [Set] ([Set.Env], [Set.Stop]), profiles, ports and the sentinel
//     errors.
//   - ready.go: waiting for a service to be ready, by TCP accept or by an output line.
//   - expand.go: [ExpandEnv], the ${PHOTON_*} expansion of config env values.
//   - procgroup_unix.go: on unix, a service's own process group, signalled and killed whole.
//   - procgroup_other.go: elsewhere, the service process alone.
package service
