---
title: Publish releases to winget
status: done
type: task
priority: 2
labels: [release]
plan: 2026-11-06 - 02 - remaining-issues-plan
created: 2026-11-06T07:11:17Z
closed: 2026-11-06T12:41:41Z
reason: Release tool submits winget manifests via komac; docs/releasing.md covers setup, resolved by plan 2026-11-06 - 02 - remaining-issues-plan (archived in 54788b7)
---

Releases ship through GitHub, Homebrew and Scoop. Windows users on winget cannot install photon.

Shelved as out of scope by `docs/plans/archived/2026-10-17 - 03 - release-tooling-plan.md`.

## Acceptance

- The release tool publishes a winget manifest for each release.
- `docs/releasing.md` covers the one-time setup.
- A published release installs with `winget install`.
