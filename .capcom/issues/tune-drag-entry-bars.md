---
title: Drag camera moves and captions on the tune timeline
status: done
type: task
priority: 2
labels: [tune]
plan: 2026-10-18 - 02 - tune-polish-plan
created: 2026-10-18T14:21:41Z
closed: 2026-10-18T16:38:38Z
reason: Resolved by plan 2026-10-18 - 02 - tune-polish-plan (1cba598)
---

A camera move's or caption's bar on the `photon tune` timeline can be clicked to select it, but not
dragged (`internal/tune/page/timeline.js`, `barNode`). Shifting a zoom, pan or caption in time
means typing its `at` moment in the inspector. A beat's own zoom (the "zoom" lane) belongs to its
beat by design, so moving a zoom freely means using a camera zoom move, and that should be as easy
to move as a beat edge is to drag.

## Acceptance

- Dragging a bar in the "camera zoom", "pan" or "captions" lane shifts the entry's start and keeps
  its length, snapping like the beat handles do, and sends one edit when the drag ends.
- A refused drag (for example an overlap) puts the bar back and shows the refusal.
- A page test drives a bar drag and asserts the edit sent.
