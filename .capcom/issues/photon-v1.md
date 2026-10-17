---
title: "photon v1: record, check and render a demo clip for any project"
status: done
type: feature
priority: 1
plan: 2026-10-17 - 00 - photon-v1-plan
created: 2026-10-17T08:54:41Z
closed: 2026-10-17T12:25:05Z
reason: Resolved by plan "2026-10-17 - 00 - photon-v1-plan" (archived in bae9f7a)
---

photon v1: a standalone CLI that records a terminal app's demo clip from a storyboard, checks
the take and renders the shipped GIF, for any project, in any language.

The design is set by Note `extension-model` (one generic CLI, hooks through commands and
services) and Note `package-layout` (internal packages, dependency direction).

## Acceptance

- Every child issue is closed.
- A project with no Go code can film its TUI using only a `photon.yaml`, a storyboard and the
  `photon` binary: `photon lint`, `photon record`, `photon check` and `photon render` work end to
  end.
