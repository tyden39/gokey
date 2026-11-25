package main

import (
	"log"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rajveermalviya/go-wayland/wayland/client"
	"golang.org/x/sys/unix"

	"gokey/engine"
	"gokey/internal/appconfig"
	"gokey/internal/hyprland"
	"gokey/internal/inputmethod"
	"gokey/internal/keymap"
	"gokey/internal/virtualkeyboard"
)

const keyStatePressed = uint32(client.KeyboardKeyStatePressed)

const activeWindowQueryTimeout = 100 * time.Millisecond

var debug = os.Getenv("GOKEY_DEBUG") != ""

func dbg(format string, args ...any) {
	if debug {
		log.Printf(format, args...)
	}
}

// cursorContextRunes is how many runes of surrounding text are logged on each
// side of the cursor.
const cursorContextRunes = 20

// cursorContext returns up to n runes before and after the byte offset cursor,
// with "|" marking the cursor, for debug logging.
func cursorContext(text string, cursor, n int) string {
	if cursor < 0 || cursor > len(text) || !utf8.ValidString(text[:cursor]) {
		return "<invalid cursor>"
	}
	before, after := []rune(text[:cursor]), []rune(text[cursor:])
	prefix, suffix := "", ""
	if len(before) > n {
		before, prefix = before[len(before)-n:], "…"
	}
	if len(after) > n {
		after, suffix = after[:n], "…"
	}
	return prefix + string(before) + "|" + string(after) + suffix
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

	tx     *engine.Telex
	serial uint32
	vnOn   bool
	active bool

	strategies   appconfig.Config
	activeWindow hyprland.ActiveWindow

	// Surrounding-text snapshot from the focused client. The protocol
	// double-buffers it: the event stores into surrPend*, and the next done
	// event promotes it into surr*/surrValid. Clients that support it resend
	// it with every state batch, so a done without a fresh event invalidates
	// the snapshot rather than keeping a stale one. Terminals never send it.
	surrPendText   string
	surrPendCursor int
	surrPendAnchor int
	surrPending    bool
	surrText       string
	surrCursor     int
	surrAnchor     int
	surrValid      bool
	surrSerial     uint32
	// surrDirty marks the snapshot stale: set whenever gokey itself changes
	// the client's text or cursor (forwarded key press, fake Backspace,
	// commit), cleared only when a fresh surrounding_text arrives with the
	// next done. Blocks surrounding deletes at an offset the client has
	// already moved away from.
	surrDirty bool

	backspaceTimer *time.Timer
	repeatInterval time.Duration
	repeatDelay    time.Duration

	keymapSet bool
	consumed  map[uint32]bool
	vkTime    uint32

	ctrl, shift, alt, super bool
	capsLock                bool
	sawCtrlShift            bool
	chordOther              bool
}

func main() {
	strategies, strategyPath := loadStrategyConfig()
	display, err := client.Connect("")
	if err != nil {
		log.Fatalf("connect wayland: %v", err)
	}
	a := &app{
		ctx:        display.Context(),
		display:    display,
		tx:         engine.New(),
		vnOn:       true,
		consumed:   map[uint32]bool{},
		strategies: strategies,
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
	log.Printf("gokey running (Telex, Vietnamese=%v, toggle=Ctrl+Shift, app-strategies=%s)", a.vnOn, strategyPath)

	a.run()
}

// Read Wayland messages while waiting for the repeat timer. Dispatch handlers
// and repeat actions on the same goroutine so they share the current IME state.
func (a *app) run() {
	type message struct {
		id, opcode uint32
		fd         int
		data       []byte
		err        error
	}
	events := make(chan message)
	go func() {
		for {
			id, opcode, fd, data, err := a.ctx.ReadMsg()
			events <- message{id, opcode, fd, data, err}
			if err != nil {
				return
			}
		}
	}()

	for {
		var repeat <-chan time.Time
		if a.backspaceTimer != nil {
			repeat = a.backspaceTimer.C
		}
		select {
		case e := <-events:
			if e.err != nil {
				log.Fatalf("read Wayland event: %v", e.err)
			}
			sender, ok := a.ctx.GetProxy(e.id).(client.Dispatcher)
			if !ok {
				log.Fatalf("dispatch: no dispatcher for object %d", e.id)
			}
			sender.Dispatch(e.opcode, e.fd, e.data)
		case <-repeat:
			a.repeatBackspace()
		}
	}
}

func loadStrategyConfig() (appconfig.Config, string) {
	config := appconfig.Defaults()
	path, err := appconfig.DefaultPath()
	if err != nil {
		log.Printf("resolve app strategy config: %v; using %s", err, config.Default)
		return config, "defaults"
	}
	loaded, err := appconfig.Load(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("load app strategy config %s: %v; using %s", path, err, config.Default)
		} else {
			dbg("app strategy config %s does not exist; using %s", path, config.Default)
		}
		return config, path
	}
	return loaded, path
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
		a.refreshActiveWindow()
	})
	im.SetDeactivateHandler(func(inputmethod.ZwpInputMethodV2DeactivateEvent) {
		dbg("deactivate")
		a.active = false
		a.resetPerFocusState()
		a.activeWindow = hyprland.ActiveWindow{}
	})
	im.SetSurroundingTextHandler(func(e inputmethod.ZwpInputMethodV2SurroundingTextEvent) {
		a.surrPendText = strings.TrimRight(e.Text, "\x00")
		a.surrPendCursor = int(e.Cursor)
		a.surrPendAnchor = int(e.Anchor)
		a.surrPending = true
		dbg("surrounding cursor=%d anchor=%d around=%q", a.surrPendCursor, a.surrPendAnchor,
			cursorContext(a.surrPendText, a.surrPendCursor, cursorContextRunes))
		// The previous snapshot must not be used while a newer state batch is
		// pending its done event and serial.
		a.surrValid = false
	})
	im.SetDoneHandler(func(inputmethod.ZwpInputMethodV2DoneEvent) {
		a.serial++
		if a.surrPending {
			a.surrText, a.surrCursor, a.surrAnchor = a.surrPendText, a.surrPendCursor, a.surrPendAnchor
			a.surrValid = true
			a.surrDirty = false
			a.surrSerial = a.serial
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
	grab.SetRepeatInfoHandler(a.onRepeatInfo)
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
	if code == keymap.KeyBackspace || pressed {
		a.stopBackspaceRepeat()
	}

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

	if name, ok := keymap.CursorKeyName(code); ok {
		dbg("cursor key %s (active=%v, word empty=%v)", name, a.active, a.tx.Empty())
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
		if a.eraseComposingCharacter() {
			a.consumed[code] = true
			if a.repeatInterval > 0 {
				a.backspaceTimer = time.NewTimer(a.repeatDelay)
			}
			return
		}
		a.forward(e)
		return
	}

	if r, ok := keymap.Letter(code); ok {
		if a.shift != a.capsLock {
			r = toUpper(r)
		}
		oldWord := a.tx.Current()
		del, ins := a.tx.ProcessChar(r)
		dbg("letter %q -> delete %d, insert %q", r, del, ins)
		a.apply(oldWord, del, ins)
		a.consumed[code] = true
		return
	}

	// A non-letter key (space, punctuation, Enter) ends the word.
	a.endWord()
	a.forward(e)
}

func (a *app) eraseComposingCharacter() bool {
	oldWord := a.tx.Current()
	del, ins, handled := a.tx.Backspace()
	if handled {
		a.apply(oldWord, del, ins)
	}
	return handled
}

func (a *app) onRepeatInfo(e inputmethod.ZwpInputMethodKeyboardGrabV2RepeatInfoEvent) {
	a.repeatDelay = time.Duration(e.Delay) * time.Millisecond
	a.repeatInterval = 0
	if e.Rate > 0 {
		a.repeatInterval = max(time.Nanosecond, time.Second/time.Duration(e.Rate))
	} else {
		a.stopBackspaceRepeat()
	}
}

func (a *app) repeatBackspace() {
	if !a.eraseComposingCharacter() {
		a.sendBackspace()
	}
	a.backspaceTimer.Reset(a.repeatInterval)
}

func (a *app) stopBackspaceRepeat() {
	if a.backspaceTimer != nil {
		a.backspaceTimer.Stop()
		a.backspaceTimer = nil
	}
}

// apply chooses the injection strategy for the cached active app. Unsafe or
// stale surrounding-text state always falls back to virtual Backspace for the
// current key rather than guessing at a cursor location.
func (a *app) apply(oldWord string, deleteRunes int, insert string) {
	if a.selectedStrategy() == appconfig.StrategySurroundingDeleteCommit {
		if a.applySurroundingDeleteCommit(oldWord, deleteRunes, insert) {
			return
		}
		dbg("surrounding delete refused for app=%q; falling back to sentinel Backspace", a.activeWindow.Identity())
		a.applySentinelBackspaceCommit(deleteRunes, insert)
		return
	}
	a.applyVirtualBackspaceCommit(deleteRunes, insert)
}

// applySentinelBackspaceCommit inserts a harmless sentinel before replacing
// text. Some clients order a commit followed by virtual Backspace more
// reliably when the cursor is first advanced by a committed character.
func (a *app) applySentinelBackspaceCommit(deleteRunes int, insert string) {
	a.surrDirty = true
	if err := a.im.CommitString("."); err != nil {
		dbg("commit sentinel: %v", err)
	}
	if err := a.im.Commit(a.serial); err != nil {
		dbg("commit sentinel serial %d: %v", a.serial, err)
	}

	for i := 0; i < deleteRunes+1; i++ {
		a.sendBackspace()
	}
	if insert != "" {
		if err := a.im.CommitString(insert); err != nil {
			dbg("commit string: %v", err)
		}
		if err := a.im.Commit(a.serial); err != nil {
			dbg("commit serial %d: %v", a.serial, err)
		}
	}
}

func (a *app) applyVirtualBackspaceCommit(deleteRunes int, insert string) {
	for i := 0; i < deleteRunes; i++ {
		a.sendBackspace()
	}
	if insert != "" {
		a.surrDirty = true
		if err := a.im.CommitString(insert); err != nil {
			dbg("commit string: %v", err)
		}
		if err := a.im.Commit(a.serial); err != nil {
			dbg("commit serial %d: %v", a.serial, err)
		}
	}
}

func (a *app) applySurroundingDeleteCommit(oldWord string, deleteRunes int, insert string) bool {
	if deleteRunes == 0 {
		if insert == "" {
			return true
		}
		a.surrDirty = true
		a.surrValid = false
		if err := a.im.CommitString(insert); err != nil {
			dbg("commit string: %v", err)
		}
		if err := a.im.Commit(a.serial); err != nil {
			dbg("commit serial %d: %v", a.serial, err)
		}
		return true
	}

	deleteBytes, ok := surroundingDeleteBytes(
		oldWord,
		deleteRunes,
		a.surrText,
		a.surrCursor,
		a.surrAnchor,
		a.surrSerial,
		a.serial,
		a.surrValid,
		a.surrDirty,
	)
	if !ok {
		return false
	}

	a.surrDirty = true
	a.surrValid = false
	dbg(
		"surrounding replace app=%q deleteBytes=%d insert=%q serial=%d",
		a.activeWindow.Identity(),
		deleteBytes,
		insert,
		a.serial,
	)
	if err := a.im.DeleteSurroundingText(deleteBytes, 0); err != nil {
		dbg("delete %d surrounding bytes: %v", deleteBytes, err)
	}
	if insert != "" {
		if err := a.im.CommitString(insert); err != nil {
			dbg("commit string: %v", err)
		}
	}
	if err := a.im.Commit(a.serial); err != nil {
		dbg("commit serial %d: %v", a.serial, err)
	}
	return true
}

func (a *app) selectedStrategy() appconfig.Strategy {
	return a.strategies.StrategyFor(a.activeWindow.Identity())
}

func (a *app) refreshActiveWindow() {
	a.activeWindow = hyprland.ActiveWindow{}
	window, err := hyprland.ActiveWindowFromEnvironment(activeWindowQueryTimeout)
	if err != nil {
		dbg("detect active Hyprland window: %v", err)
		return
	}
	a.activeWindow = window
	dbg(
		"active app address=%s class=%q initialClass=%q pid=%d xwayland=%v strategy=%s",
		window.Address,
		window.Class,
		window.InitialClass,
		window.PID,
		window.XWayland,
		a.selectedStrategy(),
	)
}

func surroundingDeleteBytes(
	oldWord string,
	deleteRunes int,
	surrounding string,
	cursor int,
	anchor int,
	snapshotSerial uint32,
	currentSerial uint32,
	valid bool,
	dirty bool,
) (uint32, bool) {
	if !valid || dirty || snapshotSerial != currentSerial || cursor != anchor {
		return 0, false
	}
	if deleteRunes <= 0 || !utf8.ValidString(oldWord) || !utf8.ValidString(surrounding) {
		return 0, false
	}
	runes := []rune(oldWord)
	if deleteRunes > len(runes) {
		return 0, false
	}
	deletedSuffix := string(runes[len(runes)-deleteRunes:])
	deleteBytes := len(deletedSuffix)
	if cursor < deleteBytes || cursor > len(surrounding) {
		return 0, false
	}
	if surrounding[cursor-deleteBytes:cursor] != deletedSuffix {
		return 0, false
	}
	return uint32(deleteBytes), true
}

// endWord finishes the current word. Text is already committed live, so this
// only resets the engine.
func (a *app) endWord() {
	a.tx.Reset()
}

// resetPerFocusState drops all state tied to the previously focused client:
// the composing word, surrounding-text snapshot, and pending key-consume
// bookkeeping (a release may never arrive after a focus change).
func (a *app) resetPerFocusState() {
	a.stopBackspaceRepeat()
	a.tx.Reset()
	a.surrValid, a.surrPending, a.surrDirty = false, false, false
	a.surrSerial = 0
	a.surrText, a.surrPendText = "", ""
	a.consumed = map[uint32]bool{}
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
