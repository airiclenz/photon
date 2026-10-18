---
title: One wire codec for a target's nth
status: open
type: task
priority: 3
labels: [tune, storyboard]
plan: 2026-10-18 - 01 - open-issues-plan
created: 2026-10-18T09:15:40Z
---

A target's nth travels as JSON in two directions, and each side spells it with its own copy. In
`internal/storyboard/edit_beat.go`, `wireTarget` and `wireNth` (with `nthLastWord`) decode it for
`Apply`. In `internal/tune/server.go`, `pickTarget` and `wireNth` (with `nthLast`) encode it for a
pick proposal, and they still carry an `UnmarshalJSON` that is a near-copy of the storyboard one.
If either side changes what it accepts or writes (another nth word, or a different first-match
rule), a pick can propose a target that `Apply` then refuses, and no test spans both copies.
Found closing out plan `2026-10-18 - 00 - architecture-review-plan` (item 21's verifier note).

## Acceptance

- Exactly one type encodes and decodes a target's nth on the wire, owned by `storyboard`. tune's
  pick proposal marshals through it, and the tune copy of `wireNth`/`nthLast` is gone.
- A test round-trips a pick-proposed target (first match, nth 2, last, with and without area)
  through `storyboard.Apply`.
