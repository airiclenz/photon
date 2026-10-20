package storyboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/airiclenz/photon/internal/yamledit"
)

// FieldPositionTransition is the camera transition an [Edit] names by the position it leads
// into: its start and end edges on the output clock, moved in one edit.
const FieldPositionTransition = "position.transition"

// transitionExample is a transition as an [Edit] types it, as its errors offer it.
const transitionExample = `{"start_ms": 1200, "end_ms": 1800}`

// wireTransition is a transition as an [Edit] types it, {"start_ms": 1200, "end_ms": 1800}: its
// start and end edges in whole milliseconds of the output clock. Both keys are required.
type wireTransition struct {
	StartMS *int64 `json:"start_ms"`
	EndMS   *int64 `json:"end_ms"`
}

// transition is the stretch of the output clock between one position's end and the next one's
// start, from start to end; a start equal to its end is a hard cut.
type transition struct {
	start, end time.Duration
}

// editTransition moves the edges of the transition that leads into the camera position at index
// in board, the storyboard s.doc holds, to the start and end value types: the position before it
// by start ends at the start — a point moves its at there, a held position changes its hold —
// and the position at index starts at the end, keeping its end — a point moves its at, a held
// position moves its at and changes its hold by as much. Each at keeps its beat and shifts its
// offset, and an edge that stays put writes nothing. It is refused for the first position by
// start, which no transition leads into, a start after the end, and an edge that would leave
// either position a negative hold; an overlap the moved edges cause is left for validation to
// refuse.
func (s *splicer) editTransition(board *Storyboard, index int, value json.RawMessage) error {
	positions := board.Camera.Positions
	if index < 0 || index >= len(positions) {
		return fmt.Errorf("the storyboard has no camera position at index %d", index)
	}
	edges, err := decodeTransition(value)
	if err != nil {
		return err
	}
	before, hasBefore := positionBefore(positions, index)
	if !hasBefore {
		return errors.New("the position is the first by start, so no transition leads into it")
	}

	if err := s.endPositionAt(before, positions[before], edges.start); err != nil {
		return err
	}
	return s.startPositionAt(index, positions[index], edges.end)
}

// positionBefore finds the position that starts last before the one at index, by their resolved
// starts, the position whose end the transition into the one at index leaves from. It reports
// false for the first position by start.
func positionBefore(positions []Position, index int) (int, bool) {
	before, hasBefore := 0, false
	for other, position := range positions {
		isEarlier := position.Start() < positions[index].Start()
		if isEarlier && (!hasBefore || position.Start() > positions[before].Start()) {
			before, hasBefore = other, true
		}
	}
	return before, hasBefore
}

// currentTransition is the transition into the camera position at index as it stands: from the
// end of the position before it by start to its own start. It reports false for the first
// position by start, which no transition leads into.
func currentTransition(positions []Position, index int) (transition, bool) {
	before, hasBefore := positionBefore(positions, index)
	if !hasBefore {
		return transition{}, false
	}
	previous := positions[before]
	return transition{start: previous.Start() + previous.Hold, end: positions[index].Start()}, true
}

// endPositionAt makes shown, the camera position at index, end at end: a point moves its at there
// and a held position changes its hold, refused when that leaves it a negative hold.
func (s *splicer) endPositionAt(index int, shown Position, end time.Duration) error {
	position := positionPath(index)
	if shown.Hold == 0 {
		return s.movePosition(position, shown, end)
	}
	hold := end - shown.Start()
	if hold < 0 {
		return fmt.Errorf("want a start_ms no earlier than camera.positions[%d] starts, %dms, "+
			"got %dms", index, shown.Start().Milliseconds(), end.Milliseconds())
	}
	if hold == shown.Hold {
		return nil
	}
	return s.writeHold(position, hold)
}

// startPositionAt makes shown, the camera position at index, start at start: a point moves its
// at there and a held position moves its at and changes its hold to keep its end, refused when
// that leaves it a negative hold.
func (s *splicer) startPositionAt(index int, shown Position, start time.Duration) error {
	position := positionPath(index)
	if shown.Hold == 0 {
		return s.movePosition(position, shown, start)
	}
	end := shown.Start() + shown.Hold
	hold := end - start
	if hold < 0 {
		return fmt.Errorf("want an end_ms no later than camera.positions[%d] ends, %dms, "+
			"got %dms", index, end.Milliseconds(), start.Milliseconds())
	}
	if err := s.movePosition(position, shown, start); err != nil {
		return err
	}
	if hold == shown.Hold {
		return nil
	}
	return s.writeHold(position, hold)
}

// movePosition moves the at of shown, the camera position at position, so that it starts at
// start, keeping its beat and shifting its offset, or writes nothing when it starts there
// already.
func (s *splicer) movePosition(position yamledit.Path, shown Position, start time.Duration) error {
	shift := start - shown.Start()
	if shift == 0 {
		return nil
	}
	at := Moment{Beat: shown.At.Beat, Offset: shown.At.Offset + shift}
	return s.writeMoment(position, positionKeyOrder, at)
}

// positionPath is the path of the camera position at index.
func positionPath(index int) yamledit.Path {
	return yamledit.Path{keyCamera, keyPositions, strconv.Itoa(index)}
}

// decodeTransition reads a transition typed as {"start_ms", "end_ms"}, refusing any other key, a
// missing one, a value that is not a whole number of milliseconds from 0 and a start after the
// end.
func decodeTransition(value json.RawMessage) (transition, error) {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	var wire wireTransition
	err := decoder.Decode(&wire)
	isWhole := err == nil && wire.StartMS != nil && wire.EndMS != nil
	if !isWhole || !isClipMilliseconds(*wire.StartMS) || !isClipMilliseconds(*wire.EndMS) {
		return transition{}, fmt.Errorf("want a transition such as %s, got %s",
			transitionExample, value)
	}
	if *wire.StartMS > *wire.EndMS {
		return transition{}, fmt.Errorf("want a start_ms no later than its end_ms, got %s", value)
	}
	return transition{
		start: time.Duration(*wire.StartMS) * time.Millisecond,
		end:   time.Duration(*wire.EndMS) * time.Millisecond,
	}, nil
}

// isClipMilliseconds reports whether milliseconds is a time on the output clock a time.Duration
// holds: from 0 to [maxMilliseconds].
func isClipMilliseconds(milliseconds int64) bool {
	return milliseconds >= 0 && milliseconds <= maxMilliseconds
}

// transitionJSON spells edges as an [Edit] types a transition, {"start_ms":1200,"end_ms":1800}.
func transitionJSON(edges transition) string {
	return `{"start_ms":` + strconv.FormatInt(edges.start.Milliseconds(), 10) +
		`,"end_ms":` + strconv.FormatInt(edges.end.Milliseconds(), 10) + `}`
}
