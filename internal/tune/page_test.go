package tune

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	// goja is a test-only dependency: a JavaScript engine in Go, so the page's scripts are parsed
	// by a real engine in `go test` without a browser or node on the host.
	"github.com/dop251/goja"

	"github.com/airiclenz/photon/internal/storyboard"
)

// pageScriptPattern finds every script of the page among the embedded files.
const pageScriptPattern = "*.js"

// pageAssetNames are the files index.html loads besides its scripts.
var pageAssetNames = []string{"style.css"}

// splitScripts are app.js and the scripts split from it, each kept under maxScriptLines.
var splitScripts = []string{"app.js", "transport.js", "controls.js", "panels.js", "entries.js"}

// maxScriptLines is the file-size guide of Note coding-conventions a split script stays below.
const maxScriptLines = 400

// pickerCalls are what picker.js must call: the pick endpoint and the clipboard its copy buttons
// write the snippets to.
var pickerCalls = []string{"/api/pick", "navigator.clipboard.writeText"}

// tuneContract lists every member of the window.tune object app.js exposes to the lanes and
// the target picker.
var tuneContract = []string{
	"state", "selected", "select", "selectedEntry", "selectEntry", "time", "frameTime", "seek", "pause",
	"edit", "onState", "frame",
}

// registeredLanes are the lanes lanes.js puts in window.tuneLanes: the name each shows and the
// class of its bars, which style.css colours.
var registeredLanes = []struct {
	name     string
	barClass string
}{
	{"camera zoom", "camera-zoom"},
	{"pan", "pan"},
	{"captions", "caption"},
}

// entryReaders are the variables the page reads state entries through, each with the state type
// whose json keys it may name: state.<key>, move.<key>, caption.<key>, moment.<key>,
// problem.<key>, owner.<key> and field.<key>.
var entryReaders = []struct {
	variable string
	of       any
}{
	{"state", State{}},
	{"move", CameraState{}},
	{"caption", CaptionState{}},
	{"moment", MomentState{}},
	{"problem", Problem{}},
	{"owner", storyboard.Owner{}},
	{"field", storyboard.Field{}},
}

// pendingPageFields are the storyboard's editable entry fields the page does not send yet; each
// is exempt from the check that the page sends every entry field, never from the check that every
// field the page sends is one the storyboard describes.
var pendingPageFields = map[string]bool{
	storyboard.FieldCameraInsert:  true,
	storyboard.FieldCameraRemove:  true,
	storyboard.FieldCameraMove:    true,
	storyboard.FieldCaptionInsert: true,
	storyboard.FieldCaptionRemove: true,
	storyboard.FieldCaptionMove:   true,
}

// entryFieldName finds a camera move's or caption's edit field written as a string literal.
var entryFieldName = regexp.MustCompile(`"((?:camera|caption)\.[a-z_]+)"`)

// stringLiteral finds a double-quoted JavaScript string literal; yamlFlowKey finds, inside one,
// a storyboard key spelled as YAML flow text ({beat: 3, offset: 200ms}, {text: 'ok', nth: 2}),
// which the page, sending typed values, never spells.
var (
	stringLiteral = regexp.MustCompile(`"(?:[^"\\\n]|\\.)*"`)
	yamlFlowKey   = regexp.MustCompile(`(?:^|[{,] ?)(?:beat|offset|text|nth|area|cell|x|y|target|factor): `)
)

// yamlSpellings returns the string literals in script that spell storyboard keys as YAML.
func yamlSpellings(script []byte) []string {
	var spellings []string
	for _, literal := range stringLiteral.FindAll(script, -1) {
		if yamlFlowKey.Match(literal[1 : len(literal)-1]) {
			spellings = append(spellings, string(literal))
		}
	}
	return spellings
}

// externalFetch finds a URL in fetch position — an attribute, a CSS url(), an ES import or a
// fetch() — that leaves this server: absolute or protocol-relative. A namespace URI is no fetch.
var externalFetch = regexp.MustCompile(
	`((src|href)\s*=\s*["']?|url\(\s*["']?|(from|import)\s*\(?\s*["']|fetch\(\s*["'])(https?:)?//`,
)

// pageReference finds what index.html loads: every src and href attribute's value.
var pageReference = regexp.MustCompile(`\b(?:src|href)\s*=\s*["']([^"']+)["']`)

// tuneObject is the window.tune assignment in app.js, up to the end of its object literal.
var tuneObject = regexp.MustCompile(`(?s)window\.tune\s*=\s*Object\.freeze\(\{(.*?)\n\s*\}\);`)

// jsonKeys returns the json keys value's struct type encodes its fields under.
func jsonKeys(t *testing.T, value any) map[string]bool {
	t.Helper()
	kind := reflect.TypeOf(value)
	keys := make(map[string]bool, kind.NumField())
	for field := range kind.Fields() {
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		keys[name] = true
	}
	return keys
}

// pageScripts returns the name of every script embedded under page/, so a check over the page's
// scripts reaches one added later without a list to update.
func pageScripts(t *testing.T) []string {
	t.Helper()
	paths, err := fs.Glob(pageFiles, path.Join(pageDir, pageScriptPattern))
	if err != nil {
		t.Fatalf("list the page scripts: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("the page embeds no script")
	}
	names := make([]string, 0, len(paths))
	for _, script := range paths {
		names = append(names, path.Base(script))
	}
	return names
}

// pageLoads returns the files index.html must load: its assets and every page script.
func pageLoads(t *testing.T) []string {
	t.Helper()
	return append(slices.Clone(pageAssetNames), pageScripts(t)...)
}

// readPageFile returns the embedded page file name, failing the test when it is missing.
func readPageFile(t *testing.T, name string) []byte {
	t.Helper()
	content, err := fs.ReadFile(pageFiles, path.Join(pageDir, name))
	if err != nil {
		t.Fatalf("the page file %s is not embedded: %v", name, err)
	}
	return content
}

func TestPage_Files_AreEmbedded(t *testing.T) {
	t.Parallel()

	for _, name := range append([]string{pageIndex}, pageLoads(t)...) {
		if content := readPageFile(t, name); len(content) == 0 {
			t.Errorf("the page file %s is empty", name)
		}
	}
}

func TestPage_IndexReferences_AreServed(t *testing.T) {
	t.Parallel()
	handler := newHandler(newSessionFixture(t).session, testAddr)
	index := string(readPageFile(t, pageIndex))
	references := pageReference.FindAllStringSubmatch(index, -1)
	root := &url.URL{Path: "/"}

	served := map[string]bool{}
	for _, reference := range references {
		target, err := root.Parse(reference[1])
		if err != nil {
			t.Fatalf("index.html references %q, which is no URL: %v", reference[1], err)
		}
		if !strings.HasPrefix(target.Path, assetsPrefix) {
			t.Errorf("index.html references %q, outside %s", reference[1], assetsPrefix)
			continue
		}
		name := strings.TrimPrefix(target.Path, assetsPrefix)

		response := get(t, handler, target.Path)

		assertStatus(t, response, http.StatusOK)
		if !bytes.Equal(response.Body.Bytes(), readPageFile(t, name)) {
			t.Errorf("GET %s: want the embedded %s", target.Path, name)
		}
		served[name] = true
	}
	for _, name := range pageLoads(t) {
		if !served[name] {
			t.Errorf("index.html does not load %s", name)
		}
	}
}

func TestPage_ExternalFetch_Pattern(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		text       string
		isExternal bool
	}{
		{"quoted script src", `<script src="https://example.com/x.js"></script>`, true},
		{"unquoted href", `<link href=http://example.com/x.css>`, true},
		{"protocol-relative src", `<img src='//example.com/x.png'>`, true},
		{"css url", `body { background: url( "https://example.com/x.png") }`, true},
		{"es import from", `import x from 'https://example.com/x.js';`, true},
		{"dynamic import", `import("https://example.com/x.js")`, true},
		{"fetch", `fetch('https://example.com/api')`, true},
		{"local asset", `<script src="/assets/app.js"></script>`, false},
		{"local fetch", `fetch("/api/state")`, false},
		{"svg namespace", `<svg xmlns="http://www.w3.org/2000/svg"></svg>`, false},
		{"namespace in script", `document.createElementNS("http://www.w3.org/2000/svg", "rect")`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			isExternal := externalFetch.MatchString(tc.text)

			if isExternal != tc.isExternal {
				t.Errorf("%q: external fetch %v, want %v", tc.text, isExternal, tc.isExternal)
			}
		})
	}
}

func TestPage_EmbeddedFiles_FetchNothingExternal(t *testing.T) {
	t.Parallel()

	err := fs.WalkDir(pageFiles, pageDir, func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := fs.ReadFile(pageFiles, name)
		if err != nil {
			return err
		}
		if match := externalFetch.Find(content); match != nil {
			t.Errorf("%s fetches from outside the server: %q", name, match)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("walk the embedded page: %v", err)
	}
}

func TestPage_App_DefinesTheTuneContract(t *testing.T) {
	t.Parallel()
	app := readPageFile(t, "app.js")

	object := tuneObject.FindSubmatch(app)

	if object == nil {
		t.Fatal("app.js does not assign window.tune = Object.freeze({…})")
	}
	for _, member := range tuneContract {
		pattern := regexp.MustCompile(`(?m)^\s*` + member + `\s*:`)
		if !pattern.Match(object[1]) {
			t.Errorf("window.tune has no member %s", member)
		}
	}
}

func TestPage_Picker_CallsPickAndClipboard(t *testing.T) {
	t.Parallel()
	picker := string(readPageFile(t, "picker.js"))

	for _, call := range pickerCalls {
		if !strings.Contains(picker, call) {
			t.Errorf("picker.js does not contain %s", call)
		}
	}
}

func TestPage_EntryFields_AreEditFields(t *testing.T) {
	t.Parallel()
	editable := map[string]bool{}
	for _, field := range newSessionFixtureOf(t, cameraCaptionsStoryboard).session.State().Fields {
		if !field.IsReadOnly {
			editable[field.Name] = true
		}
	}
	sent := map[string]bool{}

	for _, name := range pageScripts(t) {
		for _, match := range entryFieldName.FindAllSubmatch(readPageFile(t, name), -1) {
			field := string(match[1])
			sent[field] = true
			if !editable[field] {
				t.Errorf("%s sends the field %s, which the storyboard does not describe as editable", name, field)
			}
		}
	}

	for field := range editable {
		isEntryField := strings.HasPrefix(field, "camera.") || strings.HasPrefix(field, "caption.")
		if isEntryField && !sent[field] && !pendingPageFields[field] {
			t.Errorf("the page sends no %s edit", field)
		}
	}
}

func TestPage_YAMLSpellings_Pattern(t *testing.T) {
	t.Parallel()
	cases := []struct {
		script string
		want   []string
	}{
		{`return "{beat: " + moment.beat + offset + "}";`, []string{`"{beat: "`}},
		{`const offset = ", offset: " + ms + "ms";`, []string{`", offset: "`}},
		{`const hint = "{text: 'ok', area: list}";`, []string{`"{text: 'ok', area: list}"`}},
		{`applyEdit(owner, field, { beat: Number(beat.value), offset_ms: 0 });`, nil},
		{`element("input", { type: "number", step: "1", value: "0" });`, nil},
		{`console.error("tune: a state listener failed", err);`, nil},
		{`const quote = "say \"{nth: 2}\"";`, []string{`"say \"{nth: 2}\""`}},
	}
	for _, tc := range cases {
		if got := yamlSpellings([]byte(tc.script)); !slices.Equal(got, tc.want) {
			t.Errorf("yamlSpellings(%s) = %q, want %q", tc.script, got, tc.want)
		}
	}
}

func TestPage_Scripts_SpellNoYAML(t *testing.T) {
	t.Parallel()
	for _, name := range pageScripts(t) {
		for _, spelling := range yamlSpellings(readPageFile(t, name)) {
			t.Errorf("%s spells YAML in %s; send the value typed instead", name, spelling)
		}
	}
}

func TestPage_Scripts_Parse(t *testing.T) {
	t.Parallel()
	for _, name := range pageScripts(t) {
		source := string(readPageFile(t, name))

		_, err := goja.Compile(name, source, true)

		if err != nil {
			t.Errorf("%s does not parse as JavaScript: %v", name, err)
		}
	}
}

func TestPage_Lanes_RegisterNamedLanesWithStyledBars(t *testing.T) {
	t.Parallel()
	lanes := string(readPageFile(t, "lanes.js"))
	style := string(readPageFile(t, "style.css"))

	if !strings.Contains(lanes, "window.tuneLanes =") {
		t.Fatal("lanes.js does not fill window.tuneLanes")
	}
	for _, lane := range registeredLanes {
		if !strings.Contains(lanes, `name: "`+lane.name+`"`) {
			t.Errorf("lanes.js registers no lane named %q", lane.name)
		}
		if !strings.Contains(lanes, `"`+lane.barClass+`"`) {
			t.Errorf("lanes.js gives no bar the class %s", lane.barClass)
		}
		if !strings.Contains(style, ".block."+lane.barClass+" {") {
			t.Errorf("style.css does not style .block.%s", lane.barClass)
		}
	}
}

func TestPage_Index_LoadsLanesBeforeTimeline(t *testing.T) {
	t.Parallel()
	index := string(readPageFile(t, pageIndex))

	lanes := strings.Index(index, assetsPrefix+"lanes.js")
	timeline := strings.Index(index, assetsPrefix+"timeline.js")

	if lanes < 0 || timeline < 0 || lanes > timeline {
		t.Errorf("index.html loads lanes.js at %d and timeline.js at %d; want lanes.js first", lanes, timeline)
	}
}

func TestPage_EntryReads_AreStateKeys(t *testing.T) {
	t.Parallel()
	for _, name := range pageScripts(t) {
		content := readPageFile(t, name)
		for _, reader := range entryReaders {
			keys := jsonKeys(t, reader.of)
			// A quoted name is an edit field, such as "caption.for", not a read, and a name after
			// a dot, as in change.field.split, is another variable's member.
			access := regexp.MustCompile(`(?:^|[^"\w.])` + reader.variable + `\.([a-z_]+)`)
			for _, match := range access.FindAllSubmatch(content, -1) {
				if key := string(match[1]); !keys[key] {
					t.Errorf("%s reads %s.%s, which /api/state does not send", name, reader.variable, key)
				}
			}
		}
	}
}

func TestPage_Lanes_ReadCameraAndCaptions(t *testing.T) {
	t.Parallel()
	lanes := string(readPageFile(t, "lanes.js"))
	keys := []string{"camera", "captions"}

	for _, key := range keys {
		if !strings.Contains(lanes, "state."+key) {
			t.Errorf("lanes.js does not read state.%s", key)
		}
	}
}

func TestPage_SplitScripts_StayBelowTheFileSizeGuide(t *testing.T) {
	t.Parallel()

	for _, name := range splitScripts {
		lines := bytes.Count(readPageFile(t, name), []byte("\n"))

		if lines >= maxScriptLines {
			t.Errorf("%s has %d lines, want fewer than %d", name, lines, maxScriptLines)
		}
	}
}

func TestPage_App_WiresOnceEverySplitScriptHasRun(t *testing.T) {
	t.Parallel()
	index := string(readPageFile(t, pageIndex))
	app := string(readPageFile(t, "app.js"))

	appAt := strings.Index(index, assetsPrefix+"app.js")

	for _, name := range splitScripts[1:] {
		if at := strings.Index(index, assetsPrefix+name); at < 0 || at < appAt {
			t.Errorf("index.html loads %s at %d and app.js at %d; want app.js first", name, at, appAt)
		}
	}
	if !strings.Contains(app, `document.addEventListener("DOMContentLoaded", wire)`) {
		t.Error("app.js does not wire the page on DOMContentLoaded, after every split script has run")
	}
}
