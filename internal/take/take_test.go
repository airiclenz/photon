package take

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTake_BeatSpans(t *testing.T) {
	t.Parallel()
	ms := time.Millisecond
	snapshotsTo := func(at time.Duration) []Snapshot { return []Snapshot{{At: 0}, {At: at}} }
	cases := []struct {
		name      string
		take      Take
		want      []BeatSpan
		wantError string
	}{
		{
			name: "no beats",
			take: Take{Snapshots: snapshotsTo(500 * ms)},
		},
		{
			name: "last beat runs to the last snapshot",
			take: Take{
				Snapshots: snapshotsTo(900 * ms),
				Events: []Event{
					{At: 100 * ms, Kind: EventBeatStart, Beat: 1},
					{At: 150 * ms, Kind: EventActionStart, Beat: 1},
					{At: 400 * ms, Kind: EventBeatStart, Beat: 2},
					{At: 450 * ms, Kind: EventActionEnd, Beat: 2},
				},
			},
			want: []BeatSpan{{Beat: 1, Start: 100 * ms, End: 400 * ms}, {Beat: 2, Start: 400 * ms, End: 900 * ms}},
		},
		{
			name: "last beat runs to the last event",
			take: Take{
				Snapshots: snapshotsTo(300 * ms),
				Events: []Event{
					{At: 0, Kind: EventBeatStart, Beat: 7},
					{At: 800 * ms, Kind: EventClick, Beat: 7, Cell: &Point{X: 1, Y: 2}},
				},
			},
			want: []BeatSpan{{Beat: 7, Start: 0, End: 800 * ms}},
		},
		{
			name: "spans follow start order, not beat ids",
			take: Take{
				Events: []Event{
					{At: 0, Kind: EventBeatStart, Beat: 3},
					{At: 200 * ms, Kind: EventBeatStart, Beat: 1},
					{At: 600 * ms, Kind: EventActionEnd, Beat: 1},
				},
			},
			want: []BeatSpan{{Beat: 3, Start: 0, End: 200 * ms}, {Beat: 1, Start: 200 * ms, End: 600 * ms}},
		},
		{
			name: "a beat started twice",
			take: Take{
				Events: []Event{
					{At: 0, Kind: EventBeatStart, Beat: 4},
					{At: 100 * ms, Kind: EventBeatStart, Beat: 5},
					{At: 200 * ms, Kind: EventBeatStart, Beat: 4},
				},
			},
			wantError: "beat 4 started twice",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got, err := c.take.BeatSpans()

			if c.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantError) {
					t.Fatalf("BeatSpans error = %v, want one containing %q", err, c.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("BeatSpans: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("BeatSpans = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestTake_ScreenAt(t *testing.T) {
	t.Parallel()
	recorded := Take{Snapshots: []Snapshot{{At: time.Second}, {At: 2 * time.Second}, {At: 4 * time.Second}}}
	cases := []struct {
		name string
		take Take
		at   time.Duration
		want int
	}{
		{name: "before the first", take: recorded, at: 0, want: 0},
		{name: "exactly at the first", take: recorded, at: time.Second, want: 0},
		{name: "just before the second", take: recorded, at: 1999 * time.Millisecond, want: 0},
		{name: "exactly at the second", take: recorded, at: 2 * time.Second, want: 1},
		{name: "between the second and the last", take: recorded, at: 3 * time.Second, want: 1},
		{name: "exactly at the last", take: recorded, at: 4 * time.Second, want: 2},
		{name: "after the last", take: recorded, at: 9 * time.Second, want: 2},
		{name: "empty take", take: Take{}, at: 0, want: -1},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.take.ScreenAt(tt.at); got != tt.want {
				t.Errorf("ScreenAt(%s): want %d, got %d", tt.at, tt.want, got)
			}
		})
	}
}

func TestTake_ScreensDuringAndThrough(t *testing.T) {
	t.Parallel()
	recorded := Take{Snapshots: []Snapshot{{At: time.Second}, {At: 2 * time.Second}, {At: 4 * time.Second}}}
	cases := []struct {
		name        string
		take        Take
		start, end  time.Duration
		wantDuring  []int
		wantThrough []int
	}{
		{name: "before the first snapshot", take: recorded, start: 0, end: 500 * time.Millisecond},
		{name: "ending exactly on the first snapshot", take: recorded, start: 0, end: time.Second, wantThrough: []int{0}},
		{name: "inside one", take: recorded, start: 2500 * time.Millisecond, end: 3 * time.Second, wantDuring: []int{1}, wantThrough: []int{1}},
		{name: "across several", take: recorded, start: 1500 * time.Millisecond, end: 5 * time.Second, wantDuring: []int{0, 1, 2}, wantThrough: []int{0, 1, 2}},
		{name: "from before the first into the second", take: recorded, start: 0, end: 3 * time.Second, wantDuring: []int{0, 1}, wantThrough: []int{0, 1}},
		{name: "ending exactly on a snapshot's At", take: recorded, start: 1500 * time.Millisecond, end: 4 * time.Second, wantDuring: []int{0, 1}, wantThrough: []int{0, 1, 2}},
		{name: "starting exactly on a snapshot's At", take: recorded, start: 2 * time.Second, end: 3 * time.Second, wantDuring: []int{1}, wantThrough: []int{1}},
		{name: "zero length between snapshots", take: recorded, start: 3 * time.Second, end: 3 * time.Second, wantDuring: []int{1}, wantThrough: []int{1}},
		{name: "zero length exactly on a snapshot's At", take: recorded, start: 2 * time.Second, end: 2 * time.Second, wantThrough: []int{1}},
		{name: "after the last", take: recorded, start: 9 * time.Second, end: 10 * time.Second, wantDuring: []int{2}, wantThrough: []int{2}},
		{name: "empty take", take: Take{}, start: 0, end: time.Second},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.take.ScreensDuring(tt.start, tt.end); !reflect.DeepEqual(got, tt.wantDuring) {
				t.Errorf("ScreensDuring(%s, %s): want %v, got %v", tt.start, tt.end, tt.wantDuring, got)
			}
			if got := tt.take.ScreensThrough(tt.start, tt.end); !reflect.DeepEqual(got, tt.wantThrough) {
				t.Errorf("ScreensThrough(%s, %s): want %v, got %v", tt.start, tt.end, tt.wantThrough, got)
			}
		})
	}
}
