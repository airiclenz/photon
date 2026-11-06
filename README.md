# **photon** - record terminal demo GIFs from a script, for any CLI or TUI app

[![Release](https://img.shields.io/github/v/release/airiclenz/photon)](https://github.com/airiclenz/photon/releases/latest)
[![CI](https://github.com/airiclenz/photon/actions/workflows/ci.yml/badge.svg)](https://github.com/airiclenz/photon/actions/workflows/ci.yml)
[![Go version](https://img.shields.io/github/go-mod/go-version/airiclenz/photon)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Scripted, repeatable demo GIFs for terminal apps.**

photon is a terminal recorder for README demos. You write a storyboard in YAML. photon runs your
real app, types, presses keys, clicks and drags the mouse, and records the screen. It checks that
the take shows what it should. Then it renders a small, sharp GIF with smooth zooms and a drawn
mouse cursor. When your UI changes, you re-record with one command instead of by hand.

It works with any terminal program in any language: a CLI, a TUI, a REPL, a shell script. One
binary for Linux, macOS and Windows, written in Go.

**Contents:** [Why](#why) · [Features](#features) · [Install](#install) ·
[Quick start](#quick-start) · [Commands](#commands) · [How a take works](#how-a-take-works) ·
[photon.yaml](#photonyaml-reference) · [Storyboard](#storyboard-reference) ·
[Targets and areas](#targets-and-areas) · [Expects](#expects) ·
[Services and profiles](#services-and-profiles) · [Sections and cursor](#sections-and-cursor) ·
[Environment variables](#environment-variables) · [Fonts](#fonts-and-licences) · [FAQ](#faq) ·
[Development](#development) · [License](#license)

## Why

A demo GIF recorded by hand goes stale the day the UI changes. Recording it again takes many
tries: a typo, a slow network call, a missed click. photon turns the demo into a file you keep in
git. It plays the script, checks the result, and retries until the take is clean. The GIF
always matches the current app.

## Features

- **Storyboards in YAML.** One file per clip: what happens on screen, how long each part plays,
  and what a good take must show.
- **Real input.** Human-like typing, named keys, and real mouse clicks and drags sent as SGR
  mouse reports. Your app reads them through its own input code.
- **The real app in a real pty.** No mock terminal. photon launches your program and records
  every screen change.
- **Checked takes.** Expects assert what must appear on screen, or run a shell command (did the
  app save the file? did the fake backend get the request?). `record` exits 0 only on a clean take.
- **One-line retake loop.** `until photon record clip.yaml; do :; done`.
- **Fixed section lengths.** Each beat plays for exactly its `duration`. Slow parts speed up to fit.
- **Camera and cursor.** Camera positions aim at a cell or on-screen text and glide from one to
  the next; zooms push in on them. `photon tune` edits both on timeline lanes, with a marker you
  drag on the preview and a picker that sets a position's target. A drawn cursor glides to each
  click and follows each drag.
- **Helper services.** Start a fake API or other helper before the take, on a free port, and stop
  it after.
- **Small GIFs.** A per-clip palette via ffmpeg, plus an optional gifsicle pass.
- **Same output everywhere.** Fonts are built into the binary, so a clip renders the same on
  every machine.

## Install

Homebrew (macOS, Linux):

```sh
brew install airiclenz/tap/photon
```

Scoop (Windows):

```sh
scoop bucket add airiclenz https://github.com/airiclenz/scoop-bucket
scoop install airiclenz/photon
```

winget (Windows):

```sh
winget install AiricLenz.Photon
```

Homebrew, Scoop and winget pull in ffmpeg. A new version reaches winget a little after the
others, once winget's review of it is done.

npm (Linux, macOS, Windows; Node.js):

```sh
npm install -g @airiclenz/photon
```

npm does not install ffmpeg: install it yourself (see below).

From source (Go 1.26 or later):

```sh
go install github.com/airiclenz/photon/cmd/photon@latest
```

Prebuilt archives are on the [releases page](https://github.com/airiclenz/photon/releases): a
`photon_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows) for Linux, macOS and Windows on amd64
and arm64, with `SHA256SUMS`. Unpack it, put `photon` on your `PATH`, and install ffmpeg yourself.

photon uses these external programs:

- **ffmpeg** (needed by `photon render`): encodes the GIF. Get it from your package manager
  (`apt install ffmpeg`, `brew install ffmpeg`, ...).
- **gifsicle** (optional): if it is on `PATH`, `photon render` shrinks the GIF with a lossless
  `gifsicle -O3` pass.
- **Chromium or Chrome** (needed by `photon record` on a `launch.url` clip): films the page,
  headless. photon finds it on `PATH` or where it is usually installed; `launch.browser` names
  another.

Recording needs a unix pty. `photon setup` and `photon record` run on Linux and macOS only.
`photon lint`, `photon check`, `photon render` and `photon tune` run everywhere.

## Quick start

The repository has a small example, `examples/hello`. photon films its own test TUI: a title, a
list, an input line and a status row. You need Go, ffmpeg and a checkout of this repository.

```sh
git clone https://github.com/airiclenz/photon
cd photon
go install ./cmd/photon

photon lint   examples/hello/hello.yaml   # check the storyboard and its photon.yaml
photon setup  examples/hello/hello.yaml   # build the test app into the work dir, once
photon record examples/hello/hello.yaml   # record one take and check it
photon render examples/hello/hello.yaml   # write the GIF to examples/hello/out/hello.gif
```

`record` prints one row per expect. It exits 0 when every beat and expect passed:

```text
beat 1 | PASS | seen /type and enter to submit/ at 0s
beat 2 | PASS | seen /you typed: hello, photon/ in area status at 540ms
beat 3 | PASS | seen /\* beta/ in area list at 624ms
beat 3 | PASS | seen /selected: beta/ in area status at 624ms
take   | PASS | seen /photon fixture/ at 0s
take   | PASS | exit 0
```

`render` prints the GIF's path, size and length, then the speed of each beat:

```text
examples/hello/out/hello.gif  28K  7s
  beat 1  1.00×  0s of take in 2s
  beat 2  1.00×  618ms of take in 3s
  beat 3  1.00×  41ms of take in 2s
```

Use `-o <file>` to write the GIF elsewhere. Use `--work <dir>` on any command to keep the work
dir somewhere other than `~/.cache/hello-photon`.

**To film your own app:** copy the two files from `examples/hello` next to your code. Point
`setup` at your build and `launch.command` at your program. Then rewrite the beats.

## Commands

Every command except `photon version` takes the storyboard as its first argument. Each finds the
config the same way: `--config <file>` if given, else the nearest `photon.yaml` in or above the
storyboard's folder. Each command's `--help` says the same as this section.

| Command | What it does |
| --- | --- |
| `photon lint <storyboard>` | Loads the storyboard and its config and prints every problem in both, then lays the captions out with the config's fonts and reports a caption box larger than the frame. Prints `ok` with the clip name, beat count and config path if there are none. Records nothing. On a `launch.url` clip it starts no browser: it reports each frame key, selector and area the launch refuses, and lays the captions out on the viewport in CSS pixels (see [Targets and areas](#targets-and-areas)). |
| `photon setup <storyboard>` | Creates the work dir and runs the config's `setup` once. |
| `photon record <storyboard>` | Records one take to `<work>/<clip>.take`, checks it and prints the check table. |
| `photon check <storyboard> [<take>]` | Checks a take (default `<work>/<clip>.take`) against the storyboard's expects and prints the check table. |
| `photon render <storyboard> [<take>]` | Renders a take (default `<work>/<clip>.take`) to the GIF at the storyboard's `ship` path. On a `launch.url` clip each frame is the page as recorded, with no terminal font loaded; the camera, the cursor and the captions are drawn over it, a caption's `font.size` in CSS pixels as `photon lint` lays it out. |
| `photon tune <storyboard> [<take>]` | Serves a page on `127.0.0.1` to tune the storyboard's timing, camera positions and zooms, and frame against a take (default `<work>/<clip>.take`), and saves the edits back into the storyboard. Runs until Ctrl-C. |
| `photon version` | Prints photon's build string, such as `photon 1.0.0 (a1b2c3d)`: the version, the commit it was built from and `, dirty` when that tree had uncommitted changes; `photon 1.0.0` alone when the binary carries no VCS info. `photon --version` does the same. |

Flags:

| Flag | Commands | Meaning |
| --- | --- | --- |
| `--config <file>` | all | Use this config instead of the nearest `photon.yaml`. |
| `--work <dir>` | setup, record, check, render, tune | The work dir. |
| `--profile <name>` | record | The profile to record with (its services and `on_success`). |
| `-o`, `--output <file>` | render | Write the GIF here instead of the `ship` path. Its folder is created. |
| `--dry-run` | render | Print both ffmpeg command lines, palette pass first, and exit without rendering. |
| `--watch` | render | Render once, then again each time the storyboard, config, take or a caption font file changes. A failed load or render prints its error and the watch goes on. Runs until Ctrl-C. |
| `--port <n>` | tune | Serve the page on this port of `127.0.0.1`. The default, `0`, picks any free port. |

**The work dir** is the first of: `--work`, `$PHOTON_WORK`, the config's `work`, and
`~/.cache/<name>-photon` (`<name>` is the config's `name`). It holds what `setup` builds, every
take, each service's log (`<work>/<service>.log`), and the clip's palette
(`<work>/<clip>.palette.png`) while `render` runs.

**Tuning** with `photon tune` prints the page's address, such as
`photon tune: http://127.0.0.1:41234/`, and never opens a browser. The page previews the take:
while it plays or scrubs it shows draft frames, in either view, drawn from a raster at the
shipped size with a fast scaler, and once stopped it settles on the exact frame: `photon render`'s
composed frame, quantized to `frame.max_colors` colours as the GIF is. Its timeline edits
each beat's `duration`, `hold` and `cut`, and its inspector the clip's `frame.width`,
`fps`, `font_size`, `line_height`, `padding`, `scale`, `cols`, `rows` and `max_colors`, each
refused outside the range the loader takes. A `cols` or `rows` other than the take's keeps the
preview at the take's recorded size and stands as a clip problem until the take is re-recorded
with `photon record`; Save still writes it, and `photon render` refuses the mismatch. The
storyboard's `camera.positions`, `camera.zooms` and `captions` show on lanes of their own under
the beats. The position lane draws each position as a bar over its `hold`, or as a point when it
has none, and each transition, the gap into the next position by start, as a bar in its own
colour; the zoom lane draws one bar per zoom, labelled with the factor it zooms to; the captions
lane draws each caption in a row of its own, in list order with the last listed, which the clip
draws on top, in the top row. A click on a
bar shows that entry in the inspector, which edits a position's `at`, `hold` and its `cell` or
text target (`text`, `nth`, `area`), a transition's start, length and ease (the `ease` of the
position it leads into), a zoom's `at`, `over`, `to` and `ease`, and a caption's `text`, `at`,
`for` or `until`, `place`, `align`, `offset` or `position`, font, colours, box and fade. Dragging
a bar moves it in 50ms steps. Dragging a caption up or down its lane instead reorders it: it moves
to the list place of the row it is let go in, and the drag takes whichever way the pointer moved
further in its first 6 px. A position's or a transition's edge grips move just that edge, and
a transition shrunk to 0 is a hard cut. A transition has no grip on an edge that meets a point:
that edge moves with the point's marker. The zoom and captions lanes' `+` adds a zoom or a caption
at the playhead; the position lane's `+` adds a point there on the cell the camera is centred on,
so the camera does not move. The inspector removes the entry it shows, and moves a caption up or
down its list. While a position is selected the preview shows the full view, the whole frame,
in drafts too while it plays or scrubs; once paused it draws the camera's crop as a dashed
rectangle and a marker on the cell the position aims at. Dragging the marker to another cell sets the position's `cell`.
A pick on the paused preview proposes a target for the picked cells, ready to set as the selected
position's text target with **Set as position target**, or to copy as a `click`, `seen` or `drag`
end.
`pause` and the `do` actions show read-only: changing them needs a new `photon record`. Save
writes the storyboard in place and changes only
the edited values, so comments and layout stay. It is refused while an edit leaves the
storyboard invalid, or when the file changed on disk since it was loaded. **Export GIF**, beside
Save, renders the saved storyboard to its configured output, the same GIF `photon render` writes,
and shows its progress on the button; the status then names the file written or why the render
failed. It is disabled while edits are unsaved, an error stands, the file changed on disk, or an
export runs. The transport bar's
right end shows the build serving the page, the string `photon version` prints.

**Exit status** is the same for every command:

| Status | Meaning |
| --- | --- |
| `0` | Success: the clip is clean (lint), setup ran, every beat and expect passed (record, check), the GIF was written (render), the watch ran until Ctrl-C or SIGTERM (render `--watch`), the page was served until Ctrl-C (tune). |
| `1` | The run failed: a problem in a file, a failed command, beat or expect, or a take that could not be recorded, rendered or tuned, or a page that could not be served. |
| `2` | A bad command line, such as an unknown flag or a missing argument. The usage text is printed. |

Errors go to stderr as `photon: <error>`.

## How a take works

`photon record` does these steps in order:

1. Creates the work dir and runs the config's `reset` to restore a clean state.
2. Starts the services (under `--profile` if given) and waits until each is ready.
3. Launches `launch.command` in a pty of `frame.cols` × `frame.rows` and waits for its first paint.
   On a url launch it opens `launch.url` in headless Chromium at `frame.viewport` instead.
4. Runs every beat's actions in order. If an action fails, its beat fails and the take ends.
   Examples: a `wait` times out, or a `click` or `drag` target is not on screen.
5. Ends the take. It waits until the program paints nothing for `end.idle` (at most
   `end.settle_timeout`). It freezes the take, so nothing painted on the way out is kept. On a
   command launch it types `end.keys`. If the program is still running `end.kill_after` later, it
   kills its whole process group. On a url launch it closes the browser.
6. Writes the take to `<work>/<clip>.take` and prints its path on stderr.
7. If every beat passed: checks the take while the services still run, prints the check table on
   stdout, stops the services and runs the profile's `on_success`.

A take with a failed beat is still written, so you can see why. It is not checked, and
`on_success` does not run. Output from `reset`, the services and `on_success` goes to stderr.
Only the check table goes to stdout.

Some takes fail for reasons outside the script, such as a slow service or a race in the app.
`record` exits 0 only when every beat and expect passed, so the retake loop is one line:

```sh
until photon record docs/demo/tour.yaml; do :; done
photon render docs/demo/tour.yaml
```

`photon check` re-checks a saved take without recording again. Use it to try out an edited
expect. `photon render` only reads the take. It never runs the program.

A take file is gzip-compressed JSON lines. It holds every full-screen snapshot with its time, and
a log of what photon did: beat starts, actions, found targets, clicks, and each drag's press,
moves and release. It has a format version. photon refuses a newer or unknown file.

## photon.yaml reference

One `photon.yaml` per project, shared by all its storyboards. Unknown fields are an error, and
all problems are reported at once. Relative paths are relative to the file's folder. `setup`,
`reset`, services and `on_success` run in that folder.

```yaml
name: myapp                       # required; lowercase letters, digits and -
work: .rig                        # optional; default ~/.cache/myapp-photon
setup: make build && cp bin/myapp "$PHOTON_WORK/"
reset: rm -rf "$PHOTON_WORK/home" && mkdir "$PHOTON_WORK/home"

launch:
  command: exec "$PHOTON_WORK/myapp"   # required
  dir: .                               # relative to this file; default this file's dir
  env:
    HOME: ${PHOTON_WORK}/home
    MYAPP_API: http://127.0.0.1:${PHOTON_PORT_API}
  strip: ['MYAPP_*']

services:
  api:
    command: go run ./cmd/fakeapi --port "$PHOTON_PORT"
    port: 0
    ready: {tcp: true}
    timeout: 10s

profiles:
  slow:
    services:
      api: {env: {FAKEAPI_DELAY: 2s}}
    on_success: cp "$PHOTON_TAKE" takes/slow.take

end:
  keys: [ctrl+c]
  idle: 1s
  settle_timeout: 15s
  kill_after: 10s

areas:
  list:
    top: {match: '^Items$', offset: 1}
    bottom: {match: '^> ', offset: -1}

theme:
  fg: '#cdd6f4'
  bg: '#0e1117'

fonts: fonts/
```

| Field | Meaning |
| --- | --- |
| `name` | Required. The project's name: lowercase letters, digits and `-`. Names the default work dir. |
| `work` | The work dir. `--work` and `$PHOTON_WORK` override it. Default `~/.cache/<name>-photon`. |
| `setup` | A command `photon setup` runs once to build what the takes need. |
| `reset` | A command run before every take to restore a clean state. It runs before the services start, so it sees `PHOTON_WORK` but no ports. |
| `launch` | The program to film. |
| `launch.command` | Required unless `launch.url` is set. Run as `sh -c '<command>'`. No `exec` is added, so `. env.sh && exec app` works. The whole process group is killed at the end of the take. |
| `launch.url` | Instead of `launch.command`: a page to film in a browser, an `http` or `https` url. Set exactly one of the two. `${NAME}` in it expands from the launch env, so `http://127.0.0.1:${PHOTON_PORT_WEB}/` reaches a service; a name the env does not hold fails `photon record` before the browser starts. photon films the page in headless Chromium or Chrome. |
| `launch.browser` | With `launch.url` only: the browser binary to drive, instead of the one photon finds. |
| `launch.dir` | The working directory, relative to the config's folder. Default: the config's folder. |
| `launch.env` | Variables set for the program. Values may use `${PHOTON_*}`, which photon expands. |
| `launch.strip` | Globs of variable names (`MYAPP_*`) removed from your environment before launch, so your own settings cannot change the app. Not with `launch.url`. |
| `services` | Helper processes a take starts, by name. See [Services and profiles](#services-and-profiles). |
| `profiles` | Named variants of the services, picked with `photon record --profile`. |
| `end` | How a take ends. |
| `end.keys` | Keys typed to quit the program on a command launch, by [key name](#key-names). Default `[ctrl+c]`. Refused with `launch.url`: a url take ends when the page settles. |
| `end.idle` | How long the program must paint nothing before the take ends: on a command launch, before the keys are sent. Default `1s`. |
| `end.settle_timeout` | The longest photon waits for that idle. Default `15s`. |
| `end.kill_after` | How long after the keys a still-running program is killed. Default `10s`. |
| `areas` | Named bands of screen rows. See [Targets and areas](#targets-and-areas). |
| `areas.<name>.selector` | With `launch.url` only: a CSS selector whose element's box is the area, in place of `top` and `bottom`. |
| `theme` | The colours used to draw frames. |
| `theme.fg`, `theme.bg` | The default foreground and background, `#rrggbb`. Default `#cdd6f4` on `#0e1117`. |
| `theme.ansi` | The 16 ANSI colours, normal then bright: all 16 or none. Default: Catppuccin Mocha. Colours 16-255 use the standard xterm palette. Truecolor is drawn as is. |
| `fonts` | A folder of fonts that replaces the built-in terminal fonts. See [Fonts and licences](#fonts-and-licences). |
| `caption_font` | Font files that replace the built-in caption font. See [Fonts and licences](#fonts-and-licences). |
| `caption_font.regular`, `caption_font.bold` | The regular and bold caption font files, relative to the config's folder. Each must be a readable file, not a directory. A style left out keeps Source Sans 3. |

Every command (`setup`, `reset`, `launch.command`, a service's `command`, `on_success`, a `run`
expect) is one string run by `sh -c`, with the `PHOTON_*` variables set. photon expands
`${PHOTON_*}` only in `env` values. Everywhere else the shell does it.

Durations use Go syntax: `300ms`, `2s`, `1m30s`.

### Key names

`enter`, `esc`, `tab`, `shift+tab`, `space`, `backspace`, `ctrl+c`, `up`, `down`, `left`,
`right`, `pgup`, `pgdown`, `alt+up`, `alt+down`, `f1` to `f12`. photon sends the same bytes as
xterm, so your program reads them through its own input parser.

## Storyboard reference

One storyboard per clip. Unknown fields are an error, and all problems are reported at once
(`photon lint` shows them).

```yaml
clip: tour                      # required; names the take, <work>/tour.take
ship: ../tour.gif               # required; relative to this file
frame:
  cols: 100                     # the pty's size, in cells
  rows: 30
  padding: 16                   # pixels around the grid
  font_size: 14                 # points
  line_height: 1.2              # a multiple of the font size
  scale: 2                      # frames are drawn at this scale, then downscaled
  width: 800                    # the GIF's width, in pixels
  fps: 24
  max_colors: 128               # palette size, 2..256

expect:                         # what the whole take must show
  - run: test -s "$PHOTON_WORK/home/saved.json"

beats:
  - id: 1
    title: open the list
    why: the first paint shows the whole screen
    notes: free text for the storyboard's reader
    duration: 3s
    hold: 1s
    do:
      - wait: {screen: 'Items', timeout: 5s}
      - key: {name: down, repeat: 2}
      - type: {text: 'hello', humanize: false}
      - pause: {for: 500ms}
      - click: {target: {text: 'beta', nth: first, area: list}, times: 1}
      - wait: {screen: 'loading', gone: true, timeout: 10s}
    expect:
      - seen: '\* beta'
        area: list
  - id: 2
    title: the boring part
    cut: true
    do:
      - pause: {for: 2s}

camera:                         # push in on beta
  positions:                    # where the camera aims
    - {text: 'beta', area: list, at: {beat: 1, offset: 1500ms}, hold: 1s}
  zooms:                        # how far it magnifies
    - {to: 1.5, at: {beat: 1, offset: 1500ms}, over: 800ms}
```

### Header

| Field | Meaning |
| --- | --- |
| `clip` | Required. The clip's name. The take is `<work>/<clip>.take`. |
| `ship` | Required. Where `photon render` writes the GIF, relative to the storyboard's folder. |
| `frame` | The terminal size and the GIF's shape. Every field except `padding` is required. With `launch.url` the frame sets `viewport` instead of `cols`, `rows`, `padding`, `font_size` and `line_height`, which it then must not set. |
| `frame.cols`, `frame.rows` | The pty's size in cells, ≥1. `render` refuses a take recorded at another size; `tune` edits them, previewing at the take's size with a re-record problem. |
| `frame.viewport` | With `launch.url` only, and required there: the page's `width` and `height` in CSS pixels, `{width: 1280, height: 800}`, each ≥1. One CSS pixel is one cell, so a camera `cell` is a CSS pixel inside the viewport. A caption's `font.size` is then in CSS pixels, default 16. `render` refuses a take recorded at another viewport, or a page's take under a frame without one. |
| `frame.device_scale` | With `launch.url` only: device pixels per CSS pixel the page renders at, 1..4. Default 1. `render` refuses a take recorded at another device scale. |
| `frame.padding` | Pixels of background around the grid, ≥0. |
| `frame.font_size` | The font size in points, 6..72, the range a caption's `font.size` takes. A column is the width of Source Code Pro's `M`. A row is `font_size` × `line_height`. |
| `frame.line_height` | Row height as a multiple of the font size, >0. |
| `frame.scale` | Frames are drawn at this scale, then downscaled to `width`, ≥1. |
| `frame.width` | The GIF's width in pixels, ≥1. The height keeps the aspect ratio. |
| `frame.fps` | The GIF's frame rate, ≥1. |
| `frame.max_colors` | The palette size, 2..256. The palette is built per clip, without dithering. |
| `expect` | Take-level expects, checked on every screen of the take. See [Expects](#expects). |
| `beats` | The beats, at least one. |
| `camera` | The camera on the clip's clock, as two lists. `camera.positions` says where it aims: each position is a `cell` or a target (`text` or `selector`, `nth`, `area`), with `at` (a clip time, or a `beat` and an `offset`), `hold` and `ease`; the gap between two positions is a transition. `camera.zooms` says how far it magnifies: each zoom has `to` (a factor), `at`, `over` and `ease`. See [Camera](#camera). |
| `captions` | Text drawn over the composed frame on the clip's clock: each has `text`, `at`, and `for` or `until`; a `font` (`size`, `weight`); a `place` and `align` with an `offset`, or a `position`; a `color`, a `box` (`color`, `opacity`) and a `fade` (`in`, `out`). See [Captions](#captions). |

### Beats

| Field | Meaning |
| --- | --- |
| `id` | A positive whole number, unique and rising through the file. |
| `title` | Required. A short name, shown by `photon check` and `photon render`. |
| `why` | Free text: why the beat is in the clip. |
| `notes` | Free text for the reader. |
| `do` | The actions, run in order. |
| `duration` | How long the beat plays in the clip, >0. Not needed on a cut beat. |
| `hold` | How much of the start plays at 1×, ≥0 and less than `duration`. Default 0. |
| `cut` | `true` records the beat but leaves it out of the clip. |
| `expect` | What the beat must show. See [Expects](#expects). |

### Actions

Each entry of `do` sets exactly one of these.

| Action | Fields | What it does |
| --- | --- | --- |
| `type` | `text` (required), `humanize` (default `true`) | Types the text. Human-like typing spaces letters 25-45 ms apart, spaces 60-90 ms, punctuation 90-140 ms, with an occasional pause at a space. It uses a fixed seed, so every take types the same. `humanize: false` sends the text in one write. |
| `key` | `name` (required), `repeat` (default 1) | Presses a [named key](#key-names) `repeat` times. |
| `click` | `target` (required), `times` (default 1), `button` (default `left`), `with` | Finds the target on the current screen and sends `times` clicks of `button` at its centre cell, holding the `with` modifiers, as real SGR mouse press and release reports. Fails if the target is not on screen, so put a `wait` first. |
| `drag` | `from` (required), `via`, `to` (required), `steps` (default 12), `button` (default `left`), `with` | Finds `from` on the current screen and presses `button` at its centre, holding the `with` modifiers. Moves through each `via` point in turn to `to`, each leg in `steps` moves that speed up and slow down, then releases. All as real SGR mouse reports. Fails if a target is not on screen. |
| `wait` | `screen` (required), `gone`, `timeout` (required, at most `180s`) | Waits until the `screen` regex matches the screen text (rows joined by newlines). With `gone: true`, waits until it no longer matches. Fails at the timeout, or at once if the program exits. |
| `pause` | `for` (required) | Waits for the given time. |

`button` is `left`, `middle` or `right`. `with` names the modifier keys held while the button
acts: `shift`, `alt` and `ctrl`, joined with `+` in any order (`ctrl+shift`). Leave it out to hold
none.

A drag's `to` is where it releases. Its `via` is a list of points it passes through on the way,
in order, with the button held. `to` and each `via` entry are one of two kinds:

- `to.text` or `via.text`: a [target](#targets-and-areas), with `to.nth` and `to.area` (`via.nth`
  and `via.area`). The drag releases at, or passes through, its centre cell.
- `to.offset` or `via.offset`: a move in cells from the drag's previous point: the `via` entry
  before it, or the press cell when none comes before it. `to.offset.x` is columns right and
  `to.offset.y` is rows down. Negative goes left and up. At least one must be non-zero. An offset
  takes no `nth` or `area`.

Every point, `to` included, is found on the screen as it stands when the drag presses. Each leg
between two points takes its own `steps` moves.

```yaml
- drag: {from: {text: '│', area: panes}, to: {offset: {x: 12}}, steps: 8}
- drag: {from: {text: 'draft.txt'}, to: {text: 'Trash', area: sidebar}}
- click: {target: {text: 'notes.md'}, button: right}
- drag:
    from: {text: 'photo.png'}
    via: [{offset: {x: 6, y: -2}}, {text: 'Archive', area: sidebar}]
    to: {text: 'Trash', area: sidebar}
    button: right
    with: ctrl+shift
```

Your program must turn on mouse reporting itself for a click or drag to reach it. photon only
sends the reports a terminal would. A drag also needs button-event (`1002`) or any-event (`1003`)
tracking, the modes in which a terminal reports moves while a button is held.

### Camera

`camera` aims and magnifies the camera on the clip's own clock, the timeline of the shipped GIF,
not the take's. It is a mapping with two lists, both optional: `camera.positions` says where the
camera aims and `camera.zooms` how far it magnifies. Either list may be written in any order:
photon places each entry by its start.

#### Positions

A **position** is one place the camera aims at, from its `at` for its `hold`:

| Field | Meaning |
| --- | --- |
| `at` | When the position starts. A bare time such as `4500ms` is that long after the clip's first frame. `{beat: N, offset: D}` is where beat N's section starts in the clip, plus `D`. `D` may be negative and defaults to 0. A cut beat plays no time, so on a cut beat `{beat: N}` is where the cut lands: where the next kept beat starts, or the clip's end when no kept beat follows, which makes a trailing cut beat need a negative `D`. Leave `at` out to start at the clip's first frame. |
| `hold` | How long the camera rests there, ≥0. Leave it out, or set 0, and the position is a point in time. It must end by the clip's end. |
| `cell` | `{x, y}`: one cell of the grid, blank or not. `x` is the column and `y` the row, both counted from 0. The camera aims at the cell's centre. A cell takes no `nth` or `area`. |
| `text`, `nth`, `area` | A [target](#targets-and-areas). The camera aims at the centre of its box on the screen shown when the position starts, and stays on that spot for the whole hold: it does not follow the text. `photon render` fails, naming the position, if that screen does not show it. |
| `ease` | The ease of the transition into this position. See [Eases](#eases). |

Set exactly one of `cell` and `text`. Before the first position the camera holds the first one,
and after the last it holds the last one. With no positions it aims at the frame's centre.

#### Transitions

A **transition** is the gap between one position's end and the next one's start. It is not
written down: the two positions around it make it. Across it the camera glides from the centre
of the view shown as the earlier position ends to the centre of the view shown as the later one
starts, each taken at that moment's zoom, and paced by the later position's `ease`. A gap of 0 is
a hard cut. Positions may not overlap, and two that start at the same time overlap.

#### Zooms

A **zoom** eases the magnification to `to` and holds it there until the next zoom. At rest the
camera is at 1×. A zoom aims nowhere: it pushes in on wherever the positions aim at that moment,
so it may overlap positions and transitions freely. Two zooms may not overlap.

| Field | Meaning |
| --- | --- |
| `to` | The factor, from 1 to 3. `to: 1` zooms back out. |
| `at` | When the zoom starts, in the same two forms as a position's `at`. |
| `over` | How long the zoom runs, >0. It must end by the clip's end. |
| `ease` | How the zoom runs over its length. See [Eases](#eases). |

#### Eases

An `ease` sets the pace of a transition or a zoom. It takes one of four values:

- `smooth`, the default, starts and ends gently.
- `linear` runs at a steady pace.
- `ease-in` starts slow and speeds up to the end.
- `ease-out` starts fast and slows down to settle.

```yaml
camera:
  positions:
    - {cell: {x: 10, y: 4}, hold: 2s}                   # rests on a cell from the first frame
    - {text: 'beta', area: list, at: 3s, hold: 1500ms}  # glides there from 2s to 3s
    - {text: 'Saved', at: 4500ms, hold: 2s}             # a gap of 0: a hard cut
    - {cell: {x: 80, y: 4}, at: 8s, ease: ease-out}     # glides from 6500ms, settling at 8s
  zooms:
    - {to: 2, at: 1s, over: 600ms}
    - {to: 1, at: 9s, over: 800ms, ease: ease-in}
```

The crop never leaves the frame. When the camera aims near an edge, the crop stops at that edge,
so the aimed point sits off-centre rather than showing empty space. At 1× the whole frame is in
view, so a position shows only while the camera is zoomed in.

`photon lint` reports a position with both or neither of `text` and `cell`, a cell outside the
grid, an `nth` or `area` on a cell, a negative `hold`, a zoom's `to` outside 1-3, an `over` that is
not >0, an `ease` other than the four above, an `at` that names a missing beat, a position
or zoom that starts outside the clip or ends after it, and two positions or two zooms that
overlap.

### Captions

The top-level `captions` list draws text over the clip, on the same clock as the
[camera](#camera). A caption sits on the shipped frame itself: it is drawn after the camera's crop
and the downscale to `frame.width`, and over the cursor. A zoom or a position never moves or
scales it. Captions may overlap one another; one listed later draws over one listed earlier.
To reorder captions in `photon tune`, drag one up or down the captions lane, or use the
inspector's **Move up** and **Move down**.

| Field | Meaning |
| --- | --- |
| `text` | Required. The caption's text. Each line of a YAML block (`text: \|`) is a line of the caption. A line too long to fit wraps at its spaces. Tags inside it style a span of the text: see [Styled spans](#styled-spans). |
| `at` | When the caption appears, in the same two forms as a camera position's `at`: a bare time such as `4500ms`, or `{beat: N, offset: D}`. Leave it out to start at the clip's first frame. |
| `for`, `until` | When it goes. Set exactly one: `for` is how long it shows, >0; `until` is the moment it ends, in the same forms as `at`. It must end after it starts and by the clip's end. |
| `font.size` | The text size, in the same unit as `frame.font_size`, 6..72. Default `frame.font_size`, which matches the terminal text's size; with `frame.viewport`, CSS pixels, default 16. |
| `font.weight` | `regular` (default) or `bold`. See [Fonts and licences](#fonts-and-licences). |
| `place` | `top`, `middle` or `bottom` (default): where the caption sits vertically. |
| `align` | `left`, `middle` (default) or `right`: where the caption sits across the frame, and how its lines line up. |
| `offset` | `{x, y}`: moves the placed caption, in design pixels. `x` is right and `y` is down. Negative goes left and up. |
| `position` | `{x, y}`: puts the caption's box with its top-left corner here, in design pixels, instead of `place`, `align` and `offset`. Its lines line up left. |
| `color` | The text colour, `#rrggbb`. Default `#ffffff`. |
| `box.color` | The colour of the box behind the text, `#rrggbb`. Default `#000000`. |
| `box.opacity` | The box's opacity, 0..1. Default `0.6`. `0` draws no box. |
| `fade.in`, `fade.out` | How long the caption fades in after it appears and out before it goes, ≥0. Default 0, which cuts it in and out. The two together must fit in the caption's length. |
| `width` | The widest the box may be, in design pixels, ≥0: text wraps to fit it. Default `0`, which lets the box grow to the frame's width less a margin on each side. |

A line of caption text too long for the box wraps at its spaces, so the box fits the frame less a
margin on each side, or `width` when that is narrower. A line that fits is kept as written. A word
too long to fit stays whole on a line of its own. A placed caption sits a margin in from the
frame's edge it is placed against, or centred, and is then moved by its `offset`. The box pads the
text on every side. Whatever its placement, the box is kept inside the frame: one that would cross
an edge stops at that edge. A caption's box larger than the frame is a problem: `photon lint`
reports it, laying the captions out with the config's fonts, and `photon render` fails on it.

A design pixel is a pixel of the frame drawn at scale 2, before the downscale to `frame.width`.
photon scales every caption length with the frame, so a caption keeps its size and place against
the terminal text whatever `frame.scale` and `frame.width` are.

```yaml
captions:
  - text: Pick an item
    at: {beat: 1, offset: 500ms}
    for: 2s
    fade: {in: 200ms, out: 200ms}
  - text: |
      Saved.
      Nothing to set up.
    at: 7s
    until: {beat: 4}
    font: {size: 18, weight: bold}
    place: top
    align: right
    offset: {x: 0, y: 12}
    color: '#ffe08a'
    box: {color: '#202020', opacity: 0.8}
  - text: v1.2
    at: 0s
    for: 3s
    position: {x: 20, y: 20}
    box: {opacity: 0}
```

A caption changes only the rendered GIF, not what photon records. After you edit `captions`, run
`photon render` again; the take stays as it is.

`photon lint` reports a caption with no `text`, a caption that sets both or neither of `for` and
`until`, a `for` that is not >0, an `at` or `until` that names a missing beat, a caption
that starts outside the clip, ends before it starts or ends after the clip, a `font.size` outside
6..72 (its own, or the `frame.font_size` it falls back to), a `font.weight` other than `regular`
or `bold`, a `place` or `align` that is not one of its words, a `position` set with `place`,
`align` or `offset`, a colour not spelled `#rrggbb`, a `box.opacity` outside 0..1, a negative
fade, fades that together outlast the caption, and malformed span markup: an unknown token, two
colours in one tag, a tag with no token or no closing `]`, a span never closed, a `[/]` with no
span open, a tag inside a span or a span with no text. Once the storyboard and its config are
clean, it lays the captions out with the config's fonts and reports a caption whose box is larger
than the frame.

#### Styled spans

Inline tags in `text` draw a span of it in another colour, in bold, or both:

- `[#rrggbb]` opens a span in that colour.
- `[bold]` opens a span in the bold face.
- `[bold #rrggbb]` opens a bold span in that colour.
- `[/]` closes the open span.

The text outside a span keeps the caption's own `color` and `font.weight`, and a span keeps
whichever of the two its tag leaves out. A literal `[` is written `[[`. Spans do not nest, a span
may run across a line break, and a span that is too long for its line wraps at its spaces like
the rest of the text.

```yaml
captions:
  - text: 'Press [bold #ffe08a]Enter[/] to save, [[Esc] to cancel'
    at: 2s
    for: 3s
```

## Targets and areas

A **target** is a place on screen that a `click`, `drag` or camera position aims at:

| Field | Meaning |
| --- | --- |
| `text` | Required, unless `selector` is set. A regex (Go syntax) matched against one screen row at a time. It never spans rows. Rows are plain text with trailing blanks trimmed. |
| `selector` | With `launch.url` only, instead of `text`: a CSS selector, the box of the element it picks, as recorded with each frame. photon checks only its shape: not blank, with every bracket, paren and quote closed. |
| `nth` | Which match counts: `first` (default), `last`, or a positive number. |
| `area` | The area to search. Empty or `any` searches the whole screen. |

An **area** is a named band of rows. You declare it in `photon.yaml` under `areas`. photon finds
it again on every screen, so it follows your layout as it moves. An area has a `top` and a
`bottom` edge, both inclusive. A missing edge is the screen's edge, so an area with neither is
the whole screen. Each edge is a row **anchor**:

| Field | Meaning |
| --- | --- |
| `match` | Required. A regex. The rows it matches are the candidates. |
| `pick` | `first` (default) or `last`: which candidate to take. |
| `above` | Another anchor. Only rows strictly above the row it picks count. |
| `below` | Another anchor. Only rows strictly below the row it picks count. Use at most one of `above` and `below`. |
| `offset` | Rows to move from the picked row. Negative is up. |

```yaml
areas:
  # The rows between the title and the input line.
  list:
    top: {match: '^Items$', offset: 1}
    bottom: {match: '^> ', offset: -1}
  # The rows below the header rule down to the last rule above the input box.
  transcript:
    top: {match: '^─+$', offset: 1}
    bottom:
      match: '^─+$'
      pick: last
      above: {match: '^> '}
      offset: -1
```

With `launch.url`, an area may be a CSS `selector` instead: the box of the element it picks, as
recorded with each frame. A selector area has no `top` or `bottom`. photon checks only its shape:
not blank, with every bracket, paren and quote closed. On a url launch the text anchors read
bands of the page, not rows, so they take no `offset`.

```yaml
areas:
  sidebar:
    selector: 'nav[aria-label="Main"] > ul'
```

**On a page.** A url launch films a page, and each frame of it records the page's text and the
boxes of every selector the clip names, its storyboard targets and its config's selector areas.
The grid is the page in CSS pixels: one cell is one CSS pixel, so a camera `cell` is a point of
the viewport.

- A **selector target** is one of the boxes its selector picked on that frame, taken in reading
  order (top edge, then left edge), so `nth` counts them the way it counts text matches. A
  `click` or `drag` aims at the box's centre.
- A **selector area** is the first box, in reading order, its selector picked. It holds only
  what lies wholly inside that box, text or selector boxes, and is not on screen when the
  selector picked nothing.
- A **text target** matches one run of the page's text at a time, the way it matches one row of
  a terminal. The runs whose top edges line up form a line.
- An **anchor area** is a band from the top of the line its `top` anchor picks to the bottom of
  the line its `bottom` anchor picks, across the whole page. An anchor matches a line's text, its
  runs joined by one space.
- A caption's `font.size` is in CSS pixels, default 16, and grows with the page: it is drawn at
  `font.size` × `frame.device_scale` × `frame.scale` pixels of the frame before the frame is
  scaled to `frame.width`, so it keeps its size against the page at any device scale.

`photon lint` checks a url-launch clip without starting a browser: the keys the launch refuses,
the shape of each selector, the areas the storyboard names, and the captions laid out on the
viewport. `photon setup` and `photon record` run a url launch as they run a command: `reset`,
the services, the profiles, `run` expects and `on_success` all work the same. `photon record`
opens the page in headless Chromium at `frame.viewport` and `frame.device_scale`, runs the beats
against it, and once the page has painted nothing for `end.idle` closes the browser.

If an anchor finds no row, or the top lands below the bottom, the area is not on screen. A
`click` or `drag` there fails, and a `seen` expect does not match. Area names start with a letter
and use letters, digits, `-` and `_`. `any` is reserved for the whole screen. A storyboard may
only use areas its config declares, and `photon lint` checks this. Clicks, drags, camera
positions and `seen` expects all use the same area logic, so they always agree.

## Expects

An expect is something a good take must show. It is judged PASS or FAIL. A beat's expects are
checked on the screens shown during that beat. The storyboard's top-level `expect` is checked on
every screen of the take. Each expect sets exactly one of:

| Field | Meaning |
| --- | --- |
| `seen` | A regex that must match an area's text (rows joined by newlines) on at least one of those screens. |
| `area` | With `seen` only: the area to read. Default: the whole screen. |
| `run` | A command. Exit 0 is PASS. It runs by `sh -c` in the work dir, with `PHOTON_WORK`, `PHOTON_TAKE`, each service's `PHOTON_PORT_<NAME>`, and for a beat's expect `PHOTON_BEAT`. Use it to check what photon cannot see: a file the app saved, a request a fake backend logged. |

`photon record` and `photon check` print one row per expect: beats in storyboard order, then the
take-level ones. A row reads `beat N | PASS | detail`, or `take | ...` for a take-level expect.
They exit 1 if any row is a FAIL. A screen that a `wait` matched is always kept in the take, so a
`seen` on a beat that ends in a `wait` finds the screen the wait saw. On Windows, a `run` expect
with no `sh` on `PATH` is a FAIL that says so.

## Services and profiles

A **service** is a helper process photon starts before the launch and stops after the take. For
example, a fake backend your app talks to.

| Field | Meaning |
| --- | --- |
| `command` | Required. Run by `sh -c` in the config's folder, in its own process group. |
| `env` | Variables set for it. Values may use `${PHOTON_*}`. |
| `port` | The port it serves: a fixed port, or `0` to let photon pick a free one. |
| `ready` | How photon knows it is up: exactly one of `tcp` and `line`. |
| `ready.tcp` | `true`: ready once its port accepts a connection. |
| `ready.line` | A regex: ready once a line of its output matches. |
| `timeout` | How long photon waits for it to be ready. Default `10s`. |

Services start in name order. Each one's port is exported as `PHOTON_PORT_<NAME>` (upper case,
`-` becomes `_`). The launch, later services, `run` expects and `on_success` all see it. The
service itself also gets its own port as `PHOTON_PORT`. Its output goes to `<work>/<name>.log`.
After the take, each service's process group gets SIGTERM, then a kill. Service and profile names
use lowercase letters, digits, `-` and `_`, and start with a letter.

A **profile** is a named variant of the services. Pick it with `photon record --profile <name>`:

| Field | Meaning |
| --- | --- |
| `services` | For each declared service, what changes under this profile. |
| `services.<name>.command` | Replaces the service's command. |
| `services.<name>.env` | Added on top of the service's env. |
| `on_success` | A command run after a take where every beat passed, once the take is checked and the services have stopped. It runs in the config's folder with `PHOTON_WORK`, `PHOTON_TAKE` and the ports set. |

A service's name, port and readiness stay the same under every profile, so its
`PHOTON_PORT_<NAME>` never changes.

## Sections and cursor

Each beat becomes a **section** of the clip. A section is exactly `duration` long, however long
the beat took in the take:

- A beat that took no longer than `duration` plays at 1×, then holds its last frame.
- A longer beat plays its first `hold` at 1× and speeds up the rest to fit. `photon render` warns
  when a section runs faster than 6×.
- A `cut` beat is recorded and checked but left out of the clip. If every beat is cut, there is
  nothing to render, and that is an error. A camera position, zoom or caption whose `at` or
  `until` names a cut beat lands where the cut is: where the next kept beat starts, or the clip's
  end after the last one.

`photon render` prints one row per beat: `beat N  <speed>×  <take> of take in <duration>`, or
`beat N  cut`.

A beat does not zoom or aim the camera: the storyboard's [camera](#camera) positions and zooms
do, on the clip's clock. A position aims it at a target or a cell, and a zoom sets the
magnification. A `zoom` key on a beat is a load error that names `camera.zooms` and
`camera.positions`. The crop never leaves the frame. When the centre is near an edge, the crop stops at that
edge, so the aimed point sits off-centre rather than showing empty space. The cursor follows the
camera: it is drawn through the same crop, so it stays on the cell it points at.

The **cursor** is drawn, not recorded. It is a white dot with a dark outline. It fades in, glides
from the last point (the bottom row's centre at first) to each click and each drag's press, and
shows a ring on each press. It fades out after the beat's last press or release. During a drag it
follows the button: it eases from the press cell through each `via` corner to the drag's last
move, one leg at a time at the drag's own pace, then rings again on release. Moves and `via`
corners do not ring. Its size is fixed and does not grow with a zoom.
Clicks and drags in a cut beat draw nothing.

To render, photon draws every frame at `frame.scale`, adds the zoom and the cursor, downscales to
`frame.width` and draws the [captions](#captions) on top. It streams the frames through ffmpeg
twice. Pass 1 builds a palette of `max_colors` colours into `<work>/<clip>.palette.png`. Pass 2
maps the frames onto it, with no dithering, into the GIF. The palette file is then removed. No
frames are stored between passes, so memory stays flat for any clip length. Box-drawing and block
characters are drawn as shapes, not taken from a font, so lines and blocks join cleanly.

## Environment variables

| Variable | Set for | Meaning |
| --- | --- | --- |
| `PHOTON_WORK` | every command photon runs | The work dir. If you set it yourself, it picks the work dir when `--work` is not given. |
| `PHOTON_PORT` | each service | The service's own port. |
| `PHOTON_PORT_<NAME>` | the launch, later services, `run` expects, `on_success` | The named service's port. |
| `PHOTON_TAKE` | `run` expects, `on_success` | The take file's path. |
| `PHOTON_BEAT` | a beat's `run` expects | The beat's id. |

## Fonts and licences

The fonts are built into the photon binary. A clip renders the same on every machine, and nobody
has to install or ship fonts:

- **Source Code Pro** (regular, bold, italic): the terminal font.
- **Noto Sans Symbols 2**, **Noto Sans Math** and **Noto Sans Symbols**: tried in that order for
  a character Source Code Pro lacks. A fallback glyph wider than its cell is scaled down to fit.
- **Source Sans 3** (regular, bold): the caption font.

All eight use the **SIL Open Font License 1.1**. The licence texts sit next to the font files in
`internal/fonts/`: `SourceCodePro-OFL.txt`, `NotoSansMath-OFL.txt`, `NotoSansSymbols-OFL.txt`
for both Noto Sans Symbols families, and `SourceSans3-OFL.txt`. They must travel with the fonts.

To use your own terminal fonts, set the config's `fonts` folder. It must hold all six terminal
files under the same names: `SourceCodePro-Regular.ttf`, `SourceCodePro-Bold.ttf`,
`SourceCodePro-It.ttf`, `NotoSansSymbols2-Regular.ttf`, `NotoSansMath-Regular.ttf` and
`NotoSansSymbols-Regular.ttf`.

To use your own caption font, name its files in the config's `caption_font`:

```yaml
caption_font:
  regular: fonts/Inter-Regular.ttf
  bold: fonts/Inter-Bold.ttf
```

Each path is relative to the config's folder. A style left out keeps Source Sans 3. The `fonts`
folder does not change the caption font.

## FAQ

**What can photon film?**
Any program that runs in a terminal: a CLI, a full-screen TUI, a REPL or a script, in any
language. photon only talks to it through the pty, like a real terminal.

**Does it need a browser or a GUI?**
Not for a terminal app: photon draws the frames itself and only needs ffmpeg to encode the GIF.
Filming a web page (`launch.url`) needs Chromium or Chrome, which photon runs headless.

**How is it different from asciinema or VHS?**
asciinema records a session you perform by hand. VHS, like photon, turns a script into a GIF.
photon's focus is a take you can trust: it clicks and drags with a real mouse, checks each take
against expects, retries until the take is clean, and fits each part of the clip to a set length.

**Can I run it in CI?**
Yes, on Linux or macOS. `photon lint` checks storyboards without recording.
`photon record` exits non-zero on a bad take, so a job can retake or fail.

**Does it work on Windows?**
`lint`, `check`, `render` and `tune` do. `setup` and `record` need a unix pty, so record on Linux or
macOS.

## Development

`make test`, `make vet` and `make lint` run the tests, `go vet` and golangci-lint over the module.

The tune page also has a test that drives it in a headless Chromium against a real `photon tune`
server: it loads the page, plays, scrubs, edits a beat and saves. It sits behind the `browser`
build tag:

```sh
go test -tags browser ./internal/tune -run Browser -v
```

It finds Chromium or Chrome on `PATH`; set `PHOTON_TEST_BROWSER` to the browser's path to use
another. Without a browser the test is skipped. Set `PHOTON_TEST_BROWSER_REQUIRED=1` to make it
fail instead, as CI does. On a small host, cap the Go heap with `GOMEMLIMIT=2GiB`.

## License

MIT, see [LICENSE](LICENSE). The built-in fonts use the SIL Open Font License 1.1, see
[Fonts and licences](#fonts-and-licences).
