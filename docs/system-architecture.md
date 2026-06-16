# gokey — System Architecture

## High-Level Flow

```
Hardware keyboard
      │  (compositor routes all keys to the grab holder)
      ▼
┌─────────────────────────────────────────────────────────┐
│ gokey process                                            │
│                                                          │
│  zwp_input_method_v2  ◄── grab keyboard ──┐              │
│        │  key/modifier/keymap events       │             │
│        ▼                                   │             │
│   app.onKey (main.go)  ── decides ──►  Telex engine      │
│        │                                   │  (bamboo)   │
│        │  Vietnamese:                       ▼            │
│        │    delete_surrounding_text + commit_string      │
│        │  Passthrough:                                    │
│        └──► zwp_virtual_keyboard_v1.key ─────────────────┤
└─────────────────────────────────────────────────────────┘
      │                                   │
      ▼                                   ▼
 Focused text-input-v3 app          Focused app (raw key)
```

Two Wayland objects do the work:

1. **`zwp_input_method_v2`** (+ its keyboard grab): receives every key, and is used to
   *commit* transformed Vietnamese text to the focused application.
2. **`zwp_virtual_keyboard_v1`**: re-injects keys that should pass through unchanged
   (modifiers, non-letters, keys typed while VN is off or a non-shift modifier is held).
   The grabbed keymap is forwarded to the virtual keyboard so injected keycodes map
   correctly.

## Components

| Component | File | Responsibility |
|-----------|------|----------------|
| App / event loop | `main.go` | Wayland connect, registry bind, grab setup, key/modifier dispatch, toggle logic. |
| Telex engine wrapper | `engine/telex.go` | Wrap bamboo-core; produce `(deleteBytes, insert)` diffs; track displayed word. |
| Keymap | `internal/keymap/keymap.go` | evdev keycode → US lowercase letter; modifier/backspace constants. |
| input-method bindings | `internal/inputmethod/input_method.go` | Generated `zwp_input_method_*` protocol client (manually patched). |
| virtual-keyboard bindings | `internal/virtualkeyboard/virtual_keyboard.go` | Generated `zwp_virtual_keyboard_*` protocol client. |
| Protocol XML | `protocols/*.xml` | Source specs the Go bindings were generated from. |

## Key Decision: No Preedit, Diff-Based Commit

`gokey` does **not** use `set_preedit_string`. Instead each keystroke recomputes the full
Vietnamese word from the engine and applies the minimal change to already-committed text:

- `engine.Telex` keeps `displayed` = what the app currently shows for the in-progress word.
- After feeding a key, it computes the longest common rune prefix between `displayed` and
  the new word, then returns:
  - `deleteBytes` = trailing UTF-8 bytes of the old word to remove
    (Wayland `delete_surrounding_text` counts **bytes**, not runes),
  - `insert` = the suffix to commit.
- `app.apply` issues `DeleteSurroundingText` + `CommitString` + `Commit(serial)` as one
  input-method commit.

Rationale (see comment in `main.go:apply`): sending real Backspace key events to patch the
word makes Chrome's autocomplete observe intermediate edits, breaking first-character Telex
sequences like `dd` → `đ`. A single atomic IM commit avoids that.

## Toggle State Machine (Ctrl+Shift)

Tracked in `app.updateModifier` (`main.go`):

- On first modifier press of a chord: reset `chordOther` and `sawCtrlShift`.
- While both Ctrl and Shift are down: set `sawCtrlShift`.
- Any non-modifier key during the chord sets `chordOther = true` (so Ctrl+Shift+C won't toggle).
- When all modifiers release: if `sawCtrlShift && !chordOther`, flip `vnOn`.

CapsLock is tracked as a latching boolean (toggles on press).

## Event Handling Detail (`app.onKey`)

1. Modifier key → update state, forward, return.
2. Key release → if the press was consumed by gokey, swallow the release; else forward.
3. If inactive / VN off / Ctrl|Alt|Super held → reset engine, forward (passthrough).
4. Backspace → ask engine; if composing, apply diff and consume; else forward.
5. Letter key → uppercase if needed, feed engine, apply diff, consume.
6. Any other key (space, punctuation, enter) → reset engine (ends the word), forward.

`serial` increments on each input-method `done` event and is passed to `Commit`.

## External Dependencies

| Dependency | Purpose |
|------------|---------|
| `github.com/BambooEngine/bamboo-core` | Telex → Vietnamese transformation engine. |
| `github.com/rajveermalviya/go-wayland/wayland/client` | Wayland client runtime (Context, Display, Registry, Seat). |
| `golang.org/x/sys/unix` | `unix.Close` for keymap fds. |

## Lifecycle

1. `client.Connect("")` → bind `wl_seat`, `input_method_manager_v2`, `virtual_keyboard_manager_v1`.
2. `GetInputMethod` + `CreateVirtualKeyboard`; register activate/deactivate/done/unavailable handlers.
3. `GrabKeyboard`; register keymap/modifiers/key handlers.
4. Loop on `ctx.Dispatch()` forever. Fatal-logs on protocol error or missing globals.
