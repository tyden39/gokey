# gokey

A minimal **Vietnamese Telex input method for Wayland**, written in Go.

Type `tieesng vieejt` → get **tiếng việt** — in any Wayland app that supports
`text-input-v3`, with no IBus/fcitx5 framework required. `gokey` is a single Go binary
that grabs the keyboard through the standard `zwp_input_method_v2` protocol and commits
transformed Vietnamese text directly.

## Features

- **Telex** composition via [BambooEngine/bamboo-core](https://github.com/BambooEngine/bamboo-core)
  (tones, diacritics, `đ`, uppercase).
- **Direct mode by default** — commits text and patches it in place with fake Backspace
  key events + `commit_string` diffs, which works in terminals as well as GUI apps (no
  `delete_surrounding_text`, which most terminals don't support anyway).
- **Preedit mode (opt-in)** — toggle with **Ctrl+Shift+Space** to show the in-progress
  word via `set_preedit_string` instead; better for some GUI fields, but many terminals
  won't render it, which is why direct mode is the default.
- **Toggle Vietnamese with Ctrl+Shift.**
- **Backspace-restore** — end a word with one Space, then hit Backspace once to resume
  composing it (tone keys keep working): `vi` + Space + Backspace + `f` → `vì`. Direct
  mode only, one-shot (any other key disarms it).
- **Cursor-word editing** — click inside or right after an already-typed word and press a
  tone/mark key (`s f r x j w z`) to edit it: cursor after `chao` + `f` → `chào`. Works in
  both modes, but only in apps that report surrounding text (most GUI apps; terminals
  can't). The edit applies to the whole word and leaves the cursor at its end. Words that
  wouldn't become valid Vietnamese are left untouched (`hello` + `s` just types `s`),
  but short English words that are also valid Vietnamese syllables do get edited
  (`cat` + `s` → `cát`) — same as what live typing would produce.
- Passes through all non-Vietnamese keys (shortcuts, modifiers, punctuation) unchanged via
  `zwp_virtual_keyboard_v1`, and forwards everything untouched when the focused window
  isn't a text-input client (e.g. app launchers like wofi/fuzzel).
- Tiny: ~410 lines of hand-written Go in `main.go`, plus a ~100-line engine wrapper.

## Requirements

- Linux + a **Wayland compositor** exposing both `zwp_input_method_manager_v2` and
  `zwp_virtual_keyboard_manager_v1` (e.g. Sway, Hyprland, river).
- **Go 1.25+** to build.
- No other input method holding the seat's input-method slot (`run.sh` stops the
  installed/autostarted gokey for you before running a dev build).

## Build & Run

```bash
go build -o gokey .          # build
go test ./engine             # run engine unit tests (Telex transform)
./run.sh                     # build + run (stops installed gokey, relaunches on exit, debug on)
```

Or run directly:

```bash
GOKEY_DEBUG=1 ./gokey         # GOKEY_DEBUG enables verbose tracing
```

### Usage

- **Ctrl+Shift** — toggle Vietnamese transform on/off.
- **Ctrl+Shift+Space** — toggle direct-commit mode ↔ preedit mode (direct is the default).
- Typing a word, ending it with Space, then pressing Backspace once resumes composing that
  word instead of leaving tone keys to type plain letters.

### Try it

In another window, open a terminal that uses `text-input-v3`:

```bash
env -u GTK_IM_MODULE -u QT_IM_MODULE -u XMODIFIERS foot
```

Then type `tieesng vieejt` → `tiếng việt`. Press **Ctrl+Shift** to toggle Vietnamese off/on.

## How It Works

```
keyboard ──► zwp_input_method_v2 (grab) ──► gokey ──► Telex engine (bamboo)
                                              │
     Direct mode:   fake Backspace (virtual keyboard) + commit_string ──► focused app
     Preedit mode:  set_preedit_string, committed on word end ──────────► focused app
     Passthrough:   zwp_virtual_keyboard_v1.key ───────────────────────► focused app
```

Each keystroke recomputes the full Vietnamese word and applies the **minimal diff**
(delete N trailing characters, insert the new suffix) to already-committed text. See
[`docs/system-architecture.md`](docs/system-architecture.md) for the full design.

## Project Layout

| Path | What |
|------|------|
| `main.go` | Wayland app, key dispatch, Ctrl+Shift toggle logic. |
| `engine/telex.go` | Bamboo engine wrapper + diff computation. |
| `internal/keymap/` | evdev keycode → letter mapping, modifier constants. |
| `internal/inputmethod/`, `internal/virtualkeyboard/` | Generated Wayland protocol bindings. |
| `protocols/*.xml` | Source specs for the generated bindings. |
| `run.sh` | Build + run dev binary, swapping out the installed gokey instance. |

## Documentation

- [Project Overview & PDR](docs/project-overview-pdr.md) — what gokey is, requirements, key behaviors table.
- [System Architecture](docs/system-architecture.md) — components, protocols, event flow.
- [Codebase Summary](docs/codebase-summary.md) — layout, LOC, public API, `app` struct, tests.
- [Code Standards](docs/code-standards.md) — module boundaries and conventions used in this repo.
- [Project Roadmap](docs/project-roadmap.md) — done vs candidate future work.
- [Deployment Guide (Hyprland)](docs/deployment-guide.md) — install, autostart config, troubleshooting.

## Notes

- Wayland only (no X11). Telex only (no VNI/VIQR yet).
- The `internal/*` protocol bindings are generated by `go-wayland-scanner` and carry one
  manual patch (a `PutString` length fix in `input_method.go`) — re-apply it if regenerating.
  See [Code Standards](docs/code-standards.md).
