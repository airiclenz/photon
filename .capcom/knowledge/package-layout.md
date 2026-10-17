---
title: Package map and dependency direction
labels: [architecture]
---

The proposed package map. The implementation plan may refine it; a change goes back into this
Note. Dependencies point down the table; nothing imports upward, and there are no cycles.

| package | owns | imports |
|---|---|---|
| `internal/storyboard` | a clip's Storyboard (frame, beats, actions, targets, expects, section timing); strict decode and validation | — |
| `internal/config` | the project's `photon.yaml` (rig, launch, services, areas, theme, fonts); strict decode and validation | — |
| `internal/take` | the Take: snapshots of the cell grid with their times, the event log; its file format | — |
| `internal/target` | locating a target's cell box and an area's rows on a snapshot; used by clicks, zooms and `seen` checks | `take`, `storyboard`, `config` |
| `internal/terminal` | the program in a pty, the vt emulator reading it, sampled snapshots, first paint, settle, quit; a Windows stand-in that refuses | `take` |
| `internal/engine` | performing a beat's actions: humanized typing, key and mouse sequences, waits, pauses | `terminal`, `target`, `storyboard` |
| `internal/rig` | one take end to end: reset, services, launch, beats, end of take, save | `engine`, `terminal`, `config`, `storyboard`, `take` |
| `internal/check` | judging a take: one PASS/FAIL row per expect | `take`, `target`, `storyboard` |
| `internal/schedule` | laying the beats onto section durations, holds and cuts | `storyboard`, `take` |
| `internal/fonts` | the embedded faces and their OFL licences | — |
| `internal/raster` | a snapshot's cells to pixels, box drawing drawn not looked up | `take`, `fonts`, `config` |
| `internal/compose` | zoom and drawn cursor over each frame | `schedule`, `target`, `take`, `storyboard` |
| `internal/render` | the render run: frames through ffmpeg, then gifsicle | `schedule`, `raster`, `compose` |
| `cmd/photon` | the Cobra commands and the exit-code mapping only | the packages above |

Rules: `cmd/photon` holds no logic beyond flags and wiring. An interface type appears only where
two adapters exist, e.g. the engine's view of the terminal (the pty terminal plus a test fake).
A domain package (`storyboard`, `config`, `take`) imports no process, pty or image code.
