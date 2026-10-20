---
title: Transition grips cover a point marker in photon tune
status: open
type: bug
priority: 3
labels: [tune]
plan: 2026-10-20 - 00 - tune-point-grip-overlap-plan
created: 2026-10-20T10:57:41Z
---

In a real browser, a transition's edge grip covers the point marker the transition leads into.
A transition's grips are 9px wide, centred on its edge and raised with `z-index: 1`
(`internal/tune/page/style.css:548-557`); only a bar whose start equals its end gets the
`beside-start`/`beside-end` offset (`internal/tune/page/timeline.js:290-291`), so the grip at the
transition's end sits over the 10px point marker centred on the same moment. A press on that
marker therefore drags the transition's end and sends `position.transition` with the transition
selected, not `position.at` with the position selected. The storyboard result is equivalent, but
the point cannot be selected or moved as a position from its marker. The page tests run in goja,
which does not hit-test, so none catches it. Found closing out plan
`2026-10-19 - 03 - camera-positions-plan` (items 15-16).

## Acceptance

- In a browser, a press on a point marker that a transition leads into (or out of) selects that
  position and a drag sends `position.at`; a transition has no grip on an edge that meets a
  point (ratified 2026-10-20: the marker's drag moves that edge), and every other grip stays.
