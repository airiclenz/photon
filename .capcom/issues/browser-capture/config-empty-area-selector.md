---
title: An explicit empty area selector loads as a whole-screen area
status: open
type: bug
priority: 3
labels: [browser]
created: 2026-11-06T15:52:57Z
---

`areas.<name>.selector: ''` in `photon.yaml` is accepted silently as a whole-screen area: `Area.Selector` is a plain string (`internal/config/area.go:20`), and `Area.validate` reads an empty one as absent (`internal/config/area.go:56`), though a selector area's selector must be non-empty. Found closing out plan `2026-11-06 - 03 - grid-layers-plan` (item 10).

## Acceptance

- An area whose `selector` key is present but empty is refused with a problem naming the area.
