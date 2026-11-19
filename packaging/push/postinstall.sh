#!/bin/sh
set -e
if ! getent passwd kiestoon-push >/dev/null; then
  adduser --system --group --no-create-home --home /var/lib/kiestoon-push kiestoon-push >/dev/null
fi
chgrp kiestoon-push /etc/kiestoon-push /etc/kiestoon-push/kiestoon-push.env
chmod 0750 /etc/kiestoon-push
if [ -d /run/systemd/system ]; then
  systemctl daemon-reload
  systemctl enable kiestoon-push.service >/dev/null 2>&1 || true
  systemctl restart kiestoon-push.service || true
fi
exit 0
