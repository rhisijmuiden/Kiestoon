#!/bin/bash
# Builds kiestoon-sounds-nl: Dutch voice prompts for Asterisk, spoken by Piper from prompts.tsv.
# Runs in a Debian 13 container (or any Debian/Ubuntu with network access):
#   docker run --rm -v "$PWD:/src" -w /src debian:trixie packaging/sounds-nl/build.sh dist-sounds [version]
# The voice nl_NL-pim-medium is trained on a CC0 data set; the generated audio carries no extra licence.
set -euo pipefail
OUT="${1:-dist-sounds}"
VERSION="${2:-$(tr -d '[:space:]' < "$(dirname "$0")/VERSION")}"
HERE="$(cd "$(dirname "$0")" && pwd)"
PIPER_VERSION=1.8.0
VOICE=nl_NL-pim-medium
VOICE_URL="https://huggingface.co/rhasspy/piper-voices/resolve/v1.0.0/nl/nl_NL/pim/medium"
VOICE_SHA256=$(cat "$HERE/voice.sha256")
# English prompts for Kiestoon's own features (queue callback): voice trained on LJ Speech (public domain).
VOICE_EN=en_US-ljspeech-medium
VOICE_EN_URL="https://huggingface.co/rhasspy/piper-voices/resolve/v1.0.0/en/en_US/ljspeech/medium"
VOICE_EN_SHA256=$(cat "$HERE/voice-en.sha256")

if command -v apt-get >/dev/null && [ "$(id -u)" = 0 ]; then
  apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends \
    python3 python3-venv sox curl ca-certificates dpkg-dev >/dev/null
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
python3 -m venv "$WORK/venv"
"$WORK/venv/bin/pip" install -q "piper-tts==$PIPER_VERSION"
for f in onnx onnx.json; do
  curl -fsSL --retry 3 -o "$WORK/$VOICE.$f" "$VOICE_URL/$VOICE.$f"
  curl -fsSL --retry 3 -o "$WORK/$VOICE_EN.$f" "$VOICE_EN_URL/$VOICE_EN.$f"
done
(cd "$WORK" && sha256sum -c - <<<"$VOICE_SHA256" && sha256sum -c - <<<"$VOICE_EN_SHA256")

STAGE="$WORK/stage"
# speak <voice> <prompts.tsv> <sounds dir>
speak() {
  local SOUNDS="$3"
  mkdir -p "$SOUNDS"
  "$WORK/venv/bin/python" "$HERE/speak.py" "$WORK/$1.onnx" "$2" "$SOUNDS"
  while IFS= read -r -d '' raw; do
    base="${raw%.raw.wav}"
    # Trim the silence Piper adds (prompts are chained, for example "you have" 3 "messages"), then
    # telephone band, gentle compression, a short tail and the peak at -1 dBFS.
    sox -q "$raw" -r 8000 -c 1 -b 16 "$WORK/tel.wav" silence 1 0.02 1% reverse silence 1 0.02 1% reverse \
      gain -3 highpass 120 lowpass 3400 compand 0.02,0.2 -60,-60,-30,-15,0,-6 -6 0 0.05 pad 0.02 0.1 norm -1
    cp "$WORK/tel.wav" "$base.wav"
    sox -q "$WORK/tel.wav" -t al "$base.alaw"
    sox -q "$WORK/tel.wav" -t ul "$base.ulaw"
    rm -f "$raw"
  done < <(find "$SOUNDS" -name '*.raw.wav' -print0)
}
speak "$VOICE" "$HERE/prompts.tsv" "$STAGE/var/lib/asterisk/sounds/nl"
speak "$VOICE_EN" "$HERE/prompts-en.tsv" "$STAGE/var/lib/asterisk/sounds/en"

install -d "$STAGE/usr/share/doc/kiestoon-sounds-nl"
cp "$HERE/prompts.tsv" "$HERE/prompts-en.tsv" "$STAGE/usr/share/doc/kiestoon-sounds-nl/"
cat > "$STAGE/usr/share/doc/kiestoon-sounds-nl/copyright" <<'COPY'
Dutch prompts for Kiestoon, generated with Piper (https://github.com/OHF-Voice/piper1-gpl)
using the voice nl_NL-pim-medium (data set: NabuCasa voice-datasets, CC0 1.0).
English prompts for Kiestoon features use the voice en_US-ljspeech-medium (data set: LJ Speech,
public domain).
The prompt texts are part of Kiestoon (GPL-3.0).
COPY
install -d "$STAGE/DEBIAN"
SIZE="$(du -sk --exclude=DEBIAN "$STAGE" | cut -f1)"
cat > "$STAGE/DEBIAN/control" <<CONTROL
Package: kiestoon-sounds-nl
Version: $VERSION
Architecture: all
Maintainer: RHIS <info@rhis.nl>
Installed-Size: $SIZE
Enhances: kiestoon-asterisk
Section: comm
Priority: optional
Homepage: https://github.com/rhisijmuiden/kiestoon
Description: Dutch voice prompts for Kiestoon
 Voicemail, queue, conference and feature-code prompts in Dutch for Asterisk,
 plus English prompts for Kiestoon features such as the queue callback.
 Asterisk uses them when the phone system language is set to Dutch and falls
 back to English for any prompt that is not included.
CONTROL
mkdir -p "$OUT"
dpkg-deb --root-owner-group -Zxz --build "$STAGE" "$OUT/kiestoon-sounds-nl_${VERSION}_all.deb" >/dev/null
ls -l "$OUT"/kiestoon-sounds-nl_*.deb
