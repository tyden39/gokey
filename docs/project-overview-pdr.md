# gokey — Project Overview & PDR

## What it is

gokey is a Vietnamese Telex input method for Wayland, written in Go. It is a
single binary with no daemon framework (no IBus, no fcitx5). It speaks two
standard Wayland protocols directly:

- `zwp_input_method_v2` — grabs the keyboard, receives raw key/modifier
  events, and commits transformed text (`commit_string`) or shows in-progress
  text (`set_preedit_string`) to the focused application.
- `zwp_virtual_keyboard_v1` — forwards keys the input method doesn't need to
  transform (shortcuts, punctuation, navigation) and sends synthetic
  Backspace key events to erase already-committed characters so they can be
  replaced with the correct diacritics.

The actual Telex transform (letter sequence → Vietnamese word, tones,
diacritics, `đ`) is delegated to the `github.com/BambooEngine/bamboo-core`
library; gokey's own code is the Wayland glue plus a thin wrapper that turns
Bamboo's engine calls into a minimal (delete, insert) diff.

## Why it exists

Existing Vietnamese IMEs on Linux (`ibus-bamboo`, `fcitx5-bamboo`) require a
full input framework daemon and per-toolkit IM modules (`GTK_IM_MODULE`,
`QT_IM_MODULE`), which are X11-era mechanisms that don't reliably work with
native Wayland `text-input-v3` clients. gokey targets compositors that
already expose `zwp_input_method_v2` and `zwp_virtual_keyboard_v1` (Hyprland,
Sway, river) and needs nothing else installed.

## Goals (non-negotiable)

- Correct Telex composition (tones, diacritics, `đ`, uppercase) via
  bamboo-core, matching what `ibus-bamboo`/`fcitx5-bamboo` users expect.
- Work in both terminal emulators and GUI text fields without corrupting
  already-typed text.
- Never swallow keystrokes in apps that have no text-input focus (application
  launchers such as wofi/fuzzel, layer-shell overlays) — a launcher that eats
  keys is worse than no IME at all.
- Toggle Vietnamese on/off instantly with a keyboard chord, no restart.
- Stay small enough that one person can read the whole codebase in one
  sitting.

## Non-goals

- Not a general IME framework — no plugin system, no other Vietnamese input
  styles (VNI, VIQR) yet.
- Not cross-platform — Wayland only, no X11/XWayland support (XWayland apps
  do not get transformed text since they don't speak text-input-v3).
- Not a configuration system — behavior is currently compiled in, no config
  file.

## Key behaviors

| Behavior | Trigger | Result |
|---|---|---|
| Toggle Vietnamese on/off | Ctrl+Shift pressed and released together, no other key in between | `vnOn` flips; in-progress word ends; log line `Vietnamese=<bool>` |
| Toggle composition mode | Ctrl+Shift+Space | `preedit` flips between direct-commit (default) and preedit; in-progress word is flushed first |
| Compose a word (direct mode) | Any letter key while Vietnamese is on and a text-input client is focused | Bamboo processes the key; gokey deletes the trailing characters that changed and commits the new ones via `commit_string` |
| Compose a word (preedit mode) | Same, but `preedit == true` | The whole in-progress word is shown via `set_preedit_string`, replacing itself atomically each keystroke |
| End a word | Space, punctuation, Enter, or a Ctrl/Alt/Super chord | Direct mode: engine resets (text already committed). Preedit mode: pending preedit is committed as real text, then reset |
| Arm Backspace-restore | Word ended by exactly one Space (direct mode only, word was non-empty) | Raw keystrokes and on-screen form of the just-ended word are stashed |
| Backspace-restore fires | Backspace immediately after an arming Space, nothing else typed since | The space is deleted with one synthetic Backspace, the stashed word is replayed into a fresh engine, and composition continues (e.g. `vi` + Space + Backspace + `f` → `vì`) |
| Backspace-restore refused | Stashed raw keys don't replay to the exact on-screen word (e.g. after an in-word Backspace following a tone-cancel pair) | Restore is skipped, engine stays empty, Backspace behaves like a normal forwarded key |
| Pass through when inactive | Focused client has no text-input (drun launchers, overlays) or Vietnamese is off | Every key is forwarded raw via the virtual keyboard, nothing is consumed |
| Pass through modifiers/chords | Ctrl, Alt, Super held with another key | Key is forwarded; if a word was in progress it is ended first |

## Users / audience

Linux desktop users on a Wayland compositor with input-method-v2 support
(currently verified on Hyprland) who type Vietnamese and want a lightweight
alternative to ibus-bamboo/fcitx5-bamboo without pulling in a full input
framework.

## Success criteria

- `go test ./engine` passes (Telex transform correctness, including the
  Backspace-restore replay/refusal cases).
- Typing `tieesng vieejt` into a `text-input-v3` terminal (e.g. `foot`)
  produces `tiếng việt`.
- A launcher (wofi/fuzzel) receives every keystroke unmodified while
  unfocused by gokey's grabbed input method.
- Only one gokey process holds the input-method seat grab at a time; a
  second instance fails fast with a clear log message instead of silently
  fighting the first.

## Current status

All goals above are implemented and covered by the engine test suite (see
`docs/codebase-summary.md` for the exact test list). The project is in active
use as the author's daily-driver IME on a single Hyprland machine (see
`docs/deployment-guide.md`); it has not been packaged or tested on other
compositors.
