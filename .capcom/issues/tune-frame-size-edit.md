---
title: Edit cols, rows and max_colors in tune
status: done
type: task
priority: 2
labels: [tune]
plan: 2026-11-06 - 01 - tune-editing-plan
created: 2026-11-06T07:11:17Z
closed: 2026-11-06T10:52:28Z
reason: "Resolved by plan 2026-11-06 - 01 - tune-editing-plan (archived 5f51cdf): items 1-5 make cols, rows and max_colors edit fields with loader rules, preview at the take's size with a re-record problem, and edit them in the clip panel."
---

`photon tune` cannot edit the clip's `cols`, `rows` or `max_colors`; they must be changed in the
YAML and the page reloaded. They shape every frame, so tuning them beside the preview would
save round trips.

Shelved as out of scope by `docs/plans/archived/2026-10-18 - 02 - tune-polish-plan.md`.

## Acceptance

- The tune page shows `cols`, `rows` and `max_colors` as editable values and writes changes back
  to the YAML.
- The preview reflects a change without restarting `photon tune`: cols/rows changes preview at
  the take's size with a re-record problem (ratified 2026-11-06).
- Out-of-range values are refused with the same rules the loader applies.
