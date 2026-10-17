---
title: Module, CLI skeleton, Makefile, lint and CI
status: done
type: chore
priority: 1
plan: 2026-10-17 - 00 - photon-v1-plan
created: 2026-10-17T08:54:41Z
closed: 2026-10-17T12:25:04Z
reason: Resolved by plan "2026-10-17 - 00 - photon-v1-plan" (archived in bae9f7a)
---

The repository has no code yet. Everything after this needs a module, a build and a CI that
enforce the conventions from the first commit (Note `coding-conventions`).

Wanted:
- `go.mod` for `github.com/airiclenz/photon`, on Go 1.26.
- `cmd/photon`: a Cobra root command with no subcommands yet, `SilenceErrors`, errors printed
  by `main` as `photon: <err>`, and the exit-code mapping: a bad command line exits 2 with its
  usage, a run that started and failed exits 1, told apart by wrapping every error a
  subcommand's body returns.
- A `Makefile`: `build`, `test`, `vet`, `lint`, `fmt`.
- A committed `golangci-lint` config.
- `.github/workflows/ci.yml`: `go vet ./...`, `go test ./...` and lint on Linux, macOS and
  Windows.
- A shared structural-test helper that checks a package's `doc.go` maps every non-test file,
  so each package adds its doc-map test in one line.

## Acceptance

- `go build ./...`, `go vet ./...` and `go test ./...` pass locally and in CI on all three OSes.
- `photon --help` prints usage; `photon --bogus` exits 2; a test covers the mapping.
- The doc-map helper has its own test.
