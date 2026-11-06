---
title: Edit cols, rows and max_colors in tune
status: open
type: task
priority: 2
labels: [tune]
plan: 2026-11-06 - 01 - tune-editing-plan
created: 2026-11-06T07:11:17Z
---

`photon tune` cannot edit the clip's `cols`, `rows` or `max_colors`; they must be changed in the
YAML and the page reloaded. They shape every frame, so tuning them beside the preview would
save round trips.

Shelved as out of scope by `docs/plans/archived/2026-10-18 - 02 - tune-polish-plan.md`.

## Acceptance

- The tune page shows `cols`, `rows` and `max_colors` as editable values and writes changes back
  to the YAML.
- The preview reflects a change without restarting `photon tune`.
- Out-of-range values are refused with the same rules the loader applies.
