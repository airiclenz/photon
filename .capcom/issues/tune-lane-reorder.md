---
title: Reorder lane entries by dragging in tune
status: open
type: task
priority: 2
labels: [tune]
created: 2026-11-06T07:11:17Z
---

In `photon tune`, entries on a timeline lane can be dragged along the clock but not reordered
against each other. Changing the order means editing the storyboard YAML by hand.

Shelved as out of scope by `docs/plans/archived/2026-10-18 - 01 - open-issues-plan.md`.

## Acceptance

- Dragging an entry past its neighbour on a lane reorders them, and the page writes the new order
  back to the storyboard.
- A reorder that would leave the storyboard invalid is refused with a message, and the file is
  left unchanged.
