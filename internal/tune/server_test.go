package tune

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/clip"
)

// testAddr is the listen address the handler tests build the handler for.
const testAddr = "127.0.0.1:4711"

// printedPrefix starts the line Run prints, before the page's URL.
const printedPrefix = "photon tune: "

// runStopTimeout bounds how long Run may take to return once its context ends.
const runStopTimeout = 10 * time.Second

// serveRequest sends a request for method and target with body, declared as contentType when it
// is not empty, to handler under host, and returns the recorded response.
func serveRequest(
	t *testing.T,
	handler http.Handler,
	host, method, target, contentType, body string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.Host = host
	if contentType != "" {
		request.Header.Set(headerContentType, contentType)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// get sends a GET for target to handler at testAddr.
func get(t *testing.T, handler http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	return serveRequest(t, handler, testAddr, http.MethodGet, target, "", "")
}

// post sends body as JSON to target on handler at testAddr.
func post(t *testing.T, handler http.Handler, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	return serveRequest(t, handler, testAddr, http.MethodPost, target, mediaJSON, body)
}

// assertStatus fails the test when response is not status, showing its body.
func assertStatus(t *testing.T, response *httptest.ResponseRecorder, status int) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status: want %d, got %d: %s", status, response.Code, response.Body.String())
	}
}

// decodeResponse decodes response's JSON body into into.
func decodeResponse(t *testing.T, response *httptest.ResponseRecorder, into any) {
	t.Helper()
	if got := response.Header().Get(headerContentType); got != mediaJSON {
		t.Fatalf("content type: want %s, got %q", mediaJSON, got)
	}
	if err := json.Unmarshal(response.Body.Bytes(), into); err != nil {
		t.Fatalf("decode %s: %v", response.Body.String(), err)
	}
}

// frameOf gets the frame at ms from handler and returns its PNG, failing the test unless it is
// one.
func frameOf(t *testing.T, handler http.Handler, ms string) []byte {
	t.Helper()
	response := get(t, handler, "/api/frame?t="+ms)
	assertStatus(t, response, http.StatusOK)
	if got := response.Header().Get(headerContentType); got != mediaPNG {
		t.Fatalf("content type: want %s, got %q", mediaPNG, got)
	}
	if _, err := png.Decode(bytes.NewReader(response.Body.Bytes())); err != nil {
		t.Fatalf("decode the frame: %v", err)
	}
	return response.Body.Bytes()
}

func TestHandler_State_ReportsTheSession(t *testing.T) {
	t.Parallel()
	handler := newHandler(newSessionFixture(t).session, testAddr)

	response := get(t, handler, "/api/state")

	assertStatus(t, response, http.StatusOK)
	var state State
	decodeResponse(t, response, &state)
	if len(state.Beats) != 2 || beatDuration(t, state, 1) != beatOneMS {
		t.Errorf("state: want 2 beats, beat 1 lasting %dms, got %+v", beatOneMS, state.Beats)
	}
}

func TestHandler_Page_IsServed(t *testing.T) {
	t.Parallel()
	handler := newHandler(newSessionFixture(t).session, testAddr)

	for _, target := range []string{"/", assetsPrefix} {
		response := get(t, handler, target)

		assertStatus(t, response, http.StatusOK)
		if !strings.Contains(response.Body.String(), "<html") {
			t.Errorf("GET %s: want the page, got %q", target, response.Body.String())
		}
	}
}

func TestHandler_Frame_IsTheSessionsFrameAsPNG(t *testing.T) {
	t.Parallel()
	fixture := newSessionFixture(t)
	handler := newHandler(fixture.session, testAddr)

	frame := frameOf(t, handler, "500")

	want, err := fixture.session.Frame(500 * time.Millisecond)
	if err != nil {
		t.Fatalf("Frame: %v", err)
	}
	if !bytes.Equal(frame, want) {
		t.Error("the served frame is not the session's frame")
	}
}

func TestHandler_Frame_BadTime_IsRefused(t *testing.T) {
	t.Parallel()
	handler := newHandler(newSessionFixture(t).session, testAddr)
	tests := []struct {
		name   string
		query  string
		status int
	}{
		{name: "not a number", query: "soon", status: http.StatusBadRequest},
		{name: "missing", query: "", status: http.StatusBadRequest},
		{name: "outside the clip", query: "60000", status: http.StatusUnprocessableEntity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := get(t, handler, "/api/frame?t="+test.query)

			assertStatus(t, response, test.status)
		})
	}
}

func TestHandler_EditThenFrame_ChangesTheFrame(t *testing.T) {
	t.Parallel()
	handler := newHandler(newSessionFixture(t).session, testAddr)
	before := frameOf(t, handler, "500")

	response := post(t, handler, "/api/edit", `{"beat": 1, "field": "duration", "value": "2000"}`)

	assertStatus(t, response, http.StatusOK)
	var state State
	decodeResponse(t, response, &state)
	if got := beatDuration(t, state, 1); got != 2000 || !state.IsDirty {
		t.Errorf("state after the edit: want beat 1 at 2000ms and dirty, got %dms, dirty %v", got, state.IsDirty)
	}
	if after := frameOf(t, handler, "500"); bytes.Equal(after, before) {
		t.Error("the frame is unchanged by the edit")
	}
}

func TestHandler_RefusedEdit_Is422(t *testing.T) {
	t.Parallel()
	handler := newHandler(newSessionFixture(t).session, testAddr)

	response := post(t, handler, "/api/edit", `{"beat": 9, "field": "duration", "value": "2000"}`)

	assertStatus(t, response, http.StatusUnprocessableEntity)
	var failure apiError
	decodeResponse(t, response, &failure)
	if failure.Error == "" {
		t.Error("the refusal carries no message")
	}
}

func TestHandler_UndoRedoRevert_MoveThroughVersions(t *testing.T) {
	t.Parallel()
	handler := newHandler(newSessionFixture(t).session, testAddr)
	assertStatus(t, post(t, handler, "/api/undo", `{}`), http.StatusUnprocessableEntity)
	assertStatus(t, post(t, handler, "/api/edit", `{"beat": 1, "field": "duration", "value": "2000"}`), http.StatusOK)
	steps := []struct {
		route  string
		wantMS int64
	}{
		{route: "/api/undo", wantMS: beatOneMS},
		{route: "/api/redo", wantMS: 2000},
		{route: "/api/revert", wantMS: beatOneMS},
	}
	for _, step := range steps {
		response := post(t, handler, step.route, `{}`)

		assertStatus(t, response, http.StatusOK)
		var state State
		decodeResponse(t, response, &state)
		if got := beatDuration(t, state, 1); got != step.wantMS {
			t.Errorf("%s: want beat 1 at %dms, got %dms", step.route, step.wantMS, got)
		}
	}
}

func TestHandler_SaveAfterOutsideWrite_Is409UntilReload(t *testing.T) {
	t.Parallel()
	fixture := newSessionFixture(t)
	handler := newHandler(fixture.session, testAddr)
	assertStatus(t, post(t, handler, "/api/edit", `{"beat": 2, "field": "duration", "value": "1800"}`), http.StatusOK)
	fixture.writeOutside(t)

	assertStatus(t, post(t, handler, "/api/save", `{}`), http.StatusConflict)

	assertStatus(t, post(t, handler, "/api/reload", `{}`), http.StatusOK)
	assertStatus(t, post(t, handler, "/api/edit", `{"beat": 2, "field": "duration", "value": "1800"}`), http.StatusOK)
	assertStatus(t, post(t, handler, "/api/save", `{}`), http.StatusOK)
	saved := fixture.readDisk(t)
	if !strings.Contains(saved, beatOneNew) || !strings.Contains(saved, "duration: 1800ms") {
		t.Errorf("saved storyboard: want the outside write and the edit, got\n%s", saved)
	}
}

func TestHandler_FrameAndEdit_RunConcurrently(t *testing.T) {
	t.Parallel()
	handler := newHandler(newSessionFixture(t).session, testAddr)
	const rounds = 10

	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		durations := []string{"1200", "1000"}
		for index := range rounds {
			body := `{"beat": 1, "field": "duration", "value": "` + durations[index%2] + `"}`
			if response := post(t, handler, "/api/edit", body); response.Code != http.StatusOK {
				t.Errorf("edit: status %d: %s", response.Code, response.Body.String())
			}
		}
	}()
	go func() {
		defer wait.Done()
		for range rounds {
			if response := get(t, handler, "/api/frame?t=500"); response.Code != http.StatusOK {
				t.Errorf("frame: status %d: %s", response.Code, response.Body.String())
			}
		}
	}()
	wait.Wait()
}

func TestHandler_Host_OnlyTheListenAddressIsAnswered(t *testing.T) {
	t.Parallel()
	handler := newHandler(newSessionFixture(t).session, testAddr)
	tests := []struct {
		name   string
		host   string
		status int
	}{
		{name: "the listen address", host: testAddr, status: http.StatusOK},
		{name: "localhost on its port", host: "localhost:4711", status: http.StatusOK},
		{name: "a foreign name", host: "attacker.example:4711", status: http.StatusForbidden},
		{name: "another port", host: "127.0.0.1:80", status: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			response := serveRequest(t, handler, test.host, http.MethodGet, "/api/state", "", "")

			assertStatus(t, response, test.status)
		})
	}
}

func TestHandler_PostNotJSON_Is415(t *testing.T) {
	t.Parallel()
	fixture := newSessionFixture(t)
	handler := newHandler(fixture.session, testAddr)

	response := serveRequest(t, handler, testAddr, http.MethodPost, "/api/edit",
		"application/x-www-form-urlencoded", "beat=1&field=duration&value=2000")

	assertStatus(t, response, http.StatusUnsupportedMediaType)
	if got := beatDuration(t, fixture.session.State(), 1); got != beatOneMS {
		t.Errorf("a refused POST changed beat 1 to %dms", got)
	}
}

func TestHandler_Pick_ReturnsTheProposal(t *testing.T) {
	t.Parallel()
	session := newPickSession(t)
	handler := newHandler(session, testAddr)
	body := pickBody(t, 0, cellPixel(t, session, 0, 5), cellPixel(t, session, 5, 5))

	response := post(t, handler, "/api/pick", body)

	assertStatus(t, response, http.StatusOK)
	var proposal pickResponse
	decodeResponse(t, response, &proposal)
	if proposal.Target != (pickTarget{Text: "unique"}) {
		t.Errorf("target: want unique, got %+v", proposal.Target)
	}
	if proposal.Box.X != 0 || proposal.Box.Y != 5 || proposal.Box.W < 6 || proposal.Box.H != 1 {
		t.Errorf("box: want columns 0–5 of row 5 covered, got %+v", proposal.Box)
	}
	frameSize := session.goodPipeline().frames.Size()
	rect := image.Rect(proposal.Rect.X, proposal.Rect.Y, proposal.Rect.X+proposal.Rect.W, proposal.Rect.Y+proposal.Rect.H)
	if rect.Empty() || !rect.In(image.Rectangle{Max: frameSize}) {
		t.Errorf("rect: want a non-empty rectangle inside the %v frame, got %v", frameSize, rect)
	}
	for _, key := range []string{snippetZoom, snippetClick, snippetSeen, snippetDragFrom, snippetDragTo} {
		if proposal.Snippets[key] == "" {
			t.Errorf("snippet %q missing from %v", key, proposal.Snippets)
		}
	}
}

func TestHandler_PickOnBlanks_Is422(t *testing.T) {
	t.Parallel()
	session := newPickSession(t)
	handler := newHandler(session, testAddr)
	body := pickBody(t, 0, cellPixel(t, session, 20, 0), cellPixel(t, session, 25, 0))

	response := post(t, handler, "/api/pick", body)

	assertStatus(t, response, http.StatusUnprocessableEntity)
	var failure apiError
	decodeResponse(t, response, &failure)
	if failure.Error == "" {
		t.Error("the refusal carries no message")
	}
}

func TestRun_PortZero_ListensOnLoopbackAndStopsOnCancel(t *testing.T) {
	t.Parallel()
	fixture := newSessionFixture(t)
	loaded, err := clip.Load(fixture.boardPath, "", t.TempDir())
	if err != nil {
		t.Fatalf("clip.Load: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stdoutReader, stdoutWriter := io.Pipe()
	stopped := make(chan error, 1)

	go func() {
		err := Run(ctx, loaded, Options{Take: fixture.takePath}, stdoutWriter, io.Discard)
		// Closing the pipe ends the read below should Run fail before printing.
		stdoutWriter.CloseWithError(err) //nolint:errcheck,gosec // a pipe writer's close always returns nil
		stopped <- err
	}()
	line, err := bufio.NewReader(stdoutReader).ReadString('\n')
	if err != nil {
		t.Fatalf("read the printed address: %v", err)
	}
	address := listenAddressOf(t, line)
	response, err := http.Get("http://" + address + "/api/state")
	if err != nil {
		t.Fatalf("GET the state: %v", err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close the response: %v", err)
	}
	cancel()

	if response.StatusCode != http.StatusOK {
		t.Errorf("GET the state: want 200, got %d", response.StatusCode)
	}
	select {
	case err := <-stopped:
		if err != nil {
			t.Errorf("Run: want nil once cancelled, got %v", err)
		}
	case <-time.After(runStopTimeout):
		t.Fatal("Run did not return after its context ended")
	}
}

// listenAddressOf returns the host:port of the URL line Run printed, failing the test unless the
// line is the URL line on 127.0.0.1 at a port other than 0.
func listenAddressOf(t *testing.T, line string) string {
	t.Helper()
	rawURL, ok := strings.CutPrefix(strings.TrimSuffix(line, "\n"), printedPrefix)
	if !ok {
		t.Fatalf("printed line %q: want the %q prefix", line, printedPrefix)
	}
	printed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse the printed URL %q: %v", rawURL, err)
	}
	host, port, err := net.SplitHostPort(printed.Host)
	if err != nil {
		t.Fatalf("split the printed address %q: %v", printed.Host, err)
	}
	if host != listenHost || port == "0" || printed.Path != "/" {
		t.Fatalf("printed URL %q: want http://%s:<port>/ on a chosen port", rawURL, listenHost)
	}
	return printed.Host
}

// pickBody is the JSON body of a pick at ms from from to to.
func pickBody(t *testing.T, ms int64, from, to image.Point) string {
	t.Helper()
	encoded, err := json.Marshal(pickRequest{
		TimeMS: ms,
		From:   pixelPoint{X: from.X, Y: from.Y},
		To:     pixelPoint{X: to.X, Y: to.Y},
	})
	if err != nil {
		t.Fatalf("encode the pick: %v", err)
	}
	return string(encoded)
}
