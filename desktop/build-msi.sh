#!/usr/bin/env bash
# Builds the Windows softphone MSI. Runs on Windows (Git Bash) in CI.
#   desktop/build-msi.sh <tag> <out-dir>
# The MSI file version must be numeric: v0.7.0-rc.3 becomes 0.7.0-3 (MSI 0.7.0.3), v0.7.0 stays 0.7.0.
# The app itself reports the real tag (KIESTOON_VERSION) and compares that for updates.
# With the AZURE_* secrets set, the app and the MSI are signed with Azure Trusted Signing.
set -euo pipefail
TAG="${1:?tag}"
OUT="${2:?out dir}"
cd "$(dirname "$0")"

core="${TAG#v}"
core="${core%%-*}"
pre=""
if [[ "$TAG" =~ -rc\.([0-9]+)$ ]]; then pre="-${BASH_REMATCH[1]}"; fi
MSI_VERSION="${core}${pre}"
echo "Kiestoon Softphone ${TAG} (MSI ${MSI_VERSION})"

conf="{\"version\":\"${MSI_VERSION}\""
if [[ -n "${AZURE_CLIENT_ID:-}" && -n "${AZURE_SIGNING_ENDPOINT:-}" ]]; then
  cargo install trusted-signing-cli --locked --version 0.5.0
  sign="trusted-signing-cli -e ${AZURE_SIGNING_ENDPOINT} -a ${AZURE_SIGNING_ACCOUNT} -c ${AZURE_SIGNING_PROFILE} -d \\\"Kiestoon Softphone\\\" %1"
  conf+=",\"bundle\":{\"windows\":{\"signCommand\":\"${sign}\"}}"
  echo "Signing with Azure Trusted Signing"
else
  echo "No signing secrets: the MSI is not signed"
fi
conf+="}"

npm ci --no-audit --no-fund
KIESTOON_VERSION="$TAG" npx tauri build --bundles msi --config "$conf"

mkdir -p "$OUT"
msi=$(ls src-tauri/target/release/bundle/msi/*.msi | head -n1)
cp "$msi" "$OUT/Kiestoon-Softphone_${TAG}_x64.msi"
ls -l "$OUT"
