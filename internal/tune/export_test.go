package tune

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/render"
	"github.com/airiclenz/photon/internal/storyboard"
)

// ffmpegProgram is the encoder an export's render runs; a test that renders skips without it.
const ffmpegProgram = "ffmpeg"

// exportTimeout bounds how long a test waits for an export to finish.
const exportTimeout = 2 * time.Minute

// exportPollInterval is how often a test asks the state whether an export has finished.
const exportPollInterval = 50 * time.Millisecond

// heldRender is a render seam that holds an export running until released: it hands the test the
// options each render is given, then waits.
type heldRender struct {
	started chan render.Options
	release chan struct{}
}

// newHeldRender returns a heldRender that holds every render until release is closed.
func newHeldRender() *heldRender {
	return &heldRender{started: make(chan render.Options, 1), release: make(chan struct{})}
}

// render is the seam: it reports options, then returns nil once released.
func (h *heldRender) render(
	_ context.Context,
	_ *clip.Clip,
	options render.Options,
	_, _ io.Writer,
) error {
	h.started <- options
	<-h.release
	return nil
}

// holdExports makes session's exports render through a new heldRender, released and awaited
// before the test returns.
func holdExports(t *testing.T, session *Session) *heldRender {
	t.Helper()
	held := newHeldRender()
	session.renderClip = held.render
	t.Cleanup(func() {
		close(held.release)
		session.waitExport()
	})
	return held
}

// pollUntilExported asks handler for the state until the export no longer runs, and returns it.
func pollUntilExported(t *testing.T, handler http.Handler) State {
	t.Helper()
	deadline := time.Now().Add(exportTimeout)
	for {
		var state State
		response := get(t, handler, "/api/state")
		assertStatus(t, response, http.StatusOK)
		decodeResponse(t, response, &state)
		if !state.Export.IsRunning {
			return state
		}
		if time.Now().After(deadline) {
			t.Fatalf("export still running after %v: %+v", exportTimeout, state.Export)
		}
		time.Sleep(exportPollInterval)
	}
}

// readFile returns the bytes of the file at path.
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return contents
}

func TestHandler_Export_WritesTheGIFPhotonRenderWrites(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath(ffmpegProgram); err != nil {
		t.Skip("ffmpeg is not on PATH")
	}
	fixture := newSessionFixture(t)
	handler := newHandler(fixture.session, testAddr)

	assertStatus(t, post(t, handler, "/api/export", `{}`), http.StatusAccepted)
	state := pollUntilExported(t, handler)

	if state.Export.Error != "" || state.Export.Out == "" {
		t.Fatalf("export: want an out and no error, got %+v", state.Export)
	}
	if state.Export.Total == 0 || state.Export.Done != state.Export.Total {
		t.Errorf("export progress: want done = total > 0, got %+v", state.Export)
	}
	loaded, err := clip.Load(fixture.boardPath, "", t.TempDir())
	if err != nil {
		t.Fatalf("clip.Load: %v", err)
	}
	reference := filepath.Join(t.TempDir(), "reference.gif")
	options := render.Options{Take: fixture.takePath, Out: reference}
	if err := render.Run(context.Background(), loaded, options, io.Discard, io.Discard); err != nil {
		t.Fatalf("render.Run: %v", err)
	}
	if !bytes.Equal(readFile(t, state.Export.Out), readFile(t, reference)) {
		t.Errorf("the exported GIF %s differs from photon render's", state.Export.Out)
	}
}

func TestHandler_Export_AnswersTheStateAndProgressKeepsTheVersion(t *testing.T) {
	t.Parallel()
	fixture := newSessionFixture(t)
	held := holdExports(t, fixture.session)
	handler := newHandler(fixture.session, testAddr)

	response := post(t, handler, "/api/export", `{}`)
	options := <-held.started
	options.Progress(1, 4)

	assertStatus(t, response, http.StatusAccepted)
	var answered State
	decodeResponse(t, response, &answered)
	if !answered.Export.IsRunning || answered.Export.Out != "" || answered.Export.Error != "" {
		t.Errorf("answered export: want running with no outcome, got %+v", answered.Export)
	}
	ticked := fixture.session.State()
	if ticked.Version != answered.Version {
		t.Errorf("version: want %d kept by a progress tick, got %d", answered.Version, ticked.Version)
	}
	if want := (ExportState{IsRunning: true, Done: 1, Total: 4}); ticked.Export != want {
		t.Errorf("export after a tick: want %+v, got %+v", want, ticked.Export)
	}
}

func TestHandler_Export_WhileOneRuns_Is409(t *testing.T) {
	t.Parallel()
	fixture := newSessionFixture(t)
	held := holdExports(t, fixture.session)
	handler := newHandler(fixture.session, testAddr)
	assertStatus(t, post(t, handler, "/api/export", `{}`), http.StatusAccepted)
	<-held.started

	response := post(t, handler, "/api/export", `{}`)

	assertStatus(t, response, http.StatusConflict)
}

func TestHandler_Export_FinishedHold_ReportsTheShipPath(t *testing.T) {
	t.Parallel()
	fixture := newSessionFixture(t)
	held := newHeldRender()
	fixture.session.renderClip = held.render
	handler := newHandler(fixture.session, testAddr)
	assertStatus(t, post(t, handler, "/api/export", `{}`), http.StatusAccepted)
	<-held.started

	close(held.release)
	fixture.session.waitExport()

	want := filepath.Join(filepath.Dir(fixture.boardPath), "out", "tuned.gif")
	if got := fixture.session.State().Export; got != (ExportState{Out: want}) {
		t.Errorf("finished export: want out %s and nothing else, got %+v", want, got)
	}
}

func TestHandler_Export_UncleanSession_Is409(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		unclean func(t *testing.T, fixture sessionFixture)
	}{
		{
			name: "unsaved edits",
			unclean: func(t *testing.T, fixture sessionFixture) {
				t.Helper()
				fixture.apply(t, beatEdit(2, storyboard.FieldDuration, 1800))
			},
		},
		{
			name: "an error standing on a clean board",
			unclean: func(t *testing.T, fixture sessionFixture) {
				t.Helper()
				if err := os.Remove(fixture.takePath); err != nil {
					t.Fatalf("remove the take: %v", err)
				}
				if err := fixture.session.Refresh(); err != nil {
					t.Fatalf("Refresh: %v", err)
				}
			},
		},
		{
			name: "a storyboard rewritten on disk",
			unclean: func(t *testing.T, fixture sessionFixture) {
				t.Helper()
				fixture.writeOutside(t)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fixture := newSessionFixture(t)
			held := holdExports(t, fixture.session)
			handler := newHandler(fixture.session, testAddr)
			tt.unclean(t, fixture)

			response := post(t, handler, "/api/export", `{}`)

			assertStatus(t, response, http.StatusConflict)
			select {
			case <-held.started:
				t.Error("a refused export rendered")
			default:
			}
		})
	}
}

func TestSession_Export_FailingRender_ReportsItsError(t *testing.T) {
	t.Parallel()
	fixture := newSessionFixture(t)
	returned := make(chan error, 1)
	fixture.session.renderClip = func(
		ctx context.Context,
		loaded *clip.Clip,
		options render.Options,
		stdout, stderr io.Writer,
	) error {
		err := render.Run(ctx, loaded, options, stdout, stderr)
		returned <- err
		return err
	}
	if err := os.Remove(fixture.takePath); err != nil {
		t.Fatalf("remove the take: %v", err)
	}
	before := fixture.session.State()

	if err := fixture.session.Export(); err != nil {
		t.Fatalf("Export: %v", err)
	}
	fixture.session.waitExport()

	err := <-returned
	if err == nil {
		t.Fatal("render.Run: want an error for a missing take, got nil")
	}
	after := fixture.session.State()
	if want := (ExportState{Error: err.Error()}); after.Export != want {
		t.Errorf("failed export: want %+v, got %+v", want, after.Export)
	}
	if after.IsDirty != before.IsDirty || after.Version != before.Version {
		t.Errorf("session: want dirty %v at version %d kept, got dirty %v at version %d",
			before.IsDirty, before.Version, after.IsDirty, after.Version)
	}
}
