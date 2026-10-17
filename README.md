# photon

Record a terminal app's demo clip from a storyboard, check the take, and render it to a GIF.

photon runs the real program in a pty, performs the storyboard's beats — humanized typing, keys,
real mouse clicks, waits — records every screen change, and renders the shipped GIF with fixed
section lengths, zooms on on-screen targets and a drawn cursor. A re-record after the UI changes
is one command, not a hand performance.

photon is one CLI for any project in any language. A project plugs in through two kinds of YAML
file: one `photon.yaml` for the project (how its app is built, launched and filmed) and one
storyboard per clip (what happens on screen, how long each part plays, and what a good take must
show). Everything project-specific is a shell command photon runs or a service it starts.

- [Install](#install)
- [Quick start](#quick-start)
- [The commands](#the-commands)
- [The take and the retake loop](#the-take-and-the-retake-loop)
- [photon.yaml reference](#photonyaml-reference)
- [Storyboard reference](#storyboard-reference)
- [Targets and areas](#targets-and-areas)
- [Expects](#expects)
- [Services and profiles](#services-and-profiles)
- [Sections, zoom and cursor](#sections-zoom-and-cursor)
- [Environment variables](#environment-variables)
- [Fonts and licences](#fonts-and-licences)

## Install

```sh
go install github.com/airiclenz/photon/cmd/photon@latest
```

This needs Go 1.26 or newer. photon also runs two external programs:

- **ffmpeg** (required by `photon render`): encodes the GIF with a per-clip palette. Install it
  from your package manager (`apt install ffmpeg`, `brew install ffmpeg`, ...).
- **gifsicle** (optional): when it is on `PATH`, `photon render` shrinks the GIF with a
  lossless `gifsicle -O3` pass. Without it the GIF is simply larger.

Recording needs a unix pty, so `photon setup` and `photon record` run on Linux and macOS and
refuse on Windows. `photon lint`, `photon check` and `photon render` run everywhere.

## Quick start

The repository carries a small example, `examples/hello`: photon films its own fixture TUI, a
screen with a title, a list, an input line and a status row. It needs the Go toolchain, ffmpeg
and a checkout of this repository.

```sh
git clone https://github.com/airiclenz/photon
cd photon
go install ./cmd/photon

photon lint   examples/hello/hello.yaml   # check the storyboard and its photon.yaml
photon setup  examples/hello/hello.yaml   # build the fixture into the work dir, once
photon record examples/hello/hello.yaml   # record one take and check it
photon render examples/hello/hello.yaml   # render the take to examples/hello/out/hello.gif
```

`record` prints one row per expect and exits 0 when every beat and expect passed:

```text
beat 1 | PASS | seen /type and enter to submit/ at 0s
beat 2 | PASS | seen /you typed: hello, photon/ in area status at 540ms
beat 3 | PASS | seen /\* beta/ in area list at 624ms
beat 3 | PASS | seen /selected: beta/ in area status at 624ms
take   | PASS | seen /photon fixture/ at 0s
take   | PASS | exit 0
```

`render` prints the GIF's path, size and length, then how fast each beat plays in its section:

```text
examples/hello/out/hello.gif  28K  7s
  beat 1  1.00×  0s of take in 2s
  beat 2  1.00×  618ms of take in 3s
  beat 3  1.00×  41ms of take in 2s
```

Pass `-o <file>` to write the GIF somewhere else, and `--work <dir>` to any command to keep the
rig somewhere other than `~/.cache/hello-photon`.

To film your own project, copy the two files from `examples/hello` next to your code, point
`setup` at your build and `launch.command` at your program, and rewrite the beats.

## The commands

Every command but `photon version` takes the storyboard as its first argument. Each finds the
project's config the same way: `--config <file>` when given, else the nearest `photon.yaml` at or
above the storyboard's directory. Each command's `--help` repeats what is said here.

| Command | What it does |
| --- | --- |
| `photon lint <storyboard>` | Loads the storyboard and its config, and prints every problem both files have — or `ok` with the clip's name, beat count and config path. Records nothing. |
| `photon setup <storyboard>` | Creates the work dir and runs the config's `setup` once, to build the rig. |
| `photon record <storyboard>` | Records one take, writes it to `<work>/<clip>.take`, checks it and prints the check table. |
| `photon check <storyboard> [<take>]` | Judges a take (default `<work>/<clip>.take`) against the storyboard's expects and prints the check table. |
| `photon render <storyboard> [<take>]` | Renders a take (default `<work>/<clip>.take`) to the GIF at the storyboard's `ship` path. |
| `photon version` | Prints photon's version, such as `1.0.0`; `photon --version` prints the same. |

Flags:

| Flag | Commands | Meaning |
| --- | --- | --- |
| `--config <file>` | all | The photon config to use instead of the nearest `photon.yaml`. |
| `--work <dir>` | setup, record, check, render | The rig's work dir. |
| `--profile <name>` | record | The profile whose services and `on_success` the take runs with. |
| `-o`, `--output <file>` | render | The GIF to write instead of the storyboard's `ship` path; its directory is created. |
| `--dry-run` | render | Print the ffmpeg command line and exit without rendering. |

**The work dir** is the first of: `--work`, `$PHOTON_WORK`, the config's `work`, and
`~/.cache/<name>-photon` (where `<name>` is the config's `name`). It holds whatever `setup`
builds, every take, and each service's log (`<work>/<service>.log`).

**Exit status**, the same for every command:

| Status | Meaning |
| --- | --- |
| `0` | The command succeeded: the clip is clean (lint), setup ran, every beat and expect passed (record, check), the GIF was written (render). |
| `1` | A run started and failed: a problem in either file, a failed command, a beat that failed, an expect that failed, a take that could not be recorded or rendered. |
| `2` | A bad command line (an unknown flag, a missing argument). The usage text is printed. |

Errors go to stderr as `photon: <error>`.

## The take and the retake loop

`photon record` performs, in order:

1. Creates the work dir and runs the config's `reset`, which restores the stage.
2. Starts the services (under `--profile` when given) and waits for each to be ready.
3. Launches `launch.command` in a pty sized to the storyboard's `frame.cols` × `frame.rows`, and
   waits for its first paint.
4. Performs every beat's actions, in order. A failing action — a `wait` that times out, a
   `click` whose target is not on screen — fails its beat and ends the take there.
5. Ends the take: waits for the program to stop painting for `end.idle` (at most
   `end.settle_timeout`), freezes the take so nothing the program paints on its way out is kept,
   types `end.keys`, and kills the program's whole process group if it has not left
   `end.kill_after` later.
6. Writes the take to `<work>/<clip>.take` and prints its path on stderr.
7. When every beat passed: checks the take with the services still up, prints the check table on
   stdout, stops the services and runs the profile's `on_success`.

A take whose beats failed is still written, because it shows why, but it is neither checked nor
followed by `on_success`. The output of `reset`, the services and `on_success` goes to stderr;
only the check table goes to stdout.

A take is not always a keeper — a slow service, a race in the app. Because `record` exits 0 only
when every beat and every expect passed, the retake loop is one line, and the render follows it:

```sh
until photon record docs/demo/tour.yaml; do :; done
photon render docs/demo/tour.yaml
```

`photon check` re-judges a written take without recording it again, which is how an edited
expect is tried out. `photon render` reads the take only; it never runs the program.

A take file is a gzip-compressed JSON-lines file: every full-grid snapshot of the screen with its
time, plus a log of what photon did (beat starts, actions, resolved targets, clicks). It carries
a format version; a newer or foreign file is refused.

## photon.yaml reference

One `photon.yaml` per project, shared by all its storyboards. Unknown fields are an error, and
every problem in the file is reported at once. Relative paths are resolved against the file's
own directory, and `setup`, `reset`, services and `on_success` run in that directory.

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
| `name` | Required. The project's name, in lowercase letters, digits and `-`. Names the default work dir. |
| `work` | The rig's work dir. Overridden by `--work` and `$PHOTON_WORK`; default `~/.cache/<name>-photon`. |
| `setup` | A command run once by `photon setup` to build the rig. |
| `reset` | A command run before every take to restore the stage. It runs before the services start, so it sees `PHOTON_WORK` but no ports. |
| `launch` | The program filmed. |
| `launch.command` | Required. Run as `sh -c '<command>'`. No `exec` is added, so a compound command such as `. env.sh && exec app` works; the whole process group is killed at the end of the take. |
| `launch.dir` | The launch's working directory, relative to the config's dir; default the config's dir. |
| `launch.env` | Variables set for the launch. Values may name `${PHOTON_*}` variables, which photon expands. |
| `launch.strip` | Globs of variable names (`MYAPP_*`) removed from the operator's environment before the launch, so the operator's own settings cannot steer the app. |
| `services` | Helper processes a take starts, by name. See [Services and profiles](#services-and-profiles). |
| `profiles` | Named variants of the services, chosen with `photon record --profile`. |
| `end` | How a take ends. |
| `end.keys` | Keys typed to quit the program, by [key name](#key-names); default `[ctrl+c]`. |
| `end.idle` | How long the program must paint nothing before the keys are sent; default `1s`. |
| `end.settle_timeout` | The longest photon waits for that idle; default `15s`. |
| `end.kill_after` | How long after the keys a program still running is killed; default `10s`. |
| `areas` | Named bands of screen rows. See [Targets and areas](#targets-and-areas). |
| `theme` | The colours frames are drawn with. |
| `theme.fg`, `theme.bg` | The terminal's default foreground and background, `#rrggbb`. Default `#cdd6f4` on `#0e1117`. |
| `theme.ansi` | The 16 ANSI colours, normal then bright, all 16 or none. Default: Catppuccin Mocha's. Colours 16-255 are the standard xterm palette; truecolor is drawn as is. |
| `fonts` | A directory of faces that replaces the embedded ones. See [Fonts and licences](#fonts-and-licences). |

Every command — `setup`, `reset`, `launch.command`, a service's `command`, `on_success`, a `run`
expect — is one string run by `sh -c` with the `PHOTON_*` variables in its environment. photon
itself expands `${PHOTON_*}` only in `env` values; everywhere else the shell does the expanding.

Durations are Go durations: `300ms`, `2s`, `1m30s`.

### Key names

`enter`, `esc`, `tab`, `shift+tab`, `space`, `backspace`, `ctrl+c`, `up`, `down`, `left`,
`right`, `pgup`, `pgdown`, `alt+up`, `alt+down`, `f1` to `f12`. photon sends each as the bytes
an xterm sends, so the program reads them through its own input parser.

## Storyboard reference

One storyboard per clip. Unknown fields are an error, and every problem is reported at once
(`photon lint` shows them all).

```yaml
clip: tour                      # required; names the take, <work>/tour.take
ship: ../tour.gif               # required; relative to this file
frame:
  cols: 100                     # the pty's size, in cells
  rows: 30
  padding: 16                   # pixels around the grid
  font_size: 14                 # points
  line_height: 1.2              # a multiple of the font size
  scale: 2                      # frames are rasterized at this scale, then downscaled
  width: 800                    # the shipped GIF's width, in pixels
  fps: 24
  max_colors: 128               # palette size, 2..256

expect:                         # what the take as a whole must show
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
| `clip` | Required. The clip's name; the take is `<work>/<clip>.take`. |
| `ship` | Required. Where `photon render` writes the GIF, relative to the storyboard's directory. |
| `frame` | The recorded terminal and the shipped geometry. Every field but `padding` is required. |
| `frame.cols`, `frame.rows` | The pty's size in cells, ≥1. A take recorded at another size is refused by `render`. |
| `frame.padding` | Pixels of background around the cell grid, ≥0. |
| `frame.font_size` | The font size in points, >0. The cell size follows from it: a column is Source Code Pro's `M` advance, a row is `font_size` × `line_height`. |
| `frame.line_height` | The row height as a multiple of the font size, >0. |
| `frame.scale` | The scale frames are rasterized at before they are downscaled to `width`, ≥1. |
| `frame.width` | The shipped GIF's width in pixels, ≥1; the height follows the aspect ratio. |
| `frame.fps` | The shipped GIF's frame rate, ≥1. |
| `frame.max_colors` | The GIF palette's size, 2..256. The palette is computed per clip, without dithering. |
| `expect` | Take-level expects, judged on every screen of the take. See [Expects](#expects). |
| `beats` | The beats, at least one. |

### Beats

| Field | Meaning |
| --- | --- |
| `id` | A positive integer, unique and ascending through the file. |
| `title` | Required. A short name, shown in `photon check` and `photon render` output. |
| `why` | Free text: why the beat is in the clip. |
| `notes` | Free text for the storyboard's reader. |
| `do` | The actions, performed in order. |
| `duration` | The beat's section length in the clip, >0. Not needed on a cut beat. |
| `hold` | The leading stretch of the section played at 1×, ≥0 and less than `duration`; default 0. |
| `cut` | `true` records the beat but drops it from the clip. |
| `zoom` | A push into an on-screen target during the section. See [Sections, zoom and cursor](#sections-zoom-and-cursor). |
| `zoom.target` | The [target](#targets-and-areas) zoomed into. |
| `zoom.factor` | The magnification, 1 to 3. Left out, the zoom fits the target. |
| `zoom.in`, `zoom.out` | The ramp in from the section's start and back out before its end; default `1s` each. |
| `expect` | What the beat must show. See [Expects](#expects). |

### Actions

Each entry of `do` sets exactly one of these.

| Action | Fields | What it does |
| --- | --- | --- |
| `type` | `text` (required), `humanize` (default `true`) | Types the text. Humanized typing paces each character from a fixed profile — letters 25-45 ms apart, spaces 60-90 ms, punctuation 90-140 ms, an occasional thinking pause at a space — from a fixed seed, so every take types the same. `humanize: false` sends the text in one write. |
| `key` | `name` (required), `repeat` (default 1) | Presses a [named key](#key-names) `repeat` times. |
| `click` | `target` (required), `times` (default 1) | Finds the target on the screen as it stands and sends `times` left clicks at its centre cell, as real SGR mouse press and release reports. Fails when the target is not on screen, so a `wait` usually comes first. |
| `wait` | `screen` (required), `gone`, `timeout` (required, at most `180s`) | Waits until the `screen` regex matches the screen's text (its rows joined by newlines) — or, with `gone: true`, until it no longer does. Fails at the timeout, or at once when the program exits. |
| `pause` | `for` (required) | Waits the given duration. |

The program must enable mouse reporting itself for a click to reach it; photon only sends the
reports a terminal would.

## Targets and areas

A **target** is a place on screen that a `click` or a `zoom` aims at:

| Field | Meaning |
| --- | --- |
| `text` | Required. A regex (Go syntax) matched against one screen row at a time; it never spans rows. Rows are read as plain text with trailing blanks trimmed. |
| `nth` | Which match counts: `first` (the default), `last`, or a positive integer. |
| `area` | The area searched; empty or `any` searches the whole screen. |

An **area** is a named band of rows, declared in `photon.yaml` under `areas` and found afresh on
every screen, so it follows the app's layout as it moves. Each area has two edges, `top` and
`bottom`, both inclusive; a missing edge is the screen's own edge, so an area with neither is the
whole screen. Each edge is a row **anchor**:

| Field | Meaning |
| --- | --- |
| `match` | Required. A regex; the rows it matches are the candidates. |
| `pick` | `first` (the default) or `last`: which candidate the anchor takes. |
| `above` | Another anchor: only rows strictly above the row it picks are candidates. |
| `below` | Another anchor: only rows strictly below the row it picks are candidates. At most one of `above` and `below`. |
| `offset` | Rows to move from the picked row; negative is up. |

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

An area whose anchor finds no row, or whose top lands below its bottom, is not on screen: a
`click` there fails, and a `seen` expect does not match. Area names start with a letter and use
letters, digits, `-` and `_`; `any` is reserved for the whole screen. A storyboard may name only
areas its config declares — `photon lint` checks that. The same resolver serves clicks, zooms and
`seen` expects, so all three agree on what an area is.

## Expects

An expect is what a keeper take must show, judged PASS or FAIL. A beat's expects are judged on
the screens shown during that beat; the storyboard's top-level `expect` on every screen of the
take. Each expect sets exactly one of:

| Field | Meaning |
| --- | --- |
| `seen` | A regex that must match the text an area shows (its rows joined by newlines) on at least one of those screens. |
| `area` | With `seen` only: the area read; default the whole screen. |
| `run` | A command whose exit status is the verdict: 0 is PASS. It runs by `sh -c` in the work dir with `PHOTON_WORK`, `PHOTON_TAKE`, the services' `PHOTON_PORT_<NAME>` and, for a beat's expect, `PHOTON_BEAT` set. This is how a project checks what photon cannot see: a file the app saved, a request a fake backend logged. |

`photon record` and `photon check` print one row per expect, beats in storyboard order and then
the take-level ones, as `beat N | PASS | detail` (`take | ...` for a take-level expect), and exit
1 when any row is a FAIL. A screen a `wait` matched is always kept in the take, so a `seen` on a
beat that ends in a `wait` finds the screen the wait saw. On Windows, a `run` expect with no `sh`
on `PATH` is a FAIL that says so.

## Services and profiles

A **service** is a helper process photon starts before the launch and stops after the take, such
as a fake backend the filmed program talks to.

| Field | Meaning |
| --- | --- |
| `command` | Required. Run by `sh -c` in the config's dir, in its own process group. |
| `env` | Variables set for it; values may name `${PHOTON_*}`. |
| `port` | The port it serves: a fixed port, or `0` for photon to pick a free one. |
| `ready` | How photon tells it is up — exactly one of `tcp` and `line`. |
| `ready.tcp` | `true`: ready once its port accepts a connection. |
| `ready.line` | A regex: ready once a line of its output matches. |
| `timeout` | How long photon waits for it to be ready; default `10s`. |

Services start in name order. Each one's port is exported as `PHOTON_PORT_<NAME>` (the name
upper-cased, `-` turned into `_`) to the launch, to the services started after it, to `run`
expects and to `on_success`; the service itself also gets its own port as `PHOTON_PORT`. Its
output is logged to `<work>/<name>.log`. When the take is done, each service's process group gets
SIGTERM, then a kill. Service and profile names are lowercase letters, digits, `-` and `_`,
starting with a letter.

A **profile** is a named variant of the services, chosen with `photon record --profile <name>`:

| Field | Meaning |
| --- | --- |
| `services` | Per declared service, what changes under this profile. |
| `services.<name>.command` | Replaces the service's command. |
| `services.<name>.env` | Set over the service's env. |
| `on_success` | A command run after a take whose every beat passed, once the take is checked and the services have stopped. It runs in the config's dir with `PHOTON_WORK`, `PHOTON_TAKE` and the ports set. |

A service's name, port and readiness stay as declared under every profile, so its
`PHOTON_PORT_<NAME>` never changes.

## Sections, zoom and cursor

Each beat becomes a **section** of the clip, exactly `duration` long, whatever the take's own
timing was:

- A beat that took no longer than its `duration` plays at 1× and then holds its last frame for
  the rest of the section.
- A longer beat plays its first `hold` at 1× and the rest sped up to fit. A section faster than
  6× earns a warning from `photon render`.
- A `cut` beat is recorded and checked, but dropped from the clip. A storyboard whose every beat
  is cut renders nothing and is an error.

`photon render` prints one row per beat: `beat N  <speed>×  <take> of take in <duration>`, or
`beat N  cut`.

A **zoom** pushes into its target over the `in` ramp from the start of the section, holds, and
eases back out over the `out` ramp before the section ends; ramps that together outrun the
section shrink in proportion. With no `factor`, the zoom fits the target: the target plus two
cells either side fills 70 percent of the frame width, clamped to 1.25-2.5×. The zoom aims at the
box a click of the same beat on the same target logged, else at the target found on the latest
screen of the beat that shows it.

The **cursor** is drawn, not recorded: a white dot in a dark outline that fades in, glides from
the previous click point (the bottom row's centre before the first click) to each click, rings
on the press, and fades out after the beat's last click. Its size is fixed in design pixels and
does not grow with a zoom. A click inside a cut beat draws nothing.

The render rasterizes every frame at `frame.scale`, composes the zoom and the cursor, downscales
to `frame.width`, and streams the frames into ffmpeg for a per-clip palette of `max_colors`
entries with no dithering. Box-drawing and block-element characters are drawn as shapes rather
than taken from a font, so lines and blocks meet their neighbours exactly.

## Environment variables

| Variable | Set for | Meaning |
| --- | --- | --- |
| `PHOTON_WORK` | every command photon runs | The work dir. Set by the operator, it also chooses the work dir when `--work` is not given. |
| `PHOTON_PORT` | each service | The service's own port. |
| `PHOTON_PORT_<NAME>` | the launch, later services, `run` expects, `on_success` | The named service's port. |
| `PHOTON_TAKE` | `run` expects, `on_success` | The take file's path. |
| `PHOTON_BEAT` | a beat's `run` expects | The beat's id. |

## Fonts and licences

Frames are drawn with fonts embedded in the photon binary, so a clip renders the same on every
machine without the project vendoring fonts or the operator installing them:

- **Source Code Pro** (regular, bold, italic): the terminal face.
- **Noto Sans Symbols 2**, **Noto Sans Math** and **Noto Sans Symbols**: tried in that order for
  a character Source Code Pro lacks. A fallback glyph wider than its cell is scaled down to fit.

All six are licensed under the **SIL Open Font License 1.1**. The licence texts sit beside the
font files in `internal/fonts/`: `SourceCodePro-OFL.txt`, `NotoSansMath-OFL.txt`, and
`NotoSansSymbols-OFL.txt` for both Noto Sans Symbols families. They must travel with the fonts.

A project can replace the faces with the config's `fonts` directory, which must then hold all six
files under the same names: `SourceCodePro-Regular.ttf`, `SourceCodePro-Bold.ttf`,
`SourceCodePro-It.ttf`, `NotoSansSymbols2-Regular.ttf`, `NotoSansMath-Regular.ttf` and
`NotoSansSymbols-Regular.ttf`.

photon itself is released under the MIT License; see `LICENSE`.
