#!/bin/bash
# Kiestoon on Proxmox VE: downloads the newest Kiestoon LXC template to this Proxmox host and creates a
# container from it. Run it as root in the shell of the Proxmox host (Datacenter > your node > Shell):
#
#   bash -c "$(curl -fsSL https://raw.githubusercontent.com/rhisijmuiden/Kiestoon/testing/proxmox/kiestoon-lxc.sh)"
#
# Options (all optional):
#   --channel production|testing   Release channel (default: production, or testing while there is no stable release)
#   --version vX.Y.Z               This exact release instead of the newest one
#   --edge                         Kiestoon Edge (branch office) instead of Kiestoon
#   --download-only                Only put the template in the template storage, create no container
#   --id N                         Container ID (default: the next free ID)
#   --hostname NAME                Host name (default: kiestoon, or kiestoon-edge)
#   --template-storage NAME        Storage for the template (default: local)
#   --storage NAME                 Storage for the container disk (default: local-lvm, else local)
#   --disk GB                      Disk size (default: 8, Edge: 4)
#   --cores N                      CPU cores (default: 1)
#   --memory MB                    Memory (default: 1024, Edge: 512)
#   --bridge NAME                  Network bridge (default: vmbr0)
#   --ip dhcp|ADDRESS/PREFIX       IPv4 address (default: dhcp), for example 192.168.1.20/24
#   --gateway ADDRESS              IPv4 gateway, with a fixed --ip
#   --vlan N                       VLAN tag
#   --password PASSWORD            Root password of the container (default: none, use the console or pct enter)
#   --ssh-key FILE                 Public SSH key for root
#   --no-start                     Create the container but do not start it
#   --yes                          Do not ask for confirmation
set -euo pipefail

REPO="${KIESTOON_REPO:-rhisijmuiden/Kiestoon}"
CHANNEL=""
VERSION=""
EDGE=""
DOWNLOAD_ONLY=""
CTID=""
HOSTNAME_=""
TPL_STORAGE="local"
STORAGE=""
DISK=""
CORES="1"
MEMORY=""
BRIDGE="vmbr0"
IP="dhcp"
GATEWAY=""
VLAN=""
PASSWORD=""
SSH_KEY=""
START=1
YES=""

cyan() { printf '\033[1;36m%s\033[0m\n' "$*"; }
bold() { printf '\033[1m%s\033[0m\n' "$*"; }
die() { printf '\033[1;31mError:\033[0m %s\n' "$*" >&2; exit 1; }

while [ $# -gt 0 ]; do
  case "$1" in
    --channel) CHANNEL="${2:-}"; shift 2 ;;
    --version) VERSION="${2:-}"; shift 2 ;;
    --edge) EDGE=1; shift ;;
    --download-only) DOWNLOAD_ONLY=1; shift ;;
    --id) CTID="${2:-}"; shift 2 ;;
    --hostname) HOSTNAME_="${2:-}"; shift 2 ;;
    --template-storage) TPL_STORAGE="${2:-}"; shift 2 ;;
    --storage) STORAGE="${2:-}"; shift 2 ;;
    --disk) DISK="${2:-}"; shift 2 ;;
    --cores) CORES="${2:-}"; shift 2 ;;
    --memory) MEMORY="${2:-}"; shift 2 ;;
    --bridge) BRIDGE="${2:-}"; shift 2 ;;
    --ip) IP="${2:-}"; shift 2 ;;
    --gateway) GATEWAY="${2:-}"; shift 2 ;;
    --vlan) VLAN="${2:-}"; shift 2 ;;
    --password) PASSWORD="${2:-}"; shift 2 ;;
    --ssh-key) SSH_KEY="${2:-}"; shift 2 ;;
    --no-start) START=""; shift ;;
    --yes|-y) YES=1; shift ;;
    -h|--help) sed -n '2,29p' "$0" 2>/dev/null || echo "See https://rhisijmuiden.github.io/Kiestoon/install/"; exit 0 ;;
    *) die "unknown option: $1 (see --help)" ;;
  esac
done

[ "$(id -u)" = "0" ] || die "run this script as root on the Proxmox host."
command -v pveam >/dev/null && command -v pct >/dev/null && command -v pvesm >/dev/null \
  || die "this is not a Proxmox VE host. On a Debian 13 server, use install.sh instead."
for c in curl jq sha256sum; do command -v "$c" >/dev/null || die "$c is missing (apt install $c)."; done
[ -z "$CHANNEL" ] || [ "$CHANNEL" = "production" ] || [ "$CHANNEL" = "testing" ] || die "--channel must be production or testing."
[[ "$CORES" =~ ^[0-9]+$ ]] || die "--cores must be a number."
[ -z "$CTID" ] || [[ "$CTID" =~ ^[0-9]+$ ]] || die "--id must be a number."
[ "$IP" = "dhcp" ] || [[ "$IP" =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}/[0-9]{1,2}$ ]] || die "--ip must be dhcp or an address with prefix, such as 192.168.1.20/24."
[ -z "$SSH_KEY" ] || [ -r "$SSH_KEY" ] || die "cannot read $SSH_KEY."

if [ -n "$EDGE" ]; then
  NAME="kiestoon-edge"; DISK="${DISK:-4}"; MEMORY="${MEMORY:-512}"
else
  NAME="kiestoon"; DISK="${DISK:-8}"; MEMORY="${MEMORY:-1024}"
fi
HOSTNAME_="${HOSTNAME_:-$NAME}"
[[ "$DISK" =~ ^[0-9]+$ ]] || die "--disk must be a number of GB."
[[ "$MEMORY" =~ ^[0-9]+$ ]] || die "--memory must be a number of MB."

ARCH="$(dpkg --print-architecture 2>/dev/null || echo amd64)"
[ "$ARCH" = "amd64" ] || die "Kiestoon LXC templates are made for amd64 Proxmox hosts (this host: $ARCH)."

cyan "Kiestoon for Proxmox VE"

# 1. Which release.
api() { curl -fsSL -H 'Accept: application/vnd.github+json' "https://api.github.com/repos/$REPO/$1"; }
if [ -z "$VERSION" ]; then
  RELEASES="$(api 'releases?per_page=50')" || die "cannot reach GitHub."
  newest() { jq -r "[.[] | select(.draft | not) $1 | .tag_name] | first // empty" <<<"$RELEASES"; }
  if [ "$CHANNEL" = "testing" ]; then
    VERSION="$(newest '')"
  else
    VERSION="$(newest '| select(.prerelease | not)')"
    if [ -z "$VERSION" ] && [ -z "$CHANNEL" ]; then
      echo "There is no stable release yet, so this uses the testing channel (release candidates)."
      VERSION="$(newest '')"
    fi
  fi
  [ -n "$VERSION" ] || die "no release found on the ${CHANNEL:-production} channel yet."
fi
[[ "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$ ]] || die "invalid version: $VERSION"
TEMPLATE="$NAME-${VERSION#v}-debian-13-amd64.tar.zst"
echo "Release:  $VERSION"
echo "Template: $TEMPLATE"

# 2. Download the template into the template storage (vztmpl content), checked against SHA256SUMS.
pvesm status -content vztmpl 2>/dev/null | awk 'NR>1 {print $1}' | grep -qx "$TPL_STORAGE" \
  || die "storage '$TPL_STORAGE' does not hold CT templates. Pick one with --template-storage (see: pvesm status -content vztmpl)."
CACHE="$(pvesm path "$TPL_STORAGE:vztmpl/$TEMPLATE" 2>/dev/null)" || die "cannot find the template folder of storage '$TPL_STORAGE'."
mkdir -p "$(dirname "$CACHE")"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
BASE="https://github.com/$REPO/releases/download/$VERSION"
curl -fsSL -o "$WORK/SHA256SUMS" "$BASE/SHA256SUMS" || die "download failed: SHA256SUMS of $VERSION"
SUM="$(awk -v f="$TEMPLATE" '{n=$2; sub(/^\*/, "", n)} n==f {print $1}' "$WORK/SHA256SUMS")"
[ -n "$SUM" ] || die "release $VERSION has no $TEMPLATE."
if [ -f "$CACHE" ] && [ "$(sha256sum "$CACHE" | awk '{print $1}')" = "$SUM" ]; then
  echo "The template is already on this host: $TPL_STORAGE:vztmpl/$TEMPLATE"
else
  echo "Downloading the template…"
  curl -fL --progress-bar -o "$WORK/$TEMPLATE" "$BASE/$TEMPLATE" || die "download failed: $TEMPLATE"
  [ "$(sha256sum "$WORK/$TEMPLATE" | awk '{print $1}')" = "$SUM" ] || die "checksum does not match for $TEMPLATE."
  mv "$WORK/$TEMPLATE" "$CACHE"
  echo "Saved as $TPL_STORAGE:vztmpl/$TEMPLATE"
fi

if [ -n "$DOWNLOAD_ONLY" ]; then
  bold "Done. Create a container from it under Create CT > Template, or run this script again without --download-only."
  exit 0
fi

# 3. Create the container.
CTID="${CTID:-$(pvesh get /cluster/nextid)}"
pct status "$CTID" >/dev/null 2>&1 && die "container $CTID already exists. Pick another with --id."
if [ -z "$STORAGE" ]; then
  ROOTDIR="$(pvesm status -content rootdir 2>/dev/null | awk 'NR>1 && $3=="active" {print $1}')"
  if grep -qx local-lvm <<<"$ROOTDIR"; then STORAGE=local-lvm
  elif grep -qx local-zfs <<<"$ROOTDIR"; then STORAGE=local-zfs
  else STORAGE="$(head -n1 <<<"$ROOTDIR")"; fi
  [ -n "$STORAGE" ] || die "no storage for container disks found. Pick one with --storage."
fi
NET="name=eth0,bridge=$BRIDGE,ip=$IP"
[ -n "$GATEWAY" ] && NET="$NET,gw=$GATEWAY"
[ -n "$VLAN" ] && NET="$NET,tag=$VLAN"

echo
bold "Container $CTID ($HOSTNAME_)"
echo "  Disk:    ${DISK} GB on $STORAGE"
echo "  CPU/RAM: $CORES core(s), $MEMORY MB"
echo "  Network: $BRIDGE, ${IP}${VLAN:+, VLAN $VLAN}"
if [ -z "$YES" ]; then
  if [ -r /dev/tty ]; then
    read -r -p "Create it? [Y/n] " ANSWER </dev/tty
    case "${ANSWER:-y}" in [Yy]*) ;; *) echo "Stopped. The template stays available."; exit 0 ;; esac
  fi
fi

ARGS=(
  "$CTID" "$TPL_STORAGE:vztmpl/$TEMPLATE"
  --hostname "$HOSTNAME_" --ostype debian --unprivileged 1 --features nesting=1
  --cores "$CORES" --memory "$MEMORY" --swap 512 --rootfs "$STORAGE:$DISK"
  --net0 "$NET" --onboot 1 --description "Kiestoon $VERSION - https://rhisijmuiden.github.io/Kiestoon/"
)
[ -n "$PASSWORD" ] && ARGS+=(--password "$PASSWORD")
[ -n "$SSH_KEY" ] && ARGS+=(--ssh-public-keys "$SSH_KEY")
echo "Creating container $CTID…"
pct create "${ARGS[@]}" >/dev/null

if [ -z "$START" ]; then
  bold "Container $CTID is ready. Start it with: pct start $CTID"
  exit 0
fi
echo "Starting…"
pct start "$CTID"

# 4. Wait for the address (and for Kiestoon), then show how to continue.
ADDR=""
for _ in $(seq 60); do
  ADDR="$(pct exec "$CTID" -- hostname -I 2>/dev/null | awk '{print $1}' || true)"
  [ -n "$ADDR" ] && break
  sleep 2
done
echo
if [ -n "$EDGE" ]; then
  bold "Kiestoon Edge is running in container $CTID."
  echo "  Status page: http://${ADDR:-<container address>}/"
  echo "  Pair it with the code from Sites > Branch offices > Add branch office on the central Kiestoon."
else
  TOKEN=""
  for _ in $(seq 60); do
    TOKEN="$(pct exec "$CTID" -- kiestoon setup-token 2>/dev/null | tr -d '[:space:]' || true)"
    [ -n "$TOKEN" ] && break
    sleep 2
  done
  bold "Kiestoon is running in container $CTID."
  echo "  Portal:      http://${ADDR:-<container address>}:8080"
  [ -n "$TOKEN" ] && echo "  Setup token: $TOKEN"
  [ -n "$TOKEN" ] || echo "  Setup token: pct exec $CTID -- kiestoon setup-token"
  echo "  Manual:      https://rhisijmuiden.github.io/Kiestoon/first-setup/"
fi
[ "$IP" = "dhcp" ] && echo "  Tip: give the container a fixed address (DHCP reservation) so phones keep finding it."
exit 0
