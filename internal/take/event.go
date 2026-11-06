package take

import "time"

// EventKind names what an [Event] records.
type EventKind string

// The kinds of event a take logs.
const (
	// EventBeatStart marks a beat beginning; its Text is the beat's title.
	EventBeatStart EventKind = "beat-start"
	// EventActionStart marks one of the beat's actions beginning.
	EventActionStart EventKind = "action-start"
	// EventActionEnd marks that action finishing.
	EventActionEnd EventKind = "action-end"
	// EventTarget records the Box a click's or a drag's target resolved to.
	EventTarget EventKind = "target"
	// EventClick records the Cell a click was sent to.
	EventClick EventKind = "click"
	// EventPress records the Cell a drag pressed its button at; its Form is `drag`.
	EventPress EventKind = "press"
	// EventMove records one Cell a drag moved the held button onto; its Form is `drag`. The move
	// that reaches a via point has the Text [MoveVia].
	EventMove EventKind = "move"
	// EventRelease records the Cell a drag released the button at; its Form is `drag`.
	EventRelease EventKind = "release"
)

// MoveVia is the Text of the [EventMove] that reaches one of a drag's via points: the last move
// of every segment of the drag's path but the final one. A segment that moves no cell (its via
// point is the point before it) logs no move, so marks no via point.
const MoveVia = "via"

// Event is one thing photon did during the take, stamped on the same clock as the snapshots.
// Every event names its Beat (the storyboard beat id); action, target, click, press, move and
// release events also name the Action (its 0-based index in the beat's action list) and its Form
// (`type`, `key`, `click`, `drag`, `wait`, `pause`). Text is the event's free text — a
// beat-start's title, an action's typed string or key name, on a drag's target events `from`,
// `via` or `to`, naming which point of the drag the Box is, and on a move [MoveVia] when the move
// reaches a via point. A target event carries the resolved Box; a click, press, move or release
// event the Cell it was sent to; both are nil on every other kind.
type Event struct {
	At     time.Duration `json:"at"`
	Kind   EventKind     `json:"kind"`
	Beat   int           `json:"beat"`
	Action int           `json:"action,omitempty"`
	Form   string        `json:"form,omitempty"`
	Text   string        `json:"text,omitempty"`
	Box    *Box          `json:"box,omitempty"`
	Cell   *Point        `json:"cell,omitempty"`
}

// Box is a rectangle of terminal cells: its top-left cell (0-based) and its size in cells.
type Box struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Point is one terminal cell (0-based column X, row Y).
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}
