//go:build browser

package browser

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"

	"github.com/airiclenz/photon/internal/take"
)

// textPage is the fixture the text-layer test reads: a two-word button, two inline spans on one
// line, a label beside a padded button, a pretty-printed paragraph, a hidden paragraph and a list
// whose items the item selector picks.
const textPage = `<!doctype html>
<title>photon text layer test</title>
<style>
  body { margin: 8px; font: 16px sans-serif; }
  .padded { padding: 14px 24px; font: 13px sans-serif; }
  .hidden { visibility: hidden; }
</style>
<p><button>Save file</button></p>
<p><span>left</span> <span>right</span></p>
<p><label>Name</label> <button class="padded">Go</button></p>
<p>
      Pretty
      printed
    </p>
<p class="hidden">invisible</p>
<ul><li class="item">one</li><li class="item">two</li></ul>
`

// itemSelector picks the fixture's two list items.
const itemSelector = ".item"

func TestBrowser_ReadText_ReadsRunsAndSelectorBoxes(t *testing.T) {
	b := startAt(t, textPage)

	runs, boxes, err := b.ReadText([]string{itemSelector, "#absent"})

	if err != nil {
		t.Fatalf("ReadText: %v", err)
	}
	byText := map[string]take.TextRun{}
	for _, run := range runs {
		byText[run.Text] = run
	}
	t.Run("a two-word button is one run", func(t *testing.T) {
		if _, ok := byText["Save file"]; !ok {
			t.Errorf("no run %q in %q", "Save file", textsOf(runs))
		}
	})
	t.Run("two inline spans on one line share a top", func(t *testing.T) {
		assertSameTop(t, byText, "left", "right")
	})
	t.Run("a label beside a padded button shares a top", func(t *testing.T) {
		assertSameTop(t, byText, "Name", "Go")
	})
	t.Run("a pretty-printed node holds only its rendered text", func(t *testing.T) {
		if _, ok := byText["Pretty printed"]; !ok {
			t.Errorf("no run %q in %q", "Pretty printed", textsOf(runs))
		}
	})
	t.Run("hidden text has no run", func(t *testing.T) {
		if _, ok := byText["invisible"]; ok {
			t.Errorf("hidden text has a run in %q", textsOf(runs))
		}
	})
	t.Run("every run has an edge per byte and one", func(t *testing.T) {
		for _, run := range runs {
			if len(run.Edges) != len(run.Text)+1 {
				t.Errorf("run %q has %d edges, want %d", run.Text, len(run.Edges), len(run.Text)+1)
			}
		}
	})
	t.Run("a selector's boxes match its elements", func(t *testing.T) {
		want := elementBoxes(t, b, itemSelector)
		if len(want) != 2 || !reflect.DeepEqual(boxes[itemSelector], want) {
			t.Errorf("boxes[%q] = %+v, want the two items at %+v", itemSelector, boxes[itemSelector], want)
		}
		if _, ok := boxes["#absent"]; ok {
			t.Errorf("a selector that matches nothing has boxes %+v", boxes["#absent"])
		}
	})
}

func TestBrowser_ReadText_RefusedSelectorIsAnErrorNamingIt(t *testing.T) {
	b := startAt(t, textPage)

	_, _, err := b.ReadText([]string{itemSelector, "button >"})

	if err == nil || !strings.Contains(err.Error(), `selector "button >"`) {
		t.Errorf("ReadText error = %v, want one naming selector %q", err, "button >")
	}
}

// startAt starts the browser at a test server serving page and closes it when the test ends.
func startAt(t *testing.T, page string) *Browser {
	t.Helper()
	execPath := browserOrSkip(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := fmt.Fprint(w, page); err != nil {
			t.Errorf("serve the page: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	b, err := Start(context.Background(), Options{
		URL:         server.URL,
		ExecPath:    execPath,
		Width:       viewportWidth,
		Height:      viewportHeight,
		DeviceScale: deviceScale,
		FPS:         testFPS,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if _, err := b.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return b
}

// assertSameTop fails t unless the runs with texts first and second exist and share Box.Y.
func assertSameTop(t *testing.T, byText map[string]take.TextRun, first, second string) {
	t.Helper()
	a, hasFirst := byText[first]
	b, hasSecond := byText[second]
	if !hasFirst || !hasSecond {
		t.Fatalf("want runs %q and %q, got %q", first, second, keysOf(byText))
	}
	if a.Box.Y != b.Box.Y || a.Box.H != b.Box.H {
		t.Errorf("run %q at %+v and run %q at %+v, want one top and height", first, a.Box, second, b.Box)
	}
}

// elementBoxes measures the elements selector picks with the page's own rects, rounded as
// ReadText rounds them.
func elementBoxes(t *testing.T, b *Browser, selector string) []take.Box {
	t.Helper()
	got, err := chromedp.Run(b.tab, chromedp.Evaluate[[]take.Box](fmt.Sprintf(
		`[...document.querySelectorAll(%q)].map((e) => {
			const r = e.getBoundingClientRect();
			const x = Math.round(r.left), y = Math.round(r.top);
			return {x, y, w: Math.round(r.right) - x, h: Math.round(r.bottom) - y};
		})`,
		selector,
	)))
	if err != nil {
		t.Fatalf("measure %q: %v", selector, err)
	}
	return got
}

// textsOf lists the runs' texts, for a failure message.
func textsOf(runs []take.TextRun) []string {
	texts := make([]string, len(runs))
	for i, run := range runs {
		texts[i] = run.Text
	}
	return texts
}

// keysOf lists a map's texts, for a failure message.
func keysOf(byText map[string]take.TextRun) []string {
	var texts []string
	for text := range byText {
		texts = append(texts, text)
	}
	return texts
}
