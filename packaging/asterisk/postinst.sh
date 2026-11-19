#!/bin/sh
set -e
if ! getent group asterisk >/dev/null; then
  addgroup --system asterisk
fi
if ! getent passwd asterisk >/dev/null; then
  adduser --system --ingroup asterisk --home /var/lib/asterisk --no-create-home --shell /usr/sbin/nologin --gecos "Asterisk PBX" asterisk
fi
for d in /var/lib/asterisk /var/spool/asterisk /var/log/asterisk; do
  install -d -o asterisk -g asterisk -m 0750 "$d"
done
chown -R asterisk:asterisk /var/lib/asterisk /var/spool/asterisk /var/log/asterisk
ldconfig
systemctl enable asterisk.service >/dev/null 2>&1 || true
if [ -d /run/systemd/system ]; then
  systemctl daemon-reload
fi
exit 0
