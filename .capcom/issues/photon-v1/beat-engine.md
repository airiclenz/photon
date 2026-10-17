---
title: "Perform beat actions: typing, keys, clicks, waits"
status: open
type: feature
priority: 2
deps: [pty-terminal, declarative-areas]
plan: 2026-10-17 - 00 - photon-v1-plan
created: 2026-10-17T08:54:41Z
---

A beat's actions must play the same on every machine and every take, so a retake differs only
where the app does. Wanted, in `internal/engine`:
- `type`: humanized typing from a fixed, seeded profile — a short gap after a letter, longer
  after a space, longer still after punctuation, and an occasional thinking pause after a
  space, all from a seeded MINSTD generator — or flat typing.
- `key`: named keys (`enter`, `esc`, `tab`, `shift+tab`, `space`, `backspace`, `ctrl+c`, the
  arrows, `pgup`, `pgdown`, `alt+up`, `alt+down`, `f1`–`f12`) to their byte sequences, with
  `repeat`.
- `click`: resolve the target (issue `declarative-areas`) and send a real SGR mouse press and
  release at its centre cell, `times` times; the resolved cell goes into the event log, where
  the render's cursor reads it.
- `wait`: until a regex matches the screen, or with `gone` no longer does, within a required
  timeout of at most 180 s.
- `pause`.

A failing action ends the beats with an error naming the beat and action.

## Acceptance

- `internal/engine` drives the fixture TUI through every action form; tests check the bytes
  sent and the event log.
- A test pins the typing gaps generated for a fixed seed.
- A wait timeout and a missing click target each fail with the beat id named.
