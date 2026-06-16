# gokey — Code Standards & Conventions

## Language & Tooling

- **Go 1.25** (`go.mod`). Standard `gofmt` formatting (tabs, standard import grouping).
- Build: `go build -o gokey .`  •  Test: `go test ./...`  •  Vet: `go vet ./...`.
- No linter config committed; keep code `gofmt`-clean and `go vet`-clean.

## Project Structure Conventions

- `main` package at repo root (the Wayland app).
- Reusable logic under `internal/` (not importable outside the module) and `engine/`.
- One concern per package: `engine` (Telex), `keymap` (keycodes), `inputmethod` &
  `virtualkeyboard` (protocol bindings).
- Files are small and focused; hand-written files stay well under 300 LOC.

## Naming

- Packages: short lowercase (`engine`, `keymap`, `inputmethod`, `virtualkeyboard`).
- Exported identifiers documented with a leading doc comment in the Go style
  (`// Telex wraps …`). Keep comments explaining *why*, not just *what*.
- evdev key constants prefixed `Key…` (`KeyBackspace`, `KeyLeftShift`).

## Generated Code Rules (IMPORTANT)

`internal/inputmethod/input_method.go` and `internal/virtualkeyboard/virtual_keyboard.go`
are produced by `go-wayland-scanner` from `protocols/*.xml`. Treat them as vendored:

- Prefer changing the XML + regenerating over hand-editing, **except** for the documented patch below.
- **Manual patch (must survive regeneration):** in `input_method.go`, `PutString` calls pass
  `len(text)+1` as the wire string-length prefix instead of go-wayland's padded length.
  go-wayland wrote the *padded* length, producing malformed `commit_string` /
  `set_preedit_string` messages for strings whose byte length isn't 4-aligned. See the
  `NOTE:` block at the top of the file. Re-apply after any regeneration.

## Wayland / Encoding Invariants

- **`delete_surrounding_text` counts UTF-8 bytes, not runes.** All diff math in
  `engine/telex.go` returns byte counts; callers must not pass rune counts.
- **Commit atomically.** Patch a word with one `delete_surrounding_text` + `commit_string`
  + `commit(serial)` sequence. Do **not** emit real Backspace key events to edit composed
  text — Chrome autocomplete reacts to intermediate edits and breaks sequences like
  `dd` → `đ` (see comment on `app.apply` in `main.go`).
- **`serial`** passed to `Commit` must equal the number of `done` events received; it is
  incremented in the done handler.
- **Close keymap fds.** After forwarding a keymap fd to the virtual keyboard, close it
  (`unix.Close`) to avoid fd leaks.

## Input-Handling Conventions

- Pair key press/release via the `consumed` map: if a press was swallowed by gokey, swallow
  its release too; otherwise forward both.
- Passthrough rule: when `!active || !vnOn || ctrl || alt || super`, forward the key and
  `Reset()` the engine (modifiers/inactivity end the current word).
- Word-breaking keys (space, punctuation, enter, unknown) `Reset()` the engine before forwarding.

## Error Handling

- Startup/setup failures use `log.Fatalf` (unrecoverable: no compositor support, grab
  failure, another IM active). This is intentional for a single-purpose daemon.
- Per-request errors from protocol writes are generally ignored in the hot path (best-effort
  forwarding); guard with `keymapSet` / nil checks before using `vk`.

## Debugging

- Gate verbose logging behind `GOKEY_DEBUG` via the `dbg(...)` helper — no unconditional
  trace logging in the key path.

## Testing Conventions

- Engine logic is unit-tested in `engine/telex_test.go` using table-driven cases and an
  `applyAll` helper that simulates the app's text buffer by applying byte diffs.
- Wayland-bound code (`main.go`) is validated by manual smoke test (see Codebase Summary),
  not unit tests.

## Comments & Naming Hygiene

- Comments explain invariants and rationale (byte-vs-rune, Chrome autocomplete, serial
  semantics). Keep them when refactoring.
- Do not reference plan/phase/finding artifacts in code comments or filenames — explain the
  reason itself, not its origin.
