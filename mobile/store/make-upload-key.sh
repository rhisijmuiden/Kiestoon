#!/usr/bin/env bash
# Makes the upload key for Google Play and prints what goes into the GitHub secrets.
# Run it on your own computer (needs keytool from a Java JDK), NOT in the repository folder:
#   ./mobile/store/make-upload-key.sh ~/kiestoon-keys
# Keep the .p12 file and the password in your password manager. Never commit them.
set -euo pipefail
DIR=${1:?usage: make-upload-key.sh <folder outside the repository>}
mkdir -p "$DIR" && chmod 700 "$DIR"
KEY="$DIR/kiestoon-upload.p12"
[ -e "$KEY" ] && { echo "$KEY already exists, not overwriting it." >&2; exit 1; }
if git -C "$DIR" rev-parse --is-inside-work-tree > /dev/null 2>&1; then
  echo "$DIR is inside a git repository. Choose a folder outside it." >&2; exit 1
fi
PASS=$(head -c 24 /dev/urandom | base64 | tr -d '/+=' | head -c 28)
keytool -genkeypair -v -storetype PKCS12 -keystore "$KEY" -alias upload \
  -keyalg RSA -keysize 4096 -validity 10000 \
  -storepass "$PASS" -keypass "$PASS" \
  -dname "CN=Kiestoon upload key, O=RHIS, C=NL" > /dev/null
chmod 600 "$KEY"
base64 -w0 "$KEY" > "$KEY.base64" 2>/dev/null || base64 -i "$KEY" | tr -d '\n' > "$KEY.base64"
cat <<MSG

Upload key: $KEY
Password:   $PASS      (store it in your password manager)

Add these in GitHub: Settings > Secrets and variables > Actions > New repository secret
  ANDROID_UPLOAD_KEYSTORE        the contents of $KEY.base64
  ANDROID_UPLOAD_STORE_PASSWORD  $PASS

Then delete $KEY.base64. Keep $KEY itself safe: with Play App Signing a lost upload key can be
reset by Google, but that takes a few days.
Fingerprint (Play Console shows the same under App integrity > Upload key certificate):
MSG
keytool -list -keystore "$KEY" -storepass "$PASS" -alias upload | grep -i "fingerprint"
