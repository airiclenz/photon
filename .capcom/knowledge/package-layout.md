---
title: Package map and dependency direction
labels: [architecture]
---

The package map, as refined by the v1 implementation plan (`docs/plans/2026-10-17 - 00 - photon-v1-plan.md`,
2026-10-17). A later change goes back into this Note. Dependencies point down the table; nothing
imports upward, and there are no cycles.

| package | owns | imports |
|---|---|---|
| `internal/keys` | named keys to bytes, SGR mouse press, motion and release; the pointer event (`Pointer`: press, move or release at a grid cell) and its SGR report | — |
| `internal/take` | the Take: snapshots, cells, colours, typed events, `Box`, beat spans; its versioned file format | — |
| `internal/pixels` | a take's image layer: frames encoded as PNG key frames (the first frame, every 2 s, a new frame size, or changes over half the frame) and patches of one rectangle for each far-apart change, and decoded back at any snapshot | `take` |
| `internal/yamledit` | YAML source splicing: replace, insert or remove one scalar or key, leaving the rest of the bytes as written | — |
| `internal/storyboard` | a clip's Storyboard; strict decode and validation; typed edits (`Apply`) spliced into its source, a new key placed by its yaml struct tags | `keys`, `yamledit` |
| `internal/config` | the project's `photon.yaml` (rig, launch by command or url, services, profiles, end, areas by anchors or selector, theme, fonts) and its lookup; the selector shape check is storyboard's | `keys`, `storyboard` |
| `internal/clip` | a storyboard and its config loaded together, cross-checked; work dir and take path | `storyboard`, `config` |
| `internal/target` | a target's cell box and an area's rows on a snapshot; the only resolver (clicks, drags, camera positions, `seen`) | `take`, `storyboard`, `config` |
| `internal/terminal` | the program in a pty, the vt emulator, sampled snapshots, first paint, settle, quit, the EPERM probe; typed text, named keys and pointer events encoded as xterm bytes and SGR reports (the engine's `Source`); a Windows stand-in that refuses | `take`, `keys` |
| `internal/browser` | the Chromium capture source behind a url launch: finding the browser (`launch.browser` or a fixed list of names and paths, else `ErrNoBrowser`), starting it headless at the url with the clip's viewport and device scale, reading the page's text layer and selector boxes, recording a take from its screencast (a snapshot per frame that changes the picture or the text layer, at most one per frame interval), closing it; typed text, named keys and pointer events dispatched as DevTools input events (the engine's `Source`); drives it through chromedp | `take`, `pixels`, `keys` |
| `internal/engine` | performing a beat's actions; defines the `Source` interface it drives (`Type`, `Key`, `Pointer` with `PointerEvent` = `keys.Pointer`, plus `Current`, `Match`, `Done`, `Event`; pty terminal plus a test fake) and builds no input bytes | `take`, `target`, `storyboard`, `keys` |
| `internal/service` | a take's services: start, readiness, ports, logs, stop; profiles | `config` |
| `internal/check` | judging a take: one PASS/FAIL row per expect | `take`, `target`, `storyboard` |
| `internal/rig` | one take end to end: reset, services, launch, beats, end of take, save, `on_success`, check; `setup` | `clip`, `terminal`, `engine`, `service`, `check`, `take`, `config`, `storyboard`, `target`, `keys` |
| `internal/schedule` | laying the beats onto section durations, holds and cuts | `storyboard`, `take` |
| `internal/fonts` | the embedded faces and their OFL licences, or an override dir | — |
| `internal/raster` | a snapshot's cells to pixels; box drawing drawn, not looked up | `take`, `fonts`, `config` |
| `internal/caption` | a storyboard's captions laid out, timed and drawn over a shipped frame, in output space | `storyboard`, `fonts` |
| `internal/compose` | camera, drawn cursor and captions over each frame | `schedule`, `target`, `take`, `storyboard`, `caption`, `fonts` |
| `internal/render` | the render run: frames through ffmpeg, then gifsicle; the frame pipeline (`Build`) every shipped and previewed frame comes from, drawing a snapshot from the take's image layer when it has one and rasterizing its cells otherwise, reusing an earlier pipeline's rasterizer, raster cache and caption faces where they hold; the font-bound caption check `photon lint` runs (`CheckCaptions`); `render --watch`'s polling loop (`Watch`) | `clip`, `config`, `schedule`, `raster`, `pixels`, `compose`, `caption`, `fonts`, `target`, `storyboard`, `take` |
| `internal/tune` | the timing editor behind `photon tune`: session, typed edits forwarded to storyboard's `Apply`, undo, guarded save, frames on demand, target picks, and the embedded page served on 127.0.0.1; nothing imports it but `cmd/photon`; frames come from render's pipeline, rebuilt on each edit over the last good one | `clip`, `render`, `schedule`, `target`, `storyboard`, `take`, `yamledit`, `config` |
| `internal/testkit` | test support: doc-map check, fixture TUI build, polling | — |
| `cmd/photon` | the Cobra commands and the exit-code mapping only | the packages above |
| `photon` (module root) | `Version()`, embedded from the root `VERSION` file, photon's one version source; `Build()`, the build string `photon version` prints: the version plus the commit and dirty state from Go's build info | — |
| `cmd/release` | the release tool, a self-contained `package main` never shipped in an archive: `dist`, `status`, `prepare`, `publish`, `smoke` | non-test code imports only the standard library and cobra; its tests use `internal/testkit` |

Rules: `cmd/photon` holds no logic beyond flags and wiring. `cmd/release` is a dev tool run as
`go run ./cmd/release <verb>`: no shipped package imports it, it imports no photon package
outside its tests, and its files never land in a release archive. An interface type appears only where
two adapters exist, e.g. the engine's `Source` (the pty terminal plus a test fake); `engine`
never imports `terminal`, and `terminal` never imports `engine` (the pointer event lives in `keys`). A domain package (`storyboard`, `config`, `take`, `clip`) imports no
process, pty or image code; the emulator-cell conversion lives in `terminal`, palette resolution
in `raster`.

The root package exists only because `go:embed` reaches no file above a package's directory and
`VERSION` sits at the module root; it holds `Version()` and `Build()` and nothing else. A release
updates `VERSION` and nothing in Go code.



**Grid surfaces (Note `grid-surfaces`, 2026-11-06).** A snapshot carries a text layer and an
image layer on one grid. `take` owns both layers, text runs, recorded selector boxes and the
cell geometry, and stays free of image code; a new `internal/pixels` owns encoding, decoding and
seeking the image layer's frames; `internal/browser`, the Chromium capture source, comes to
import `take`, `pixels` and `keys` as it records. The engine's `Terminal` interface became `Source` (grid-layers
plan item 9; the pty terminal and the browser are its two adapters, plus a test fake); `engine` never imports
`terminal` or `browser`. The rows above change as each package's imports change.
