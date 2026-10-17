---
title: Embed the raster fonts with their OFL licences
status: done
type: feature
priority: 2
deps: [scaffold]
plan: 2026-10-17 - 00 - photon-v1-plan
created: 2026-10-17T08:54:41Z
closed: 2026-10-17T12:25:04Z
reason: Resolved by plan "2026-10-17 - 00 - photon-v1-plan" (archived in bae9f7a)
---

A clip should render the same on every machine and for every project, without each project
vendoring fonts or depending on what the operator has installed.

Wanted: `internal/fonts` embeds Source Code Pro (regular, bold, italic) and the Noto Sans Math,
Symbols and Symbols 2 fallbacks, with their OFL licence texts. The config's `fonts` dir
overrides them. The README names the fonts and their licence.

## Acceptance

- The raster (issue `render-command`) uses the embedded faces when no override is set; a test
  checks that every face loads and that a box-drawing rune and a math rune resolve.
- The OFL texts are in the repo beside the faces.
