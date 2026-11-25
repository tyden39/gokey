package main

import (
	"testing"
	"time"

	"gokey/engine"

	"gokey/internal/appconfig"
	"gokey/internal/hyprland"
	"gokey/internal/inputmethod"
	"gokey/internal/keymap"
)

func TestSurroundingDeleteBytes(t *testing.T) {
	surrounding := "before việt after"
	cursor := len("before việt")

	got, ok := surroundingDeleteBytes(
		"việt",
		2,
		surrounding,
		cursor,
		cursor,
		7,
		7,
		true,
		false,
	)
	if !ok {
		t.Fatal("safe surrounding deletion was refused")
	}
	if want := uint32(len("ệt")); got != want {
		t.Fatalf("delete bytes = %d, want %d", got, want)
	}
}

func TestSurroundingDeleteBytesASCII(t *testing.T) {
	got, ok := surroundingDeleteBytes(
		"test",
		2,
		"before test",
		len("before test"),
		len("before test"),
		3,
		3,
		true,
		false,
	)
	if !ok || got != 2 {
		t.Fatalf("ASCII delete = (%d, %v), want (2, true)", got, ok)
	}
}

func TestSurroundingDeleteBytesRefusesUnsafeState(t *testing.T) {
	baseCursor := len("before việt")
	tests := []struct {
		name           string
		oldWord        string
		deleteRunes    int
		surrounding    string
		cursor         int
		anchor         int
		snapshotSerial uint32
		currentSerial  uint32
		valid          bool
		dirty          bool
	}{
		{name: "invalid snapshot", oldWord: "việt", deleteRunes: 1, surrounding: "before việt", cursor: baseCursor, anchor: baseCursor, snapshotSerial: 2, currentSerial: 2, valid: false},
		{name: "dirty snapshot", oldWord: "việt", deleteRunes: 1, surrounding: "before việt", cursor: baseCursor, anchor: baseCursor, snapshotSerial: 2, currentSerial: 2, valid: true, dirty: true},
		{name: "stale serial", oldWord: "việt", deleteRunes: 1, surrounding: "before việt", cursor: baseCursor, anchor: baseCursor, snapshotSerial: 1, currentSerial: 2, valid: true},
		{name: "selection", oldWord: "việt", deleteRunes: 1, surrounding: "before việt", cursor: baseCursor, anchor: baseCursor - 1, snapshotSerial: 2, currentSerial: 2, valid: true},
		{name: "suffix mismatch", oldWord: "việt", deleteRunes: 2, surrounding: "before viet", cursor: len("before viet"), anchor: len("before viet"), snapshotSerial: 2, currentSerial: 2, valid: true},
		{name: "cursor outside", oldWord: "việt", deleteRunes: 1, surrounding: "việt", cursor: len("việt") + 1, anchor: len("việt") + 1, snapshotSerial: 2, currentSerial: 2, valid: true},
		{name: "too many runes", oldWord: "việt", deleteRunes: 5, surrounding: "việt", cursor: len("việt"), anchor: len("việt"), snapshotSerial: 2, currentSerial: 2, valid: true},
		{name: "zero runes", oldWord: "việt", deleteRunes: 0, surrounding: "việt", cursor: len("việt"), anchor: len("việt"), snapshotSerial: 2, currentSerial: 2, valid: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got, ok := surroundingDeleteBytes(
				test.oldWord,
				test.deleteRunes,
				test.surrounding,
				test.cursor,
				test.anchor,
				test.snapshotSerial,
				test.currentSerial,
				test.valid,
				test.dirty,
			); ok {
				t.Fatalf("unsafe deletion accepted with %d bytes", got)
			}
		})
	}
}

func TestSelectedStrategyUsesCachedInitialClass(t *testing.T) {
	a := app{
		strategies: appconfig.Config{
			Default: appconfig.StrategyVirtualBackspaceCommit,
			Apps: map[string]appconfig.Strategy{
				"chromium": appconfig.StrategySurroundingDeleteCommit,
			},
		},
		activeWindow: hyprland.ActiveWindow{
			Class:        "Chromium",
			InitialClass: "chromium",
		},
	}
	if got := a.selectedStrategy(); got != appconfig.StrategySurroundingDeleteCommit {
		t.Fatalf("selected strategy = %q", got)
	}

	a.activeWindow = hyprland.ActiveWindow{InitialClass: "kitty"}
	if got := a.selectedStrategy(); got != appconfig.StrategyVirtualBackspaceCommit {
		t.Fatalf("fallback strategy = %q", got)
	}
}

func TestCursorContext(t *testing.T) {
	cases := []struct {
		name   string
		text   string
		cursor int
		n      int
		want   string
	}{
		{"middle", "hello world", 5, 20, "hello| world"},
		{"truncated both sides", "abcdefghij", 5, 2, "…de|fg…"},
		{"unicode", "xin việt nam", len("xin việt"), 3, "…iệt| na…"},
		{"empty", "", 0, 5, "|"},
		{"bad cursor", "abc", 9, 5, "<invalid cursor>"},
		{"mid-rune cursor", "việt", 3, 5, "<invalid cursor>"},
	}
	for _, c := range cases {
		if got := cursorContext(c.text, c.cursor, c.n); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestComposingBackspaceRepeat(t *testing.T) {
	for _, stop := range []string{"release", "focus change", "repeat disabled"} {
		t.Run(stop, func(t *testing.T) {
			a := &app{tx: engine.New(), active: true, vnOn: true, consumed: map[uint32]bool{}}
			t.Cleanup(a.stopBackspaceRepeat)
			for _, r := range "ban" {
				a.tx.ProcessChar(r)
			}
			a.onRepeatInfo(inputmethod.ZwpInputMethodKeyboardGrabV2RepeatInfoEvent{Rate: 25, Delay: 300})
			if a.repeatDelay != 300*time.Millisecond || a.repeatInterval != 40*time.Millisecond {
				t.Fatal("repeat timing should follow compositor settings")
			}
			event := inputmethod.ZwpInputMethodKeyboardGrabV2KeyEvent{Key: keymap.KeyBackspace, State: keyStatePressed}
			a.onKey(event)
			if a.tx.Current() != "ba" || a.backspaceTimer == nil {
				t.Fatal("Backspace should remove one character and schedule repeat")
			}
			a.repeatBackspace()
			a.repeatBackspace()
			a.repeatBackspace()
			if !a.tx.Empty() || a.backspaceTimer == nil {
				t.Fatal("repeat should continue after the composing word is erased")
			}
			switch stop {
			case "release":
				event.State = 0
				a.onKey(event)
				if a.consumed[keymap.KeyBackspace] {
					t.Fatal("release should clear the consumed press")
				}
			case "focus change":
				a.resetPerFocusState()
			case "repeat disabled":
				a.onRepeatInfo(inputmethod.ZwpInputMethodKeyboardGrabV2RepeatInfoEvent{})
			}
			if a.backspaceTimer != nil {
				t.Fatal("repeat should stop")
			}
		})
	}
}
