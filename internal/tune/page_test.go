package tune

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"testing"
)

// pageFileNames are the files the page is made of, each embedded under page/.
var pageFileNames = []string{"index.html", "style.css", "app.js"}

// tuneContract lists every member of the window.tune object app.js exposes to the lanes and
// the target picker.
var tuneContract = []string{
	"state", "selected", "select", "time", "frameTime", "seek", "pause", "edit", "onState", "frame",
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

	for _, name := range pageFileNames {
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
	for _, name := range pageFileNames[1:] {
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
