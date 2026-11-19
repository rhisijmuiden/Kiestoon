#!/bin/bash
# Builds a Raspberry Pi image with Kiestoon Edge: Raspberry Pi OS Lite (64-bit, Debian 13) with the
# kiestoon-edge package installed. Runs on an arm64 machine, or on amd64 with qemu-user-static.
#
#   sudo ./packaging/edge/pi-image.sh dist/kiestoon-edge_0.6.0-rc.1_arm64.deb dist/
#
# Result: dist/kiestoon-edge-<version>-raspberry-pi-arm64.img.xz
# Write it with Raspberry Pi Imager ("Use custom"); its settings (user, SSH, Wi-Fi) work as usual.
# To pair without a screen, put the pairing code in kiestoon-edge.txt on the boot partition.
set -euo pipefail

DEB="$(readlink -f "${1:?usage: pi-image.sh <kiestoon-edge_arm64.deb> <output-dir>}")"
OUT="$(readlink -f "${2:?usage: pi-image.sh <kiestoon-edge_arm64.deb> <output-dir>}")"
# Raspberry Pi OS Lite (64-bit) based on Debian 13 "trixie". Update both lines together.
BASE_URL="${BASE_URL:-https://downloads.raspberrypi.com/raspios_lite_arm64/images/raspios_lite_arm64-2026-10-06/2026-10-06-raspios-trixie-arm64-lite.img.xz}"
BASE_SHA256="${BASE_SHA256:-483db18a48da399b5b7022ffae9a07bc6d99daab30e593aedac09b69ff422843}"
[ "$(id -u)" = "0" ] || { echo "run as root" >&2; exit 1; }
[ "$(dpkg-deb -f "$DEB" Architecture)" = "arm64" ] || { echo "needs the arm64 package" >&2; exit 1; }
for c in losetup parted resize2fs xz curl; do command -v "$c" >/dev/null || { echo "missing $c" >&2; exit 1; }; done

VERSION="$(dpkg-deb -f "$DEB" Version)"
VERSION="${VERSION//\~/-}"
WORK="$(mktemp -d /tmp/kiestoon-pi.XXXXXX)"
ROOT="$WORK/root"
LOOP=""
cleanup() {
  for m in boot/firmware dev/pts dev proc sys; do umount -l "$ROOT/$m" 2>/dev/null || true; done
  umount -l "$ROOT" 2>/dev/null || true
  [ -n "$LOOP" ] && losetup -d "$LOOP" 2>/dev/null || true
  rm -rf "$WORK"
}
trap cleanup EXIT

CACHE="${CACHE_DIR:-$OUT}/$(basename "$BASE_URL")"
if [ ! -s "$CACHE" ] || ! echo "$BASE_SHA256  $CACHE" | sha256sum -c --quiet - 2>/dev/null; then
  echo "==> downloading $(basename "$BASE_URL")"
  mkdir -p "$(dirname "$CACHE")"
  curl -fL --retry 3 -o "$CACHE" "$BASE_URL"
fi
echo "$BASE_SHA256  $CACHE" | sha256sum -c --quiet - || { echo "checksum of the base image does not match" >&2; exit 1; }

IMG="$WORK/edge.img"
echo "==> unpacking"
xz -dc "$CACHE" >"$IMG"
# Room for Kamailio and rtpengine. Raspberry Pi OS grows the root partition to the card on first boot.
truncate -s +1G "$IMG"
parted -s "$IMG" resizepart 2 100%
LOOP="$(losetup -P --show -f "$IMG")"
e2fsck -fy "${LOOP}p2" >/dev/null || true
resize2fs "${LOOP}p2" >/dev/null

mkdir -p "$ROOT"
mount "${LOOP}p2" "$ROOT"
mount "${LOOP}p1" "$ROOT/boot/firmware"
mount -t proc proc "$ROOT/proc"
mount -t sysfs sys "$ROOT/sys"
mount --bind /dev "$ROOT/dev"
mount --bind /dev/pts "$ROOT/dev/pts"
# Name resolution inside the chroot; the original (often a symlink) is put back afterwards.
mv "$ROOT/etc/resolv.conf" "$ROOT/etc/resolv.conf.kiestoon" 2>/dev/null || true
cp /etc/resolv.conf "$ROOT/etc/resolv.conf"

# Services must not start inside the chroot.
printf '#!/bin/sh\nexit 101\n' >"$ROOT/usr/sbin/policy-rc.d"
chmod +x "$ROOT/usr/sbin/policy-rc.d"

echo "==> installing Kiestoon Edge $VERSION"
cp "$DEB" "$ROOT/tmp/kiestoon-edge.deb"
chroot "$ROOT" apt-get update -qq
chroot "$ROOT" env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends /tmp/kiestoon-edge.deb
rm -f "$ROOT/tmp/kiestoon-edge.deb"
echo kiestoon-edge >"$ROOT/etc/hostname"
sed -i 's/\braspberrypi\b/kiestoon-edge/g' "$ROOT/etc/hosts"
cat >"$ROOT/boot/firmware/kiestoon-edge.txt.example" <<'TXT'
# Kiestoon Edge: to pair without a screen, put the pairing code from the Kiestoon portal
# (Sites > Add branch office) in a file named kiestoon-edge.txt next to this one.
# The edge reads it on start, pairs and removes the file.
TXT

rm -f "$ROOT/usr/sbin/policy-rc.d"
chroot "$ROOT" apt-get clean
rm -rf "$ROOT"/var/lib/apt/lists/* "$ROOT"/tmp/*
rm -f "$ROOT/etc/resolv.conf"
if [ -e "$ROOT/etc/resolv.conf.kiestoon" ] || [ -L "$ROOT/etc/resolv.conf.kiestoon" ]; then
  mv "$ROOT/etc/resolv.conf.kiestoon" "$ROOT/etc/resolv.conf"
fi
for m in boot/firmware dev/pts dev proc sys; do umount -l "$ROOT/$m"; done
umount "$ROOT"
losetup -d "$LOOP"
LOOP=""

mkdir -p "$OUT"
TARGET="$OUT/kiestoon-edge-${VERSION}-raspberry-pi-arm64.img"
if [ -n "${SKIP_COMPRESS:-}" ]; then
  mv "$IMG" "$TARGET"
else
  TARGET="$TARGET.xz"
  echo "==> writing $TARGET"
  xz -T0 -6 -c "$IMG" >"$TARGET"
fi
ls -lh "$TARGET"
