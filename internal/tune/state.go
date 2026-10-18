package tune

import (
	"slices"
	"time"

	"github.com/airiclenz/photon/internal/render"
	"github.com/airiclenz/photon/internal/schedule"
	"github.com/airiclenz/photon/internal/storyboard"
)

// State is everything the page shows of a session, as `/api/state` sends it. Durations are
// whole milliseconds, the spelling an [Edit] takes. Beats, Camera and Captions come from the
// current storyboard; Sections, Warnings and Length from its schedule, or from the last good
// pipeline's while the current bytes do not schedule. Version rises on every change, so a page that saw it already
// has nothing new to draw.
type State struct {
	Version  int            `json:"version"`
	Beats    []BeatState    `json:"beats"`
	Camera   []CameraState  `json:"camera"`
	Captions []CaptionState `json:"captions"`
	Sections []SectionState `json:"sections"`
	Warnings []string       `json:"warnings"`
	LengthMS int64          `json:"length_ms"`
	FPS      int            `json:"fps"`
	Width    int            `json:"width"`

	// The frame's other fields, which photon tune shows but does not edit.
	Cols       int     `json:"cols"`
	Rows       int     `json:"rows"`
	Padding    int     `json:"padding"`
	FontSize   float64 `json:"font_size"`
	LineHeight float64 `json:"line_height"`
	Scale      int     `json:"scale"`
	MaxColors  int     `json:"max_colors"`

	Errors  []Problem `json:"errors"`
	IsDirty bool      `json:"dirty"`
	IsStale bool      `json:"stale"`
	CanUndo bool      `json:"can_undo"`
	CanRedo bool      `json:"can_redo"`
}

// BeatState is one beat's tunable timing, its zoom, and the pauses its actions hold, which are
// read-only: changing one needs a new `photon record`.
type BeatState struct {
	ID         int        `json:"id"`
	Title      string     `json:"title"`
	DurationMS int64      `json:"duration_ms"`
	HoldMS     int64      `json:"hold_ms"`
	IsCut      bool       `json:"cut"`
	Zoom       *ZoomState `json:"zoom,omitempty"`
	PausesMS   []int64    `json:"pauses_ms"`
}

// ZoomState is a beat's zoom. Factor 0 is auto, fitted to the target; the ramps carry their
// defaults when the storyboard leaves them out.
type ZoomState struct {
	Target TargetState `json:"target"`
	Factor float64     `json:"factor"`
	InMS   int64       `json:"in_ms"`
	OutMS  int64       `json:"out_ms"`
}

// TargetState is a zoom's target: a regex over the screen's rows, which match counts (0 is the
// first, -1 the last) and the area searched (empty is the whole screen).
type TargetState struct {
	Text string `json:"text"`
	Nth  int    `json:"nth"`
	Area string `json:"area"`
}

// CameraState is one camera move, in storyboard order: its Index in the storyboard's camera
// list, its Kind (zoom or pan), where it starts and ends on the output clock, and its fields.
// Factor is a zoom's destination magnification. A pan to aims at its Target or its Cell, one of
// the two; a via pan has neither and reports ViaCount, the points of its path, instead, which
// photon tune shows but does not edit.
type CameraState struct {
	Index    int          `json:"index"`
	Kind     string       `json:"kind"`
	StartMS  int64        `json:"start_ms"`
	EndMS    int64        `json:"end_ms"`
	At       MomentState  `json:"at"`
	OverMS   int64        `json:"over_ms"`
	Ease     string       `json:"ease"`
	Factor   float64      `json:"factor,omitempty"`
	Target   *TargetState `json:"target,omitempty"`
	Cell     *CellState   `json:"cell,omitempty"`
	ViaCount int          `json:"via_count"`
}

// The spellings of CameraState's Kind.
const (
	cameraKindZoom = "zoom"
	cameraKindPan  = "pan"
)

// CellState is one cell of the terminal grid a pan aims at: column X and row Y, both 0-based.
type CellState struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// CaptionState is one caption, in storyboard order: its Index in the storyboard's captions
// list, its text, where it starts and stops on the output clock, and its timing and placement.
// It ends after ForMS or at Until, exactly one of the two. Place and Align are empty for a
// caption set by position.
type CaptionState struct {
	Index   int          `json:"index"`
	Text    string       `json:"text"`
	StartMS int64        `json:"start_ms"`
	StopMS  int64        `json:"stop_ms"`
	At      MomentState  `json:"at"`
	ForMS   int64        `json:"for_ms,omitempty"`
	Until   *MomentState `json:"until,omitempty"`
	Place   string       `json:"place"`
	Align   string       `json:"align"`
}

// MomentState is a moment on the clip's output clock as the storyboard spells it: OffsetMS
// after the start of beat Beat, or, with Beat 0, after the clip's first frame.
type MomentState struct {
	Beat     int   `json:"beat"`
	OffsetMS int64 `json:"offset_ms"`
}

// SectionState is one scheduled beat on the output clock: where it starts and for how long,
// its hold, the speed its stretch after the hold plays at, and render's summary row for it.
type SectionState struct {
	Beat       int     `json:"beat"`
	StartMS    int64   `json:"start_ms"`
	DurationMS int64   `json:"duration_ms"`
	HoldMS     int64   `json:"hold_ms"`
	IsCut      bool    `json:"cut"`
	Speed      float64 `json:"speed"`
	Report     string  `json:"report"`
}

// Problem is one error standing on the storyboard: Owner is the beat, camera move or caption it
// belongs to, or the clip as a whole.
type Problem struct {
	Owner   storyboard.Owner `json:"owner"`
	Message string           `json:"message"`
}

// State reports the session as the page shows it.
func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()

	board := s.current.board
	if board == nil {
		board = s.good.board
	}
	// A take rewritten at another frame size, or one that no longer schedules, builds no
	// pipeline: the schedule shown is then the good one's.
	clipSchedule := s.good.frames.Schedule()
	if s.current.frames != nil {
		clipSchedule = s.current.frames.Schedule()
	}
	return State{
		Version:    s.version,
		Beats:      beatStates(board),
		Camera:     cameraStates(board),
		Captions:   captionStates(board),
		Sections:   sectionStates(clipSchedule),
		Warnings:   append([]string{}, clipSchedule.Warnings...),
		LengthMS:   clipSchedule.Length.Milliseconds(),
		FPS:        board.Frame.FPS,
		Width:      board.Frame.Width,
		Cols:       board.Frame.Cols,
		Rows:       board.Frame.Rows,
		Padding:    board.Frame.Padding,
		FontSize:   board.Frame.FontSize,
		LineHeight: board.Frame.LineHeight,
		Scale:      board.Frame.Scale,
		MaxColors:  board.Frame.MaxColors,
		Errors:     append([]Problem{}, s.problems...),
		IsDirty:    s.isDirty(),
		IsStale:    s.isStale,
		CanUndo:    s.history.canUndo(),
		CanRedo:    s.history.canRedo(),
	}
}

// beatStates lists board's beats in storyboard order.
func beatStates(board *storyboard.Storyboard) []BeatState {
	beats := make([]BeatState, 0, len(board.Beats))
	for _, beat := range board.Beats {
		state := BeatState{
			ID:         beat.ID,
			Title:      beat.Title,
			DurationMS: beat.Duration.Milliseconds(),
			HoldMS:     beat.Hold.Milliseconds(),
			IsCut:      beat.Cut,
			PausesMS:   pausesOf(beat),
		}
		if zoom := beat.Zoom; zoom != nil {
			state.Zoom = &ZoomState{
				Target: TargetState{Text: zoom.Target.Text, Nth: int(zoom.Target.Nth), Area: zoom.Target.Area},
				Factor: zoom.Factor,
				InMS:   zoom.In.Milliseconds(),
				OutMS:  zoom.Out.Milliseconds(),
			}
		}
		beats = append(beats, state)
	}
	return beats
}

// cameraStates lists board's camera moves in storyboard order.
func cameraStates(board *storyboard.Storyboard) []CameraState {
	clock := board.Clock()
	moves := make([]CameraState, 0, len(board.Camera))
	for index, move := range board.Camera {
		var state CameraState
		switch {
		case move.Zoom != nil:
			zoom := move.Zoom
			state = movedState(clock, cameraKindZoom, zoom.At, zoom.Over, zoom.Ease)
			state.Factor = zoom.To
		case move.Pan != nil:
			pan := move.Pan
			state = movedState(clock, cameraKindPan, pan.At, pan.Over, pan.Ease)
			state.ViaCount = len(pan.Via)
			if to := pan.To; to != nil && to.Cell != nil {
				state.Cell = &CellState{X: to.Cell.X, Y: to.Cell.Y}
			} else if to != nil {
				state.Target = &TargetState{Text: to.Text, Nth: int(to.Nth), Area: to.Area}
			}
		}
		state.Index = index
		moves = append(moves, state)
	}
	return moves
}

// movedState is the part of a camera move's state every form shares: its kind, its timing as
// the storyboard spells it, and where it starts and ends on clock, the board's output clock.
func movedState(clock storyboard.OutputClock, kind string, at storyboard.Moment, over time.Duration, ease storyboard.Ease) CameraState {
	start := clipMS(clock, at)
	return CameraState{
		Kind:    kind,
		StartMS: start,
		EndMS:   start + over.Milliseconds(),
		At:      momentState(at),
		OverMS:  over.Milliseconds(),
		Ease:    string(ease),
	}
}

// captionStates lists board's captions in storyboard order.
func captionStates(board *storyboard.Storyboard) []CaptionState {
	clock := board.Clock()
	captions := make([]CaptionState, 0, len(board.Captions))
	for index, caption := range board.Captions {
		start := clipMS(clock, caption.At)
		state := CaptionState{
			Index:   index,
			Text:    caption.Text,
			StartMS: start,
			StopMS:  start + caption.For.Milliseconds(),
			At:      momentState(caption.At),
			ForMS:   caption.For.Milliseconds(),
			Place:   string(caption.Place),
			Align:   string(caption.Align),
		}
		if until := caption.Until; until != nil {
			state.StopMS = clipMS(clock, *until)
			untilState := momentState(*until)
			state.Until = &untilState
		}
		captions = append(captions, state)
	}
	return captions
}

// momentState spells moment as the page reads it.
func momentState(moment storyboard.Moment) MomentState {
	return MomentState{Beat: moment.Beat, OffsetMS: moment.Offset.Milliseconds()}
}

// clipMS resolves moment on clock, a board's output clock, in whole milliseconds. Every board
// State reports passed validation, which resolves each of its moments, so a moment that does not
// resolve cannot arise; were one to, it reports the clip's start.
func clipMS(clock storyboard.OutputClock, moment storyboard.Moment) int64 {
	at, err := clock.Resolve(moment)
	if err != nil {
		return 0
	}
	return at.Milliseconds()
}

// pausesOf lists how long each of beat's pause actions idles, in action order.
func pausesOf(beat storyboard.Beat) []int64 {
	pauses := []int64{}
	for _, action := range beat.Do {
		if action.Pause != nil {
			pauses = append(pauses, action.Pause.For.Milliseconds())
		}
	}
	return pauses
}

// sectionStates lists the schedule's sections in the order the take started the beats.
func sectionStates(clipSchedule *schedule.Schedule) []SectionState {
	sections := make([]SectionState, 0, len(clipSchedule.Sections))
	for _, section := range clipSchedule.Sections {
		sections = append(sections, SectionState{
			Beat:       section.Beat,
			StartMS:    section.OutStart.Milliseconds(),
			DurationMS: section.Duration.Milliseconds(),
			HoldMS:     section.Hold.Milliseconds(),
			IsCut:      section.Cut,
			Speed:      section.Speed,
			Report:     render.BeatReport(section),
		})
	}
	return sections
}

// problemsOf splits err into one problem per error it joins — each rule a validation error
// lists counts once — attributing each to the owner its [storyboard.LocatedError] names.
func problemsOf(err error, board *storyboard.Storyboard) []Problem {
	if err == nil {
		return nil
	}
	joined, isJoined := err.(interface{ Unwrap() []error })
	if !isJoined {
		return []Problem{{Owner: ownerOn(storyboard.OwnerOf(err), board), Message: err.Error()}}
	}
	var problems []Problem
	for _, part := range joined.Unwrap() {
		problems = append(problems, problemsOf(part, board)...)
	}
	return problems
}

// ownerOn returns owner, or the clip when owner is a beat board does not have, so the problem
// still shows on a panel.
func ownerOn(owner storyboard.Owner, board *storyboard.Storyboard) storyboard.Owner {
	if owner.Kind != storyboard.OwnerBeat {
		return owner
	}
	isOnBoard := board != nil && slices.ContainsFunc(board.Beats, func(beat storyboard.Beat) bool {
		return beat.ID == owner.Index
	})
	if !isOnBoard {
		return storyboard.Owner{Kind: storyboard.OwnerClip}
	}
	return owner
}
