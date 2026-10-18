---
title: Resolve storyboard values once, at Parse
status: done
type: task
priority: 3
labels: [architecture]
plan: 2026-10-18 - 00 - architecture-review-plan
created: 2026-10-18T05:46:01Z
closed: 2026-10-18T08:56:31Z
reason: resolved by plan 2026-10-18 - 00 - architecture-review-plan (archived in 0778002)
---

Storyboard validation resolves moments, checks hex colours and bounds, then discards the results;
`caption` (`span`, `parseColor`), `compose` (`planMoves`, `planPan`), `schedule` (`checkTiming`) and
tune (`clipMS`) resolve again behind error branches that cannot fire. Checks that need only fonts and
config (caption box overflow) never run in `photon lint`. A storyboard with every beat cut
validates and fails only at render. Architecture review 2026-10-18, finding 6.

Wanted: Parse hands out resolved values; the three tiers (static, font-bound, take-bound) get names;
`photon lint` always runs the font-bound tier; an all-cut storyboard is a Parse error.

## Acceptance

- Consumers read resolved starts, stops and colours and have no timing or colour error returns.
- `photon lint` reports a caption box larger than the frame, with a test.
- `photon lint` reports an all-cut storyboard.
