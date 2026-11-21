#!/bin/bash
# Kiestoon installer for Debian 13 (Trixie).
#
#   curl -fsSL https://raw.githubusercontent.com/rhisijmuiden/Kiestoon/testing/install.sh | sudo bash
#   curl -fsSL https://raw.githubusercontent.com/rhisijmuiden/Kiestoon/testing/install.sh | sudo bash -s -- --channel testing
#
# Options:
#   --channel production|testing   Update channel to follow (default: production, or testing while there is no stable release)
#   --version vX.Y.Z               Install this exact release instead of the newest one
#   --deb /path/to/kiestoon.deb    Install a local package (for development)
#   --asterisk-deb /path/to.deb    Local kiestoon-asterisk package to install with --deb
#   --sounds-deb /path/to.deb      Local kiestoon-sounds-nl package (Dutch prompts) to install with --deb
#   --speech-deb /path/to.deb      Local kiestoon-speech package (text-to-speech, speech-to-text) to install with --deb
#   --dry-run                      Only show which release and packages would be installed
set -euo pipefail

REPO="${KIESTOON_REPO:-rhisijmuiden/kiestoon}"
CHANNEL="production"
CHANNEL_SET=""
VERSION=""
LOCAL_DEB=""
AST_PATH=""
SND_PATH=""
SPEECH_PATH=""
DRY_RUN=""

cyan() { printf '\033[1;36m%s\033[0m\n' "$*"; }
die() { printf '\033[1;31mError:\033[0m %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --channel) CHANNEL="${2:-}"; CHANNEL_SET=1; shift 2 ;;
    --version) VERSION="${2:-}"; shift 2 ;;
    --deb) LOCAL_DEB="${2:-}"; shift 2 ;;
    --asterisk-deb) AST_PATH="$(readlink -f "${2:-}")"; shift 2 ;;
    --sounds-deb) SND_PATH="$(readlink -f "${2:-}")"; shift 2 ;;
    --speech-deb) SPEECH_PATH="$(readlink -f "${2:-}")"; shift 2 ;;
    --dry-run) DRY_RUN=1; shift ;;
    -h|--help) sed -n '2,14p' "$0"; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done

[ -n "$DRY_RUN" ] || [ "$(id -u)" = "0" ] || die "run this installer as root (sudo)."
[ "$CHANNEL" = "production" ] || [ "$CHANNEL" = "testing" ] || die "--channel must be production or testing."

# Read os-release in a subshell: it defines VERSION (for example "13 (trixie)"), which would
# overwrite our own VERSION and break the release lookup.
OS_RELEASE="${KIESTOON_OS_RELEASE:-/etc/os-release}"
# shellcheck disable=SC1090
os_field() { [ -r "$OS_RELEASE" ] && (. "$OS_RELEASE" && printf '%s' "${!1:-}") || true; }
OS_ID="$(os_field ID)"
OS_VERSION_ID="$(os_field VERSION_ID)"
OS_NAME="$(os_field PRETTY_NAME)"
if [ "$OS_ID" != "debian" ] || [ "$OS_VERSION_ID" != "13" ]; then
  echo "Warning: Kiestoon is made for Debian 13. This system is ${OS_NAME:-unknown}. Continuing in 5 seconds…"
  [ -n "$DRY_RUN" ] || sleep 5
fi

cyan "Kiestoon installer"
# Find the release to install (the newest on the channel unless --version was given) and check it.
resolve_version() {
  if [ -z "$VERSION" ]; then
    echo "Looking up the newest release on the $CHANNEL channel…"
    FILTER='.[] | select(.draft | not)'
    [ "$CHANNEL" = "production" ] && FILTER="$FILTER | select(.prerelease | not)"
    RELEASES="$(curl -fsSL "https://api.github.com/repos/$REPO/releases?per_page=50")" || die "cannot reach GitHub."
    VERSION="$(jq -r "[$FILTER | .tag_name] | first // empty" <<<"$RELEASES")"
    if [ -z "$VERSION" ] && [ "$CHANNEL" = "production" ] && [ -z "$CHANNEL_SET" ]; then
      echo "There is no stable release yet, so this follows the testing channel (release candidates)."
      CHANNEL="testing"
      VERSION="$(jq -r '[.[] | select(.draft | not) | .tag_name] | first // empty' <<<"$RELEASES")"
    fi
    [ -n "$VERSION" ] || die "no release found on the $CHANNEL channel yet."
  fi
  [[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] || die "invalid version: $VERSION"
}
if [ -n "$DRY_RUN" ]; then
  if [ -n "$LOCAL_DEB" ]; then
    echo "Would install local package: $LOCAL_DEB"
    exit 0
  fi
  resolve_version
  echo "Would install Kiestoon $VERSION: kiestoon_${VERSION#v}_$(dpkg --print-architecture 2>/dev/null || echo amd64).deb (channel $CHANNEL)"
  exit 0
fi
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq curl jq ca-certificates zstd >/dev/null

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
ARCH="$(dpkg --print-architecture)"

if [ -n "$LOCAL_DEB" ]; then
  DEB_PATH="$(readlink -f "$LOCAL_DEB")"
else
  resolve_version
  DEB="kiestoon_${VERSION#v}_${ARCH}.deb"
  echo "Downloading Kiestoon $VERSION ($ARCH)…"
  curl -fsSL -o "$WORK/$DEB" "https://github.com/$REPO/releases/download/$VERSION/$DEB" || die "download failed: $DEB"
  curl -fsSL -o "$WORK/SHA256SUMS" "https://github.com/$REPO/releases/download/$VERSION/SHA256SUMS" || die "download failed: SHA256SUMS"
  (cd "$WORK" && grep -E " [* ]?${DEB}\$" SHA256SUMS | sha256sum -c --status -) || die "checksum does not match for $DEB"
  DEB_PATH="$WORK/$DEB"
  AST_DEB="$(grep -oE "kiestoon-asterisk_[0-9A-Za-z.+~-]+_${ARCH}\.deb" "$WORK/SHA256SUMS" | head -n1 || true)"
  if [ -n "$AST_DEB" ]; then
    echo "Downloading Asterisk ($AST_DEB)…"
    curl -fsSL -o "$WORK/$AST_DEB" "https://github.com/$REPO/releases/download/$VERSION/$AST_DEB" || die "download failed: $AST_DEB"
    (cd "$WORK" && grep -E " [* ]?${AST_DEB}\$" SHA256SUMS | sha256sum -c --status -) || die "checksum does not match for $AST_DEB"
    AST_PATH="$WORK/$AST_DEB"
  fi
  SND_DEB="$(grep -oE "kiestoon-sounds-nl_[0-9A-Za-z.+~-]+_all\.deb" "$WORK/SHA256SUMS" | head -n1 || true)"
  if [ -n "$SND_DEB" ]; then
    echo "Downloading Dutch voice prompts ($SND_DEB)…"
    curl -fsSL -o "$WORK/$SND_DEB" "https://github.com/$REPO/releases/download/$VERSION/$SND_DEB" || die "download failed: $SND_DEB"
    (cd "$WORK" && grep -E " [* ]?${SND_DEB}\$" SHA256SUMS | sha256sum -c --status -) || die "checksum does not match for $SND_DEB"
    SND_PATH="$WORK/$SND_DEB"
  fi
  SPEECH_DEB="$(grep -oE "kiestoon-speech_[0-9A-Za-z.+~-]+_${ARCH}\.deb" "$WORK/SHA256SUMS" | head -n1 || true)"
  if [ -n "$SPEECH_DEB" ]; then
    echo "Downloading local speech ($SPEECH_DEB)…"
    curl -fsSL -o "$WORK/$SPEECH_DEB" "https://github.com/$REPO/releases/download/$VERSION/$SPEECH_DEB" || die "download failed: $SPEECH_DEB"
    (cd "$WORK" && grep -E " [* ]?${SPEECH_DEB}\$" SHA256SUMS | sha256sum -c --status -) || die "checksum does not match for $SPEECH_DEB"
    SPEECH_PATH="$WORK/$SPEECH_DEB"
  fi
fi

# Remember the channel before the service starts for the first time.
install -d -m 0755 /etc/kiestoon
if [ -f /etc/kiestoon/kiestoon.env ]; then
  sed -i "s/^KIESTOON_CHANNEL=.*/KIESTOON_CHANNEL=$CHANNEL/" /etc/kiestoon/kiestoon.env
fi

echo "Installing…"
PKGS=("$DEB_PATH")
[ -n "$AST_PATH" ] && PKGS+=("$AST_PATH")
[ -n "$SND_PATH" ] && PKGS+=("$SND_PATH")
[ -n "$SPEECH_PATH" ] && PKGS+=("$SPEECH_PATH")
apt-get install -y -qq --allow-downgrades "${PKGS[@]}" >/dev/null
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
