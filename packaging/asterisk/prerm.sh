#!/bin/sh
set -e
if [ "$1" = "remove" ] && [ -d /run/systemd/system ]; then
  systemctl stop asterisk.service || true
  systemctl disable asterisk.service || true
fi
exit 0
