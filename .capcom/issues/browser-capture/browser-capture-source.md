---
title: Capture a web app in headless Chromium
status: open
type: feature
priority: 3
labels: [browser]
deps: [snapshot-two-layers, take-cell-geometry, text-layer-resolver, pixel-take-storage, browser-launch-config]
created: 2026-11-06T13:42:51Z
---

A second capture source beside the pty terminal: start Chromium headless at the configured
URL and viewport, drive it over the DevTools Protocol, and record snapshots whose image layer
is the page's screencast and whose text layer is the page's text with its on-screen boxes.
The engine performs beats through it: clicks and drags at a target's pixel box, keys and
typing as browser input events, waits on the text layer. It is the engine's second adapter
behind the interface the `Terminal` one implements today.

## Why

It is the feature itself: `photon setup`, `record` and `check` working on a web app.

## Acceptance

- `photon record` films a fixture web page end to end: types, clicks a text target and a selector target, waits, and writes a take that `photon check` judges.
- Services, profiles, `reset`, `on_success` and `run:` expects work as they do for a terminal take.
- The test is skipped without Chromium, as tune's browser test is, and runs on Linux and macOS.
