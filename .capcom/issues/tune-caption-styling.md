---
title: Edit a caption's font, colours, box, fade and position in photon tune
status: open
type: feature
priority: 3
labels: [tune]
deps: [clip-captions]
created: 2026-10-17T20:17:38Z
---

`photon tune` edits a caption's timing, text, `place` and `align` (plan
`2026-10-17 - 05 - camera-captions-plan`, item 16), but not its look: font size and weight,
text colour, background box colour and opacity, fade in and out, offset, or explicit position.
Getting a caption readable over a busy screen is a by-eye job that wants the live preview.

Wanted: in the tune page's caption inspector, edit `font.size`, `font.weight`, `color`,
`box.color`, `box.opacity`, `fade.in`, `fade.out`, `offset` and `position`, each an undoable
edit spliced into the storyboard and refused when the result fails validation. A value equal to
its default removes the key, as other tune edits do.

## Acceptance

- Edits exist for each field above, covered by edit tests on block and flow captions, including
  switching between `place`/`align`/`offset` and `position`.
- The page shows a control for each (number, select, colour picker, opacity slider) and the
  preview frame updates after each edit.
