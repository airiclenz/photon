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
	"slices"
	"strconv"
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
	elementBeatSelect = "beat-select"
	elementInspector  = "inspector-body"
	elementTimeline   = "timeline"
	elementTime       = "time"
	elementBuild      = "build"
	elementStatus     = "status"
	elementDocument   = "document"
)

// captionedStoryboard is sessionStoryboard with two captions: one set by for in beat 1, one by
// until in beat 2.
const captionedStoryboard = sessionStoryboard + `captions:
  - {text: 'Watch this', at: {beat: 1, offset: 100ms}, for: 700ms, place: top, align: left}
  - {text: "All green\n", at: {beat: 2}, until: {beat: 2, offset: 1200ms}}
`

// The captions of captionedStoryboard the list tests add, move and remove around: the caption
// index a caption added at addCaptionAtMS becomes, after the caption at 100ms and before the one
// at 1s, and the caption the move and remove test moves down, and the index it moves to.
const (
	addCaptionAtMS = 500
	addedCaption   = 1
	movedCaption   = 0
	movedToCaption = 1
)

// styledCaption is the caption of captionedStoryboard the style tests edit: set by place and
// align, and laid out in a box.
const styledCaption = 0

// frameAPIPrefix starts the path of every preview frame request.
const frameAPIPrefix = "/api/frame?"

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
	p.setControl("input", label, property, value)
}

// setTextArea sets the value of the textarea in the inspector's one field row labelled label to
// text, fires its change and settles the page.
func (p *pageHarness) setTextArea(label, text string) {
	p.t.Helper()
	p.setControl("textarea", label, "value", text)
}

// setControl sets property of the first tag element in the inspector's one field row labelled
// label to value, fires its change and settles the page. tag is one element name: the DOM stub
// takes no selector list.
func (p *pageHarness) setControl(tag, label, property string, value any) {
	p.t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		p.t.Fatalf("encode %v: %v", value, err)
	}
	found := p.eval(fmt.Sprintf(`{
		const rows = document.getElementById(%q).querySelectorAll("label")
			.filter((row) => row.children.length > 0 && row.children[0].textContent === %q);
		const input = rows.length === 1 ? rows[0].querySelectorAll(%q)[0] : undefined;
		if (input && !input.disabled) {
			input[%q] = %s;
			input.dispatchEvent(new Event("change"));
		}
		Boolean(input && !input.disabled);
	}`, elementInspector, label, tag, property, encoded)).ToBoolean()
	if !found {
		p.t.Fatalf("no one enabled %s in a field row labelled %q under #%s", tag, label, elementInspector)
	}
	p.settle()
}

// errorRows are the texts of the error rows the inspector shows.
func (p *pageHarness) errorRows() []string {
	p.t.Helper()
	var rows []string
	encoded := p.eval(fmt.Sprintf(`JSON.stringify(document.getElementById(%q).querySelectorAll("p.error")
		.map((row) => row.textContent))`, elementInspector)).String()
	if err := json.Unmarshal([]byte(encoded), &rows); err != nil {
		p.t.Fatalf("decode the inspector's error rows %s: %v", encoded, err)
	}
	return rows
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
	page := loadPage(t, newSessionFixtureOf(t, captionedStoryboard).session)

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
}

func TestPageJS_Build_ShowsTheSessionsBuild(t *testing.T) {
	t.Parallel()
	session := newSessionFixture(t).session
	session.photonBuild = testBuild

	page := loadPage(t, session)

	if got := page.text(elementBuild); got != testBuild {
		t.Errorf("the transport bar's build: want %q, got %q", testBuild, got)
	}
}

// entryValueOf spells a caption's entry as the beat select's value for it.
func entryValueOf(kind storyboard.OwnerKind, index int) string {
	return fmt.Sprintf("%s:%d", kind, index)
}

func TestPageJS_AddCaptionAtThePlayhead(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, captionedStoryboard).session
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

// The bar drag of captionedStoryboard, on a lane laid out one pixel per millisecond: its until
// caption, from 1s at {beat: 2}, grabbed at captionGrabMS and let go at captionReleaseMS, a shift
// snapped to captionShiftMS.
const (
	untilCaption     = 1
	captionGrabMS    = 1500
	captionReleaseMS = 1730
	captionShiftMS   = 250
	captionStartMS   = 1000
	captionStopMS    = 2200
)

// laneCaptions is the name of the timeline's lane the bar drags run on.
const laneCaptions = "captions"

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

func TestPageJS_DragAnUntilCaptionBar_SendsOneCaptionShift(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, captionedStoryboard).session
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
	session := newSessionFixtureOf(t, captionedStoryboard).session
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

func TestPageJS_RemoveAndMoveACaption(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, captionedStoryboard).session
	page := loadPage(t, session)
	captions := len(session.State().Captions)
	page.selectValue(elementBeatSelect, entryValueOf(storyboard.OwnerCaption, movedCaption))

	page.click(elementInspector, "Move down")

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,"value":%d}`,
		storyboard.OwnerCaption, movedCaption, storyboard.FieldCaptionMove, movedToCaption))
	moved := fmt.Sprintf(`{"kind":%q,"index":%d}`, storyboard.OwnerCaption, movedToCaption)
	if got := page.selectedEntry(); got != moved {
		t.Fatalf("the selection after the move: want %s, got %s", moved, got)
	}

	page.click(elementInspector, "Remove")

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,"value":null}`,
		storyboard.OwnerCaption, movedToCaption, storyboard.FieldCaptionRemove))
	if got := len(session.State().Captions); got != captions-1 {
		t.Errorf("the captions after the remove: want %d, got %d", captions-1, got)
	}
	if got := page.selectedEntry(); got != "null" {
		t.Errorf("the selection after the remove: want none, got %s", got)
	}
}

func TestPageJS_MoveButtons_DisabledAtTheListEnds(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, captionedStoryboard).session
	state := session.State()
	page := loadPage(t, session)
	cases := []struct {
		kind   storyboard.OwnerKind
		index  int
		button string
	}{
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

// zoomedStoryboard is sessionStoryboard under two camera zooms: one from 100ms to 400ms by clip
// time, one from 1.4s to 1.7s, 400ms into beat 2.
const zoomedStoryboard = sessionStoryboard + `camera:
  zooms:
    - {to: 2, at: 100ms, over: 300ms}
    - {to: 3, at: {beat: 2, offset: 400ms}, over: 300ms}
`

// The zooms of zoomedStoryboard the zoom tests edit, add around and drag, on a lane laid out one
// pixel per millisecond: a zoom added at addZoomAtMS becomes index addedZoom, after the zoom at
// 100ms; the zoom in beat 2, from 1.4s, grabbed at beatZoomGrabMS and let go at
// beatZoomReleaseMS, snaps to a shift of beatZoomShiftMS; and the first zoom, grabbed at
// firstZoomGrabMS and let go at firstZoomReleaseMS, lands over the zoom in beat 2.
const (
	firstZoom          = 0
	beatZoom           = 1
	addZoomAtMS        = 700
	addedZoom          = 1
	beatZoomOffsetMS   = 400
	beatZoomStartMS    = 1400
	beatZoomGrabMS     = 1500
	beatZoomReleaseMS  = 1290
	beatZoomShiftMS    = -200
	firstZoomGrabMS    = 200
	firstZoomReleaseMS = 1400
)

// laneZoom is the name of the timeline's zoom lane.
const laneZoom = "zoom"

func TestPageJS_AddZoomAtThePlayhead(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, zoomedStoryboard).session
	page := loadPage(t, session)
	page.eval(fmt.Sprintf("window.tune.seek(%d)", addZoomAtMS))
	page.settle()

	page.click(elementTimeline, "Add a zoom at the playhead")

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,`+
		`"value":{"at":{"beat":0,"offset_ms":%d}}}`,
		storyboard.OwnerZoom, addedZoom, storyboard.FieldZoomInsert, addZoomAtMS))
	if added := session.State().Zooms[addedZoom]; added.StartMS != addZoomAtMS {
		t.Errorf("the added zoom: want it from %dms, got %+v", addZoomAtMS, added)
	}
	want := fmt.Sprintf(`{"kind":%q,"index":%d}`, storyboard.OwnerZoom, addedZoom)
	if got := page.selectedEntry(); got != want {
		t.Errorf("the selection after the add: want %s, got %s", want, got)
	}
}

func TestPageJS_DragAZoomBar_SendsOneZoomAtEditKeepingItsBeat(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, zoomedStoryboard).session
	page := loadPage(t, session)

	page.dragBar(laneZoom, storyboard.OwnerZoom, beatZoom, beatZoomGrabMS, beatZoomReleaseMS)

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,`+
		`"value":{"beat":2,"offset_ms":%d}}`,
		storyboard.OwnerZoom, beatZoom, storyboard.FieldZoomAt, beatZoomOffsetMS+beatZoomShiftMS))
	if got := page.editCount(); got != 1 {
		t.Errorf("the edits the drag sent: want 1, got %d", got)
	}
	if got := session.State().Zooms[beatZoom].StartMS; got != beatZoomStartMS+beatZoomShiftMS {
		t.Errorf("the dragged zoom's start: want %dms, got %dms", beatZoomStartMS+beatZoomShiftMS, got)
	}
	want := fmt.Sprintf(`{"kind":%q,"index":%d}`, storyboard.OwnerZoom, beatZoom)
	if got := page.selectedEntry(); got != want {
		t.Errorf("the selection after the drag: want %s, got %s", want, got)
	}
}

// barLeft is the left of the timeline's bar for the entry of kind and index, as drawn now.
func (p *pageHarness) barLeft(kind storyboard.OwnerKind, index int) string {
	p.t.Helper()
	return p.eval(fmt.Sprintf(`document.getElementById(%q).querySelectorAll(".block")
		.find((node) => node.dataset.entry === %q).style.left`,
		elementTimeline, entryValueOf(kind, index))).String()
}

func TestPageJS_RefusedZoomDrag_RedrawsInPlaceAndShowsTheRefusal(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, zoomedStoryboard).session
	page := loadPage(t, session)
	before := page.barLeft(storyboard.OwnerZoom, firstZoom)

	page.dragBar(laneZoom, storyboard.OwnerZoom, firstZoom, firstZoomGrabMS, firstZoomReleaseMS)

	refused := page.lastRequest("/api/edit")
	if refused.status == http.StatusOK {
		t.Fatalf("the zoom dragged onto another zoom: want it refused, got 200: %s", refused.answer)
	}
	if got := page.barLeft(storyboard.OwnerZoom, firstZoom); got != before {
		t.Errorf("the refused bar's left: want %s, got %s", before, got)
	}
	if status := page.text(elementStatus); !strings.Contains(status, "overlaps") {
		t.Errorf("the status after the refused drag: want it containing %q, got %q", "overlaps", status)
	}
}

// choices are the values of the options of the select in the inspector's one field row labelled
// label.
func (p *pageHarness) choices(label string) []string {
	p.t.Helper()
	var values []string
	encoded := p.eval(fmt.Sprintf(`JSON.stringify(document.getElementById(%q).querySelectorAll("label")
		.filter((row) => row.children.length > 0 && row.children[0].textContent === %q)
		.flatMap((row) => row.querySelectorAll("option").map((option) => option.value)))`,
		elementInspector, label)).String()
	if err := json.Unmarshal([]byte(encoded), &values); err != nil {
		p.t.Fatalf("decode the choices of %q %s: %v", label, encoded, err)
	}
	return values
}

func TestPageJS_ZoomPanel_OffersTheFourEases(t *testing.T) {
	t.Parallel()
	page := loadPage(t, newSessionFixtureOf(t, zoomedStoryboard).session)
	want := []string{
		string(storyboard.EaseSmooth), string(storyboard.EaseLinear),
		string(storyboard.EaseIn), string(storyboard.EaseOut),
	}

	page.selectValue(elementBeatSelect, entryValueOf(storyboard.OwnerZoom, firstZoom))

	if got := page.choices("ease"); !reflect.DeepEqual(got, want) {
		t.Errorf("the zoom's ease choices: want %q, got %q", want, got)
	}
}

func TestPageJS_ZoomPanel_SendsEachFieldsEdit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		edit  func(page *pageHarness)
		field string
		value string
	}{
		{
			name:  "at",
			edit:  func(page *pageHarness) { page.setField("at (ms)", "value", "150") },
			field: storyboard.FieldZoomAt,
			value: `{"beat":0,"offset_ms":150}`,
		},
		{
			name:  "over",
			edit:  func(page *pageHarness) { page.setField("over (ms)", "value", "500") },
			field: storyboard.FieldZoomOver,
			value: "500",
		},
		{
			name:  "ease",
			edit:  func(page *pageHarness) { page.chooseField("ease", string(storyboard.EaseIn)) },
			field: storyboard.FieldZoomEase,
			value: fmt.Sprintf("%q", storyboard.EaseIn),
		},
		{
			name:  "to",
			edit:  func(page *pageHarness) { page.setField("to (×)", "value", "2.5") },
			field: storyboard.FieldZoomTo,
			value: "2.5",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			page := loadPage(t, newSessionFixtureOf(t, zoomedStoryboard).session)
			page.selectValue(elementBeatSelect, entryValueOf(storyboard.OwnerZoom, firstZoom))

			tc.edit(page)

			page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,"value":%s}`,
				storyboard.OwnerZoom, firstZoom, tc.field, tc.value))
		})
	}
}

func TestPageJS_RemoveAZoom(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, zoomedStoryboard).session
	page := loadPage(t, session)
	zooms := len(session.State().Zooms)
	page.selectValue(elementBeatSelect, entryValueOf(storyboard.OwnerZoom, firstZoom))

	page.click(elementInspector, "Remove")

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,"value":null}`,
		storyboard.OwnerZoom, firstZoom, storyboard.FieldZoomRemove))
	if got := len(session.State().Zooms); got != zooms-1 {
		t.Errorf("the zooms after the remove: want %d, got %d", zooms-1, got)
	}
	if got := page.selectedEntry(); got != "null" {
		t.Errorf("the selection after the remove: want none, got %s", got)
	}
}

// positionedStoryboard is sessionStoryboard with three camera positions listed out of start
// order: a text target held over the first 500ms of beat 2, a cell held for 300ms from 200ms into
// beat 1, and a cell point at 700ms by clip time.
const positionedStoryboard = sessionStoryboard + `camera:
  positions:
    - {text: tests, at: {beat: 2}, hold: 500ms}
    - {cell: {x: 3, y: 1}, at: {beat: 1, offset: 200ms}, hold: 300ms}
    - {cell: {x: 0, y: 0}, at: 700ms}
`

// The positions of positionedStoryboard, by their index in camera.positions: by start the held
// cell comes first, the point second and the held text target last.
const (
	textPosition  = 0
	firstPosition = 1
	pointPosition = 2
)

// entryTransition is the page's selection kind for a camera transition, its index the position
// it leads into's; the storyboard owns a transition's edits through that position.
const entryTransition storyboard.OwnerKind = "transition"

// lanePosition is the name of the timeline's position lane.
const lanePosition = "position"

// laneBar is one bar of a timeline lane as drawn: the entry it selects, whether it is drawn as a
// point, and the output times it spans, read back from its left and width.
type laneBar struct {
	Entry   string `json:"entry"`
	IsPoint bool   `json:"point"`
	FromMS  int64  `json:"from"`
	ToMS    int64  `json:"to"`
}

// laneBars are the bars of the timeline's lane named lane, sorted by the entry each selects.
func (p *pageHarness) laneBars(lane string) []laneBar {
	p.t.Helper()
	encoded := p.eval(fmt.Sprintf(`{
		const length = window.tune.state().length_ms;
		const ms = (percent) => Math.round(percent * length / 100);
		const row = document.getElementById(%q).querySelectorAll(".lane")
			.find((candidate) => candidate.querySelectorAll(".lane-name")[0].textContent.startsWith(%q));
		JSON.stringify(row.querySelectorAll(".lane-track")[0].children.map((node) => ({
			entry: node.dataset.entry,
			point: node.classList.contains("point"),
			from: ms(parseFloat(node.style.left)),
			to: ms(parseFloat(node.style.left) + parseFloat(node.style.width)),
		})));
	}`, elementTimeline, lane)).String()
	var bars []laneBar
	if err := json.Unmarshal([]byte(encoded), &bars); err != nil {
		p.t.Fatalf("decode the %s lane's bars %s: %v", lane, encoded, err)
	}
	slices.SortFunc(bars, func(a, b laneBar) int { return strings.Compare(a.Entry, b.Entry) })
	return bars
}

// clickBar clicks the timeline's bar for the entry of kind and index and settles the page.
func (p *pageHarness) clickBar(kind storyboard.OwnerKind, index int) {
	p.t.Helper()
	found := p.eval(fmt.Sprintf(`{
		const bar = document.getElementById(%q).querySelectorAll(".block")
			.find((node) => node.dataset.entry === %q);
		if (bar) {
			bar.dispatchEvent(new Event("click"));
		}
		Boolean(bar);
	}`, elementTimeline, entryValueOf(kind, index))).ToBoolean()
	if !found {
		p.t.Fatalf("the timeline has no bar for %s", entryValueOf(kind, index))
	}
	p.settle()
}

// heading is the text of the inspector's heading.
func (p *pageHarness) heading() string {
	p.t.Helper()
	return p.eval(fmt.Sprintf(`document.getElementById(%q).querySelectorAll("h2")[0].textContent`,
		elementInspector)).String()
}

func TestPageJS_PositionLane_DrawsPositionsPointsAndTransitionsByStart(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, positionedStoryboard).session
	positions := session.State().Positions
	first, point, text := positions[firstPosition], positions[pointPosition], positions[textPosition]
	want := []laneBar{
		{entryValueOf(storyboard.OwnerPosition, textPosition), false, text.StartMS, text.EndMS},
		{entryValueOf(storyboard.OwnerPosition, firstPosition), false, first.StartMS, first.EndMS},
		{entryValueOf(storyboard.OwnerPosition, pointPosition), true, point.StartMS, point.StartMS},
		{entryValueOf(entryTransition, textPosition), false, point.EndMS, text.StartMS},
		{entryValueOf(entryTransition, pointPosition), false, first.EndMS, point.StartMS},
	}

	page := loadPage(t, session)

	if got := page.laneBars(lanePosition); !reflect.DeepEqual(got, want) {
		t.Errorf("the position lane's bars: want %+v, got %+v", want, got)
	}
}

func TestPageJS_PositionLane_SelectsEachBarsEntry(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind    storyboard.OwnerKind
		index   int
		heading string
	}{
		{storyboard.OwnerPosition, firstPosition, "position 1"},
		{storyboard.OwnerPosition, pointPosition, "position 2"},
		{entryTransition, pointPosition, "transition into position 2"},
		{entryTransition, textPosition, "transition into position 0"},
	}
	page := loadPage(t, newSessionFixtureOf(t, positionedStoryboard).session)

	for _, tc := range cases {
		page.clickBar(tc.kind, tc.index)

		if got, want := page.selectedEntry(), fmt.Sprintf(`{"kind":%q,"index":%d}`, tc.kind, tc.index); got != want {
			t.Errorf("the selection after clicking %s %d: want %s, got %s", tc.kind, tc.index, want, got)
		}
		if got := page.heading(); got != tc.heading {
			t.Errorf("the inspector after clicking %s %d: want %q, got %q", tc.kind, tc.index, tc.heading, got)
		}
	}
}

func TestPageJS_PositionPanels_SendEachFieldsEdit(t *testing.T) {
	t.Parallel()
	positions := newSessionFixtureOf(t, positionedStoryboard).session.State().Positions
	start, end := positions[pointPosition].EndMS, positions[textPosition].StartMS
	const shiftMS, lengthMS = 100, 200
	cases := []struct {
		name  string
		entry string
		edit  func(page *pageHarness)
		index int
		field string
		value string
	}{
		{
			name:  "at",
			entry: entryValueOf(storyboard.OwnerPosition, firstPosition),
			edit:  func(page *pageHarness) { page.setField("at (ms)", "value", "250") },
			index: firstPosition,
			field: storyboard.FieldPositionAt,
			value: `{"beat":1,"offset_ms":250}`,
		},
		{
			name:  "hold",
			entry: entryValueOf(storyboard.OwnerPosition, firstPosition),
			edit:  func(page *pageHarness) { page.setField("hold (ms)", "value", "100") },
			index: firstPosition,
			field: storyboard.FieldPositionHold,
			value: "100",
		},
		{
			name:  "cell",
			entry: entryValueOf(storyboard.OwnerPosition, firstPosition),
			edit:  func(page *pageHarness) { page.setField("cell (x, y)", "value", "5") },
			index: firstPosition,
			field: storyboard.FieldPositionCell,
			value: `{"x":5,"y":1}`,
		},
		{
			name:  "text",
			entry: entryValueOf(storyboard.OwnerPosition, textPosition),
			edit:  func(page *pageHarness) { page.setField("text", "value", "failing") },
			index: textPosition,
			field: storyboard.FieldPositionTarget,
			value: `{"text":"failing"}`,
		},
		{
			name:  "nth",
			entry: entryValueOf(storyboard.OwnerPosition, textPosition),
			edit:  func(page *pageHarness) { page.setField("nth", "value", "last") },
			index: textPosition,
			field: storyboard.FieldPositionTarget,
			value: `{"text":"tests","nth":"last"}`,
		},
		{
			name:  "transition start",
			entry: entryValueOf(entryTransition, textPosition),
			edit: func(page *pageHarness) {
				page.setField("start (ms)", "value", strconv.FormatInt(start-shiftMS, 10))
			},
			index: textPosition,
			field: storyboard.FieldPositionTransition,
			value: fmt.Sprintf(`{"start_ms":%d,"end_ms":%d}`, start-shiftMS, end-shiftMS),
		},
		{
			name:  "transition length",
			entry: entryValueOf(entryTransition, textPosition),
			edit:  func(page *pageHarness) { page.setField("length (ms)", "value", strconv.Itoa(lengthMS)) },
			index: textPosition,
			field: storyboard.FieldPositionTransition,
			value: fmt.Sprintf(`{"start_ms":%d,"end_ms":%d}`, start, start+lengthMS),
		},
		{
			name:  "transition ease",
			entry: entryValueOf(entryTransition, textPosition),
			edit:  func(page *pageHarness) { page.chooseField("ease", string(storyboard.EaseIn)) },
			index: textPosition,
			field: storyboard.FieldPositionEase,
			value: fmt.Sprintf("%q", storyboard.EaseIn),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			page := loadPage(t, newSessionFixtureOf(t, positionedStoryboard).session)
			page.selectValue(elementBeatSelect, tc.entry)

			tc.edit(page)

			page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,"value":%s}`,
				storyboard.OwnerPosition, tc.index, tc.field, tc.value))
			if got := page.editCount(); got != 1 {
				t.Errorf("the edits sent: want 1, got %d", got)
			}
		})
	}
}

func TestPageJS_TransitionPanel_OffersTheFourEases(t *testing.T) {
	t.Parallel()
	page := loadPage(t, newSessionFixtureOf(t, positionedStoryboard).session)
	want := []string{
		string(storyboard.EaseSmooth), string(storyboard.EaseLinear),
		string(storyboard.EaseIn), string(storyboard.EaseOut),
	}

	page.selectValue(elementBeatSelect, entryValueOf(entryTransition, textPosition))

	if got := page.choices("ease"); !reflect.DeepEqual(got, want) {
		t.Errorf("the transition's ease choices: want %q, got %q", want, got)
	}
}

func TestPageJS_RefusedTransitionLength_ShowsOnTheTransitionPanel(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, positionedStoryboard).session
	positions := session.State().Positions
	pastTheEnd := positions[textPosition].EndMS - positions[pointPosition].EndMS + 100
	page := loadPage(t, session)
	transition := entryValueOf(entryTransition, textPosition)
	page.selectValue(elementBeatSelect, transition)

	page.setField("length (ms)", "value", strconv.FormatInt(pastTheEnd, 10))

	if refused := page.lastRequest("/api/edit"); refused.status == http.StatusOK {
		t.Fatalf("a transition past its position's end: want it refused, got 200: %s", refused.answer)
	}
	if got, want := page.selectedEntry(), fmt.Sprintf(`{"kind":%q,"index":%d}`, entryTransition, textPosition); got != want {
		t.Errorf("the selection after the refusal: want %s, got %s", want, got)
	}
	if !containsRow(page.errorRows(), "no later than") {
		t.Errorf("the transition's panel: want the refusal, got %q", page.errorRows())
	}
}

func TestPageJS_RemoveAPosition(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, positionedStoryboard).session
	page := loadPage(t, session)
	positions := len(session.State().Positions)
	page.selectValue(elementBeatSelect, entryValueOf(storyboard.OwnerPosition, pointPosition))

	page.click(elementInspector, "Remove")

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":%d},"field":%q,"value":null}`,
		storyboard.OwnerPosition, pointPosition, storyboard.FieldPositionRemove))
	if got := len(session.State().Positions); got != positions-1 {
		t.Errorf("the positions after the remove: want %d, got %d", positions-1, got)
	}
}

func TestPageJS_DragAPositionOrTransitionBar_SendsNoEdit(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind  storyboard.OwnerKind
		index int
	}{
		{storyboard.OwnerPosition, firstPosition},
		{entryTransition, textPosition},
	}

	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			t.Parallel()
			session := newSessionFixtureOf(t, positionedStoryboard).session
			page := loadPage(t, session)
			bar := session.State().Positions[tc.index]

			page.dragBar(lanePosition, tc.kind, tc.index, int(bar.StartMS), int(bar.StartMS)+300)

			if got := page.editCount(); got != 0 {
				t.Errorf("the edits the drag sent: want 0, got %d", got)
			}
		})
	}
}

func TestPageJS_CaptionColour_SendsTheEditAndReloadsTheFrame(t *testing.T) {
	t.Parallel()
	const color = "#ff8800"
	session := newSessionFixtureOf(t, captionedStoryboard).session
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
	session := newSessionFixtureOf(t, captionedStoryboard).session
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

func TestPageJS_ClipFrame_EditsTheFontFields(t *testing.T) {
	t.Parallel()
	session := newSessionFixture(t).session
	page := loadPage(t, session)
	cases := []struct {
		label, field, typed string
		value               any
		stateOf             func(State) any
	}{
		{"font size", storyboard.FieldFrameFontSize, "12.5", 12.5, func(s State) any { return s.FontSize }},
		{"line height", storyboard.FieldFrameLineHeight, "1.25", 1.25, func(s State) any { return s.LineHeight }},
		{"padding (px)", storyboard.FieldFramePadding, "0", 0, func(s State) any { return s.Padding }},
		{"scale", storyboard.FieldFrameScale, "3", 3, func(s State) any { return s.Scale }},
	}
	for _, label := range []string{"cols", "rows", "max_colors"} {
		if !page.hasField(label) {
			t.Errorf("the Clip frame panel shows no read-only %q row", label)
		}
	}

	for _, edit := range cases {
		page.setField(edit.label, "value", edit.typed)

		page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":0},"field":%q,"value":%v}`,
			storyboard.OwnerClip, edit.field, edit.value))
		if got := edit.stateOf(session.State()); got != edit.value {
			t.Errorf("the frame's %s after the edit: want %v, got %v", edit.label, edit.value, got)
		}
		if !page.isRequestedAfter("/api/edit", frameAPIPrefix) {
			t.Errorf("the page did not reload the preview frame after the %s edit", edit.label)
		}
	}
}

// hasField says the inspector shows exactly one field row labelled label.
func (p *pageHarness) hasField(label string) bool {
	p.t.Helper()
	return p.fieldCount(label) == 1
}

// fieldCount is the number of field rows labelled label the inspector shows.
func (p *pageHarness) fieldCount(label string) int {
	p.t.Helper()
	return int(p.eval(fmt.Sprintf(`document.getElementById(%q).querySelectorAll("label")
		.filter((row) => row.children.length > 0 && row.children[0].textContent === %q)
		.length`, elementInspector, label)).ToInteger())
}

func TestPageJS_CaptionPosition_SeedsTheLaidOutCorner(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, captionedStoryboard).session
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

// clipFrameEntry is the beat select's value for the Clip frame panel.
const clipFrameEntry = ""

// The problem edits' values: a beat's hold past its 1s duration, a frame rate under one, a zoom
// factor past the largest a zoom takes, a position's hold past the start of the next one, and a
// caption text wider than the session frame.
const (
	holdPastDuration = "5000"
	fpsUnderOne      = "0"
	zoomPastLargest  = "9"
	holdPastNext     = "5000"
)

var captionTooWide = strings.Repeat("too wide ", 20)

func TestPageJS_EditProblem_ShowsOnItsOwningPanel(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		board string
		// owner is the beat select's value for the panel the edit is sent from, and elsewhere
		// another owner's, which must not show the problem.
		owner, elsewhere string
		edit             func(page *pageHarness)
		// status is the edit's answer: a refusal, or 200 with the problem standing on the owner.
		status int
		want   string
	}{
		{
			name:      "beat",
			board:     sessionStoryboard,
			owner:     "1",
			elsewhere: clipFrameEntry,
			edit:      func(page *pageHarness) { page.setField("hold (ms)", "value", holdPastDuration) },
			status:    http.StatusUnprocessableEntity,
			want:      "want less than duration",
		},
		{
			name:      "clip frame",
			board:     sessionStoryboard,
			owner:     clipFrameEntry,
			elsewhere: "1",
			edit:      func(page *pageHarness) { page.setField("fps", "value", fpsUnderOne) },
			status:    http.StatusUnprocessableEntity,
			want:      storyboard.FieldFrameFPS,
		},
		{
			name:      "zoom",
			board:     zoomedStoryboard,
			owner:     entryValueOf(storyboard.OwnerZoom, firstZoom),
			elsewhere: clipFrameEntry,
			edit:      func(page *pageHarness) { page.setField("to (×)", "value", zoomPastLargest) },
			status:    http.StatusUnprocessableEntity,
			want:      "to: want in",
		},
		{
			name:      "position",
			board:     positionedStoryboard,
			owner:     entryValueOf(storyboard.OwnerPosition, firstPosition),
			elsewhere: entryValueOf(storyboard.OwnerPosition, pointPosition),
			edit:      func(page *pageHarness) { page.setField("hold (ms)", "value", holdPastNext) },
			status:    http.StatusUnprocessableEntity,
			want:      fmt.Sprintf("positions[%d]", firstPosition),
		},
		{
			name:      "caption",
			board:     captionedStoryboard,
			owner:     entryValueOf(storyboard.OwnerCaption, styledCaption),
			elsewhere: clipFrameEntry,
			edit:      func(page *pageHarness) { page.setTextArea("text", captionTooWide) },
			status:    http.StatusOK,
			want:      fmt.Sprintf("captions[%d]", styledCaption),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			page := loadPage(t, newSessionFixtureOf(t, tc.board).session)
			page.selectValue(elementBeatSelect, tc.owner)

			tc.edit(page)

			if got := page.lastRequest("/api/edit"); got.status != tc.status {
				t.Fatalf("the edit: want %d, got %d: %s", tc.status, got.status, got.answer)
			}
			if !containsRow(page.errorRows(), tc.want) {
				t.Errorf("the %s's panel: want an error row containing %q, got %q", tc.name, tc.want, page.errorRows())
			}
			page.selectValue(elementBeatSelect, tc.elsewhere)
			page.settle()
			if containsRow(page.errorRows(), tc.want) {
				t.Errorf("the panel of %q: want no error row containing %q, got %q", tc.elsewhere, tc.want, page.errorRows())
			}
		})
	}
}

// containsRow says one of rows contains want.
func containsRow(rows []string, want string) bool {
	for _, row := range rows {
		if strings.Contains(row, want) {
			return true
		}
	}
	return false
}

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

// The elements the preview fit measures and the window it hears a resize on.
const (
	elementPreview       = "preview"
	elementTransport     = "transport"
	elementTimelinePanel = "timeline-panel"
	elementPickBox       = "pick-box"
	elementWindow        = "window"
)

// The page the fit test lays out: a preview column fitWidthPX wide showing a frame of fitWidthPX
// by fitHeightPX at its natural size, fitAbovePX under the page's top, fitBelowPX of the column
// under it, then the transport bar and the timeline, fitGapPX before, between and after them.
const (
	fitWidthPX     = 800
	fitHeightPX    = 450
	fitAbovePX     = 12
	fitBelowPX     = 40
	fitGapPX       = 12
	fitTransportPX = 36
	fitTimelinePX  = 200
	fitFloorPX     = 120
)

// layOut lays the element of id out at top, width wide and height tall, at the left edge.
func (p *pageHarness) layOut(id string, top, width, height int) {
	p.t.Helper()
	p.eval(fmt.Sprintf(`document.getElementById(%q).layout = { left: 0, top: %d, width: %d, height: %d };`,
		id, top, width, height))
}

// resizeWindow makes the window height tall, fires its resize and settles the page.
func (p *pageHarness) resizeWindow(height int) {
	p.t.Helper()
	p.eval(fmt.Sprintf("window.innerHeight = %d;", height))
	p.dispatch(elementWindow, "resize", nil)
	p.settle()
}

// styleOf is the style property of the element of id, "" while unset.
func (p *pageHarness) styleOf(id, property string) string {
	p.t.Helper()
	return p.eval(fmt.Sprintf(`document.getElementById(%q).style[%q] || ""`, id, property)).String()
}

func TestPageJS_FitPreview_ShrinksTheFrameToAShortWindow(t *testing.T) {
	t.Parallel()
	page := loadPage(t, newSessionFixtureOf(t, captionedStoryboard).session)
	page.eval(fmt.Sprintf(`{
		const frame = document.getElementById(%q);
		frame.naturalWidth = %d;
		frame.naturalHeight = %d;
	}`, elementFrame, fitWidthPX, fitHeightPX))
	transportTop := fitAbovePX + fitHeightPX + fitBelowPX + fitGapPX
	timelineTop := transportTop + fitTransportPX + fitGapPX
	page.layOut(elementFrame, fitAbovePX, fitWidthPX, fitHeightPX)
	page.layOut(elementPreview, fitAbovePX, fitWidthPX, fitHeightPX+fitBelowPX)
	page.layOut(elementTransport, transportTop, fitWidthPX, fitTransportPX)
	page.layOut(elementTimelinePanel, timelineTop, fitWidthPX, fitTimelinePX)
	// rest is the height everything but the frame takes.
	rest := fitAbovePX + fitBelowPX + 3*fitGapPX + fitTransportPX + fitTimelinePX

	// The cases run in order on one page, so each one re-fits after the one before.
	cases := []struct {
		name   string
		window int
		want   string
	}{
		{"a short window shrinks it to the room left", rest + 300, "300px"},
		{"a shorter window stops at the floor", rest + fitFloorPX/2, fmt.Sprintf("%dpx", fitFloorPX)},
		{"a tall window shows it width-fitted", rest + fitHeightPX, ""},
		{"a short window again shrinks it again", rest + 200, "200px"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page.resizeWindow(tc.window)

			if got := page.styleOf(elementFrame, "maxHeight"); got != tc.want {
				t.Errorf("the frame's max height in a %dpx window: want %q, got %q", tc.window, tc.want, got)
			}
		})
	}
}

func TestPageJS_FitPreview_KeepsThePickBoxOnTheShrunkFrame(t *testing.T) {
	t.Parallel()
	session := newSessionFixtureOf(t, captionedStoryboard).session
	page := loadPage(t, session)
	page.pickAcross(session, 0, 2, 4)
	var proposal struct {
		Rect pixelRect `json:"rect"`
	}
	if err := json.Unmarshal([]byte(page.lastRequest("/api/pick").answer), &proposal); err != nil {
		t.Fatalf("decode the pick's answer: %v", err)
	}
	naturalWidth := page.eval(fmt.Sprintf(`document.getElementById(%q).naturalWidth`, elementFrame)).ToInteger()
	naturalHeight := page.eval(fmt.Sprintf(`document.getElementById(%q).naturalHeight`, elementFrame)).ToInteger()

	// Each step shrinks the frame to 1/divisor of its natural size, as fit.js would: once on a
	// window resize, once on the frame's own resize with no window resize.
	steps := []struct {
		name    string
		divisor int64
		shrink  func(page *pageHarness)
	}{
		{"on a short window's resize", 2, func(page *pageHarness) { page.resizeWindow(fitFloorPX) }},
		{"on the frame's own resize", 4, func(page *pageHarness) {
			page.call("resized", elementFrame)
			page.settle()
		}},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			width, height := float64(naturalWidth)/float64(step.divisor), float64(naturalHeight)/float64(step.divisor)
			page.layOut(elementFrame, 0, int(width), int(height))
			// The layout above is whole pixels; the ratio the page draws by is the one it sees.
			width, height = float64(int(width)), float64(int(height))

			step.shrink(page)

			ratio := float64(naturalWidth) / width
			rect := proposal.Rect
			wantBox := map[string]float64{
				"left": float64(rect.X) / ratio, "top": float64(rect.Y) / ratio,
				"width": float64(rect.W) / ratio, "height": float64(rect.H) / ratio,
			}
			for property, want := range wantBox {
				if got := page.styleOf(elementPickBox, property); got != pixels(want) {
					t.Errorf("the pick box's %s: want %s, got %s", property, pixels(want), got)
				}
			}
			if page.isHidden(elementPickBox) {
				t.Error("the pick box is not shown on the shrunk frame")
			}
		})
	}
}

// pixels spells value as a CSS pixel length, as JavaScript writes the number.
func pixels(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64) + "px"
}

// The playback of sessionStoryboard, at 10 fps: the wall-clock time a play runs before it is
// read, landing on the frame at playedMS, the frame a step on from there shows, and the times a
// scrub across the axis, laid out one pixel per millisecond, presses at and lets go at.
const (
	playedMS       = 300
	steppedMS      = 400
	scrubPressMS   = 500
	scrubReleaseMS = 1000
)

// The keys of the transport the playback tests press.
const (
	keyPlay      = " "
	keyStepAhead = "ArrowRight"
	keyStepBack  = "ArrowLeft"
)

// The prefetch of frames.js while playing: the requests it keeps in flight and the frames, the
// playhead's first, it fetches ahead.
const (
	prefetchRequests = 4
	prefetchFrames   = 8
)

// elementFrameMessage is the note under the preview that tells why a frame is missing.
const elementFrameMessage = "frame-message"

// frameStepMS is the output time one frame of sessionStoryboard's 10 fps lasts.
const frameStepMS = 100

// exactFrame is the path of the request for the exact frame at ms.
func exactFrame(ms int) string {
	return frameAPIPrefix + "t=" + strconv.Itoa(ms)
}

// draftFrame is the path of the request for the draft frame at ms.
func draftFrame(ms int) string {
	return exactFrame(ms) + "&draft=1"
}

// framesAfter are the paths of the preview frame requests the page sent after its first mark
// requests, in order.
func (p *pageHarness) framesAfter(mark int) []string {
	p.t.Helper()
	var frames []string
	for _, request := range p.requests[mark:] {
		if strings.HasPrefix(request.path, frameAPIPrefix) {
			frames = append(frames, request.path)
		}
	}
	return frames
}

// draftFramesFrom are the paths of the draft frame requests for count frames from the one at ms.
func draftFramesFrom(ms, count int) []string {
	frames := make([]string, 0, count)
	for i := range count {
		frames = append(frames, draftFrame(ms+i*frameStepMS))
	}
	return frames
}

// holdFrames holds the answers of the preview frame requests from now on.
func (p *pageHarness) holdFrames() {
	p.t.Helper()
	p.call("holdFetches", frameAPIPrefix)
}

// releaseFrames delivers every held frame answer and settles the page.
func (p *pageHarness) releaseFrames() {
	p.t.Helper()
	p.call("releaseFetches")
	p.settle()
}

// heldFrames is how many frame requests have answers held.
func (p *pageHarness) heldFrames() int64 {
	p.t.Helper()
	return p.call("heldFetches").ToInteger()
}

// assertFrameTime fails the test unless the preview shows the frame at ms.
func (p *pageHarness) assertFrameTime(ms int) {
	p.t.Helper()
	if got := p.eval("window.tune.frameTime()").Export(); got != int64(ms) {
		p.t.Errorf("the frame shown: want the one at %dms, got %v", ms, got)
	}
}

// pressKey fires a keydown of key on the document, as the transport hears it, without settling.
func (p *pageHarness) pressKey(key string) {
	p.t.Helper()
	p.dispatch(elementDocument, "keydown", map[string]any{"key": key})
}

// playFor starts a play, lets ms of wall-clock time pass and settles the page.
func (p *pageHarness) playFor(ms int) {
	p.t.Helper()
	p.pressKey(keyPlay)
	p.call("advance", ms)
	p.settle()
}

// scrubAxis fires a pointer event of type at output time ms on the timeline's axis, laid out one
// pixel per millisecond, and settles the page.
func (p *pageHarness) scrubAxis(eventType string, ms int) {
	p.t.Helper()
	found := p.eval(fmt.Sprintf(`{
		const track = document.getElementById(%q).querySelectorAll(".axis")[0];
		if (track) {
			track.layout = { left: 0, top: 0, width: window.tune.state().length_ms, height: 20 };
			track.dispatchEvent(new Event(%q, { clientX: %d }));
		}
		Boolean(track);
	}`, elementTimeline, eventType, ms)).ToBoolean()
	if !found {
		p.t.Fatal("the timeline has no axis")
	}
	p.settle()
}

// assertFrames fails the test unless the page's frame requests after its first mark requests
// are want, in order.
func (p *pageHarness) assertFrames(mark int, want ...string) {
	p.t.Helper()
	if got := p.framesAfter(mark); !reflect.DeepEqual(got, want) {
		p.t.Errorf("the frames requested: want %q, got %q", want, got)
	}
}

func TestPageJS_Play_RequestsDraftFrames(t *testing.T) {
	t.Parallel()
	page := loadPage(t, newSessionFixture(t).session)
	mark := len(page.requests)

	page.playFor(playedMS)

	page.assertFrames(mark, draftFramesFrom(playedMS, prefetchFrames)...)
}

func TestPageJS_Pause_RequestsTheExactFrameAtThePlayhead(t *testing.T) {
	t.Parallel()
	page := loadPage(t, newSessionFixture(t).session)
	page.playFor(playedMS)
	mark := len(page.requests)

	page.pressKey(keyPlay)
	page.settle()

	page.assertFrames(mark, exactFrame(playedMS))
}

func TestPageJS_FrameStep_RequestsADraftThenTheExactFrame(t *testing.T) {
	t.Parallel()
	page := loadPage(t, newSessionFixture(t).session)
	page.eval(fmt.Sprintf("window.tune.seek(%d);", playedMS))
	page.settle()
	mark := len(page.requests)

	page.pressKey(keyStepAhead)
	page.settle()

	page.assertFrames(mark, draftFrame(steppedMS), exactFrame(steppedMS))
}

func TestPageJS_Scrub_RequestsDraftsAndItsReleaseTheExactFrame(t *testing.T) {
	t.Parallel()
	page := loadPage(t, newSessionFixture(t).session)
	mark := len(page.requests)

	page.scrubAxis("pointerdown", scrubPressMS)
	page.scrubAxis("pointermove", scrubReleaseMS)
	scrubbed := len(page.requests)
	page.scrubAxis("pointerup", scrubReleaseMS)

	page.assertFrames(mark, draftFrame(scrubPressMS), draftFrame(scrubReleaseMS), exactFrame(scrubReleaseMS))
	page.assertFrames(scrubbed, exactFrame(scrubReleaseMS))
}

func TestPageJS_Scrub_ReleasedOffARebuiltAxisSettlesOnTheExactFrame(t *testing.T) {
	t.Parallel()
	page := loadPage(t, newSessionFixture(t).session)
	page.scrubAxis("pointerdown", scrubPressMS)
	page.call("resized", elementTimeline)
	page.settle()
	mark := len(page.requests)

	page.scrubAxis("pointermove", scrubReleaseMS)
	page.dispatch(elementWindow, "pointerup", nil)
	page.settle()
	page.playFor(playedMS)
	page.pressKey(keyPlay)
	page.settle()

	// The release settles on the exact frame, and so does the pause after a later play.
	want := []string{draftFrame(scrubReleaseMS), exactFrame(scrubReleaseMS)}
	want = append(want, draftFramesFrom(scrubReleaseMS+playedMS, prefetchFrames)...)
	page.assertFrames(mark, append(want, exactFrame(scrubReleaseMS+playedMS))...)
}

func TestPageJS_StepWhilePlaying_RequestsNoFrameAtTheStalePlayhead(t *testing.T) {
	t.Parallel()
	page := loadPage(t, newSessionFixture(t).session)
	page.playFor(playedMS)
	mark := len(page.requests)

	page.pressKey(keyStepAhead)
	page.settle()

	// The play fetched the draft at steppedMS ahead, so the step shows it without a request.
	page.assertFrames(mark, exactFrame(steppedMS))
	page.assertFrameTime(steppedMS)
}

func TestPageJS_EditWhilePaused_RequestsTheExactFrameOnly(t *testing.T) {
	t.Parallel()
	page := loadPage(t, newSessionFixture(t).session)
	page.eval(fmt.Sprintf("window.tune.seek(%d);", playedMS))
	page.settle()
	mark := len(page.requests)

	page.eval(fmt.Sprintf(`window.tune.edit({owner: {kind: %q, index: 1}, field: %q, value: %d});`,
		storyboard.OwnerBeat, storyboard.FieldDuration, draggedDurationMS))
	page.settle()

	page.assertEdit(fmt.Sprintf(`{"owner":{"kind":%q,"index":1},"field":%q,"value":%d}`,
		storyboard.OwnerBeat, storyboard.FieldDuration, draggedDurationMS))
	page.assertFrames(mark, exactFrame(playedMS))
}

func TestPageJS_Play_KeepsFourDraftRequestsInFlightAhead(t *testing.T) {
	t.Parallel()
	page := loadPage(t, newSessionFixture(t).session)
	page.holdFrames()
	mark := len(page.requests)

	page.playFor(playedMS)

	page.assertFrames(mark, draftFramesFrom(playedMS, prefetchRequests)...)
	if got := page.heldFrames(); got != prefetchRequests {
		t.Errorf("the frame requests in flight: want %d, got %d", prefetchRequests, got)
	}
}

func TestPageJS_Play_ShowsACachedFrameWithoutARequest(t *testing.T) {
	t.Parallel()
	page := loadPage(t, newSessionFixture(t).session)
	page.holdFrames()
	page.playFor(playedMS)
	page.releaseFrames()
	mark := len(page.requests)

	page.call("advance", frameStepMS)
	page.settle()

	page.assertFrames(mark)
	page.assertFrameTime(playedMS + frameStepMS)
}

func TestPageJS_EditWhilePlaying_ShowsNoFrameOfTheOldVersion(t *testing.T) {
	t.Parallel()
	page := loadPage(t, newSessionFixture(t).session)
	page.holdFrames()
	page.playFor(playedMS)

	page.eval(fmt.Sprintf(`window.tune.edit({owner: {kind: %q, index: 1}, field: %q, value: %d});`,
		storyboard.OwnerBeat, storyboard.FieldDuration, draggedDurationMS))
	page.settle()
	page.releaseFrames()

	page.assertFrameTime(0)
	if got := page.heldFrames(); got != prefetchRequests {
		t.Errorf("the frame requests of the new version in flight: want %d, got %d", prefetchRequests, got)
	}
	page.releaseFrames()
	page.assertFrameTime(playedMS)
}

func TestPageJS_Paused_HasAtMostOneFrameRequestOpen(t *testing.T) {
	t.Parallel()
	page := loadPage(t, newSessionFixture(t).session)
	page.holdFrames()
	page.playFor(playedMS)
	mark := len(page.requests)

	page.pressKey(keyPlay)
	page.settle()
	page.pressKey(keyStepAhead)
	page.settle()

	page.assertFrames(mark, exactFrame(playedMS))
	page.releaseFrames()
	page.assertFrames(mark, exactFrame(playedMS), exactFrame(steppedMS))
	if got := page.heldFrames(); got != 1 {
		t.Errorf("the frame requests in flight while paused: want 1, got %d", got)
	}
	page.releaseFrames()
	page.assertFrameTime(steppedMS)
}

func TestPageJS_StepBackWhilePaused_ShowsTheCachedFrame(t *testing.T) {
	t.Parallel()
	page := loadPage(t, newSessionFixture(t).session)
	page.eval(fmt.Sprintf("window.tune.seek(%d);", playedMS))
	page.settle()
	page.pressKey(keyStepAhead)
	page.settle()
	mark := len(page.requests)

	page.pressKey(keyStepBack)
	page.settle()

	page.assertFrames(mark)
	page.assertFrameTime(playedMS)
	if !page.isHidden(elementFrameMessage) {
		t.Errorf("the frame message after the step back: want hidden, got %q", page.text(elementFrameMessage))
	}
}
