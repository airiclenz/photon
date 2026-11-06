---
title: Configure a browser launch, its areas and selector targets
status: done
type: task
priority: 3
labels: [browser, config, storyboard]
deps: [grid-surfaces-decision, text-layer-resolver]
plan: 2026-11-06 - 04 - browser-capture-plan
created: 2026-11-06T13:42:51Z
closed: 2026-11-06T20:17:34Z
reason: resolved by 2026-11-06 - 04 - browser-capture-plan (e406ff0)
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

## Log

- 2026-11-06 Airic Lenz: grid-layers-plan (6999cd1) landed launch.url/browser, selector areas, viewport/device_scale/selector targets and browser lint; open gaps: areas.<name>.selector: '' is accepted as a whole-screen area, position.cell/target edits do not clear a selector key and decodeTarget drops a selector JSON key, and pageLayout's Cell ignores frame.scale
