# First setup

This guide takes a fresh Kiestoon from the first sign-in to the first real phone call. Plan about half an hour. Each
step links to the full chapter.

## 1. Sign in for the first time

Open the URL from the installer or the console (`http://<server>:8080`) and sign in with the user name `admin` and the
generated password (shown again with `sudo kiestoon initial-admin`). The portal then asks for:

- **Language**. Every user picks their own language later.
- **Organization name**: shown in the portal and later on phone displays.
- **Your name and your own password** (at least 12 characters; a sentence of a few words works well). The generated
  password stops working.

Administrators always use **two-factor authentication**. Right after choosing your password, scan the QR code with an
authenticator app (Microsoft Authenticator, Google Authenticator, 1Password …), enter the 6-digit code and save the
**recovery codes**. Each recovery code works once, if you lose your phone.

!!! tip "Joining as a standby node"
    The first screen also has *Join an existing Kiestoon as standby node*. Use that only for the second machine of a
    [high-availability pair](high-availability.md).

## 2. Basic settings

Open **Settings** (bottom of the left menu).

![Settings: number plan, emergency calls and network](shot:settings)

1. **Number plan**: internal numbers have 3 digits by default (extensions 200-599, ring groups 600-699, queues
   700-799, conference rooms 800-899). Click *Change number length* for 4 or 5 digits; existing extensions are
   renumbered for you.
2. **Calls**: language of the prompts (English or Dutch, also used for voicemail e-mails), time zone and country
   code (`31` for the Netherlands).
3. **Emergency calls**: 112 and 911 always work from every extension. Set the *caller ID for emergency calls* to a
   number that can be called back and is tied to your address.
4. **Network and NAT**: when the server is behind NAT and phones connect from outside, fill in the *public address*
   and the *local networks*.

See [Settings](settings.md) for every option.

## 3. Mail server

Voicemail to e-mail, welcome e-mails, fax and alerts need a mail server. Open **Mail server**, fill in your SMTP
server (or sign in with Microsoft 365 / Google OAuth2) and send a test mail. See [Mail server](mail.md).

## 4. Connect your telecom provider

Open **Trunks & routes**:

1. **Trunks › Add trunk**: pick your provider template (for example [Voys](providers/voys.md) or
   [Cheapconnect](providers/cheapconnect.md)) and fill in the account details. The status turns *Registered*.
2. **Inbound routes**: send each phone number somewhere: an extension, a ring group, a queue, an IVR menu or a time
   condition.
3. **Outbound routes**: add a route with the *Dutch numbers* preset and choose the trunk(s).

See [Trunks](trunks.md) and [Inbound and outbound routes](routes.md).

## 5. Add extensions

Open **Extensions › Add extension**. Kiestoon picks the next free number and generates a strong SIP password. Fill
in the e-mail address and keep *Create a portal account and send a welcome e-mail* on: the employee gets a link to
set a password, the portal address and the details for a softphone app. See [Extensions](extensions.md).

## 6. Connect phones

Open **Phones & provisioning**:

1. Turn **provisioning on** and copy the provisioning URL.
2. Put that URL in DHCP option 66 of your router, or enter it in the phone's web interface.
3. Plug in the phones. They show up under *Waiting for an extension*: pick an extension and click **Link**.

See [Phones and provisioning](phones.md) and [Phone keys and modules](phone-keys.md).

## 7. Secure and back up

- Switch on the **firewall** with the SIP allow list of your provider and fail2ban. Safe mode rolls back a change
  if you lose access. See [Firewall](firewall.md).
- Add a **backup** location (SFTP, FTPS, S3, OneDrive …) and a schedule, and write down the encryption key. See
  [Backups](backups.md).
- Check **Info & updates** (bottom of the left menu) for the channel and the latest version. See
  [Updates](updates.md).

## Checklist

- [ ] Administrator with MFA and saved recovery codes
- [ ] Number plan, time zone, prompt language, emergency caller ID
- [ ] Mail server tested
- [ ] Trunk registered, inbound and outbound route
- [ ] Extensions with welcome e-mails
- [ ] Phones linked and calling
- [ ] Firewall on, backups scheduled, encryption key stored safely
