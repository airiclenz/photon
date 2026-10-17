---
title: Areas declared in config; one target resolver
status: done
type: feature
priority: 2
deps: [storyboard-schema, project-config, take-format]
plan: 2026-10-17 - 00 - photon-v1-plan
created: 2026-10-17T08:54:41Z
closed: 2026-10-17T12:25:04Z
reason: Resolved by plan "2026-10-17 - 00 - photon-v1-plan" (archived in bae9f7a)
---

Clicks, zooms and `seen` expects often need to look in one part of the screen only: a footer,
a status line, the content above an input box. Every app draws that chrome differently, so
photon cannot know it: a project declares its areas in the config.

Wanted: an area is a named band of rows whose top and bottom edges are row anchors: a regex
that picks a row (first or last match, optionally limited to rows above or below another
anchor) plus a row offset. `any` needs no definition. Target resolution — regex over rows
(never spanning rows), `nth` first/N/last, limited to an area — lives in one package,
`internal/target`, used by clicks, zooms and `seen` expects.

## Acceptance

- `internal/target` resolves targets and areas on a snapshot; table tests cover first/N/last,
  no match, `nth` past the matches, and an anchor not on screen, each failing with an error
  naming the pattern and area.
- A test declares three areas — a footer row directly above a bottom hairline row, a status
  row directly below the lowest rule above an input box, and everything above that rule — and
  resolves them on a fixture snapshot.
