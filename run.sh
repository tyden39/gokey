#!/usr/bin/env bash
# Test runner for gokey.
# Only one input method can hold the seat at a time. The installed instance
# (~/.local/bin/gokey, autostarted by Hyprland) is stopped while the dev binary
# runs, and relaunched on exit so your session is never left without an IME.
set -euo pipefail
cd "$(dirname "$0")"

restore() {
	echo
	echo "[run.sh] stopping dev gokey, relaunching installed instance..."
	hyprctl dispatch exec '~/.local/bin/gokey' >/dev/null 2>&1 || true
}
trap restore EXIT

echo "[run.sh] building..."
go build -o gokey .

echo "[run.sh] stopping installed gokey (only one input method can run at a time)..."
pkill -x gokey 2>/dev/null || true
sleep 0.5

echo "[run.sh] starting dev gokey. Toggle Vietnamese with Ctrl+Shift,"
echo "[run.sh] toggle preedit mode with Ctrl+Shift+Space. Press Ctrl+C to quit."
echo "[run.sh] In ANOTHER window, open a text-input-v3 app, e.g.:"
echo "         env -u GTK_IM_MODULE -u QT_IM_MODULE -u XMODIFIERS foot"
echo "         then type:  tieesng vieejt  ->  tiếng việt"
exec env GOKEY_DEBUG=1 ./gokey
