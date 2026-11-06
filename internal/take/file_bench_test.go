package take

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// The take BenchmarkLoad times: a full-size terminal recorded for a minute at ten snapshots a
// second.
const (
	benchCols      = 160
	benchRows      = 48
	benchSnapshots = 600
)

// The small take the round-trip test and the equivalence tests use, so that they stay fast under
// the race detector.
const (
	smallCols      = 40
	smallRows      = 12
	smallSnapshots = 20
)

// The shape of the take largeTake builds: one snapshot every largeStep, a beat starting every
// largeBeatEvery snapshots and a click every largeClickEvery, and the cell styles cycling through
// largeCellKinds kinds.
const (
	largeStep       = 100 * time.Millisecond
	largeBeatEvery  = 50
	largeClickEvery = 7
	largeCellKinds  = 8
	largeAlphabet   = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	largeWide       = "漢"
)

// largeTake is a take of snapshots full screens of cols×rows cells, each screen shifted from the
// last so that no two snapshots are alike. Its cells mix the default, palette and RGB colour
// forms, every attribute, and wide and combining-mark graphemes; its events start a beat every
// largeBeatEvery snapshots, type on every snapshot and click every largeClickEvery, all in time
// order between the snapshots.
func largeTake(cols, rows, snapshots int) *Take {
	screens := make([]Snapshot, 0, snapshots)
	var events []Event
	for index := range snapshots {
		at := time.Duration(index) * largeStep
		screens = append(screens, Snapshot{
			At:     at,
			Cursor: Cursor{X: index % cols, Y: index % rows, Visible: index%2 == 0},
			Cells:  largeScreen(cols, rows, index),
		})
		events = append(events, largeEvents(cols, rows, index, at)...)
	}
	return &Take{
		Cols:      cols,
		Rows:      rows,
		FPS:       int(time.Second / largeStep),
		Started:   time.Date(2026, time.November, 6, 9, 30, 0, 0, time.UTC),
		Snapshots: screens,
		Events:    events,
	}
}

// largeScreen is snapshot index's grid of rows rows of cols cells for largeTake.
func largeScreen(cols, rows, index int) [][]Cell {
	grid := make([][]Cell, rows)
	for row := range rows {
		line := make([]Cell, 0, cols)
		for len(line) < cols {
			seed := index + row + len(line)
			kind := seed % largeCellKinds
			if kind == 0 && len(line)+1 < cols {
				line = append(line,
					Cell{Grapheme: largeWide, Width: 2, FG: Color{Kind: ColorPalette, Index: 3}},
					Cell{Width: 0})
				continue
			}
			line = append(line, largeCell(kind, seed))
		}
		grid[row] = line
	}
	return grid
}

// largeCell is one narrow cell of the given style kind, its grapheme and colours picked by seed.
func largeCell(kind, seed int) Cell {
	letter := largeAlphabet[seed%len(largeAlphabet) : seed%len(largeAlphabet)+1]
	palette := Color{Kind: ColorPalette, Index: uint8(seed % 256)}
	rgb := Color{Kind: ColorRGB, R: uint8(seed), G: uint8(seed * 3), B: uint8(seed * 7)}
	switch kind {
	case 1:
		return Cell{Grapheme: letter, Width: 1, FG: palette, Bold: true}
	case 2:
		return Cell{Grapheme: letter, Width: 1, FG: rgb, BG: palette, Italic: true}
	case 3:
		return Cell{Grapheme: combiningGrapheme, Width: 1, BG: rgb, Underline: true}
	case 4:
		return Cell{Grapheme: letter, Width: 1, Faint: true, Reverse: true}
	case 5:
		return Cell{Grapheme: " ", Width: 1, BG: palette}
	default:
		return Cell{Grapheme: letter, Width: 1}
	}
}

// largeEvents are the events largeTake logs during snapshot index, which appears at at: a beat
// start every largeBeatEvery snapshots, a typed action on every one, and a resolved target and
// its click every largeClickEvery.
func largeEvents(cols, rows, index int, at time.Duration) []Event {
	beat := index/largeBeatEvery + 1
	var events []Event
	if index%largeBeatEvery == 0 {
		events = append(events,
			Event{At: at, Kind: EventBeatStart, Beat: beat, Text: fmt.Sprintf("beat %d", beat)})
	}
	action := index % largeBeatEvery
	typed := fmt.Sprintf("line %d", index)
	events = append(events,
		Event{At: at + 10*time.Millisecond, Kind: EventActionStart, Beat: beat, Action: action,
			Form: "type", Text: typed},
		Event{At: at + 50*time.Millisecond, Kind: EventActionEnd, Beat: beat, Action: action,
			Form: "type", Text: typed})
	if index%largeClickEvery == 0 {
		cell := Point{X: index % cols, Y: index % rows}
		events = append(events,
			Event{At: at + 60*time.Millisecond, Kind: EventTarget, Beat: beat, Action: action,
				Form: "click", Box: &Box{X: cell.X, Y: cell.Y, W: 1, H: 1}},
			Event{At: at + 70*time.Millisecond, Kind: EventClick, Beat: beat, Action: action,
				Form: "click", Cell: &cell})
	}
	return events
}

func TestLargeTakeRoundTrips(t *testing.T) {
	t.Parallel()
	want := largeTake(smallCols, smallRows, smallSnapshots)
	path := filepath.Join(t.TempDir(), "large.take")

	if err := Save(path, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(path)

	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load gave a take unlike the one saved")
	}
}

func BenchmarkLoad(b *testing.B) {
	path := filepath.Join(b.TempDir(), "large.take")
	if err := Save(path, largeTake(benchCols, benchRows, benchSnapshots)); err != nil {
		b.Fatalf("Save: %v", err)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := Load(path); err != nil {
			b.Fatalf("Load: %v", err)
		}
	}
}
