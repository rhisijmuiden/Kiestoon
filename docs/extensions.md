# Extensions

An **extension** is an internal number for one person or one phone, with a SIP account, a voicemail box and
personal settings. Every desk phone, softphone and browser tab of that person rings on it.

![Extensions with their status, voicemail and transport](shot:extensions)

## Add an extension

1. Open **Extensions** and click **Add extension**. Kiestoon fills in the next free number from the
   [number plan](concepts.md#the-number-plan).
2. Fill in the **name** and the **e-mail** address.
3. Keep **Create a portal account and send a welcome e-mail** on. The employee receives (in English or Dutch):
    - a one-time link to set a password (valid for 72 hours; no password is ever sent by e-mail),
    - the portal addresses, their username and extension,
    - the SIP details for a mobile softphone app, and how voicemail works,
    - a note when single sign-on is enabled.
4. Click **Save**. The SIP password is generated (24 characters) and the phone configuration is rebuilt.

Without a mail server the account is still created and you get the set-password link to hand over yourself. You
can send the welcome e-mail again later with the **Send welcome e-mail** button on the row.

![Editing an extension](shot:extension-edit)

## Settings per extension

| Setting | What it does |
| --- | --- |
| **Transport** | UDP (default), TCP or TLS. With TLS you can also **encrypt audio (SRTP)** |
| **Devices at the same time** | How many devices may register at once, for example desk phone, softphone and mobile app (default 3) |
| **Outbound caller ID** | The number others see. Empty: the outbound route or trunk decides |
| **Ring time** | Seconds before the call goes to voicemail or the no-answer forward (default 25) |
| **Voicemail** | On or off, with a **PIN** (4-8 digits, generated when empty) |
| **Send voicemail to e-mail** | The message arrives as audio attachment; optionally **delete after sending** |
| **Outbound calls** | Internal only, national or international |
| **Allow premium-rate numbers** | 0900, 0906 and similar. Off is safer |
| **Recording** | Never, on request (`*1` during a call) or always. See [recordings](history.md) |
| **Pickup group** | `*8` picks up calls ringing in the same group |
| **Browser softphone** | The user can call with a headset in the [user portal](portal.md) |

## SIP details and phones

The **SIP details** button on a row shows the SIP details (server, port, username, password, voicemail number) for a
softphone or a phone you set up by hand. **New password** makes a new SIP password; phones with the old one stop
working until they get the new one. Provisioned phones get it automatically.

The **Phones of this extension** button on a row shows the desk phones linked to this extension, and lets you link a waiting phone
right there. See [Phones and provisioning](phones.md).

## Status

The **Status** column shows *Online* when at least one device is registered. The dashboard shows the same
information as "Phones online".

## Forwarding, do not disturb and follow-me

These are personal settings. The employee sets them in the [user portal](portal.md) (My phone) or with
[feature codes](feature-codes.md) on the phone: `*72<number>` forwards everything, `*73` stops, `*76` toggles do not
disturb. Administrators see and can change the same settings in the extension form.

## Delete an extension

The bin button deletes the extension and its voicemail box. Linked phones keep working without an account until
you link them to another extension.
