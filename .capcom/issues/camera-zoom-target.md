---
title: Give a camera zoom move a target
status: done
type: task
priority: 2
labels: [tune]
plan: 2026-10-18 - 02 - tune-polish-plan
created: 2026-10-18T14:38:11Z
closed: 2026-10-18T16:38:39Z
reason: Resolved by plan 2026-10-18 - 02 - tune-polish-plan (1cba598)
---

A camera zoom move (`storyboard.ZoomMove`, `internal/storyboard/camera.go`) has only a factor
(`to`). Where the camera looks while it zooms comes from the pan track, so a camera zoom can't
aim at anything. In `photon tune`, "Set as zoom target" works only on a beat's own zoom
(`internal/tune/page/picker.js`, `setAsZoomTarget`). With a camera zoom selected it says "Select a
beat to zoom first.", which is misleading.

A beat's zoom already aims at a target: compose plans its pan toward the zoom box's centre
(`internal/compose/zoom.go`). A camera zoom should be able to do the same.

## Acceptance

- A camera zoom move takes an optional `target` (the same target shape a pan's `to` takes, nth and
  area included). With one set, the zoom aims at that target's box, and compose plans the pan track
  for it the way it does for a beat zoom. Without one it behaves as it does today.
- Validation refuses a targeted zoom that overlaps a pan move on the pan track, with a message
  naming both moves. A target that is missing at the move's end resolves the way a pan target does.
- A `camera.target` edit field sets or removes it, and tune's state carries it.
- In `photon tune`, "Set as zoom target" with a camera zoom selected sets that move's target. With
  nothing zoomable selected, the message names both choices: a beat or a camera zoom.
- The camera zoom inspector shows the target rows (as a pan's `to` does), and a page test covers
  setting it from a pick.
- The storyboard reference docs describe the new key.
