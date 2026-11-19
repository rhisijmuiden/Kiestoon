#!/bin/sh
# Adds what the iPhone app needs to the Xcode project that `tauri ios init` generates (that project
# is not kept in git): VoIP push for incoming calls (PushKit + CallKit), visible notifications for
# voicemail, the microphone, and audio that keeps playing while the phone is locked.
#   ./ios-setup.sh src-tauri/gen/apple
set -eu
dir="${1:-src-tauri/gen/apple}"
pb=/usr/libexec/PlistBuddy

plist="$(find "$dir" -maxdepth 2 -name Info.plist -path '*_iOS/*' | head -n1)"
ent="$(find "$dir" -maxdepth 2 -name '*.entitlements' -path '*_iOS/*' | head -n1)"
[ -n "$plist" ] && [ -n "$ent" ] || { echo "no Info.plist or entitlements in $dir" >&2; exit 1; }

set_plist() { $pb -c "Delete :$1" "$2" 2>/dev/null || true; $pb -c "Add :$1 $3 $4" "$2"; }

$pb -c "Delete :UIBackgroundModes" "$plist" 2>/dev/null || true
$pb -c "Add :UIBackgroundModes array" "$plist"
i=0
for m in voip audio remote-notification; do
  $pb -c "Add :UIBackgroundModes:$i string $m" "$plist"
  i=$((i + 1))
done
set_plist NSMicrophoneUsageDescription "$plist" string "Kiestoon uses the microphone for your calls."
set_plist ITSAppUsesNonExemptEncryption "$plist" bool false

# Development push for debug builds; the App Store build is signed with production.
set_plist aps-environment "$ent" string "${APS_ENVIRONMENT:-development}"

echo "iOS project set up: $plist, $ent"
