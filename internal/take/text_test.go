package take

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// cellsOf is a row of one-column cells, one per grapheme of text.
func cellsOf(text string) []Cell {
	row := make([]Cell, 0, len(text))
	for _, r := range text {
		row = append(row, Cell{Grapheme: string(r), Width: 1})
	}
	return row
}

func TestSnapshot_Text_DerivesOneRunPerRowFromCells(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		row  []Cell
		want TextRun
	}{
		{
			name: "plain text",
			row:  cellsOf("ab"),
			want: TextRun{Text: "ab", Box: Box{W: 2, H: 1}, Edges: []int{0, 1, 2}},
		},
		{
			name: "wide grapheme counts once and covers two columns",
			row: []Cell{
				{Grapheme: "a", Width: 1}, {Grapheme: "漢", Width: 2}, {Width: 0}, {Grapheme: "b", Width: 1},
			},
			want: TextRun{Text: "a漢b", Box: Box{W: 4, H: 1}, Edges: []int{0, 1, 1, 1, 3, 4}},
		},
		{
			name: "combining mark stays with its base",
			row:  []Cell{{Grapheme: combiningGrapheme, Width: 1}, {Grapheme: "x", Width: 1}},
			want: TextRun{Text: combiningGrapheme + "x", Box: Box{W: 2, H: 1}, Edges: []int{0, 0, 0, 1, 2}},
		},
		{
			name: "trailing blanks trimmed, the run still spans the row",
			row:  append(cellsOf("ab  "), Cell{Width: 1}),
			want: TextRun{Text: "ab", Box: Box{W: 5, H: 1}, Edges: []int{0, 1, 2}},
		},
		{
			name: "empty grapheme reads as a space",
			row:  []Cell{{Grapheme: "a", Width: 1}, {Width: 1}, {Grapheme: "b", Width: 1}},
			want: TextRun{Text: "a b", Box: Box{W: 3, H: 1}, Edges: []int{0, 1, 2, 3}},
		},
		{
			name: "blank row",
			row:  cellsOf("   "),
			want: TextRun{Text: "", Box: Box{W: 3, H: 1}, Edges: []int{0}},
		},
		{
			name: "row of no cells",
			row:  []Cell{},
			want: TextRun{Text: "", Box: Box{W: 0, H: 1}, Edges: []int{0}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			const y = 1
			screen := Snapshot{Cells: [][]Cell{cellsOf("top"), c.row}}
			want := c.want
			want.Box.Y = y

			runs := screen.Text()

			if len(runs) != 2 {
				t.Fatalf("Text() = %d runs, want one per row (2)", len(runs))
			}
			if !reflect.DeepEqual(runs[y], want) {
				t.Fatalf("Text()[%d] = %+v, want %+v", y, runs[y], want)
			}
		})
	}
}

func TestSnapshot_Text_KeepsEveryRowInOrder(t *testing.T) {
	t.Parallel()
	screen := Snapshot{Cells: [][]Cell{cellsOf("a"), cellsOf("  "), cellsOf("b")}}

	runs := screen.Text()

	texts := make([]string, len(runs))
	for i, run := range runs {
		texts[i] = run.Text
		if run.Box.Y != i {
			t.Errorf("run %d Box.Y = %d, want %d", i, run.Box.Y, i)
		}
	}
	if got := strings.Join(texts, "\n"); got != "a\n\nb" {
		t.Fatalf("Text() rows = %q, want %q", got, "a\n\nb")
	}
}

func TestSnapshot_Text_ReturnsStoredRunsWithoutCells(t *testing.T) {
	t.Parallel()
	runs := []TextRun{
		{Text: "Sign in", Box: Box{X: 10, Y: 20, W: 49, H: 18}, Edges: []int{10, 17, 24, 31, 38, 45, 52, 59}},
		{Text: "Help", Box: Box{X: 80, Y: 20, W: 28, H: 18}, Edges: []int{80, 87, 94, 101, 108}},
	}
	screen := Snapshot{Runs: runs}

	got := screen.Text()

	if !reflect.DeepEqual(got, runs) {
		t.Fatalf("Text() = %+v, want the stored runs %+v", got, runs)
	}
}

func TestSnapshot_Text_PrefersCellsOverRuns(t *testing.T) {
	t.Parallel()
	screen := Snapshot{
		Cells: [][]Cell{cellsOf("cell")},
		Runs:  []TextRun{{Text: "run", Box: Box{W: 3, H: 1}, Edges: []int{0, 1, 2, 3}}},
	}

	got := screen.Text()

	if len(got) != 1 || got[0].Text != "cell" {
		t.Fatalf("Text() = %+v, want the run derived from the cells", got)
	}
}

func TestTextRun_Span(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		run  TextRun
		a, b int
		want Box
	}{
		{
			name: "terminal row, a wide grapheme",
			run:  TextRun{Text: "a漢b", Box: Box{Y: 4, W: 6, H: 1}, Edges: []int{0, 1, 1, 1, 3, 4}},
			a:    1, b: 4,
			want: Box{X: 1, Y: 4, W: 2, H: 1},
		},
		{
			name: "browser run off character cells",
			run:  TextRun{Text: "Sign", Box: Box{X: 10, Y: 20, W: 29, H: 18}, Edges: []int{10, 17, 21, 25, 39}},
			a:    1, b: 3,
			want: Box{X: 17, Y: 20, W: 8, H: 18},
		},
		{
			name: "empty span",
			run:  TextRun{Text: "ab", Box: Box{Y: 2, W: 2, H: 1}, Edges: []int{0, 1, 2}},
			a:    1, b: 1,
			want: Box{X: 1, Y: 2, W: 0, H: 1},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := c.run.Span(c.a, c.b)

			if got != c.want {
				t.Fatalf("Span(%d, %d) = %+v, want %+v", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestSnapshot_Bounds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		screen Snapshot
		want   Box
	}{
		{
			name:   "cell grid",
			screen: Snapshot{Cells: [][]Cell{cellsOf("abc"), cellsOf("def")}},
			want:   Box{W: 3, H: 2},
		},
		{
			name:   "ragged rows span the longest",
			screen: Snapshot{Cells: [][]Cell{cellsOf("ab"), cellsOf("abcde"), {}}},
			want:   Box{W: 5, H: 3},
		},
		{
			name: "run-only snapshot spans the union of its runs",
			screen: Snapshot{Runs: []TextRun{
				{Text: "a", Box: Box{X: 10, Y: 20, W: 30, H: 18}, Edges: []int{10, 40}},
				{Text: "b", Box: Box{X: 5, Y: 50, W: 10, H: 18}, Edges: []int{5, 15}},
			}},
			want: Box{X: 5, Y: 20, W: 35, H: 48},
		},
		{
			name:   "nothing on screen",
			screen: Snapshot{},
			want:   Box{},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := c.screen.Bounds()

			if got != c.want {
				t.Fatalf("Bounds() = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestFile_RunsAndSelectorsRoundTrip(t *testing.T) {
	t.Parallel()
	want := sampleTake()
	want.Snapshots = append(want.Snapshots, Snapshot{
		At: want.Snapshots[len(want.Snapshots)-1].At + 1,
		Runs: []TextRun{
			{Text: "Sign in", Box: Box{X: 10, Y: 20, W: 49, H: 18}, Edges: []int{10, 17, 24, 31, 38, 45, 52, 59}},
		},
		Selectors: map[string][]Box{
			"#login":    {{X: 8, Y: 18, W: 60, H: 22}},
			"li.result": {{X: 0, Y: 100, W: 200, H: 20}, {X: 0, Y: 120, W: 200, H: 20}},
		},
	})
	path := filepath.Join(t.TempDir(), "runs.take")

	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)

	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip changed the take:\n got %+v\nwant %+v", got, want)
	}
}

func TestFile_TerminalTakeCarriesNoTextLayerFields(t *testing.T) {
	t.Parallel()

	lines := takeLines(t, sampleTake())

	for i, line := range lines {
		if strings.Contains(line, `"runs"`) || strings.Contains(line, `"sel"`) {
			t.Errorf("line %d = %s, want neither runs nor sel on a terminal take", i+1, line)
		}
	}
}

func TestLoad_VersionOneTakeReadsWithoutTextLayerFields(t *testing.T) {
	t.Parallel()
	data := gzipLines(t,
		`{"format":"photon-take","version":1,"cols":1,"rows":1}`,
		`{"snap":{"at":0,"cursor":{"x":0,"y":0,"visible":false},"cells":[[{"g":"a","w":1}]]}}`,
	)
	path := filepath.Join(t.TempDir(), "v1.take")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write the fixture: %v", err)
	}

	got, err := Load(path)

	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	screen := got.Snapshots[0]
	if screen.Runs != nil || screen.Selectors != nil {
		t.Fatalf("snapshot = %+v, want neither Runs nor Selectors", screen)
	}
	if runs := screen.Text(); len(runs) != 1 || runs[0].Text != "a" {
		t.Fatalf("Text() = %+v, want the one row %q", runs, "a")
	}
}
