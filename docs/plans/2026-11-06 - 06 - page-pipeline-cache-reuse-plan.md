# Page pipeline cache reuse

**Goal:** a `Reuse` rebuild of a url-launch pipeline over the same take takes over the previous
pipeline's page frame caches, so `photon tune` stops decoding the page frames again on every edit.
**Date:** 2026-11-06
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** 85a3194
**Closes:** page-pipeline-cache-reuse

**Sources:**
- `.capcom/issues/browser-capture/page-pipeline-cache-reuse.md`
- `.capcom/knowledge/grid-surfaces.md`, `.capcom/knowledge/package-layout.md`

**Ratified design calls:**
- **What a page frame depends on:** only the take — a url-launch cache holds the image layer's
  decoded frame at a snapshot index, unscaled, in both the exact and the draft cache (the draft
  composition scales it). So the caches hold while the take is the same `*take.Take`; frame size,
  fonts and theme do not matter.

**Standing requirements:**
- skills: coding-standards
- Tests per AGENTS.md: single packages, `GOMEMLIMIT=2GiB`, never `./...` with `-race`.

**Regression check (2026-11-06, 85a3194):**
- 1: guard folded — comment rule widened to cache-ownership comments; render/doc.go, tune/session.go, tune/cache.go added to Files.

**Out of scope:**
- Merging a url-launch pipeline's exact and draft caches into one.
- Any change to tune's cache sizes or to command-launch reuse.

## 1. Take over the page frame caches on a url-launch Reuse rebuild

**What**

**Goal:** `render.Build` of a url-launch clip with `BuildOptions.Reuse` set to a url-launch
pipeline that built in full over the same take keeps that pipeline's exact cache as its own, and
its draft cache too when both pipelines have one; otherwise it keeps the caches it was given.
A reused pipeline's frames equal the page oracle's.

**Approach (assumed at the header base):** in `internal/render/pipeline.go`, `sourcesFrom`'s
url-launch branch returns `p.cache` / `p.draftCache`. Add a private predicate beside
`drawsAlike` (e.g. `canReusePageFrames(previous)`): previous non-nil, `previous.compositor != nil`,
`previous.recorded == p.recorded`, `previous.loaded.Config.IsBrowser()`, `previous.images != nil`.
When it holds, the branch returns `previous.cache`, and `previous.draftCache` when both
`p.draftCache` and `previous.draftCache` are non-nil (else `p.draftCache`). `buildFrames` already
stores whatever `sources` returns. Update the doc comments that enumerate what Reuse carries over —
`BuildOptions`, `Build`, `sourcesFrom` — and the `internal/render` row of
`.capcom/knowledge/package-layout.md` ("reusing an earlier pipeline's rasterizer, raster cache and
caption faces") to name the page frame caches. Comment rule: every comment or Note line that lists
what a Reuse rebuild takes over (`grep -rn "Reuse\|reuse" internal/render/*.go internal/tune/session.go .capcom/knowledge`).

**Regression guard.** The comment rule covers the `internal/render/doc.go` package comment (what
`BuildOptions` reuse carries over; url-launch drafts "kept in the draft cache") and `Session.build`'s
comment in `internal/tune/session.go`, and also every comment stating what a cache belongs to
(`grep -rn "belongs to one" internal/render internal/tune`: `RasterCache` in pipeline.go, `rasterCache` in internal/tune/cache.go).

**Files:** internal/render/pipeline.go; internal/render/pipeline_test.go; internal/render/doc.go; internal/tune/session.go; internal/tune/cache.go; .capcom/knowledge/package-layout.md
**Read first:** internal/render/pipeline.go — sourcesFrom, drawsAlike, imageLayerFrom, BuildOptions; internal/render/pipeline_test.go — TestBuild_AURLClipReusingAURLPipeline_TakesOverItsImageLayerAndCaptionFaces, loadPageTake, pageOracleFrames; internal/render/pipeline_draft_test.go — countingCache

**Tests** (in `internal/render/pipeline_test.go`, beside
`TestBuild_AURLClipReusingAURLPipeline_TakesOverItsImageLayerAndCaptionFaces`, using
`countingCache`, `loadPageTake`, `pageOracleFrames`):
- Same take, both with draft caches: `built.cache == previous.cache` and
  `built.draftCache == previous.draftCache`; after drawing a frame through `previous`, drawing the
  same frame through `built` puts nothing new into the given caches; frames equal the oracle.
- Previous without a draft cache, new with one: exact cache taken over, draft cache is the new one.
- Another take (a second `loadPageTake`) or a previous partial pipeline: the given caches are kept.

**Acceptance**
- `GOMEMLIMIT=2GiB go test ./internal/render/ -run 'TestBuild_AURLClip|TestPipeline_.*URLClip|TestBuild_Reuse'`
- `GOMEMLIMIT=2GiB go test ./internal/render/`
- `GOMEMLIMIT=2GiB go test ./internal/tune/ -run 'Reuse|Session'`
- `go vet ./internal/render/` and `capcom check`

**Commit:** `perf(render): take over the page frame caches on a url-launch rebuild`
