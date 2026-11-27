# Google Play release of the Kiestoon app

Everything for publishing the Android app `nl.rhis.kiestoon` in Google Play. Dutch: [README.nl.md](README.nl.md).

| Path | What |
| --- | --- |
| `android/<lang>/title.txt`, `short_description.txt`, `full_description.txt` | Store listing, English (`en-US`) and Dutch (`nl-NL`). Fastlane `supply` layout |
| `android/<lang>/changelogs/default.txt` | "What's new" text of every upload (`{version}` is filled in) |
| `android/<lang>/images/` | Icon 512 x 512, feature graphic 1024 x 500, five phone screenshots 1080 x 1920 |
| `screenshots.mjs`, `graphics.mjs` | Make the screenshots and graphics again from the demo office |
| `make-upload-key.sh` | Makes the upload key and prints the GitHub secrets |
| `../../.github/workflows/mobile-release.yml` | Builds the signed `.aab` and uploads it to Google Play |
| `../../docs/app-privacy.md` | Privacy policy, published at https://rhisijmuiden.github.io/Kiestoon/app-privacy/ (Dutch: `/nl/app-privacy/`) |

## How a release reaches Google Play

1. A release (tag `vX.Y.Z-rc.N` or `vX.Y.Z`) runs the workflow **Release**.
2. When it succeeds, **Mobile app (Google Play)** builds the App Bundle for arm64, armv7 and x86_64, signs it with
   the upload key and uploads it to the **internal test track**. The versionCode comes from `VERSION`:
   `MMmmpp * 1000 + N` for `-rc.N` and `+ 999` for the release itself (0.7.0-rc.35 = 700035, 0.7.0 = 700999).
3. Promote the build in Play Console (internal > closed > production), or run the workflow by hand and pick a track.

Google Play App Signing holds the real app signing key; the repository only ever sees the **upload key**, and only
as a GitHub secret. `build.gradle.kts` reads it from `KIESTOON_UPLOAD_KEYSTORE` and the password variables; without
them the release build is unsigned.

## Secrets

| Secret | Value |
| --- | --- |
| `ANDROID_UPLOAD_KEYSTORE` | The upload key file as base64 (`make-upload-key.sh` makes it) |
| `ANDROID_UPLOAD_STORE_PASSWORD` | Its password |
| `ANDROID_UPLOAD_KEY_PASSWORD` | Only when the key password differs from the store password |
| `GOOGLE_SERVICES_JSON` | `google-services.json` of the Firebase project, with the Android app `nl.rhis.kiestoon` |
| `PLAY_SERVICE_ACCOUNT_JSON` | JSON key of the service account that may release in Play Console |

Variable `ANDROID_UPLOAD_KEY_ALIAS` only when the alias is not `upload`.

## Remake the screenshots

```sh
make build VERSION="$(cat VERSION)"
tools/docs/demo.sh build/kiestoon-linux-amd64 /tmp/kiestoon-demo 8090
node mobile/store/screenshots.mjs http://127.0.0.1:8090 /tmp/kiestoon-demo mobile/store/android
node mobile/store/graphics.mjs mobile/store/android
```

## Play Console answers

### Data safety

| Question | Answer |
| --- | --- |
| Does your app collect or share any of the required user data types? | Yes |
| Is all of the user data collected by your app encrypted in transit? | Yes |
| Do you provide a way for users to request that their data is deleted? | Yes (unlink the phone under My devices, or ask the organisation; see the privacy policy) |

Data types (all: **collected, not shared**, **required**, purpose **App functionality**):

| Data type | Why | Processed ephemerally |
| --- | --- | --- |
| Personal info > **User IDs** | The sign-in name, sent to the organisation's phone system | No |
| Audio > **Voice or sound recordings** | Call audio, sent to the organisation's phone system (which may record calls when the organisation turned that on) | No |
| App activity > **App interactions** | Calls placed, queue sign-in, do not disturb, sent to the phone system | No |
| Device or other IDs | Push token and phone name, to ring the app | No |

"Not shared": the data only goes to the phone system the user links the app to (a transfer the user starts and
expects) and to Google Firebase Cloud Messaging as a service provider. No ads, analytics or crash reporting.

### Other declarations

| Section | Answer |
| --- | --- |
| App access | Some functionality is restricted. Give the address of a demo Kiestoon, a demo user (with an extension, MFA off) and: "Open the app, enter the server address, leave the pairing code empty, tap Link, sign in with the user below." |
| Ads | No ads |
| Content rating | Category: Utility, productivity, communication or other. Users can communicate with each other (voice calls): Yes. All other questions: No |
| Target audience | 18 and over |
| News app, Government app, Financial features, Health | No |
| Foreground service permissions | `phoneCall` and `microphone`: "Keeps an ongoing VoIP call (audio) running when the user leaves the app or locks the screen." Needs a short video of a call that stays up with the screen locked |
| Full-screen intent | The app is a calling app: incoming calls are shown full screen |
| App category | Business. Tags: Business, Communication |
| Privacy policy | https://rhisijmuiden.github.io/Kiestoon/app-privacy/ |
