package config

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"image/color"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/airiclenz/photon/internal/keys"
)

// Config is a project's photon.yaml: everything about how its app is filmed. Name names the
// project. Work is the rig's work dir. Setup builds the rig and Reset restores its stage before
// every take; both run by `sh -c` in Dir. Launch is the program filmed, Services the helpers a
// take starts and Profiles the variants of them. End is how a take ends, Areas the named bands
// of the screen targets are searched in, Theme the colours the raster draws with, Fonts an
// optional dir of faces that replace the embedded terminal ones, and CaptionFont optional font
// files that replace the embedded caption faces.
//
// [Load] is the only way to get one: a Config in hand has passed validation, its Work, Fonts,
// CaptionFont files and Launch.Dir are resolved against Dir, each CaptionFont file it names is
// readable, and its defaults are filled. Work is left empty when
// the file gives none; the default work dir is the caller's to derive.
type Config struct {
	// Path is the absolute path of the file Load read.
	Path string `yaml:"-"`
	// Dir is the directory Path sits in: the dir setup, reset and services run in, and the one
	// relative paths in the file are written against.
	Dir         string             `yaml:"-"`
	Name        string             `yaml:"name"`
	Work        string             `yaml:"work"`
	Setup       string             `yaml:"setup"`
	Reset       string             `yaml:"reset"`
	Launch      Launch             `yaml:"launch"`
	Services    map[string]Service `yaml:"services"`
	Profiles    map[string]Profile `yaml:"profiles"`
	End         End                `yaml:"end"`
	Areas       map[string]Area    `yaml:"areas"`
	Theme       Theme              `yaml:"theme"`
	Fonts       string             `yaml:"fonts"`
	CaptionFont CaptionFont        `yaml:"caption_font"`
}

// CaptionFont names the font files captions are drawn with: Regular and Bold, each a path
// relative to the config's dir or absolute. A style left empty keeps the embedded Source Sans 3
// face.
type CaptionFont struct {
	Regular string `yaml:"regular"`
	Bold    string `yaml:"bold"`
}

// Launch is the program filmed: Command, run as `sh -c '<command>'` in Dir (relative to the
// config's dir, and that dir when omitted) with Env added. No `exec` is prefixed, so a compound
// command such as `. env.sh && exec app` works; a shell left between the pty and the app shares
// the app's process group, which is what the terminal kills. Strip lists globs of the operator's
// environment variable names removed before the launch, such as `MYAPP_*`, so the operator's own
// settings cannot steer the app away from the rig.
type Launch struct {
	Command string            `yaml:"command"`
	Dir     string            `yaml:"dir"`
	Env     map[string]string `yaml:"env"`
	Strip   []string          `yaml:"strip"`
}

// End is how a take ends: once the program has painted nothing for Idle — waiting at most
// SettleTimeout for that — photon sends Keys, and kills the program if it has not exited
// KillAfter later. [Load] fills each field the file leaves out from the Default* constants and
// [DefaultEndKeys].
type End struct {
	Keys          []string      `yaml:"keys"`
	Idle          time.Duration `yaml:"idle"`
	SettleTimeout time.Duration `yaml:"settle_timeout"`
	KillAfter     time.Duration `yaml:"kill_after"`
}

// The end-of-take defaults: a second without painting counts as idle, a busy program gets 15 s to
// fall idle, and one that will not quit is killed 10 s after the keys.
const (
	DefaultIdle          = time.Second
	DefaultSettleTimeout = 15 * time.Second
	DefaultKillAfter     = 10 * time.Second
)

// DefaultEndKeys is the quit a take ends with when the config names none: one ctrl+c, which
// ends most terminal programs.
var DefaultEndKeys = []string{"ctrl+c"}

// Theme is the palette the raster draws with: FG and BG, the default foreground and background,
// and ANSI, the 16 indexed colours. [Load] fills each part the file leaves out from
// [DefaultForeground], [DefaultBackground] and [DefaultANSI].
type Theme struct {
	FG   Color   `yaml:"fg"`
	BG   Color   `yaml:"bg"`
	ANSI []Color `yaml:"ansi"`
}

// Color is a colour written `#rrggbb`.
type Color string

// RGBA returns the colour as an opaque color.RGBA. A Color that is not `#rrggbb` — which no
// Config from [Load] holds — is black.
func (c Color) RGBA() color.RGBA {
	if !colorSpelling.MatchString(string(c)) {
		return color.RGBA{A: opaque}
	}
	value, err := strconv.ParseUint(string(c[1:]), hexBase, rgbBits)
	if err != nil {
		return color.RGBA{A: opaque}
	}
	return color.RGBA{R: uint8(value >> 16), G: uint8(value >> 8), B: uint8(value), A: opaque}
}

// The default theme: Catppuccin Mocha's text colour and ANSI colours over a near-black canvas.
const (
	DefaultForeground Color = "#cdd6f4"
	DefaultBackground Color = "#0e1117"
)

// DefaultANSI is the default theme's 16 ANSI colours, normal then bright.
var DefaultANSI = [ansiColors]Color{
	"#45475a", "#f38ba8", "#a6e3a1", "#f9e2af", "#89b4fa", "#f5c2e7", "#94e2d5", "#bac2de",
	"#585b70", "#f38ba8", "#a6e3a1", "#f9e2af", "#89b4fa", "#f5c2e7", "#94e2d5", "#a6adc8",
}

// ansiColors is the size of the ANSI palette a theme sets.
const ansiColors = 16

// How a `#rrggbb` colour parses: six hex digits, 24 bits, fully opaque.
const (
	hexBase = 16
	rgbBits = 24
	opaque  = 0xff
)

var (
	// colorSpelling is the one way a theme colour is written.
	colorSpelling = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	// projectName is the spelling of the config's name.
	projectName = regexp.MustCompile(`^[a-z0-9-]+$`)
	// envName is the spelling of an environment variable name.
	envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Load reads and validates a config. The decode is strict — an unknown field is an error — and
// every validation rule the file breaks is reported at once, in a *[ValidationError]. An area
// name is checked for its spelling only; whether a storyboard names only declared areas is the
// caller's cross-check.
func Load(file string) (*Config, error) {
	absolute, err := filepath.Abs(file)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	raw, err := os.ReadFile(absolute)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	var loaded Config
	// An empty file decodes to io.EOF; it is an empty config, which validation then reports
	// field by field rather than as a bare end-of-file.
	if err := decoder.Decode(&loaded); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if problems := loaded.validate(); len(problems) > 0 {
		return nil, &ValidationError{Path: file, Problems: problems}
	}
	loaded.Path = absolute
	loaded.Dir = filepath.Dir(absolute)
	loaded.resolvePaths()
	// Readability is checked on the resolved paths, so a relative file is found against the
	// config's dir whatever the process's working dir is.
	if problems := loaded.CaptionFont.validateFiles(); len(problems) > 0 {
		return nil, &ValidationError{Path: file, Problems: problems}
	}
	loaded.applyDefaults()
	return &loaded, nil
}

// resolvePaths joins each relative path the file gives to Dir, and sets an omitted launch dir to
// Dir itself.
func (c *Config) resolvePaths() {
	for _, field := range []*string{&c.Work, &c.Fonts, &c.CaptionFont.Regular, &c.CaptionFont.Bold} {
		if *field != "" && !filepath.IsAbs(*field) {
			*field = filepath.Join(c.Dir, *field)
		}
	}
	if !filepath.IsAbs(c.Launch.Dir) {
		c.Launch.Dir = filepath.Join(c.Dir, c.Launch.Dir)
	}
}

// applyDefaults fills what a valid config may leave out: the end of take, the theme, a
// service's timeout and an anchor's pick.
func (c *Config) applyDefaults() {
	if len(c.End.Keys) == 0 {
		c.End.Keys = slices.Clone(DefaultEndKeys)
	}
	c.End.Idle = cmp.Or(c.End.Idle, DefaultIdle)
	c.End.SettleTimeout = cmp.Or(c.End.SettleTimeout, DefaultSettleTimeout)
	c.End.KillAfter = cmp.Or(c.End.KillAfter, DefaultKillAfter)

	c.Theme.FG = cmp.Or(c.Theme.FG, DefaultForeground)
	c.Theme.BG = cmp.Or(c.Theme.BG, DefaultBackground)
	if len(c.Theme.ANSI) == 0 {
		c.Theme.ANSI = slices.Clone(DefaultANSI[:])
	}

	for name, service := range c.Services {
		service.Timeout = cmp.Or(service.Timeout, DefaultServiceTimeout)
		c.Services[name] = service
	}
	for _, area := range c.Areas {
		area.Top.applyDefaults()
		area.Bottom.applyDefaults()
	}
}

// ValidationError lists every rule a config breaks.
type ValidationError struct {
	Path     string
	Problems []error
}

// Error joins the problems one per line, under the file's path.
func (e *ValidationError) Error() string {
	lines := make([]string, 0, len(e.Problems)+1)
	lines = append(lines, fmt.Sprintf("%s: %d problem(s)", e.Path, len(e.Problems)))
	for _, problem := range e.Problems {
		lines = append(lines, "  "+problem.Error())
	}
	return strings.Join(lines, "\n")
}

// Unwrap exposes the individual problems to errors.Is and errors.As.
func (e *ValidationError) Unwrap() []error { return e.Problems }

// reporter collects one validation problem.
type reporter func(format string, args ...any)

// prefixed returns a reporter that files every problem under prefix.
func prefixed(report reporter, prefix string) reporter {
	return func(format string, args ...any) {
		report(prefix+format, args...)
	}
}

// validate applies every schema rule and returns the ones the config breaks.
func (c *Config) validate() []error {
	var problems []error
	report := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf(format, args...))
	}

	switch {
	case c.Name == "":
		report("name: missing")
	case !projectName.MatchString(c.Name):
		report("name: want lowercase letters, digits and -, got %q", c.Name)
	}
	validateCommand("setup", c.Setup, false, report)
	validateCommand("reset", c.Reset, false, report)
	c.Launch.validate(prefixed(report, "launch."))
	validateServices(c.Services, report)
	validateProfiles(c.Profiles, c.Services, report)
	c.End.validate(prefixed(report, "end."))
	validateAreas(c.Areas, report)
	c.Theme.validate(prefixed(report, "theme."))
	return problems
}

// validate checks the launch command, its env and its strip globs.
func (l Launch) validate(report reporter) {
	validateCommand("command", l.Command, true, report)
	validateEnv("env", l.Env, report)
	for index, glob := range l.Strip {
		if _, err := path.Match(glob, ""); glob == "" || err != nil {
			report("strip[%d]: want a glob of variable names, got %q", index, glob)
		}
	}
}

// validate checks that every end key is one photon can send and that no timeout is negative.
func (e End) validate(report reporter) {
	for index, name := range e.Keys {
		if _, known := keys.Lookup(name); !known {
			report("keys[%d]: unknown key %q, want one of %s", index, name, strings.Join(keys.Names(), ", "))
		}
	}
	for _, timeout := range []struct {
		name  string
		value time.Duration
	}{{"idle", e.Idle}, {"settle_timeout", e.SettleTimeout}, {"kill_after", e.KillAfter}} {
		if timeout.value < 0 {
			report("%s: want ≥0, got %s", timeout.name, timeout.value)
		}
	}
}

// validate checks that every colour the theme sets is `#rrggbb` and that an ANSI palette, when
// given, is all 16 colours.
func (t Theme) validate(report reporter) {
	validateColor("fg", t.FG, report)
	validateColor("bg", t.BG, report)
	if len(t.ANSI) != 0 && len(t.ANSI) != ansiColors {
		report("ansi: want %d colours, got %d", ansiColors, len(t.ANSI))
	}
	for index, ansi := range t.ANSI {
		validateColor(fmt.Sprintf("ansi[%d]", index), ansi, report)
	}
}

// validateFiles reports every caption font file named that cannot be opened for reading.
func (f CaptionFont) validateFiles() []error {
	var problems []error
	for _, file := range []struct{ name, path string }{{"regular", f.Regular}, {"bold", f.Bold}} {
		if file.path == "" {
			continue
		}
		opened, err := os.Open(file.path)
		if err != nil {
			problems = append(problems, fmt.Errorf("caption_font.%s: %w", file.name, err))
			continue
		}
		if err := opened.Close(); err != nil {
			problems = append(problems, fmt.Errorf("caption_font.%s: %w", file.name, err))
		}
	}
	return problems
}

// validateColor reports a set colour that is not written `#rrggbb`.
func validateColor(field string, value Color, report reporter) {
	if value != "" && !colorSpelling.MatchString(string(value)) {
		report("%s: want #rrggbb, got %q", field, value)
	}
}

// validateCommand reports a required command that is missing, and any set command that is only
// whitespace.
func validateCommand(field, command string, isRequired bool, report reporter) {
	switch {
	case command == "" && isRequired:
		report("%s: missing", field)
	case command != "" && strings.TrimSpace(command) == "":
		report("%s: want a command, got %q", field, command)
	}
}

// validateEnv reports every env entry, in name order, whose name is not a variable name.
func validateEnv(field string, env map[string]string, report reporter) {
	for _, name := range sortedKeys(env) {
		if !envName.MatchString(name) {
			report("%s: %q is not a variable name", field, name)
		}
	}
}

// validateRegex reports a missing or non-compiling regex under the field's name.
func validateRegex(field, pattern string, report reporter) {
	if pattern == "" {
		report("%s: missing", field)
		return
	}
	if _, err := regexp.Compile(pattern); err != nil {
		report("%s: %v", field, err)
	}
}

// sortedKeys returns a map's keys in order, so problems are reported in a stable order.
func sortedKeys[V any](entries map[string]V) []string {
	return slices.Sorted(maps.Keys(entries))
}
