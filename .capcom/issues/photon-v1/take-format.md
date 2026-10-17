---
title: Take type and versioned take file format
status: done
type: feature
priority: 2
deps: [scaffold]
plan: 2026-10-17 - 00 - photon-v1-plan
created: 2026-10-17T08:54:41Z
closed: 2026-10-17T12:25:04Z
reason: Resolved by plan "2026-10-17 - 00 - photon-v1-plan" (archived in bae9f7a)
---

A take is the recording every later step reads — check and render never go back to the app —
so its type and file format come first. A Take holds full-grid snapshots stamped on the take's
clock (each cell's rune, width, colours and attributes, plus the cursor) and an event log of
what photon did (beat starts, typed strings, keys, clicks with the cell each target resolved
to).

The file is gzip-compressed JSON lines with a version header naming photon's take format. A
newer or foreign file is refused with a clear error.

## Acceptance

- `internal/take` writes and reads a take losslessly; a round-trip test covers every cell
  attribute and event kind.
- A wrong header and a truncated file each fail with an error naming the file.
