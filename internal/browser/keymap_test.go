package browser

import (
	"slices"
	"strings"
	"testing"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp/kb"

	"github.com/airiclenz/photon/internal/keys"
)

func TestKeyEvents_EveryKeyName_IsAKeyDownAndAKeyUpOfAKnownKey(t *testing.T) {
	for _, name := range keys.Names() {
		t.Run(name, func(t *testing.T) {
			events, err := keyEvents(name)

			if err != nil {
				t.Fatalf("keyEvents(%q): %v", name, err)
			}
			if len(events) < 2 {
				t.Fatalf("keyEvents(%q) = %d events, want a keyDown and a keyUp", name, len(events))
			}
			first, last := events[0], events[len(events)-1]
			if first.Type != kb.KeyDown || last.Type != kb.KeyUp {
				t.Errorf("events run %s to %s, want keyDown to keyUp", first.Type, last.Type)
			}
			if first.Key == "" || first.Key == "Unidentified" || first.Code == "" {
				t.Errorf("key %q code %q, want a known DOM key and code", first.Key, first.Code)
			}
			if first.WindowsVirtualKeyCode == 0 {
				t.Errorf("no virtual key code")
			}
		})
	}
}

func TestKeyEvents(t *testing.T) {
	tests := []struct {
		name      string
		wantKey   string
		wantText  string
		wantMods  kb.Modifier
		wantChars int
	}{
		{"enter", "Enter", "\r", kb.ModifierNone, 1},
		{"space", " ", " ", kb.ModifierNone, 1},
		{"esc", "Escape", "", kb.ModifierNone, 0},
		{"shift+tab", "Tab", "", kb.ModifierShift, 0},
		{"ctrl+c", "c", "", kb.ModifierCtrl, 0},
		{"alt+down", "ArrowDown", "", kb.ModifierAlt, 0},
		{"pgup", "PageUp", "", kb.ModifierNone, 0},
		{"f12", "F12", "", kb.ModifierNone, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := keyEvents(tt.name)

			if err != nil {
				t.Fatalf("keyEvents(%q): %v", tt.name, err)
			}
			if got := events[0].Key; got != tt.wantKey {
				t.Errorf("key = %q, want %q", got, tt.wantKey)
			}
			for _, event := range events {
				if got := kb.Modifier(event.Modifiers); got != tt.wantMods {
					t.Errorf("%s modifiers = %s, want %s", event.Type, got, tt.wantMods)
				}
			}
			chars := charsOf(events)
			if len(chars) != tt.wantChars || strings.Join(chars, "") != tt.wantText {
				t.Errorf("chars = %q, want %d typing %q", chars, tt.wantChars, tt.wantText)
			}
		})
	}
}

func TestKeyEvents_UnknownName_IsAnErrorNamingIt(t *testing.T) {
	_, err := keyEvents("ctrl+z")

	if err == nil || !strings.Contains(err.Error(), `"ctrl+z"`) {
		t.Errorf("keyEvents error = %v, want one naming %q", err, "ctrl+z")
	}
}

func TestTypeEvents_EachRuneIsAKeyDownACharAndAKeyUp(t *testing.T) {
	events := typeEvents("aB\n")

	var downs []string
	for _, event := range events {
		if event.Type == kb.KeyDown {
			downs = append(downs, event.Key)
		}
	}
	if want := []string{"a", "B", "Enter"}; !slices.Equal(downs, want) {
		t.Errorf("keyDowns = %q, want %q", downs, want)
	}
	if got, want := charsOf(events), []string{"a", "B", "\r"}; !slices.Equal(got, want) {
		t.Errorf("chars = %q, want %q", got, want)
	}
	if got := len(events); got != 9 {
		t.Errorf("%d events, want 9", got)
	}
}

func TestMouseEvent(t *testing.T) {
	tests := []struct {
		name  string
		event keys.Pointer
		want  input.DispatchMouseEventParams
	}{
		{
			name:  "a left press with no modifier",
			event: keys.Pointer{Kind: keys.PointerPress, X: 12, Y: 34},
			want: input.DispatchMouseEventParams{
				Type: input.DispatchMouseEventTypeMousePressed, X: 12, Y: 34,
				Button: input.MouseButtonLeft, Buttons: 1, ClickCount: 1,
			},
		},
		{
			name: "a right move with shift and ctrl held",
			event: keys.Pointer{
				Kind: keys.PointerMove, X: 5, Y: 6,
				Mouse: keys.Mouse{Button: keys.RightButton, Mods: keys.Shift | keys.Ctrl},
			},
			want: input.DispatchMouseEventParams{
				Type: input.DispatchMouseEventTypeMouseMoved, X: 5, Y: 6,
				Modifiers: int64(kb.ModifierShift | kb.ModifierCtrl),
				Button:    input.MouseButtonRight, Buttons: 2,
			},
		},
		{
			name: "a middle release with alt held",
			event: keys.Pointer{
				Kind: keys.PointerRelease, X: 7, Y: 8,
				Mouse: keys.Mouse{Button: keys.MiddleButton, Mods: keys.Alt},
			},
			want: input.DispatchMouseEventParams{
				Type: input.DispatchMouseEventTypeMouseReleased, X: 7, Y: 8,
				Modifiers: int64(kb.ModifierAlt),
				Button:    input.MouseButtonMiddle, ClickCount: 1,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := mouseEvent(tt.event)

			if err != nil {
				t.Fatalf("mouseEvent: %v", err)
			}
			if got != tt.want {
				t.Errorf("mouseEvent = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestMouseEvent_UnknownKindOrButton_IsAnError(t *testing.T) {
	tests := []struct {
		name  string
		event keys.Pointer
	}{
		{"unknown kind", keys.Pointer{Kind: keys.PointerRelease + 1}},
		{"unknown button", keys.Pointer{Mouse: keys.Mouse{Button: keys.RightButton + 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mouseEvent(tt.event)

			if err == nil {
				t.Errorf("mouseEvent(%+v) = nil error, want one", tt.event)
			}
		})
	}
}

// charsOf returns the text of every char event among events, in order.
func charsOf(events []input.DispatchKeyEventParams) []string {
	var chars []string
	for _, event := range events {
		if event.Type == kb.KeyChar {
			chars = append(chars, event.Text)
		}
	}
	return chars
}
