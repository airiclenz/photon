---
title: Speed up tune start-up
status: open
type: task
priority: 2
labels: [tune]
plan: 2026-11-06 - 00 - authoring-loop-plan
created: 2026-11-06T07:11:17Z
---

`photon tune` takes noticeable time to start because it decodes the take's JSON up front.

Shelved as out of scope by `docs/plans/archived/2026-10-18 - 03 - tune-playback-plan.md`.

## Acceptance

- Start-up time on a large take is measured before and after, and the change shows a clear gain.
- The loaded take is identical to what the current decoder produces.
