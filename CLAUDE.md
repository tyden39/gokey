# CLAUDE.md — gokey

Vietnamese Telex input method for Wayland. Go, no preedit by default. Uses
`zwp_input_method_v2` (commit text) + `zwp_virtual_keyboard_v1` (forward/Backspace).
Telex transform is delegated to `bamboo-core`.

> Note: the global `~/.claude/CLAUDE.md` is for a marketing project — ignore it here.

## Commands

```bash
go build -o gokey .         # build
go test ./engine            # run engine tests (Telex transform)
./run.sh                    # stops fcitx5, builds, runs gokey with GOKEY_DEBUG=1
GOKEY_DEBUG=1 ./gokey       # run with key-by-key debug logging
```

Only one input method can run at a time — stop fcitx5/ibus before running gokey.
Toggle Vietnamese with **Ctrl+Shift**.

## Layout

- `engine/telex.go` — wraps bamboo-core for one in-progress word; returns a
  (deleteRunes, insert) diff. Pure logic, well tested. **Bugs are usually NOT here.**
- `main.go` — the frontend: Wayland glue + how text is injected into apps
  (`onKey`, `apply`, `sendBackspace`, `forward`). **Most real-world bugs live here.**
- `internal/` — generated Wayland protocol bindings (inputmethod, virtualkeyboard,
  keymap). Don't hand-edit unless regenerating.

## Known sharp edges (read before touching `main.go`)

- **Text-inject has two paths, picked per focused client (`apply` in `main.go`).**
  When the client supports text-input it sends a `surrounding_text` event → set
  `a.surround` → delete via `delete_surrounding_text` + `commit_string` in one atomic
  `commit()` (no ordering race; fixes Chrome omnibox `aa→aâ` and Facebook chat). When
  it doesn't (terminals never send `surrounding_text`) → fall back to fake Backspace
  via virtual-keyboard. `a.surround` resets on (de)activate, so it tracks the focused
  client. Don't collapse back to a single path — terminals have no surrounding text to
  delete, GUI fields race on fake Backspace.
- **Only consume keys when `a.active` is true.** When the focused app has no
  text-input (drun launchers: wofi/fuzzel/rofi, layer-shell overlays), gokey must
  `forward()` raw keys, never `commit_string` — else keys get swallowed.
- `forward()` silently drops keys if `!keymapSet` — watch for that when "nothing types".

## Conventions

- Keep files focused; engine logic stays in `engine/`, Wayland/inject in `main.go`.
- Conventional commits, no AI references.
- Don't add a preedit/inject mode without deciding the terminal-vs-GUI trade-off first.
