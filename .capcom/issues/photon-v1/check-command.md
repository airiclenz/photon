---
title: "photon check: seen and run expects"
status: open
type: feature
priority: 2
deps: [take-format, declarative-areas]
plan: 2026-10-17 - 00 - photon-v1-plan
created: 2026-10-17T08:54:41Z
---

A take must be judged before it ships, and re-judged by hand after a render tweak.

Wanted: one `beat N | PASS/FAIL | detail` row per expect, then the take-level expects, and
exit 1 on any FAIL. Expect kinds:
- `seen`: the regex matches at least one screen recorded inside the beat (areas allowed).
- `run`: a command run in the rig's work dir with `PHOTON_WORK`, `PHOTON_TAKE` and the beat id
  in its env; exit 0 passes, and its trimmed output is the detail. This is how a project checks
  anything photon cannot see: files the app wrote, a repo left dirty, the app's own records
  (Note `extension-model`).

`photon check <storyboard> [<take>]` reads `<work>/<clip>.take` by default.

## Acceptance

- `internal/check` judges both kinds; table tests cover pass, fail and a `run` command that
  cannot start.
- `photon check` prints the table and exits 0 or 1 accordingly.
