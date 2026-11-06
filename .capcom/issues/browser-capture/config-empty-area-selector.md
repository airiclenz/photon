---
title: An explicit empty area selector loads as a whole-screen area
status: done
type: bug
priority: 3
labels: [browser]
plan: 2026-11-06 - 04 - browser-capture-plan
created: 2026-11-06T15:52:57Z
closed: 2026-11-06T20:17:34Z
reason: resolved by 2026-11-06 - 04 - browser-capture-plan (e406ff0)
---

`areas.<name>.selector: ''` in `photon.yaml` is accepted silently as a whole-screen area: `Area.Selector` is a plain string (`internal/config/area.go:20`), and `Area.validate` reads an empty one as absent (`internal/config/area.go:56`), though a selector area's selector must be non-empty. Found closing out plan `2026-11-06 - 03 - grid-layers-plan` (item 10).

## Acceptance

- An area whose `selector` key is present but empty is refused with a problem naming the area.
