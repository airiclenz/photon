---
title: Position lane with transition bars in tune
status: open
type: feature
priority: 2
labels: [tune]
deps: [position-schema]
plan: 2026-10-19 - 03 - camera-positions-plan
created: 2026-10-19T19:44:34Z
---

The tune page's `pan` lane draws one bar per pan move (`internal/tune/page/lanes.js`,
`timeline.js`). With the schema of issue `position-schema`, the lane shows positions and the
transitions between them.

Wanted, in `photon tune`:

- The lane is named **position**. Each position draws as a bar over its hold, or as a point when
  its hold is 0. Each gap between two positions draws as a **transition** bar in its own colour,
  always attached to both neighbours.
- A position or a transition can be selected. The inspector shows a position's `at`, `hold` and
  cell or text target; a transition's start, length and ease (`linear`, `smooth`, `ease-in`,
  `ease-out`).
- Dragging a position along the lane moves it; the transitions beside it stretch or shrink.
- Dragging a transition, or resizing one of its edges, trims only its two neighbours: dragging it
  shortens one neighbour's hold and lengthens the other's, resizing an edge eats into that
  neighbour's hold, and a point neighbour moves with the edge. Nothing further along the lane
  moves. An edit that would make a hold negative or make positions overlap is refused, and the
  bar goes back. A transition may be shrunk to 0 (a cut).
- Dragging a position's edge changes its hold; a point dragged open becomes a bar.
- The lane's `+` adds a point at the playhead, on the cell the camera centres on at that moment,
  so adding one does not move the camera.
- The zoom lane keeps its move bars and gains the four eases in its inspector.

## Acceptance

- The page tests (goja, `internal/tune/pagejs_test.go`) and server tests cover each edit above:
  move, resize hold, drag a transition, resize a transition edge, shrink to a cut, a refused edit
  snapping back, and `+` adding a point on the current centre.
- Each edit is one undoable storyboard edit and is saved back changing only the edited values.
