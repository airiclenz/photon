package tune

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/render"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

// testAddr is the listen address the handler tests build the handler for.
const testAddr = "127.0.0.1:4711"

// printedPrefix starts the line Run prints, before the page's URL.
const printedPrefix = "photon tune: "

// runStopTimeout bounds how long Run may take to return once its context ends.
const runStopTimeout = 10 * time.Second

// testBuild is the build string the tests hand a run or a session to report.
const testBuild = "photon 9.8.7 (a1b2c3d, dirty)"

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

func TestHandler_Frame_Draft_IsADraftOfTheExactFramesSize(t *testing.T) {
	t.Parallel()
	fixture := newSessionFixture(t)
	handler := newHandler(fixture.session, testAddr)
	exact := frameOf(t, handler, "500")

	draft := frameOf(t, handler, "500&draft=1")

	assertSameSize(t, draft, exact)
	if bytes.Equal(decodedFrame(t, draft).Pix, decodedFrame(t, exact).Pix) {
		t.Error("the served draft frame shows the exact frame's pixels on a scale-2 clip")
	}
}

func TestHandler_Frame_Draft_IsAnUncompressedPNG(t *testing.T) {
	t.Parallel()
	fixture := newSessionFixture(t)
	handler := newHandler(fixture.session, testAddr)
	exact := frameOf(t, handler, "500")

	draft := frameOf(t, handler, "500&draft=1")

	assertSameSize(t, draft, exact)
	// Go writes the same zlib level bits for no compression and for best speed, so the first
	// deflate block's type tells them apart: 00 is a stored, uncompressed block.
	stream := idatData(t, draft)
	if len(stream) < zlibHeaderBytes+1 {
		t.Fatalf("draft IDAT data: %d bytes, want a zlib header and a deflate block", len(stream))
	}
	if blockType := (stream[zlibHeaderBytes] >> 1) & 0b11; blockType != 0 {
		t.Errorf("draft's first deflate block type = %02b, want 00 (stored)", blockType)
	}
	if len(exact) >= len(draft) {
		t.Errorf("exact frame %d bytes, want fewer than its draft's %d", len(exact), len(draft))
	}
}

// zlibHeaderBytes is the length of the zlib header before a PNG's deflate stream.
const zlibHeaderBytes = 2

// idatData returns the PNG encoded's image data: its IDAT chunks' data joined in order.
func idatData(t *testing.T, encoded []byte) []byte {
	t.Helper()
	const signatureBytes, lengthBytes, typeBytes, crcBytes = 8, 4, 4, 4
	var data []byte
	rest := encoded[signatureBytes:]
	for len(rest) >= lengthBytes+typeBytes+crcBytes {
		length := int(binary.BigEndian.Uint32(rest))
		chunkType := string(rest[lengthBytes : lengthBytes+typeBytes])
		body := rest[lengthBytes+typeBytes:]
		if len(body) < length+crcBytes {
			t.Fatalf("PNG chunk %s: %d bytes, want %d", chunkType, len(body), length+crcBytes)
		}
		if chunkType == "IDAT" {
			data = append(data, body[:length]...)
		}
		rest = body[length+crcBytes:]
	}
	return data
}

func TestHandler_Frame_DraftOff_IsTheExactFrame(t *testing.T) {
	t.Parallel()
	fixture := newSessionFixture(t)
	handler := newHandler(fixture.session, testAddr)

	frame := frameOf(t, handler, "500&draft=0")

	want, err := fixture.session.Frame(500 * time.Millisecond)
	if err != nil {
		t.Fatalf("Frame: %v", err)
	}
	if !bytes.Equal(frame, want) {
		t.Error("the frame served with draft=0 is not the session's exact frame")
	}
}

func TestHandler_Frame_BadDraft_IsRefused(t *testing.T) {
	t.Parallel()
	handler := newHandler(newSessionFixture(t).session, testAddr)

	response := get(t, handler, "/api/frame?t=500&draft=x")

	assertStatus(t, response, http.StatusBadRequest)
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

	response := post(t, handler, "/api/edit", `{"owner": {"kind": "beat", "index": 1}, "field": "duration", "value": 2000}`)

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

func TestHandler_EditsAsThePageSendsThem_AreApplied(t *testing.T) {
	t.Parallel()
	handler := newHandler(newPickSession(t), testAddr)
	drag := `{"owner":{"kind":"beat","index":1},"field":"duration","value":1500}`

	dragged := post(t, handler, "/api/edit", drag)

	assertStatus(t, dragged, http.StatusOK)
	var state State
	decodeResponse(t, dragged, &state)
	if got := beatDuration(t, state, 1); got != 1500 {
		t.Errorf("beat 1 after the drag: want 1500ms, got %dms", got)
	}
}

func TestHandler_CameraEditsAsThePageSendsThem_AreApplied(t *testing.T) {
	t.Parallel()
	handler := newHandler(newSessionFixtureOf(t, cameraStoryboard).session, testAddr)
	edits := []string{
		`{"owner":{"kind":"position","index":1},"field":"position.hold","value":600}`,
		`{"owner":{"kind":"zoom","index":0},"field":"zoom.to","value":2.5}`,
	}

	var state State
	for _, edit := range edits {
		response := post(t, handler, "/api/edit", edit)
		assertStatus(t, response, http.StatusOK)
		decodeResponse(t, response, &state)
	}

	if got := state.Positions[1]; got.HoldMS != 600 || got.EndMS != 800 {
		t.Errorf("position 1 after the edit: want a 600ms hold ending at 800ms, got %+v", got)
	}
	if got := state.Zooms[0].Factor; got != 2.5 {
		t.Errorf("zoom 0 after the edit: want factor 2.5, got %g", got)
	}
}

func TestHandler_PositionEditUndoneAndSaved_ChangesOnlyTheEditedValue(t *testing.T) {
	t.Parallel()
	f := newSessionFixtureOf(t, cameraStoryboard)
	handler := newHandler(f.session, testAddr)
	steps := []struct{ route, body string }{
		{"/api/edit", `{"owner":{"kind":"position","index":1},"field":"position.hold","value":600}`},
		{"/api/edit", `{"owner":{"kind":"zoom","index":0},"field":"zoom.to","value":2.5}`},
		{"/api/undo", `{}`},
		{"/api/save", `{}`},
	}

	for _, step := range steps {
		assertStatus(t, post(t, handler, step.route, step.body), http.StatusOK)
	}

	want := strings.Replace(cameraStoryboard, "hold: 300ms}", "hold: 600ms}", 1)
	if got := f.readDisk(t); got != want {
		t.Errorf("saved:\n%s\nwant:\n%s", got, want)
	}
}

func TestHandler_RefusedEdit_Is422(t *testing.T) {
	t.Parallel()
	handler := newHandler(newSessionFixture(t).session, testAddr)

	response := post(t, handler, "/api/edit", `{"owner": {"kind": "beat", "index": 9}, "field": "duration", "value": 2000}`)

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
	assertStatus(t, post(t, handler, "/api/edit", `{"owner": {"kind": "beat", "index": 1}, "field": "duration", "value": 2000}`), http.StatusOK)
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
	assertStatus(t, post(t, handler, "/api/edit", `{"owner": {"kind": "beat", "index": 2}, "field": "duration", "value": 1800}`), http.StatusOK)
	fixture.writeOutside(t)

	assertStatus(t, post(t, handler, "/api/save", `{}`), http.StatusConflict)

	assertStatus(t, post(t, handler, "/api/reload", `{}`), http.StatusOK)
	assertStatus(t, post(t, handler, "/api/edit", `{"owner": {"kind": "beat", "index": 2}, "field": "duration", "value": 1800}`), http.StatusOK)
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
			body := `{"owner": {"kind": "beat", "index": 1}, "field": "duration", "value": ` + durations[index%2] + `}`
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
	if proposal.Target != (storyboard.Target{Text: "unique"}) {
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
		err := Run(ctx, loaded, Options{Take: fixture.takePath, Build: testBuild}, stdoutWriter, io.Discard)
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
	var state State
	decodeErr := json.NewDecoder(response.Body).Decode(&state)
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close the response: %v", err)
	}
	cancel()

	if response.StatusCode != http.StatusOK {
		t.Errorf("GET the state: want 200, got %d", response.StatusCode)
	}
	if decodeErr != nil {
		t.Errorf("decode the state: %v", decodeErr)
	} else if state.Build != testBuild {
		t.Errorf("the state's build: want %q, got %q", testBuild, state.Build)
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

// The draft frame benchmarks' take: the 135×44 terminal the preview's playback speed was measured
// on, one distinct snapshot every draftBenchStep. Drawn at the shipped frame's size, a draft
// raster holds about 3.7 MiB, so the 256 MiB draft raster cache keeps about 69 of them; the take
// holds draftBenchCacheMultiple times as many snapshots as the cache keeps, the count measured
// from the draft raster itself (draftBenchSnapshots), so requests cycling through them miss the
// cache every time.
const (
	draftBenchCols = 135
	draftBenchRows = 44
	draftBenchStep = 100 * time.Millisecond
	// draftBenchCacheMultiple is how many times the draft cache's capacity the take's snapshots
	// fill when rasterized.
	draftBenchCacheMultiple = 2
	// draftBenchInFlight is how many draft frames a playing preview keeps requested at once.
	draftBenchInFlight = 4
)

// draftBenchStoryboard frames a benchmark take of snapshots requested snapshots as the measured
// clip: font_size 30, scale 2, width 1250. Its one beat lasts as long as those snapshots play, so
// it plays 1× and output time is take time.
func draftBenchStoryboard(snapshots int) string {
	return fmt.Sprintf(`clip: tuned
ship: out/tuned.gif
frame: {cols: 135, rows: 44, padding: 4, font_size: 30, line_height: 1.2, scale: 2, width: 1250, fps: 24, max_colors: 256}
beats:
  - id: 1
    title: playing
    duration: %dms
`, (time.Duration(snapshots) * draftBenchStep).Milliseconds())
}

// BenchmarkHandler_DraftFrame measures GET /api/frame?draft=1 on the cold path a playing preview
// pays: each request lands on a snapshot the draft raster cache no longer holds, since the
// requests cycle through more snapshots than it keeps.
func BenchmarkHandler_DraftFrame(b *testing.B) {
	handler, snapshots := newDraftBenchHandler(b)
	serveDraftFrame(b, handler, 0)
	b.ResetTimer()

	for i := range b.N {
		serveDraftFrame(b, handler, (i+1)%snapshots)
	}
}

// BenchmarkHandler_DraftFramePlaying measures cold draft frames served the way a playing preview
// asks for them: draftBenchInFlight requests at once, each on a snapshot the draft raster cache
// no longer holds. It reports the frames served per second.
func BenchmarkHandler_DraftFramePlaying(b *testing.B) {
	handler, snapshots := newDraftBenchHandler(b)
	serveDraftFrame(b, handler, 0)
	var next atomic.Int64
	var group sync.WaitGroup
	b.ResetTimer()

	for range draftBenchInFlight {
		group.Go(func() {
			for {
				i := next.Add(1)
				if i > int64(b.N) {
					return
				}
				if err := requestDraftFrame(handler, int(i)%snapshots); err != nil {
					b.Error(err)
					return
				}
			}
		})
	}
	group.Wait()

	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "frames/s")
}

// newDraftBenchHandler serves a session on a benchmark take of draftBenchSnapshots distinct
// snapshots, and returns it with that count.
func newDraftBenchHandler(b *testing.B) (http.Handler, int) {
	b.Helper()
	snapshots := draftBenchSnapshots(b)
	loaded, takePath, _ := writeDraftBench(b, snapshots)
	session, err := NewSession(loaded, takePath)
	if err != nil {
		b.Fatalf("NewSession: %v", err)
	}
	return newHandler(session, testAddr), snapshots
}

// draftBenchSnapshots is how many distinct snapshots the benchmark take needs to fill
// draftBenchCacheMultiple times the draft raster cache's capacity: one snapshot is drawn into a
// probe draft cache and its raster's bytes measured.
func draftBenchSnapshots(b *testing.B) int {
	b.Helper()
	loaded, takePath, recorded := writeDraftBench(b, 1)
	probe := newRasterCache(draftCacheBytes)
	pipeline, err := render.Build(loaded, takePath, recorded, render.BuildOptions{
		Cache:      newRasterCache(rasterCacheBytes),
		DraftCache: probe,
	})
	if err != nil {
		b.Fatalf("render.Build: %v", err)
	}
	if _, err := pipeline.DraftAt(draftBenchStep / 2); err != nil {
		b.Fatalf("DraftAt: %v", err)
	}
	if probe.used == 0 {
		b.Fatal("the draft raster was not kept in the draft cache")
	}
	return (draftBenchCacheMultiple*draftCacheBytes + probe.used - 1) / probe.used
}

// writeDraftBench writes the config, draftBenchStoryboard and a benchmark take of snapshots
// requested snapshots into a fresh project dir, and returns the loaded clip, the take's path and
// the take.
func writeDraftBench(b *testing.B, snapshots int) (*clip.Clip, string, *take.Take) {
	b.Helper()
	project := b.TempDir()
	boardPath := filepath.Join(project, "tuned.yaml")
	takePath := filepath.Join(project, "tuned.take")
	for path, content := range map[string]string{
		filepath.Join(project, "photon.yaml"): sessionConfig,
		boardPath:                             draftBenchStoryboard(snapshots),
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			b.Fatalf("write %s: %v", path, err)
		}
	}
	recorded := draftBenchTake(snapshots)
	if err := take.Save(takePath, recorded); err != nil {
		b.Fatalf("take.Save: %v", err)
	}
	loaded, err := clip.Load(boardPath, "", b.TempDir())
	if err != nil {
		b.Fatalf("clip.Load: %v", err)
	}
	return loaded, takePath, recorded
}

// draftBenchTake is a take of snapshots requested full screens, each row of each one numbered so
// that no two snapshots rasterize alike, and one more screen at the take's end so that the last
// requested snapshot plays for its whole step.
func draftBenchTake(snapshots int) *take.Take {
	filler := strings.Repeat("the quick brown fox jumps over the lazy dog ", draftBenchCols)
	screens := make([]take.Snapshot, 0, snapshots+1)
	for index := range snapshots + 1 {
		lines := make([]string, 0, draftBenchRows)
		for row := range draftBenchRows {
			prefix := fmt.Sprintf("snapshot %03d row %02d ", index, row)
			lines = append(lines, (prefix + filler)[:draftBenchCols])
		}
		screens = append(screens, lineSnapshot(time.Duration(index)*draftBenchStep, lines...))
	}
	return &take.Take{
		Cols: draftBenchCols, Rows: draftBenchRows, FPS: 10,
		Snapshots: screens,
		Events:    []take.Event{{At: 0, Kind: take.EventBeatStart, Beat: 1}},
	}
}

// serveDraftFrame requests the draft frame halfway through snapshot index and fails the benchmark
// unless it is a PNG.
func serveDraftFrame(b *testing.B, handler http.Handler, index int) {
	b.Helper()
	if err := requestDraftFrame(handler, index); err != nil {
		b.Fatal(err)
	}
}

// requestDraftFrame requests the draft frame halfway through snapshot index and reports an error
// unless it is a PNG. It is safe to call from several goroutines at once.
func requestDraftFrame(handler http.Handler, index int) error {
	out := time.Duration(index)*draftBenchStep + draftBenchStep/2
	target := fmt.Sprintf("/api/frame?t=%d&draft=1", out.Milliseconds())
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Host = testAddr
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Header().Get(headerContentType) != mediaPNG {
		return fmt.Errorf("GET %s: status %d, type %q: %s", target, recorder.Code,
			recorder.Header().Get(headerContentType), recorder.Body.String())
	}
	return nil
}
