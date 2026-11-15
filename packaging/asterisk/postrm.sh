#!/bin/sh
set -e
ldconfig || true
if [ -d /run/systemd/system ]; then
  systemctl daemon-reload || true
fi
exit 0
