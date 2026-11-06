# **photon** - agent guide

photon records a terminal app's demo clip from a storyboard, checks the take, and renders the
shipped GIF. It is a standalone Go CLI: the binary is built from `cmd/photon`, the module root
package only embeds `VERSION`, and every other package lives under `internal/`, so photon has no
Go API to keep stable. Beside them, `cmd/release` is the release tool: a self-contained
`package main` whose non-test code imports only the standard library and cobra (its tests use
`internal/testkit`), run with `go run ./cmd/release` and never shipped in an archive. A project
plugs its own pieces in through its photon config and storyboards — commands it runs, services
it starts — never through Go code (Note `extension-model`).

Project knowledge lives in capcom's Knowledge tree (listed at the end of this file). Before
starting work, run `capcom know tree --depth 2`, then `capcom know show <id>` on every Note whose
title matches the task.

## Working rules

- Commit to `main`; no feature branches or pull requests unless asked.
- No AI attribution anywhere: no co-author trailers, no "generated with" footers, not in commits,
  pull requests, code or docs.
- Follow the coding standards in Note `coding-conventions` before writing code.
- When a plan under `docs/plans/` is saved, link every issue it will close with
  `capcom update <id> --plan "<plan basename without .md>"`.

## Tests on the Raspberry Pi 5 host

Memory is tight. Run single packages or single tests with `GOMEMLIMIT=2GiB`. Never run `./...`
with `-race`, and never run two test commands at once.

<!-- BEGIN CAPCOM -->
<!-- Managed by capcom: run "capcom agents" to refresh. Edits inside these markers are overwritten. -->
## Issues and knowledge: capcom

capcom is the issue register of record for this repository. Issues and Knowledge Notes are
markdown files under `.capcom/`, versioned with the code; track work there and nowhere else.

Without a `capcom` binary, install it from https://github.com/airiclenz/capcom#install.

- `capcom next`: the most urgent ready issue, in full
- `capcom ready`: the issues ready to work on, most urgent first
- `capcom show <id>`: one issue in full, with what blocks it
- `capcom context <id>`: the issue with its deps, plan and the Notes that apply; run it before you start
- `capcom start <id>`: claim it and take the Tree lock before you start
- `capcom log <id> <text>`: note progress as work lands
- `capcom close <id> --reason <text>`: close an issue; it names the issues this made ready
- `capcom finish <id> --reason <text>`: close it and free the Tree lock; it names the issue to work on next
- `capcom reopen <id> --reason <text>`: undo a close
- `capcom create --id <slug> --title <title> [--dep <id>]`: file a new issue
- `capcom apply <file>`: file a linked set of issues in one call, all or none
- `capcom dep add <id> <dep-id>`: make an issue wait for another; `capcom dep list <id>` shows both sides
- `capcom list --tree --depth 2`: the outline of the issues
- `capcom know tree`: the outline of the Knowledge tree; `capcom know show <id>` reads a Note

Each command's flags are in `capcom <cmd> --help`.

An issue's body says what is wrong or wanted, why, and how to tell it is done (`## Acceptance`),
readable on its own; a plan says how and in what order.

Where things are: an issue is `.capcom/issues/<id>.md`, and its children sit in the directory
`.capcom/issues/<id>/` beside it; Notes follow the same shape under `.capcom/knowledge/`.
Prefer capcom commands; after a hand edit run `capcom check`.

A standing issue (`standing: true` in its file) only groups its children: never claim or close it.

Tree lock: before a change that spans several files or commits, it is recommended to claim this
worktree with `capcom lock acquire --purpose <text>`. capcom keeps the Lock token for the session:
running the same acquire again resumes the lock after a restart or compaction and prints the token
again (or pass `--token <token>`).
`capcom start <id>` takes the lock for an issue, and running the same start again resumes it too.
`capcom lock touch --token <token>` renews it during long work,
`capcom lock release --token <token>` frees it when done. When `capcom lock check --token <token>` exits 3, or
`capcom lock check` does while you hold no token, another session holds this worktree: ask the user
before editing it. The lock is signalled, not enforced: a capcom write is never refused, but it
warns, naming the holder, while the lock is held for a purpose other than the issue it writes; if
the lock is yours, carry on.

Knowledge tree, top level:

- `coding-conventions`: Before writing code: house conventions from coding-standards
- `extension-model`: Decision: one generic CLI, projects plug in through commands and services
- `glossary`: Domain glossary: clip, storyboard, take, beat, target, area and the rest
- `grid-surfaces`: Decision: one grid, a text layer and an image layer, for terminal and browser takes
- `package-layout`: Package map and dependency direction
<!-- END CAPCOM -->
