---
title: Attach typed owners to problems instead of scraping messages
status: open
type: task
priority: 2
labels: [architecture]
plan: 2026-10-18 - 00 - architecture-review-plan
created: 2026-10-18T05:46:01Z
---

`internal/tune/state.go` (`problemsOf`, `beatNamed`, `clipLevelPrefixes`) and
`internal/tune/page/app.js` (`errorRows`, `entryErrorPrefix`) guess which beat, camera move or
caption an error belongs to by scraping its message. A compose-stage or caption-stage error starts
with `compose: ` or `caption: `, so both prefix checks miss it: it never shows on its move's or
caption's panel and lands on a beat if it names one. Architecture review 2026-10-18, finding 3.

Wanted: the code that writes the `camera[i]: ` / `captions[i]: ` / `beat N: ` prefix attaches a
typed owner instead; tune's `Problem` serialises it and the page filters on owner fields. CLI error
text stays byte-identical. A camera or caption problem shows on its own panel only, and camera and
caption options get the error mark beats have.

## Acceptance

- No regex, prefix list or `startsWith` attributes problems in tune or its page.
- A compose `camera[i]: pan:` error and a caption box-overflow error land on their own entry.
- Every existing CLI error text is unchanged.
