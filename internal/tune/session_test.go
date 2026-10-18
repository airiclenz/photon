package tune

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/clip"
	"github.com/airiclenz/photon/internal/render"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

// sessionConfig is the session tests' photon config: nothing but what validation requires.
const sessionConfig = `name: tuned
launch:
  command: ./app
`

// sessionStoryboard is the storyboard a session tunes: beat 1 lasts 1s, beat 2 1.5s and zooms
// onto the PASS its take shows.
const sessionStoryboard = `# A storyboard a session tunes.
clip: tuned
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
    duration: 1500ms # kept in ms
    zoom: {target: {text: 'PASS'}}
`

// cameraCaptionsStoryboard is sessionStoryboard with a camera of one zoom and three pans — a via
// path and a pan to a target and to a cell — inside beat 1, and two captions: one set by for in
// beat 1, one by until in beat 2.
const cameraCaptionsStoryboard = sessionStoryboard + `camera:
  - zoom: {to: 2, at: 100ms, over: 300ms}
  - pan: {via: [{cell: {x: 2, y: 1}}, {text: 'failing'}], at: {beat: 1, offset: 200ms}, over: 600ms, ease: linear}
  - pan: {to: {text: 'tests', nth: last}, at: 800ms, over: 100ms}
  - pan: {to: {cell: {x: 3, y: 2}}, at: 900ms, over: 100ms}
captions:
  - {text: 'Watch this', at: {beat: 1, offset: 100ms}, for: 700ms, place: top, align: left}
  - {text: "All green\n", at: {beat: 2}, until: {beat: 2, offset: 1200ms}}
`

// The session fixture's own values, as sessionStoryboard and sessionTake set them.
const (
	fixtureCols = 20
	// beatOneMS is beat 1's duration and outsideMS the one an outside write gives it.
	beatOneMS  = 1000
	outsideMS  = 2000
	beatOneOld = "duration: 1s\n"
	beatOneNew = "duration: 2s\n"
	// boardMode is the storyboard file's mode, which a save keeps.
	boardMode = 0o640
)

// sessionFixture is a session on a fresh project with its storyboard and take on disk.
type sessionFixture struct {
	session   *Session
	boardPath string
	takePath  string
}

// newSessionFixture writes the config, sessionStoryboard and a take into a fresh project dir and
// starts a session on them.
func newSessionFixture(t *testing.T) sessionFixture {
	t.Helper()
	return newSessionFixtureOf(t, sessionStoryboard)
}

// newSessionFixtureOf writes the config, board as the storyboard and a take into a fresh project
// dir and starts a session on them.
func newSessionFixtureOf(t *testing.T, board string) sessionFixture {
	t.Helper()
	project := t.TempDir()
	writeTestFile(t, filepath.Join(project, "photon.yaml"), sessionConfig)
	boardPath := filepath.Join(project, "tuned.yaml")
	writeTestFile(t, boardPath, board)
	if err := os.Chmod(boardPath, boardMode); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	takePath := filepath.Join(t.TempDir(), "tuned.take")
	saveSessionTake(t, takePath, fixtureCols, time.Now())
	loaded, err := clip.Load(boardPath, "", t.TempDir())
	if err != nil {
		t.Fatalf("clip.Load: %v", err)
	}
	session, err := NewSession(loaded, takePath)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	return sessionFixture{session: session, boardPath: boardPath, takePath: takePath}
}

// writeTestFile writes content to path.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// saveSessionTake writes a take of cols columns to path, stamped modified at modTime: beat 1
// runs 2s of take, beat 2 1s and shows PASS from 2.5s.
func saveSessionTake(t *testing.T, path string, cols int, modTime time.Time) {
	t.Helper()
	recorded := &take.Take{
		Cols: cols, Rows: 4, FPS: 10,
		Snapshots: []take.Snapshot{
			lineSnapshot(0, "$ app"),
			lineSnapshot(time.Second, "> tests are failing"),
			lineSnapshot(2500*time.Millisecond, "Tests PASS"),
			lineSnapshot(3*time.Second, "Tests PASS", "done"),
		},
		Events: []take.Event{
			{At: 0, Kind: take.EventBeatStart, Beat: 1},
			{At: 2 * time.Second, Kind: take.EventBeatStart, Beat: 2},
		},
	}
	if err := take.Save(path, recorded); err != nil {
		t.Fatalf("take.Save: %v", err)
	}
	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

// lineSnapshot is a snapshot at at showing one line of text per row.
func lineSnapshot(at time.Duration, lines ...string) take.Snapshot {
	rows := make([][]take.Cell, 0, len(lines))
	for _, line := range lines {
		row := make([]take.Cell, 0, len(line))
		for _, r := range line {
			row = append(row, take.Cell{Grapheme: string(r), Width: 1})
		}
		rows = append(rows, row)
	}
	return take.Snapshot{At: at, Cells: rows}
}

// apply applies edit to the fixture's session and fails the test on an error.
func (f sessionFixture) apply(t *testing.T, edit Edit) {
	t.Helper()
	if err := f.session.Apply(edit); err != nil {
		t.Fatalf("Apply(%+v): %v", edit, err)
	}
}

// writeOutside rewrites the storyboard on disk as another editor would: beat 1 lasts 2s.
func (f sessionFixture) writeOutside(t *testing.T) {
	t.Helper()
	writeTestFile(t, f.boardPath, strings.Replace(sessionStoryboard, beatOneOld, beatOneNew, 1))
}

// readDisk returns the storyboard as it is on disk.
func (f sessionFixture) readDisk(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(f.boardPath)
	if err != nil {
		t.Fatalf("read the storyboard: %v", err)
	}
	return string(src)
}

// beatDuration returns the duration in ms state reports for the beat with id.
func beatDuration(t *testing.T, state State, id int) int64 {
	t.Helper()
	for _, beat := range state.Beats {
		if beat.ID == id {
			return beat.DurationMS
		}
	}
	t.Fatalf("state has no beat %d", id)
	return 0
}

func TestSession_History_MovesThroughVersions(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	steps := []struct {
		name              string
		act               func() error
		wantMS            int64
		isDirty, canUndo  bool
		canRedo           bool
		wantNothingToUndo bool
	}{
		{"undo with no history", f.session.Undo, beatOneMS, false, false, false, true},
		{"apply", func() error { return f.session.Apply(Edit{Beat: 1, Field: fieldDuration, Value: "1200"}) }, 1200, true, true, false, false},
		{"undo", f.session.Undo, beatOneMS, false, false, true, false},
		{"redo", f.session.Redo, 1200, true, true, false, false},
		{"revert", func() error { f.session.Revert(); return nil }, beatOneMS, false, true, false, false},
		{"undo the revert", f.session.Undo, 1200, true, true, true, false},
	}
	for _, step := range steps {
		err := step.act()

		if step.wantNothingToUndo != errors.Is(err, ErrNoUndo) || (!step.wantNothingToUndo && err != nil) {
			t.Fatalf("%s: err %v", step.name, err)
		}
		state := f.session.State()
		if got := beatDuration(t, state, 1); got != step.wantMS {
			t.Errorf("%s: beat 1 duration %dms, want %dms", step.name, got, step.wantMS)
		}
		if state.IsDirty != step.isDirty || state.CanUndo != step.canUndo || state.CanRedo != step.canRedo {
			t.Errorf("%s: dirty %v undo %v redo %v, want %v %v %v", step.name,
				state.IsDirty, state.CanUndo, state.CanRedo, step.isDirty, step.canUndo, step.canRedo)
		}
	}
}

func TestSession_Redo_WithNothingUndone_IsRefused(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)

	err := f.session.Redo()

	if !errors.Is(err, ErrNoRedo) {
		t.Fatalf("Redo: got %v, want ErrNoRedo", err)
	}
}

func TestSession_State_ReportsTheSchedule(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)

	state := f.session.State()

	if state.LengthMS != 2500 || state.FPS != 10 || state.Width != 160 {
		t.Errorf("length %dms fps %d width %d, want 2500ms 10 160", state.LengthMS, state.FPS, state.Width)
	}
	if len(state.Sections) != 2 || state.Sections[0].Report != "beat 1  2.00×  2s of take in 1s" {
		t.Errorf("sections %+v", state.Sections)
	}
	beat := state.Beats[1]
	if beat.Zoom == nil || beat.Zoom.Target.Text != "PASS" || beat.Zoom.InMS != 1000 {
		t.Errorf("beat 2 zoom %+v", beat.Zoom)
	}
	if len(beat.PausesMS) != 1 || beat.PausesMS[0] != 400 {
		t.Errorf("beat 2 pauses %v, want [400]", beat.PausesMS)
	}
}

func TestSession_State_ReportsTheReadOnlyFrame(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)

	state := f.session.State()

	frame := []any{state.Cols, state.Rows, state.Padding, state.FontSize, state.LineHeight, state.Scale, state.MaxColors}
	if want := []any{fixtureCols, 4, 4, 10.0, 1.2, 2, 16}; !slices.Equal(frame, want) {
		t.Errorf("cols, rows, padding, font_size, line_height, scale, max_colors %v, want %v", frame, want)
	}
}

func TestSession_State_ReportsCameraMovesAndCaptions(t *testing.T) {
	t.Parallel()
	f := newSessionFixtureOf(t, cameraCaptionsStoryboard)

	state := f.session.State()

	wantCamera := []CameraState{
		{Index: 0, Kind: "zoom", StartMS: 100, EndMS: 400, At: MomentState{0, 100}, OverMS: 300, Ease: "smooth", Factor: 2},
		{Index: 1, Kind: "pan", StartMS: 200, EndMS: 800, At: MomentState{1, 200}, OverMS: 600, Ease: "linear", ViaCount: 2},
		{
			Index: 2, Kind: "pan", StartMS: 800, EndMS: 900, At: MomentState{0, 800}, OverMS: 100, Ease: "smooth",
			Target: &TargetState{Text: "tests", Nth: -1},
		},
		{
			Index: 3, Kind: "pan", StartMS: 900, EndMS: 1000, At: MomentState{0, 900}, OverMS: 100, Ease: "smooth",
			Cell: &CellState{X: 3, Y: 2},
		},
	}
	if !reflect.DeepEqual(state.Camera, wantCamera) {
		t.Errorf("camera\n got %+v\nwant %+v", state.Camera, wantCamera)
	}
	wantCaptions := []CaptionState{
		{
			Index: 0, Text: "Watch this", StartMS: 100, StopMS: 800, At: MomentState{1, 100}, ForMS: 700,
			Place: "top", Align: "left",
		},
		{
			Index: 1, Text: "All green", StartMS: 1000, StopMS: 2200, At: MomentState{2, 0},
			Until: &MomentState{2, 1200}, Place: "bottom", Align: "middle",
		},
	}
	if !reflect.DeepEqual(state.Captions, wantCaptions) {
		t.Errorf("captions\n got %+v\nwant %+v", state.Captions, wantCaptions)
	}
}

func TestSession_State_CaptionTimesFollowABeatDuration(t *testing.T) {
	t.Parallel()
	f := newSessionFixtureOf(t, cameraCaptionsStoryboard)

	f.apply(t, Edit{Beat: 1, Field: fieldDuration, Value: "1200"})

	caption := f.session.State().Captions[1]
	if caption.StartMS != 1200 || caption.StopMS != 2400 {
		t.Errorf("caption 1 from %dms to %dms, want 1200ms to 2400ms", caption.StartMS, caption.StopMS)
	}
}

func TestSession_Refresh_CameraProblemNamingABeat_IsOnTheMove(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	writeTestFile(t, f.boardPath, sessionStoryboard+"camera:\n  - zoom: {to: 2, at: 1200ms, over: 300ms}\n")

	if err := f.session.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	errs := f.session.State().Errors
	isOnTheMove := slices.ContainsFunc(errs, func(problem Problem) bool {
		return problem.Owner == storyboard.Owner{Kind: storyboard.OwnerCamera, Index: 0} &&
			strings.HasPrefix(problem.Message, "camera[0]:") && strings.Contains(problem.Message, "beat 2")
	})
	if !isOnTheMove {
		t.Errorf("errors %+v, want camera[0]'s problem naming beat 2 on camera 0", errs)
	}
}

func TestSession_PanTargetNotOnScreen_IsOnTheMove(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	writeTestFile(t, f.boardPath, sessionStoryboard+"camera:\n  - pan: {to: {text: 'nowhere'}, at: 100ms, over: 300ms}\n")

	if err := f.session.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	errs := f.session.State().Errors

	want := storyboard.Owner{Kind: storyboard.OwnerCamera, Index: 0}
	if len(errs) != 1 || errs[0].Owner != want || !strings.HasPrefix(errs[0].Message, "compose: camera[0]: ") {
		t.Errorf("errors %+v, want one compose problem on camera 0", errs)
	}
}

func TestSession_CaptionBoxLargerThanTheFrame_IsOnTheCaption(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	wide := strings.Repeat("too wide ", 20)
	writeTestFile(t, f.boardPath, sessionStoryboard+"captions:\n  - {text: '"+wide+"', at: 100ms, for: 300ms}\n")

	if err := f.session.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	errs := f.session.State().Errors

	want := storyboard.Owner{Kind: storyboard.OwnerCaption, Index: 0}
	if len(errs) != 1 || errs[0].Owner != want || !strings.HasPrefix(errs[0].Message, "caption: captions[0]: ") {
		t.Errorf("errors %+v, want one caption problem on caption 0", errs)
	}
}

func TestSession_BeatTheTakeNeverStarted_IsOnTheBeat(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	writeTestFile(t, f.boardPath, sessionStoryboard+"  - id: 3\n    title: extra\n    duration: 1s\n")

	if err := f.session.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	errs := f.session.State().Errors
	want := storyboard.Owner{Kind: storyboard.OwnerBeat, Index: 3}
	if len(errs) != 1 || errs[0].Owner != want || !strings.Contains(errs[0].Message, "never started") {
		t.Errorf("errors %+v, want one on beat 3 saying the take never started it", errs)
	}
}

func TestSession_TooFastSection_Warns(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)

	f.apply(t, Edit{Beat: 1, Field: fieldDuration, Value: "300"})

	warnings := f.session.State().Warnings
	if len(warnings) != 1 || !strings.HasPrefix(warnings[0], "beat 1 plays at 6.7×") {
		t.Fatalf("warnings %q, want beat 1 at 6.7×", warnings)
	}
}

func TestSession_RefusedEdit_ChangesNothing(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	before := f.session.State()

	err := f.session.Apply(Edit{Beat: 1, Field: fieldHold, Value: "5000"})

	if err == nil {
		t.Fatal("Apply of a hold past the duration: want an error")
	}
	if after := f.session.State(); after.Version != before.Version || after.CanUndo {
		t.Errorf("version %d → %d, can undo %v; want unchanged", before.Version, after.Version, after.CanUndo)
	}
}

func TestSession_ZoomTargetNotOnScreen_KeepsTheLastGoodPipelineAndBlocksSave(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	good := f.session.good.frames
	beatTwo := storyboard.Owner{Kind: storyboard.OwnerBeat, Index: 2}

	f.apply(t, Edit{Beat: 2, Field: fieldZoomTarget, Value: "{text: 'nowhere'}"})

	state := f.session.State()
	if len(state.Errors) != 1 || state.Errors[0].Owner != beatTwo || !strings.Contains(state.Errors[0].Message, "nowhere") {
		t.Errorf("errors %+v, want one on beat 2 naming the target", state.Errors)
	}
	if f.session.good.frames != good {
		t.Error("the last good pipeline was replaced")
	}
	if err := f.session.Save(); !errors.Is(err, ErrInvalid) {
		t.Errorf("Save: got %v, want ErrInvalid", err)
	}
	if got := f.readDisk(t); got != sessionStoryboard {
		t.Errorf("the storyboard on disk changed:\n%s", got)
	}
}

func TestSession_EveryEditsBuild_ReusesTheGoodPipeline(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	var reused []bool
	f.session.buildPipeline = func(
		loaded *clip.Clip,
		takePath string,
		recorded *take.Take,
		options render.BuildOptions,
	) (*render.Pipeline, error) {
		// build runs under the session's lock, so good is the one this build replaces.
		reused = append(reused, options.Reuse != nil && options.Reuse == f.session.good.frames)
		return render.Build(loaded, takePath, recorded, options)
	}
	edits := []Edit{
		{Beat: 1, Field: fieldDuration, Value: "1100"},
		{Beat: 2, Field: fieldZoomTarget, Value: "{text: 'nowhere'}"},
		{Beat: 1, Field: fieldDuration, Value: "1200"},
		{Field: fieldFrameWidth, Value: "120"},
	}

	for _, edit := range edits {
		f.apply(t, edit)
	}

	want := []bool{true, true, true, true}
	if !slices.Equal(reused, want) {
		t.Errorf("each build given the good pipeline to reuse: %v, want %v", reused, want)
	}
}

func TestSession_Save_WritesInPlaceKeepingTheMode(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.apply(t, Edit{Beat: 1, Field: fieldDuration, Value: "1200"})

	err := f.session.Save()

	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	want := strings.Replace(sessionStoryboard, beatOneOld, "duration: 1.2s\n", 1)
	if got := f.readDisk(t); got != want {
		t.Errorf("saved:\n%s\nwant:\n%s", got, want)
	}
	// Windows keeps only a read-only bit, so the storyboard reads back as 0666 there.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(f.boardPath)
		if err != nil || info.Mode().Perm() != boardMode {
			t.Errorf("mode %v (%v), want %v", info.Mode().Perm(), err, os.FileMode(boardMode))
		}
	}
	entries, err := os.ReadDir(filepath.Dir(f.boardPath))
	if err != nil || len(entries) != 2 {
		t.Errorf("project dir holds %d entries (%v), want the config and the storyboard", len(entries), err)
	}
	if f.session.State().IsDirty {
		t.Error("dirty after save")
	}
}

func TestSession_Refresh_AfterOutsideWrite(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		isDirty   bool
		wantStale bool
		wantMS    int64
	}{
		{"clean reloads", false, false, outsideMS},
		{"dirty turns stale", true, true, beatOneMS},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newSessionFixture(t)
			if tc.isDirty {
				f.apply(t, Edit{Beat: 2, Field: fieldDuration, Value: "1600"})
			}
			f.writeOutside(t)

			err := f.session.Refresh()

			if err != nil {
				t.Fatalf("Refresh: %v", err)
			}
			state := f.session.State()
			if state.IsStale != tc.wantStale || beatDuration(t, state, 1) != tc.wantMS {
				t.Errorf("stale %v beat 1 %dms, want %v %dms",
					state.IsStale, beatDuration(t, state, 1), tc.wantStale, tc.wantMS)
			}
			if tc.isDirty {
				if err := f.session.Save(); !errors.Is(err, ErrStale) {
					t.Errorf("Save: got %v, want ErrStale", err)
				}
			}
		})
	}
}

func TestSession_SaveAfterOutsideWrite_IsStale(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.apply(t, Edit{Beat: 1, Field: fieldDuration, Value: "1200"})
	f.writeOutside(t)

	err := f.session.Save()

	if !errors.Is(err, ErrStale) {
		t.Fatalf("Save: got %v, want ErrStale", err)
	}
	if !f.session.State().IsStale {
		t.Error("not stale after the refused save")
	}
	if got := f.readDisk(t); !strings.Contains(got, beatOneNew) {
		t.Errorf("the outside write was overwritten:\n%s", got)
	}
}

func TestSession_SaveThenRefresh_KeepsTheVersionAndUndo(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.apply(t, Edit{Beat: 1, Field: fieldDuration, Value: "1200"})
	if err := f.session.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	saved := f.session.State()

	err := f.session.Refresh()

	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if after := f.session.State(); after.Version != saved.Version || !after.CanUndo {
		t.Errorf("version %d → %d, can undo %v; want the same version and undo", saved.Version, after.Version, after.CanUndo)
	}
}

func TestSession_ReloadAfterOutsideWrite_ClearsStaleAndSaves(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	f.apply(t, Edit{Beat: 2, Field: fieldDuration, Value: "1600"})
	f.writeOutside(t)
	if err := f.session.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	err := f.session.Reload()

	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	state := f.session.State()
	if state.IsStale || state.IsDirty || state.CanUndo || beatDuration(t, state, 1) != outsideMS {
		t.Errorf("after reload: %+v", state)
	}
	f.apply(t, Edit{Beat: 2, Field: fieldDuration, Value: "1600"})
	if err := f.session.Save(); err != nil {
		t.Fatalf("Save after reload: %v", err)
	}
	if got := f.readDisk(t); !strings.Contains(got, beatOneNew) || !strings.Contains(got, "duration: 1600ms") {
		t.Errorf("saved:\n%s", got)
	}
}

func TestSession_RewrittenTake_IsReadAgain(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	later := time.Now().Add(time.Hour)
	saveSessionTake(t, f.takePath, fixtureCols+10, later)

	err := f.session.Refresh()

	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	errs := f.session.State().Errors
	if len(errs) != 1 || errs[0].Owner != (storyboard.Owner{Kind: storyboard.OwnerClip}) || !strings.Contains(errs[0].Message, "recorded at 30×4") {
		t.Fatalf("errors %+v, want the take's size on the clip", errs)
	}
	saveSessionTake(t, f.takePath, fixtureCols, later.Add(time.Hour))
	if err := f.session.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if errs := f.session.State().Errors; len(errs) != 0 {
		t.Errorf("errors after the take is back: %+v", errs)
	}
}

func TestNewSession_TakeOfAnotherFrame_IsRefused(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	saveSessionTake(t, f.takePath, fixtureCols+10, time.Now())

	_, err := NewSession(f.session.base, f.takePath)

	if err == nil || !strings.Contains(err.Error(), "re-record it") {
		t.Fatalf("NewSession: got %v, want the take's size refused", err)
	}
}

func TestSession_ApplyAndState_RunInParallel(t *testing.T) {
	t.Parallel()
	f := newSessionFixture(t)
	const workers = 4
	durations := []string{"1100", "1200"}
	var group sync.WaitGroup
	errs := make(chan error, workers*len(durations))

	for worker := range workers {
		group.Add(2)
		go func() {
			defer group.Done()
			for _, duration := range durations {
				errs <- f.session.Apply(Edit{Beat: 1, Field: fieldDuration, Value: duration})
			}
		}()
		go func() {
			defer group.Done()
			for range durations {
				if state := f.session.State(); len(state.Beats) != 2 {
					t.Errorf("worker %d: %d beats", worker, len(state.Beats))
				}
			}
		}()
	}
	group.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("Apply: %v", err)
		}
	}
	if got := beatDuration(t, f.session.State(), 1); got != 1100 && got != 1200 {
		t.Errorf("beat 1 duration %dms, want one of the applied", got)
	}
}

func TestSession_CameraEdit_MovesTheMoveAndUndoes(t *testing.T) {
	t.Parallel()
	f := newSessionFixtureOf(t, cameraCaptionsStoryboard)

	f.apply(t, Edit{Index: 0, Field: fieldCameraAt, Value: "{beat: 1, offset: 150ms}"})
	edited := f.session.State()
	undoErr := f.session.Undo()
	undone := f.session.State()

	if move := edited.Camera[0]; move.StartMS != 150 || move.At != (MomentState{1, 150}) || !edited.IsDirty {
		t.Errorf("after the edit: camera[0] %+v, dirty %v; want a start at 150ms on beat 1, dirty", move, edited.IsDirty)
	}
	if undoErr != nil {
		t.Fatalf("Undo: %v", undoErr)
	}
	if move := undone.Camera[0]; move.StartMS != 100 || undone.IsDirty {
		t.Errorf("after the undo: camera[0] %+v, dirty %v; want the start at 100ms, the bytes on disk", move, undone.IsDirty)
	}
}
