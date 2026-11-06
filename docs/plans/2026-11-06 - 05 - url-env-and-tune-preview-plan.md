# url launch env and tune without a preview — plan

**Goal:** A url launch's browser runs with exactly the env photon builds for it, and
`launch.strip` works on a url launch. `photon tune` opens on a terminal take under a url-launch
clip and shows the take-size problem with no preview, instead of failing to start.
**Date:** 2026-11-06
**Status:** unexecuted
**sized for:** ~200k-context host
**base:** 5ee12a1

**Sources:**
- `.capcom/issues/browser-capture/browser-env-strip.md`
- `.capcom/issues/browser-capture/tune-terminal-take-viewport.md`
- Notes `coding-conventions`, `package-layout`, `grid-surfaces`
- chromedp v0.23.0 `allocate.go` — `Env`, `NoInheritEnv`, `ExecAllocator.setCmdEnv`

**Ratified design calls:**
- **Strip on a url launch:** config accepts `launch.strip` with `launch.url`; the browser env is exactly the built env (user, 2026-11-06).
- **Undrawable take in tune:** the session opens with no pipeline, the take-size problem stands as the clip problem, and the page shows no picture (user, 2026-11-06).

**Standing requirements:**
- skills: coding-standards
- Tests on the Pi: one package at a time, `GOMEMLIMIT=2GiB`, never `./...` with `-race`, never two test commands at once.
- No forbidden name from `AGENTS.local.md` lands in any file.

**Out of scope:**
- `page-pipeline-cache-reuse` (the frame caches a url-launch rebuild drops).
- Previewing a terminal take under a url-launch clip by rasterizing its cells.

**Regression check (2026-11-06, 5ee12a1):**
- 1: guard folded
- 2: guard folded; supersedes Note `grid-surfaces` (`.capcom/knowledge/grid-surfaces.md:32`)
- 3: guard folded (decision applied); supersedes README.md:195-197 for a url-launch clip's terminal take
- 4: guard folded (decision applied)

## 1. The browser's env is exactly Options.Env — ✅ DONE (2026-11-06)

NOTES (2026-11-06): env_test.go uses literal viewport/scale/fps values because the package's test constants live in the `browser`-tagged browser_test.go, and its fake browser calls `/usr/bin/env` by absolute path since it runs with no PATH.

**What:**
**Goal:** `browser.Start` runs the browser process with exactly `Options.Env` as its env, and
nothing inherited from photon's process; a variable in photon's env that `Options.Env` leaves out
is absent from the browser process.
**Approach (assumed at the header base):** in `internal/browser/browser.go`, `allocatorOptions`
adds `chromedp.NoInheritEnv` beside `chromedp.Env(options.Env...)`. The `Options` doc comment
says Env is the browser process's whole env (KEY=VALUE entries), not extra vars "on top of
photon's own environment". The `//go:build browser` tests that call `Start` pass
`Env: os.Environ()`. `recorder.openPage`'s comment in `internal/rig/record.go` says environ *is*
the browser's env. Rule for prose: every comment saying the browser env adds to photon's env
(`grep -rn "on top of photon\|added to the browser's env" internal`).
**Regression guard.** The prose rule also covers `allocatorOptions`' doc comment
(`internal/browser/browser.go:291-293`, "options' extra environment"), which says the browser's
whole env instead; the rule's grep is
`grep -rn "extra environment\|extra KEY\|on top of photon\|added to the browser's env" internal`.
**Files:** internal/browser/browser.go, internal/browser/env_test.go, internal/browser/browser_test.go, internal/browser/textlayer_browser_test.go, internal/rig/record.go
**Read first:** internal/browser/browser.go — allocatorOptions, Options, Start; internal/rig/record.go — recorder.openPage; internal/browser/browser_test.go — TestBrowser_Start_LoadsThePageAtTheViewportAndScale;
internal/browser/textlayer_browser_test.go — startAt; chromedp@v0.23.0 allocate.go — NoInheritEnv, ExecAllocator.setCmdEnv
**Tests:** new `internal/browser/env_test.go` (`//go:build !windows`, no `browser` tag). It sets
a variable with `t.Setenv`, then calls `Start` with `ExecPath` a shell script in `t.TempDir()`
that writes `env` to a file and exits 1, and `Env` holding a marker var but not the set one.
`Start` fails; the file holds the marker and not the set variable.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/browser/`
- `GOMEMLIMIT=2GiB go vet -tags browser ./internal/browser/ ./internal/rig/`
- `GOMEMLIMIT=2GiB go build ./...`
**Commit:** `fix(browser): run the browser with exactly the launch env`

## 2. launch.strip on a url launch — ✅ DONE (2026-11-06)

NOTES (2026-11-06): CHANGELOG is an amendment of the existing Unreleased entry (CHANGELOG.md:11), per the plan, not a new entry; the closeout applies it as a replacement of that sentence.
NOTES (2026-11-06): the valid case is a new test `TestLoadURLLaunchKeepsStrip` on `urlBase` plus `strip: ['APP_*']`, rather than an extension of `TestLoadURLLaunch`.

**What:**
**Goal:** a config with `launch.url` and `launch.strip` loads without a problem; a url take's
browser env lacks every variable a `launch.strip` glob names; the README documents strip for
both launch kinds.
Depends on item 1.
**Approach (assumed at the header base):** in `internal/config/config.go`, the `Launch`
validation drops the `strip: only a command launch takes strip` refusal (the `isBrowser() &&
len(l.Strip) > 0` case); the glob-shape check still runs for both launch kinds. The `Launch` doc
comment no longer says a url launch "takes no Strip". `rig`'s `launchEnv` already strips for
both kinds, and `openPage` passes its result as `browser.Options.Env`, so `internal/rig`
needs no code change. README.md's `launch.strip` row drops "Not with `launch.url`." and says the
globs are removed for the program or the browser. Rule for prose: every doc or comment saying
strip is command-only (`grep -rn -i "strip" README.md docs/*.md internal/config/*.go`).
**Regression guard.** `TestLoadRejectsOnAURLLaunch` runs `assertRejects`, so every row fails:
delete its `"strip"` row, add there a malformed-glob row (`strip: ['APP_[']` → `launch.strip[0]`),
and test the valid case in `TestLoadURLLaunch` (or a new test on `urlBase` plus `strip: ['APP_*']`
checking `Launch.Strip` is kept). The prose rule's grep also covers `.capcom/knowledge`.
This supersedes Note `grid-surfaces` (`.capcom/knowledge/grid-surfaces.md:32`, "`strip` is
refused with a url"), per the ratified call; the archived grid-layers plan stays as history. The
CHANGELOG sidecar amends the Unreleased entry (CHANGELOG.md:11, "A url launch refuses
`launch.strip`") instead of adding a new one.
**Files:** internal/config/config.go, internal/config/config_test.go, README.md, internal/rig/env_test.go, .capcom/knowledge/grid-surfaces.md
**Read first:** internal/config/config.go — Launch.validate, Launch; internal/config/config_test.go — TestLoadRejectsOnAURLLaunch, TestLoadURLLaunch, urlBase;
internal/rig/env.go — launchEnv; internal/rig/env_test.go — TestLaunchEnv; .capcom/knowledge/grid-surfaces.md — Capture sources
**Tests:** `TestLoadRejectsOnAURLLaunch` loses its `"strip"` row and gains a row where a url
launch with `strip: ['APP_[']` fails with the `launch.strip[0]` problem; `TestLoadURLLaunch` (or a
new test on `urlBase`) loads a url launch with `strip: ['APP_*']` cleanly and keeps
`Launch.Strip`. `internal/rig/env_test.go` gains a `launchEnv` case
with a url-shaped `config.Launch` (URL set, Strip set) whose result lacks the stripped name.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/config/`
- `GOMEMLIMIT=2GiB go test ./internal/rig/ -run 'LaunchEnv'`
**Closes:** browser-env-strip
**Commit:** `feat(config): allow launch.strip on a url launch`

## 3. tune opens with no pipeline on a take it cannot draw

**What:** fixes `tune-terminal-take-viewport`: `NewSession` fails on a terminal take under a
url-launch clip with a `frame.viewport`, because `atTakeSize` leaves the viewport and render's
url-launch sources need an image layer (`errNoImageLayer`).
**Goal:** `NewSession` on a url-launch clip and a take with no cell size returns a session.
Its `State().Errors` holds the one clip problem wrapping `render.ErrTakeSize`, ending
"re-record it with `photon record`". `Frame`, `DraftFrame`, `FullFrame`, `FullDraftFrame`,
`Camera` and target picks return the new exported sentinel `tune.ErrNoPreview`. `State` reports
the storyboard's schedule. Every other take keeps its current session behaviour.
**Approach (assumed at the header base):** in `Session.build` (`internal/tune/session.go`),
when `render.CheckFrame` refuses and the take cannot be drawn, set `sizeErr`, skip
`buildPipeline`, and return a `pipeline` with nil `frames` and no error. "Cannot be drawn" means
`built.loaded.Config.IsBrowser()` and `recorded.Cell == (take.CellSize{})`. `atTakeSize` is
unchanged for every other take. Each reader of `good.frames`/`current.frames`
(`goodPipeline` callers in `frame.go`, `pick.go`, and `State` in `state.go`) handles nil
`frames`: frame and pick calls return `ErrNoPreview`, and `State` takes its schedule from
`schedule.New(board, s.recorded)` when no pipeline built. `BuildOptions.Reuse` takes a nil
pipeline.
**Regression guard.** Tests add one test helper in internal/tune/session_test.go, newNoPreviewSession(t) — the url-launch clip from pageSessionConfig/pageSessionStoryboard plus a saved terminal take with no cell size, opened through NewSession — which item 4's server and page tests reuse.
On the skip path `build` runs `schedule.New(board, s.recorded)` and returns its error as the
build error (so `NewSession` still refuses a take whose beats mismatch, and a failed rebuild keeps
`good`, as today); the schedule is kept on the `pipeline` for `State`, not rebuilt per poll.
Every comment or doc line saying a mismatched take previews at the take's size
(`grep -rn "take's size\|keeps drawing\|recorded size" internal/tune/*.go README.md`) carves out
the url-launch clip's terminal take; this supersedes README.md:195-197 for that take, and the
released CHANGELOG 0.3.0 entry (CHANGELOG.md:37) stays as history.
**Files:** internal/tune/session.go, internal/tune/frame.go, internal/tune/pick.go, internal/tune/state.go, internal/tune/session_test.go, internal/tune/doc.go, README.md
**Read first:** internal/tune/session.go — Session.build, atTakeSize, NewSession; internal/tune/state.go — Session.State, sectionStates;
internal/tune/frame.go — goodPipeline; internal/render/pipeline.go — Build; internal/tune/session_test.go — startSessionFixture
**Tests:** in `internal/tune/session_test.go`: open the session with `newNoPreviewSession(t)`;
a no-cell-size take whose beats the storyboard lacks still fails `NewSession`; `NewSession` succeeds,
`State().Errors` is the take-size problem, `Frame(0)` and `Camera(0, nil)` fail with
`ErrNoPreview`, and `State().LengthMS > 0`.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/ -run 'Session'`
- `GOMEMLIMIT=2GiB go vet ./internal/tune/`
**Closes:** tune-terminal-take-viewport
**Commit:** `fix(tune): open on a take the clip cannot draw, without a preview`

## 4. tune's server and page show no preview

**What:**
**Goal:** with a session from item 3, the frame, camera and pick routes answer 409 with the
`ErrNoPreview` message. The state JSON carries `"preview": false` (true otherwise). The page,
on `preview: false`, fetches no frames and shows the text "No preview — the take does not fit
this clip; re-record it with `photon record`." in the `#preview` section in place of the picture.
Depends on item 3.
**Approach (assumed at the header base):** `statusOf` in `internal/tune/server.go` maps
`ErrNoPreview` to `http.StatusConflict`. `State` (`internal/tune/state.go`) gains
`Preview bool \`json:"preview"\``, true when the shown pipeline has frames. `frames.js` gates
its `request`/`prefetch` on `page.state().preview`, and `app.js` toggles a placeholder element
added to `index.html` inside `#preview`. The text is a named constant in the script.
**Regression guard.** the server and page tests build their session with item 3's newNoPreviewSession(t) helper.
`app.js` is at 395 lines against `maxScriptLines` (400): the placeholder lookup, toggle and text
constant go in `frames.js`, or `app.js` gains at most 4 lines. `handlePick` does not use
`statusOf`: it answers 409 when `errors.Is(err, ErrNoPreview)` and 422 otherwise, and
`newHandler`'s status doc names the no-preview 409.
**Files:** internal/tune/server.go, internal/tune/state.go, internal/tune/page/index.html, internal/tune/page/frames.js, internal/tune/page/app.js, internal/tune/page/style.css, internal/tune/server_test.go, internal/tune/pagejs_test.go
**Read first:** internal/tune/server.go — statusOf, handlePick, newHandler; internal/tune/state.go — State; internal/tune/page/frames.js — request, prefetch;
internal/tune/page_test.go — maxScriptLines; internal/tune/pagejs_test.go — loadPage
**Tests:** `server_test.go`: a handler over `newNoPreviewSession(t)` answers `GET` on the frame
route with 409, a pick `POST` with 409, and the state route with `"preview":false`. `pagejs_test.go` (goja): with a
state of `preview: false` the page makes no frame request and shows the placeholder text.
**Acceptance:**
- `GOMEMLIMIT=2GiB go test ./internal/tune/ -run 'Handler|Page'`
**Commit:** `feat(tune): show no preview when the take cannot be drawn`
