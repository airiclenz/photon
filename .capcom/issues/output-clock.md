---
title: One output clock for zooms, moves and captions
status: open
type: task
priority: 2
labels: [architecture]
plan: 2026-10-18 - 00 - architecture-review-plan
created: 2026-10-18T05:46:01Z
---

Output time is summed in four places in two orders: `Storyboard.ClipTime`/`Length` and
`beatZoomSpans` in storyboard order, `schedule.New`'s `OutStart` in take-start order. Beat zooms use
one order, camera moves and captions the other; they agree only because the rig runs beats in
storyboard order. Nothing rejects a take whose beats ran in another order, so zooms and captions
would silently drift apart. Architecture review 2026-10-18, finding 2.

Wanted: one output clock derived from the beats that storyboard, schedule, compose and caption all
read; a take whose beat order differs from the storyboard's is a hard error ("re-record it") in
`schedule.New` and `photon check`.

## Acceptance

- Output time is summed in exactly one place.
- A take started in another beat order is refused by schedule and by check, with a test each.
