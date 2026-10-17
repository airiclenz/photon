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
[Services and profiles](#services-and-profiles) · [Sections, zoom and cursor](#sections-zoom-and-cursor) ·
[Environment variables](#environment-variables) · [Fonts](#fonts-and-licences) · [FAQ](#faq) ·
[License](#license)

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
- **Zoom and cursor.** Push into an on-screen target. A drawn cursor glides to each click and
  follows each drag.
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

Both pull in ffmpeg.

From source (Go 1.26 or later):

```sh
go install github.com/airiclenz/photon/cmd/photon@latest
```

Prebuilt archives are on the [releases page](https://github.com/airiclenz/photon/releases): a
`photon_<version>_<os>_<arch>.tar.gz` (`.zip` on Windows) for Linux, macOS and Windows on amd64
and arm64, with `SHA256SUMS`. Unpack it, put `photon` on your `PATH`, and install ffmpeg yourself.

photon uses two external programs:

- **ffmpeg** (needed by `photon render`): encodes the GIF. Get it from your package manager
  (`apt install ffmpeg`, `brew install ffmpeg`, ...).
- **gifsicle** (optional): if it is on `PATH`, `photon render` shrinks the GIF with a lossless
  `gifsicle -O3` pass.

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
| `photon lint <storyboard>` | Loads the storyboard and its config and prints every problem in both. Prints `ok` with the clip name, beat count and config path if there are none. Records nothing. |
| `photon setup <storyboard>` | Creates the work dir and runs the config's `setup` once. |
| `photon record <storyboard>` | Records one take to `<work>/<clip>.take`, checks it and prints the check table. |
| `photon check <storyboard> [<take>]` | Checks a take (default `<work>/<clip>.take`) against the storyboard's expects and prints the check table. |
| `photon render <storyboard> [<take>]` | Renders a take (default `<work>/<clip>.take`) to the GIF at the storyboard's `ship` path. |
| `photon tune <storyboard> [<take>]` | Serves a page on `127.0.0.1` to tune the storyboard's timing, zoom and frame against a take (default `<work>/<clip>.take`), and saves the edits back into the storyboard. Runs until Ctrl-C. |
| `photon version` | Prints photon's version, such as `1.0.0`. `photon --version` does the same. |

Flags:

| Flag | Commands | Meaning |
| --- | --- | --- |
| `--config <file>` | all | Use this config instead of the nearest `photon.yaml`. |
| `--work <dir>` | setup, record, check, render, tune | The work dir. |
| `--profile <name>` | record | The profile to record with (its services and `on_success`). |
| `-o`, `--output <file>` | render | Write the GIF here instead of the `ship` path. Its folder is created. |
| `--dry-run` | render | Print both ffmpeg command lines, palette pass first, and exit without rendering. |
| `--port <n>` | tune | Serve the page on this port of `127.0.0.1`. The default, `0`, picks any free port. |

**The work dir** is the first of: `--work`, `$PHOTON_WORK`, the config's `work`, and
`~/.cache/<name>-photon` (`<name>` is the config's `name`). It holds what `setup` builds, every
take, each service's log (`<work>/<service>.log`), and the clip's palette
(`<work>/<clip>.palette.png`) while `render` runs.

**Tuning** with `photon tune` prints the page's address, such as
`photon tune: http://127.0.0.1:41234/`, and never opens a browser. The page previews the take
frame for frame as `photon render` composes it. Its timeline edits each beat's `duration`,
`hold`, `cut` and `zoom`, and its inspector the clip's `frame.width` and `frame.fps`. A pick on
the paused preview proposes a target for the picked cells, ready to use as a zoom target or
to copy as a `click`, `seen` or `drag` end. `pause` and the `do` actions show read-only:
changing them needs a new `photon record`. Save writes the storyboard in place and changes only
the edited values, so comments and layout stay. It is refused while an edit leaves the
storyboard invalid, or when the file changed on disk since it was loaded.

**Exit status** is the same for every command:

| Status | Meaning |
| --- | --- |
| `0` | Success: the clip is clean (lint), setup ran, every beat and expect passed (record, check), the GIF was written (render), the page was served until Ctrl-C (tune). |
| `1` | The run failed: a problem in a file, a failed command, beat or expect, or a take that could not be recorded, rendered or tuned, or a page that could not be served. |
| `2` | A bad command line, such as an unknown flag or a missing argument. The usage text is printed. |

Errors go to stderr as `photon: <error>`.

## How a take works

`photon record` does these steps in order:

1. Creates the work dir and runs the config's `reset` to restore a clean state.
2. Starts the services (under `--profile` if given) and waits until each is ready.
3. Launches `launch.command` in a pty of `frame.cols` × `frame.rows` and waits for its first paint.
4. Runs every beat's actions in order. If an action fails, its beat fails and the take ends.
   Examples: a `wait` times out, or a `click` or `drag` target is not on screen.
5. Ends the take. It waits until the program paints nothing for `end.idle` (at most
   `end.settle_timeout`). It freezes the take, so nothing painted on the way out is kept. It types
   `end.keys`. If the program is still running `end.kill_after` later, it kills its whole process
   group.
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
| `launch.command` | Required. Run as `sh -c '<command>'`. No `exec` is added, so `. env.sh && exec app` works. The whole process group is killed at the end of the take. |
| `launch.dir` | The working directory, relative to the config's folder. Default: the config's folder. |
| `launch.env` | Variables set for the program. Values may use `${PHOTON_*}`, which photon expands. |
| `launch.strip` | Globs of variable names (`MYAPP_*`) removed from your environment before launch, so your own settings cannot change the app. |
| `services` | Helper processes a take starts, by name. See [Services and profiles](#services-and-profiles). |
| `profiles` | Named variants of the services, picked with `photon record --profile`. |
| `end` | How a take ends. |
| `end.keys` | Keys typed to quit the program, by [key name](#key-names). Default `[ctrl+c]`. |
| `end.idle` | How long the program must paint nothing before the keys are sent. Default `1s`. |
| `end.settle_timeout` | The longest photon waits for that idle. Default `15s`. |
| `end.kill_after` | How long after the keys a still-running program is killed. Default `10s`. |
| `areas` | Named bands of screen rows. See [Targets and areas](#targets-and-areas). |
| `theme` | The colours used to draw frames. |
| `theme.fg`, `theme.bg` | The default foreground and background, `#rrggbb`. Default `#cdd6f4` on `#0e1117`. |
| `theme.ansi` | The 16 ANSI colours, normal then bright: all 16 or none. Default: Catppuccin Mocha. Colours 16-255 use the standard xterm palette. Truecolor is drawn as is. |
| `fonts` | A folder of fonts that replaces the built-in ones. See [Fonts and licences](#fonts-and-licences). |

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
    zoom:
      target: {text: 'beta', area: list}
      factor: 1.5
      in: 800ms
      out: 800ms
    expect:
      - seen: '\* beta'
        area: list
  - id: 2
    title: the boring part
    cut: true
    do:
      - pause: {for: 2s}
```

### Header

| Field | Meaning |
| --- | --- |
| `clip` | Required. The clip's name. The take is `<work>/<clip>.take`. |
| `ship` | Required. Where `photon render` writes the GIF, relative to the storyboard's folder. |
| `frame` | The terminal size and the GIF's shape. Every field except `padding` is required. |
| `frame.cols`, `frame.rows` | The pty's size in cells, ≥1. `render` refuses a take recorded at another size. |
| `frame.padding` | Pixels of background around the grid, ≥0. |
| `frame.font_size` | The font size in points, >0. A column is the width of Source Code Pro's `M`. A row is `font_size` × `line_height`. |
| `frame.line_height` | Row height as a multiple of the font size, >0. |
| `frame.scale` | Frames are drawn at this scale, then downscaled to `width`, ≥1. |
| `frame.width` | The GIF's width in pixels, ≥1. The height keeps the aspect ratio. |
| `frame.fps` | The GIF's frame rate, ≥1. |
| `frame.max_colors` | The palette size, 2..256. The palette is built per clip, without dithering. |
| `expect` | Take-level expects, checked on every screen of the take. See [Expects](#expects). |
| `beats` | The beats, at least one. |

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
| `zoom` | A push into an on-screen target. See [Sections, zoom and cursor](#sections-zoom-and-cursor). |
| `zoom.target` | The [target](#targets-and-areas) to zoom into. |
| `zoom.factor` | The magnification, 1 to 3. Leave it out to fit the target. |
| `zoom.in`, `zoom.out` | The ramp in at the start and back out before the end. Default `1s` each. |
| `expect` | What the beat must show. See [Expects](#expects). |

### Actions

Each entry of `do` sets exactly one of these.

| Action | Fields | What it does |
| --- | --- | --- |
| `type` | `text` (required), `humanize` (default `true`) | Types the text. Human-like typing spaces letters 25-45 ms apart, spaces 60-90 ms, punctuation 90-140 ms, with an occasional pause at a space. It uses a fixed seed, so every take types the same. `humanize: false` sends the text in one write. |
| `key` | `name` (required), `repeat` (default 1) | Presses a [named key](#key-names) `repeat` times. |
| `click` | `target` (required), `times` (default 1) | Finds the target on the current screen and sends `times` left clicks at its centre cell, as real SGR mouse press and release reports. Fails if the target is not on screen, so put a `wait` first. |
| `drag` | `from` (required), `to` (required), `steps` (default 12) | Finds `from` on the current screen and presses the left button at its centre. Moves to `to` in `steps` moves that speed up and slow down, then releases. All as real SGR mouse reports. Fails if a target is not on screen. |
| `wait` | `screen` (required), `gone`, `timeout` (required, at most `180s`) | Waits until the `screen` regex matches the screen text (rows joined by newlines). With `gone: true`, waits until it no longer matches. Fails at the timeout, or at once if the program exits. |
| `pause` | `for` (required) | Waits for the given time. |

A drag's `to` is one of two kinds:

- `to.text`: a [target](#targets-and-areas), with `to.nth` and `to.area`. The drag releases at
  its centre cell.
- `to.offset`: a move in cells from the press cell. `to.offset.x` is columns right and
  `to.offset.y` is rows down. Negative goes left and up. At least one must be non-zero. An offset
  takes no `nth` or `area`.

```yaml
- drag: {from: {text: '│', area: panes}, to: {offset: {x: 12}}, steps: 8}
- drag: {from: {text: 'draft.txt'}, to: {text: 'Trash', area: sidebar}}
```

Your program must turn on mouse reporting itself for a click or drag to reach it. photon only
sends the reports a terminal would. A drag also needs button-event (`1002`) or any-event (`1003`)
tracking, the modes in which a terminal reports moves while a button is held.

## Targets and areas

A **target** is a place on screen that a `click`, `drag` or `zoom` aims at:

| Field | Meaning |
| --- | --- |
| `text` | Required. A regex (Go syntax) matched against one screen row at a time. It never spans rows. Rows are plain text with trailing blanks trimmed. |
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

If an anchor finds no row, or the top lands below the bottom, the area is not on screen. A
`click` or `drag` there fails, and a `seen` expect does not match. Area names start with a letter
and use letters, digits, `-` and `_`. `any` is reserved for the whole screen. A storyboard may
only use areas its config declares, and `photon lint` checks this. Clicks, drags, zooms and
`seen` expects all use the same area logic, so they always agree.

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

## Sections, zoom and cursor

Each beat becomes a **section** of the clip. A section is exactly `duration` long, however long
the beat took in the take:

- A beat that took no longer than `duration` plays at 1×, then holds its last frame.
- A longer beat plays its first `hold` at 1× and speeds up the rest to fit. `photon render` warns
  when a section runs faster than 6×.
- A `cut` beat is recorded and checked but left out of the clip. If every beat is cut, there is
  nothing to render, and that is an error.

`photon render` prints one row per beat: `beat N  <speed>×  <take> of take in <duration>`, or
`beat N  cut`.

A **zoom** pushes into its target over the `in` ramp, holds, and eases back out over the `out`
ramp before the section ends. If the two ramps are longer than the section, both shrink to fit.
With no `factor`, the zoom fits the target: the target plus two cells each side fills 70 percent
of the frame width, clamped to 1.25-2.5×. The zoom aims at the spot where a click or drag in the
same beat hit the same target. If there is none, it aims at the target on the latest screen of
the beat that shows it.

The **cursor** is drawn, not recorded. It is a white dot with a dark outline. It fades in, glides
from the last point (the bottom row's centre at first) to each click and each drag's press, and
shows a ring on each press. It fades out after the beat's last press or release. During a drag it
follows the button: it eases from the press cell to the drag's last move, at the drag's own pace,
then rings again on release. Moves do not ring. Its size is fixed and does not grow with a zoom.
Clicks and drags in a cut beat draw nothing.

To render, photon draws every frame at `frame.scale`, adds the zoom and the cursor, and downscales
to `frame.width`. It streams the frames through ffmpeg twice. Pass 1 builds a palette of
`max_colors` colours into `<work>/<clip>.palette.png`. Pass 2 maps the frames onto it, with no
dithering, into the GIF. The palette file is then removed. No frames are stored between passes,
so memory stays flat for any clip length. Box-drawing and block characters are drawn as shapes,
not taken from a font, so lines and blocks join cleanly.

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

All six use the **SIL Open Font License 1.1**. The licence texts sit next to the font files in
`internal/fonts/`: `SourceCodePro-OFL.txt`, `NotoSansMath-OFL.txt`, and `NotoSansSymbols-OFL.txt`
for both Noto Sans Symbols families. They must travel with the fonts.

To use your own fonts, set the config's `fonts` folder. It must hold all six files under the same
names: `SourceCodePro-Regular.ttf`, `SourceCodePro-Bold.ttf`, `SourceCodePro-It.ttf`,
`NotoSansSymbols2-Regular.ttf`, `NotoSansMath-Regular.ttf` and `NotoSansSymbols-Regular.ttf`.

## FAQ

**What can photon film?**
Any program that runs in a terminal: a CLI, a full-screen TUI, a REPL or a script, in any
language. photon only talks to it through the pty, like a real terminal.

**Does it need a browser or a GUI?**
No. photon draws the frames itself and only needs ffmpeg to encode the GIF.

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

## License

MIT, see [LICENSE](LICENSE). The built-in fonts use the SIL Open Font License 1.1, see
[Fonts and licences](#fonts-and-licences).
