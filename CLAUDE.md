# CLAUDE.md — gokey

Vietnamese Telex input method for Wayland. Go, direct composition only. Uses
`zwp_input_method_v2` (commit text) + `zwp_virtual_keyboard_v1` (forward/Backspace).
Telex transform is delegated to `bamboo-core`.

> Note: the global `~/.claude/CLAUDE.md` is for a marketing project — ignore it here.

## Commands

```bash
go build -o gokey .         # build
go test ./...               # run all unit tests
./run.sh                    # stops installed gokey, builds, runs dev binary (GOKEY_DEBUG=1)
GOKEY_DEBUG=1 ./gokey       # run with key-by-key debug logging
```

Only one input method can run at a time — stop the installed/autostarted gokey
before running a dev build (`run.sh` does this for you).
Toggle Vietnamese with **Ctrl+Shift**.

## Layout

- `engine/telex.go` — wraps bamboo-core for one in-progress word; returns a
  (deleteRunes, insert) diff. Pure logic, well tested. **Bugs are usually NOT here.**
- `main.go` — the frontend: Wayland glue + how text is injected into apps
  (`onKey`, `apply`, `sendBackspace`, `forward`). **Most real-world bugs live here.**
- `internal/appconfig/` — loads the per-app strategy file once at startup.
- `internal/hyprland/` — queries active-window identity on input-method activation.
- `internal/inputmethod/`, `internal/virtualkeyboard/` — generated Wayland bindings;
  don't hand-edit unless regenerating. `internal/keymap/` is hand-written.

## Known sharp edges (read before touching `main.go`)

- **Live typing chooses one of two named strategies in `apply`.** The default
  `virtual-backspace-commit` works in terminals. Chromium is configured for
  `surrounding-delete-commit`, but that path is allowed only when the snapshot is
  valid, clean, selection-free, on the current serial, and the exact UTF-8 suffix is
  still immediately before the cursor. Any failed check falls back for that key.
- **Active-app identity is cached, not polled per key.** On input-method activation,
  `internal/hyprland` queries `j/activewindow` with a 100 ms deadline and caches
  `initialClass` (falling back to `class`) until deactivation. IPC or config failure
  selects the default strategy and must never stop typing.
- **Only consume keys when `a.active` is true.** When the focused app has no
  text-input (drun launchers: wofi/fuzzel/rofi, layer-shell overlays), gokey must
  `forward()` raw keys, never `commit_string` — else keys get swallowed.
- `forward()` silently drops keys if `!keymapSet` — watch for that when "nothing types".
- **One composition mode.** Each letter applies a (delete, insert) diff live via
  `apply`; the selected injection strategy changes transport only. `endWord` resets
  the engine because visible text has already been committed.

## Conventions

- Keep files focused; engine logic stays in `engine/`, Wayland/inject in `main.go`.
- Conventional commits, no AI references.
- Keep strategy dispatch in `apply`; new strategies must preserve the default fallback
  and must not duplicate Bamboo/Telex behavior.
