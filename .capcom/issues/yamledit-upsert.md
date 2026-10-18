---
title: Give yamledit query, upsert and delete-if-present
status: done
type: task
priority: 3
labels: [architecture]
plan: 2026-10-18 - 00 - architecture-review-plan
created: 2026-10-18T05:46:01Z
closed: 2026-10-18T08:56:31Z
reason: resolved by plan 2026-10-18 - 00 - architecture-review-plan (archived in 0778002)
---

`internal/yamledit` offers strict Set/Insert/Delete/Replace and no query. Tune's splicer
(`internal/tune/edit.go`: `lookup`, `childNode`, `writeScalar`, `deleteKey`) rebuilds
insert-or-update and delete-if-present on a third parse (a `yaml.Node` tree), and multi-step edits
read presence from the pre-edit tree. Architecture review 2026-10-18, finding 5.

Wanted: yamledit answers presence and scalar-text queries and offers upsert and delete-if-present on
its always-fresh tree; tune drops its own tree.

## Acceptance

- yamledit exposes the query, upsert and delete-if-present operations, tested on block, flow and
  sequence-item mappings.
- `internal/tune` holds no `yaml.Node` tree and does not import `gopkg.in/yaml.v3` for editing.
- `internal/tune/edit_test.go` passes with its cases unchanged.
