# Tune panel problems plan

**Goal:** prove under goja that an edit's problem shows on its owning inspector panel for a
beat, the clip's frame, a camera move and a caption, so the tune page's scripts are checked by
running them, not by a manual session.
**Date:** 2026-10-19
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** 5ddd825
**Closes:** tune-page-js-check

**Regression check (2026-10-19, 5ddd825):**
- 1: guard folded (no `input, textarea` selector; the DOM stub rejects a selector list)

**Sources:**
- `.capcom/issues/tune-page-js-check.md`
- `internal/tune/pagejs_test.go` (the goja page harness; it already loads every script
  `internal/tune/page/index.html` names, in CI on every OS)

**Ratified design calls** (Airic Lenz, 2026-10-19):
- **Manual session:** goja tests that drive the four edits and assert each problem on its owning
  panel replace the issue's manual `photon tune` session; the issue's Acceptance is rewritten to
  name them.
- **Playback issue:** `tune-preview-playback-speed` is out of this plan; its by-eye check is
  measured by hand.

**Standing requirements:**
- skills: coding-standards
- Pi 5 host: run single packages with `GOMEMLIMIT=2GiB`; never `-race` on `./...`; one test
  command at a time.
- Any authorized deviation from item text lands as a dated NOTES line under the item.

**Out of scope:**
- `tune-preview-playback-speed`, any playback readout on the page.
- Changing page scripts or server behaviour; this plan adds tests and amends one issue file.
- A Node or other JavaScript engine in CI.

## 1. Edit problems show on their owning panel

**What:**
**Goal:** `internal/tune/pagejs_test.go` holds goja tests that, for each of a beat, the clip's
frame, a camera move and a caption, send one edit through the inspector that leaves a problem on
that owner and assert the problem's message as a `p.error` row under `#inspector-body` while that
owner's panel is selected, and not on another owner's panel; and the Acceptance of
`.capcom/issues/tune-page-js-check.md` names these tests in place of the manual session.

**Regression guard.** Never widen `setField`'s query to `querySelectorAll("input, textarea")`:
`testdata/dom.js` `queryAll` splits on whitespace and `compoundOf` throws on `"input,"`, turning
every existing `setField` caller red. Reach the caption's `<textarea>` with one helper beside
`setField` (query `"textarea"`) or by filtering the row's children by `tagName`; dom.js stays unchanged.

**Approach (assumed at the header base):**
- Refused edits (422; app.js `edit` stores the refusal, `errorRows` adds it on the panel whose
  `ownerKey` matches):
  - beat: select beat `"1"` via `elementBeatSelect`, `setField("hold (ms)", "value", "5000")`;
    message contains `want less than duration`.
  - clip frame: default selection (`""`), `setField("fps", "value", "0")`; message contains
    `frame.fps`. `framePanel` also renders `state.warnings` as `p.error`, so match by message.
- Standing problems (200; `state.errors` → `problemsOn`):
  - camera move: `aimedStoryboard`, select `entryValueOf(storyboard.OwnerCamera, aimedPan)`,
    set the "to" target text to a target not on screen (as
    `TestSession_PanTargetNotOnScreen_KeepsTheLastGoodPipelineAndBlocksSave` does); message
    names `camera[1]`.
  - caption: `cameraCaptionsStoryboard`, caption `styledCaption`, text too wide for the frame
    (as `TestSession_CaptionBoxLargerThanTheFrame_IsOnTheCaption`); message names
    `captions[0]`. The "text" row is a `<textarea>`: add one textarea helper beside `setField`
    (query `"textarea"`), or filter the row's children by `tagName`.
- Read rows with one harness helper (e.g. `errorRows()` returning the `p.error` texts under
  `elementInspector`); for "not on another panel", select a different owner and assert the
  message is absent. Table-driven where the four cases share one shape; never `assertEdit` on a
  refused edit (it asserts 200).
- Rewrite the issue's second Acceptance bullet to name the goja tests (no manual session); keep
  the first bullet; add a `## Log` line only via `capcom log`, never by hand. Run `capcom check`.

**Files:** internal/tune/pagejs_test.go; .capcom/issues/tune-page-js-check.md
**Read first:** internal/tune/pagejs_test.go — setField, selectValue, entryValueOf;
internal/tune/page/controls.js — errorRows, problemsOn; internal/tune/page/app.js — edit, ownerKey;
internal/tune/testdata/dom.js — compoundOf

**Tests:** the new `TestPageJS_*` tests above.

**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune -run 'TestPageJS_' -count=1`
- `GOMEMLIMIT=2GiB go vet ./internal/tune`
- `capcom check`
- `grep -n 'goja\|TestPageJS' .capcom/issues/tune-page-js-check.md`

**Commit:** `test(tune): check each edit's problem shows on its owning panel`
