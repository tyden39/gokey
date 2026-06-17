package main

import (
	"log"
	"os"

	"github.com/rajveermalviya/go-wayland/wayland/client"
	"golang.org/x/sys/unix"

	"gokey/engine"
	"gokey/internal/inputmethod"
	"gokey/internal/keymap"
	"gokey/internal/virtualkeyboard"
)

const keyStatePressed = uint32(client.KeyboardKeyStatePressed)

var debug = os.Getenv("GOKEY_DEBUG") != ""

func dbg(format string, args ...any) {
	if debug {
		log.Printf(format, args...)
	}
}

type app struct {
	ctx      *client.Context
	display  *client.Display
	registry *client.Registry

	seat  *client.Seat
	imMgr *inputmethod.ZwpInputMethodManagerV2
	vkMgr *virtualkeyboard.ZwpVirtualKeyboardManagerV1

	im   *inputmethod.ZwpInputMethodV2
	grab *inputmethod.ZwpInputMethodKeyboardGrabV2
	vk   *virtualkeyboard.ZwpVirtualKeyboardV1

	tx      *engine.Telex
	serial  uint32
	vnOn    bool
	active  bool
	preedit bool // false: commit live diff; true: show set_preedit_string

	keymapSet bool
	consumed  map[uint32]bool
	vkTime    uint32

	ctrl, shift, alt, super bool
	capsLock                bool
	sawCtrlShift            bool
	chordOther              bool
}

func main() {
	display, err := client.Connect("")
	if err != nil {
		log.Fatalf("connect wayland: %v", err)
	}
	a := &app{
		ctx:      display.Context(),
		display:  display,
		tx:       engine.New(),
		vnOn:     true,
		consumed: map[uint32]bool{},
	}

	display.SetErrorHandler(func(e client.DisplayErrorEvent) {
		log.Fatalf("wayland protocol error: object=%v code=%d message=%q", e.ObjectId, e.Code, e.Message)
	})

	a.registry, err = display.GetRegistry()
	if err != nil {
		log.Fatalf("get registry: %v", err)
	}
	a.registry.SetGlobalHandler(a.onGlobal)
	a.roundtrip()

	if a.seat == nil || a.imMgr == nil || a.vkMgr == nil {
		log.Fatal("compositor missing input-method-v2 or virtual-keyboard-v1 support")
	}
	a.setup()
	log.Printf("gokey running (Telex, Vietnamese=%v, toggle=Ctrl+Shift, preedit toggle=Ctrl+Shift+Space)", a.vnOn)

	for {
		if err := a.ctx.Dispatch(); err != nil {
			log.Fatalf("dispatch: %v", err)
		}
	}
}

func (a *app) roundtrip() {
	cb, err := a.display.Sync()
	if err != nil {
		log.Fatalf("sync: %v", err)
	}
	done := false
	cb.SetDoneHandler(func(client.CallbackDoneEvent) { done = true })
	for !done {
		if err := a.ctx.Dispatch(); err != nil {
			log.Fatalf("dispatch: %v", err)
		}
	}
}

func (a *app) onGlobal(e client.RegistryGlobalEvent) {
	switch e.Interface {
	case "wl_seat":
		seat := client.NewSeat(a.ctx)
		if err := a.registry.Bind(e.Name, e.Interface, e.Version, seat); err == nil {
			a.seat = seat
		}
	case "zwp_input_method_manager_v2":
		mgr := inputmethod.NewZwpInputMethodManagerV2(a.ctx)
		if err := a.registry.Bind(e.Name, e.Interface, e.Version, mgr); err == nil {
			a.imMgr = mgr
		}
	case "zwp_virtual_keyboard_manager_v1":
		mgr := virtualkeyboard.NewZwpVirtualKeyboardManagerV1(a.ctx)
		if err := a.registry.Bind(e.Name, e.Interface, e.Version, mgr); err == nil {
			a.vkMgr = mgr
		}
	}
}

func (a *app) setup() {
	im, err := a.imMgr.GetInputMethod(a.seat)
	if err != nil {
		log.Fatalf("get input method: %v", err)
	}
	a.im = im

	vk, err := a.vkMgr.CreateVirtualKeyboard(a.seat)
	if err != nil {
		log.Fatalf("create virtual keyboard: %v", err)
	}
	a.vk = vk

	im.SetActivateHandler(func(inputmethod.ZwpInputMethodV2ActivateEvent) {
		dbg("activate")
		a.active = true
		a.tx.Reset()
	})
	im.SetDeactivateHandler(func(inputmethod.ZwpInputMethodV2DeactivateEvent) {
		dbg("deactivate")
		a.active = false
		a.tx.Reset()
	})
	im.SetDoneHandler(func(inputmethod.ZwpInputMethodV2DoneEvent) { a.serial++ })
	im.SetUnavailableHandler(func(inputmethod.ZwpInputMethodV2UnavailableEvent) {
		log.Fatal("input method unavailable: another input method (e.g. fcitx5) is already active; stop it first")
	})

	grab, err := im.GrabKeyboard()
	if err != nil {
		log.Fatalf("grab keyboard: %v", err)
	}
	a.grab = grab
	grab.SetKeymapHandler(a.onKeymap)
	grab.SetModifiersHandler(a.onModifiers)
	grab.SetKeyHandler(a.onKey)
}

func (a *app) onKeymap(e inputmethod.ZwpInputMethodKeyboardGrabV2KeymapEvent) {
	if a.vk != nil {
		if err := a.vk.Keymap(e.Format, e.Fd, e.Size); err == nil {
			a.keymapSet = true
			dbg("keymap set (format=%d size=%d)", e.Format, e.Size)
		}
	}
	if e.Fd != -1 {
		unix.Close(e.Fd)
	}
}

func (a *app) onModifiers(e inputmethod.ZwpInputMethodKeyboardGrabV2ModifiersEvent) {
	if a.vk != nil && a.keymapSet {
		a.vk.Modifiers(e.ModsDepressed, e.ModsLatched, e.ModsLocked, e.Group)
	}
}

func (a *app) onKey(e inputmethod.ZwpInputMethodKeyboardGrabV2KeyEvent) {
	code := e.Key
	pressed := e.State == keyStatePressed

	if keymap.IsModifier(code) {
		a.updateModifier(code, pressed)
		a.forward(e)
		return
	}

	if !pressed {
		if a.consumed[code] {
			delete(a.consumed, code)
			return
		}
		a.forward(e)
		return
	}

	a.chordOther = true

	// Ctrl+Shift+Space toggles between direct-commit and preedit modes.
	if code == keymap.KeySpace && a.ctrl && a.shift && !a.alt && !a.super {
		a.togglePreedit()
		a.consumed[code] = true
		return
	}

	if !a.active || !a.vnOn || a.ctrl || a.alt || a.super {
		if a.ctrl || a.alt || a.super {
			a.endWord()
		}
		a.forward(e)
		return
	}

	if code == keymap.KeyBackspace {
		dbg("backspace received (word empty=%v)", a.tx.Empty())
		if del, ins, handled := a.tx.Backspace(); handled {
			if a.preedit {
				a.setPreedit(a.tx.Current())
			} else {
				a.apply(del, ins)
			}
			a.consumed[code] = true
			return
		}
		a.forward(e)
		return
	}

	if r, ok := keymap.Letter(code); ok {
		if a.shift != a.capsLock {
			r = toUpper(r)
		}
		del, ins := a.tx.ProcessChar(r)
		dbg("letter %q -> delete %d, insert %q", r, del, ins)
		if a.preedit {
			a.setPreedit(a.tx.Current())
		} else {
			a.apply(del, ins)
		}
		a.consumed[code] = true
		return
	}

	// A non-letter key (space, punctuation, Enter) ends the word.
	a.endWord()
	a.forward(e)
}

// apply deletes deleteRunes trailing characters by sending real Backspace key
// events through the virtual keyboard (works in every app, including
// terminals, unlike delete_surrounding_text), then inserts text via the input
// method's commit_string.
func (a *app) apply(deleteRunes int, insert string) {
	for i := 0; i < deleteRunes; i++ {
		a.sendBackspace()
	}
	if insert != "" {
		a.im.CommitString(insert)
		a.im.Commit(a.serial)
	}
}

// setPreedit shows the in-progress word as underlined preedit text instead of
// committing it. The compositor replaces the whole preedit atomically each
// time, so there are no fake Backspaces and no ordering race. Used only in
// preedit mode. The cursor sits at the end of the word.
func (a *app) setPreedit(word string) {
	a.im.SetPreeditString(word, int32(len(word)), int32(len(word)))
	a.im.Commit(a.serial)
}

// flushPreedit commits the current composing word as real text and clears the
// preedit. Called when a word ends or when leaving preedit mode. No-op against
// the application when not focused; the engine is reset either way.
func (a *app) flushPreedit() {
	if a.active {
		word := a.tx.Current()
		a.im.SetPreeditString("", 0, 0)
		if word != "" {
			a.im.CommitString(word)
		}
		a.im.Commit(a.serial)
	}
	a.tx.Reset()
}

// endWord finishes the current word. In direct mode the text is already
// committed live, so this only resets the engine; in preedit mode it commits
// the pending preedit first.
func (a *app) endWord() {
	if a.preedit {
		a.flushPreedit()
	} else {
		a.tx.Reset()
	}
}

// togglePreedit switches between direct-commit and preedit modes, flushing any
// in-progress word so nothing is left half-composed across the switch.
func (a *app) togglePreedit() {
	a.endWord()
	a.preedit = !a.preedit
	log.Printf("preedit=%v", a.preedit)
}

func (a *app) sendBackspace() {
	if a.vk == nil || !a.keymapSet {
		return
	}
	a.vkTime++
	a.vk.Key(a.vkTime, keymap.KeyBackspace, keyStatePressed)
	a.vkTime++
	a.vk.Key(a.vkTime, keymap.KeyBackspace, 0)
}

func (a *app) forward(e inputmethod.ZwpInputMethodKeyboardGrabV2KeyEvent) {
	if a.vk == nil || !a.keymapSet {
		return
	}
	a.vk.Key(e.Time, e.Key, e.State)
}

func (a *app) updateModifier(code uint32, pressed bool) {
	wasAny := a.ctrl || a.shift || a.alt || a.super
	switch code {
	case keymap.KeyLeftCtrl, keymap.KeyRightCtrl:
		a.ctrl = pressed
	case keymap.KeyLeftShift, keymap.KeyRightShift:
		a.shift = pressed
	case keymap.KeyLeftAlt, keymap.KeyRightAlt:
		a.alt = pressed
	case keymap.KeyLeftMeta, keymap.KeyRightMeta:
		a.super = pressed
	case keymap.KeyCapsLock:
		if pressed {
			a.capsLock = !a.capsLock
		}
		return
	}

	nowAny := a.ctrl || a.shift || a.alt || a.super
	if !wasAny && nowAny {
		a.chordOther = false
		a.sawCtrlShift = false
	}
	if a.ctrl && a.shift {
		a.sawCtrlShift = true
	}
	if wasAny && !nowAny {
		if a.sawCtrlShift && !a.chordOther {
			a.vnOn = !a.vnOn
			a.endWord()
			log.Printf("Vietnamese=%v", a.vnOn)
		}
		a.sawCtrlShift = false
		a.chordOther = false
	}
}

func toUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 'a' + 'A'
	}
	return r
}
