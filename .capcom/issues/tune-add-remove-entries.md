---
title: Add, remove and reorder camera moves and captions in photon tune
status: open
type: feature
priority: 3
labels: [tune]
deps: [camera-moves, clip-captions, typed-storyboard-edits]
plan: 2026-10-18 - 01 - open-issues-plan
created: 2026-10-17T20:17:38Z
---

`photon tune` shows camera moves and captions and edits their fields (plan
`2026-10-17 - 05 - camera-captions-plan`, items 14-18), but cannot add a new one, remove one
or change their order. Doing any of that means leaving the editor for the YAML and reloading,
which breaks the tune loop the editor exists for.

Wanted: in the tune page, add a camera move (zoom or pan) or a caption at the playhead, remove
the selected one, and reorder entries. Each is one undoable edit spliced into the storyboard's
`camera:` / `captions:` list, keeping the rest of the source as written, and refused when the
result fails validation (for example an overlap on a track).

## Acceptance

- Edits exist to insert an entry into `camera:` or `captions:` (creating the list when absent),
  delete one by index and move one to another index; each splices only the list's text and
  undoes cleanly, covered by edit tests on block and flow lists.
- The page offers add (at the playhead), remove and reorder for both lists.
- An insert that makes two moves on one track overlap is refused naming both.

## Log

- 2026-10-18 Airic Lenz: Plan 2026-10-18 - 01 - open-issues-plan (70f28a8), items 8-9: storyboard insert/remove/move edits for camera and captions (overlap refused naming both) and page add/remove/reorder landed; left open: a refused add (e.g. an overlap) shows only as app.js's refusal on the entry index it named, so it is invisible unless that entry is in the inspector.
