//go:build browser

package browser

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/airiclenz/photon/internal/pixels"
	"github.com/airiclenz/photon/internal/take"
)

// changingPage shows "one" in an element the test changes the text of.
const changingPage = `<!doctype html><title>changing</title>
<style>body { margin: 0; font: 24px monospace; }</style>
<p id="text">one</p>`

// The waits the recording test hands the browser, with room for a loaded host.
const (
	firstFrameTimeout = 10 * time.Second
	textTimeout       = 10 * time.Second
	stillSpell        = 300 * time.Millisecond
	stillTimeout      = 5 * time.Second
)

func TestBrowser_Record_KeepsEachChangeAsASnapshot(t *testing.T) {
	b := startAt(t, changingPage)
	waitForFirstFrame(t, b)
	for _, text := range []string{"two", "three"} {
		setText(t, b, text)
		waitForText(t, b, text)
	}
	if !b.Settle(context.Background(), stillSpell, stillTimeout) {
		t.Fatalf("the page did not go still within %s", stillTimeout)
	}
	b.Freeze()
	recorded, err := b.Close()
	if err != nil {
		t.Fatalf("Close: %v", err)
	}

	path := filepath.Join(t.TempDir(), "page.take")
	if err := take.Save(path, recorded); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := take.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.Cols != viewportWidth || loaded.Rows != viewportHeight ||
		loaded.Cell != (take.CellSize{W: deviceScale, H: deviceScale}) || loaded.FPS != testFPS {
		t.Errorf("take = %dx%d, cell %+v, %d fps; want %dx%d, cell %d, %d fps",
			loaded.Cols, loaded.Rows, loaded.Cell, loaded.FPS,
			viewportWidth, viewportHeight, deviceScale, testFPS)
	}
	if len(loaded.Snapshots) != len(recorded.Snapshots) {
		t.Fatalf("loaded %d snapshots, recorded %d", len(loaded.Snapshots), len(recorded.Snapshots))
	}
	first := loaded.Snapshots[0]
	if first.At != 0 || first.Image == nil || !first.Image.Key {
		t.Errorf("first snapshot at %v, image %+v; want a key frame at 0 s", first.At, first.Image)
	}
	assertFrameSize(t, loaded)

	var texts []string
	hasPatch := false
	for index, snapshot := range loaded.Snapshots {
		if len(snapshot.Runs) == 0 {
			t.Errorf("snapshot %d has no text runs", index)
		}
		if snapshot.Image != nil && !snapshot.Image.Key {
			hasPatch = true
		}
		if index > 0 && snapshot.At-loaded.Snapshots[index-1].At <= 0 {
			t.Errorf("snapshot %d at %v, not after the one before", index, snapshot.At)
		}
		text := strings.Join(textsOf(snapshot.Runs), " ")
		if len(texts) == 0 || texts[len(texts)-1] != text {
			texts = append(texts, text)
		}
	}
	if !hasPatch {
		t.Error("no snapshot holds a patch: every change was kept as a key frame or not at all")
	}
	if want := []string{"one", "two", "three"}; !slices.Equal(texts, want) {
		t.Errorf("texts in order = %q, want %q", texts, want)
	}
}

// waitForFirstFrame waits until the take holds its first snapshot, and fails t when it does not
// within firstFrameTimeout.
func waitForFirstFrame(t *testing.T, b *Browser) {
	t.Helper()
	deadline := time.Now().Add(firstFrameTimeout)
	for {
		b.rec.mu.Lock()
		recorded := len(b.rec.take.Snapshots)
		b.rec.mu.Unlock()
		if recorded > 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the page pushed no frame within %s", firstFrameTimeout)
		}
		time.Sleep(settlePollInterval)
	}
}

// setText sets the text of the page's #text element.
func setText(t *testing.T, b *Browser, text string) {
	t.Helper()
	script := fmt.Sprintf(`(document.getElementById("text").textContent = %q, true)`, text)
	if _, err := chromedp.Run(b.tab, chromedp.Evaluate[bool](script)); err != nil {
		t.Fatalf("set the text to %q: %v", text, err)
	}
}

// waitForText waits, through Match, until the page's text layer reads want, and fails t when it
// does not within textTimeout.
func waitForText(t *testing.T, b *Browser, want string) {
	t.Helper()
	deadline := time.Now().Add(textTimeout)
	for !b.Match(func(s take.Snapshot) bool { return strings.Join(textsOf(s.Runs), " ") == want }) {
		if time.Now().After(deadline) {
			t.Fatalf("the page did not read %q within %s; it reads %q", want, textTimeout,
				textsOf(b.Current().Runs))
		}
		time.Sleep(settlePollInterval)
	}
}

// assertFrameSize fails t unless the take's first frame is the viewport in device pixels.
func assertFrameSize(t *testing.T, recorded *take.Take) {
	t.Helper()
	picture, err := pixels.NewDecoder(recorded).Frame(0)
	if err != nil {
		t.Fatalf("decode the first frame: %v", err)
	}
	want := [2]int{viewportWidth * deviceScale, viewportHeight * deviceScale}
	if got := [2]int{picture.Rect.Dx(), picture.Rect.Dy()}; got != want {
		t.Errorf("first frame is %dx%d device pixels, want %dx%d", got[0], got[1], want[0], want[1])
	}
}
