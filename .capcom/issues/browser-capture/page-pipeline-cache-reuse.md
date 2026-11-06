---
title: A url-launch rebuild drops the page frame caches
status: done
type: task
priority: 3
plan: 2026-11-06 - 06 - page-pipeline-cache-reuse-plan
created: 2026-11-06T20:12:42Z
closed: 2026-11-06T21:21:28Z
reason: "Resolved by 2026-11-06 - 06 - page-pipeline-cache-reuse-plan (archived at 9d335f8): a url-launch Reuse rebuild over the same take takes over the previous pipeline's page frame caches, tested in internal/render/pipeline_test.go"
---

On a rebuild with `BuildOptions.Reuse` set, a url-launch pipeline takes over only the previous pipeline's image-layer decoder (`imageLayerFrom`, internal/render/pipeline.go:559) and caption faces; `sourcesFrom` (internal/render/pipeline.go:203) hands it its own fresh caches (internal/render/pipeline.go:210-211), so every `photon tune` rebuild of a browser take decodes the page frames again instead of reusing the frames the previous pipeline already decoded.

## Why

Tune rebuilds on every edit; decoding the page frames again each time slows the browser-take preview.

## Acceptance

- A Reuse rebuild of a url-launch pipeline over the same take takes over the previous pipeline's frame caches where they still hold, with a test in internal/render/pipeline_test.go
