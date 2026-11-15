#!/bin/sh
set -e
if [ -d /run/systemd/system ]; then
  systemctl daemon-reload || true
fi
# "purge" removes the data too. A normal remove keeps /var/lib/kiestoon.
if [ "$1" = "purge" ]; then
  rm -rf /var/lib/kiestoon /var/lib/kiestoon-backups /etc/kiestoon
  deluser --system kiestoon >/dev/null 2>&1 || true
fi
exit 0
