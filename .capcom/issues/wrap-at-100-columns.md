---
title: Settle the 100-column wrap
status: done
type: task
priority: 4
labels: [style]
plan: 2026-10-18 - 01 - open-issues-plan
created: 2026-10-18T09:15:40Z
closed: 2026-10-18T12:34:25Z
reason: Resolved by plan 2026-10-18 - 01 - open-issues-plan (archived in 70f28a8)
---

The coding conventions (Note `coding-conventions`) ask for a ~100-column wrap, but nothing enforces
it. `.golangci.yml` enables no line-length linter, and 1012 lines across the tracked Go files run
past 100 columns. Each plan run re-reports the overflow in the files it touched: the run of plan
`2026-10-18 - 00 - architecture-review-plan` deferred the table and assert lines in
`internal/tune/session_test.go`, `edit_test.go` and `server_test.go`,
`internal/storyboard/storyboard_test.go`, `edit_test.go` and `fields_test.go`, the `place`
signature at `internal/caption/caption.go:76`, and a stray blank line before `want` in the tune
pan-target test. Reviewers spend findings on it, and nothing stops more of it from landing.

## Acceptance

- Either the wrap is enforced (a line-length linter in `.golangci.yml` at the chosen width, with
  existing violations fixed or excluded by rule), or the Note `coding-conventions` drops or relaxes
  the rule, for test tables at least.
- The stray blank line in the tune pan-target test is gone.
