---
title: Services and profiles started for a take
status: open
type: feature
priority: 2
deps: [project-config]
created: 2026-10-17T08:54:41Z
---

Some apps need a backend while they are filmed, and a demo must not depend on a live one (an
app that calls an HTTP API films against a replay of recorded responses). Per Note
`extension-model` that backend is a **service**: a command in the config, not Go code.

Wanted:
- A service has a name, a command line, env, and a readiness check (a TCP port accepting, or a
  line on its output) with a timeout.
- photon starts every service before the launch, passes `PHOTON_WORK` and the service's port in
  its env, waits for readiness, and stops it after the take (SIGTERM, then kill after a
  timeout), even when the take fails. Its output goes to a log file in the work dir.
- Named **profiles** choose different service commands for the same take (e.g. `replay` by
  default, `capture` against the live backend), selected with `photon record --profile <name>`.
- An optional `on_success` command per profile runs only when every beat passed (e.g. keep
  what a capture service recorded).

## Acceptance

- A test service built from `testdata` is started, reached by the fixture app, and stopped; a
  service that never becomes ready fails the take naming the service and its log path.
- A profile switch and an `on_success` run are tested; `on_success` does not run after a
  failed beat.
