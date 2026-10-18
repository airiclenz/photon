package tune

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/dop251/goja"

	"github.com/airiclenz/photon/internal/storyboard"
)

// domStubPath is the minimal DOM the page's scripts run over under goja.
var domStubPath = filepath.Join("testdata", "dom.js")

// maxSettleRounds bounds how many rounds of queued tasks and an animation frame settle runs
// before it calls the page busy for ever.
const maxSettleRounds = 100

// The patterns that read index.html: the scripts it loads, in order, and its elements by id.
var (
	indexScriptPattern  = regexp.MustCompile(`<script src="` + assetsPrefix + `([^"]+)"`)
	indexElementPattern = regexp.MustCompile(`<(\w+)\b[^>]*\bid="([^"]+)"`)
)

// The page's elements the tests drive and read, by id.
const (
	elementFrame      = "frame"
	elementStage      = "stage"
	elementPick       = "pick"
	elementPickPan    = "pick-pan"
	elementPickStatus = "pick-status"
	elementBeatSelect = "beat-select"
	elementTime       = "time"
	elementDocument   = "document"
)

// panToTarget is the index of cameraCaptionsStoryboard's pan to a target.
const panToTarget = 2

// pageRequest is one request the page sent through fetch and the handler's answer.
type pageRequest struct {
	method string
	path   string
	body   string
	status int
	answer string
}

// pageHarness runs the tune page's scripts, in index.html's order, under goja over the DOM stub,
// its fetch answered by a tune handler.
type pageHarness struct {
	t        *testing.T
	vm       *goja.Runtime
	dom      *goja.Object
	handler  http.Handler
	requests []pageRequest
	logged   []string
	rejected map[*goja.Promise]bool
}

// loadPage loads the page over session's handler, fires DOMContentLoaded and settles it, failing
// the test when a script throws, logs an error or leaves a rejection unhandled.
func loadPage(t *testing.T, session *Session) *pageHarness {
	t.Helper()
	page := &pageHarness{
		t:        t,
		vm:       goja.New(),
		handler:  newHandler(session, testAddr),
		rejected: map[*goja.Promise]bool{},
	}
	page.vm.SetPromiseRejectionTracker(page.trackRejection)
	page.set("testHost", map[string]any{"serve": page.serve})
	page.set("console", map[string]any{
		"log":   page.consoleLog,
		"info":  page.consoleLog,
		"debug": page.consoleLog,
		"warn":  page.consoleError,
		"error": page.consoleError,
	})
	stub, err := os.ReadFile(domStubPath)
	if err != nil {
		t.Fatalf("read the DOM stub: %v", err)
	}
	page.runScript(domStubPath, string(stub))
	page.dom = page.vm.Get("testDOM").ToObject(page.vm)

	index := string(readPageFile(t, pageIndex))
	for _, match := range indexElementPattern.FindAllStringSubmatch(index, -1) {
		page.call("declare", match[2], match[1])
	}
	scripts := indexScriptPattern.FindAllStringSubmatch(index, -1)
	if len(scripts) == 0 {
		t.Fatal("index.html loads no script")
	}
	for _, script := range scripts {
		page.runScript(script[1], string(readPageFile(t, script[1])))
	}
	page.dispatch(elementDocument, "DOMContentLoaded", nil)
	page.settle()
	return page
}

func (p *pageHarness) set(name string, value any) {
	p.t.Helper()
	if err := p.vm.Set(name, value); err != nil {
		p.t.Fatalf("set %s: %v", name, err)
	}
}

func (p *pageHarness) runScript(name, source string) {
	p.t.Helper()
	if _, err := p.vm.RunScript(name, source); err != nil {
		p.t.Fatalf("run %s: %v", name, err)
	}
}

// call calls testDOM's method name with args and returns its result.
func (p *pageHarness) call(name string, args ...any) goja.Value {
	p.t.Helper()
	method, ok := goja.AssertFunction(p.dom.Get(name))
	if !ok {
		p.t.Fatalf("testDOM has no method %s", name)
	}
	values := make([]goja.Value, 0, len(args))
	for _, arg := range args {
		values = append(values, p.vm.ToValue(arg))
	}
	result, err := method(p.dom, values...)
	if err != nil {
		p.t.Fatalf("testDOM.%s: %v", name, err)
	}
	return result
}

// eval runs expression in the page and returns its value.
func (p *pageHarness) eval(expression string) goja.Value {
	p.t.Helper()
	value, err := p.vm.RunString(expression)
	if err != nil {
		p.t.Fatalf("eval %s: %v", expression, err)
	}
	return value
}

// dispatch fires an event of type on the element of id, init holding the event's own fields.
func (p *pageHarness) dispatch(id, eventType string, init map[string]any) {
	p.t.Helper()
	p.call("dispatch", id, eventType, init)
}

// settle runs the queued tasks, each followed by its promise jobs, and then one animation frame,
// until no task is left; it fails the test on a logged error or an unhandled rejection.
func (p *pageHarness) settle() {
	p.t.Helper()
	for range maxSettleRounds {
		p.runTasks()
		p.call("runFrame")
		if !p.call("hasTasks").ToBoolean() {
			p.assertQuiet()
			return
		}
	}
	p.t.Fatalf("the page still has tasks queued after %d rounds", maxSettleRounds)
}

// runTasks runs the queued tasks one call at a time, so each one's promise jobs run before the
// next, as a browser runs its microtasks.
func (p *pageHarness) runTasks() {
	p.t.Helper()
	isTaskRun := true
	for isTaskRun {
		isTaskRun = p.call("runTask").ToBoolean()
	}
}

func (p *pageHarness) assertQuiet() {
	p.t.Helper()
	if len(p.logged) > 0 {
		p.t.Fatalf("the page logged errors: %q", p.logged)
	}
	for promise := range p.rejected {
		p.t.Fatalf("the page left a rejection unhandled: %v", promise.Result())
	}
}

// serve answers one fetch through the handler: the status, the body as text, and an image's
// size when the answer is a PNG.
func (p *pageHarness) serve(method, path, contentType, body string) map[string]any {
	response := serveRequest(p.t, p.handler, testAddr, method, path, contentType, body)
	answer := map[string]any{
		"status":      response.Code,
		"statusText":  http.StatusText(response.Code),
		"contentType": response.Header().Get(headerContentType),
		"size":        response.Body.Len(),
		"body":        "",
		"imageWidth":  0,
		"imageHeight": 0,
	}
	recorded := pageRequest{method: method, path: path, body: body, status: response.Code}
	if answer["contentType"] == mediaPNG {
		if config, err := png.DecodeConfig(bytes.NewReader(response.Body.Bytes())); err == nil {
			answer["imageWidth"], answer["imageHeight"] = config.Width, config.Height
		}
	} else {
		answer["body"] = response.Body.String()
		recorded.answer = response.Body.String()
	}
	p.requests = append(p.requests, recorded)
	return answer
}

func (p *pageHarness) consoleLog(call goja.FunctionCall) goja.Value {
	p.t.Logf("page: %s", joinArgs(call))
	return goja.Undefined()
}

func (p *pageHarness) consoleError(call goja.FunctionCall) goja.Value {
	p.logged = append(p.logged, joinArgs(call))
	return goja.Undefined()
}

func joinArgs(call goja.FunctionCall) string {
	parts := make([]string, 0, len(call.Arguments))
	for _, argument := range call.Arguments {
		parts = append(parts, argument.String())
	}
	return strings.Join(parts, " ")
}

func (p *pageHarness) trackRejection(promise *goja.Promise, operation goja.PromiseRejectionOperation) {
	if operation == goja.PromiseRejectionReject {
		p.rejected[promise] = true
	} else {
		delete(p.rejected, promise)
	}
}

// fitFrame lays the preview <img> out at its natural size, so displayed pixels are output ones.
func (p *pageHarness) fitFrame() {
	p.t.Helper()
	p.eval(fmt.Sprintf(`{
		const frame = document.getElementById(%q);
		frame.layout = { left: 0, top: 0, width: frame.naturalWidth, height: frame.naturalHeight };
	}`, elementFrame))
}

// selectValue chooses value in the select of id and fires its change.
func (p *pageHarness) selectValue(id, value string) {
	p.t.Helper()
	p.eval(fmt.Sprintf(`document.getElementById(%q).value = %q;`, id, value))
	p.dispatch(id, "change", nil)
}

// text is the text content of the element of id.
func (p *pageHarness) text(id string) string {
	p.t.Helper()
	return p.eval(fmt.Sprintf(`document.getElementById(%q).textContent`, id)).String()
}

// isHidden says the element of id is hidden.
func (p *pageHarness) isHidden(id string) bool {
	p.t.Helper()
	return p.eval(fmt.Sprintf(`document.getElementById(%q).hidden`, id)).ToBoolean()
}

// lastRequest is the last request the page sent for path, failing the test when there is none.
func (p *pageHarness) lastRequest(path string) pageRequest {
	p.t.Helper()
	for i := len(p.requests) - 1; i >= 0; i-- {
		if p.requests[i].path == path {
			return p.requests[i]
		}
	}
	p.t.Fatalf("the page sent no request for %s", path)
	return pageRequest{}
}

// pointerAt is a primary-button pointer event's fields at point, in displayed pixels.
func pointerAt(point image.Point) map[string]any {
	return map[string]any{"button": 0, "pointerId": 1, "clientX": point.X, "clientY": point.Y}
}

// decodeJSON decodes text into a generic value, failing the test when it is not JSON.
func decodeJSON(t *testing.T, text string) any {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		t.Fatalf("decode %s: %v", text, err)
	}
	return value
}

func TestPageJS_LoadsAndWires(t *testing.T) {
	t.Parallel()
	page := loadPage(t, newSessionFixtureOf(t, cameraCaptionsStoryboard).session)

	state := page.lastRequest("/api/state")
	frame := page.lastRequest("/api/frame?t=0")

	if state.method != http.MethodGet || state.status != http.StatusOK {
		t.Errorf("the state request: want GET answered 200, got %s answered %d", state.method, state.status)
	}
	if frame.status != http.StatusOK {
		t.Errorf("the frame request: want 200, got %d", frame.status)
	}
	if got := page.eval("window.tune.frameTime()").Export(); got != int64(0) {
		t.Errorf("the frame shown: want the one at 0ms, got %v", got)
	}
	if got := page.text(elementTime); !strings.HasPrefix(got, "0:00.000 / ") {
		t.Errorf("the transport's time: want it from 0:00.000, got %q", got)
	}
	if page.isHidden(elementPickPan) {
		t.Error("the pan target button is hidden on a storyboard with pan moves")
	}
}

func TestPageJS_SetAsPanTarget_SendsTheProposal(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, cameraCaptionsStoryboard).session
	if move := session.State().Camera[panToTarget]; move.Kind != "pan" || move.Target == nil {
		t.Fatalf("camera %d: want a pan to a target, got %+v", panToTarget, move)
	}
	page := loadPage(t, session)
	page.fitFrame()
	from, to := cellPixel(t, session, 2, 0), cellPixel(t, session, 4, 0)

	page.dispatch(elementPick, "click", nil)
	page.dispatch(elementStage, "pointerdown", pointerAt(from))
	page.dispatch(elementStage, "pointerup", pointerAt(to))
	page.settle()
	page.selectValue(elementBeatSelect, entryValueOf(storyboard.OwnerCamera, panToTarget))
	page.dispatch(elementPickPan, "click", nil)
	page.settle()

	picked := page.lastRequest("/api/pick")
	if picked.status != http.StatusOK {
		t.Fatalf("the pick: want 200, got %d: %s", picked.status, picked.answer)
	}
	var proposal struct {
		Target json.RawMessage `json:"target"`
	}
	if err := json.Unmarshal([]byte(picked.answer), &proposal); err != nil {
		t.Fatalf("decode the pick's answer %s: %v", picked.answer, err)
	}
	edit := page.lastRequest("/api/edit")
	want := decodeJSON(t, fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":"camera.to","value":%s}`,
		storyboard.OwnerCamera, panToTarget, proposal.Target))
	if got := decodeJSON(t, edit.body); !reflect.DeepEqual(got, want) {
		t.Errorf("the edit sent: want %v, got %v", want, got)
	}
	if edit.status != http.StatusOK {
		t.Errorf("the edit: want 200, got %d: %s", edit.status, edit.answer)
	}
	wantReport := fmt.Sprintf("Pan target set on camera %d", panToTarget)
	if got := page.text(elementPickStatus); got != wantReport {
		t.Errorf("the pick status: want %q, got %q", wantReport, got)
	}
}

// entryValueOf spells a camera move's or caption's entry as the beat select's value for it.
func entryValueOf(kind storyboard.OwnerKind, index int) string {
	return fmt.Sprintf("%s:%d", kind, index)
}
