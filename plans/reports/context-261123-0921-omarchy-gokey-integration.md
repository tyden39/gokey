# GoKey on Omarchy — Planning Context

Status: investigation complete; no code or system configuration has been changed.

## Desired outcome

Run GoKey as the Vietnamese Telex input method on the current Omarchy desktop.
The Omarchy header does not need to display or switch the input method. GoKey's
existing `Ctrl+Shift` toggle remains the user-facing Vietnamese on/off control.

## Verified current state

- The machine runs Omarchy 4.5 on Wayland with Hyprland 0.56.2, matching the
  compositor family expected by GoKey's Wayland bindings in `main.go`.
- `omarchy-fcitx5.service` is enabled and active. Fcitx5 currently owns the
  single input-method slot, so it cannot run concurrently with GoKey.
- Omarchy supplies Fcitx environment variables from
  `/usr/lib/environment.d/10-omarchy-fcitx.conf`.
- Go is not installed. The repository requires Go 1.25 or newer; the current
  Arch package repository offers a compatible version.
- The existing deployment guide targets an older `hyprland.conf`-based setup.
  This Omarchy installation uses Lua configuration and systemd user services.
- The header widget is Fcitx-aware. With Fcitx unavailable and only the `us`
  keyboard layout configured, it should hide automatically; no shell or widget
  customization is required for this outcome.

## Recommended scope for the future plan

### Repository changes

1. Add a systemd user unit for `~/.local/bin/gokey`, tied to
   `graphical-session.target` and conditioned on `WAYLAND_DISPLAY`.
2. Update `run.sh` to stop and restore `gokey.service` during development
   instead of managing the installed process through `pkill` and
   `hyprctl dispatch exec`.
3. Update `docs/deployment-guide.md` for current Omarchy:
   - install Go through the Omarchy package route;
   - build and install the binary under `~/.local/bin`;
   - use the systemd user service for lifecycle management;
   - mask Fcitx rather than requiring package removal;
   - override the Fcitx environment and require a full logout/login;
   - document the native-Wayland-only limitation and rollback route.
4. Do not change the Telex engine or Wayland event handling unless runtime
   verification exposes a separate defect.

### Machine configuration

1. Install Go, then run `go test ./...` and build GoKey.
2. Mask and stop `omarchy-fcitx5.service` so D-Bus activation cannot reclaim
   the input-method slot. Keep the Fcitx packages installed for easy rollback.
3. Create `~/.config/environment.d/99-gokey.conf` with empty values for
   `INPUT_METHOD`, `GTK_IM_MODULE`, `QT_IM_MODULE`, `XMODIFIERS`, and
   `SDL_IM_MODULE`. GoKey does not implement XIM, so do not advertise
   `XMODIFIERS=@im=gokey`.
4. Install and enable the GoKey systemd user service.
5. Log out and back in before performing the final application-level test.

## Acceptance criteria

- `go test ./...` and the production build succeed.
- After a fresh graphical login, exactly one GoKey process is running under the
  user service and Fcitx5 is not running.
- In a native Wayland `text-input-v3` application, typing
  `tieesng vieejt` produces `tiếng việt`.
- Pressing `Ctrl+Shift` disables Vietnamese transformation, and pressing it
  again restores transformation.
- Normal shortcuts and typing in non-text-input surfaces still pass through.
- The service survives logout/login and can be restarted without spawning a
  duplicate input method.
- The header requires no custom plugin or input-method control.

## Constraints and known limitation

- Wayland permits only one input method per seat; Fcitx and GoKey are mutually
  exclusive while running.
- GoKey supports native Wayland clients using `text-input-v3`. XWayland clients
  do not gain Vietnamese input through this integration.
- Do not edit files under `/usr/share/omarchy/`; use user-owned configuration
  under `~/.config/`.

## Rollback route

Stop and disable the GoKey user service, remove the GoKey environment override,
unmask and re-enable `omarchy-fcitx5.service`, then log out and back in. Keeping
the installed Fcitx packages makes rollback configuration-only.

## Evidence pointers

- `README.md` — runtime requirements and supported application path.
- `main.go` — Wayland globals, input-method ownership, and toggle behavior.
- `run.sh` — current development lifecycle that needs service awareness.
- `docs/deployment-guide.md` — deployment guidance to replace for Omarchy.
- `/usr/lib/systemd/user/omarchy-fcitx5.service` — current Omarchy lifecycle
  pattern and the competing input-method owner.
- `/usr/lib/environment.d/10-omarchy-fcitx.conf` — packaged Fcitx environment.
- `/usr/share/omarchy/shell/plugins/bar/widgets/KeyboardLayout.qml` — why the
  header needs no separate change when Fcitx and extra layouts are absent.
