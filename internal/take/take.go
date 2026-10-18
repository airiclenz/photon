package take

import (
	"fmt"
	"sort"
	"time"
)

// Take is one recording of a program in a terminal: every distinct picture the terminal showed,
// each stamped with when it appeared, and a log of what photon did to the program along the way.
//
// A snapshot is a full grid rather than a diff, so a take can be read at any instant (a section
// boundary, a zoom's first frame) without replaying what came before. Snapshots and Events are
// each kept in time order, on one clock measured from Started.
type Take struct {
	// Cols and Rows are the terminal's size; every snapshot's grid is Rows rows of Cols cells.
	Cols int
	Rows int
	// FPS is the most snapshots per second the take was sampled at.
	FPS int
	// Started is the wall-clock time the take's clock counts from.
	Started time.Time

	Snapshots []Snapshot
	Events    []Event
}

// Snapshot is what the terminal showed from At (measured from the take's start) until the next
// snapshot: its cell grid, one slice per row, and its cursor.
type Snapshot struct {
	At     time.Duration `json:"at"`
	Cursor Cursor        `json:"cursor"`
	Cells  [][]Cell      `json:"cells"`
}

// Cursor is the cell the terminal's cursor sat on (0-based) and whether the program showed it.
type Cursor struct {
	X       int  `json:"x"`
	Y       int  `json:"y"`
	Visible bool `json:"visible"`
}

// ScreenAt returns the index of the snapshot on screen at the instant at on the take's clock: the
// last one taken at or before it, the first one when it precedes them all, and -1 when the take
// holds no snapshot.
func (t *Take) ScreenAt(at time.Duration) int {
	if len(t.Snapshots) == 0 {
		return -1
	}
	after := sort.Search(len(t.Snapshots), func(i int) bool { return t.Snapshots[i].At > at })
	return max(after-1, 0)
}

// BeatSpan is one beat's stretch of the take's clock: from its beat-start event to the next
// beat's start, or, for the last beat, to the take's end.
type BeatSpan struct {
	Beat  int
	Start time.Duration
	End   time.Duration
}

// BeatSpans returns the span of every beat the take started, in start order. Each beat runs from
// its [EventBeatStart] to the next beat's start; the last beat runs to the later of the last
// snapshot and the last event. A beat started more than once is an error naming its id.
// Whether every storyboard beat started is the caller's question: this sees only the take.
func (t *Take) BeatSpans() ([]BeatSpan, error) {
	var spans []BeatSpan
	started := make(map[int]bool)
	for _, event := range t.Events {
		if event.Kind != EventBeatStart {
			continue
		}
		if started[event.Beat] {
			return nil, fmt.Errorf("beat %d started twice in the take", event.Beat)
		}
		started[event.Beat] = true
		spans = append(spans, BeatSpan{Beat: event.Beat, Start: event.At})
	}

	sort.SliceStable(spans, func(a, b int) bool { return spans[a].Start < spans[b].Start })
	end := t.end()
	for index := range spans {
		spans[index].End = end
		if index+1 < len(spans) {
			spans[index].End = spans[index+1].Start
		}
	}
	return spans, nil
}

// end is the take's last instant on its clock: its last snapshot or its last event, whichever
// came later.
func (t *Take) end() time.Duration {
	var end time.Duration
	if n := len(t.Snapshots); n > 0 {
		end = t.Snapshots[n-1].At
	}
	if n := len(t.Events); n > 0 {
		end = max(end, t.Events[n-1].At)
	}
	return end
}
