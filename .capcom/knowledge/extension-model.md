---
title: "Decision: one generic CLI, projects plug in through commands and services"
labels: [decision]
---

**Decision (2026-10-17):** photon is one generic CLI. A project customizes it only through its
photon config and storyboards: the commands photon runs and the services it starts for a take.
photon exports no Go API, and every package except `cmd/photon` is `internal/`, save the
module root's `Version()`, which only embeds the `VERSION` file, and `cmd/release`, the release
tool: a self-contained `package main` whose non-test code imports only the standard library and
cobra (its tests use `internal/testkit`), never shipped in an archive (Note `package-layout`).

**Why.** What a project needs beyond typing, clicking and rendering varies with the project: a
fake backend, checks against the app's own records, its environment variables, its screen
chrome. The alternative was a Go library that each project wraps in its own binary,
registering Go hooks. It was rejected because:

- the exported packages would become a public API to keep stable;
- only Go projects could extend it;
- most projects need no extension at all, and should not pay for one.

**How a project plugs in, instead:**

| need | hook |
|---|---|
| a known starting state | a `reset` command run before every take; `setup` run once to build the rig |
| a fake backend (e.g. a replayed HTTP API) | a **service**: a command photon starts before the take, waits until ready, and stops after; it gets the port and work dir through the environment |
| more than one recording mode (e.g. replay vs live capture) | named service profiles chosen on the command line, plus a command run only when a take's beats all passed |
| a project-specific check (files written, the app's own records) | a `run:` expect: a command whose exit status is the verdict and whose output is the detail |
| the app's screen layout | declarative **areas** in the config, located by row anchors, never hard-coded glyphs |
