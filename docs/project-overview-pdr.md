# gokey — Project Overview & PDR

## What It Is

`gokey` is a Vietnamese **Telex** input method for **Wayland** compositors, written in Go.
It lets users type Vietnamese (e.g. `tieesng vieejt` → `tiếng việt`) in any Wayland
application that supports the `text-input-v3` protocol, without needing a heavyweight
input-method framework (IBus/fcitx5).

- **Module:** `gokey` (Go 1.25+)
- **Binary:** `gokey` (single static-ish binary)
- **Input method:** Telex (via [BambooEngine/bamboo-core](https://github.com/BambooEngine/bamboo-core))
- **Platform:** Linux + Wayland only (uses `wl_seat`, `input-method-v2`, `virtual-keyboard-v1`)

## Problem

On Wayland, typing Vietnamese normally requires IBus or fcitx5 plus an engine
(e.g. ibus-bamboo). These are large, stateful, and can conflict. `gokey` is a
minimal, self-contained alternative: one Go binary that grabs the keyboard via the
standard `zwp_input_method_v2` protocol and commits transformed text directly.

## Goals

- Correct Telex composition for common Vietnamese words (tones, diacritics, `đ`).
- Work in real apps (terminals, browsers) using **no preedit** — text is committed
  and patched in place via `delete_surrounding_text` + `commit_string` diffs.
- Toggle Vietnamese on/off with a single chord: **Ctrl+Shift**.
- Stay tiny and dependency-light; pass through all non-Vietnamese keys unchanged.

## Non-Goals

- X11 support (Wayland only).
- Input methods other than Telex (VNI, VIQR, etc.) — not currently wired up.
- Candidate UI, configuration files, or a settings GUI. (Preedit is supported as an
  opt-in mode — see Key Behaviors — but there is no candidate window.)
- Coexistence with another active input method on the same seat (only one input
  method may hold the seat at a time).

## Key Behaviors

| Behavior | Detail |
|----------|--------|
| Toggle VN | Press and release **Ctrl+Shift** with no other key in the chord. |
| Toggle preedit | **Ctrl+Shift+Space** switches direct-commit ↔ preedit mode (direct default). |
| Passthrough | When VN off, or app inactive, or Ctrl/Alt/Super held → key forwarded as-is. |
| Composition | Letters fed to Bamboo engine; result applied as minimal byte diff. |
| Backspace | If composing a word, edits the word; otherwise forwarded to app. |
| Caps/Shift | Uppercases letters before feeding the engine (`shift XOR capsLock`). |
| No-IM safety | `run.sh` stops the installed gokey before a dev build, relaunches it on exit. |

## Requirements

**Runtime:**
- A Wayland compositor exposing `zwp_input_method_manager_v2` **and**
  `zwp_virtual_keyboard_manager_v1` (e.g. Sway, Hyprland, river, KWin with the protocol).
- No other input method holding the `input-method-v2` slot on the seat.

**Build:**
- Go 1.25+ (`go build -o gokey .`).

## Environment Variables

| Var | Effect |
|-----|--------|
| `GOKEY_DEBUG` | When non-empty, enables verbose `log.Printf` tracing of activate/key/diff events. |

## Status

Working prototype. Core Telex path, toggle, backspace, and passthrough implemented and
unit-tested (`engine/telex_test.go`). Generated Wayland bindings carry a manual patch
(see Code Standards) that must be re-applied if regenerated.
