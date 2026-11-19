#!/bin/bash
# Builds Asterisk (LTS) as the kiestoon-asterisk Debian package.
#
# Run as root on Debian 13 (for example in a debian:trixie container):
#   packaging/asterisk/build.sh dist/
# The version and the SHA-256 of the source tarball are pinned in packaging/asterisk/VERSION.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
OUT="$(mkdir -p "${1:-dist}" && cd "${1:-dist}" && pwd)"
read -r AST_VERSION AST_SHA256 < "$HERE/VERSION"
PKG_REVISION="${PKG_REVISION:-1}"
ARCH="$(dpkg --print-architecture)"
WORK="${WORK:-/tmp/kiestoon-asterisk-build}"
STAGE="$WORK/stage"
JOBS="$(nproc)"

export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq --no-install-recommends \
  build-essential ca-certificates curl pkg-config bzip2 patch file binutils \
  libedit-dev libjansson-dev libxml2-dev uuid-dev libsqlite3-dev libssl-dev libsrtp2-dev \
  libcurl4-openssl-dev libspeexdsp-dev libspeex-dev libogg-dev libvorbis-dev libopus-dev \
  libncurses-dev libgsm1-dev libxslt1-dev libpopt-dev libspandsp-dev libtiff-dev libical-dev \
  libunbound-dev >/dev/null

rm -rf "$WORK" && mkdir -p "$WORK" "$STAGE"
cd "$WORK"
TARBALL="asterisk-$AST_VERSION.tar.gz"
curl -fsSL --retry 3 -o "$TARBALL" "https://downloads.asterisk.org/pub/telephony/asterisk/releases/$TARBALL" \
  || curl -fsSL --retry 3 -o "$TARBALL" "https://downloads.asterisk.org/pub/telephony/asterisk/$TARBALL"
echo "$AST_SHA256  $TARBALL" | sha256sum -c -
tar xzf "$TARBALL"
cd "asterisk-$AST_VERSION"

./configure --prefix=/usr --sysconfdir=/etc --localstatedir=/var --libdir=/usr/lib \
  --with-pjproject-bundled >/dev/null

make menuselect.makeopts >/dev/null
# Portable binaries (no CPU-specific optimisation), only the modules a business PBX needs,
# and the English prompts and music on hold in G.711 a-law and u-law.
for opt in \
  "--disable BUILD_NATIVE" "--disable-category MENUSELECT_TESTS" \
  "--disable chan_iax2" "--disable chan_unistim" "--disable chan_motif" "--disable res_xmpp" \
  "--disable cdr_radius" "--disable cel_radius" "--disable cdr_tds" "--disable cel_tds" \
  "--enable app_voicemail" "--enable cdr_manager" "--enable res_srtp" "--enable res_fax" "--enable res_fax_spandsp" \
  "--enable CORE-SOUNDS-EN-ALAW" "--enable CORE-SOUNDS-EN-ULAW" "--enable CORE-SOUNDS-EN-G722" \
  "--enable EXTRA-SOUNDS-EN-ALAW" "--enable EXTRA-SOUNDS-EN-ULAW" \
  "--enable MOH-OPSOUND-ALAW" "--enable MOH-OPSOUND-ULAW" "--enable MOH-OPSOUND-G722"; do
  # shellcheck disable=SC2086 # $opt holds a flag and its value
  menuselect/menuselect $opt menuselect.makeopts >/dev/null || echo "warning: menuselect $opt not applied"
done
if grep -qE '^MENUSELECT_CFLAGS=.*\bBUILD_NATIVE\b' menuselect.makeopts; then echo "BUILD_NATIVE is still on"; exit 1; fi
grep -E '^MENUSELECT_(CFLAGS|CORE_SOUNDS|MOH|EXTRA_SOUNDS)=' menuselect.makeopts

make -j"$JOBS" ASTCFLAGS="-O2 -g0" NOISY_BUILD=no >/dev/null
make install DESTDIR="$STAGE" NOISY_BUILD=no >/dev/null

# Fax-to-mail needs SpanDSP (T.38 and G.711 fax).
for mod in res_fax.so res_fax_spandsp.so; do
  find "$STAGE" -name "$mod" | grep -q . || { echo "$mod was not built"; exit 1; }
done

# No sample configuration: the kiestoon package ships and generates the configuration.
rm -rf "$STAGE/etc/asterisk" "$STAGE/usr/share/man"/*/astdb2* 2>/dev/null || true
install -D -m 0644 "$HERE/asterisk.service" "$STAGE/usr/lib/systemd/system/asterisk.service"
find "$STAGE" -type f -name '*.so*' -exec strip --strip-unneeded {} + 2>/dev/null || true
strip --strip-unneeded "$STAGE/usr/sbin/asterisk" 2>/dev/null || true

# Work out the runtime dependencies from the libraries the binaries link against.
deps=()
while read -r lib; do
  [ -n "$lib" ] || continue
  # Libraries we ship ourselves (libasteriskpj, libasteriskssl).
  if find "$STAGE/usr/lib" -maxdepth 1 -name "$lib" | grep -q .; then continue; fi
  path="$(ldconfig -p | awk -v l="$lib" '$1 == l { print $NF; exit }')"
  [ -n "$path" ] || { echo "warning: $lib not found"; continue; }
  pkg="$(dpkg -S "$(readlink -f "$path")" 2>/dev/null | head -n1 | cut -d: -f1)"
  [ -n "$pkg" ] || pkg="$(dpkg -S "$path" 2>/dev/null | head -n1 | cut -d: -f1)"
  [ -n "$pkg" ] && deps+=("$pkg")
done < <(find "$STAGE" -type f \( -name '*.so*' -o -path '*/sbin/*' \) -exec sh -c 'file -b "$1" | grep -q ELF && objdump -p "$1" | awk "/NEEDED/ {print \$2}"' _ {} \; | sort -u)
DEPENDS="$(printf '%s\n' "${deps[@]}" adduser | sort -u | paste -sd, | sed 's/,/, /g')"

install -d "$STAGE/DEBIAN"
install -m 0755 "$HERE/postinst.sh" "$STAGE/DEBIAN/postinst"
install -m 0755 "$HERE/prerm.sh" "$STAGE/DEBIAN/prerm"
install -m 0755 "$HERE/postrm.sh" "$STAGE/DEBIAN/postrm"
SIZE="$(du -sk --exclude=DEBIAN "$STAGE" | cut -f1)"
cat > "$STAGE/DEBIAN/control" <<CONTROL
Package: kiestoon-asterisk
Version: $AST_VERSION-$PKG_REVISION
Architecture: $ARCH
Maintainer: RHIS <info@rhis.nl>
Installed-Size: $SIZE
Depends: $DEPENDS
Conflicts: asterisk, asterisk-modules
Replaces: asterisk, asterisk-modules
Provides: asterisk
Section: comm
Priority: optional
Homepage: https://github.com/rhisijmuiden/kiestoon
Description: Asterisk $AST_VERSION LTS for Kiestoon
 Asterisk telephony engine built for the Kiestoon phone system: PJSIP, voicemail,
 SRTP, fax (SpanDSP), CDR over AMI and English prompts in G.711 and G.722.
CONTROL
DEB="$OUT/kiestoon-asterisk_${AST_VERSION}-${PKG_REVISION}_${ARCH}.deb"
dpkg-deb --root-owner-group -Zxz --build "$STAGE" "$DEB" >/dev/null
echo "built $DEB"
dpkg-deb -I "$DEB" | sed -n '/Depends/p'
