# **photon** - agent guide

photon records a terminal app's demo clip from a storyboard, checks the take, and renders the
shipped GIF. It is a standalone Go CLI: the binary is built from `cmd/photon` and every other
package lives under `internal/`, so photon has no Go API to keep stable. A project plugs its own
pieces in through its photon config and storyboards — commands it runs, services it starts —
never through Go code (Note `extension-model`).

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

- `capcom ready`: the issues ready to work on, most urgent first
- `capcom show <id>`: one issue in full, with what blocks it
- `capcom update <id> --status active`: claim it before you start
- `capcom log <id> <text>`: note progress as work lands
- `capcom close <id> --reason <text>`: close an issue; it names the issues this made ready
- `capcom reopen <id> --reason <text>`: undo a close
- `capcom create --id <slug> --title <title> [--dep <id>]`: file a new issue
- `capcom dep add <id> <dep-id>`: make an issue wait for another; `capcom dep list <id>` shows both sides
- `capcom list --tree --depth 2`: the outline of the issues
- `capcom know tree`: the outline of the Knowledge tree; `capcom know show <id>` reads a Note

Each command's flags are in `capcom <cmd> --help`.

An issue's body says what is wrong or wanted, why, and how to tell it is done (`## Acceptance`),
readable on its own; a plan says how and in what order.

Where things are: an issue is `.capcom/issues/<id>.md`, and its children sit in the directory
`.capcom/issues/<id>/` beside it; Notes follow the same shape under `.capcom/knowledge/`.
Prefer capcom commands; after a hand edit run `capcom check`.

Tree lock: before a change that spans several files or commits, it is recommended to claim this
worktree with `capcom lock acquire --purpose <text>` and keep the token it prints.
`capcom lock touch --token <token>` renews it during long work,
`capcom lock release --token <token>` frees it when done, and acquiring again with `--token <token>`
resumes it after a restart or compaction. When `capcom lock check --token <token>` exits 3, or
`capcom lock check` does while you hold no token, another session holds this worktree: ask the user
before editing it.

Knowledge tree, top level:

- `coding-conventions`: Before writing code: house conventions from coding-standards
- `extension-model`: Decision: one generic CLI, projects plug in through commands and services
- `glossary`: Domain glossary: clip, storyboard, take, beat, target, area and the rest
- `package-layout`: Package map and dependency direction
<!-- END CAPCOM -->
