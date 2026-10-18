---
title: Show photon's version and commit in photon tune
status: open
type: task
priority: 3
labels: [tune]
plan: 2026-10-18 - 02 - tune-polish-plan
created: 2026-10-18T14:21:41Z
---

`photon tune` doesn't show which build is serving the page. VERSION reads the same for every
build, so a stale binary looks like a current one: a test session on 2026-10-18 reported a
missing pan lane and font-size control, both of which `main` has, because the Mac ran an older
build.

## Acceptance

- The tune page shows photon's version and the commit it was built from, marked dirty when the
  tree had uncommitted changes (for example `photon 0.1.0 (93ed3c1, dirty)`), read from Go's build
  info.
- `photon version` prints the same string.
- A build without VCS info (for example `go run` outside a checkout) still shows the version
  alone.
