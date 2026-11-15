#!/bin/bash
# Installs the Kiestoon and Asterisk packages in a Debian 13 container with systemd and checks
# that the portal, the helper and Asterisk come up and talk to each other.
#   packaging/ci/install-test.sh dist/kiestoon_*.deb dist/kiestoon-asterisk_*.deb
set -euo pipefail
KT_DEB="$1"
AST_DEB="$2"
SND_DEB="${3:-}"
SPEECH_DEB="${4:-}"
NAME=kt-install-test

# DOCKER_RUN_ARGS: extra options for docker run, for example --network host behind a proxy.
docker image inspect kt-systemd >/dev/null 2>&1 ||
  docker build -q -t kt-systemd -f "$(dirname "$0")/systemd.Dockerfile" "$(dirname "$0")" >/dev/null
docker rm -f "$NAME" >/dev/null 2>&1 || true
# shellcheck disable=SC2086
docker run -d --name "$NAME" ${DOCKER_RUN_ARGS:-} --privileged --cgroupns=host -v /sys/fs/cgroup:/sys/fs/cgroup:rw \
  --tmpfs /run --tmpfs /run/lock kt-systemd >/dev/null
[ -n "${KEEP:-}" ] || trap 'docker rm -f "$NAME" >/dev/null' EXIT
ct() { docker exec "$NAME" bash -c "$*"; }

for _ in $(seq 1 30); do ct 'systemctl is-system-running 2>/dev/null | grep -qE "running|degraded"' && break; sleep 1; done
docker cp "$KT_DEB" "$NAME:/root/kiestoon.deb"
docker cp "$AST_DEB" "$NAME:/root/kiestoon-asterisk.deb"
EXTRA=""
if [ -n "$SND_DEB" ]; then
  docker cp "$SND_DEB" "$NAME:/root/kiestoon-sounds-nl.deb"
  EXTRA=/root/kiestoon-sounds-nl.deb
fi
if [ -n "$SPEECH_DEB" ]; then
  docker cp "$SPEECH_DEB" "$NAME:/root/kiestoon-speech.deb"
  EXTRA="$EXTRA /root/kiestoon-speech.deb"
fi
ct "apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq /root/kiestoon-asterisk.deb /root/kiestoon.deb $EXTRA >/dev/null"

fail() {
  echo "FAIL: $*"
  ct 'systemctl --no-pager status kiestoon kiestoon-helper asterisk || true; journalctl --no-pager -n 80 -u kiestoon -u kiestoon-helper -u asterisk || true'
  exit 1
}
for _ in $(seq 1 30); do ct 'curl -fsS http://127.0.0.1:8080/api/health' >/dev/null 2>&1 && break; sleep 1; done
ct 'curl -fsS http://127.0.0.1:8080/api/health' || fail "portal health"
ct 'curl -fsS http://127.0.0.1:8080/api/setup/status | grep -q "\"setupRequired\":true"' || fail "setup status"
for _ in $(seq 1 15); do ct 'curl -fsSk https://127.0.0.1:8443/ | grep -q "<div id=\"root\">"' && break; sleep 1; done
ct 'curl -fsSk https://127.0.0.1:8443/ | grep -q "<div id=\"root\">"' || fail "portal over https"
ct 'kiestoon setup-token' >/dev/null || fail "setup token"
for unit in kiestoon kiestoon-helper asterisk fail2ban; do
  ct "systemctl is-active -q $unit" || fail "$unit not active"
done
ct 'systemctl is-enabled -q kiestoon-updater.path' || fail "updater path"
ct 'test -S /run/kiestoon-helper/helper.sock' || fail "helper socket"
if [ -n "$SND_DEB" ]; then
  ct 'test -s /var/lib/asterisk/sounds/nl/vm-intro.wav && test -s /var/lib/asterisk/sounds/nl/digits/7.alaw' || fail "Dutch prompts"
fi

# The portal writes the Asterisk configuration and connects over AMI.
for _ in $(seq 1 30); do ct 'asterisk -rx "manager show connected" | grep -q kiestoon' && break; sleep 1; done
ct 'asterisk -rx "manager show connected" | grep -q kiestoon' || fail "AMI not connected"
# grep without -q: with pipefail, grep -q exiting early can kill the writer (SIGPIPE) and fail a passing check.
ct 'asterisk -rx "pjsip show transports"' | tee /dev/stderr | grep 'transport-udp' >/dev/null || fail "pjsip transports"
ct 'asterisk -rx "core show version"'
# The portal may reload Asterisk right after it connects: give the fax modules time to come up.
for _ in $(seq 1 30); do ct 'asterisk -rx "module show like res_fax_spandsp" | grep -q res_fax_spandsp' && break; sleep 1; done
ct 'asterisk -rx "module show like res_fax_spandsp" | grep -q res_fax_spandsp' || fail "fax module (SpanDSP) not loaded"
ct 'test -f /var/lib/kiestoon/pbx/pjsip.conf && stat -c "%U:%G %a %n" /var/lib/kiestoon /var/lib/kiestoon/pbx /var/lib/kiestoon/pbx/pjsip.conf'
ct 'asterisk -rx "dialplan show 112@kiestoon-emergency" | grep -q 112' || fail "emergency dialplan"
ct 'fail2ban-client status | grep -q kiestoon-sip' || fail "fail2ban jails"
ct 'command -v tiff2pdf >/dev/null' || fail "tiff2pdf missing"
ct 'command -v gs >/dev/null' || fail "ghostscript missing (fax sending)"
if [ -n "$SPEECH_DEB" ]; then
  ct '/opt/kiestoon-speech/bin/whisper-cli --help >/dev/null 2>&1' || fail "whisper-cli does not start"
  ct 'echo Kiestoon | /opt/kiestoon-speech/bin/piper --help >/dev/null' || fail "piper does not start"
  ct '/opt/kiestoon-speech/llama/bin/llama-server --help >/dev/null 2>&1' || fail "llama-server does not start"
fi
ct '! systemctl is-active --quiet caddy.service' || fail "caddy runs while internet mode is off"
echo "install test passed"
