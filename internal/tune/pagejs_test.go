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
	"time"

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
	elementInspector  = "inspector-body"
	elementTimeline   = "timeline"
	elementTime       = "time"
	elementStatus     = "status"
	elementDocument   = "document"
)

// panToTarget is the index of cameraCaptionsStoryboard's pan to a target.
const panToTarget = 2

// The entries of cameraCaptionsStoryboard the list tests move and add around: its via pan, the
// camera move after it, and the caption index a caption added at addCaptionAtMS becomes, after
// the caption at 100ms and before the one at 1s.
const (
	viaPan         = 1
	afterViaPan    = 2
	addCaptionAtMS = 500
	addedCaption   = 1
)

// styledCaption is the caption of cameraCaptionsStoryboard the style tests edit: set by place
// and align, and laid out in a box.
const styledCaption = 0

// frameAPIPrefix starts the path of every preview frame request.
const frameAPIPrefix = "/api/frame?"

// roomyCameraStoryboard is sessionStoryboard without beat 2's zoom, its camera a zoom early in
// beat 1 and a pan late in beat 2: a 600ms zoom or pan added at addMoveAtMS overlaps neither.
const roomyCameraStoryboard = `clip: tuned
ship: out/tuned.gif
frame: {cols: 20, rows: 4, padding: 4, font_size: 10, line_height: 1.2, scale: 2, width: 160, fps: 10, max_colors: 16}
beats:
  - id: 1
    title: typing
    duration: 1s
  - id: 2
    title: result
    do:
      - pause: {for: 400ms}
    duration: 1500ms
camera:
  - zoom: {to: 2, at: 100ms, over: 300ms}
  - pan: {to: {cell: {x: 3, y: 2}}, at: 2000ms, over: 100ms}
`

// The camera moves of roomyCameraStoryboard the add test adds and moves: the playhead it adds
// both at, the index the zoom added there becomes, after the zoom at 100ms, and the index of the
// pan added after it, before the pan at 2s.
const (
	addMoveAtMS = 500
	addedZoom   = 1
	addedPan    = 2
)

// The refused add of roomyCameraStoryboard: a zoom added at overlapZoomAtMS falls inside its zoom
// at 100ms, becoming camera move overlapZoom on the zoom track after it, and unrelatedBeat is the
// beat the inspector shows meanwhile.
const (
	overlapZoomAtMS = 200
	overlapZoom     = 1
	unrelatedBeat   = "2"
)

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

// click clicks the one enabled button under the element of within whose title, or else text, is
// name, and settles the page.
func (p *pageHarness) click(within, name string) {
	p.t.Helper()
	found := p.eval(fmt.Sprintf(`{
		const named = document.getElementById(%q).querySelectorAll("button")
			.filter((button) => (button.title || button.textContent) === %q);
		if (named.length === 1 && !named[0].disabled) {
			named[0].dispatchEvent(new Event("click"));
		}
		named.length === 1 && !named[0].disabled;
	}`, within, name)).ToBoolean()
	if !found {
		p.t.Fatalf("no one enabled %q button under #%s", name, within)
	}
	p.settle()
}

// isDisabled says the one button under the element of within whose text is name is disabled.
func (p *pageHarness) isDisabled(within, name string) bool {
	p.t.Helper()
	return p.eval(fmt.Sprintf(`document.getElementById(%q).querySelectorAll("button")
		.find((button) => button.textContent === %q).disabled`, within, name)).ToBoolean()
}

// setField sets property of the first input in the inspector's one field row labelled label to
// value, fires the input's change and settles the page.
func (p *pageHarness) setField(label, property string, value any) {
	p.t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		p.t.Fatalf("encode %v: %v", value, err)
	}
	found := p.eval(fmt.Sprintf(`{
		const rows = document.getElementById(%q).querySelectorAll("label")
			.filter((row) => row.children.length > 0 && row.children[0].textContent === %q);
		const input = rows.length === 1 ? rows[0].querySelectorAll("input")[0] : undefined;
		if (input && !input.disabled) {
			input[%q] = %s;
			input.dispatchEvent(new Event("change"));
		}
		Boolean(input && !input.disabled);
	}`, elementInspector, label, property, encoded)).ToBoolean()
	if !found {
		p.t.Fatalf("no one enabled input in a field row labelled %q under #%s", label, elementInspector)
	}
	p.settle()
}

// isRequestedAfter says the page sent a request whose path starts with prefix after its last
// request for path.
func (p *pageHarness) isRequestedAfter(path, prefix string) bool {
	p.t.Helper()
	last := -1
	for i, request := range p.requests {
		if request.path == path {
			last = i
		}
	}
	if last < 0 {
		return false
	}
	for _, request := range p.requests[last+1:] {
		if strings.HasPrefix(request.path, prefix) {
			return true
		}
	}
	return false
}

// selectedEntry is window.tune.selectedEntry() as JSON, "null" when no entry is selected.
func (p *pageHarness) selectedEntry() string {
	p.t.Helper()
	return p.eval("JSON.stringify(window.tune.selectedEntry())").String()
}

// assertEdit fails the test unless the page's last edit sent want, as JSON, and was applied.
func (p *pageHarness) assertEdit(want string) {
	p.t.Helper()
	edit := p.lastRequest("/api/edit")
	if got := decodeJSON(p.t, edit.body); !reflect.DeepEqual(got, decodeJSON(p.t, want)) {
		p.t.Errorf("the edit sent: want %s, got %s", want, edit.body)
	}
	if edit.status != http.StatusOK {
		p.t.Errorf("the edit: want 200, got %d: %s", edit.status, edit.answer)
	}
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

func TestPageJS_AddCaptionAtThePlayhead(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, cameraCaptionsStoryboard).session
	page := loadPage(t, session)
	page.eval(fmt.Sprintf("window.tune.seek(%d)", addCaptionAtMS))
	page.settle()

	page.click(elementTimeline, "Add a caption at the playhead")

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,`+
		`"value":{"kind":"caption","at":{"beat":0,"offset_ms":%d}}}`,
		storyboard.OwnerCaption, addedCaption, storyboard.FieldCaptionInsert, addCaptionAtMS))
	added := session.State().Captions[addedCaption]
	if added.StartMS != addCaptionAtMS {
		t.Errorf("the added caption: want it from %dms, got %+v", addCaptionAtMS, added)
	}
	want := fmt.Sprintf(`{"kind":%q,"index":%d}`, storyboard.OwnerCaption, addedCaption)
	if got := page.selectedEntry(); got != want {
		t.Errorf("the selection after the add: want %s, got %s", want, got)
	}
}

func TestPageJS_AddAZoomAndAPanAtThePlayhead_ThenMoveUp(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, roomyCameraStoryboard).session
	page := loadPage(t, session)
	page.eval(fmt.Sprintf("window.tune.seek(%d)", addMoveAtMS))
	page.settle()
	selected := func(index int) string {
		return fmt.Sprintf(`{"kind":%q,"index":%d}`, storyboard.OwnerCamera, index)
	}

	page.click(elementTimeline, "Add a zoom at the playhead")

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,`+
		`"value":{"kind":"zoom","at":{"beat":0,"offset_ms":%d}}}`,
		storyboard.OwnerCamera, addedZoom, storyboard.FieldCameraInsert, addMoveAtMS))
	if got := page.selectedEntry(); got != selected(addedZoom) {
		t.Errorf("the selection after the zoom add: want %s, got %s", selected(addedZoom), got)
	}

	page.click(elementTimeline, "Add a pan at the playhead")

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,`+
		`"value":{"kind":"pan","at":{"beat":0,"offset_ms":%d}}}`,
		storyboard.OwnerCamera, addedPan, storyboard.FieldCameraInsert, addMoveAtMS))
	if got := page.selectedEntry(); got != selected(addedPan) {
		t.Fatalf("the selection after the pan add: want %s, got %s", selected(addedPan), got)
	}

	page.click(elementInspector, "Move up")

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,"value":%d}`,
		storyboard.OwnerCamera, addedPan, storyboard.FieldCameraMove, addedZoom))
	moved := session.State().Camera[addedZoom]
	if moved.Kind != "pan" || moved.StartMS != addMoveAtMS {
		t.Errorf("the camera move after the move up: want the pan from %dms, got %+v",
			addMoveAtMS, moved)
	}
	if got := page.selectedEntry(); got != selected(addedZoom) {
		t.Errorf("the selection after the move up: want %s, got %s", selected(addedZoom), got)
	}
}

func TestPageJS_RefusedLaneAdd_ShowsInTheStatusUntilAnAcceptedEdit(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, roomyCameraStoryboard).session
	page := loadPage(t, session)
	page.selectValue(elementBeatSelect, unrelatedBeat)
	page.eval(fmt.Sprintf("window.tune.seek(%d)", overlapZoomAtMS))
	page.settle()

	page.click(elementTimeline, "Add a zoom at the playhead")

	refused := page.lastRequest("/api/edit")
	if refused.status == http.StatusOK {
		t.Fatalf("the overlapping zoom add: want it refused, got 200: %s", refused.answer)
	}
	status := page.text(elementStatus)
	for _, move := range []string{"camera[0]", fmt.Sprintf("camera[%d]", overlapZoom)} {
		if !strings.Contains(status, move) {
			t.Errorf("the status after the refused add: want it naming %s, got %q", move, status)
		}
	}
	if !page.eval(fmt.Sprintf(`document.getElementById(%q).classList.contains("failed")`,
		elementStatus)).ToBoolean() {
		t.Error("the status after the refused add is not styled as a failure")
	}

	page.click(elementTimeline, "Add a pan at the playhead")

	if got := page.lastRequest("/api/edit"); got.status != http.StatusOK {
		t.Fatalf("the pan add: want 200, got %d: %s", got.status, got.answer)
	}
	if got := page.text(elementStatus); got != "" {
		t.Errorf("the status after an accepted edit: want it cleared, got %q", got)
	}
}

// The duration drag of sessionStoryboard: beat 1's end is dragged through dragPassMS and let go
// at dragReleaseMS, on a beats lane laid out one pixel per millisecond, so the one edit sends
// draggedDurationMS, dragReleaseMS snapped to 50ms.
const (
	dragPassMS        = 600
	dragReleaseMS     = 1234
	draggedDurationMS = 1250
)

// dragBeatsHandle drags the first handle on the beats lane through each of the output times at,
// in turn, and lets go at the last; the lane's track is laid out one pixel per millisecond.
func (p *pageHarness) dragBeatsHandle(at ...int) {
	p.t.Helper()
	moves, err := json.Marshal(at)
	if err != nil {
		p.t.Fatalf("encode the drag's moves: %v", err)
	}
	found := p.eval(fmt.Sprintf(`{
		const lane = document.getElementById(%q).querySelectorAll(".lane")
			.find((row) => row.querySelectorAll(".lane-name")[0].textContent === "beats");
		const track = lane ? lane.querySelectorAll(".lane-track")[0] : null;
		const handle = track ? track.querySelectorAll(".handle")[0] : null;
		if (handle) {
			track.layout = { left: 0, top: 0, width: window.tune.state().length_ms, height: 20 };
			const moves = %s;
			const fire = (type, ms) => handle.dispatchEvent(new Event(type, { clientX: ms }));
			fire("pointerdown", 0);
			moves.forEach((ms) => fire("pointermove", ms));
			fire("pointerup", moves[moves.length - 1]);
		}
		Boolean(handle);
	}`, elementTimeline, moves)).ToBoolean()
	if !found {
		p.t.Fatal("the beats lane has no handle")
	}
	p.settle()
}

func TestPageJS_DragABeatsEnd_SendsOneSnappedDurationEdit(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, sessionStoryboard).session
	page := loadPage(t, session)

	page.dragBeatsHandle(dragPassMS, dragReleaseMS)

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":1},"field":%q,"value":%d}`,
		storyboard.OwnerBeat, storyboard.FieldDuration, draggedDurationMS))
	edits := 0
	for _, request := range page.requests {
		if request.path == "/api/edit" {
			edits++
		}
	}
	if edits != 1 {
		t.Errorf("the edits the drag sent: want 1, got %d", edits)
	}
}

// The bar drags of cameraCaptionsStoryboard, on lanes laid out one pixel per millisecond: its
// via pan, from 200ms at {beat: 1, offset: 200ms}, grabbed at panGrabMS and let go at
// panReleaseMS, a shift snapped to panShiftMS; its until caption, from 1s at {beat: 2}, grabbed
// at captionGrabMS and let go at captionReleaseMS, a shift snapped to captionShiftMS; and its
// zoom, from 100ms, grabbed at zoomGrabMS and let go at zoomReleaseMS, inside beat 2's zoom.
const (
	panGrabMS        = 400
	panReleaseMS     = 290
	panShiftMS       = -100
	viaPanStartMS    = 200
	untilCaption     = 1
	captionGrabMS    = 1500
	captionReleaseMS = 1730
	captionShiftMS   = 250
	captionStartMS   = 1000
	captionStopMS    = 2200
	movedZoom        = 0
	zoomGrabMS       = 200
	zoomReleaseMS    = 1400
)

// The lanes of the timeline the bar drags run on, by name.
const (
	laneCameraZoom = "camera zoom"
	lanePan        = "pan"
	laneCaptions   = "captions"
)

// dragBar drags the bar of the entry of kind and index on the lane named lane, grabbed at output
// time grabMS and let go at releaseMS, and settles the page; the lane's track is laid out one
// pixel per millisecond.
func (p *pageHarness) dragBar(lane string, kind storyboard.OwnerKind, index, grabMS, releaseMS int) {
	p.t.Helper()
	found := p.eval(fmt.Sprintf(`{
		const lane = document.getElementById(%q).querySelectorAll(".lane")
			.find((row) => row.querySelectorAll(".lane-name")[0].textContent.startsWith(%q));
		const track = lane ? lane.querySelectorAll(".lane-track")[0] : null;
		const bar = track ? track.children.find((node) => node.dataset.entry === %q) : null;
		if (bar) {
			track.layout = { left: 0, top: 0, width: window.tune.state().length_ms, height: 20 };
			const fire = (type, ms) => bar.dispatchEvent(new Event(type, { clientX: ms }));
			fire("pointerdown", %d);
			fire("pointermove", %d);
			fire("pointerup", %d);
		}
		Boolean(bar);
	}`, elementTimeline, lane, entryValueOf(kind, index), grabMS, releaseMS, releaseMS)).ToBoolean()
	if !found {
		p.t.Fatalf("the %s lane has no bar for %s", lane, entryValueOf(kind, index))
	}
	p.settle()
}

// barLeft is the left of the timeline's bar for the entry of kind and index, as drawn now.
func (p *pageHarness) barLeft(kind storyboard.OwnerKind, index int) string {
	p.t.Helper()
	return p.eval(fmt.Sprintf(`document.getElementById(%q).querySelectorAll(".block")
		.find((node) => node.dataset.entry === %q).style.left`,
		elementTimeline, entryValueOf(kind, index))).String()
}

// editCount is how many edits the page has sent.
func (p *pageHarness) editCount() int {
	edits := 0
	for _, request := range p.requests {
		if request.path == "/api/edit" {
			edits++
		}
	}
	return edits
}

func TestPageJS_DragAPanBar_SendsOneCameraAtEditKeepingItsBeat(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, cameraCaptionsStoryboard).session
	page := loadPage(t, session)

	page.dragBar(lanePan, storyboard.OwnerCamera, viaPan, panGrabMS, panReleaseMS)

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,`+
		`"value":{"beat":1,"offset_ms":%d}}`,
		storyboard.OwnerCamera, viaPan, storyboard.FieldCameraAt, viaPanStartMS+panShiftMS))
	if got := page.editCount(); got != 1 {
		t.Errorf("the edits the drag sent: want 1, got %d", got)
	}
	if got := session.State().Camera[viaPan].StartMS; got != viaPanStartMS+panShiftMS {
		t.Errorf("the dragged pan's start: want %dms, got %dms", viaPanStartMS+panShiftMS, got)
	}
	if got, want := page.selectedEntry(), fmt.Sprintf(`{"kind":%q,"index":%d}`,
		storyboard.OwnerCamera, viaPan); got != want {
		t.Errorf("the selection after the drag: want %s, got %s", want, got)
	}
}

func TestPageJS_DragAnUntilCaptionBar_SendsOneCaptionShift(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, cameraCaptionsStoryboard).session
	page := loadPage(t, session)

	page.dragBar(laneCaptions, storyboard.OwnerCaption, untilCaption, captionGrabMS, captionReleaseMS)

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,"value":%d}`,
		storyboard.OwnerCaption, untilCaption, storyboard.FieldCaptionShift, captionShiftMS))
	shifted := session.State().Captions[untilCaption]
	if shifted.StartMS != captionStartMS+captionShiftMS || shifted.StopMS != captionStopMS+captionShiftMS {
		t.Errorf("the dragged caption: want it from %dms to %dms, got %+v",
			captionStartMS+captionShiftMS, captionStopMS+captionShiftMS, shifted)
	}
}

func TestPageJS_ClickABarWithoutMoving_SelectsAndSendsNoEdit(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, cameraCaptionsStoryboard).session
	page := loadPage(t, session)

	page.dragBar(laneCaptions, storyboard.OwnerCaption, untilCaption, captionGrabMS, captionGrabMS)

	if got := page.editCount(); got != 0 {
		t.Errorf("the edits a click sent: want 0, got %d", got)
	}
	if got, want := page.selectedEntry(), fmt.Sprintf(`{"kind":%q,"index":%d}`,
		storyboard.OwnerCaption, untilCaption); got != want {
		t.Errorf("the selection after the click: want %s, got %s", want, got)
	}
}

func TestPageJS_RefusedBarDrag_RedrawsInPlaceAndShowsTheRefusal(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, cameraCaptionsStoryboard).session
	page := loadPage(t, session)
	before := page.barLeft(storyboard.OwnerCamera, movedZoom)

	page.dragBar(laneCameraZoom, storyboard.OwnerCamera, movedZoom, zoomGrabMS, zoomReleaseMS)

	refused := page.lastRequest("/api/edit")
	if refused.status == http.StatusOK {
		t.Fatalf("the zoom dragged into beat 2's zoom: want it refused, got 200: %s", refused.answer)
	}
	if got := page.barLeft(storyboard.OwnerCamera, movedZoom); got != before {
		t.Errorf("the refused bar's left: want %s, got %s", before, got)
	}
	status := page.text(elementStatus)
	for _, want := range []string{"overlaps", "section", "which holds both tracks"} {
		if !strings.Contains(status, want) {
			t.Errorf("the status after the refused drag: want it containing %q, got %q", want, status)
		}
	}
}

func TestPageJS_RemoveAndMoveACameraMove(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, cameraCaptionsStoryboard).session
	page := loadPage(t, session)
	moves := len(session.State().Camera)
	page.selectValue(elementBeatSelect, entryValueOf(storyboard.OwnerCamera, viaPan))

	page.click(elementInspector, "Move down")

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,"value":%d}`,
		storyboard.OwnerCamera, viaPan, storyboard.FieldCameraMove, afterViaPan))
	moved := fmt.Sprintf(`{"kind":%q,"index":%d}`, storyboard.OwnerCamera, afterViaPan)
	if got := page.selectedEntry(); got != moved {
		t.Fatalf("the selection after the move: want %s, got %s", moved, got)
	}

	page.click(elementInspector, "Remove")

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,"value":null}`,
		storyboard.OwnerCamera, afterViaPan, storyboard.FieldCameraRemove))
	if got := len(session.State().Camera); got != moves-1 {
		t.Errorf("the camera after the remove: want %d moves, got %d", moves-1, got)
	}
	if got := page.selectedEntry(); got != "null" {
		t.Errorf("the selection after the remove: want none, got %s", got)
	}
}

func TestPageJS_MoveButtons_DisabledAtTheListEnds(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, cameraCaptionsStoryboard).session
	state := session.State()
	page := loadPage(t, session)
	cases := []struct {
		kind   storyboard.OwnerKind
		index  int
		button string
	}{
		{storyboard.OwnerCamera, 0, "Move up"},
		{storyboard.OwnerCamera, len(state.Camera) - 1, "Move down"},
		{storyboard.OwnerCaption, 0, "Move up"},
		{storyboard.OwnerCaption, len(state.Captions) - 1, "Move down"},
	}

	for _, tc := range cases {
		page.selectValue(elementBeatSelect, entryValueOf(tc.kind, tc.index))

		if !page.isDisabled(elementInspector, tc.button) {
			t.Errorf("%s %d: %s is enabled", tc.kind, tc.index, tc.button)
		}
	}
}

func TestPageJS_CaptionColour_SendsTheEditAndReloadsTheFrame(t *testing.T) {
	t.Parallel()
	const color = "#ff8800"
	session := newSessionFixtureOf(t, cameraCaptionsStoryboard).session
	page := loadPage(t, session)
	page.selectValue(elementBeatSelect, entryValueOf(storyboard.OwnerCaption, styledCaption))

	page.setField("colour", "value", color)

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,"value":%q}`,
		storyboard.OwnerCaption, styledCaption, storyboard.FieldCaptionColor, color))
	if got := session.State().Captions[styledCaption].Color; got != color {
		t.Errorf("the caption's colour after the edit: want %s, got %s", color, got)
	}
	if !page.isRequestedAfter("/api/edit", frameAPIPrefix) {
		t.Error("the page did not reload the preview frame after the edit")
	}
}

func TestPageJS_CaptionFont_ShowsSizeAndWeight_AndSendsTheEdits(t *testing.T) {
	t.Parallel()
	const size = 14.5
	weight := string(storyboard.WeightBold)
	session := newSessionFixtureOf(t, cameraCaptionsStoryboard).session
	page := loadPage(t, session)
	page.selectValue(elementBeatSelect, entryValueOf(storyboard.OwnerCaption, styledCaption))

	for _, label := range []string{"font size", "weight"} {
		if !page.hasField(label) {
			t.Fatalf("the caption's inspector shows no %q field row", label)
		}
	}

	page.setField("font size", "value", fmt.Sprint(size))

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,"value":%v}`,
		storyboard.OwnerCaption, styledCaption, storyboard.FieldCaptionFontSize, size))
	if got := session.State().Captions[styledCaption].FontSize; got != size {
		t.Errorf("the caption's font size after the edit: want %v, got %v", size, got)
	}
	if !page.isRequestedAfter("/api/edit", frameAPIPrefix) {
		t.Error("the page did not reload the preview frame after the font size edit")
	}

	page.chooseField("weight", weight)

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,"value":%q}`,
		storyboard.OwnerCaption, styledCaption, storyboard.FieldCaptionFontWeight, weight))
	if got := session.State().Captions[styledCaption].FontWeight; got != weight {
		t.Errorf("the caption's font weight after the edit: want %s, got %s", weight, got)
	}
	if !page.isRequestedAfter("/api/edit", frameAPIPrefix) {
		t.Error("the page did not reload the preview frame after the weight edit")
	}
}

// hasField says the inspector shows exactly one field row labelled label.
func (p *pageHarness) hasField(label string) bool {
	p.t.Helper()
	return p.eval(fmt.Sprintf(`document.getElementById(%q).querySelectorAll("label")
		.filter((row) => row.children.length > 0 && row.children[0].textContent === %q)
		.length === 1`, elementInspector, label)).ToBoolean()
}

func TestPageJS_CaptionPosition_SeedsTheLaidOutCorner(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, cameraCaptionsStoryboard).session
	corner := session.State().Captions[styledCaption].BoxPosition
	if corner == nil {
		t.Fatalf("caption %d: want a laid-out box, got none", styledCaption)
	}
	page := loadPage(t, session)
	page.selectValue(elementBeatSelect, entryValueOf(storyboard.OwnerCaption, styledCaption))

	page.setField("explicit position", "checked", true)

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,"value":{"x":%d,"y":%d}}`,
		storyboard.OwnerCaption, styledCaption, storyboard.FieldCaptionPosition, corner.X, corner.Y))
	if got := session.State().Captions[styledCaption].Position; got == nil || *got != *corner {
		t.Fatalf("the caption's position after the check: want %+v, got %+v", *corner, got)
	}

	page.setField("explicit position", "checked", false)

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,"value":null}`,
		storyboard.OwnerCaption, styledCaption, storyboard.FieldCaptionPosition))
	if got := session.State().Captions[styledCaption].Position; got != nil {
		t.Errorf("the caption's position after the uncheck: want none, got %+v", *got)
	}
}

// The via path overlay's elements and the class that shows it.
const (
	elementViaPath = "via-path"
	elementViaLine = "via-line"
	viaPathDrawn   = "drawn"
)

// pickAcross picks the cells from column first to last of row on the preview and returns the
// proposal's target the pick answered.
func (p *pageHarness) pickAcross(session *Session, row, first, last int) json.RawMessage {
	p.t.Helper()
	p.fitFrame()
	p.dispatch(elementPick, "click", nil)
	p.dispatch(elementStage, "pointerdown", pointerAt(cellPixel(p.t, session, first, row)))
	p.dispatch(elementStage, "pointerup", pointerAt(cellPixel(p.t, session, last, row)))
	p.settle()
	picked := p.lastRequest("/api/pick")
	if picked.status != http.StatusOK {
		p.t.Fatalf("the pick: want 200, got %d: %s", picked.status, picked.answer)
	}
	var proposal struct {
		Target json.RawMessage `json:"target"`
	}
	if err := json.Unmarshal([]byte(picked.answer), &proposal); err != nil {
		p.t.Fatalf("decode the pick's answer %s: %v", picked.answer, err)
	}
	return proposal.Target
}

// chooseField chooses value in the select of the inspector's one field row labelled label,
// fires its change and settles the page.
func (p *pageHarness) chooseField(label, value string) {
	p.t.Helper()
	found := p.eval(fmt.Sprintf(`{
		const rows = document.getElementById(%q).querySelectorAll("label")
			.filter((row) => row.children.length > 0 && row.children[0].textContent === %q);
		const input = rows.length === 1 ? rows[0].querySelectorAll("select")[0] : undefined;
		if (input) {
			input.value = %q;
			input.dispatchEvent(new Event("change"));
		}
		Boolean(input);
	}`, elementInspector, label, value)).ToBoolean()
	if !found {
		p.t.Fatalf("no one select in a field row labelled %q under #%s", label, elementInspector)
	}
	p.settle()
}

// isViaPathDrawn says the via path overlay shows.
func (p *pageHarness) isViaPathDrawn() bool {
	p.t.Helper()
	return p.eval(fmt.Sprintf(`document.getElementById(%q).classList.contains(%q)`,
		elementViaPath, viaPathDrawn)).ToBoolean()
}

// assertViaPath fails the test unless the page asked for the via pan's path on the frame it
// shows and its line draws the points the session answers for it.
func (p *pageHarness) assertViaPath(session *Session) {
	p.t.Helper()
	ms := p.eval("window.tune.frameTime()").ToInteger()
	asked := p.lastRequest(fmt.Sprintf("/api/path?camera=%d&t=%d", viaPan, ms))
	if asked.status != http.StatusOK {
		p.t.Fatalf("the path request at %dms: want 200, got %d: %s", ms, asked.status, asked.answer)
	}
	points, err := session.Path(time.Duration(ms)*time.Millisecond, viaPan)
	if err != nil {
		p.t.Fatalf("the session's path at %dms: %v", ms, err)
	}
	spelled := make([]string, 0, len(points))
	for _, point := range points {
		spelled = append(spelled, fmt.Sprintf("%d,%d", point.X, point.Y))
	}
	want := strings.Join(spelled, " ")
	got := p.eval(fmt.Sprintf(`document.getElementById(%q).getAttribute("points")`,
		elementViaLine)).String()
	if got != want {
		p.t.Errorf("the via line at %dms: want points %q, got %q", ms, want, got)
	}
	if !p.isViaPathDrawn() {
		p.t.Errorf("the via path at %dms is not drawn", ms)
	}
}

func TestPageJS_ViaWaypoint_SetFromPick(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, cameraCaptionsStoryboard).session
	page := loadPage(t, session)
	page.selectValue(elementBeatSelect, entryValueOf(storyboard.OwnerCamera, viaPan))
	page.settle()

	page.click(elementInspector, "Set waypoint 0 from the pick")

	if got := page.text(elementPickStatus); !strings.HasPrefix(got, "Pick a target first") {
		t.Errorf("the pick status with no pick standing: want it to ask for a pick, got %q", got)
	}
	for _, request := range page.requests {
		if request.path == "/api/edit" {
			t.Fatalf("the page sent an edit with no pick standing: %s", request.body)
		}
	}

	target := page.pickAcross(session, 0, 2, 4)
	page.click(elementInspector, "Set waypoint 0 from the pick")

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,`+
		`"value":{"index":0,"point":%s}}`,
		storyboard.OwnerCamera, viaPan, storyboard.FieldCameraVia, target))
	if got := session.State().Camera[viaPan].Via[0]; got.Target == nil {
		t.Errorf("waypoint 0 after the set: want the picked target, got %+v", got)
	}
}

func TestPageJS_ViaWaypoint_InsertAndRemove(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, cameraCaptionsStoryboard).session
	first := *session.State().Camera[viaPan].Via[0].Cell
	page := loadPage(t, session)
	page.selectValue(elementBeatSelect, entryValueOf(storyboard.OwnerCamera, viaPan))
	page.settle()
	owner := fmt.Sprintf(`{"kind":%q,"index":%d}`, storyboard.OwnerCamera, viaPan)

	page.click(elementInspector, "Insert a copy of waypoint 0 after it")

	page.assertEdit(fmt.Sprintf(`{"owner":%s,"field":%q,`+
		`"value":{"index":1,"point":{"cell":{"x":%d,"y":%d}}}}`,
		owner, storyboard.FieldCameraViaInsert, first.X, first.Y))
	via := session.State().Camera[viaPan].Via
	if len(via) != 3 || via[1].Cell == nil || *via[1].Cell != first {
		t.Fatalf("the via path after the insert: want 3 waypoints, the second %+v, got %+v", first, via)
	}

	page.click(elementInspector, "Remove waypoint 2")

	page.assertEdit(fmt.Sprintf(`{"owner":%s,"field":%q,"value":{"index":2}}`,
		owner, storyboard.FieldCameraViaRemove))
	if via := session.State().Camera[viaPan].Via; len(via) != 2 || via[1].Cell == nil {
		t.Fatalf("the via path after the remove: want the two cells, got %+v", via)
	}

	page.setField("waypoint 1 (x, y)", "value", "5")

	page.assertEdit(fmt.Sprintf(`{"owner":%s,"field":%q,`+
		`"value":{"index":1,"point":{"cell":{"x":5,"y":%d}}}}`,
		owner, storyboard.FieldCameraVia, first.Y))

	page.chooseField("path", "to")

	page.assertEdit(fmt.Sprintf(`{"owner":%s,"field":%q,"value":"to"}`,
		owner, storyboard.FieldCameraPath))
	moved := session.State().Camera[viaPan]
	if moved.Via != nil || moved.Cell == nil || moved.Cell.X != 5 {
		t.Errorf("the pan after the path change: want it straight to the last waypoint, got %+v", moved)
	}
}

func TestPageJS_ViaPath_IsDrawnForTheSelectedPan(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, cameraCaptionsStoryboard).session
	page := loadPage(t, session)
	page.fitFrame()

	if page.isViaPathDrawn() {
		t.Fatal("the via path is drawn with nothing selected")
	}

	page.selectValue(elementBeatSelect, entryValueOf(storyboard.OwnerCamera, viaPan))
	page.settle()

	page.assertViaPath(session)

	page.eval(fmt.Sprintf("window.tune.seek(%d)", beatOneMS))
	page.settle()

	if got := page.eval("window.tune.frameTime()").ToInteger(); got != beatOneMS {
		t.Fatalf("the frame shown after the seek: want the one at %dms, got %d", beatOneMS, got)
	}
	page.assertViaPath(session)

	page.selectValue(elementBeatSelect, entryValueOf(storyboard.OwnerCamera, panToTarget))
	page.settle()

	if page.isViaPathDrawn() {
		t.Error("the via path is drawn with a pan to selected")
	}
}
