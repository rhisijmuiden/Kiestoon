#!/bin/sh
set -e

if ! getent group kiestoon >/dev/null; then
  addgroup --system kiestoon
fi
if ! getent passwd kiestoon >/dev/null; then
  adduser --system --ingroup kiestoon --home /var/lib/kiestoon --no-create-home --shell /usr/sbin/nologin --gecos "Kiestoon" kiestoon
fi

install -d -o kiestoon -g kiestoon -m 0750 /var/lib/kiestoon
install -d -o root -g root -m 0700 /var/lib/kiestoon-backups
chown root:kiestoon /etc/kiestoon/kiestoon.env
chmod 0640 /etc/kiestoon/kiestoon.env

systemctl enable kiestoon.service kiestoon-updater.path >/dev/null 2>&1 || true

# Only talk to systemd when it is running (not while building the LXC image in a chroot).
if [ -d /run/systemd/system ]; then
  systemctl daemon-reload
  systemctl restart kiestoon.service
  systemctl start kiestoon-updater.path
fi
exit 0
