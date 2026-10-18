package take

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The problems a [BeatError] names, one per way a take's beats can disagree with the storyboard's.
const (
	problemNeverStarted = "the take never started it"
	problemUndeclared   = "the take started it but the storyboard has no such beat"
	problemStartedTwice = "started twice in the take"
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

// ScreensDuring returns the indices, oldest first, of the snapshots on screen at some instant of
// the half-open stretch [start, end) of the take's clock, where snapshot i is shown over
// [At_i, At_{i+1}) and the last one from its At onward. A zero-length stretch between two
// snapshots yields the one on show; one exactly on a snapshot's At yields none. The result is
// empty when no snapshot is on screen during the stretch.
func (t *Take) ScreensDuring(start, end time.Duration) []int {
	stop := sort.Search(len(t.Snapshots), func(i int) bool { return t.Snapshots[i].At >= end })
	return t.screensSince(start, stop)
}

// ScreensThrough is [Take.ScreensDuring] over the closed stretch [start, end]: it also counts the
// snapshot taken exactly at end.
func (t *Take) ScreensThrough(start, end time.Duration) []int {
	stop := sort.Search(len(t.Snapshots), func(i int) bool { return t.Snapshots[i].At > end })
	return t.screensSince(start, stop)
}

// screensSince returns the indices of the snapshots before stop still on screen after start: the
// one in effect at start (the first, when start precedes them all) through stop-1.
func (t *Take) screensSince(start time.Duration, stop int) []int {
	after := sort.Search(len(t.Snapshots), func(i int) bool { return t.Snapshots[i].At > start })
	first := max(after-1, 0)
	if first >= stop {
		return nil
	}
	indices := make([]int, 0, stop-first)
	for index := first; index < stop; index++ {
		indices = append(indices, index)
	}
	return indices
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

// BeatError is one beat on which a take and its storyboard disagree: a storyboard beat the take
// never started, a beat the take started that the storyboard lacks, or one it started twice. It
// renders as `beat N: <problem>`.
type BeatError struct {
	Beat    int
	Problem string
}

// Error renders the beat's id before its problem.
func (e *BeatError) Error() string {
	return fmt.Sprintf("beat %d: %s", e.Beat, e.Problem)
}

// MatchBeats checks the take's beat starts against ids, the storyboard's beat ids in storyboard
// order with its cut beats included. Every id must have started exactly once and the take must
// have started no other beat: each disagreement is a [*BeatError], all of them joined — the
// never-started beats in storyboard order, then the undeclared and the twice-started ones in
// start order. When the beats agree, a take that started them in another order than ids is an
// error naming both orders, since every later step lays the beats out in storyboard order.
func (t *Take) MatchBeats(ids []int) error {
	order := t.startOrder()
	starts := make(map[int]int, len(order))
	for _, beat := range order {
		starts[beat]++
	}
	declared := make(map[int]bool, len(ids))
	var problems []error
	for _, id := range ids {
		if !declared[id] && starts[id] == 0 {
			problems = append(problems, &BeatError{Beat: id, Problem: problemNeverStarted})
		}
		declared[id] = true
	}
	for _, beat := range uniqueBeats(order) {
		if !declared[beat] {
			problems = append(problems, &BeatError{Beat: beat, Problem: problemUndeclared})
		}
		if starts[beat] > 1 {
			problems = append(problems, &BeatError{Beat: beat, Problem: problemStartedTwice})
		}
	}
	if len(problems) > 0 {
		return errors.Join(problems...)
	}

	if !slices.Equal(order, ids) {
		return fmt.Errorf(
			"the take started its beats in the order %s; the storyboard runs %s — re-record it",
			idList(order), idList(ids),
		)
	}
	return nil
}

// startOrder returns the id of every beat-start event in start order, a beat started twice
// appearing twice. It orders the starts as [Take.BeatSpans] does.
func (t *Take) startOrder() []int {
	var starts []Event
	for _, event := range t.Events {
		if event.Kind == EventBeatStart {
			starts = append(starts, event)
		}
	}
	sort.SliceStable(starts, func(a, b int) bool { return starts[a].At < starts[b].At })
	order := make([]int, 0, len(starts))
	for _, event := range starts {
		order = append(order, event.Beat)
	}
	return order
}

// uniqueBeats returns order without its repeats, each beat where it first appears.
func uniqueBeats(order []int) []int {
	seen := make(map[int]bool, len(order))
	unique := make([]int, 0, len(order))
	for _, beat := range order {
		if !seen[beat] {
			seen[beat] = true
			unique = append(unique, beat)
		}
	}
	return unique
}

// idList renders beat ids as `1, 2, 3`.
func idList(ids []int) string {
	words := make([]string, 0, len(ids))
	for _, id := range ids {
		words = append(words, strconv.Itoa(id))
	}
	return strings.Join(words, ", ")
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
