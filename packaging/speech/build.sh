#!/bin/bash
# Builds kiestoon-speech: local text-to-speech (Piper) and speech-to-text (whisper.cpp) for Kiestoon.
# Runs in a Debian 13 container of the target architecture:
#   docker run --rm -v "$PWD:/src" -w /src debian:trixie packaging/speech/build.sh dist-speech [version]
# The package holds the programs only. Voices and speech models are downloaded from the portal
# (Settings > Speech) into /var/lib/kiestoon/speech, with pinned SHA-256 checksums.
set -euo pipefail
OUT="${1:-dist-speech}"
HERE="$(cd "$(dirname "$0")" && pwd)"
VERSION="${2:-$(tr -d '[:space:]' < "$HERE/VERSION")}"
PIPER_VERSION=1.8.0
WHISPER_TAG=v1.9.5
WHISPER_COMMIT=d1be6fde11ac6e0407606b4e42fe72d34add8037
LLAMA_TAG=b12000
LLAMA_COMMIT=68d20b884c3878217de79eade0a249efde96ab38
PREFIX=/opt/kiestoon-speech
ARCH="$(dpkg --print-architecture)"

if command -v apt-get >/dev/null && [ "$(id -u)" = 0 ]; then
  apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends \
    python3 python3-venv git cmake build-essential ca-certificates dpkg-dev >/dev/null
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK" "$PREFIX"' EXIT
STAGE="$WORK/stage"

# Piper in its own virtual environment at the final path (venvs are not relocatable).
rm -rf "$PREFIX"
python3 -m venv "$PREFIX/venv"
"$PREFIX/venv/bin/pip" install -q --no-cache-dir "piper-tts==$PIPER_VERSION"
"$PREFIX/venv/bin/pip" uninstall -q -y pip setuptools 2>/dev/null || true
find "$PREFIX/venv" -name '__pycache__' -prune -exec rm -rf {} +

# whisper.cpp, with a CPU backend for every instruction set (picked at run time).
git clone -q --depth 1 --branch "$WHISPER_TAG" https://github.com/ggml-org/whisper.cpp "$WORK/whisper"
[ "$(git -C "$WORK/whisper" rev-parse HEAD)" = "$WHISPER_COMMIT" ] || { echo "whisper.cpp tag moved"; exit 1; }
cmake -S "$WORK/whisper" -B "$WORK/whisper/build" -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$PREFIX" \
  -DCMAKE_INSTALL_RPATH='$ORIGIN' -DCMAKE_BUILD_WITH_INSTALL_RPATH=ON -DBUILD_SHARED_LIBS=ON -DGGML_NATIVE=OFF -DGGML_BACKEND_DL=ON -DGGML_CPU_ALL_VARIANTS=ON \
  -DWHISPER_BUILD_TESTS=OFF -DWHISPER_BUILD_SERVER=OFF -DWHISPER_SDL2=OFF -DWHISPER_CURL=OFF >/dev/null
cmake --build "$WORK/whisper/build" -j"$(nproc)" --target whisper-cli >/dev/null
# ggml looks for its CPU backends next to the program (it resolves the symlink in bin/).
install -d "$PREFIX/bin" "$PREFIX/lib"
install -m 0755 "$WORK/whisper/build/bin/whisper-cli" "$PREFIX/lib/whisper-cli"
find "$WORK/whisper/build" -name '*.so*' \( -type f -o -type l \) -exec cp -a {} "$PREFIX/lib/" \;
strip --strip-unneeded "$PREFIX/lib/whisper-cli" "$PREFIX"/lib/*.so* 2>/dev/null || true
ln -s ../lib/whisper-cli "$PREFIX/bin/whisper-cli"

# llama.cpp's llama-server for local AI summaries. It has its own ggml libraries in llama/lib, so
# whisper.cpp and llama.cpp can each use the ggml version they were released with.
git clone -q --depth 1 --branch "$LLAMA_TAG" https://github.com/ggml-org/llama.cpp "$WORK/llama"
[ "$(git -C "$WORK/llama" rev-parse HEAD)" = "$LLAMA_COMMIT" ] || { echo "llama.cpp tag moved"; exit 1; }
cmake -S "$WORK/llama" -B "$WORK/llama/build" -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_INSTALL_RPATH='$ORIGIN' -DCMAKE_BUILD_WITH_INSTALL_RPATH=ON -DBUILD_SHARED_LIBS=ON -DGGML_NATIVE=OFF -DGGML_BACKEND_DL=ON -DGGML_CPU_ALL_VARIANTS=ON \
  -DLLAMA_CURL=OFF -DLLAMA_OPENSSL=OFF -DLLAMA_BUILD_TESTS=OFF -DLLAMA_BUILD_EXAMPLES=OFF -DLLAMA_BUILD_SERVER=ON >/dev/null
cmake --build "$WORK/llama/build" -j"$(nproc)" --target llama-server >/dev/null
install -d "$PREFIX/llama/bin" "$PREFIX/llama/lib"
install -m 0755 "$WORK/llama/build/bin/llama-server" "$PREFIX/llama/lib/llama-server"
find "$WORK/llama/build" -name '*.so*' \( -type f -o -type l \) -exec cp -a {} "$PREFIX/llama/lib/" \;
strip --strip-unneeded "$PREFIX/llama/lib/llama-server" "$PREFIX"/llama/lib/*.so* 2>/dev/null || true
ln -s ../lib/llama-server "$PREFIX/llama/bin/llama-server"

cat > "$PREFIX/bin/piper" <<'SH'
#!/bin/sh
exec /opt/kiestoon-speech/venv/bin/python3 -m piper "$@"
SH
chmod 0755 "$PREFIX/bin/piper"

# Smoke test: the programs start.
"$PREFIX/bin/whisper-cli" --help >/dev/null
"$PREFIX/llama/bin/llama-server" --help >/dev/null
"$PREFIX/bin/piper" --help >/dev/null

mkdir -p "$STAGE/opt"
cp -a "$PREFIX" "$STAGE/opt/"
install -d "$STAGE/usr/share/doc/kiestoon-speech"
cat > "$STAGE/usr/share/doc/kiestoon-speech/copyright" <<COPY
kiestoon-speech bundles:
 - Piper $PIPER_VERSION (https://github.com/OHF-Voice/piper1-gpl), GPL-3.0, with ONNX Runtime (MIT)
   and espeak-ng (GPL-3.0) as installed by pip.
 - whisper.cpp $WHISPER_TAG (https://github.com/ggml-org/whisper.cpp), MIT.
 - llama.cpp $LLAMA_TAG (https://github.com/ggml-org/llama.cpp), MIT.
Voices, Whisper models and language models are downloaded separately; their licences are shown in the portal.
COPY
install -d "$STAGE/DEBIAN"
SIZE="$(du -sk --exclude=DEBIAN "$STAGE" | cut -f1)"
PYDEP="$(python3 -c 'import sys; print(f"python3 (>= {sys.version_info[0]}.{sys.version_info[1]}), python3 (<< {sys.version_info[0]}.{sys.version_info[1]+1})")')"
cat > "$STAGE/DEBIAN/control" <<CONTROL
Package: kiestoon-speech
Version: $VERSION
Architecture: $ARCH
Maintainer: RHIS <info@rhis.nl>
Installed-Size: $SIZE
Depends: $PYDEP, sox, libgomp1
Enhances: kiestoon
Section: sound
Priority: optional
Homepage: https://github.com/rhisijmuiden/kiestoon
Description: Local text-to-speech, speech-to-text and AI summaries for Kiestoon
 Piper turns typed text into voice prompts; whisper.cpp transcribes voicemail and
 recorded calls; llama.cpp summarises them with a local language model.
 Everything runs on the phone system itself: no audio leaves the server.
CONTROL
mkdir -p "$OUT"
dpkg-deb --root-owner-group -Zxz --build "$STAGE" "$OUT/kiestoon-speech_${VERSION}_${ARCH}.deb" >/dev/null
ls -l "$OUT"/kiestoon-speech_*.deb
