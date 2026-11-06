---
title: Configure a browser launch, its areas and selector targets
status: open
type: task
priority: 3
labels: [browser, config, storyboard]
deps: [grid-surfaces-decision, text-layer-resolver]
created: 2026-11-06T13:42:51Z
---

Extend `photon.yaml` and the storyboard for a web app: a browser launch (URL, viewport size,
device scale) beside today's command launch; areas found by CSS selector or by a text match;
and a target that names a CSS selector as well as text. `photon lint` checks them.

## Why

Projects plug in through config and storyboards (Note `extension-model`), so a browser clip
needs the same declarative surface a terminal clip has.

## Acceptance

- The config and storyboard schemas accept the new keys, decode strictly and validate, with errors naming the bad key.
- `photon lint` checks a browser storyboard against its config without starting a browser.
- The README's config and storyboard references describe the new keys.
