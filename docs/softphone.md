# Windows softphone

**Kiestoon Softphone** is a desktop app for Windows 10 and 11. It puts the phone in the taskbar: call with your
headset, with your colleagues, phone book and queues at hand, and transfer calls by dragging them onto a colleague.

<div class="grid" markdown>

![Contacts with live status](shot:softphone-contacts){ .kt-phone }

![Queues](shot:softphone-queues){ .kt-phone }

![Calls](shot:softphone-history){ .kt-phone }

![Voicemail](shot:softphone-voicemail){ .kt-phone }

</div>

## What it can do

- **Contacts**: colleagues with live status (available, ringing, in a call, do not disturb), the shared
  [phone book](phonebook.md) and your favourites. Search on name, number or company.
- **Calls**: answer, decline, mute, hold, keypad and transfer. Drag the call onto a colleague to transfer it,
  **Direct** or **Ask first**, just like in the [user portal](portal.md#colleagues-and-drag-and-drop-transfer).
- **Queues**: sign in, sign out or pause per queue, with the number of waiting callers.
- **Voicemail** and **call history** with call back.
- **Headset buttons**: answer, hang up and mute with the buttons of Jabra, Poly/Plantronics, EPOS and other USB
  headsets. Sound works on every headset.
- **Sound**: choose the microphone, the speaker for calls, and where it rings, for example on your speakers while
  you talk on the headset.
- **Windows integration**: tray icon, incoming-call popup, notifications for missed calls and new voicemail,
  `tel:` and `sip:` links open in the softphone, and **Start with Windows**.
- **Updates**: under **Info** the softphone checks for new versions and updates itself, on the **Production** or
  **Testing** channel.

## Install and link

1. Open the user portal and go to **Softphone & devices**.
2. Click **Download for Windows** and run the installer (`.msi`). Every Kiestoon server serves the installer that
   matches its own version, at `/download/kiestoon-softphone.msi`.
3. Click **Link this computer**. The softphone opens and links itself to your account. Did it not open by itself?
   Enter the code shown in the portal in the softphone. The code is valid for 10 minutes.

The softphone can also be set up from the app itself: on the welcome screen enter the **Server address** (and a
**Pairing code** from the portal or the welcome e-mail, if you have one) and click **Connect**. Then sign in with
your username and password, or choose **Sign in with your browser (SSO)** for [single
sign-on](users.md#single-sign-on) and passkeys. Your browser then hands the link to the softphone.

A linked softphone stays signed in for 30 days after its last use, and can only use the user portal functions of
your own extension. Linking switches on the [browser softphone](portal.md#softphone-devices) (WebRTC) for your
extension.

!!! info "Servers with their own certificate"
    When the server has no public certificate yet, the portal shows the certificate fingerprint next to the code.
    The softphone pins exactly that certificate, so the connection stays encrypted and cannot be taken over. With
    [internet access](internet.md) switched on, the Let's Encrypt certificate is used and nothing has to be
    pinned.

## For administrators

- The softphone uses the same WebRTC connection as the browser softphone: the server needs **https** with a
  hostname (see [Internet access](internet.md) or [own reverse proxy](reverse-proxy.md)).
- The MSI can be rolled out with Intune, Group Policy or any other software distribution.
- Each release on GitHub contains the MSI and a `SHA256SUMS` file; the softphone checks the checksum before it
  updates.
