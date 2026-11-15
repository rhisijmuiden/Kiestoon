#!/usr/bin/env bash
# Starts a throwaway Kiestoon with the fake Asterisk manager and the demo office, for documentation screenshots.
# Usage: tools/docs/demo.sh <kiestoon-binary> <work-dir> [port]
# Leaves the processes running; their PIDs are written to <work-dir>/pids.
set -euo pipefail
BIN=$(realpath "$1"); WORK=$(realpath -m "$2"); PORT=${3:-8090}
HERE=$(cd "$(dirname "$0")" && pwd)
[ -f "$WORK/pids" ] && kill $(cat "$WORK/pids") 2>/dev/null || true
rm -rf "$WORK" && mkdir -p "$WORK/data"
AMI_PORT=$((PORT + 1000))
node "$HERE/fake-ami.mjs" "$AMI_PORT" > "$WORK/ami.log" 2>&1 &
echo $! > "$WORK/pids"
node "$HERE/fake-helper.mjs" "$WORK/helper.sock" "$WORK/wg-keys.json" > "$WORK/helper.log" 2>&1 &
echo $! >> "$WORK/pids"
start() {
KIESTOON_DATA_DIR="$WORK/data" KIESTOON_LISTEN="127.0.0.1:$PORT" KIESTOON_TLS_LISTEN=off KIESTOON_CHANNEL=testing KIESTOON_AI_BASE_URL=http://127.0.0.1:9 \
  KIESTOON_AMI_ADDR="127.0.0.1:$AMI_PORT" KIESTOON_HELPER_SOCKET="$WORK/helper.sock" TZ=Europe/Amsterdam \
  "$BIN" >> "$WORK/server.log" 2>&1 &
echo $! > "$WORK/server.pid"
for _ in $(seq 50); do curl -fs "http://127.0.0.1:$PORT/api/setup/status" >/dev/null 2>&1 && break; sleep 0.2; done
}
start
node "$HERE/seed.mjs" "http://127.0.0.1:$PORT" "$WORK/data"
node --no-warnings "$HERE/dress.mjs" "http://127.0.0.1:$PORT" "$WORK/data" "$WORK/wg-keys.json"
# Restart once so the "restart the phone engine" notice from the seeded settings is gone.
kill "$(cat "$WORK/server.pid")"; sleep 1
start
cat "$WORK/server.pid" >> "$WORK/pids"
