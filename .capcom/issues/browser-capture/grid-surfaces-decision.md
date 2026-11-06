---
title: "Decision: one grid model for terminal and browser takes"
status: done
type: task
priority: 3
labels: [browser, architecture]
plan: 2026-11-06 - 03 - grid-layers-plan
created: 2026-11-06T13:42:51Z
closed: 2026-11-06T15:53:42Z
reason: resolved by 2026-11-06 - 03 - grid-layers-plan (6999cd1)
---

Record the decision that photon films more than terminals, and how: a snapshot is a grid
with a text layer and an image layer, and each capture source (pty terminal, browser) fills
both. It amends the `extension-model` Note (projects still plug in through config and
storyboards; a browser is a launch kind, not a Go hook) and the `package-layout` Note (which
package owns the layers, the cell geometry, the browser source, and how the engine's
`Terminal` interface generalizes).

It also settles the open questions the later issues depend on: which edits a pixel take
refuses (anything that reflows text: `font_size`, `line_height`, `cols`, `rows`), how pixel
frames are stored, and that the browser is Chromium driven over the DevTools Protocol.

## Why

Today's Notes assume a terminal throughout. Every later issue changes a package boundary
they describe, so the boundary has to be decided once, before code.

## Acceptance

- A decision Note under `.capcom/knowledge/` states the two-layer grid model, the capture sources, and the edits a pixel take refuses.
- The `extension-model` and `package-layout` Notes are amended to match, and `capcom check` reports no problems.
