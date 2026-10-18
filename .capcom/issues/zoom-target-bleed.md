---
title: A camera zoom first aims at the previous zoom's target
status: open
type: bug
priority: 2
labels: [tune]
created: 2026-10-18T19:02:14Z
---

After setting the target of the selected camera zoom in `photon tune` (Set as zoom target, or the
inspector's target field), that zoom can start by zooming towards the target of the camera zoom
before it (zoom N-1). Partway through the move it jumps to the target that was actually set.
The camera is meant to move smoothly to one target. The jump is visible in the clip.

Reported by the user on 2026-10-18 and not yet reproduced. It is unknown whether only the tune
preview shows it or `photon render` does too.

Leads, unverified:
- `internal/compose/camera.go`: `planMoves` appends a targeted zoom's aim (`planZoomAim`) to the
  pan track. If zoom N-1's aim is still running when zoom N starts, or the two aims are ordered
  wrongly on the track, `trackAt` would pick the wrong one.
- The tune page sends the target edit from `entries.js` and `picker.js` (commit `8c4e3f2`). An
  edit that lands on the wrong camera index, or a stale cached preview frame, would look the same.

## Acceptance

- A storyboard with two consecutive targeted camera zooms on different targets renders zoom N
  aiming from where the camera is straight to its own target, with no jump, both in `photon render`
  and in the `photon tune` preview after setting zoom N's target there.
- A `internal/compose` test pins the aim path of the second zoom (no sample heads for the first
  zoom's target), and fails on the tree before the fix.
