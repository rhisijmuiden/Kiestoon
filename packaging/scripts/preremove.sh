#!/bin/sh
set -e
# On upgrade ($1 = upgrade) keep everything running; the new package restarts the service.
if [ "$1" = "remove" ] && [ -d /run/systemd/system ]; then
  systemctl disable --now kiestoon-updater.path kiestoon.service >/dev/null 2>&1 || true
fi
exit 0
