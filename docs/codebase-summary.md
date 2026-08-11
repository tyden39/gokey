# gokey — Codebase Summary

## Layout

```
gokey/
├── main.go                          410 lines — Wayland app: connects, binds
│                                     globals, grabs keyboard, handles every
│                                     key event, injects text. All real-world
│                                     bugs live here.
├── engine/
│   ├── telex.go                     104 lines — wraps bamboo-core for one
│   │                                 in-progress word; pure logic, no
│   │                                 Wayland dependency.
│   └── telex_test.go                155 lines — engine unit tests.
├── internal/
│   ├── inputmethod/input_method.go  946 lines — generated bindings for
│   │                                 zwp_input_method_v2 (+ manual patch).
│   ├── virtualkeyboard/
│   │   virtual_keyboard.go          283 lines — generated bindings for
│   │                                 zwp_virtual_keyboard_v1.
│   └── keymap/keymap.go             43 lines — evdev keycodes, US-layout
│                                     letter map, modifier detection.
├── protocols/
│   ├── input-method-unstable-v2.xml   upstream protocol spec (source for
│   └── virtual-keyboard-unstable-v1.xml the internal/ generated bindings)
├── run.sh                           28 lines — dev build/run script.
├── go.mod / go.sum                  module deps.
├── README.md
└── docs/                            this documentation set.
```

Total hand-written Go: main.go (410) + engine (104 + 155) + keymap (43) =
~712 lines. Generated protocol bindings: 946 + 283 = 1229 lines, not
hand-maintained.

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
func (t *Telex) RawKeys() string
func (t *Telex) RestoreWord(raw, onScreen string) bool
func (t *Telex) ProcessChar(r rune) (deleteRunes int, insert string)
func (t *Telex) Backspace() (deleteRunes int, insert string, handled bool)
```

- `New()` builds a `Telex` around a fresh `bamboo.IEngine` configured for the
  `"Telex"` input method with `bamboo.EstdFlags`.
- `Reset()` ends the current word in the engine; it does not touch any text
  already committed to the focused application.
- `Empty()` reports whether `displayed` (the currently-composed word as it
  should appear on screen) is the empty string.
- `Current()` returns that same `displayed` string — used by preedit mode,
  which needs the whole word rather than a diff.
- `RawKeys()` returns `eng.GetProcessedString(bamboo.EnglishMode)`, i.e. the
  original keystrokes typed for the current word with case preserved (e.g.
  `"vieejt"` while the screen shows `"việt"`).
- `RestoreWord(raw, onScreen string) bool` resets the engine, replays `raw`
  through `bamboo.VietnameseMode`, and compares the result to `onScreen`. If
  they match, the engine's `displayed` is set to `onScreen` and composition
  can continue; if they don't match, the engine is reset again, `displayed`
  is left empty, and the method returns `false`. Replay is not always
  faithful — see "Known limitation" below.
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

**Known limitation (by design, not a bug):** an in-word Backspace that lands
right after a tone-cancel key pair can leave raw keystrokes that replay to a
different word than what's on screen — e.g. typing `"cass"` shows `"ca"` on
screen (the second `s` cancels the sắc tone), but replaying the raw string
`"cas"` produces `"cá"`. `RestoreWord` detects this by comparing the replay
result against the caller-supplied `onScreen` string and refuses the restore
rather than silently reintroducing a diacritic. Covered by
`TestRestoreWordRefusesUnfaithfulReplay`.

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

### `internal/inputmethod` and `internal/virtualkeyboard`

Generated from `protocols/*.xml` via `go-wayland-scanner`. Exposed types used
by `main.go`:

- `ZwpInputMethodManagerV2` → `GetInputMethod(seat) (*ZwpInputMethodV2, error)`
- `ZwpInputMethodV2` → `GrabKeyboard()`, `CommitString(text)`,
  `SetPreeditString(text, cursorBegin, cursorEnd)`, `Commit(serial)`,
  `DeleteSurroundingText(before, after)` (bound but **not called** by
  main.go — see the architecture doc for why), plus
  `SetActivateHandler`/`SetDeactivateHandler`/`SetDoneHandler`/
  `SetUnavailableHandler`/`SetSurroundingTextHandler`.
- `ZwpInputMethodKeyboardGrabV2` → `SetKeymapHandler`,
  `SetModifiersHandler`, `SetKeyHandler`.
- `ZwpVirtualKeyboardManagerV1` → `CreateVirtualKeyboard(seat)`.
- `ZwpVirtualKeyboardV1` → `Keymap(format, fd, size)`, `Key(time, key, state)`,
  `Modifiers(depressed, latched, locked, group)`.

One manual patch survives regeneration: `input_method.go` around
`CommitString`/`SetPreeditString` calls `client.PutString` with
`len(text)+1` instead of the scanner's default length calculation, because
the upstream generator mis-sized the padded wire buffer for strings whose
byte length is not 4-aligned. If these bindings are ever regenerated from the
XML, this patch must be re-applied (see `docs/code-standards.md`).

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

    tx      *engine.Telex
    serial  uint32
    vnOn    bool
    active  bool
    preedit bool // false: commit live diff; true: show set_preedit_string

    restoreRaw  string // non-empty arms Backspace-restore
    restoreWord string

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
- `preedit` — composition mode; `false` (direct/live-commit) is the default,
  toggled by Ctrl+Shift+Space.
- `restoreRaw`/`restoreWord` — Backspace-restore stash: the raw keystrokes
  and on-screen form of the last word ended by a single Space, direct mode
  only. Cleared (one-shot) at the very start of handling any key press.
- `keymapSet` — whether the compositor's keymap has been forwarded to the
  virtual keyboard yet; `forward()` and `sendBackspace()` are no-ops until
  this is true.
- `consumed` — set of evdev keycodes whose key-release event should be
  swallowed because gokey used the key press itself (Backspace-restore,
  Ctrl+Shift+Space) rather than forwarding it.
- `vkTime` — monotonically increasing timestamp counter used for the two
  synthetic key events `sendBackspace()` sends to the virtual keyboard.
- `ctrl`/`shift`/`alt`/`super` — held-modifier booleans, updated by
  `updateModifier`.
- `capsLock` — latched (toggle-on-press) state.
- `sawCtrlShift`/`chordOther` — chord-detection bookkeeping: `sawCtrlShift`
  records that Ctrl+Shift were both down during the current modifier chord;
  `chordOther` records that some other (non-modifier) key was pressed during
  it, which disqualifies the chord from being a bare Ctrl+Shift toggle.

`resetPerFocusState()` clears `tx`, `restoreRaw`/`restoreWord`, and
`consumed` — called on both activate and deactivate, since a focus change can
leave a key-release event for the previous client's grab unanswered.

## Tests

All tests live in `engine/telex_test.go` (`go test ./engine`); `main.go` has
no automated tests since it depends on a live Wayland compositor connection —
it is verified with the manual smoke test in `docs/deployment-guide.md`.

| Test | What it checks |
|---|---|
| `TestTelexBasic` | Table of Telex sequences → Vietnamese words (`tieesng`→`tiếng`, `vieejt`→`việt`, `ddafi`→`đài`, `chuaarn`→`chuẩn`, `mootj`→`một`, `VIEETJ`→`VIỆT`, `aw`→`ă`, `xin`→`xin`, `chaof`→`chào`), applying diffs through a simulated app buffer |
| `TestBackspace` | One in-word Backspace shortens `việt` to `việ` |
| `TestRawKeys` | `RawKeys()` returns the original typed string (`"tieengs"`) and is empty after `Reset()` |
| `TestRestoreWordContinuesTone` | `vi`+Space+restore+`f`→`vì`; `tieeng`+restore+`s`→`tiếng`; `Vieet`+restore+`j`→`Việt` (uppercase round-trip) |
| `TestRestoreWordRefusesUnfaithfulReplay` | `cass`+in-word-Backspace (screen `ca`) refuses to restore because raw `cas` replays to `cá` |
| `TestRestoreWordThenBackspace` | After a successful restore, a further Backspace is still handled and shortens the word |
| `TestResetEndsWord` | `Reset()` clears `Empty()`; a subsequent Backspace reports `handled == false` |

Run with:

```bash
go test ./engine
```
