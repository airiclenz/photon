---
title: Package map and dependency direction
labels: [architecture]
---

The package map, as refined by the v1 implementation plan (`docs/plans/2026-10-17 - 00 - photon-v1-plan.md`,
2026-10-17). A later change goes back into this Note. Dependencies point down the table; nothing
imports upward, and there are no cycles.

| package | owns | imports |
|---|---|---|
| `internal/keys` | named keys to bytes, SGR mouse press, motion and release | — |
| `internal/take` | the Take: snapshots, cells, colours, typed events, `Box`, beat spans; its versioned file format | — |
| `internal/storyboard` | a clip's Storyboard; strict decode and validation | `keys` |
| `internal/config` | the project's `photon.yaml` (rig, launch, services, profiles, end, areas, theme, fonts) and its lookup | `keys` |
| `internal/clip` | a storyboard and its config loaded together, cross-checked; work dir and take path | `storyboard`, `config` |
| `internal/target` | a target's cell box and an area's rows on a snapshot; the only resolver (clicks, drags, zooms, `seen`) | `take`, `storyboard`, `config` |
| `internal/terminal` | the program in a pty, the vt emulator, sampled snapshots, first paint, settle, quit, the EPERM probe; a Windows stand-in that refuses | `take` |
| `internal/engine` | performing a beat's actions; defines the `Terminal` interface it drives (pty terminal plus a test fake) | `take`, `target`, `storyboard`, `keys` |
| `internal/service` | a take's services: start, readiness, ports, logs, stop; profiles | `config` |
| `internal/check` | judging a take: one PASS/FAIL row per expect | `take`, `target`, `storyboard` |
| `internal/rig` | one take end to end: reset, services, launch, beats, end of take, save, `on_success`, check; `setup` | `clip`, `terminal`, `engine`, `service`, `check`, `take`, `config`, `storyboard`, `target`, `keys` |
| `internal/schedule` | laying the beats onto section durations, holds and cuts | `storyboard`, `take` |
| `internal/fonts` | the embedded faces and their OFL licences, or an override dir | — |
| `internal/raster` | a snapshot's cells to pixels; box drawing drawn, not looked up | `take`, `fonts`, `config` |
| `internal/caption` | a storyboard's captions laid out, timed and drawn over a shipped frame, in output space | `storyboard`, `fonts` |
| `internal/compose` | camera and drawn cursor over each frame | `schedule`, `target`, `take`, `storyboard` |
| `internal/render` | the render run: frames through ffmpeg, then gifsicle | `clip`, `schedule`, `raster`, `compose`, `fonts`, `target`, `storyboard`, `take` |
| `internal/yamledit` | YAML source splicing: replace, insert or remove one scalar or key, leaving the rest of the bytes as written | — |
| `internal/tune` | the timing editor behind `photon tune`: session, edits spliced into the storyboard, undo, guarded save, frames on demand, target picks, and the embedded page served on 127.0.0.1; nothing imports it but `cmd/photon` | `clip`, `render`, `schedule`, `compose`, `raster`, `target`, `storyboard`, `take`, `yamledit`, `config` |
| `internal/testkit` | test support: doc-map check, fixture TUI build, polling | — |
| `cmd/photon` | the Cobra commands and the exit-code mapping only | the packages above |
| `photon` (module root) | `Version()`, embedded from the root `VERSION` file, photon's one version source | — |
| `cmd/release` | the release tool, a self-contained `package main` never shipped in an archive: `dist`, `status`, `prepare`, `publish`, `smoke` | non-test code imports only the standard library and cobra; its tests use `internal/testkit` |

Rules: `cmd/photon` holds no logic beyond flags and wiring. `cmd/release` is a dev tool run as
`go run ./cmd/release <verb>`: no shipped package imports it, it imports no photon package
outside its tests, and its files never land in a release archive. An interface type appears only where
two adapters exist, e.g. the engine's `Terminal` (the pty terminal plus a test fake); `engine`
never imports `terminal`. A domain package (`storyboard`, `config`, `take`, `clip`) imports no
process, pty or image code; the emulator-cell conversion lives in `terminal`, palette resolution
in `raster`.

The root package exists only because `go:embed` reaches no file above a package's directory and
`VERSION` sits at the module root; it holds `Version()` and nothing else. A release updates
`VERSION` and nothing in Go code.
