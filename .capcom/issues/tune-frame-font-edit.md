---
title: Edit the terminal font size in photon tune
status: done
type: task
priority: 2
labels: [tune]
plan: 2026-10-18 - 02 - tune-polish-plan
created: 2026-10-18T14:38:11Z
closed: 2026-10-18T16:38:39Z
reason: Resolved by plan 2026-10-18 - 02 - tune-polish-plan (1cba598)
---

The `photon tune` inspector's "Clip frame" panel lets you edit only `width` and `fps`. The terminal's
`font_size` shows read-only, as do `line_height`, `padding` and `scale`, because
`storyboard.clipFields` (`internal/storyboard/fields.go`) marks every frame field outside
`editFields` read-only. Tuning how big the terminal text looks means editing the YAML by hand and
restarting.

`cols` and `rows` set the recorded terminal's size, so they rightly need a new `photon record`.
Which of the other fields only need a re-render should be confirmed first.

## Acceptance

- `frame.font_size` is an edit field (a decimal above 0, bounded like a caption's size is), applied
  through `storyboard.Apply` with comments and layout kept, and the preview re-renders.
- `line_height`, `padding` and `scale` are editable too if they only need a re-render. Any that need
  a re-record stay read-only, with the reason written down.
- The Clip frame panel shows each editable field as an input, and storyboard and session tests
  cover each new field.
