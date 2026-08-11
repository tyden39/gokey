# gokey — System Architecture

## Components

```
┌─────────────────────────────────────────────────────────────────┐
│ Wayland compositor (Hyprland / Sway / river)                    │
│                                                                   │
│   physical keyboard                                              │
│         │                                                        │
│         ▼                                                        │
│   zwp_input_method_v2  ───────grab_keyboard───────►  gokey        │
│   (compositor routes ALL key events to the grabbing IM           │
│    while it holds the seat's input-method slot)                  │
│                                                                   │
│   zwp_virtual_keyboard_v1  ◄──synthetic key events────  gokey     │
│   (compositor re-delivers these to the focused client            │
│    exactly like real keyboard input)                             │
│                                                                   │
│   focused text-input-v3 client (terminal, GTK/Qt app, browser)   │
│         ▲                                                        │
│         └── commit_string / set_preedit_string (via im object)   │
└─────────────────────────────────────────────────────────────────┘
```

gokey is a single Go process (`main.go`) built around one `app` struct. It
opens one Wayland connection, binds three globals (`wl_seat`,
`zwp_input_method_manager_v2`, `zwp_virtual_keyboard_manager_v1`), creates
one input method + grab and one virtual keyboard, and then runs an event
loop (`a.ctx.Dispatch()`) forever.

Inside the process, key handling is split into two layers:

- **`engine/telex.go`** — pure Telex logic. Wraps a single
  `bamboo.IEngine` instance and knows nothing about Wayland. Given a stream
  of runes, it tracks what the composing word should currently look like
  (`displayed`) and returns `(deleteRunes, insert)` diffs so the caller can
  patch already-committed text with the minimum number of edits.
- **`main.go`** — Wayland glue. Decides, per key event, whether the key is
  Vietnamese-relevant (compose it via `engine`), a control chord (toggle
  Vietnamese or preedit mode), or something to forward untouched (shortcuts,
  navigation, punctuation once a word has ended, or anything typed while
  Vietnamese is off or the client isn't focused for input).

## Protocols used

### `zwp_input_method_v2`

- gokey calls `GetInputMethod(seat)` once, then `GrabKeyboard()` to receive
  every keyboard event that would otherwise go straight to the compositor's
  normal input routing. While the grab is held, no other input method can
  hold the same grab — the compositor sends the `unavailable` event to any
  second grab attempt instead.
- Events consumed: `keymap` (compositor's keyboard layout, forwarded to the
  virtual keyboard so it stays in sync), `key` (every physical key
  press/release), `modifiers` (forwarded to the virtual keyboard), `activate`
  / `deactivate` (focus gained/lost — resets per-focus state), `done`
  (increments the serial number used on `Commit()` calls), `unavailable`
  (another input method already holds the grab — gokey logs and exits).
- Requests used: `CommitString(text)` and `SetPreeditString(text,
  cursorBegin, cursorEnd)` followed by `Commit(serial)` to atomically apply
  them. `DeleteSurroundingText` is used on exactly one path — cursor-word
  adoption (`adoptWord`), where it removes the adopted word atomically with
  the replacement in the same `Commit`. Live typing never calls it — see
  "Why not surrounding-text delete" below.
- Events used for adoption: `surrounding_text` (text around the cursor with
  byte offsets for cursor/anchor, double-buffered) is snapshotted and
  promoted on the next `done`; a `done` without a fresh surrounding event
  invalidates the snapshot, and `surrDirty` marks it stale whenever gokey
  itself forwards a key press, fakes a Backspace, or commits text.

### `zwp_virtual_keyboard_v1`

- gokey creates exactly one virtual keyboard tied to the same seat.
- `Keymap(format, fd, size)` is called once, as soon as the input method's
  own `keymap` event arrives, so the virtual keyboard's layout matches what
  the compositor expects. Nothing can be forwarded or faked until this
  succeeds (`keymapSet` guards `forward()` and `sendBackspace()`).
- `Modifiers(...)` mirrors every modifier state change from the input method
  grab so the focused client sees consistent modifier state even though the
  physical keyboard is grabbed.
- `Key(time, key, state)` is used two ways: **forwarding** — replaying a
  physical key event byte-for-byte when it doesn't need Vietnamese
  transformation — and **faking Backspace** — sending a synthetic
  press+release pair (`sendBackspace()`) to delete one character of
  already-committed text, using an internal monotonically increasing
  `vkTime` counter since there's no real hardware timestamp for a synthetic
  event.

## Event flow

### Direct mode (default), composing a Vietnamese word

1. Physical key press arrives as a `key` event on the input-method grab.
2. `onKey` checks it's a letter (`keymap.Letter`), applies shift/caps-lock to
   get upper/lowercase, and calls `a.tx.ProcessChar(r)`.
3. `engine.Telex.ProcessChar` feeds the rune into bamboo-core, gets back the
   new full word, and diffs it against what's already displayed, returning
   `(deleteRunes, insert)`.
4. `a.apply(deleteRunes, insert)` sends `deleteRunes` synthetic Backspace
   key events through the virtual keyboard (one rune per Backspace), then —
   if `insert` is non-empty — calls `im.CommitString(insert)` followed by
   `im.Commit(serial)`.
5. The focused client sees: some Backspaces (deleting the stale suffix of
   the word), then new committed text. Because there is only one inject
   path, there's no race between two different mechanisms — every edit to
   already-typed text goes through the same fake-Backspace-then-commit
   sequence, whether the client is a terminal or a GUI text field.

### Word ends (Space / punctuation / Enter)

1. `onKey` computes `arm := code == KeySpace && !preedit && !tx.Empty()` and
   snapshots `tx.RawKeys()` and `tx.Current()` *before* ending the word.
2. `endWord()` resets the engine (direct mode) or flushes the preedit text
   (preedit mode).
3. If `arm` was true, `restoreRaw`/`restoreWord` are set to the snapshot.
4. The triggering key itself (Space/punctuation/Enter) is forwarded normally
   — it was never Vietnamese-relevant.

### Backspace-restore (direct mode only)

This is a same-week feature: ending a word with a single Space stashes it so
the very next Backspace can un-end it, letting tone keys keep editing a word
that already looks "finished" on screen — e.g. `vi` + Space + Backspace +
`f` → `vì`.

1. Every key press first disarms the stash unconditionally
   (`restoreRaw, restoreWord := a.restoreRaw, a.restoreWord; a.restoreRaw, a.restoreWord = "", ""`)
   — restore is one-shot, so only the key press immediately following the
   arming Space can use the snapshot taken from local variables.
2. If the key is Backspace and the engine currently has nothing composing
   (`tx.Backspace()` returned `handled == false`) and a stash exists,
   `tx.RestoreWord(restoreRaw, restoreWord)` is tried.
3. `RestoreWord` resets the engine, replays `restoreRaw` through bamboo in
   Vietnamese mode, and compares the result to `restoreWord`. If they match,
   the word is composing again and the method returns `true`; if not (see
   the tone-cancel edge case in `docs/codebase-summary.md`), it returns
   `false` and the Backspace falls through to being forwarded like normal.
4. On success, gokey calls `sendBackspace()` once (a synthetic press+release,
   not a forward of the physical key) to delete the space character in the
   app, and marks the physical Backspace's keycode in `consumed` so its
   matching key-release event is swallowed instead of forwarded. Consuming
   the physical press (rather than forwarding it) matters because letting
   the client auto-repeat a forwarded physical Backspace could delete
   further into the just-restored word before gokey regains control.
5. Subsequent keys (e.g. `f` for a tone) go through the normal `ProcessChar`
   → `apply` path, editing the now-restored word.

### Preedit mode (opt-in, Ctrl+Shift+Space)

1. Each letter still goes through `tx.ProcessChar`, but instead of `apply`,
   `setPreedit(tx.Current())` is called: `im.SetPreeditString(word,
   len(word), len(word))` followed by `Commit(serial)`. The compositor
   replaces the entire preedit string atomically each time, so there's no
   fake-Backspace sequence and no ordering race — but preedit rendering is a
   client responsibility, and terminal emulators commonly don't render
   `set_preedit_string` at all, which is why direct mode is the default.
2. Word end calls `flushPreedit()`: clears the preedit
   (`SetPreeditString("", 0, 0)`), commits the finished word as real text via
   `CommitString`, then resets the engine.
3. Backspace-restore is not armed in preedit mode (`arm` requires
   `!a.preedit`) since the word was never committed as discrete diffable
   text.

### Toggling

- **Ctrl+Shift (Vietnamese on/off):** `updateModifier` tracks whether
  Ctrl+Shift were both held during a modifier chord (`sawCtrlShift`) and
  whether any other key was pressed during it (`chordOther`, set by every
  non-modifier key press). When the chord fully releases
  (`wasAny && !nowAny`), if `sawCtrlShift && !chordOther`, `vnOn` flips, the
  current word ends, and the restore stash is cleared.
- **Ctrl+Shift+Space (preedit mode on/off):** checked directly in `onKey`
  before the general Ctrl/Alt/Super passthrough branch — it flushes the
  current word via `endWord()`, then flips `preedit`, and consumes the Space
  key press so it doesn't also get typed as a literal space.

### Passthrough (inactive client, Vietnamese off, or modifier chords)

If `!a.active` (no text-input focus — e.g. a drun launcher like wofi/fuzzel,
or a layer-shell overlay), or `!a.vnOn`, or a Ctrl/Alt/Super modifier is held,
`onKey` calls `a.forward(e)` to replay the key through the virtual keyboard
unchanged, ending any in-progress word first if a modifier chord was
involved. This is what keeps launchers usable: they never get `text-input`
focus from gokey's point of view, so every key — including letters that
would otherwise be Vietnamese-composed — passes straight through. `forward`
(and `sendBackspace`) silently do nothing if `keymapSet` is still false,
which is the usual cause of "gokey is running but nothing types."

## Why not surrounding-text delete

`zwp_input_method_v2::delete_surrounding_text` exists specifically to let an
input method delete part of the text around the cursor without needing fake
key presses. gokey uses it only for cursor-word adoption (`adoptWord`);
for live typing, `apply()` in `main.go` has exactly one inject path — fake
Backspace key events through the virtual keyboard, followed by
`commit_string`. This is documented in the
`apply()` doc-comment in `main.go` as intentional: fake Backspace works in
every application, including terminal emulators, which generally do not
implement the `zwp_input_method_v2::surrounding_text` event needed to make
`delete_surrounding_text` usable at all. Using surrounding-text deletion for
GUI clients and fake Backspace for terminals would mean two different inject
paths to keep correct, and is called out in `docs/project-roadmap.md` as a
candidate future improvement rather than a rejected idea — see that
document for the tradeoff it would address (a known race in some GUI
clients between a fake Backspace and their own text-input state).

## Process lifecycle

- `main()` connects to the compositor, binds globals via the registry's
  global handler, then blocks on a Wayland roundtrip (`a.roundtrip()`) so all
  three globals (`seat`, `imMgr`, `vkMgr`) are guaranteed bound before setup
  continues. Missing any of them is a fatal error — the compositor doesn't
  support input-method-v2 or virtual-keyboard-v1.
- `setup()` creates the input method, the virtual keyboard, registers all
  event handlers, and grabs the keyboard. If another process already holds
  the grab, the compositor's `unavailable` event fires and gokey logs a
  message naming the likely cause (another input method, e.g. fcitx5) and
  exits via `log.Fatal`.
- The main loop is a bare `for { a.ctx.Dispatch() }` — gokey is entirely
  event-driven; there is no polling, timer, or background goroutine.
