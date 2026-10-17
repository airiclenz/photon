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
- **Snapshot**: the full cell grid at one moment of a take, with its time.
- **Beat**: one step of a storyboard: its actions, section timing, zoom and expects.
- **Action**: one thing a beat does: `type`, `key`, `click`, `wait`, `pause`.
- **Target**: a place on screen, found by regex, `nth` and area; what a click or zoom aims at.
- **Area**: a band of rows a target is searched in, defined in the config by row anchors.
- **Expect**: what a keeper take must show, judged PASS or FAIL by `check`.
- **Section**: a beat's stretch of the rendered clip; its `duration` sets the playback speed.
- **Render**: a take turned into the shipped GIF.
