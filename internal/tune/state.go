package tune

import (
	"image"
	"math"
	"slices"
	"time"

	"github.com/airiclenz/photon/internal/render"
	"github.com/airiclenz/photon/internal/schedule"
	"github.com/airiclenz/photon/internal/storyboard"
)

// State is everything the page shows of a session, as `/api/state` sends it. Durations are
// whole milliseconds, the spelling an [Edit] takes. Beats, Camera and Captions come from the
// current storyboard, and so does Fields, the description of every field the page shows; Sections,
// Warnings and Length from its schedule, or from the last good pipeline's while the current bytes
// do not schedule; a caption's laid-out box from the current bytes' pipeline, only while it builds
// in full. Version rises on every change, so a page that saw it already has nothing new
// to draw. Build is the build string of the photon serving the page, as `photon version` prints
// it.
type State struct {
	Version  int            `json:"version"`
	Build    string         `json:"build"`
	Beats    []BeatState    `json:"beats"`
	Camera   []CameraState  `json:"camera"`
	Captions []CaptionState `json:"captions"`
	Sections []SectionState `json:"sections"`
	Warnings []string       `json:"warnings"`
	LengthMS int64          `json:"length_ms"`
	FPS      int            `json:"fps"`
	Width    int            `json:"width"`

	// The frame's other fields: padding, font_size, line_height and scale, which photon tune
	// edits as frame.<key>, and cols, rows and max_colors, which it shows but does not edit.
	Cols       int     `json:"cols"`
	Rows       int     `json:"rows"`
	Padding    int     `json:"padding"`
	FontSize   float64 `json:"font_size"`
	LineHeight float64 `json:"line_height"`
	Scale      int     `json:"scale"`
	MaxColors  int     `json:"max_colors"`

	// Fields describes every field of the storyboard's owners, a read-only frame field under the
	// key it is sent as above.
	Fields []storyboard.Field `json:"fields"`

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
// Factor is a zoom's destination magnification, and its Target, when it has one, what it aims
// at. A pan to aims at its Target or its Cell, one of the two; a via pan has neither and reports
// Via, the points of its path in order, each spelled as a pan to's destination is, and ViaCount,
// how many there are. Where the path runs on a frame is [Session.Path]'s.
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
	Via      []PointState `json:"via,omitempty"`
	ViaCount int          `json:"via_count"`
}

// PointState is one point a pan aims at: a Target or a Cell, one of the two.
type PointState struct {
	Target *TargetState `json:"target,omitempty"`
	Cell   *CellState   `json:"cell,omitempty"`
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
// list, its text, where it starts and stops on the output clock, its timing, placement and style,
// and where the render pipeline lays it out. It ends after ForMS or at Until, exactly one of the
// two. Place and Align are empty for a caption set by Position, which is nil for one set by place
// and align and moved by Offset; both are in design pixels. The style carries the defaults the
// storyboard leaves out: FontSize in the unit of frame.font_size, colours as #rrggbb.
//
// Box is the box the caption fills on the shipped frame, in shipped pixels, and BoxPosition its
// top-left in design pixels — the Position that would place the box where it is; both are nil
// while the current bytes build no full pipeline.
type CaptionState struct {
	Index       int          `json:"index"`
	Text        string       `json:"text"`
	StartMS     int64        `json:"start_ms"`
	StopMS      int64        `json:"stop_ms"`
	At          MomentState  `json:"at"`
	ForMS       int64        `json:"for_ms,omitempty"`
	Until       *MomentState `json:"until,omitempty"`
	Place       string       `json:"place"`
	Align       string       `json:"align"`
	Offset      PixelsState  `json:"offset"`
	Position    *PixelsState `json:"position,omitempty"`
	FontSize    float64      `json:"font_size"`
	FontWeight  string       `json:"font_weight"`
	Color       string       `json:"color"`
	BoxColor    string       `json:"box_color"`
	BoxOpacity  float64      `json:"box_opacity"`
	FadeInMS    int64        `json:"fade_in_ms"`
	FadeOutMS   int64        `json:"fade_out_ms"`
	Box         *BoxState    `json:"box,omitempty"`
	BoxPosition *PixelsState `json:"box_position,omitempty"`
}

// PixelsState is a pair of lengths in design pixels: X to the right and Y down.
type PixelsState struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// BoxState is a rectangle of the shipped frame in shipped pixels: its top-left X and Y, its
// width W and its height H.
type BoxState struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
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
		Build:      s.photonBuild,
		Beats:      beatStates(board),
		Camera:     cameraStates(board),
		Captions:   captionStates(board, s.current.frames),
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
		Fields:     storyboard.Fields(board),
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
				Target: TargetState{
					Text: zoom.Target.Text, Nth: int(zoom.Target.Nth), Area: zoom.Target.Area,
				},
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
	moves := make([]CameraState, 0, len(board.Camera))
	for index, move := range board.Camera {
		var state CameraState
		switch {
		case move.Zoom != nil:
			zoom := move.Zoom
			state = movedState(cameraKindZoom, zoom.Start(), zoom.At, zoom.Over, zoom.Ease)
			state.Factor = zoom.To
			if target := zoom.Target; target != nil {
				state.Target = &TargetState{
					Text: target.Text, Nth: int(target.Nth), Area: target.Area,
				}
			}
		case move.Pan != nil:
			pan := move.Pan
			state = movedState(cameraKindPan, pan.Start(), pan.At, pan.Over, pan.Ease)
			state.ViaCount = len(pan.Via)
			if to := pan.To; to != nil {
				aimed := pointState(*to)
				state.Target, state.Cell = aimed.Target, aimed.Cell
			}
			for _, point := range pan.Via {
				state.Via = append(state.Via, pointState(point))
			}
		}
		state.Index = index
		moves = append(moves, state)
	}
	return moves
}

// pointState spells point, a pan's destination or one of its via points, as the page reads it.
func pointState(point storyboard.Point) PointState {
	if point.Cell != nil {
		return PointState{Cell: &CellState{X: point.Cell.X, Y: point.Cell.Y}}
	}
	return PointState{Target: &TargetState{Text: point.Text, Nth: int(point.Nth), Area: point.Area}}
}

// movedState is the part of a camera move's state every form shares: its kind, its timing as
// the storyboard spells it, and where it starts and ends on the output clock, from start, its
// resolved At.
func movedState(
	kind string, start time.Duration, at storyboard.Moment, over time.Duration,
	ease storyboard.Ease,
) CameraState {
	return CameraState{
		Kind:    kind,
		StartMS: start.Milliseconds(),
		EndMS:   (start + over).Milliseconds(),
		At:      momentState(at),
		OverMS:  over.Milliseconds(),
		Ease:    string(ease),
	}
}

// captionStates lists board's captions in storyboard order, each with the box frames lays it
// out in. frames is the pipeline built on board, or nil when none was; a partial one lays out
// no box.
func captionStates(board *storyboard.Storyboard, frames *render.Pipeline) []CaptionState {
	var (
		boxes []image.Rectangle
		unit  float64
	)
	if frames != nil {
		boxes, unit = frames.CaptionBoxes(), frames.Unit()
	}
	captions := make([]CaptionState, 0, len(board.Captions))
	for index, caption := range board.Captions {
		state := captionState(caption)
		state.Index = index
		if index < len(boxes) && unit > 0 {
			box := boxes[index]
			state.Box = &BoxState{X: box.Min.X, Y: box.Min.Y, W: box.Dx(), H: box.Dy()}
			state.BoxPosition = &PixelsState{
				X: int(math.Round(float64(box.Min.X) / unit)),
				Y: int(math.Round(float64(box.Min.Y) / unit)),
			}
		}
		captions = append(captions, state)
	}
	return captions
}

// captionState is caption's state as the storyboard sets it: its timing, placement and style.
func captionState(caption storyboard.Caption) CaptionState {
	state := CaptionState{
		Text:       caption.Text,
		StartMS:    caption.Start().Milliseconds(),
		StopMS:     caption.Stop().Milliseconds(),
		At:         momentState(caption.At),
		ForMS:      caption.For.Milliseconds(),
		Place:      string(caption.Place),
		Align:      string(caption.Align),
		Offset:     PixelsState{X: caption.Offset.X, Y: caption.Offset.Y},
		FontSize:   caption.Font.Size,
		FontWeight: string(caption.Font.Weight),
		Color:      caption.Color,
		BoxColor:   caption.Box.Color,
		FadeInMS:   caption.Fade.In.Milliseconds(),
		FadeOutMS:  caption.Fade.Out.Milliseconds(),
	}
	if until := caption.Until; until != nil {
		untilState := momentState(*until)
		state.Until = &untilState
	}
	if position := caption.Position; position != nil {
		state.Position = &PixelsState{X: position.X, Y: position.Y}
	}
	if opacity := caption.Box.Opacity; opacity != nil {
		state.BoxOpacity = *opacity
	}
	return state
}

// momentState spells moment as the page reads it.
func momentState(moment storyboard.Moment) MomentState {
	return MomentState{Beat: moment.Beat, OffsetMS: moment.Offset.Milliseconds()}
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

// ownerOn returns owner, or the clip when owner is a beat, camera move or caption board does not
// have, so the problem still shows on a panel: an outside write that fails to parse adds what the
// last good board, the one the page shows, lacks.
func ownerOn(owner storyboard.Owner, board *storyboard.Storyboard) storyboard.Owner {
	if owner.Kind == storyboard.OwnerClip {
		return owner
	}
	if board == nil || !hasOwner(board, owner) {
		return storyboard.Owner{Kind: storyboard.OwnerClip}
	}
	return owner
}

// hasOwner reports whether board has the beat, camera move or caption owner names.
func hasOwner(board *storyboard.Storyboard, owner storyboard.Owner) bool {
	switch owner.Kind {
	case storyboard.OwnerBeat:
		return slices.ContainsFunc(board.Beats, func(beat storyboard.Beat) bool {
			return beat.ID == owner.Index
		})
	case storyboard.OwnerCamera:
		return owner.Index >= 0 && owner.Index < len(board.Camera)
	case storyboard.OwnerCaption:
		return owner.Index >= 0 && owner.Index < len(board.Captions)
	default:
		return false
	}
}
