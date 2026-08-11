# gokey — Roadmap

Kept short and honest: only items with real evidence in the code or recent
plans go here. No aspirational feature list.

## Done

- **Core Telex composition** via bamboo-core — tones, diacritics, `đ`,
  uppercase, verified by `TestTelexBasic` in `engine/telex_test.go`.
- **Vietnamese on/off toggle** — Ctrl+Shift chord, implemented in
  `updateModifier` in `main.go`.
- **Preedit mode toggle** — Ctrl+Shift+Space switches between direct-commit
  (default) and `set_preedit_string`-based composition, implemented in
  `togglePreedit`/`setPreedit`/`flushPreedit` in `main.go`.
- **Backspace-restore after a single Space** — ending a word with Space and
  then pressing Backspace once resumes composing it instead of leaving tone
  keys to type plain letters (`vi` + Space + Backspace + `f` → `vì`).
  Direct mode only. Implemented via `engine.Telex.RawKeys`/`RestoreWord` and
  the `restoreRaw`/`restoreWord` stash in `main.go`; refuses to restore when
  replay wouldn't reproduce the on-screen word. See
  `plans/260730-1033-backspace-restore-word/plan.md` for the original design
  and code-review history.
- **drun/launcher passthrough** — keys are only transformed when
  `a.active` is true (the focused client has granted input-method focus);
  everything else, including full alphabetic input, is forwarded raw so
  application launchers (wofi, fuzzel) and layer-shell overlays keep working
  normally.

## Candidate future work

Nothing below is scheduled or committed to — these are known gaps or
weaknesses observed while building the features above, listed so they aren't
rediscovered from scratch.

- **A surrounding-text-based inject path for GUI clients.** Right now
  `apply()` has exactly one inject path (fake Backspace + `commit_string`)
  for every client, which is what makes terminals work at all — but it is a
  known weakness for some GUI clients (e.g. a race window in Chrome's
  omnibox between the fake Backspace and the client's own text-state
  update) that a `delete_surrounding_text`-based path could avoid for
  clients that support `zwp_input_method_v2::surrounding_text`. Would need
  a second inject path selected per-client, which the current codebase
  deliberately avoids (see `docs/code-standards.md`) — so this needs a
  careful design pass before implementation, not a quick patch.
- **Other input methods (e.g. VNI).** gokey is hardcoded to Bamboo's
  `"Telex"` input method (`bamboo.ParseInputMethod(bamboo.InputMethodDefinitions,
  "Telex")` in `engine.New()`). Supporting VNI or VIQR would mean making that
  a runtime choice.
- **A configuration file.** There is currently no config file at all —
  Vietnamese-on-by-default, direct mode default, and all keybindings are
  compiled in. A config file would be needed before exposing any of this to
  users who aren't comfortable rebuilding the binary.
- **Multi-word restore history.** Backspace-restore currently stashes only
  the single most-recently-ended word. Restoring further back (e.g. after
  two Spaces) is out of scope for the current one-shot design and would need
  a different data structure (a small history/stack) plus a decision about
  how far back "undo" should reasonably reach.

## Explicitly out of scope

- X11/XWayland support — XWayland apps don't speak `text-input-v3`, so
  there's nothing for gokey's Wayland protocols to attach to.
- A plugin/extension system — gokey is intentionally a single small binary,
  not an IME framework.
