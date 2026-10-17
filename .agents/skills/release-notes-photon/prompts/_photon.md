# photon facts — the repository this run describes

A shared include, verified against the tree when the skill was written. Read it
after `_shared.md`; when the tree contradicts a line here, trust the tree and
say so on your `FLAG` line.

## The baseline (`BASELINE`) and the diff base

The file is JSON. Use `baseline.tag` (the last **published** GitHub release,
e.g. `v0.3.1`), `baseline.published` (its publish time), `baseline.range`
(`<tag>..HEAD`) and `baseline.commits`. Ignore `gate`, `steps` and
`highestTag`. `version` is the checkout's `VERSION` file with a `v` in front;
it is never the release's number — that is the dispatch's `VERSION:` key,
`unreleased` when absent.

**No published release yet.** Then `baseline.tag` and `baseline.published` are
`""` and `baseline.range` is `HEAD`: the baseline is the **whole history**.
This is a normal state, never a reason to stop. In it:

- **The log range** is `baseline.range` either way: `git log HEAD` lists every
  commit when there is no tag.
- **The diff base** is `baseline.tag`, or with no tag the root commit the
  dispatch passes as `ROOT:` (from `git rev-list --max-parents=0 HEAD`).
  Every `git diff <diff base>` compares that commit with the working tree;
  `git diff <diff base> HEAD` covers the committed range alone.
- Where a prompt writes `<baseline.tag>` into a heading or title, write
  `the first commit` instead.

## Changelog

- File: `CHANGELOG.md`, in the [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
  shape. Unreleased heading, literally: `## [Unreleased]`.
- Release heading shape: `## [X.Y.Z] - YYYY-MM-DD` (no `v`, a hyphen), e.g.
  `## [0.3.1] - 2026-11-02`. Entries collect under `[Unreleased]` until a
  release cut rolls them under a new heading.
- Entries may sit directly under `## [Unreleased]` as plain bullets, or be
  grouped into `### Added`, `### Changed` and `### Fixed` subsections
  (`### Deprecated`, `### Removed` and `### Security` are read the same way
  when present). A bullet with no subsection carries no `Kind:` hint.
- An entry is one `- ` bullet: plain prose naming commands, flags and files in
  backticks, often several sentences long. There is no bold lead.

## Commit conventions

Conventional commits, `type(scope): subject` — types `feat`, `fix`, `docs`,
`ci`, `build`, `refactor`, `test`, `chore`; scopes are package or area names
(`cli`, `terminal`, `render`, `storyboard`, `release`, …). Bookkeeping that
ships nothing: `chore(plans)`, `docs(plans)`, `chore(issues)`,
`chore(release)`, `chore(version)`, and any commit touching only
`.capcom/issues/`, `.capcom/knowledge/` (except the glossary,
`.capcom/knowledge/glossary.md`), `docs/plans/`, `docs/handoffs/`, `.agents/`
or `.claude/`. Ticket ids are issue-register ids: the slug of a file under
`.capcom/issues/` (e.g. `release-setup`), with no prefix. No AI attribution
trailers. Plans live in `docs/plans/`; decisions are Knowledge Notes under
`.capcom/knowledge/` (e.g. `extension-model`). There are no ADR files.

## User surfaces — where a change becomes visible

| Surface | Paths |
|---|---|
| CLI commands & flags | `cmd/photon/root.go`, `cmd/photon/*.go` (one file per command; skip `_test.go`) |
| Exit codes | `cmd/photon/exit.go` |
| Config keys & defaults | `internal/config/` (the project's `photon.yaml`: `config.go`, `area.go`, `service.go`) |
| Storyboard format | `internal/storyboard/` (the storyboard YAML: header, beats, actions, targets, expects), `internal/keys/` (the key names a `key` action accepts) |
| Take & check | `internal/take/` (the take file `photon record` writes), `internal/check/` (what `photon check` reports) |
| Rendered output | `internal/render/`, `internal/raster/`, `internal/compose/`, `internal/fonts/` (the GIF, its theme, zoom and cursor, the bundled fonts) |
| Environment | `internal/rig/env.go`, `internal/service/`, `internal/clip/clip.go` (the `PHOTON_*` variables a project's commands see) |
| Docs | `README.md`, `docs/releasing.md`, `examples/`, `.capcom/knowledge/glossary.md` |
| Install & packaging | `.github/workflows/`, `cmd/release/templates/`, `cmd/release/dist.go` (what a release archive holds) |
| Emitted strings | `cmd/photon/` user messages and usage text, the `ValidationError` text of `internal/config/` and `internal/storyboard/` (what `photon lint` reports), `internal/check/` |

`VERSION` moves with every release and is not a surface. `cmd/release` is the
maintainer's release tool, not a user surface, apart from the two paths above.
