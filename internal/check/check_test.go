package check

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/config"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
)

// footerArea is the fixture config's area: the rows below a `---` rule.
const footerArea = "footer"

// fixtureTakePath is the take path the judging tests hand Judge; nothing reads it.
const fixtureTakePath = "fixture.take"

// screen is a snapshot at at whose rows read lines, one cell per rune.
func screen(at time.Duration, lines ...string) take.Snapshot {
	cells := make([][]take.Cell, 0, len(lines))
	for _, line := range lines {
		row := make([]take.Cell, 0, len(line))
		for _, r := range line {
			row = append(row, take.Cell{Grapheme: string(r), Width: 1})
		}
		cells = append(cells, row)
	}
	return take.Snapshot{At: at, Cells: cells}
}

// fixtureTake is a take whose beats start at 0, 2s and 4s. Its screens: `photon ready` at the
// start; `hello world` above a footer from 1s, still showing when beat 2 starts; `saved hello`
// above a footer reading `mode view` from 2.5s, still showing when beat 3 starts; `bye`, with no
// footer, from 5s.
func fixtureTake() *take.Take {
	return &take.Take{
		Cols: 20, Rows: 3, FPS: 24,
		Snapshots: []take.Snapshot{
			screen(0, "photon ready"),
			screen(time.Second, "hello world", "---", "mode edit"),
			screen(2500*time.Millisecond, "saved hello", "---", "mode view"),
			screen(5*time.Second, "bye"),
		},
		Events: []take.Event{
			{At: 0, Kind: take.EventBeatStart, Beat: 1},
			{At: 2 * time.Second, Kind: take.EventBeatStart, Beat: 2},
			{At: 4 * time.Second, Kind: take.EventBeatStart, Beat: 3},
		},
	}
}

// fixtureBoard is a storyboard of the fixture take's three beats, with no expect.
func fixtureBoard() *storyboard.Storyboard {
	return &storyboard.Storyboard{
		Clip:  "demo",
		Beats: []storyboard.Beat{{ID: 1}, {ID: 2}, {ID: 3}},
	}
}

// fixtureResolver knows footerArea.
func fixtureResolver(t *testing.T) target.Resolver {
	t.Helper()
	resolver, err := target.New(map[string]config.Area{
		footerArea: {Top: &config.Anchor{Match: `^---$`, Offset: 1}},
	})
	if err != nil {
		t.Fatalf("target.New: %v", err)
	}
	return resolver
}

// judge judges board against the fixture take with work as the work dir, failing t on an error.
func judge(t *testing.T, board *storyboard.Storyboard, work, takePath string, options Options) []Row {
	t.Helper()
	rows, err := Judge(context.Background(), board, fixtureTake(), fixtureResolver(t), work, takePath, options)
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	return rows
}

// onlyRow fails t unless rows holds exactly one row, and returns it.
func onlyRow(t *testing.T, rows []Row) Row {
	t.Helper()
	if len(rows) != 1 {
		t.Fatalf("want one row, got %+v", rows)
	}
	return rows[0]
}

func TestJudge_Seen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		beat        int
		expect      storyboard.Expect
		wantVerdict Verdict
		wantDetail  string
	}{
		{
			name:        "a screen inside the beat",
			beat:        2,
			expect:      storyboard.Expect{Seen: `saved`},
			wantVerdict: Pass,
			wantDetail:  "seen /saved/ at 2.5s",
		},
		{
			name:        "the screen showing when the beat starts",
			beat:        2,
			expect:      storyboard.Expect{Seen: `hello world`},
			wantVerdict: Pass,
			wantDetail:  "seen /hello world/ at 1s",
		},
		{
			name:        "only outside the beat",
			beat:        3,
			expect:      storyboard.Expect{Seen: `ready`},
			wantVerdict: Fail,
			wantDetail:  "not seen /ready/ on any of the beat's 2 screen(s)",
		},
		{
			name:        "inside the area",
			beat:        2,
			expect:      storyboard.Expect{Seen: `mode view`, Area: footerArea},
			wantVerdict: Pass,
			wantDetail:  "seen /mode view/ in area footer at 2.5s",
		},
		{
			name:        "on screen but outside the area",
			beat:        2,
			expect:      storyboard.Expect{Seen: `saved`, Area: footerArea},
			wantVerdict: Fail,
			wantDetail:  "not seen /saved/ in area footer on any of the beat's 2 screen(s)",
		},
		{
			name:        "a screen without the area does not match",
			beat:        3,
			expect:      storyboard.Expect{Seen: `bye`, Area: footerArea},
			wantVerdict: Fail,
			wantDetail:  "not seen /bye/ in area footer",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board := fixtureBoard()
			board.Beats[tt.beat-1].Expect = []storyboard.Expect{tt.expect}

			row := onlyRow(t, judge(t, board, t.TempDir(), fixtureTakePath, Options{}))

			if row.Subject != beatSubject(tt.beat) || row.Verdict != tt.wantVerdict || !strings.Contains(row.Detail, tt.wantDetail) {
				t.Errorf("row = %+v, want beat %d %s containing %q", row, tt.beat, tt.wantVerdict, tt.wantDetail)
			}
		})
	}
}

func TestJudge_TakeLevelSeenSearchesEveryScreen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		seen        string
		wantVerdict Verdict
		wantDetail  string
	}{
		{name: "the first screen", seen: `ready`, wantVerdict: Pass, wantDetail: "seen /ready/ at 0s"},
		{name: "the last screen", seen: `bye`, wantVerdict: Pass, wantDetail: "seen /bye/ at 5s"},
		{name: "no screen", seen: `absent`, wantVerdict: Fail, wantDetail: "on any of the take's 4 screen(s)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board := fixtureBoard()
			board.Expect = []storyboard.Expect{{Seen: tt.seen}}

			row := onlyRow(t, judge(t, board, t.TempDir(), fixtureTakePath, Options{}))

			if row.Subject != takeSubject || row.Verdict != tt.wantVerdict || !strings.Contains(row.Detail, tt.wantDetail) {
				t.Errorf("row = %+v, want take %s containing %q", row, tt.wantVerdict, tt.wantDetail)
			}
		})
	}
}

func TestJudge_Run(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		command     string
		options     Options
		wantVerdict Verdict
		wantDetail  string
	}{
		{name: "exit 0 passes with its trimmed output", command: `echo "  all good  "`, wantVerdict: Pass, wantDetail: "all good"},
		{name: "a silent pass", command: `true`, wantVerdict: Pass, wantDetail: passedDetail},
		{name: "a non-zero exit fails with its output", command: `echo broken >&2; exit 3`, wantVerdict: Fail, wantDetail: "broken"},
		{name: "a silent failure names its exit status", command: `exit 4`, wantVerdict: Fail, wantDetail: "exit status 4"},
		{name: "a command not found fails", command: `no-such-command-anywhere`, wantVerdict: Fail, wantDetail: "not found"},
		{name: "the beat id is in its env", command: `printf %s "$PHOTON_BEAT"`, wantVerdict: Pass, wantDetail: "2"},
		{
			name:        "extra env reaches it",
			command:     `printf %s "$PHOTON_PORT_API"`,
			options:     Options{Env: []string{"PHOTON_PORT_API=4242"}},
			wantVerdict: Pass,
			wantDetail:  "4242",
		},
		{
			name:        "photon's own variables win over extra env",
			command:     `printf %s "$PHOTON_BEAT"`,
			options:     Options{Env: []string{"PHOTON_BEAT=9"}},
			wantVerdict: Pass,
			wantDetail:  "2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board := fixtureBoard()
			board.Beats[1].Expect = []storyboard.Expect{{Run: tt.command}}

			row := onlyRow(t, judge(t, board, t.TempDir(), fixtureTakePath, tt.options))

			if row.Subject != beatSubject(2) || row.Verdict != tt.wantVerdict || !strings.Contains(row.Detail, tt.wantDetail) {
				t.Errorf("row = %+v, want beat 2 %s containing %q", row, tt.wantVerdict, tt.wantDetail)
			}
		})
	}
}

func TestJudge_TakeLevelRunHasNoBeat(t *testing.T) {
	t.Parallel()
	board := fixtureBoard()
	board.Expect = []storyboard.Expect{{Run: `printf %s "${PHOTON_BEAT-unset}"`}}

	row := onlyRow(t, judge(t, board, t.TempDir(), fixtureTakePath, Options{Env: []string{"PHOTON_BEAT=9"}}))

	if row.Subject != takeSubject || row.Verdict != Pass || row.Detail != "unset" {
		t.Errorf("row = %+v, want a take PASS reading unset", row)
	}
}

func TestJudge_RunInAMissingWorkDirCreatesIt(t *testing.T) {
	t.Parallel()
	work := filepath.Join(t.TempDir(), "not", "yet")
	board := fixtureBoard()
	// The shell compares the two dirs itself: Git Bash on Windows spells `pwd` as /c/... while
	// PHOTON_WORK reads C:\...
	board.Beats[0].Expect = []storyboard.Expect{{
		Run: `[ "$(pwd -P)" = "$(cd "$PHOTON_WORK" && pwd -P)" ] && printf %s "$PHOTON_WORK"`,
	}}

	row := onlyRow(t, judge(t, board, work, fixtureTakePath, Options{}))

	if _, err := os.Stat(work); err != nil {
		t.Fatalf("the work dir was not created: %v", err)
	}
	if row.Verdict != Pass || row.Detail != work {
		t.Errorf("row = %+v, want a PASS run in the work dir reading %q", row, work)
	}
}

func TestJudge_RelativeTakeIsExportedAbsolute(t *testing.T) {
	t.Parallel()
	relative := filepath.Join("takes", "demo.take")
	board := fixtureBoard()
	board.Expect = []storyboard.Expect{{Run: `printf %s "$PHOTON_TAKE"`}}

	row := onlyRow(t, judge(t, board, t.TempDir(), relative, Options{}))

	want, err := filepath.Abs(relative)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	if row.Verdict != Pass || row.Detail != want {
		t.Errorf("row = %+v, want PHOTON_TAKE %q", row, want)
	}
}

func TestJudge_WithoutShFails(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	board := fixtureBoard()
	board.Expect = []storyboard.Expect{{Run: `true`}}

	row := onlyRow(t, judge(t, board, t.TempDir(), fixtureTakePath, Options{}))

	if row.Verdict != Fail || !strings.Contains(row.Detail, "sh not found") {
		t.Errorf("row = %+v, want a FAIL naming sh", row)
	}
}

func TestJudge_BeatsMustAgree(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		beats   []storyboard.Beat
		mutate  func(*take.Take)
		wantErr string
	}{
		{
			name:    "a storyboard beat the take never started",
			beats:   []storyboard.Beat{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}},
			wantErr: "beat 4: the take never started it",
		},
		{
			name:    "a take beat the storyboard lacks",
			beats:   []storyboard.Beat{{ID: 1}, {ID: 2}},
			wantErr: "beat 3: the take started it but the storyboard has no such beat",
		},
		{
			name:  "a beat the take started twice",
			beats: []storyboard.Beat{{ID: 1}, {ID: 2}, {ID: 3}},
			mutate: func(recorded *take.Take) {
				recorded.Events = append(recorded.Events, take.Event{At: 5 * time.Second, Kind: take.EventBeatStart, Beat: 2})
			},
			wantErr: "beat 2: started twice in the take",
		},
		{
			name:    "beats the take started out of storyboard order",
			beats:   []storyboard.Beat{{ID: 2}, {ID: 1}, {ID: 3}},
			wantErr: "the take started its beats in the order 1, 2, 3; the storyboard runs 2, 1, 3 — re-record it",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board := fixtureBoard()
			board.Beats = tt.beats
			recorded := fixtureTake()
			if tt.mutate != nil {
				tt.mutate(recorded)
			}

			_, err := Judge(context.Background(), board, recorded, fixtureResolver(t), t.TempDir(), fixtureTakePath, Options{})

			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Judge error = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestJudge_OrdersBeatRowsThenTakeRows(t *testing.T) {
	t.Parallel()
	board := fixtureBoard()
	board.Expect = []storyboard.Expect{{Seen: `bye`}}
	board.Beats[2].Expect = []storyboard.Expect{{Seen: `bye`}, {Seen: `absent`}}
	board.Beats[0].Expect = []storyboard.Expect{{Seen: `ready`}}

	rows := judge(t, board, t.TempDir(), fixtureTakePath, Options{})

	var subjects []string
	for _, row := range rows {
		subjects = append(subjects, row.Subject+" "+string(row.Verdict))
	}
	want := "beat 1 PASS, beat 3 PASS, beat 3 FAIL, take PASS"
	if got := strings.Join(subjects, ", "); got != want || Failed(rows) != 1 {
		t.Errorf("rows = %s (failed %d), want %s (failed 1)", got, Failed(rows), want)
	}
}

func TestJudge_SeenOnlyLeavesTheWorkDirAlone(t *testing.T) {
	t.Parallel()
	work := filepath.Join(t.TempDir(), "absent")
	board := fixtureBoard()
	board.Expect = []storyboard.Expect{{Seen: `bye`}}

	judge(t, board, work, fixtureTakePath, Options{})

	if _, err := os.Stat(work); !os.IsNotExist(err) {
		t.Errorf("stat %s = %v, want it never created", work, err)
	}
}

func TestWriteTable(t *testing.T) {
	t.Parallel()
	rows := []Row{
		{Subject: "beat 9", Verdict: Pass, Detail: "seen /x/ at 1s"},
		{Subject: "beat 10", Verdict: Fail, Detail: "broken"},
		{Subject: takeSubject, Verdict: Pass, Detail: "exit 0"},
	}
	var out bytes.Buffer

	err := WriteTable(&out, rows)

	want := "beat 9  | PASS | seen /x/ at 1s\n" +
		"beat 10 | FAIL | broken\n" +
		"take    | PASS | exit 0\n"
	if err != nil || out.String() != want {
		t.Errorf("WriteTable = %v\n%q, want\n%q", err, out.String(), want)
	}
}
