package engine

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/airiclenz/photon/internal/keys"
	"github.com/airiclenz/photon/internal/storyboard"
	"github.com/airiclenz/photon/internal/take"
	"github.com/airiclenz/photon/internal/target"
)

// fakeCols is how wide a fake screen is: wider than any line a test draws.
const fakeCols = 40

// fakeScreen is what the fake terminal shows unless a test changes it.
var fakeScreen = []string{"title", "  alpha", "  beta", "ready"}

// fakeTerminal is the engine's test adapter: it shows a screen the test sets and records every
// write, every event and every screen a match recorded.
type fakeTerminal struct {
	mu      sync.Mutex
	screen  take.Snapshot
	sent    [][]byte
	events  []take.Event
	matched int
	done    chan struct{}
}

func newFakeTerminal(lines ...string) *fakeTerminal {
	return &fakeTerminal{screen: screenOf(lines...), done: make(chan struct{})}
}

func (f *fakeTerminal) Send(p []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, append([]byte(nil), p...))
	return nil
}

func (f *fakeTerminal) Current() take.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.screen
}

func (f *fakeTerminal) Match(match func(take.Snapshot) bool) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !match(f.screen) {
		return false
	}
	f.matched++
	return true
}

func (f *fakeTerminal) Done() <-chan struct{} { return f.done }

func (f *fakeTerminal) Event(event take.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
}

func (f *fakeTerminal) show(lines ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.screen = screenOf(lines...)
}

func (f *fakeTerminal) sentChunks() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	chunks := make([]string, 0, len(f.sent))
	for _, chunk := range f.sent {
		chunks = append(chunks, string(chunk))
	}
	return chunks
}

func (f *fakeTerminal) loggedEvents() []take.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]take.Event(nil), f.events...)
}

// screenOf is a fakeCols-wide snapshot showing lines, one narrow grapheme per cell.
func screenOf(lines ...string) take.Snapshot {
	cells := make([][]take.Cell, len(lines))
	for y, line := range lines {
		row := make([]take.Cell, fakeCols)
		for x := range row {
			row[x] = take.Cell{Width: 1}
		}
		for x, character := range []rune(line) {
			row[x] = take.Cell{Grapheme: string(character), Width: 1}
		}
		cells[y] = row
	}
	return take.Snapshot{Cells: cells}
}

func keyBytes(t *testing.T, name string) string {
	t.Helper()
	sequence, isKnown := keys.Lookup(name)
	if !isKnown {
		t.Fatalf("keys.Lookup(%q): unknown", name)
	}
	return string(sequence)
}

func isFlat() *bool {
	flat := false
	return &flat
}

// beatOf is beat 7, "the beat", doing actions.
func beatOf(actions ...storyboard.Action) storyboard.Beat {
	return storyboard.Beat{ID: 7, Title: "the beat", Do: actions}
}

// stepEvents are the beat-start, action-start, middle and action-end events of beat 7's only
// action, of form and text.
func stepEvents(form, text string, middle ...take.Event) []take.Event {
	step := take.Event{Beat: 7, Form: form, Text: text}
	events := []take.Event{{Kind: take.EventBeatStart, Beat: 7, Text: "the beat"}}
	start, end := step, step
	start.Kind, end.Kind = take.EventActionStart, take.EventActionEnd
	events = append(events, start)
	events = append(events, middle...)
	return append(events, end)
}

func TestEngine_Run_ActionForms(t *testing.T) {
	t.Parallel()
	betaBox := take.Box{X: 2, Y: 2, W: 4, H: 1}
	betaCentre := take.Point{X: 3, Y: 2}
	clickStep := take.Event{Beat: 7, Form: formClick}
	targetEvent, clickEvent := clickStep, clickStep
	targetEvent.Kind, targetEvent.Box = take.EventTarget, &betaBox
	clickEvent.Kind, clickEvent.Cell = take.EventClick, &betaCentre
	press, release := string(keys.Click(3, 2)), string(keys.Release(3, 2))

	tests := []struct {
		name        string
		action      storyboard.Action
		wantSent    []string
		wantEvents  []take.Event
		wantMatched int
	}{
		{
			name:       "flat type is one write",
			action:     storyboard.Action{Type: &storyboard.TypeAction{Text: "hi there", Humanize: isFlat()}},
			wantSent:   []string{"hi there"},
			wantEvents: stepEvents(formType, "hi there"),
		},
		{
			name:       "humanized type is one write per character",
			action:     storyboard.Action{Type: &storyboard.TypeAction{Text: "a—b"}},
			wantSent:   []string{"a", "—", "b"},
			wantEvents: stepEvents(formType, "a—b"),
		},
		{
			name:       "key once",
			action:     storyboard.Action{Key: &storyboard.KeyAction{Name: "enter"}},
			wantSent:   []string{keyBytes(t, "enter")},
			wantEvents: stepEvents(formKey, "enter"),
		},
		{
			name:       "key with repeat",
			action:     storyboard.Action{Key: &storyboard.KeyAction{Name: "shift+tab", Repeat: 3}},
			wantSent:   []string{keyBytes(t, "shift+tab"), keyBytes(t, "shift+tab"), keyBytes(t, "shift+tab")},
			wantEvents: stepEvents(formKey, "shift+tab"),
		},
		{
			name:       "click at the target's centre",
			action:     storyboard.Action{Click: &storyboard.ClickAction{Target: storyboard.Target{Text: "beta"}}},
			wantSent:   []string{press, release},
			wantEvents: stepEvents(formClick, "", targetEvent, clickEvent),
		},
		{
			name: "click with times",
			action: storyboard.Action{Click: &storyboard.ClickAction{
				Target: storyboard.Target{Text: "beta"}, Times: 2,
			}},
			wantSent:   []string{press, release, press, release},
			wantEvents: stepEvents(formClick, "", targetEvent, clickEvent, clickEvent),
		},
		{
			name: "wait for a shown screen records it",
			action: storyboard.Action{Wait: &storyboard.WaitAction{
				Screen: `(?m)^ready$`, Timeout: time.Second,
			}},
			wantEvents:  stepEvents(formWait, ""),
			wantMatched: 1,
		},
		{
			name: "wait for a gone screen records it",
			action: storyboard.Action{Wait: &storyboard.WaitAction{
				Screen: "gamma", Gone: true, Timeout: time.Second,
			}},
			wantEvents:  stepEvents(formWait, ""),
			wantMatched: 1,
		},
		{
			name:       "pause",
			action:     storyboard.Action{Pause: &storyboard.PauseAction{For: 10 * time.Millisecond}},
			wantEvents: stepEvents(formPause, ""),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			term := newFakeTerminal(fakeScreen...)

			err := New(term, target.Resolver{}).Run(context.Background(), []storyboard.Beat{beatOf(tt.action)})

			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got := term.sentChunks(); !slices.Equal(got, tt.wantSent) {
				t.Errorf("sent %q, want %q", got, tt.wantSent)
			}
			if got := term.loggedEvents(); !reflect.DeepEqual(got, tt.wantEvents) {
				t.Errorf("events:\n got %+v\nwant %+v", got, tt.wantEvents)
			}
			if term.matched != tt.wantMatched {
				t.Errorf("recorded %d matched screens, want %d", term.matched, tt.wantMatched)
			}
		})
	}
}

func TestEngine_Run_WaitSeesTheScreenChange(t *testing.T) {
	t.Parallel()
	term := newFakeTerminal(fakeScreen...)
	go func() {
		time.Sleep(5 * waitPollInterval)
		term.show("you typed: hello")
	}()
	wait := storyboard.Action{Wait: &storyboard.WaitAction{Screen: "you typed: hello", Timeout: 5 * time.Second}}

	err := New(term, target.Resolver{}).Run(context.Background(), []storyboard.Beat{beatOf(wait)})

	if err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestEngine_Run_Failures(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		action  storyboard.Action
		exited  bool
		wantErr error
		wantMsg string
	}{
		{
			name:    "wait timeout",
			action:  storyboard.Action{Wait: &storyboard.WaitAction{Screen: "never", Timeout: 50 * time.Millisecond}},
			wantMsg: "timed out after 50ms",
		},
		{
			name:    "wait on an exited program",
			action:  storyboard.Action{Wait: &storyboard.WaitAction{Screen: "never", Timeout: 5 * time.Second}},
			exited:  true,
			wantMsg: "the program exited",
		},
		{
			name:    "wait timeout out of bounds",
			action:  storyboard.Action{Wait: &storyboard.WaitAction{Screen: "ready", Timeout: 181 * time.Second}},
			wantMsg: "outside (0, 3m0s]",
		},
		{
			name:    "missing click target",
			action:  storyboard.Action{Click: &storyboard.ClickAction{Target: storyboard.Target{Text: "delta"}}},
			wantErr: target.ErrNotOnScreen,
			wantMsg: "target /delta/",
		},
		{
			name:    "unknown key",
			action:  storyboard.Action{Key: &storyboard.KeyAction{Name: "hyper"}},
			wantMsg: `unknown key name "hyper"`,
		},
		{
			name:    "no form",
			action:  storyboard.Action{},
			wantErr: errNoForm,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			term := newFakeTerminal(fakeScreen...)
			if tt.exited {
				close(term.done)
			}
			never := storyboard.Action{Type: &storyboard.TypeAction{Text: "never sent", Humanize: isFlat()}}
			beats := []storyboard.Beat{beatOf(tt.action, never), {ID: 8, Title: "never started"}}

			err := New(term, target.Resolver{}).Run(context.Background(), beats)

			var beatErr *BeatError
			if !errors.As(err, &beatErr) {
				t.Fatalf("Run = %v, want a *BeatError", err)
			}
			if beatErr.Beat != 7 || beatErr.Action != 0 {
				t.Errorf("failed at beat %d do[%d], want beat 7 do[0]", beatErr.Beat, beatErr.Action)
			}
			if !strings.HasPrefix(err.Error(), "beat 7 (the beat): do[0]") {
				t.Errorf("error %q does not name beat 7 and do[0]", err)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("error %q does not wrap %v", err, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not say %q", err, tt.wantMsg)
			}
			if sent := term.sentChunks(); len(sent) != 0 {
				t.Errorf("sent %q after the failure, want nothing", sent)
			}
			if events := term.loggedEvents(); len(events) != 2 || events[1].Kind != take.EventActionStart {
				t.Errorf("events %+v, want only beat-start and action-start", events)
			}
		})
	}
}

func TestEngine_Run_Cancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pause := storyboard.Action{Pause: &storyboard.PauseAction{For: time.Hour}}

	err := New(newFakeTerminal(fakeScreen...), target.Resolver{}).Run(ctx, []storyboard.Beat{beatOf(pause)})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run = %v, want context.Canceled", err)
	}
}
