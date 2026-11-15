#!/bin/bash
# Builds a Proxmox LXC template (Debian 13 + Kiestoon) from a Kiestoon .deb.
#
#   sudo ./lxc/build-lxc.sh dist/kiestoon_0.1.0_amd64.deb dist/ [dist/kiestoon-asterisk_22.x_amd64.deb] [dist/kiestoon-sounds-nl_x_all.deb] [dist/kiestoon-speech_x_amd64.deb]
#
# Result: dist/kiestoon-<version>-debian-13-<arch>.tar.zst
# Upload it to Proxmox (Datacenter > storage > CT Templates > Upload) and create a container from it.
# Unprivileged containers work. Give it 1 CPU, 1 GB RAM and 8 GB disk to start.
set -euo pipefail

DEB="$(readlink -f "${1:?usage: build-lxc.sh <kiestoon.deb> <output-dir>}")"
OUT="$(readlink -f "${2:?usage: build-lxc.sh <kiestoon.deb> <output-dir>}")"
AST_DEB="${3:-}"
[ -n "$AST_DEB" ] && AST_DEB="$(readlink -f "$AST_DEB")"
SND_DEB="${4:-}"
[ -n "$SND_DEB" ] && SND_DEB="$(readlink -f "$SND_DEB")"
SPEECH_DEB="${5:-}"
[ -n "$SPEECH_DEB" ] && SPEECH_DEB="$(readlink -f "$SPEECH_DEB")"
MIRROR="${MIRROR:-http://deb.debian.org/debian}"
[ "$(id -u)" = "0" ] || { echo "run as root" >&2; exit 1; }
command -v debootstrap >/dev/null || { echo "install debootstrap first" >&2; exit 1; }

VERSION="$(dpkg-deb -f "$DEB" Version)"
ARCH="$(dpkg-deb -f "$DEB" Architecture)"
VERSION="${VERSION//\~/-}" # 0.1.0~rc.1 -> 0.1.0-rc.1 in the file name
ROOT="$(mktemp -d /tmp/kiestoon-lxc.XXXXXX)"
cleanup() {
  for m in dev/pts dev proc sys; do umount -l "$ROOT/$m" 2>/dev/null || true; done
  rm -rf "$ROOT"
}
trap cleanup EXIT

# Older debootstrap versions do not know "trixie" yet; the sid script handles it.
SCRIPT=""
[ -e /usr/share/debootstrap/scripts/trixie ] || SCRIPT=/usr/share/debootstrap/scripts/sid

echo "==> debootstrap Debian 13 (trixie, $ARCH)"
debootstrap --arch="$ARCH" --variant=minbase \
  --include=systemd,systemd-sysv,dbus,ifupdown,isc-dhcp-client,iproute2,iputils-ping,ca-certificates,curl,jq,zstd,adduser,openssh-server,less,nano,tzdata,locales,cron,logrotate \
  trixie "$ROOT" "$MIRROR" $SCRIPT

mount -t proc proc "$ROOT/proc"
mount -t sysfs sys "$ROOT/sys"
mount --bind /dev "$ROOT/dev"
mount --bind /dev/pts "$ROOT/dev/pts"

cat >"$ROOT/etc/apt/sources.list" <<SRC
deb http://deb.debian.org/debian trixie main
deb http://deb.debian.org/debian trixie-updates main
deb http://security.debian.org/debian-security trixie-security main
SRC

# Proxmox writes /etc/network/interfaces and the hostname when it creates the container.
printf 'auto lo\niface lo inet loopback\n\nauto eth0\niface eth0 inet dhcp\n' >"$ROOT/etc/network/interfaces"
echo kiestoon >"$ROOT/etc/hostname"
echo "en_US.UTF-8 UTF-8" >"$ROOT/etc/locale.gen"
chroot "$ROOT" locale-gen >/dev/null
echo 'LANG=en_US.UTF-8' >"$ROOT/etc/default/locale"
ln -sf /usr/share/zoneinfo/Europe/Amsterdam "$ROOT/etc/localtime"
echo Europe/Amsterdam >"$ROOT/etc/timezone"

echo "==> installing Kiestoon $VERSION"
cp "$DEB" "$ROOT/tmp/kiestoon.deb"
PKGS=(/tmp/kiestoon.deb)
if [ -n "$AST_DEB" ]; then
  cp "$AST_DEB" "$ROOT/tmp/kiestoon-asterisk.deb"
  PKGS+=(/tmp/kiestoon-asterisk.deb)
fi
if [ -n "$SND_DEB" ]; then
  cp "$SND_DEB" "$ROOT/tmp/kiestoon-sounds-nl.deb"
  PKGS+=(/tmp/kiestoon-sounds-nl.deb)
fi
if [ -n "$SPEECH_DEB" ]; then
  cp "$SPEECH_DEB" "$ROOT/tmp/kiestoon-speech.deb"
  PKGS+=(/tmp/kiestoon-speech.deb)
fi
chroot "$ROOT" env DEBIAN_FRONTEND=noninteractive apt-get install -y "${PKGS[@]}"
rm -f "$ROOT"/tmp/kiestoon*.deb

# Every container gets its own SSH host keys and machine id on first boot.
rm -f "$ROOT"/etc/ssh/ssh_host_*
cat >"$ROOT/etc/systemd/system/kiestoon-firstboot.service" <<'UNIT'
[Unit]
Description=Kiestoon first boot: generate SSH host keys
Before=ssh.service
ConditionPathExists=!/etc/ssh/ssh_host_ed25519_key

[Service]
Type=oneshot
ExecStart=/usr/bin/ssh-keygen -A

[Install]
WantedBy=multi-user.target
UNIT
chroot "$ROOT" systemctl enable kiestoon-firstboot.service >/dev/null
: >"$ROOT/etc/machine-id"
rm -f "$ROOT/var/lib/dbus/machine-id"

chroot "$ROOT" apt-get clean
rm -rf "$ROOT"/var/lib/apt/lists/* "$ROOT"/var/log/*.log "$ROOT"/tmp/* "$ROOT"/root/.bash_history

for m in dev/pts dev proc sys; do umount -l "$ROOT/$m"; done

mkdir -p "$OUT"
TARGET="$OUT/kiestoon-${VERSION}-debian-13-${ARCH}.tar.zst"
echo "==> writing $TARGET"
tar --zstd -cpf "$TARGET" -C "$ROOT" --numeric-owner --xattrs .
ls -lh "$TARGET"
