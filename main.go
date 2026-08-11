package main

import (
	"log"
	"os"
	"strings"
	"unicode/utf8"

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

	// restoreRaw/restoreWord hold the raw keystrokes and on-screen form of
	// the last word when it was ended by a single Space and nothing was typed
	// since. Non-empty restoreRaw means the next Backspace deletes that space
	// and resumes composing the word (so tone keys still edit it). Direct
	// mode only.
	restoreRaw  string
	restoreWord string

	// Surrounding-text snapshot from the focused client. The protocol
	// double-buffers it: the event stores into surrPend*, and the next done
	// event promotes it into surr*/surrValid. Clients that support it resend
	// it with every state batch, so a done without a fresh event invalidates
	// the snapshot rather than keeping a stale one. Terminals never send it,
	// which is the feature gate for cursor-word adoption.
	surrPendText   string
	surrPendCursor int
	surrPendAnchor int
	surrPending    bool
	surrText       string
	surrCursor     int
	surrAnchor     int
	surrValid      bool
	// surrDirty marks the snapshot stale: set whenever gokey itself changes
	// the client's text or cursor (forwarded key press, fake Backspace,
	// commit), cleared only when a fresh surrounding_text arrives with the
	// next done. Blocks adoption from deleting at an offset the client has
	// already moved away from.
	surrDirty bool

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
		a.resetPerFocusState()
	})
	im.SetDeactivateHandler(func(inputmethod.ZwpInputMethodV2DeactivateEvent) {
		dbg("deactivate")
		a.active = false
		a.resetPerFocusState()
	})
	im.SetSurroundingTextHandler(func(e inputmethod.ZwpInputMethodV2SurroundingTextEvent) {
		a.surrPendText = strings.TrimRight(e.Text, "\x00")
		a.surrPendCursor = int(e.Cursor)
		a.surrPendAnchor = int(e.Anchor)
		a.surrPending = true
	})
	im.SetDoneHandler(func(inputmethod.ZwpInputMethodV2DoneEvent) {
		a.serial++
		if a.surrPending {
			a.surrText, a.surrCursor, a.surrAnchor = a.surrPendText, a.surrPendCursor, a.surrPendAnchor
			a.surrValid = true
			a.surrDirty = false
			a.surrPending = false
			a.surrPendText, a.surrPendCursor, a.surrPendAnchor = "", 0, 0
		} else {
			// State batches resend surrounding text when it is supported; a
			// batch without it means the previous snapshot no longer holds.
			a.surrValid = false
		}
	})
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

	// Restore is one-shot: only the very next key press after the arming
	// Space may use it. Any other press — letters, punctuation, chords,
	// a second Space — invalidates the stashed word.
	restoreRaw, restoreWord := a.restoreRaw, a.restoreWord
	a.restoreRaw, a.restoreWord = "", ""

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
		if restoreRaw != "" && a.keymapSet && a.tx.RestoreWord(restoreRaw, restoreWord) {
			// Delete the space with a synthetic press+release and consume the
			// physical key: forwarding a bare press would let the client
			// auto-repeat and eat into the word the engine just restored.
			dbg("backspace restores word %q", restoreWord)
			a.sendBackspace()
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
		if a.tx.Empty() && isAdoptKey(r) && a.adoptWord(r) {
			a.consumed[code] = true
			return
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

	// A non-letter key (space, punctuation, Enter) ends the word. Space (in
	// direct mode, with a word in progress) also arms Backspace-restore:
	// stash the word's raw keys and on-screen form before the reset so one
	// Backspace can bring it back for further tone edits.
	arm := code == keymap.KeySpace && !a.preedit && !a.tx.Empty()
	raw, word := a.tx.RawKeys(), a.tx.Current()
	a.endWord()
	if arm {
		a.restoreRaw, a.restoreWord = raw, word
	}
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
		a.surrDirty = true
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
			a.surrDirty = true
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

// resetPerFocusState drops all state tied to the previously focused client:
// the composing word, the Backspace-restore stash, the surrounding-text
// snapshot, and pending key-consume bookkeeping (a release may never arrive
// after a focus change).
func (a *app) resetPerFocusState() {
	a.tx.Reset()
	a.restoreRaw, a.restoreWord = "", ""
	a.surrValid, a.surrPending, a.surrDirty = false, false, false
	a.surrText, a.surrPendText = "", ""
	a.consumed = map[uint32]bool{}
}

// isAdoptKey reports whether r is a Telex tone/mark key that may edit the
// committed word next to the cursor: tones s f r x j, horn/breve w, and
// tone-removal z. Plain letters deliberately start a new word instead.
func isAdoptKey(r rune) bool {
	switch r {
	case 's', 'f', 'r', 'x', 'j', 'w', 'z',
		'S', 'F', 'R', 'X', 'J', 'W', 'Z':
		return true
	}
	return false
}

// maxAdoptRunes caps the total length of an adoptable word. A longer
// unbroken letter run (identifiers in editors, truncated surrounding text)
// is refused outright — adopting a *partial* word would delete and rewrite
// the middle of something that is not a word.
const maxAdoptRunes = 24

// wordAtCursor extracts the committed word the cursor touches from the
// surrounding-text snapshot: the rune immediately before the cursor must be a
// word rune (cursor inside or at the end of a word), and the word extends over
// letters on both sides. Returns the word plus its byte extents before and
// after the cursor. Refused when there is a selection — typing would replace
// it, not edit a word — and when the snapshot may be stale (surrDirty).
func (a *app) wordAtCursor() (word string, beforeBytes, afterBytes int, ok bool) {
	if !a.surrValid || a.surrDirty || a.surrCursor != a.surrAnchor {
		return "", 0, 0, false
	}
	text, cur := a.surrText, a.surrCursor
	if cur <= 0 || cur > len(text) {
		return "", 0, 0, false
	}
	runes := 0
	left := cur
	for left > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:left])
		if r == utf8.RuneError || !engine.IsWordRune(r) {
			break
		}
		if runes++; runes > maxAdoptRunes {
			return "", 0, 0, false
		}
		left -= size
	}
	if left == cur {
		return "", 0, 0, false
	}
	right := cur
	for right < len(text) {
		r, size := utf8.DecodeRuneInString(text[right:])
		if r == utf8.RuneError || !engine.IsWordRune(r) {
			break
		}
		if runes++; runes > maxAdoptRunes {
			return "", 0, 0, false
		}
		right += size
	}
	return text[left:right], cur - left, right - cur, true
}

// adoptWord pulls the committed word around the cursor back into the engine
// and applies the pressed tone/mark key to it. The edit only happens when the
// key actually transforms the word AND the result still spells like a
// Vietnamese syllable — so "viet"+j edits to "viẹt" but "hello"+s refuses
// (bamboo would mangle it to "helló") and falls back to plain typing at the
// cursor. This first edit replaces the on-screen word atomically via
// delete_surrounding_text + commit_string (direct) or reopens it as preedit;
// no fake Backspace, so no ordering race — later keys of the same word go
// through the normal per-mode paths. The snapshot is consumed either way:
// one adoption attempt per surrounding_text event.
func (a *app) adoptWord(r rune) bool {
	word, before, after, ok := a.wordAtCursor()
	if !ok {
		return false
	}
	a.surrValid = false
	newWord, ok := a.tx.AdoptEdit(word, r)
	if !ok {
		return false
	}
	dbg("adopt %q + %q -> %q (before=%d after=%d)", word, r, newWord, before, after)
	a.surrDirty = true
	a.im.DeleteSurroundingText(uint32(before), uint32(after))
	if a.preedit {
		a.setPreedit(newWord)
	} else {
		a.im.CommitString(newWord)
		a.im.Commit(a.serial)
	}
	return true
}

func (a *app) sendBackspace() {
	if a.vk == nil || !a.keymapSet {
		return
	}
	a.surrDirty = true
	a.vkTime++
	a.vk.Key(a.vkTime, keymap.KeyBackspace, keyStatePressed)
	a.vkTime++
	a.vk.Key(a.vkTime, keymap.KeyBackspace, 0)
}

func (a *app) forward(e inputmethod.ZwpInputMethodKeyboardGrabV2KeyEvent) {
	if a.vk == nil || !a.keymapSet {
		return
	}
	// A forwarded key press may change the client's text or cursor, making
	// the surrounding snapshot stale. Modifier presses and releases don't.
	if e.State == keyStatePressed && !keymap.IsModifier(e.Key) {
		a.surrDirty = true
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
			a.restoreRaw, a.restoreWord = "", ""
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
