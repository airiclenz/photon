---
title: A directory named as caption_font passes config.Load
status: done
type: bug
priority: 3
labels: [config]
plan: 2026-10-18 - 01 - open-issues-plan
created: 2026-10-17T22:00:32Z
closed: 2026-10-18T12:34:25Z
reason: Resolved by plan 2026-10-18 - 01 - open-issues-plan (archived in 70f28a8)
---

The config's `caption_font` readability check, `CaptionFont.validateFiles`
(`internal/config/config.go:327`), opens each named file with `os.Open`
(`internal/config/config.go:333`). `os.Open` succeeds on a directory, so a `caption_font.regular`
or `caption_font.bold` that names a directory passes `config.Load` and fails only later, when
`fonts.LoadCaption` tries to read it as a font. Found closing out plan
`2026-10-17 - 05 - camera-captions-plan` (item 9).

## Acceptance

- `config.Load` reports a `caption_font.regular` or `caption_font.bold` that names a directory as
  a validation problem naming the field, as it does a file that cannot be opened.
- A config test covers a directory named as each caption font.
