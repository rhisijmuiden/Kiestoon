# Privacy policy of the Kiestoon app

_Applies to the Kiestoon app for Android and iPhone, published by RHIS. Last updated: 27 November 2026._

The Kiestoon app is the mobile client of a **Kiestoon phone system**. That phone system is run by your employer
or their IT partner (the "organisation"), not by RHIS. The app connects straight to the phone system of your
organisation; RHIS does not run that phone system and cannot see your calls, contacts, call history or voicemail.

## What the app processes, and where it goes

| Data | Why | Where it goes |
| --- | --- | --- |
| **Address of the phone system** and the **sign-in session** | To connect to your phone system | Stored on your phone only |
| **Microphone audio** during a call | To make the call | Straight to the phone system of your organisation, encrypted (DTLS-SRTP). Never to RHIS. The microphone is only used during a call |
| **Push token** of your phone (from Google Firebase Cloud Messaging or Apple) and the **name of the phone** (for example "Pixel 9") | To ring the app for incoming calls when it is closed, and to show the phone under My devices | To the phone system of your organisation. The push token is sealed by the RHIS push service, see below |
| Your **colleagues, phone book, call history, queues and voicemail** | To show them in the app | Loaded from the phone system of your organisation and shown in the app. Not stored by RHIS |

The phone system of your organisation keeps its own records (for example the call history). For those, your
organisation is responsible; ask them for their privacy policy.

## The push service of RHIS

Google and Apple only deliver notifications for an app on behalf of its publisher, so the phone system of your
organisation sends wake-up notifications through the push service of RHIS (`push.rhis.nl`):

- It **stores nothing**: the push token is turned into a sealed handle that only the push service can open,
  and the handle is kept by the phone system of your organisation, not by RHIS.
- It **cannot read** the notifications: who is calling and the voicemail summary are encrypted with a key that
  only the phone system and your phone know.
- It **logs no phone numbers, tokens or contents**.

The notification itself is delivered by Google (Firebase Cloud Messaging) or Apple (Apple Push Notification service).

## What the app does not do

- No advertising, no tracking, no analytics and no crash reporting.
- No access to the contacts, photos, location or files on your phone.
- No selling or sharing of data with third parties.

## Permissions

| Permission | Why |
| --- | --- |
| Microphone | So the other side hears you during a call |
| Notifications | So the app can ring for incoming calls and show new voicemail |
| Full-screen notification (Android) | To show an incoming call over the lock screen, like the phone's own call screen |
| Foreground service for phone calls and the microphone (Android) | To keep a call going when you switch apps or lock the screen |

## Your choices

- **Unlink** the phone under My devices in the Kiestoon portal (or ask your administrator). The phone system then
  forgets the push token at once.
- **Remove the app** from your phone: everything the app stored on the phone is removed with it.
- For data that the phone system of your organisation keeps, contact your organisation.

There are no Kiestoon accounts in the app: your account is made and removed by your organisation.

## Security

All connections use HTTPS and encrypted audio. The software of the app and the push service is open source:
[github.com/rhisijmuiden/Kiestoon](https://github.com/rhisijmuiden/Kiestoon).

## Contact

Questions about this policy: use the contact details of RHIS on the Google Play or App Store page of the app, or
open an issue on [GitHub](https://github.com/rhisijmuiden/Kiestoon/issues). Changes to this policy are published on
this page.
