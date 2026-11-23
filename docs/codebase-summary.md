# gokey — Codebase Summary

## Layout

```
gokey/
├── main.go                                   — Wayland app: connects, binds
│                                     globals, grabs keyboard, handles every
│                                     key event, selects a strategy, injects text.
├── main_test.go                               — UTF-8/snapshot safety tests.
├── engine/
│   ├── telex.go                               — wraps bamboo-core for one
│   │                                 in-progress word; pure logic, no
│   │                                 Wayland dependency.
│   └── telex_test.go                          — composition unit tests.
├── internal/
│   ├── appconfig/                             — strategy config + tests.
│   ├── hyprland/                              — active-window IPC + tests.
│   ├── inputmethod/input_method.go            — generated bindings for
│   │                                 zwp_input_method_v2 (+ manual patch).
│   ├── virtualkeyboard/
│   │   virtual_keyboard.go                    — generated bindings for
│   │                                 zwp_virtual_keyboard_v1.
│   └── keymap/keymap.go                       — evdev keycodes, US-layout
│                                     letter map, modifier detection.
├── protocols/
│   ├── input-method-unstable-v2.xml   upstream protocol spec (source for
│   └── virtual-keyboard-unstable-v1.xml the internal/ generated bindings)
├── run.sh                                     — dev build/run script.
├── go.mod / go.sum                  module deps.
├── README.md
└── docs/                            this documentation set.
```

Generated protocol bindings are not hand-maintained. App detection, config
parsing, and safety validation are small hand-written packages/functions with
focused unit tests.

## Dependencies (go.mod, Go 1.25.0)

| Module | Purpose |
|---|---|
| `github.com/BambooEngine/bamboo-core` | Telex transform engine (tones, diacritics, `đ`) |
| `github.com/rajveermalviya/go-wayland/wayland` | Wayland client protocol library (`client` package: `Context`, `Display`, `Registry`, `Seat`, key/keyboard event types) |
| `golang.org/x/sys` | `unix.Close` for closing the keymap file descriptor |

## Public API surface

### `engine` package (`engine/telex.go`)

```go
func New() *Telex

func (t *Telex) Reset()
func (t *Telex) Empty() bool
func (t *Telex) Current() string
func (t *Telex) ProcessChar(r rune) (deleteRunes int, insert string)
func (t *Telex) Backspace() (deleteRunes int, insert string, handled bool)
```

- `New()` builds a `Telex` around a fresh `bamboo.IEngine` configured for the
  `"Telex"` input method with `bamboo.EstdFlags`.
- `Reset()` ends the current word in the engine; it does not touch any text
  already committed to the focused application.
- `Empty()` reports whether `displayed` (the currently-composed word as it
  should appear on screen) is the empty string.
- `Current()` returns that same `displayed` string.
- `ProcessChar(r rune) (deleteRunes int, insert string)` feeds one letter
  into bamboo and returns the (delete, insert) diff needed to turn the
  currently-displayed word into the new one.
- `Backspace() (deleteRunes int, insert string, handled bool)` removes the
  last character of the composing word via `eng.RemoveLastChar(true)`. If no
  word is being composed, `handled` is `false` and the caller (main.go) must
  treat the Backspace as an ordinary key. If the word becomes empty after the
  removal, the engine is reset.

`diff(next string) (int, string)` is an unexported helper: it walks both the
old `displayed` string and `next` rune-by-rune, finds the common prefix, and
returns how many trailing runes of the old string differ (to delete) plus the
new trailing runes (to insert). Deletion is counted in runes, not bytes,
because each application Backspace removes exactly one rune.

### `internal/keymap` package

```go
const (
    KeyBackspace, KeyTab, KeyEnter,
    KeyLeftCtrl, KeyLeftShift, KeyRightShift, KeyLeftAlt, KeySpace,
    KeyCapsLock, KeyRightCtrl, KeyRightAlt, KeyLeftMeta, KeyRightMeta uint32
)

func Letter(code uint32) (rune, bool)
func IsModifier(code uint32) bool
```

Linux evdev keycodes (from `<linux/input-event-codes.h>`) plus a hardcoded
US-layout map of the 26 alphabetic keys to lowercase runes. `Letter` looks up
that map; `IsModifier` checks whether a code is one of the Ctrl/Shift/Alt/
Meta/CapsLock keys.

### `internal/appconfig` and `internal/hyprland`

- `appconfig.Load` reads `~/.config/gokey/app-strategies.json`; `StrategyFor`
  recognizes only `virtual-backspace-commit` and
  `surrounding-delete-commit`, otherwise returning the universal default.
- `hyprland.ActiveWindowFromEnvironment` resolves the current instance socket
  and sends `j/activewindow` with a deadline. `ActiveWindow.Identity` prefers
  `initialClass` and falls back to `class`.

### `internal/inputmethod` and `internal/virtualkeyboard`

Generated from `protocols/*.xml` via `go-wayland-scanner`. Exposed types used
by `main.go`:

- `ZwpInputMethodManagerV2` → `GetInputMethod(seat) (*ZwpInputMethodV2, error)`
- `ZwpInputMethodV2` → `GrabKeyboard()`, `CommitString(text)`,
  `Commit(serial)`, `DeleteSurroundingText(before, after)`, plus
  `SetActivateHandler`/`SetDeactivateHandler`/`SetDoneHandler`/
  `SetUnavailableHandler`/`SetSurroundingTextHandler`.
- `ZwpInputMethodKeyboardGrabV2` → `SetKeymapHandler`,
  `SetModifiersHandler`, `SetKeyHandler`.
- `ZwpVirtualKeyboardManagerV1` → `CreateVirtualKeyboard(seat)`.
- `ZwpVirtualKeyboardV1` → `Keymap(format, fd, size)`, `Key(time, key, state)`,
  `Modifiers(depressed, latched, locked, group)`.

One manual patch survives regeneration: `input_method.go` around
`CommitString` and the generated, currently unused `SetPreeditString` method
calls `client.PutString` with `len(text)+1` instead of the scanner's default
length calculation, because the upstream generator mis-sized the padded wire
buffer for strings whose byte length is not 4-aligned. If these bindings are
ever regenerated from the XML, this patch must be re-applied (see
`docs/code-standards.md`).

## `app` struct (main.go) — per-process and per-focus state

```go
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

    keymapSet bool
    consumed  map[uint32]bool
    vkTime    uint32

    ctrl, shift, alt, super bool
    capsLock                bool
    sawCtrlShift            bool
    chordOther              bool
}
```

- `ctx`/`display`/`registry` — Wayland connection plumbing, set up once in
  `main()`.
- `seat`/`imMgr`/`vkMgr` — bound Wayland globals, populated by `onGlobal`.
- `im`/`grab`/`vk` — the input method object, its keyboard grab, and the
  virtual keyboard, created in `setup()`.
- `tx` — the `*engine.Telex` for the word currently being composed.
- `serial` — bumped on every `zwp_input_method_v2::done` event; passed to
  `Commit()` calls.
- `vnOn` — whether Vietnamese transform is active (toggled by Ctrl+Shift);
  starts `true`.
- `active` — whether the currently focused client has granted input-method
  focus (set by activate/deactivate handlers). Keys are only transformed
  when `active` is true.
- `strategies` — immutable startup configuration; it is not reread per key.
- `activeWindow` — Hyprland metadata cached on activation and cleared on
  deactivation. Its identity selects the injection strategy.
- surrounding-text fields include the promoted byte cursor/anchor,
  `surrValid`, `surrDirty`, and `surrSerial`. Atomic replacement requires a
  clean snapshot whose serial still equals `serial`.
- `keymapSet` — whether the compositor's keymap has been forwarded to the
  virtual keyboard yet; `forward()` and `sendBackspace()` are no-ops until
  this is true.
- `consumed` — set of evdev keycodes whose key-release event should be
  swallowed because gokey consumed the corresponding press while composing
  or editing text.
- `vkTime` — monotonically increasing timestamp counter used for the two
  synthetic key events `sendBackspace()` sends to the virtual keyboard.
- `ctrl`/`shift`/`alt`/`super` — held-modifier booleans, updated by
  `updateModifier`.
- `capsLock` — latched (toggle-on-press) state.
- `sawCtrlShift`/`chordOther` — chord-detection bookkeeping: `sawCtrlShift`
  records that Ctrl+Shift were both down during the current modifier chord;
  `chordOther` records that some other (non-modifier) key was pressed during
  it, which disqualifies the chord from being a bare Ctrl+Shift toggle.

`resetPerFocusState()` clears `tx`, the surrounding-text snapshot, and
`consumed` — called on both activate and deactivate, since a focus change can
leave a key-release event for the previous client's grab unanswered.

## Tests

Tests cover pure engine behavior, strategy configuration, Hyprland IPC framing,
and the pure safety decision used by `main.go`. Actual Wayland injection is
still verified with the manual smoke test in the deployment guide.

| Test | What it checks |
|---|---|
| `TestTelexBasic` | Table of Telex sequences → Vietnamese words (`tieesng`→`tiếng`, `vieejt`→`việt`, `ddafi`→`đài`, `chuaarn`→`chuẩn`, `mootj`→`một`, `VIEETJ`→`VIỆT`, `aw`→`ă`, `xin`→`xin`, `chaof`→`chào`), applying diffs through a simulated app buffer |
| `TestBackspace` | One in-word Backspace shortens `việt` to `việ` |
| `TestResetEndsWord` | `Reset()` clears `Empty()`; a subsequent Backspace reports `handled == false` |
| `TestSurroundingDeleteBytes` | Converts a deleted multibyte rune suffix to the exact UTF-8 byte count |
| `TestSurroundingDeleteBytesRefusesUnsafeState` | Rejects invalid/dirty snapshots, stale serials, selections, bad cursors, suffix mismatches, and invalid rune counts |
| `TestLoadAndStrategyFor` | Loads Chromium mapping and makes unknown apps/values use the default |
| `TestQueryActiveWindow` | Sends the Hyprland IPC request and parses cached window identity/metadata |

Run with:

```bash
go test ./...
```
