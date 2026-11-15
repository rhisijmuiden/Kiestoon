#!/bin/sh
set -e

if ! getent group kiestoon >/dev/null; then
  addgroup --system kiestoon
fi
if ! getent passwd kiestoon >/dev/null; then
  adduser --system --ingroup kiestoon --home /var/lib/kiestoon --no-create-home --shell /usr/sbin/nologin --gecos "Kiestoon" kiestoon
fi

install -d -o root -g root -m 0700 /var/lib/kiestoon-backups
/usr/lib/kiestoon/fix-permissions
chown root:kiestoon /etc/kiestoon/kiestoon.env
chmod 0640 /etc/kiestoon/kiestoon.env

systemctl enable kiestoon-helper.service kiestoon.service kiestoon-updater.path asterisk.service fail2ban.service >/dev/null 2>&1 || true

# Only talk to systemd when it is running (not while building the LXC image in a chroot).
if [ -d /run/systemd/system ]; then
  systemctl daemon-reload
  systemctl restart kiestoon-helper.service || true
  systemctl restart kiestoon.service
  # Asterisk reads the configuration the portal generates. Reloading keeps calls up during upgrades.
  sleep 2
  if systemctl is-active --quiet asterisk.service; then
    systemctl reload asterisk.service || true
  else
    systemctl start asterisk.service || true
  fi
  systemctl start kiestoon-updater.path
  systemctl start fail2ban.service >/dev/null 2>&1 || true
fi
exit 0
