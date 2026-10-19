---
title: Picker sets a position's text target
status: open
type: feature
priority: 2
labels: [tune]
deps: [tune-position-lane]
created: 2026-10-19T19:44:34Z
---

The target picker (`internal/tune/page/picker.js`, `internal/tune/pick.go`) offers "Set as pan
target", which sets a pan move's `to`. Pan moves are gone (issue `position-schema`).

Wanted: the button becomes **Set as position target** and sets the selected position to the
proposed text target. The picker keeps proposing text targets only; setting a cell is done with
the preview marker (issue `tune-position-marker`). Blank-cell picking stays a camera-only feature:
action targets (clicks, drags) are unchanged.

## Acceptance

- With a position selected, the button sets its text target in one undoable edit; without one it
  is disabled or refused with a message.
- Tests cover the edit and the refusal.
