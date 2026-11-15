#!/bin/bash
# Kiestoon installer for Debian 13 (Trixie).
#
#   curl -fsSL https://raw.githubusercontent.com/rhisijmuiden/kiestoon/main/install.sh | sudo bash
#   curl -fsSL https://raw.githubusercontent.com/rhisijmuiden/kiestoon/main/install.sh | sudo bash -s -- --channel testing
#
# Options:
#   --channel production|testing   Update channel to follow (default: production)
#   --version vX.Y.Z               Install this exact release instead of the newest one
#   --deb /path/to/kiestoon.deb    Install a local package (for development)
set -euo pipefail

REPO="${KIESTOON_REPO:-rhisijmuiden/kiestoon}"
CHANNEL="production"
VERSION=""
LOCAL_DEB=""

cyan() { printf '\033[1;36m%s\033[0m\n' "$*"; }
die() { printf '\033[1;31mError:\033[0m %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --channel) CHANNEL="${2:-}"; shift 2 ;;
    --version) VERSION="${2:-}"; shift 2 ;;
    --deb) LOCAL_DEB="${2:-}"; shift 2 ;;
    -h|--help) sed -n '2,12p' "$0"; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done

[ "$(id -u)" = "0" ] || die "run this installer as root (sudo)."
[ "$CHANNEL" = "production" ] || [ "$CHANNEL" = "testing" ] || die "--channel must be production or testing."

. /etc/os-release
if [ "${ID:-}" != "debian" ] || [ "${VERSION_ID:-}" != "13" ]; then
  echo "Warning: Kiestoon is made for Debian 13. This system is ${PRETTY_NAME:-unknown}. Continuing in 5 seconds…"
  sleep 5
fi

cyan "Kiestoon installer"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq curl jq ca-certificates zstd >/dev/null

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
ARCH="$(dpkg --print-architecture)"

if [ -n "$LOCAL_DEB" ]; then
  DEB_PATH="$(readlink -f "$LOCAL_DEB")"
else
  if [ -z "$VERSION" ]; then
    echo "Looking up the newest release on the $CHANNEL channel…"
    FILTER='.[] | select(.draft | not)'
    [ "$CHANNEL" = "production" ] && FILTER="$FILTER | select(.prerelease | not)"
    VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases?per_page=50" | jq -r "[$FILTER | .tag_name] | first // empty")"
    [ -n "$VERSION" ] || die "no release found on the $CHANNEL channel yet."
  fi
  [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] || die "invalid version: $VERSION"
  DEB="kiestoon_${VERSION#v}_${ARCH}.deb"
  echo "Downloading Kiestoon $VERSION ($ARCH)…"
  curl -fsSL -o "$WORK/$DEB" "https://github.com/$REPO/releases/download/$VERSION/$DEB" || die "download failed: $DEB"
  curl -fsSL -o "$WORK/SHA256SUMS" "https://github.com/$REPO/releases/download/$VERSION/SHA256SUMS" || die "download failed: SHA256SUMS"
  (cd "$WORK" && grep -E " [* ]?${DEB}\$" SHA256SUMS | sha256sum -c --status -) || die "checksum does not match for $DEB"
  DEB_PATH="$WORK/$DEB"
fi

# Remember the channel before the service starts for the first time.
install -d -m 0755 /etc/kiestoon
if [ -f /etc/kiestoon/kiestoon.env ]; then
  sed -i "s/^KIESTOON_CHANNEL=.*/KIESTOON_CHANNEL=$CHANNEL/" /etc/kiestoon/kiestoon.env
fi

echo "Installing…"
apt-get install -y -qq --allow-downgrades "$DEB_PATH" >/dev/null
sed -i "s/^KIESTOON_CHANNEL=.*/KIESTOON_CHANNEL=$CHANNEL/" /etc/kiestoon/kiestoon.env
systemctl restart kiestoon.service

# Wait for the portal and, on a fresh install, for the one-time setup token.
for _ in $(seq 1 30); do
  curl -fsS -o /dev/null http://127.0.0.1:8080/api/health 2>/dev/null && break
  sleep 1
done
IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
echo
cyan "Kiestoon $(kiestoon version | awk '{print $2}') is installed."
echo "  Open:        http://${IP:-<this-machine>}:8080"
if [ -s /var/lib/kiestoon/setup-token ]; then
  echo "  Setup token: $(cat /var/lib/kiestoon/setup-token)"
  echo "  (Show it again later with: kiestoon setup-token)"
fi
echo "  Channel:     $CHANNEL"
echo
