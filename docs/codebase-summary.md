# gokey — Codebase Summary

## Layout

```
gokey/
├── main.go                                  # Wayland app + key dispatch + toggle (299 LOC)
├── go.mod / go.sum                          # module gokey, Go 1.25
├── run.sh                                   # build + run, swaps out fcitx5 safely
├── engine/
│   ├── telex.go                             # Bamboo engine wrapper, diff logic (71 LOC)
│   └── telex_test.go                        # unit tests for composition/backspace/diff
├── internal/
│   ├── keymap/keymap.go                     # evdev keycode → letter, modifier consts (43 LOC)
│   ├── inputmethod/input_method.go          # generated input-method-v2 bindings (946 LOC, patched)
│   └── virtualkeyboard/virtual_keyboard.go  # generated virtual-keyboard-v1 bindings (283 LOC)
└── protocols/
    ├── input-method-unstable-v2.xml         # source spec for inputmethod bindings
    └── virtual-keyboard-unstable-v1.xml     # source spec for virtualkeyboard bindings
```

Total hand-written Go: ~415 LOC (`main.go` + `engine` + `keymap`). The two `internal/*`
protocol files are machine-generated and rarely edited.

## Hand-Written vs Generated

- **Hand-written (edit freely):** `main.go`, `engine/telex.go`, `engine/telex_test.go`,
  `internal/keymap/keymap.go`, `run.sh`.
- **Generated (treat as vendored):** `internal/inputmethod/input_method.go`,
  `internal/virtualkeyboard/virtual_keyboard.go` — produced by `go-wayland-scanner` from
  `protocols/*.xml`. One **manual patch** lives in `input_method.go` (PutString length fix,
  see Code Standards); re-apply it if regenerating.

## Public API Surface (internal packages)

**`engine`**
- `New() *Telex`
- `(*Telex) Reset()`, `Empty() bool`
- `(*Telex) ProcessChar(r rune) (deleteBytes int, insert string)`
- `(*Telex) Backspace() (deleteBytes int, insert string, handled bool)`

**`keymap`**
- `Letter(code uint32) (rune, bool)`
- `IsModifier(code uint32) bool`
- Constants: `KeyBackspace`, `KeyLeftCtrl`, `KeyLeftShift`, … `KeyCapsLock`, etc.

**`inputmethod` / `virtualkeyboard`**: generated `Zwp*` proxy types, their requests
(`CommitString`, `DeleteSurroundingText`, `Commit`, `GrabKeyboard`, `Key`, `Keymap`,
`Modifiers`, …) and event handler setters.

## Core Data: `app` struct (`main.go`)

Holds Wayland objects (`ctx/display/registry/seat/im/grab/vk` + managers), the Telex
engine `tx`, the IM `serial`, and runtime flags:
- `vnOn` (Vietnamese active), `active` (text input focused),
- `keymapSet`, `consumed map[uint32]bool` (press/release pairing),
- modifier booleans `ctrl/shift/alt/super`, `capsLock`,
- chord-toggle bookkeeping `sawCtrlShift`, `chordOther`.

## Build, Run, Test

```bash
go build -o gokey .       # build
go test ./...             # run engine tests
./run.sh                  # build + run (stops fcitx5, GOKEY_DEBUG=1)
GOKEY_DEBUG=1 ./gokey      # run with verbose tracing
```

Manual smoke test: in a Wayland terminal that uses text-input-v3
(`env -u GTK_IM_MODULE -u QT_IM_MODULE -u XMODIFIERS foot`), type `tieesng vieejt`
→ should produce `tiếng việt`. Toggle with Ctrl+Shift.

## Tests

`engine/telex_test.go` covers:
- `TestTelexBasic` — table of Telex inputs → expected Vietnamese (incl. uppercase `VIEETJ`→`VIỆT`).
- `TestBackspace` — backspace edits the composing word.
- `TestDiffDeletesUTF8Bytes` — diff returns UTF-8 byte counts, not rune counts.
- `TestResetEndsWord` — `Reset()` clears word; backspace after reset is unhandled.

No tests cover `main.go` (Wayland-bound; exercised via manual smoke test).
