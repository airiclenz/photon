---
title: A zoom-in first heads for a target the camera left earlier
status: done
type: bug
priority: 2
labels: [tune]
plan: 2026-10-18 - 04 - open-issues-plan
created: 2026-10-18T19:02:14Z
closed: 2026-10-18T21:05:12Z
reason: pan paths start from the visible crop centre; resolved by plan "2026-10-18 - 04 - open-issues-plan" (archived at 74103a6)
---

A zoom that aims at a target can start by heading for an older target and switch to its own
partway through the move. Seen in `photon tune`: zoom 0 zooms in on target A, zoom 1 zooms
back out to 1×, then zoom 2, aiming at B, first zooms towards A and only then turns to B.

Cause: `internal/compose/camera.go` `newCamera` starts each pan path from the previous pan's
`To`, here A. At 1× the crop shows the whole frame, so `(*Compositor).crop` never reads the
centre and A stays hidden. When zoom 2 raises the factor, the crop's centre follows the path
from A to B, and the old target shows through.

Ratified 2026-10-18 by the user: a pan path starts from the crop centre visible at its start.
That is the frame centre at 1×, otherwise the centre clamped as the crop clamps it. A pan at 1×
still sets where the next zoom aims.

## Acceptance

- Zoom in with a pan to A, zoom out to 1×, then zoom in with a pan to B: during the last move
  the crop's centre runs from the frame centre to B, and no sample moves towards A.
- A pan that starts while the crop is clamped against a frame edge starts from the clamped
  centre: the crop does not jump at the pan's first frame.
- `internal/compose` tests pin both cases and fail on the tree before the fix.
