---
title: Generic storyboard schema, strict loader and photon lint
status: open
type: feature
priority: 2
deps: [scaffold]
plan: 2026-10-17 - 00 - photon-v1-plan
created: 2026-10-17T08:54:41Z
---

A clip is defined by its storyboard: what photon performs, how long each part plays in the
clip, where it zooms, and what a keeper take must show. photon needs the schema and a strict
loader.

The storyboard holds:
- header: `clip`, `ship` (output path), `frame` (cols, rows, padding, font size, line height,
  scale, width, fps, max colours). Paths resolve relative to the storyboard file.
- `beats`: id (positive, unique, ascending), title, why, notes, `do` actions (`type`, `key`,
  `click`, `wait`, `pause`), section timing (`duration`, `hold`, `cut`), optional `zoom`
  (target and factor), `expect`s.
- targets `{text, nth, area}`: `text` a regex, `nth` first/N/last, `area` an area from the
  config (issue `declarative-areas`) or `any`.
- expects: `seen` (a regex over the beat's screens) and `run` (a command whose exit status is
  the verdict); take-level expects take the same forms.

Decoding is strict: an unknown field is an error, and validation reports every problem at
once, not just the first. `photon lint <storyboard>` loads the storyboard and its config and
prints `ok` or every problem, exiting 1.

## Acceptance

- `internal/storyboard` loads and validates the schema above; table tests cover every
  validation rule and an unknown field.
- `photon lint` on a valid and an invalid fixture exits 0 and 1.
- A fixture storyboard using every action, target form, zoom, cut and expect kind lints clean.
