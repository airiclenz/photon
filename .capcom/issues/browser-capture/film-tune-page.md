---
title: Film photon tune's own page as an example clip
status: open
type: task
priority: 3
labels: [browser, docs]
deps: [pixel-take-render-tune]
created: 2026-11-06T13:42:51Z
---

Dogfood the browser source: an example under `examples/` that starts `photon tune` on the
`examples/hello` take as a service, films its page (play, scrub, drag a beat edge, save),
and renders a GIF the README can show.

## Why

It proves the feature on a real app, and it is the clip that started this: photon could not
film its own tune page.

## Acceptance

- An example storyboard and config record, check and render the tune page with every expect passing.
- The README shows the rendered clip and links the example.
