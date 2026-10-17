package config

import (
	"errors"
	"fmt"
	"image/color"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// validBase is a config that sets every field and breaks no rule. Each row of TestLoadRejects
// swaps one fragment of it, which must occur in it exactly once, to break one rule.
const validBase = `name: demo-app
work: rig
setup: ./scripts/setup.sh
reset: ./scripts/reset.sh
launch:
  command: ./bin/app --demo
  dir: stage
  env: {APP_HOME: '${PHOTON_WORK}/home'}
  strip: ['APP_*', 'XDG_CONFIG_HOME']
services:
  backend:
    command: ./bin/fake-backend --replay
    env: {LOG_LEVEL: debug}
    port: 0
    ready: {tcp: true}
    timeout: 5s
  mail-relay:
    command: ./bin/relay
    port: 2525
    ready: {line: 'listening on'}
profiles:
  capture:
    services:
      backend: {command: ./bin/fake-backend --capture, env: {LOG_LEVEL: info}}
    on_success: ./scripts/keep-capture.sh
end:
  keys: [esc, ctrl+c]
  idle: 2s
  settle_timeout: 20s
  kill_after: 3s
areas:
  footer:
    top: {match: '^─+$', pick: last, offset: 1}
  status:
    top: {match: '^─+$', pick: last, below: {match: 'Inbox'}}
    bottom: {match: '^─+$', above: {match: '> ', pick: last}, offset: -1}
theme:
  fg: '#ffffff'
  bg: '#000000'
  ansi: ['#000001', '#000002', '#000003', '#000004', '#000005', '#000006', '#000007', '#000008',
         '#000009', '#00000a', '#00000b', '#00000c', '#00000d', '#00000e', '#00000f', '#000010']
fonts: fonts
`

// writeConfig writes body to a photon.yaml in a fresh temp dir and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// writeFontFile writes a stand-in font file at path, creating its dir. Load checks a caption font
// file is readable, not that it parses.
func writeFontFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("a font"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestLoadEveryField(t *testing.T) {
	t.Parallel()
	path := writeConfig(t, validBase)
	dir := filepath.Dir(path)

	config, err := Load(path)

	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if config.Path != path || config.Dir != dir {
		t.Errorf("path, dir: want %q, %q, got %q, %q", path, dir, config.Path, config.Dir)
	}
	if config.Name != "demo-app" || config.Setup != "./scripts/setup.sh" || config.Reset != "./scripts/reset.sh" {
		t.Errorf("header: got name %q, setup %q, reset %q", config.Name, config.Setup, config.Reset)
	}
	for _, field := range []struct{ name, got, want string }{
		{"work", config.Work, filepath.Join(dir, "rig")},
		{"fonts", config.Fonts, filepath.Join(dir, "fonts")},
		{"launch.dir", config.Launch.Dir, filepath.Join(dir, "stage")},
	} {
		if field.got != field.want {
			t.Errorf("%s: want %q, got %q", field.name, field.want, field.got)
		}
	}
	if !slices.Equal(config.Launch.Strip, []string{"APP_*", "XDG_CONFIG_HOME"}) || config.Launch.Env["APP_HOME"] != "${PHOTON_WORK}/home" {
		t.Errorf("launch: got %+v", config.Launch)
	}
	wantEnd := End{Keys: []string{"esc", "ctrl+c"}, Idle: 2 * time.Second, SettleTimeout: 20 * time.Second, KillAfter: 3 * time.Second}
	if !slices.Equal(config.End.Keys, wantEnd.Keys) || config.End.Idle != wantEnd.Idle ||
		config.End.SettleTimeout != wantEnd.SettleTimeout || config.End.KillAfter != wantEnd.KillAfter {
		t.Errorf("end: want %+v, got %+v", wantEnd, config.End)
	}
	if config.Theme.FG != "#ffffff" || config.Theme.BG != "#000000" || config.Theme.ANSI[15] != "#000010" {
		t.Errorf("theme: got %+v", config.Theme)
	}
}

func TestLoadServicesAndProfiles(t *testing.T) {
	t.Parallel()

	config, err := Load(writeConfig(t, validBase))

	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	backend := config.Services["backend"]
	if backend.Port != 0 || !backend.Ready.TCP || backend.Timeout != 5*time.Second || backend.Env["LOG_LEVEL"] != "debug" {
		t.Errorf("backend: got %+v", backend)
	}
	relay := config.Services["mail-relay"]
	if relay.Port != 2525 || relay.Ready.Line != "listening on" || relay.Timeout != DefaultServiceTimeout {
		t.Errorf("mail-relay: want port 2525, line ready and the default timeout, got %+v", relay)
	}
	capture := config.Profiles["capture"]
	override := capture.Services["backend"]
	if override.Command != "./bin/fake-backend --capture" || override.Env["LOG_LEVEL"] != "info" {
		t.Errorf("capture override: got %+v", override)
	}
	if capture.OnSuccess != "./scripts/keep-capture.sh" {
		t.Errorf("on_success: got %q", capture.OnSuccess)
	}
}

func TestLoadAreas(t *testing.T) {
	t.Parallel()

	config, err := Load(writeConfig(t, validBase))

	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	footer := config.Areas["footer"]
	if footer.Bottom != nil || *footer.Top != (Anchor{Match: "^─+$", Pick: PickLast, Offset: 1}) {
		t.Errorf("footer: want a last-rule top edge one row down and the screen's bottom, got %+v", footer)
	}
	status := config.Areas["status"]
	if status.Top.Below == nil || *status.Top.Below != (Anchor{Match: "Inbox", Pick: PickFirst}) {
		t.Errorf("status top: want a below anchor defaulted to first, got %+v", status.Top.Below)
	}
	bottom := status.Bottom
	if bottom.Pick != PickFirst || bottom.Offset != -1 || bottom.Above == nil || bottom.Above.Pick != PickLast {
		t.Errorf("status bottom: got %+v (above %+v)", bottom, bottom.Above)
	}
}

func TestLoadMinimal(t *testing.T) {
	t.Parallel()
	path := filepath.Join("testdata", "minimal", FileName)

	config, err := Load(path)

	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	if config.Launch.Command != "./app --demo" || config.Launch.Dir != dir {
		t.Errorf("launch: want the command run in %q, got %+v", dir, config.Launch)
	}
	if config.Work != "" || config.Fonts != "" || len(config.Services) != 0 || len(config.Areas) != 0 {
		t.Errorf("want no work, fonts, services or areas, got %+v", config)
	}
	wantEnd := End{Keys: DefaultEndKeys, Idle: DefaultIdle, SettleTimeout: DefaultSettleTimeout, KillAfter: DefaultKillAfter}
	if !slices.Equal(config.End.Keys, wantEnd.Keys) || config.End.Idle != wantEnd.Idle ||
		config.End.SettleTimeout != wantEnd.SettleTimeout || config.End.KillAfter != wantEnd.KillAfter {
		t.Errorf("end: want the defaults %+v, got %+v", wantEnd, config.End)
	}
	if config.Theme.FG != DefaultForeground || config.Theme.BG != DefaultBackground || !slices.Equal(config.Theme.ANSI, DefaultANSI[:]) {
		t.Errorf("theme: want the default theme, got %+v", config.Theme)
	}
}

func TestLoadKeepsAbsolutePaths(t *testing.T) {
	t.Parallel()
	absolute := t.TempDir()
	body := strings.Replace(validBase, "work: rig", "work: '"+absolute+"'", 1)
	body = strings.Replace(body, "  dir: stage", "  dir: '"+absolute+"'", 1)

	config, err := Load(writeConfig(t, body))

	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if config.Work != absolute || config.Launch.Dir != absolute {
		t.Errorf("want both kept as %q, got work %q, launch.dir %q", absolute, config.Work, config.Launch.Dir)
	}
}

// captionFontConfig is a minimal config whose caption_font names regular and bold.
const captionFontConfig = `name: demo-app
launch: {command: ./bin/app}
caption_font: {regular: %q, bold: %q}
`

// TestLoadResolvesCaptionFontAgainstTheConfigDir runs from a working dir other than the config's,
// so a relative caption font file is found only if it is resolved against the config's dir. It
// changes the process's working dir and so cannot run in parallel.
func TestLoadResolvesCaptionFontAgainstTheConfigDir(t *testing.T) {
	t.Chdir(t.TempDir())
	absolute := filepath.Join(t.TempDir(), "caption-bold.ttf")
	writeFontFile(t, absolute)
	path := writeConfig(t, fmt.Sprintf(captionFontConfig, "fonts/caption.ttf", absolute))
	dir := filepath.Dir(path)
	writeFontFile(t, filepath.Join(dir, "fonts", "caption.ttf"))

	config, err := Load(path)

	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := CaptionFont{Regular: filepath.Join(dir, "fonts", "caption.ttf"), Bold: absolute}
	if config.CaptionFont != want {
		t.Errorf("caption_font: want %+v, got %+v", want, config.CaptionFont)
	}
}

func TestLoadRejectsAnUnreadableCaptionFont(t *testing.T) {
	t.Parallel()
	path := writeConfig(t, fmt.Sprintf(captionFontConfig, "", "missing.ttf"))

	_, err := Load(path)

	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("want a *ValidationError, got %v", err)
	}
	if len(invalid.Problems) != 1 || !strings.Contains(err.Error(), "caption_font.bold: ") ||
		!strings.Contains(err.Error(), filepath.Join(filepath.Dir(path), "missing.ttf")) {
		t.Errorf("want one problem naming caption_font.bold and its resolved path, got:\n%v", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("want fs.ErrNotExist in the chain, got %v", err)
	}
}

func TestLoadRejects(t *testing.T) {
	t.Parallel()
	const servicesSection = "  mail-relay:\n    command: ./bin/relay\n"

	cases := []struct {
		name   string
		old    string
		new    string
		want   string
		decode bool // true: a decode error, not a *ValidationError
	}{
		{"unknown field", "fonts: fonts", "fonts: fonts\ncolour: red", "field colour not found", true},
		{"unknown nested field", "kill_after: 3s", "kill_after: 3s\n  quit: 1s", "field quit not found", true},
		{"duration spelling", "idle: 2s", "idle: soon", "cannot unmarshal", true},
		{"name missing", "name: demo-app\n", "", "name: missing", false},
		{"name spelling", "name: demo-app", "name: Demo_App", `name: want lowercase letters, digits and -, got "Demo_App"`, false},
		{"setup blank", "setup: ./scripts/setup.sh", "setup: ' '", `setup: want a command, got " "`, false},
		{"reset blank", "reset: ./scripts/reset.sh", "reset: ' '", `reset: want a command, got " "`, false},
		{"launch command missing", "  command: ./bin/app --demo\n", "", "launch.command: missing", false},
		{"launch env name", "APP_HOME:", "APP HOME:", `launch.env: "APP HOME" is not a variable name`, false},
		{"strip glob", "'APP_*'", "'APP_['", `launch.strip[0]: want a glob of variable names, got "APP_["`, false},
		{"strip empty", "'APP_*'", "''", `launch.strip[0]: want a glob of variable names, got ""`, false},
		{"service name", "  mail-relay:\n", "  Mail:\n",
			`services.Mail: name: want lowercase letters, digits, - and _ starting with a letter, got "Mail"`, false},
		{"service port variable shared", servicesSection, "  mail_relay:\n    command: ./bin/relay\n    ready: {tcp: true}\n" + servicesSection,
			"services.mail_relay: name: shares PHOTON_PORT_MAIL_RELAY with service mail-relay", false},
		{"service command missing", "    command: ./bin/relay\n", "", "services.mail-relay: command: missing", false},
		{"service env name", "LOG_LEVEL: debug", "1LOG: debug", `services.backend: env: "1LOG" is not a variable name`, false},
		{"service port range", "port: 2525", "port: 70000", "services.mail-relay: port: want 0..65535, got 70000", false},
		{"service port shared", "port: 0", "port: 2525", "services.mail-relay: port: 2525 is also service backend's", false},
		{"service ready missing", "    ready: {tcp: true}\n", "", "services.backend: ready: want exactly one of tcp or line", false},
		{"service ready both", "{tcp: true}", "{tcp: true, line: up}", "services.backend: ready: want exactly one of tcp or line", false},
		{"service ready regex", "'listening on'", "'(listening'", "services.mail-relay: ready.line: error parsing regexp", false},
		{"service timeout negative", "timeout: 5s", "timeout: -5s", "services.backend: timeout: want ≥0, got -5s", false},
		{"profile name", "  capture:\n", "  Capture:\n", `profiles.Capture: name: want lowercase letters`, false},
		{"profile service undeclared", "      backend: {command", "      frontend: {command",
			"profiles.capture: services.frontend: not a declared service", false},
		{"profile override command blank", "{command: ./bin/fake-backend --capture,", "{command: ' ',",
			`profiles.capture: services.backend: command: want a command, got " "`, false},
		{"profile override env name", "LOG_LEVEL: info", "LOG-LEVEL: info",
			`profiles.capture: services.backend: env: "LOG-LEVEL" is not a variable name`, false},
		{"on_success blank", "on_success: ./scripts/keep-capture.sh", "on_success: ' '",
			`profiles.capture: on_success: want a command, got " "`, false},
		{"end key unknown", "keys: [esc, ctrl+c]", "keys: [esc, ctrl+q]", `end.keys[1]: unknown key "ctrl+q", want one of enter,`, false},
		{"end idle negative", "idle: 2s", "idle: -2s", "end.idle: want ≥0, got -2s", false},
		{"end settle negative", "settle_timeout: 20s", "settle_timeout: -1s", "end.settle_timeout: want ≥0, got -1s", false},
		{"end kill negative", "kill_after: 3s", "kill_after: -3s", "end.kill_after: want ≥0, got -3s", false},
		{"area any declared", "  footer:\n", "  any:\n", "areas.any: any is the whole screen and needs no declaration", false},
		{"area name", "  footer:\n", "  9footer:\n", "areas.9footer: want a name of letters", false},
		{"anchor match missing", "{match: '^─+$', pick: last, offset: 1}", "{pick: last, offset: 1}", "areas.footer: top.match: missing", false},
		{"anchor match regex", "{match: 'Inbox'}", "{match: '[Inbox'}", "areas.status: top.below.match: error parsing regexp", false},
		{"anchor pick", "pick: last, offset: 1", "pick: middle, offset: 1", `areas.footer: top.pick: want first or last, got "middle"`, false},
		{"anchor both sides", "{match: '^─+$', pick: last, below: {match: 'Inbox'}}", "{match: '^─+$', below: {match: 'Inbox'}, above: {match: x}}",
			"areas.status: top.want at most one of above or below", false},
		{"nested anchor pick", "{match: '> ', pick: last}", "{match: '> ', pick: final}", `areas.status: bottom.above.pick: want first or last, got "final"`, false},
		{"theme fg", "fg: '#ffffff'", "fg: white", `theme.fg: want #rrggbb, got "white"`, false},
		{"theme bg", "bg: '#000000'", "bg: '#000'", `theme.bg: want #rrggbb, got "#000"`, false},
		{"theme ansi count", "'#000001', ", "", "theme.ansi: want 16 colours, got 15", false},
		{"theme ansi colour", "'#000003'", "'#00000g'", `theme.ansi[2]: want #rrggbb, got "#00000g"`, false},
	}
	if _, err := Load(writeConfig(t, validBase)); err != nil {
		t.Fatalf("the base config must load clean: %v", err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if count := strings.Count(validBase, tc.old); count != 1 {
				t.Fatalf("fragment %q occurs %d times in the base, want once", tc.old, count)
			}

			_, err := Load(writeConfig(t, strings.Replace(validBase, tc.old, tc.new, 1)))

			if err == nil {
				t.Fatalf("want an error containing %q, got none", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want an error containing %q, got:\n%v", tc.want, err)
			}
			var invalid *ValidationError
			isValidation := errors.As(err, &invalid)
			if tc.decode {
				if isValidation {
					t.Fatalf("want a decode error, got a *ValidationError:\n%v", err)
				}
				return
			}
			if !isValidation {
				t.Fatalf("want a *ValidationError, got %T: %v", err, err)
			}
			if got := len(invalid.Problems); got != 1 {
				t.Errorf("problems: want exactly the one broken rule, got %d:\n%v", got, err)
			}
		})
	}
}

func TestLoadReportsEveryProblem(t *testing.T) {
	t.Parallel()
	path := writeConfig(t, `name: Bad
launch: {strip: ['[']}
services:
  api: {command: ./api, ready: {}}
profiles:
  live: {services: {web: {command: ./web}}}
end: {keys: [hyper]}
theme: {ansi: ['#000000']}
`)

	_, err := Load(path)

	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("want a *ValidationError, got %v", err)
	}
	want := []string{
		"name: want lowercase letters",
		"launch.command: missing",
		"launch.strip[0]",
		"services.api: ready: want exactly one of tcp or line",
		"profiles.live: services.web: not a declared service",
		`end.keys[0]: unknown key "hyper"`,
		"theme.ansi: want 16 colours, got 1",
	}
	if got := len(invalid.Problems); got != len(want) {
		t.Fatalf("problems: want %d, got %d:\n%v", len(want), got, err)
	}
	for index, problem := range invalid.Problems {
		if !strings.Contains(problem.Error(), want[index]) {
			t.Errorf("problem %d: want %q, got %q", index, want[index], problem)
		}
	}
	if !strings.HasPrefix(err.Error(), path+": 7 problem(s)\n") {
		t.Errorf("want the message headed by the path and the count, got:\n%v", err)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	t.Parallel()

	_, err := Load(writeConfig(t, ""))

	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("want a *ValidationError, got %v", err)
	}
	for _, want := range []string{"name: missing", "launch.command: missing"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("want a problem containing %q, got:\n%v", want, err)
		}
	}
}

func TestLoadMissingFile(t *testing.T) {
	t.Parallel()

	_, err := Load(filepath.Join(t.TempDir(), FileName))

	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("want an error wrapping fs.ErrNotExist, got %v", err)
	}
}

func TestPortEnvName(t *testing.T) {
	t.Parallel()
	cases := []struct{ service, want string }{
		{"backend", "PHOTON_PORT_BACKEND"},
		{"mail-relay", "PHOTON_PORT_MAIL_RELAY"},
		{"cache_2", "PHOTON_PORT_CACHE_2"},
	}
	for _, tc := range cases {
		if got := PortEnvName(tc.service); got != tc.want {
			t.Errorf("PortEnvName(%q): want %q, got %q", tc.service, tc.want, got)
		}
	}
}

func TestColorRGBA(t *testing.T) {
	t.Parallel()
	cases := []struct {
		color Color
		want  color.RGBA
	}{
		{"#cdd6f4", color.RGBA{R: 0xcd, G: 0xd6, B: 0xf4, A: 0xff}},
		{"#0E1117", color.RGBA{R: 0x0e, G: 0x11, B: 0x17, A: 0xff}},
		{"white", color.RGBA{A: 0xff}},
	}
	for _, tc := range cases {
		if got := tc.color.RGBA(); got != tc.want {
			t.Errorf("%q.RGBA(): want %v, got %v", tc.color, tc.want, got)
		}
	}
}
