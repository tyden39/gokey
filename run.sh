#!/usr/bin/env bash
# Test runner for gokey.
# Stops fcitx5 while gokey runs, and restores it on exit so your session is
# never left without an input method.
set -euo pipefail
cd "$(dirname "$0")"

restore() {
	echo
	echo "[run.sh] stopping gokey, restarting fcitx5..."
	fcitx5 -d >/dev/null 2>&1 || true
}
trap restore EXIT

echo "[run.sh] building..."
go build -o gokey .

echo "[run.sh] stopping fcitx5 (only one input method can run at a time)..."
pkill -x fcitx5 2>/dev/null || true
sleep 0.5

echo "[run.sh] starting gokey. Toggle Vietnamese with Ctrl+Shift. Press Ctrl+C to quit."
echo "[run.sh] In ANOTHER window, open a text-input-v3 app, e.g.:"
echo "         env -u GTK_IM_MODULE -u QT_IM_MODULE -u XMODIFIERS foot"
echo "         then type:  tieesng vieejt  ->  tiếng việt"
exec env GOKEY_DEBUG=1 ./gokey
