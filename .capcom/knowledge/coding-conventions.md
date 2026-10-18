---
title: "Before writing code: house conventions from coding-standards"
labels: [conventions]
---

photon follows the `coding-standards` skill (base rules plus the Go extension). The points that
shape this repository from its first commit:

- **Layout.** `cmd/photon` holds the Cobra commands and nothing else: parse flags, call one
  `internal/` package, map the error to an exit code. Every other package is under `internal/`
  (Note `extension-model`: photon exports no Go API), but for the root package that embeds
  `VERSION` and `cmd/release`, the release tool: a self-contained `package main`, never shipped
  in an archive, whose non-test code imports only the standard library and cobra (its tests use
  `internal/testkit`). One package per directory, named by what it owns, short and lowercase. The package
  map and dependency direction are Note `package-layout`.
- **Formatting.** `gofmt`/`goimports` only, no divider comments. `golangci-lint` runs in CI with
  the config committed at the repo root. Lines wrap at 100 columns (a tab counts as 4), enforced
  by its `lll` linter for non-test Go files; `_test.go` files are exempt.
- **Deep modules.** A package exposes a small interface over substantial behaviour. An interface
  type exists only where two adapters exist (production plus test fake counts). A helper used by
  one caller stays in that caller.
- **Doc maps.** Every non-trivial package has a `doc.go`; once a package passes ~10 files its
  `doc.go` maps every non-test file in a half line, and a structural test fails when a file is
  missing from the map.
- **Files.** One primary type per file, file named after it, under ~400 lines. A command's
  helpers sit beside the command.
- **Errors.** Wrapped with `%w` and context; sentinel errors for conditions a caller tells
  apart; never discarded with `_`. A bad command line exits 2 with usage, a run that started and
  failed exits 1, a check with a FAIL exits 1.
- **Constants.** Every timeout, limit, glyph and file name is a named constant beside its user.
- **Naming.** The domain terms are Note `glossary`; code, flags, messages and docs use them and
  nothing else.
- **Tests.** Standard `testing`, table-driven with `t.Run`, `t.Helper()` in helpers, tested
  through the package's interface. Hermetic: no network, no files outside `t.TempDir()`, no
  dependency on the operator's fonts, ffmpeg or shell rc. A test that needs ffmpeg skips when it
  is absent. Recording is tested against the small fixture TUI in `internal/testkit/fixturetui`
  (built by `testkit.BuildFixtureTUI`), never a real app.
- **Dependencies.** Pinned in `go.mod`; each non-obvious one has its reason in the package that
  imports it.
