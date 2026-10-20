---
title: Drag a position's cell on the tune preview
status: done
type: feature
priority: 2
labels: [tune]
deps: [tune-position-lane]
plan: 2026-10-19 - 03 - camera-positions-plan
created: 2026-10-19T19:44:34Z
closed: 2026-10-20T10:58:42Z
reason: "Resolved by camera-positions-plan (c433437): preview marker drag sets a position's cell (item 18)"
---

A position's cell can only be typed in as numbers today. Aiming the camera should happen on the
preview itself.

Wanted, in `photon tune`: when a position is selected on the position lane (issue
`tune-position-lane`), the preview jumps to the position's start and shows the full, unzoomed
frame with the zoom's crop at that moment drawn as a rectangle and the position's cell as a
marker. Dragging the marker moves it cell by cell over any cell, blank or not; dropping it sets
the position to that cell (replacing a text target). Selecting anything else returns the preview
to the normal zoomed view.

## Acceptance

- Selecting a position shows the unzoomed frame, the crop rectangle and the marker on the
  position's cell; for a text target, the marker sits on the resolved cell.
- Dropping the marker sends one edit setting the position's `cell`; the crop rectangle follows,
  clamped as the render clamps it.
- Page tests cover the marker landing on a blank cell and the drop edit.
