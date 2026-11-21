#!/bin/bash
# Pairs a Kiestoon Edge with a running Kiestoon and checks the tunnel, the heartbeat and that SIP
# from the office reaches the central Asterisk through the edge.
#   packaging/ci/edge-test.sh <hub container> dist/kiestoon-edge_*_amd64.deb
# The hub container must have Kiestoon installed and the setup not done yet.
set -euo pipefail
HUB="$1"
EDGE_DEB="$2"
NAME=kt-edge-test

docker rm -f "$NAME" >/dev/null 2>&1 || true
# shellcheck disable=SC2086
docker run -d --name "$NAME" ${EDGE_DOCKER_RUN_ARGS:-} --privileged --cgroupns=host -v /sys/fs/cgroup:/sys/fs/cgroup:rw \
  --tmpfs /run --tmpfs /run/lock kt-systemd >/dev/null
[ -n "${KEEP:-}" ] || trap 'docker rm -f "$NAME" >/dev/null' EXIT
hub() { docker exec "$HUB" bash -c "$*"; }
edge() { docker exec "$NAME" bash -c "$*"; }
fail() {
  echo "FAIL: $*"
  edge 'systemctl --no-pager status kiestoon-edge kamailio rtpengine-daemon wg-quick@kt-edge || true; journalctl --no-pager -n 60 -u kiestoon-edge -u kamailio -u wg-quick@kt-edge || true; kiestoon-edge status || true; wg show || true'
  hub 'journalctl --no-pager -n 60 -u kiestoon -u kiestoon-helper || true; wg show || true; nft list ruleset | head -80 || true'
  exit 1
}

for _ in $(seq 1 30); do edge 'systemctl is-system-running 2>/dev/null | grep -qE "running|degraded"' && break; sleep 1; done
docker cp "$EDGE_DEB" "$NAME:/root/kiestoon-edge.deb"
edge 'apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq /root/kiestoon-edge.deb sipsak netcat-openbsd >/dev/null'
for _ in $(seq 1 20); do edge 'curl -fsS http://127.0.0.1/api/status' >/dev/null 2>&1 && break; sleep 1; done
edge 'curl -fsS http://127.0.0.1/api/status | grep -q "\"paired\":false"' || fail "edge status"
edge 'curl -fsS http://127.0.0.1/ | grep -q "Kiestoon Edge"' || fail "edge page"

# Without the WireGuard kernel module (some CI kernels), wg-quick falls back to wireguard-go.
for c in "$HUB" "$NAME"; do
  docker exec "$c" bash -c 'ip link add kt-probe type wireguard 2>/dev/null && ip link del kt-probe ||
    { apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq wireguard-go >/dev/null; }'
done

HUB_IP="$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$HUB")"
for _ in $(seq 1 30); do hub 'test -s /var/lib/kiestoon/initial-admin' && break; sleep 1; done
INITIAL="$(hub 'kiestoon initial-admin' | sed -n 's/^Password: //p')"
J='-H Content-Type:application/json -b /root/c -c /root/c'
hub "curl -fsS $J -d '{\"username\":\"admin\",\"password\":\"$INITIAL\"}' http://127.0.0.1:8080/api/auth/login" >/dev/null || fail "first sign-in"
hub "curl -fsS $J -d '{\"organization\":\"Test\",\"displayName\":\"Admin\",\"password\":\"Edge-test-pass-123\",\"language\":\"en\"}' http://127.0.0.1:8080/api/setup/first-run" >/dev/null || fail "first run"
# Administrators must set up a second factor before they can change anything.
hub 'command -v oathtool >/dev/null || (apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq oathtool >/dev/null)'
SECRET="$(hub "curl -fsS $J -X POST -d '{}' http://127.0.0.1:8080/api/account/mfa/enroll" | sed -n 's/.*"secret":"\([^"]*\)".*/\1/p')"
[ -n "$SECRET" ] || fail "mfa enroll"
OTP="$(hub "oathtool --totp -b $SECRET")"
hub "curl -fsS $J -d '{\"code\":\"$OTP\"}' http://127.0.0.1:8080/api/account/mfa/confirm" >/dev/null || fail "mfa confirm"
hub "curl -fsS $J -X PUT -d '{\"enabled\":true,\"host\":\"$HUB_IP\",\"listenPort\":51820}' http://127.0.0.1:8080/api/sites/hub" >/dev/null || fail "hub on"
CODE="$(hub "curl -fsS $J -d '{\"name\":\"Branch\",\"url\":\"https://$HUB_IP:8443\"}' http://127.0.0.1:8080/api/sites/edges" | sed -n 's/.*"code":"\([^"]*\)".*/\1/p')"
[ -n "$CODE" ] || fail "no pairing code"
for _ in $(seq 1 20); do hub 'wg show kt0 >/dev/null 2>&1' && break; sleep 1; done
hub 'wg show kt0' >/dev/null || fail "hub tunnel interface"

# Pair the way a headless Raspberry Pi does: the code in a file on the boot partition.
edge "mkdir -p /boot/firmware && echo '$CODE' > /boot/firmware/kiestoon-edge.txt && systemctl restart kiestoon-edge"
for _ in $(seq 1 40); do edge 'curl -fsS http://127.0.0.1/api/status | grep -q "\"connected\":true"' && break; sleep 2; done
edge 'curl -fsS http://127.0.0.1/api/status' | tee /dev/stderr | grep '"connected":true' >/dev/null || fail "edge not connected"
edge 'test ! -e /boot/firmware/kiestoon-edge.txt' || fail "pairing file not removed"
edge 'wg show kt-edge latest-handshakes' | grep -vq $'\t0$' || fail "no handshake"
for unit in kiestoon-edge kamailio rtpengine-daemon wg-quick@kt-edge; do
  edge "systemctl is-active -q $unit" || fail "$unit not active"
done
hub "curl -fsS $J http://127.0.0.1:8080/api/sites" | grep '"connected":true' >/dev/null || fail "hub does not see the edge"

# SIP from the office: OPTIONS to the edge is relayed through the tunnel to Asterisk, which answers.
LAN="$(edge "curl -fsS http://127.0.0.1/api/status" | sed -n 's/.*"lanAddress":"\([^"]*\)".*/\1/p')"
[ -n "$LAN" ] || fail "no office address"
edge "sipsak -v -s sip:ping@$LAN:5060 2>&1 || true" | tee /dev/stderr | grep 'realm="asterisk"' >/dev/null || fail "SIP through the edge"

# A phone in the office registers through the edge (Path), and Asterisk reaches it back through it.
hub "curl -fsS $J -d '{\"number\":\"201\",\"name\":\"Branch phone\",\"transport\":\"udp\",\"maxContacts\":2,\"sipSecret\":\"Edge-phone-201-pass\",\"ringSeconds\":25,\"enabled\":true}' http://127.0.0.1:8080/api/pbx/extensions" >/dev/null || fail "extension"
for _ in $(seq 1 20); do hub 'asterisk -rx "pjsip show endpoint 201"' 2>/dev/null | grep -qE 'Endpoint: +201' && break; sleep 1; done
edge "sipsak -U -C sip:201@$LAN:5099 -s sip:201@$LAN -a Edge-phone-201-pass -u 201 -x 300" || fail "register through the edge"
hub 'asterisk -rx "database show registrar"' | grep -q '"path":"<sip:10\.' || fail "registration without Path"
PORT="$(hub 'asterisk -rx "database show registrar"' | grep -o "alias=$LAN~[0-9]*" | tail -1 | cut -d~ -f2)"
[ -n "$PORT" ] || fail "no alias in the contact"
edge "(timeout 10 nc -ul $PORT > /tmp/options.txt &)"
sleep 1
hub 'asterisk -rx "pjsip qualify 201"' >/dev/null
for _ in $(seq 1 8); do edge 'grep -q "^OPTIONS sip:201" /tmp/options.txt' && break; sleep 1; done
edge 'cat /tmp/options.txt; grep -q "^OPTIONS sip:201" /tmp/options.txt' || fail "Asterisk does not reach the phone through the edge"
echo "edge test passed"
