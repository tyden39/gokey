# CLAUDE.md — gokey

Vietnamese Telex input method for Wayland. Go, no preedit by default. Uses
`zwp_input_method_v2` (commit text) + `zwp_virtual_keyboard_v1` (forward/Backspace).
Telex transform is delegated to `bamboo-core`.

> Note: the global `~/.claude/CLAUDE.md` is for a marketing project — ignore it here.

## Commands

```bash
go build -o gokey .         # build
go test ./engine            # run engine tests (Telex transform)
./run.sh                    # stops installed gokey, builds, runs dev binary (GOKEY_DEBUG=1)
GOKEY_DEBUG=1 ./gokey       # run with key-by-key debug logging
```

Only one input method can run at a time — stop the installed/autostarted gokey
before running a dev build (`run.sh` does this for you).
Toggle Vietnamese with **Ctrl+Shift**. Toggle direct-commit ↔ preedit mode with
**Ctrl+Shift+Space** (direct is the default).

## Layout

- `engine/telex.go` — wraps bamboo-core for one in-progress word; returns a
  (deleteRunes, insert) diff. Pure logic, well tested. **Bugs are usually NOT here.**
- `main.go` — the frontend: Wayland glue + how text is injected into apps
  (`onKey`, `apply`, `sendBackspace`, `forward`). **Most real-world bugs live here.**
- `internal/` — generated Wayland protocol bindings (inputmethod, virtualkeyboard,
  keymap). Don't hand-edit unless regenerating.

## Known sharp edges (read before touching `main.go`)

- **Live typing has ONE inject path (`apply` in `main.go`): fake Backspace via
  virtual-keyboard + `commit_string`.** It works everywhere including terminals, but
  some GUI fields (Chrome omnibox) race a fake Backspace against their own text-input
  state — switching capable clients to `delete_surrounding_text` is a known candidate
  improvement, not yet implemented.
- **Cursor-word adoption (`adoptWord` in `main.go`).** With no word composing, a
  tone/mark key (`s f r x j w z`) adopts the committed word the cursor touches:
  `surrounding_text` snapshot → `wordAtCursor` → `tx.ResumeWord` (reverse-Telex +
  replay-verify in `engine/reverse.go`) → atomic `delete_surrounding_text` +
  `commit_string` (or reopen as preedit). Guards, all load-bearing: only fires in
  clients that send `surrounding_text` (terminals never do); refuses unless the key
  really transforms the word AND the result passes `SpellingValid` (else `hello`+s
  would rewrite to `helló`); `surrDirty` blocks adoption after any forwarded
  key press / fake Backspace / commit until a FRESH `surrounding_text`+`done` arrives
  (else `delete_surrounding_text` deletes at a stale offset); snapshot is one-shot
  per event; refuses selections, cap-exceeding words, and mid-identifier truncation.
- **Only consume keys when `a.active` is true.** When the focused app has no
  text-input (drun launchers: wofi/fuzzel/rofi, layer-shell overlays), gokey must
  `forward()` raw keys, never `commit_string` — else keys get swallowed.
- `forward()` silently drops keys if `!keymapSet` — watch for that when "nothing types".
- **Two composition modes, toggled by Ctrl+Shift+Space (`a.preedit`).** Direct
  (default): each key applies a (delete, insert) diff live via `apply` (fake
  Backspace + `commit_string`); works in terminals. Preedit: word shown via
  `set_preedit_string` (`setPreedit`) and committed on word-end (`flushPreedit`);
  no Backspace race but terminals may not render it. `endWord` ends the current
  word per mode (flush vs reset); both the vnOn toggle and word-ending keys call
  it. Switching modes flushes the in-progress word so nothing is left dangling.

- **Backspace-restore (direct mode).** Ending a word with a single Space stashes
  its raw keys + on-screen form (`restoreRaw`/`restoreWord`); the next Backspace
  deletes the space via `sendBackspace` (consumed, never forwarded — a forwarded
  bare press would client-auto-repeat into the word) and `tx.RestoreWord` replays
  the keys so tone keys keep editing the word. One-shot: any other key press,
  focus change, or VN toggle disarms. `RestoreWord` verifies the replay equals
  the on-screen word and refuses otherwise — replay is NOT always faithful
  (in-word Backspace after a tone-cancel like `cass`⌫ leaves raw keys that
  replay to `cá` while the screen shows `ca`).

## Conventions

- Keep files focused; engine logic stays in `engine/`, Wayland/inject in `main.go`.
- Conventional commits, no AI references.
- Preedit mode exists as an opt-in toggle (default off) so the terminal-vs-GUI
  trade-off stays the user's choice; keep direct mode the default.
