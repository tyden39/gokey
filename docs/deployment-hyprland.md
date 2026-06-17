# gokey — Deployment Guide (Wayland / Hyprland)

How gokey is installed and run as the Vietnamese input method on Hyprland.

## Current Deployment (this machine)

- **Binary:** `~/.local/bin/gokey`
- **Compositor:** Hyprland 0.55.2 (Wayland) — exposes `input-method-v2` + `virtual-keyboard-v1`.
- **Autostart:** `exec-once = ~/.local/bin/gokey` in `~/.config/hypr/hyprland.conf`.
- **Replaces:** the previous `exec-once = fcitx5 -d` (kept in
  `~/.config/hypr/hyprland.conf.bak.*`). Only one input method may hold a seat at a time.
- **Toggle Vietnamese:** Ctrl+Shift.

## Install / Update

```bash
cd /home/ducnguyen/ws/gokey
go build -o gokey .              # compile latest source
cp ./gokey ~/.local/bin/gokey   # install to user bin
```

Reload the running instance (kill old, relaunch under Hyprland so it survives the shell):

```bash
pkill -x gokey
hyprctl dispatch exec '~/.local/bin/gokey'
```

> Keep `~/.local/bin/gokey` in sync with the build after editing `main.go` or `engine/`.
> The deployed binary is what runs — a stale copy means old behavior.

## Hyprland Configuration

In `~/.config/hypr/hyprland.conf` (already applied here):

```ini
# Vietnamese IME (gokey - Wayland native, text-input-v3)
env = XMODIFIERS, @im=gokey
# Leave GTK/QT IM modules empty so apps use Wayland text-input-v3, not an IM module
env = GTK_IM_MODULE,
env = QT_IM_MODULE,
exec-once = ~/.local/bin/gokey
```

Why empty `GTK_IM_MODULE` / `QT_IM_MODULE`: gokey works through the compositor's
`text-input-v3` protocol, not via toolkit IM modules. Setting these to `fcitx`/`ibus`
would route apps to a different (absent) IM and Vietnamese typing would not work.

`env` changes only take effect for processes started **after** Hyprland reloads them — log
out/in (or restart Hyprland) for the env vars to apply to your whole session. `exec-once`
runs on Hyprland start; use `hyprctl dispatch exec` to (re)launch without a restart.

## Testing a dev build

gokey is autostarted at boot via `exec-once = ~/.local/bin/gokey` in
`~/.config/hypr/hyprland.conf`. That instance holds the seat, and **only one input
method (`zwp_input_method_v2` grab) may be active at a time**. So before running a
freshly built `./gokey` (or `./run.sh`), you MUST stop the autostarted one — otherwise
the new process hits `SetUnavailableHandler` → `log.Fatal("input method unavailable")`
and exits immediately.

```bash
pkill -x gokey      # stop the autostarted (installed) instance
./run.sh            # build + run dev binary in foreground (GOKEY_DEBUG=1)
```

Notes:
- `run.sh` only stops/restarts **fcitx5**, not gokey — killing gokey is on you.
- On exit, `run.sh`'s trap restarts fcitx5, not gokey. To return to the boot state:
  `hyprctl dispatch exec '~/.local/bin/gokey'` (or just relog/reboot).
- To make a tested fix permanent, install it: see [Install / Update](#install--update).

## Verify

```bash
pgrep -x gokey                  # should print exactly one PID
pgrep -x fcitx5 || echo "no fcitx5"   # must NOT be running (would steal the seat)
```

Functional test — open a Wayland app using text-input-v3 and type `tieesng vieejt`:

```bash
env -u GTK_IM_MODULE -u QT_IM_MODULE -u XMODIFIERS foot
# type:  tieesng vieejt   ->   tiếng việt
# press Ctrl+Shift to toggle Vietnamese off/on
```

Enable tracing if needed: `GOKEY_DEBUG=1 ~/.local/bin/gokey` (run from a terminal,
after stopping the autostarted instance).

## Troubleshooting

| Symptom | Cause / Fix |
|---------|-------------|
| Log: `input method unavailable: another input method ... already active` | Another IM (fcitx5/ibus, or a second gokey) holds the seat. `pkill -x fcitx5; pkill -x ibus-daemon`, ensure only one gokey runs, relaunch. |
| Log: `compositor missing input-method-v2 or virtual-keyboard-v1 support` | Compositor doesn't expose the protocols. Hyprland/Sway/river do; some others don't. |
| Vietnamese not transformed in an app | App isn't using text-input-v3 (e.g. an XWayland/X11 app, or one forced to a toolkit IM module). Native Wayland GTK/Qt apps with empty IM modules work. |
| Old behavior after a code change | Deployed `~/.local/bin/gokey` is stale — rebuild and `cp`, then reload. |
| gokey dies when terminal closes | It was a child of the shell. Launch via `hyprctl dispatch exec` or `exec-once` so Hyprland owns it. |

## Notes

- `run.sh` is for **development** (it stops/restarts fcitx5 around a foreground run with
  debug). For the persistent session IME, the `exec-once` autostart above is what's used.
- Wayland only; Telex only. See [system-architecture.md](system-architecture.md) for design.
