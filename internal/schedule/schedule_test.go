package schedule

import (
	"strings"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
)

const (
	sec = time.Second
	ms  = time.Millisecond
)

// beat is one storyboard beat's timing and the source time the take started it at.
type beat struct {
	id       int
	start    time.Duration
	duration time.Duration
	hold     time.Duration
	cut      bool
}

// recording builds the storyboard declaring beats and a take that starts each at its start time
// and ends with a snapshot at end.
func recording(t *testing.T, end time.Duration, beats ...beat) (*storyboard.Storyboard, *take.Take) {
	t.Helper()
	board := &storyboard.Storyboard{}
	recorded := &take.Take{Snapshots: []take.Snapshot{{At: 0}, {At: end}}}
	for _, b := range beats {
		board.Beats = append(board.Beats, storyboard.Beat{ID: b.id, Duration: b.duration, Hold: b.hold, Cut: b.cut})
		recorded.Events = append(recorded.Events, take.Event{At: b.start, Kind: take.EventBeatStart, Beat: b.id})
	}
	return board, recorded
}

// mustSchedule builds a schedule the test expects to be valid.
func mustSchedule(t *testing.T, end time.Duration, beats ...beat) *Schedule {
	t.Helper()
	schedule, err := New(recording(t, end, beats...))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return schedule
}

func TestSchedule_Source(t *testing.T) {
	t.Parallel()
	type probe struct {
		out    time.Duration
		source time.Duration
		beat   int
	}
	tests := []struct {
		name   string
		end    time.Duration
		beats  []beat
		length time.Duration
		probes []probe
	}{
		{
			name:   "slow beat sped up",
			end:    12 * sec,
			beats:  []beat{{id: 1, start: 2 * sec, duration: 5 * sec}},
			length: 5 * sec,
			probes: []probe{{0, 2 * sec, 1}, {2500 * ms, 7 * sec, 1}, {4 * sec, 10 * sec, 1}},
		},
		{
			name:   "short beat frozen on its last frame",
			end:    3 * sec,
			beats:  []beat{{id: 1, start: 1 * sec, duration: 5 * sec}},
			length: 5 * sec,
			probes: []probe{{1 * sec, 2 * sec, 1}, {3 * sec, 3*sec - time.Nanosecond, 1}, {4900 * ms, 3*sec - time.Nanosecond, 1}},
		},
		{
			name:   "hold played at 1x",
			end:    10 * sec,
			beats:  []beat{{id: 1, start: 0, duration: 4 * sec, hold: 2 * sec}},
			length: 4 * sec,
			probes: []probe{{1 * sec, 1 * sec, 1}, {2 * sec, 2 * sec, 1}, {3 * sec, 6 * sec, 1}},
		},
		{
			name: "cut beat removed",
			end:  23 * sec,
			beats: []beat{
				{id: 1, start: 0, duration: 2 * sec},
				{id: 2, start: 2 * sec, cut: true},
				{id: 3, start: 20 * sec, duration: 3 * sec},
			},
			length: 5 * sec,
			probes: []probe{{1 * sec, 1 * sec, 1}, {2 * sec, 20 * sec, 3}, {4 * sec, 22 * sec, 3}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			schedule := mustSchedule(t, tt.end, tt.beats...)

			if schedule.Length != tt.length {
				t.Errorf("Length = %s, want %s", schedule.Length, tt.length)
			}
			for _, p := range tt.probes {
				source, beat, ok := schedule.Source(p.out)
				if !ok || source != p.source || beat != p.beat {
					t.Errorf("Source(%s) = (%s, beat %d, ok=%v), want (%s, beat %d)", p.out, source, beat, ok, p.source, p.beat)
				}
			}
			if _, _, ok := schedule.Source(schedule.Length); ok {
				t.Errorf("Source(Length): want absent")
			}
		})
	}
}

func TestSchedule_Frames_LengthExactAt24FPS(t *testing.T) {
	t.Parallel()
	schedule := mustSchedule(t, 41*sec,
		beat{id: 1, start: 0, duration: 3300 * ms, hold: 500 * ms},
		beat{id: 2, start: 9 * sec, duration: 2250 * ms},
		beat{id: 3, start: 10 * sec, cut: true},
		beat{id: 4, start: 40 * sec, duration: 1 * sec},
	)
	const fps = 24
	want := 3300*ms + 2250*ms + sec
	if schedule.Length != want {
		t.Fatalf("Length = %s, want %s", schedule.Length, want)
	}

	frames := schedule.Frames(fps)

	clip := time.Duration(len(frames)) * sec / fps
	if diff := (clip - want).Abs(); diff > sec/fps {
		t.Errorf("clip runs %s for %d frames, want %s to within a frame", clip, len(frames), want)
	}
	for index, f := range frames {
		if index > 0 && f.Source < frames[index-1].Source {
			t.Fatalf("frame %d: source runs backwards (%s after %s)", index, f.Source, frames[index-1].Source)
		}
		if f.Beat == 3 {
			t.Fatalf("frame %d shows the cut beat", index)
		}
	}
	if got := schedule.Frames(0); got != nil {
		t.Errorf("Frames(0) = %d frames, want none", len(got))
	}
}

func TestNew_WarnsAboveSixX(t *testing.T) {
	t.Parallel()

	schedule := mustSchedule(t, 26*sec,
		beat{id: 1, start: 0, duration: 2 * sec},
		beat{id: 7, start: 12 * sec, duration: 2 * sec},
	)

	if len(schedule.Warnings) != 1 || !strings.Contains(schedule.Warnings[0], "beat 7") {
		t.Errorf("Warnings = %q, want one naming beat 7 (7×)", schedule.Warnings)
	}
}

func TestSchedule_Output_InvertsSource(t *testing.T) {
	t.Parallel()
	schedule := mustSchedule(t, 60*sec,
		beat{id: 1, start: 1 * sec, duration: 5 * sec},
		beat{id: 2, start: 4 * sec, duration: 7 * sec, hold: 1 * sec},
		beat{id: 3, start: 34 * sec, cut: true},
		beat{id: 4, start: 50 * sec, duration: 4 * sec},
	)

	for _, frame := range schedule.Frames(24) {
		section := schedule.Sections[frame.Beat-1]
		length := section.End - section.Start
		if length <= section.Duration && frame.Out-section.OutStart >= length {
			continue // a frozen frame: not a played instant
		}
		out, ok := schedule.Output(frame.Source)
		if !ok || (out-frame.Out).Abs() > time.Microsecond {
			t.Fatalf("Output(Source(%s) = %s) = %s (ok=%v), want %s", frame.Out, frame.Source, out, ok, frame.Out)
		}
	}

	if _, ok := schedule.Output(40 * sec); ok {
		t.Errorf("Output(40s) inside the cut beat: want absent")
	}
	if _, ok := schedule.Output(500 * ms); ok {
		t.Errorf("Output(0.5s) before the first beat: want absent")
	}
	if out, ok := schedule.Output(60 * sec); !ok || out != schedule.Length {
		t.Errorf("Output(take end) = %s (ok=%v), want %s", out, ok, schedule.Length)
	}
}

func TestSchedule_Output_ClickInsideAFastBeat(t *testing.T) {
	t.Parallel()
	// Beat 2 plays 30 s in 10 s: 3×. A click at source 5 s lands 3 s into it, 1 s on its output.
	schedule := mustSchedule(t, 32*sec,
		beat{id: 1, start: 0, duration: 2 * sec},
		beat{id: 2, start: 2 * sec, duration: 10 * sec},
	)

	if out, ok := schedule.Output(5 * sec); !ok || out != 3*sec {
		t.Errorf("Output(5s) = %s (ok=%v), want 3s", out, ok)
	}
	// An instant between two sampled frames still maps: 5.007 s → 3.002333 s, on no 24 fps frame.
	if out, ok := schedule.Output(5*sec + 7*ms); !ok || out != 3*sec+2333333*time.Nanosecond {
		t.Errorf("Output(5.007s) = %s (ok=%v), want 3.002333333s", out, ok)
	}
}

func TestNew_SpansFromTake(t *testing.T) {
	t.Parallel()
	board := &storyboard.Storyboard{Beats: []storyboard.Beat{
		{ID: 1, Duration: 2 * sec},
		{ID: 2, Duration: 4 * sec, Hold: sec},
		{ID: 3, Cut: true},
	}}
	recorded := &take.Take{
		Snapshots: []take.Snapshot{{At: 0}, {At: 9 * sec}},
		Events: []take.Event{
			{At: 500 * ms, Kind: take.EventBeatStart, Beat: 1},
			{At: 600 * ms, Kind: take.EventClick, Beat: 1, Cell: &take.Point{}},
			{At: 3 * sec, Kind: take.EventBeatStart, Beat: 2},
			{At: 7 * sec, Kind: take.EventBeatStart, Beat: 3},
			{At: 8 * sec, Kind: take.EventActionEnd, Beat: 3},
		},
	}

	schedule, err := New(board, recorded)

	if err != nil {
		t.Fatalf("New: %v", err)
	}
	want := []Section{
		{BeatSpan: take.BeatSpan{Beat: 1, Start: 500 * ms, End: 3 * sec}, Duration: 2 * sec, Speed: 1.25},
		{BeatSpan: take.BeatSpan{Beat: 2, Start: 3 * sec, End: 7 * sec}, Duration: 4 * sec, Hold: sec, OutStart: 2 * sec, Speed: 1},
		{BeatSpan: take.BeatSpan{Beat: 3, Start: 7 * sec, End: 9 * sec}, Cut: true, OutStart: 6 * sec, Speed: 1},
	}
	if len(schedule.Sections) != len(want) {
		t.Fatalf("Sections = %+v, want %+v", schedule.Sections, want)
	}
	for index := range want {
		if schedule.Sections[index] != want[index] {
			t.Errorf("Sections[%d] = %+v, want %+v", index, schedule.Sections[index], want[index])
		}
	}
}

func TestNew_RejectsBadSpans(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		end     time.Duration
		beats   []beat
		mutate  func(*storyboard.Storyboard, *take.Take)
		wantErr string
	}{
		{
			name:  "a beat never started",
			end:   3 * sec,
			beats: []beat{{id: 1, duration: sec}, {id: 2, start: 2 * sec, duration: sec}},
			mutate: func(_ *storyboard.Storyboard, recorded *take.Take) {
				recorded.Events = recorded.Events[:1]
			},
			wantErr: "schedule: beat 2: the take never started it",
		},
		{
			name:  "a beat started twice",
			end:   3 * sec,
			beats: []beat{{id: 1, duration: sec}},
			mutate: func(_ *storyboard.Storyboard, recorded *take.Take) {
				recorded.Events = append(recorded.Events, take.Event{At: 2 * sec, Kind: take.EventBeatStart, Beat: 1})
			},
			wantErr: "schedule: beat 1: started twice in the take",
		},
		{
			name:  "a beat the storyboard lacks",
			end:   3 * sec,
			beats: []beat{{id: 1, duration: sec}, {id: 2, start: 2 * sec, duration: sec}},
			mutate: func(board *storyboard.Storyboard, _ *take.Take) {
				board.Beats = board.Beats[:1]
			},
			wantErr: "schedule: beat 2: the take started it but the storyboard has no such beat",
		},
		{
			name:    "beats started out of storyboard order",
			end:     3 * sec,
			beats:   []beat{{id: 1, start: 2 * sec, duration: sec}, {id: 2, duration: sec}},
			wantErr: "schedule: the take started its beats in the order 2, 1; the storyboard runs 1, 2 — re-record it",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			board, recorded := recording(t, tt.end, tt.beats...)
			if tt.mutate != nil {
				tt.mutate(board, recorded)
			}

			_, err := New(board, recorded)

			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("New error = %v, want one containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestNew_AgreesWithTheStoryboardClipClock(t *testing.T) {
	t.Parallel()
	board, recorded := recording(t, 20*sec,
		beat{id: 1, start: 0, duration: 2 * sec},
		beat{id: 2, start: 3 * sec, duration: 4 * sec, hold: sec},
		beat{id: 3, start: 9 * sec, cut: true},
		beat{id: 4, start: 12 * sec, duration: 1500 * ms},
		beat{id: 6, start: 13 * sec, duration: 3 * sec},
	)

	schedule, err := New(board, recorded)

	if err != nil {
		t.Fatalf("New: %v", err)
	}
	clock := board.Clock()
	for _, section := range schedule.Sections {
		sectionStart, err := clock.SectionStart(section.Beat)
		if err != nil {
			t.Fatalf("SectionStart(beat %d): %v", section.Beat, err)
		}
		if sectionStart != section.OutStart {
			t.Errorf("beat %d: SectionStart = %s, want OutStart %s", section.Beat, sectionStart, section.OutStart)
		}
	}
	if length := clock.Length(); length != schedule.Length {
		t.Errorf("Length() = %s, want Schedule.Length %s", length, schedule.Length)
	}
}

func TestNew_RefusesATakeOutOfStoryboardOrder(t *testing.T) {
	t.Parallel()
	board, recorded := recording(t, 20*sec,
		beat{id: 1, start: 3 * sec, duration: 2 * sec},
		beat{id: 2, start: 0, duration: 4 * sec},
	)

	schedule, err := New(board, recorded)

	if err == nil || !strings.Contains(err.Error(), "re-record it") {
		t.Fatalf("New = %+v, %v; want the out-of-order refusal", schedule, err)
	}
}

func TestNew_JoinsOneLeafPerMismatchedBeat(t *testing.T) {
	t.Parallel()
	board, recorded := recording(t, 3*sec, beat{id: 1, duration: sec}, beat{id: 2, start: sec, duration: sec})
	recorded.Events = []take.Event{{At: 0, Kind: take.EventBeatStart, Beat: 9}}

	_, err := New(board, recorded)

	joined, isJoined := err.(interface{ Unwrap() []error })
	if !isJoined {
		t.Fatalf("New error = %#v, want a joined error", err)
	}
	want := []string{
		"schedule: beat 1: the take never started it",
		"schedule: beat 2: the take never started it",
		"schedule: beat 9: the take started it but the storyboard has no such beat",
	}
	leaves := joined.Unwrap()
	if len(leaves) != len(want) {
		t.Fatalf("New error leaves = %q, want %q", leaves, want)
	}
	for index, leaf := range leaves {
		if leaf.Error() != want[index] {
			t.Errorf("leaf %d = %q, want %q", index, leaf.Error(), want[index])
		}
	}
	wantOwners := []storyboard.Owner{
		{Kind: storyboard.OwnerBeat, Index: 1},
		{Kind: storyboard.OwnerBeat, Index: 2},
		{Kind: storyboard.OwnerClip},
	}
	for index, leaf := range leaves {
		if owner := storyboard.OwnerOf(leaf); owner != wantOwners[index] {
			t.Errorf("leaf %d owner = %+v, want %+v", index, owner, wantOwners[index])
		}
	}
}
