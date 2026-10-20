---
title: Camera positions and zooms in the storyboard and render
status: done
type: feature
priority: 2
labels: [storyboard]
plan: 2026-10-19 - 03 - camera-positions-plan
created: 2026-10-19T19:44:34Z
closed: 2026-10-20T10:58:42Z
reason: "Resolved by camera-positions-plan (c433437): positions/zooms schema, lint, render, docs (items 4-12)"
---

The storyboard's camera is a list of `zoom` and `pan` moves (`internal/storyboard/camera.go`).
A pan is a glide to a destination over `over`; the place the camera looks at is implied by the
end of the last move. The parent issue `camera-positions` replaces that with positions and
transitions.

Wanted, in the storyboard schema, lint and the render (`internal/storyboard`, `internal/compose`):

- `camera:` becomes a mapping with two lists, `positions` and `zooms`, both optional.
- A **position** has `at` (a clip time or `{beat, offset}`, as moves take today), `hold` (a
  duration of 0 or more; 0 is a point), and exactly one of `cell: {x, y}` (any cell inside
  `frame.cols`/`rows`, blank or not) or a text target (`text`, `nth`, `area`). It may carry `ease`,
  the ease of the transition that leads into it.
- A **transition** is the gap between two consecutive positions and is not stored. The camera
  rests on a position for its hold, then glides to the next one across the gap with the next
  position's ease. A gap of 0 is a hard cut.
- Before the first position the camera holds the first one; after the last it holds the last one.
  With no positions the camera centres on the frame, as today at rest.
- A text target is resolved on the screen shown at the position's start; the camera stays on that
  spot for the whole hold and does not follow the text. Not on screen there fails the render under
  that position.
- A **zoom** keeps today's move shape (`to` a factor from 1 to 3, `at`, `over`, `ease`) and has no
  target or position; it centres on where the positions aim at that moment.
- `ease` is one of `linear`, `smooth` (the default), `ease-in` or `ease-out`, for transitions and
  zoom moves alike.
- The camera's centre is clamped so the crop stays inside the frame, as today.
- Lint refuses overlapping positions (half-open spans), a negative hold, a position or zoom that
  ends after the clip, an unknown ease, and a cell outside the grid. The old `pan` move, `via` and
  the list form of `camera:` fail with the ordinary schema errors; no special message.
- The tune session and its edits follow the new shape far enough that `photon tune` still loads
  and saves a storyboard with positions (the lane UI is issue `tune-position-lane`).
- The README camera section, the glossary (entries for position, transition, zoom and cell) and
  the CHANGELOG describe the new shape; `pan` and `via` are gone from them.

## Acceptance

- A storyboard with positions (cells, text targets, points, held bars, a zero-length gap) and
  zooms renders, and tests pin the camera centre at hold, mid-transition for each ease, at a cut,
  and before the first and after the last position.
- Each lint refusal above has a test naming the offending entry.
- `photon render` and `photon tune` load the new shape; a storyboard with a `pan` move fails to
  load.
- README, glossary and CHANGELOG are updated as above.
