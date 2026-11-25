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
│         └── commit_string (via input-method object)              │
└─────────────────────────────────────────────────────────────────┘
```

gokey is a single Go process (`main.go`) built around one `app` struct. It
opens one Wayland connection, binds three globals (`wl_seat`,
`zwp_input_method_manager_v2`, `zwp_virtual_keyboard_manager_v1`), creates
one input method + grab and one virtual keyboard, and then runs an event
loop (`a.ctx.Dispatch()`) forever.

On Hyprland, input-method activation also triggers one bounded query to
`.socket.sock` for `j/activewindow`. The returned `initialClass` and window
metadata are cached until deactivation. `~/.config/gokey/app-strategies.json`
is loaded once at startup; neither the file nor Hyprland is queried per key.

Inside the process, key handling is split into two layers:

- **`engine/telex.go`** — pure Telex logic. Wraps a single
  `bamboo.IEngine` instance and knows nothing about Wayland. Given a stream
  of runes, it tracks what the composing word should currently look like
  (`displayed`) and returns `(deleteRunes, insert)` diffs so the caller can
  patch already-committed text with the minimum number of edits.
- **`main.go`** — Wayland glue. Decides, per key event, whether the key is
  Vietnamese-relevant (compose it via `engine`), a control chord (toggle
  Vietnamese), or something to forward untouched (shortcuts, navigation,
  punctuation once a word has ended, or anything typed while Vietnamese is
  off or the client isn't focused for input).
- **`internal/appconfig`** — validates the two supported strategy names and
  resolves a cached app identity to a strategy with a fail-open default.
- **`internal/hyprland`** — performs the active-window Unix-socket query with
  connect/read/write deadlines and no shell process.

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
- Requests used: `CommitString(text)` followed by `Commit(serial)` to apply
  transformed text. `DeleteSurroundingText(beforeBytes, afterBytes)` is used
  by the configured live-composition strategy;
  deletion and insertion are double-buffered and applied by the same commit.
- Event used for the surrounding-delete strategy: `surrounding_text` (text around the cursor with
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

### Composing a Vietnamese word

1. Physical key press arrives as a `key` event on the input-method grab.
2. `onKey` checks it's a letter (`keymap.Letter`), applies shift/caps-lock to
   get upper/lowercase, and calls `a.tx.ProcessChar(r)`.
3. `engine.Telex.ProcessChar` feeds the rune into bamboo-core, gets back the
   new full word, and diffs it against what's already displayed, returning
   `(deleteRunes, insert)`.
4. `a.apply(oldWord, deleteRunes, insert)` resolves a strategy from the cached
   active-window identity.
5. `virtual-backspace-commit` sends one synthetic Backspace per rune, then
   commits the replacement suffix. `surrounding-delete-commit` converts the
   old rune suffix to its exact UTF-8 byte length, validates it against the
   current surrounding-text snapshot, queues deletion plus insertion, and
   calls `Commit(serial)` once. A failed validation immediately uses the
   virtual-Backspace path for that key.

### Holding Backspace

While a word is composing, GoKey consumes the physical Backspace and applies
the Telex deletion. It uses the compositor's repeat delay and rate to keep
deleting while the key is held, including after the word is erased. Releasing
the key, pressing another key, changing focus, or disabling compositor repeat
stops the timer. When a press starts outside composition, the focused client
handles the forwarded Backspace and its repeat.

### Word ends (Space / punctuation / Enter)

1. `endWord()` resets the engine because the transformed text was committed
   as each letter arrived.
2. The triggering key itself (Space/punctuation/Enter) is forwarded normally
   — it was never Vietnamese-relevant.

### Toggling

- **Ctrl+Shift (Vietnamese on/off):** `updateModifier` tracks whether
  Ctrl+Shift were both held during a modifier chord (`sawCtrlShift`) and
  whether any other key was pressed during it (`chordOther`, set by every
  non-modifier key press). When the chord fully releases
  (`wasAny && !nowAny`), if `sawCtrlShift && !chordOther`, `vnOn` flips, the
  current word ends.

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

## Injection strategy and surrounding-text safety

`virtual-backspace-commit` remains the default because it works in terminals,
which usually do not report surrounding text. Chromium is mapped to
`surrounding-delete-commit` to avoid ordering a virtual Backspace separately
from the replacement text.

The atomic path is allowed only when all of these hold:

- the client supplied a surrounding-text event in the latest state batch;
- GoKey has not forwarded or committed text since that snapshot;
- the snapshot serial equals the current count of `done` events;
- cursor and anchor are equal and the cursor byte offset is in bounds;
- the exact UTF-8 bytes that the Telex diff wants to remove are still directly
  before the cursor.

The Telex engine continues to count deletion in runes because one virtual
Backspace removes one character. Only the atomic strategy converts that rune
suffix to bytes, as required by the Wayland request. After queueing deletion
and insertion, it issues exactly one `Commit(serial)` and invalidates the
snapshot. A missing app identity, invalid config, Hyprland IPC error, stale
state, or mismatch selects/falls back to the default path.

## Process lifecycle

- `main()` connects to the compositor, binds globals via the registry's
  global handler, then blocks on a Wayland roundtrip (`a.roundtrip()`) so all
  three globals (`seat`, `imMgr`, `vkMgr`) are guaranteed bound before setup
  continues. Missing any of them is a fatal error — the compositor doesn't
  support input-method-v2 or virtual-keyboard-v1.
- Before connecting, `main()` loads the app-strategy JSON once. Missing or
  invalid files keep the built-in `virtual-backspace-commit` default.
- `activate` resets focus-local state and caches Hyprland's active window with
  a 100 ms deadline. `deactivate` clears it. Failure only leaves the identity
  empty, which resolves to the default strategy.
- `setup()` creates the input method, the virtual keyboard, registers all
  event handlers, and grabs the keyboard. If another process already holds
  the grab, the compositor's `unavailable` event fires and gokey logs a
  message naming the likely cause (another input method, e.g. fcitx5) and
  exits via `log.Fatal`.
- The main loop is a bare `for { a.ctx.Dispatch() }` — gokey is entirely
  event-driven; there is no polling, timer, or background goroutine.
