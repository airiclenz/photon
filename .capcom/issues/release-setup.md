---
title: Versioning, changelog and release binaries
status: done
type: chore
priority: 3
deps: [user-docs]
plan: 2026-10-17 - 03 - release-tooling-plan
created: 2026-10-17T08:54:41Z
closed: 2026-10-17T19:54:29Z
reason: "v0.1.0 is cut and published: GitHub release with notes and binaries for Linux, macOS and Windows (amd64, arm64), plus Homebrew and Scoop. The pinned-install goal is met at v0.1.0 rather than v1.0.0."
---

Once v1 works, photon needs a way to be installed at a pinned version by other projects.

Wanted: a `VERSION` file and `photon version`, semantic-version tags, a CHANGELOG, and GitHub
releases with binaries for Linux, macOS and Windows. Whether it joins the Homebrew tap is
decided then.

## Acceptance

- `go install github.com/airiclenz/photon/cmd/photon@v1.0.0` works, and `photon version` prints
  `1.0.0`.
- A release has notes and binaries for the three OSes.

## Log

- 2026-10-17 Airic Lenz: VERSION at the repo root is the one version source, embedded by the root package's Version(); photon version and --version print it. A release updates VERSION only.
