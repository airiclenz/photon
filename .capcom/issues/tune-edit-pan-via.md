---
title: Edit a pan's via waypoints in photon tune
status: open
type: feature
priority: 3
labels: [tune]
deps: [camera-moves, typed-storyboard-edits]
created: 2026-10-17T20:17:38Z
---

`photon tune` edits a pan's single `to` point, set from a pick, but refuses a pan that moves
along `via` waypoints (plan `2026-10-17 - 05 - camera-captions-plan`, item 15). Shaping a curved
pan means editing the YAML by hand and reloading to see the path.

Wanted: in the tune page, for a selected `via` pan, set each waypoint from a pick or as a cell
point, add or remove waypoints, and turn a `to` pan into a `via` pan and back. The preview shows
the curved path the camera's centre follows.

## Acceptance

- Edits exist to set, insert and remove a waypoint of `camera[i].pan.via`, and to switch a pan
  between `to` and `via`; each undoes cleanly and is refused when the result fails validation
  (fewer than two waypoints for `via`).
- The page sets a waypoint from a pick, as it does a pan's `to`.
- The timeline or frame preview draws the pan's path through its waypoints.
