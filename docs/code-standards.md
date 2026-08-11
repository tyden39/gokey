# gokey — Code Standards

## Module boundaries

- **`engine/`** contains only pure Telex logic. It imports `bamboo-core` and
  nothing else — no Wayland types, no `log`, no `os`. This keeps it testable
  with plain `go test` and reusable if gokey ever needs a second frontend.
  If you're fixing a Telex correctness bug (wrong tone, wrong diacritic,
  wrong word boundary), the fix belongs here, not in `main.go`.
- **`main.go`** contains all Wayland glue and text-injection logic: connecting
  to the compositor, binding globals, the key-event state machine, and both
  inject paths (`apply`/`sendBackspace` for direct mode, `setPreedit`/
  `flushPreedit` for preedit mode). Real-world bugs — a key that gets
  swallowed, a race between deleting and inserting, a launcher that stops
  responding — are almost always here.
- **`internal/inputmethod/`, `internal/virtualkeyboard/`** are generated
  Wayland protocol bindings produced by `go-wayland-scanner` from
  `protocols/*.xml`. Do not hand-edit these except for the one documented
  patch (see "Generated code" below). If the protocol XML changes, regenerate
  and re-apply the patch rather than hand-patching the diff.
- **`internal/keymap/`** is small and hand-written (not generated): evdev
  keycode constants and the US-layout letter map. Fine to edit directly.

## Generated code

`internal/inputmethod/input_method.go` carries one manual patch on top of
whatever `go-wayland-scanner` produces: the `CommitString` and
`SetPreeditString` methods call `client.PutString` with `len(text)+1` instead
of the scanner's default length, because the generator mis-sizes the padded
wire buffer for strings whose byte length is not a multiple of 4. This is
called out in a `NOTE:` comment directly above the affected code (around line
36 of that file). If these bindings are ever regenerated from
`protocols/input-method-unstable-v2.xml`, re-apply this fix before shipping —
without it, some committed strings will have their length field wrong on the
wire.

## Naming and formatting

- Standard Go conventions: exported identifiers in `CamelCase`, unexported in
  `camelCase`, file names in `snake_case.go`.
- Format with `gofmt` (or `go fmt ./...`) before committing; there is no
  separate lint config or CI pipeline in this repo — `gofmt` plus `go vet`
  are the baseline.
- Wayland-protocol-derived names (`ZwpInputMethodV2`, `zwp_virtual_keyboard_v1`,
  evdev key constants like `KeyBackspace`) keep the protocol's own casing —
  don't rename them to look more Go-idiomatic, since they need to map back to
  the XML/evdev headers for anyone cross-referencing the spec.

## Comments

Comments in this codebase explain **invariants and non-obvious reasoning**,
not narration of what the next line does. Examples already in the code that
set the pattern to follow:

- `apply()`'s doc-comment explains *why* fake Backspace is used instead of
  `delete_surrounding_text` (works in terminals too), not just what the
  function does.
- The `restoreRaw`/`restoreWord` field comment in the `app` struct explains
  the one-shot arming rule and which mode it applies to, because that's not
  derivable just from the field's type.
- The comment before `restoreRaw, restoreWord := a.restoreRaw, ...` at the
  top of `onKey` explains that restore is deliberately one-shot and why
  every other key invalidates it.

When adding a similar piece of state or a non-obvious inject decision, write
the comment as "why this and not the obvious alternative," matching that
existing tone. Avoid comments that just restate the code (e.g. `// increment
counter` above `i++`).

## Testing

- Engine logic (`engine/telex.go`) must have `go test ./engine` coverage for
  any new behavior — it's cheap to test since it has no Wayland dependency.
  Follow the existing pattern in `engine/telex_test.go`: build a `*Telex`,
  feed it a sequence of runes through `ProcessChar`, apply the returned
  diffs to a local `[]rune` buffer the way the frontend would, and assert on
  the final string. See `applyAll` and `feedTracked` for the two existing
  helpers.
- `main.go` has no automated tests; it's verified manually against a live
  compositor (see `docs/deployment-guide.md` for the smoke test). Keep
  Wayland-dependent logic thin enough that most real behavior can still be
  exercised through `engine` tests — e.g. `apply()`'s diff-application logic
  is intentionally mirrored by `engine`'s own diff tests rather than needing
  a live Wayland harness.

## Commits

Conventional commit format (`feat:`, `fix:`, `docs:`, `refactor:`, etc.), no
AI-authorship references in commit messages, matching this repo's existing
history.

## Design conventions worth preserving

- **Direct mode stays the default.** Preedit mode is an intentional opt-in
  (`Ctrl+Shift+Space`) because it trades a terminal-compatibility problem
  (fake-Backspace races) for a different one (some terminals don't render
  `set_preedit_string` at all). Don't flip the default without discussing the
  tradeoff — it was a deliberate user-facing choice, not an oversight.
- **Only one text-inject path per mode.** `apply()` in direct mode and
  `setPreedit`/`flushPreedit` in preedit mode are each meant to be the single
  way text reaches the focused app in that mode. Don't add a second
  conditional path (e.g. surrounding-text delete for some clients) without
  updating `docs/system-architecture.md` and `docs/project-roadmap.md`,
  since the current single-path design is what avoids ordering races.
- **Gate all Vietnamese-relevant handling on `a.active`.** Any new key
  handling added to `onKey` must still forward raw keys when `!a.active`
  (launchers, overlays) — this is the difference between an IME that plays
  nicely with the rest of the desktop and one that breaks app launchers.
