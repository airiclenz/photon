package tune

import (
	"encoding/json"
	"errors"
	"image"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/render"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

// pickConfig declares the areas the pick tests search: list is rows 0–3, upper and pair are
// both rows 0–1, and status is row 4 alone.
const pickConfig = `name: picked
launch:
  command: ./app
areas:
  list:
    top: {match: '^Items'}
    bottom: {match: '^--'}
  upper:
    top: {match: '^Items'}
    bottom: {match: '^  one'}
  pair:
    top: {match: '^Items'}
    bottom: {match: '^Items', offset: 1}
  status:
    top: {match: '^a\+b'}
    bottom: {match: '^a\+b'}
`

// pickStoryboard is one 2s beat over pickScreen, zoomed onto its first row.
const pickStoryboard = `clip: picked
ship: out/picked.gif
frame: {cols: 30, rows: 9, padding: 4, font_size: 10, line_height: 1.2, scale: 2, width: 600, fps: 10, max_colors: 16}
beats:
  - id: 1
    title: list
    duration: 2s
    zoom: {target: {text: 'Items'}}
`

// pickScreen is what the pick take shows, one line per row.
var pickScreen = []string{
	"Items",
	"  one  beta",
	"  two  beta",
	"-- footer",
	"a+b (x) here",
	"unique words",
	"beta",
	"here",
	"aaa",
}

// The pick take's frame, in cells.
const (
	pickCols = 30
	pickRows = 9
)

// newPickSession starts a session on pickConfig, pickStoryboard and a take showing pickScreen.
func newPickSession(t *testing.T) *Session {
	t.Helper()
	project := t.TempDir()
	writeTestFile(t, filepath.Join(project, "photon.yaml"), pickConfig)
	boardPath := filepath.Join(project, "picked.yaml")
	writeTestFile(t, boardPath, pickStoryboard)
	takePath := filepath.Join(t.TempDir(), "picked.take")
	recorded := &take.Take{
		Cols: pickCols, Rows: pickRows, FPS: 10,
		Snapshots: []take.Snapshot{lineSnapshot(0, pickScreen...), lineSnapshot(2*time.Second, pickScreen...)},
		Events:    []take.Event{{At: 0, Kind: take.EventBeatStart, Beat: 1}},
	}
	if err := take.Save(takePath, recorded); err != nil {
		t.Fatalf("take.Save: %v", err)
	}
	loaded, err := clip.Load(boardPath, "", t.TempDir())
	if err != nil {
		t.Fatalf("clip.Load: %v", err)
	}
	session, err := NewSession(loaded, takePath)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	return session
}

// cellPixel is the output-frame pixel at the centre of the cell at col, row, at output time 0.
func cellPixel(t *testing.T, session *Session, col, row int) image.Point {
	t.Helper()
	rect := session.goodPipeline().frames.Rect(0, take.Box{X: col, Y: row, W: 1, H: 1})
	if rect.Empty() {
		t.Fatalf("cell %d,%d is not on the frame", col, row)
	}
	return rect.Min.Add(rect.Max).Div(2)
}

// decodeSnippet decodes a proposal's snippet into into, failing the test when it is not YAML of
// that shape.
func decodeSnippet(t *testing.T, snippet string, into any) {
	t.Helper()
	if err := yaml.Unmarshal([]byte(snippet), into); err != nil {
		t.Fatalf("decode snippet %q: %v", snippet, err)
	}
}

// assertSnippetsWrite checks that every snippet of proposal decodes to its target in the place
// it is written for.
func assertSnippetsWrite(t *testing.T, proposal Proposal) {
	t.Helper()
	want := proposal.Target
	var zoomed storyboard.Target
	decodeSnippet(t, proposal.Snippets[snippetZoom], &zoomed)
	var clicked storyboard.Action
	decodeSnippet(t, proposal.Snippets[snippetClick], &clicked)
	var seen storyboard.Expect
	decodeSnippet(t, proposal.Snippets[snippetSeen], &seen)
	var from, to storyboard.DragAction
	decodeSnippet(t, "{"+proposal.Snippets[snippetDragFrom]+"}", &from)
	decodeSnippet(t, "{"+proposal.Snippets[snippetDragTo]+"}", &to)

	if zoomed != want {
		t.Errorf("zoom snippet %q: want %+v, got %+v", proposal.Snippets[snippetZoom], want, zoomed)
	}
	if clicked.Click == nil || clicked.Click.Target != want {
		t.Errorf("click snippet %q: want a click on %+v, got %+v", proposal.Snippets[snippetClick], want, clicked.Click)
	}
	if seen.Seen != want.Text || seen.Area != want.Area {
		t.Errorf("seen snippet %q: want %q in %q, got %+v", proposal.Snippets[snippetSeen], want.Text, want.Area, seen)
	}
	if from.From != want || to.To.Target != want {
		t.Errorf("drag snippets %q, %q: want both ends on %+v, got %+v and %+v",
			proposal.Snippets[snippetDragFrom], proposal.Snippets[snippetDragTo], want, from.From, to.To.Target)
	}
}

func TestSession_Pick_ProposesAResolverCheckedTarget(t *testing.T) {
	t.Parallel()
	session := newPickSession(t)
	tests := []struct {
		name     string
		row      int
		fromCol  int
		toCol    int
		want     storyboard.Target
		wantCols [2]int
	}{
		{name: "unique text", row: 5, fromCol: 0, toCol: 5,
			want: storyboard.Target{Text: "unique"}, wantCols: [2]int{0, 6}},
		{name: "text repeated on the screen", row: 6, fromCol: 0, toCol: 3,
			want: storyboard.Target{Text: "beta", Nth: 3}, wantCols: [2]int{0, 4}},
		{name: "repeated text unique in an area", row: 4, fromCol: 8, toCol: 11,
			want: storyboard.Target{Text: "here", Area: "status"}, wantCols: [2]int{8, 4}},
		{name: "regex metacharacters escaped", row: 4, fromCol: 0, toCol: 6,
			want: storyboard.Target{Text: `a\+b \(x\)`, Area: "status"}, wantCols: [2]int{0, 7}},
		{name: "repeated in its smallest area", row: 2, fromCol: 7, toCol: 10,
			want: storyboard.Target{Text: "beta", Nth: 2, Area: "list"}, wantCols: [2]int{7, 4}},
		{name: "two equal-height areas, name order", row: 1, fromCol: 7, toCol: 10,
			want: storyboard.Target{Text: "beta", Area: "pair"}, wantCols: [2]int{7, 4}},
		{name: "edge blanks dropped", row: 1, fromCol: 5, toCol: 14,
			want: storyboard.Target{Text: "beta", Area: "pair"}, wantCols: [2]int{7, 4}},
		{name: "picked right to left", row: 1, fromCol: 10, toCol: 7,
			want: storyboard.Target{Text: "beta", Area: "pair"}, wantCols: [2]int{7, 4}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			from := cellPixel(t, session, test.fromCol, test.row)
			to := cellPixel(t, session, test.toCol, test.row)

			proposal, err := session.Pick(0, from, to)

			if err != nil {
				t.Fatalf("Pick: %v", err)
			}
			if proposal.Target != test.want {
				t.Errorf("target: want %+v, got %+v", test.want, proposal.Target)
			}
			wantBox := take.Box{X: test.wantCols[0], Y: test.row, W: test.wantCols[1], H: 1}
			if !covers(proposal.Box, wantBox) {
				t.Errorf("box %+v does not cover the picked cells %+v", proposal.Box, wantBox)
			}
			if inner := cellPixel(t, session, wantBox.X, test.row); !inner.In(proposal.Rect) {
				t.Errorf("rect %v does not hold the first picked cell's pixel %v", proposal.Rect, inner)
			}
			assertSnippetsWrite(t, proposal)
		})
	}
}

func TestSession_Pick_WritesTheSnippets(t *testing.T) {
	t.Parallel()
	session := newPickSession(t)

	proposal, err := session.Pick(0, cellPixel(t, session, 7, 2), cellPixel(t, session, 10, 2))

	if err != nil {
		t.Fatalf("Pick: %v", err)
	}
	flow := "{text: 'beta', nth: 2, area: 'list'}"
	want := map[string]string{
		snippetZoom:     flow,
		snippetClick:    "click: {target: " + flow + "}",
		snippetSeen:     "{seen: 'beta', area: 'list'}",
		snippetDragFrom: "from: " + flow,
		snippetDragTo:   "to: " + flow,
	}
	for key, snippet := range want {
		if proposal.Snippets[key] != snippet {
			t.Errorf("snippet %s: want %q, got %q", key, snippet, proposal.Snippets[key])
		}
	}
	if len(proposal.Snippets) != len(want) {
		t.Errorf("snippets: want exactly %d keys, got %v", len(want), proposal.Snippets)
	}
}

func TestSession_Pick_NoTarget_IsRefused(t *testing.T) {
	t.Parallel()
	session := newPickSession(t)
	tests := []struct {
		name     string
		out      time.Duration
		from, to func() image.Point
		want     error
	}{
		{name: "a blank pick", out: 0,
			from: func() image.Point { return cellPixel(t, session, 0, 1) },
			to:   func() image.Point { return cellPixel(t, session, 1, 1) },
			want: ErrNoProposal},
		{name: "a pick no match covers", out: 0,
			from: func() image.Point { return cellPixel(t, session, 1, 8) },
			to:   func() image.Point { return cellPixel(t, session, 2, 8) },
			want: ErrNoProposal},
		{name: "a pick starting on the padding", out: 0,
			from: func() image.Point { return image.Pt(0, 0) },
			to:   func() image.Point { return cellPixel(t, session, 3, 0) },
			want: ErrNoProposal},
		{name: "a time outside the clip", out: 2 * time.Second,
			from: func() image.Point { return cellPixel(t, session, 0, 5) },
			to:   func() image.Point { return cellPixel(t, session, 5, 5) },
			want: render.ErrOutsideClip},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := session.Pick(test.out, test.from(), test.to())

			if !errors.Is(err, test.want) {
				t.Errorf("Pick: want %v, got %v", test.want, err)
			}
		})
	}
}

func TestSession_Pick_ProposedTargetIsAZoomTargetEdit(t *testing.T) {
	t.Parallel()
	session := newPickSession(t)
	proposal, err := session.Pick(0, cellPixel(t, session, 0, 4), cellPixel(t, session, 6, 4))
	if err != nil {
		t.Fatalf("Pick: %v", err)
	}

	err = session.Apply(beatEdit(1, fieldZoomTarget, responseOf(proposal).Target))

	if err != nil {
		t.Fatalf("Apply the proposed target: %v", err)
	}
	state := session.State()
	if len(state.Errors) != 0 {
		t.Errorf("errors after the edit: %+v", state.Errors)
	}
	want := TargetState{Text: proposal.Target.Text, Nth: int(proposal.Target.Nth), Area: proposal.Target.Area}
	if zoom := state.Beats[0].Zoom; zoom == nil || zoom.Target != want {
		t.Errorf("beat 1's zoom: want the target %+v, got %+v", want, zoom)
	}
}

func TestSession_LastMatchTarget_RoundTripsThroughTheProposalAndItsApply(t *testing.T) {
	t.Parallel()
	session := newPickSession(t)
	last := storyboard.Target{Text: "beta", Nth: storyboard.NthLast}
	response := responseOf(Proposal{Target: last, Snippets: snippetsOf(last)})

	encoded, encodeErr := json.Marshal(response.Target)
	applyErr := session.Apply(beatEdit(1, fieldZoomTarget, json.RawMessage(encoded)))

	if encodeErr != nil {
		t.Fatalf("encode the proposed target: %v", encodeErr)
	}
	if want := `{"text":"beta","nth":"last"}`; string(encoded) != want {
		t.Errorf("the proposed target on the wire: want %s, got %s", want, encoded)
	}
	if want := "{text: 'beta', nth: last}"; response.Snippets[snippetZoom] != want {
		t.Errorf("the zoom snippet: want %q, got %q", want, response.Snippets[snippetZoom])
	}
	if applyErr != nil {
		t.Fatalf("Apply the proposed target: %v", applyErr)
	}
	want := TargetState{Text: "beta", Nth: int(storyboard.NthLast)}
	if zoom := session.State().Beats[0].Zoom; zoom == nil || zoom.Target != want {
		t.Errorf("beat 1's zoom: want the target %+v, got %+v", want, zoom)
	}
}
