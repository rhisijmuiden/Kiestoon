#!/bin/bash
# Checks install.sh without installing anything: its own VERSION must survive reading
# /etc/os-release, which on Debian 13 sets VERSION="13 (trixie)".
#   packaging/ci/install-sh-test.sh
set -euo pipefail
cd "$(dirname "$0")/../.."
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
cat >"$TMP/os-release" <<'OS'
PRETTY_NAME="Debian GNU/Linux 13 (trixie)"
NAME="Debian GNU/Linux"
VERSION_ID="13"
VERSION="13 (trixie)"
VERSION_CODENAME=trixie
ID=debian
OS
fail() { echo "FAIL: $*"; exit 1; }
run() { KIESTOON_OS_RELEASE="$TMP/os-release" bash install.sh --dry-run "$@" 2>&1; }

out="$(run --version v0.7.0-rc.10)" || fail "dry run with --version: $out"
grep -q "Would install Kiestoon v0.7.0-rc.10: kiestoon_0.7.0-rc.10_" <<<"$out" || fail "unexpected output: $out"
grep -q "Warning" <<<"$out" && fail "Debian 13 should not get the warning: $out"

out="$(run --version '13 (trixie)')" && fail "a bad --version was accepted: $out"
grep -q "invalid version: 13 (trixie)" <<<"$out" || fail "unexpected output: $out"

sed -i 's/^VERSION_ID=.*/VERSION_ID="12"/' "$TMP/os-release"
out="$(run --version v1.0.0)" || fail "dry run on Debian 12: $out"
grep -q "Warning: Kiestoon is made for Debian 13" <<<"$out" || fail "no warning on Debian 12: $out"

out="$(run --deb /tmp/kiestoon.deb)" || fail "dry run with --deb: $out"
grep -q "Would install local package" <<<"$out" || fail "unexpected output: $out"
echo "install.sh test passed"
