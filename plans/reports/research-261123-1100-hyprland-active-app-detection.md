# Active application detection for GoKey

Research conducted: 2026-11-23 11:00 Asia/Ho_Chi_Minh

## Outcome

For this Omarchy system, identify the application when
`zwp_input_method_v2.activate` fires, query Hyprland's `activewindow` once,
and cache the result until `deactivate`. Do not query Hyprland on every key and
do not add a permanent socket2 listener unless testing later proves that
Hyprland misses an input-method activation transition.

## Verified system evidence

- The running desktop is Omarchy 4.5.0-3 with Hyprland 0.56.2 at commit
  `efb50993780079460b0cbed1363e2166a2de1d9f`.
- Both Hyprland IPC sockets exist under
  `$XDG_RUNTIME_DIR/hypr/$HYPRLAND_INSTANCE_SIGNATURE/`.
- A direct `j/activewindow` request to `.socket.sock` returned the focused
  window's `address`, `class`, `initialClass`, `pid`, `xwayland`, and workspace.
- GoKey already receives `activate` and `deactivate`, but those Wayland events
  contain no application identity. They only establish whether a focused text
  input currently needs the input method.

Hyprland documents `activewindow` as the current focused window query and
documents that IPC request connections are synchronous and must be closed
promptly. It also exposes `activewindowv2` on socket2, but a listener is not
needed for the first implementation because input-method activation already
defines the lifetime GoKey cares about:

- [Hyprland IPC](https://wiki.hypr.land/ipc/)
- [Using hyprctl](https://wiki.hypr.land/configuring/core/advanced-configuration/using-hyprctl/)

## Chosen design

1. Add a small Hyprland focus query component using Go's standard-library Unix
   socket support. Open `.socket.sock`, send `j/activewindow`, read one JSON
   response with a short deadline, and always close the connection.
2. On input-method `activate`, query synchronously before processing later key
   events and cache only:
   - `address`
   - `class`
   - `initialClass`
   - `pid`
   - `xwayland`
3. Use `initialClass` as the stable per-application policy key, with `class` as
   display/debug information. Do not retain the window title because it is not
   needed and may contain private document or page names.
4. While `a.active` is true, key handling may read this cached identity without
   additional IPC. On `deactivate`, clear it.
5. If the environment variables, socket, query, timeout, or JSON parsing fail,
   record the identity as unknown and continue normal Telex handling. App
   detection must never prevent typing.

## Rejected alternatives

- **Input-method-v2 alone:** it exposes active/inactive state, not an app name,
  PID, surface, or window ID.
- **Run `hyprctl` for every key:** Hyprland handles queries synchronously and
  warns against spamming them; process creation also puts unnecessary work on
  the typing path.
- **Permanent socket2 listener:** it adds a goroutine, reconnection logic, and
  event/query correlation that are unnecessary when app identity is refreshed
  at each input-method activation.
- **Hyprland Lua hook:** it would spread GoKey behavior into desktop config and
  is harder to deploy and test than a self-contained query in the binary.

## Accuracy boundary

This identifies the focused Hyprland toplevel associated with the active text
input. It distinguishes apps such as `kitty`, `chromium`, and
`com.microsoft.VSCode`, but not browser tabs or individual text fields. Layer
surfaces that do not map to a normal Hyprland toplevel may remain unknown.
Window class metadata is supplied by the client and is suitable for local
per-app behavior, not as a security boundary.

## Next step

Implement the query component and expose the cached identity to `app.onKey`.
Add unit tests for JSON parsing, timeout/error fallback, cache clearing, and
stable `initialClass` selection. Manually verify focus transitions between
kitty, Chromium, and VS Code on this Hyprland session.

## Unresolved questions

None for choosing the detection mechanism. The behavior that should vary by
application remains a separate product decision.
