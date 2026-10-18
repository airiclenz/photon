---
title: Remove beat zoom
status: done
type: task
priority: 2
labels: [tune]
plan: 2026-10-18 - 04 - open-issues-plan
created: 2026-10-18T19:29:47Z
closed: 2026-10-18T21:05:12Z
reason: beat zoom removed and refused on load, CHANGELOG notes the break; resolved by plan "2026-10-18 - 04 - open-issues-plan" (archived at 74103a6)
---

A beat's `zoom` (`beats[i].zoom`) duplicates the camera's zoom and pan moves. It is a second
way to zoom, with its own lane, panel and edits in `photon tune`. Two zoom mechanisms are
redundant and confusing; the camera list covers the use.

Ratified 2026-10-18 by the user: remove beat zoom entirely. It shipped in v0.1.0, so this is a
breaking change: a storyboard with a beat `zoom:` fails to load, naming the beat and saying
to use camera zoom and pan moves. Its beat-only features go with it: the auto-fit factor,
zooming onto a drag's logged box, and ramps tied to the beat's section. The CHANGELOG records
the break.

## Acceptance

- `storyboard.Beat` has no zoom. Loading a beat with `zoom:` fails with an error under
  the beat (`beat <id>`) that names the replacement.
- compose plans no beat-zoom sections; tune has no zoom lane, zoom panel or zoom edits; the
  "Set as zoom target" button is gone.
- README, glossary and command help text describe zooming only through camera moves; the
  CHANGELOG notes the breaking change.
