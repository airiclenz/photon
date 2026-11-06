//go:build browser

package browser

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/keys"
	"github.com/airiclenz/photon/internal/take"
)

// inputPage logs the keydown, input, mousedown, mousemove and mouseup events it sees, one line
// each, into the DOM, where the text layer reads them back. Its text field holds the focus, and
// its body has no margin, so a client point is the cell the test dispatched at.
const inputPage = `<!doctype html><title>input</title>
<style>body { margin: 0; font: 16px monospace; } input { margin: 200px 0 0 200px; }</style>
<div id="log"></div>
<input id="field">
<script>
const field = document.getElementById("field");
const log = (line) => {
	const entry = document.createElement("div");
	entry.textContent = line;
	document.getElementById("log").append(entry);
};
const mods = (e) => (e.shiftKey ? "+shift" : "") + (e.ctrlKey ? "+ctrl" : "") +
	(e.altKey ? "+alt" : "");
document.addEventListener("keydown", (e) => log("kd:" + e.key + mods(e)));
field.addEventListener("input", () => log("in:" + field.value));
document.addEventListener("mousedown", (e) =>
	log("md:" + e.button + ":" + e.clientX + "," + e.clientY + mods(e)));
document.addEventListener("mousemove", (e) =>
	log("mm:" + e.buttons + ":" + e.clientX + "," + e.clientY + mods(e)));
document.addEventListener("mouseup", (e) =>
	log("mu:" + e.button + ":" + e.clientX + "," + e.clientY + mods(e)));
field.focus();
</script>`

// logTimeout is how long the input test waits for the page to log an event, with room for a
// loaded host.
const logTimeout = 10 * time.Second

func TestBrowser_Input_ReachesThePage(t *testing.T) {
	b := startAt(t, inputPage)
	steps := []struct {
		name string
		send func() error
		want []string
	}{
		{"type", func() error { return b.Type("hi") }, []string{"kd:h", "kd:i", "in:hi"}},
		{"backspace", func() error { return b.Key("backspace") }, []string{"kd:Backspace", "in:h"}},
		{"ctrl+c", func() error { return b.Key("ctrl+c") }, []string{"kd:c+ctrl"}},
		{"alt+up", func() error { return b.Key("alt+up") }, []string{"kd:ArrowUp+alt"}},
		{"shift+tab", func() error { return b.Key("shift+tab") }, []string{"kd:Tab+shift"}},
		{"press", pointer(b, keys.PointerPress, 100, 120), []string{"md:0:100,120+shift"}},
		{"move", pointer(b, keys.PointerMove, 110, 130), []string{"mm:1:110,130+shift"}},
		{"release", pointer(b, keys.PointerRelease, 110, 130), []string{"mu:0:110,130+shift"}},
	}

	for _, step := range steps {
		if err := step.send(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		waitForLog(t, b, step.name, step.want)
	}
}

func TestBrowser_Key_UnknownNameIsAnErrorNamingIt(t *testing.T) {
	b := startAt(t, inputPage)

	err := b.Key("ctrl+z")

	if err == nil || !strings.Contains(err.Error(), `"ctrl+z"`) {
		t.Errorf("Key error = %v, want one naming %q", err, "ctrl+z")
	}
}

// pointer returns a step that dispatches a left-button pointer event of kind at x,y with shift
// held.
func pointer(b *Browser, kind keys.PointerKind, x, y int) func() error {
	return func() error {
		return b.Pointer(keys.Pointer{Kind: kind, X: x, Y: y, Mouse: keys.Mouse{Mods: keys.Shift}})
	}
}

// waitForLog waits, through Match, until every line of want is among the page's logged lines,
// and fails t naming step when they are not within logTimeout.
func waitForLog(t *testing.T, b *Browser, step string, want []string) {
	t.Helper()
	hasAll := func(s take.Snapshot) bool {
		lines := textsOf(s.Runs)
		return !slices.ContainsFunc(want, func(line string) bool {
			return !slices.Contains(lines, line)
		})
	}
	deadline := time.Now().Add(logTimeout)
	for !b.Match(hasAll) {
		if time.Now().After(deadline) {
			t.Fatalf("%s: the page did not log %q within %s; it logged %q", step, want,
				logTimeout, textsOf(b.Current().Runs))
		}
		time.Sleep(settlePollInterval)
	}
}
