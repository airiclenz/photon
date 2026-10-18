---
title: Storyboard-owned typed edits and field description
status: open
type: task
priority: 3
labels: [architecture]
deps: [yamledit-upsert, located-problems]
plan: 2026-10-18 - 00 - architecture-review-plan
created: 2026-10-18T05:46:01Z
---

Tune holds a hand copy of the storyboard schema: six key-order tables copied from struct tags,
the "absent means" defaults, about 20 field constants, JS enum lists, and YAML text carried inside
JSON strings (`value.go`, `momentText`, `flowTarget`). Page tests catch renames only when they follow
naming conventions. Every new editable field (open issues `tune-caption-styling`,
`tune-add-remove-entries`, `tune-edit-pan-via`) adds more copies. Architecture review 2026-10-18,
finding 4.

Wanted: `internal/storyboard` owns typed edits (`Apply(src, edit)`) and an editable-field
description (`Fields(board)`) built from its struct tags, defaults and enums; tune's `/api/edit`
carries typed JSON values; `/api/state` carries the field description and the page renders kinds,
choices, defaults and read-only fields from it.

## Acceptance

- `internal/tune` holds no key-order table, field-default rule or YAML value parser; `value.go` is gone.
- The page holds no enum list or default value of a storyboard field.
- A Go round-trip test over every field in `Fields` replaces the JS regex tests that pin field names.
