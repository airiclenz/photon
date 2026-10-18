---
title: Aim only through the pan track
status: done
type: task
priority: 2
labels: [tune]
plan: 2026-10-18 - 04 - open-issues-plan
created: 2026-10-18T19:29:47Z
closed: 2026-10-18T21:05:12Z
reason: zoom target removed and refused on load, docs updated; resolved by plan "2026-10-18 - 04 - open-issues-plan" (archived at 74103a6)
---

Targets belong on the pan track only. The pan track holds the camera's focus points, and
everything else takes its focus from there: a zoom magnifies around the pan track's centre.
Today a camera `zoom` move can also take a `target`, which plants a second, hidden aim on the
pan track, and the two ways of aiming confuse authors (see `zoom-target-bleed`). The zoom
`target` has not been released.

Ratified 2026-10-18 by the user: remove `target` from camera zoom moves. A storyboard that still
has one fails to load, naming `camera[i]` and saying to aim with a pan move over the same
stretch. In `photon tune`, the zoom inspector's target rows and the camera-zoom branch of
"Set as zoom target" go; "Set as pan target" stays.

## Acceptance

- `storyboard.ZoomMove` has no target. Loading a camera zoom with `target:` fails with an error
  under `camera[i]` that names the replacement, a pan move.
- compose, clip, render and tune carry no zoom-target code path; the tune page offers no way to
  set a target on a zoom.
- README and CHANGELOG describe aiming only through pans.
