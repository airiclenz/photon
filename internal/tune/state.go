package tune

import (
	"regexp"
	"slices"
	"strconv"

	"github.com/airiclenz/photon/internal/render"
	"github.com/airiclenz/photon/internal/schedule"
	"github.com/airiclenz/photon/internal/storyboard"
)

// State is everything the page shows of a session, as `/api/state` sends it. Durations are
// whole milliseconds, the spelling an [Edit] takes. Beats come from the current storyboard;
// Sections, Warnings and Length from its schedule, or from the last good pipeline's while the
// current bytes do not schedule. Version rises on every change, so a page that saw it already
// has nothing new to draw.
type State struct {
	Version  int            `json:"version"`
	Beats    []BeatState    `json:"beats"`
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

// Problem is one error standing on the storyboard: Beat is the beat it names, or 0 for one on
// the clip as a whole.
type Problem struct {
	Beat    int    `json:"beat"`
	Message string `json:"message"`
}

// beatMention finds the beat an error message is about: `beat 2: hold: …`.
var beatMention = regexp.MustCompile(`\bbeat (\d+)\b`)

// State reports the session as the page shows it.
func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()

	board := s.current.board
	if board == nil {
		board = s.good.board
	}
	clipSchedule := s.current.schedule
	if clipSchedule == nil {
		clipSchedule = s.good.schedule
	}
	return State{
		Version:    s.version,
		Beats:      beatStates(board),
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
// lists counts once — attributing each to the beat of board its message names.
func problemsOf(err error, board *storyboard.Storyboard) []Problem {
	if err == nil {
		return nil
	}
	joined, isJoined := err.(interface{ Unwrap() []error })
	if !isJoined {
		return []Problem{{Beat: beatNamed(err.Error(), board), Message: err.Error()}}
	}
	var problems []Problem
	for _, part := range joined.Unwrap() {
		problems = append(problems, problemsOf(part, board)...)
	}
	return problems
}

// beatNamed returns the id of the beat of board that message names first, or 0 when it names
// none board has.
func beatNamed(message string, board *storyboard.Storyboard) int {
	match := beatMention.FindStringSubmatch(message)
	if match == nil || board == nil {
		return 0
	}
	id, err := strconv.Atoi(match[1])
	if err != nil {
		return 0
	}
	if !slices.ContainsFunc(board.Beats, func(beat storyboard.Beat) bool { return beat.ID == id }) {
		return 0
	}
	return id
}
