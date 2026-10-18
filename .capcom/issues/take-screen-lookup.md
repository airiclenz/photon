---
title: Take owns screen lookup and beat matching
status: done
type: task
priority: 3
labels: [architecture]
plan: 2026-10-18 - 00 - architecture-review-plan
created: 2026-10-18T05:46:01Z
closed: 2026-10-18T08:56:31Z
reason: resolved by plan 2026-10-18 - 00 - architecture-review-plan (archived in 0778002)
---

"Which screen shows at t" is written three ways (`render.SnapshotAt`, `compose`'s `resolveShown`
with a `+time.Nanosecond` workaround in `panPoint`, `check`'s `beatScreens`), and "did the take start
exactly these beats" twice (`schedule.sectionsOf`, `check.matchBeats`) with different error texts.
Architecture review 2026-10-18, finding 7.

Wanted: `take.Take` answers both questions and its callers stop walking snapshots.

## Acceptance

- `render.SnapshotAt`, `resolveShown`'s snapshot walk, `beatScreens`' walk and `check.matchBeats`
  are gone; their callers use `take` methods tested in `internal/take`.
- The `+time.Nanosecond` workaround is gone.
- Schedule and check report a beat mismatch with one text pair.
