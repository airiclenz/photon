---
title: Allow camera moves inside a cut beat
status: open
type: task
priority: 2
labels: [render]
created: 2026-11-06T07:11:17Z
---

Camera positions and zooms cannot be anchored at a cut beat (`cut: true`): an `at` or `until`
naming one is refused on load. A camera move that starts inside a cut beat is therefore
impossible.

Shelved as out of scope by `docs/plans/archived/2026-10-17 - 05 - camera-captions-plan.md`.
Decide first what a camera move inside a beat that plays no time should mean.

## Acceptance

- The intended meaning is written down (README and glossary).
- Storyboards using it load, render, and show in `photon tune` consistently with that meaning.
