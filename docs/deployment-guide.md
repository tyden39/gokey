# gokey — Deployment Guide (Hyprland)

This is the deployment setup verified on the author's Hyprland machine. Other
compositors that expose `zwp_input_method_manager_v2` and
`zwp_virtual_keyboard_manager_v1` (Sway, river) should work the same way in
principle, but only the Hyprland setup below has actually been run.

## Prerequisites

- A Wayland compositor exposing both `zwp_input_method_manager_v2` and
  `zwp_virtual_keyboard_manager_v1` (Hyprland, Sway, river). gokey logs a
  fatal error and exits at startup if either is missing.
- Go 1.25+ to build.
- Wayland only — there is no X11 support, and Telex is the only input style
  (no VNI/VIQR).

## Install

Build the binary and copy it to a stable location outside the repo so
Hyprland's autostart doesn't depend on the working tree:

```bash
go build -o gokey .
cp ./gokey ~/.local/bin/gokey
```

## Hyprland configuration

In `~/.config/hypr/hyprland.conf`:

```
exec-once = ~/.local/bin/gokey

env = XMODIFIERS,@im=gokey
env = GTK_IM_MODULE,
env = QT_IM_MODULE,
```

- `exec-once` starts gokey once per Hyprland session.
- `GTK_IM_MODULE` and `QT_IM_MODULE` are set **empty** deliberately. These
  are X11-era environment variables that tell GTK/Qt apps to route input
  through a named IM module (historically `ibus` or `fcitx`); with no such
  module installed, forcing them empty makes GTK/Qt apps fall back to
  Wayland's native `text-input-v3` protocol instead of trying to load an
  IM module that no longer exists. `XMODIFIERS=@im=gokey` is set for
  parity/documentation of intent but the actual transformation path is
  `text-input-v3`, not the XIM protocol this variable traditionally
  configures.
- **Environment variable changes require a full logout/login** to take
  effect session-wide — reloading Hyprland config (`hyprctl reload`) is not
  enough, since `env =` lines are read once when the Hyprland session
  environment is built.

This setup replaces fcitx5 entirely (fully removed from the system). The
previous Hyprland config is backed up at
`~/.config/hypr/hyprland.conf.bak.*` on this machine, in case the old
fcitx5-based setup ever needs to be restored for comparison.

## Updating an installed build

```bash
go build -o gokey .
cp ./gokey ~/.local/bin/gokey
pkill -x gokey
hyprctl dispatch exec '~/.local/bin/gokey'
```

Always relaunch via `hyprctl dispatch exec`, not by running the binary
directly in a terminal — a process launched as a shell's child dies when
that terminal closes, while `hyprctl dispatch exec` hands the process to
Hyprland the same way `exec-once` does, so it survives independently of
whatever shell issued the command.

## Only one input method at a time

`zwp_input_method_v2` allows exactly one client to hold the keyboard grab
per seat. If a second gokey (or any other input method) tries to grab while
one is already running, the compositor sends the `unavailable` event and
gokey logs:

```
input method unavailable: another input method (e.g. fcitx5) is already active; stop it first
```

and exits via `log.Fatal`. This is why any dev/test workflow must stop the
installed instance first — `run.sh` does this automatically
(`pkill -x gokey` before building and running the dev binary, then
relaunches the installed instance on exit via a trap). See
`docs/codebase-summary.md` for what `run.sh` does step by step.

## Verifying the installed instance

Check exactly one gokey process is running:

```bash
pgrep -x gokey
```

Should print exactly one PID. If it prints none, gokey isn't running or
crashed at startup (check `journalctl` or run it manually with
`GOKEY_DEBUG=1 ~/.local/bin/gokey` in a terminal to see the log). If it
prints more than one, kill all of them (`pkill -x gokey`) and relaunch
once via `hyprctl dispatch exec '~/.local/bin/gokey'`.

## Functional test

Open a terminal that uses `text-input-v3` with the IM-module environment
variables explicitly unset (so nothing routes around gokey even if they're
set elsewhere in the session):

```bash
env -u GTK_IM_MODULE -u QT_IM_MODULE -u XMODIFIERS foot
```

Type:

```
tieesng vieejt
```

Expected result: `tiếng việt`. Press Ctrl+Shift to toggle Vietnamese off and
confirm the same keys now type literally (`tieesng vieejt`), then Ctrl+Shift
again to turn it back on.

## Testing dev builds without disturbing the installed instance

```bash
./run.sh
```

This builds, stops the installed gokey, runs the dev binary in the
foreground with `GOKEY_DEBUG=1` (verbose per-key logging), and on exit
(Ctrl+C or otherwise) relaunches the installed instance via
`hyprctl dispatch exec` so the session is never left without an IME running.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `input method unavailable: another input method ... is already active` | Another process (often a second gokey, or a leftover fcitx5) already holds the `zwp_input_method_v2` grab | `pkill -x gokey` (and stop any other IME), then relaunch: `hyprctl dispatch exec '~/.local/bin/gokey'` |
| `compositor missing input-method-v2 or virtual-keyboard-v1 support` | Running on a compositor that doesn't expose these protocols | Only Hyprland/Sway/river (and similar wlroots-based compositors) are supported; there is no fallback |
| Vietnamese doesn't transform in a specific app | The app isn't using `text-input-v3` — either an X11/XWayland app, or a toolkit forcing an IM module via `GTK_IM_MODULE`/`QT_IM_MODULE` | Confirm the app is native Wayland; check it isn't overriding the empty IM module env vars itself |
| Old behavior persists after a code change | `~/.local/bin/gokey` is stale — the running process is still the previously installed build | Rebuild, `cp` over `~/.local/bin/gokey`, then `pkill -x gokey && hyprctl dispatch exec '~/.local/bin/gokey'` |
| gokey exits as soon as a terminal window is closed | It was launched as a child of that terminal's shell instead of via Hyprland | Always launch via `exec-once` (session start) or `hyprctl dispatch exec` (manual relaunch), never by running the binary directly in an interactive shell you intend to close |
| Nothing types at all, even with Vietnamese off | `keymapSet` never became true (compositor's keymap event never arrived, or the virtual keyboard's `Keymap()` call failed) — `forward()` and the Backspace-restore path both silently no-op until `keymapSet` is true | Restart gokey; if it recurs, run with `GOKEY_DEBUG=1` and check for a `keymap set (format=...)` log line at startup |
