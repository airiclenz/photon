---
title: "Domain glossary: clip, storyboard, take, beat, target, area and the rest"
labels: [glossary]
---

The terms code, flags, messages and docs use. One meaning each.

- **Clip**: one recorded demo, named by its storyboard's `clip:`.
- **Storyboard**: the YAML file defining a clip: frame, beats, expects. One per clip.
- **Config**: the project's `photon.yaml`, shared by its storyboards: rig, launch, services,
  areas, theme, fonts.
- **Rig**: the work dir a project's `setup` builds: an isolated home, the stage, generated files.
- **Stage**: the state the filmed program works on (a repo, a home dir), reset before each take.
- **Service**: a helper process photon starts for a take and stops after (a fake backend).
- **Take**: one recording, written to `<work>/<clip>.take`: snapshots plus the event log.
- **Snapshot**: the full grid at one moment of a take, with its time: a text layer and an image
  layer on that grid (Note `grid-surfaces`).
- **Text layer**: what a snapshot shows as text with its place on the grid; what targets,
  `wait: screen`, `seen` and areas search. A terminal derives it from its cells; a browser from
  the page's layout, as text runs.
- **Image layer**: what a snapshot shows as pixels; what render and tune draw. A terminal
  derives it from its cells; a browser stores screenshot frames.
- **Text run**: a stretch of a browser page's text with its Box on the grid.
- **Capture source**: what a take is recorded from: the pty terminal, or headless Chromium.
- **Beat**: one step of a storyboard: its actions, section timing and expects.
- **Action**: one thing a beat does: `type`, `key`, `click`, `drag`, `wait`, `pause`.
- **Target**: a place on screen, found by regex, `nth` and area; what a click, drag or camera
  position aims at.
- **Cell**: one cell of the grid, `{x, y}` counted from 0: a terminal character cell, or one CSS
  pixel on a browser take; a fixed place a camera position can aim at instead of a target.
- **Area**: a band of rows a target is searched in, defined in the config by row anchors or, on a
  browser clip, by a CSS selector's recorded box.
- **Expect**: what a keeper take must show, judged PASS or FAIL by `check`.
- **Position**: one place the camera aims at on the clip's clock, a cell or a target, from its
  `at` for its `hold`; a hold of 0 makes it a point in time. A text target is found on the screen
  shown at the position's start.
- **Transition**: the gap between one position's end and the next one's start; not stored. The
  camera glides across it with the later position's ease; a gap of 0 is a hard cut.
- **Zoom**: one change of the camera's magnification on the clip's clock, to a factor from 1 to
  3 over its `over`; it aims nowhere and pushes in on wherever the positions aim.
- **Section**: a beat's stretch of the rendered clip; its `duration` sets the playback speed.
- **Cut beat**: a beat marked `cut: true`; recorded and checked like any beat, it plays no time in
  the clip. A position, zoom or caption anchored at it resolves to where the cut lands: the next
  kept beat's start, or the clip's end after the last kept beat.
- **Render**: a take turned into the shipped GIF.
- **Draft frame**: a frame `photon tune` shows while playing or scrubbing: drawn from a raster at
  the shipped size with a fast scaler, at the exact frame's pixel size with its cursor and
  captions and never quantized; once stopped the preview settles on the exact frame,
  `photon render`'s composed frame quantized to `frame.max_colors` colours, which never draws one.
