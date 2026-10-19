---
title: Aim the camera with positions and transitions
status: open
type: feature
priority: 2
labels: [storyboard, tune]
created: 2026-10-19T19:44:34Z
---

The camera's `pan` track is hard to read and to shape. A pan bar on the timeline is a move: the
camera glides from wherever it is aimed to the bar's `to` over the bar's length, and holds in the
gaps. To aim the camera you think in moves, not in the places it should look, and a destination
can only be a text target found on screen or a cell typed in by number. The picker refuses blank
cells, so the camera cannot be aimed at an empty part of the screen by picking.

Wanted: the camera is aimed by **positions** placed on a timeline lane. A position is a place the
camera rests on (a cell or a text target) for a hold of zero or more; the gap between two
positions is a **transition**, the eased glide from one to the next, shown and edited as a bar of
its own. Zoom stays a separate lane of factor-only moves, always centred on where the position
lane aims at that moment. The `camera:` list is unreleased, so the new shape replaces the `pan`
and `zoom` moves outright, with no compatibility layer.

The work is split across the child issues: the storyboard schema and render, the tune lane, the
preview marker, and the picker.

## Acceptance

- Every child issue is done.
- A storyboard aims the camera with `camera.positions` and zooms with `camera.zooms` only; `pan`
  moves and `via` paths are gone from the schema, the README and the glossary.
