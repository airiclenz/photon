---
title: Pick a target by clicking cells in the editor's preview
status: done
type: feature
priority: 3
labels: [render]
deps: [timing-editor]
plan: 2026-10-17 - 04 - timing-editor-plan
created: 2026-10-17T17:28:47Z
closed: 2026-10-17T19:51:09Z
reason: Resolved by the timing-editor plan (2026-10-17 - 04 - timing-editor-plan), archived at 207e422
---

Writing a target means guessing a regular expression and an area, then recording or rendering to
see whether it landed. In the timing editor the frame is already on screen, so the target can be
picked from it instead.

Wanted: in the timing editor, click or drag over cells in a preview frame to set a beat's zoom
target, or to copy a target for a `click`, `drag` or `seen` into the clipboard. The editor proposes
a text regex and, where one contains the cells, an area from `photon.yaml`, and checks the proposal
with the one resolver in `internal/target` before offering it, showing the box it resolves to.

Full visual authoring of actions, waits and expects against a live app is out of scope: it needs a
live pty and a record/replay UI, and editing the storyboard then running `photon record` already
works.

## Acceptance

- Picking cells in a fixture frame proposes a target that the resolver resolves to a box covering
  those cells, and the editor shows that box.
- A proposed zoom target saves into the storyboard through the editor's node-preserving save.
- Unit tests cover the regex proposal (escaping, uniqueness or `nth` when the text repeats) and
  area selection.
- `go vet ./...` and `go test ./...` pass.
