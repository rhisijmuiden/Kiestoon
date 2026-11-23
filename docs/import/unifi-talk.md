# Import from UniFi Talk

Kiestoon can take over a UniFi Talk phone system from a Talk backup: the smart attendants
(voice menus) **with their recordings**, the users with their extension, call groups, call
queues, the SIP trunk and the phone numbers. The import runs on the **Import** page; see the
[import overview](../import.md) for the steps every system shares.

## 1. Make the backup in UniFi OS

1. Open the UniFi OS console (UDM, UDM Pro, UCG, Cloud Key or UniFi OS Server) where Talk runs.
2. Go to **Settings → Control Plane → Backups** (older versions: **System → Backups**).
3. Choose **Download** and pick the **Talk** application. You get a file like
   `unifi_os_backup_for_talk_1795438324390_97bfc7f6.unifi`.

A full console backup works too, as long as Talk is in it.

!!! warning "Keep the file safe"
    A UniFi OS backup also holds certificates, private keys and SIP passwords. UniFi encrypts it
    with a key that is the same on every console, so the file is effectively readable by anyone
    who has it. Kiestoon reads only the Talk part, keeps the file in memory for the preview and
    does not store it.

## 2. Upload and check the preview

Upload the `.unifi` file on the **Import** page. Kiestoon recognises it automatically and
shows what it will create:

| From Talk | Becomes in Kiestoon |
|---|---|
| User (extension, name, SIP password, voicemail on/off) | Extension. The SIP password is kept when Kiestoon accepts it, so a phone only needs the new server address and number |
| Call group (members, simultaneous or one by one, no-answer action) | Ring group |
| Call queue (agents, strategy, maximum wait, ring time, wrap-up, overflow to voicemail) | Queue |
| SIP gateway (provider, account, password, numbers) | Trunk, with the matching Kiestoon provider template (for example Voys) |
| Phone number and where it rings | Inbound route |
| Smart attendant with its keys, sub menus and recordings | Voice menu in the [IVR builder](../ivr.md), one recording per step |
| Global voicemail greeting | Prompt |

Every extension, group and queue gets a free number in your Kiestoon number plan. Talk
numbers are kept when they fit (same length, right range); you can change every number in
the preview. Things that already exist in Kiestoon with the same name are reused, so you can
run the import again without getting duplicates.

### How the attendants are converted

- The welcome recording plays first, then the caller chooses a key, as in Talk.
- A key with its own recording plays it and then goes to the user, group, queue or attendant
  it points to.
- A key that opens a sub menu becomes a **Choice menu** step with its own keys.
- "Press 9 to hear the menu again" built in Talk with an extra attendant that forwards back
  goes straight back to the main menu in Kiestoon; the extra attendant is not imported.
- What happens when the caller presses nothing becomes the **No choice** step.
- Recordings (MP3) are converted to telephone quality (8 kHz) in your browser and stored as
  prompts, named after the attendant and the step (for example
  `Talk - IJRB V2 - Zwembad`).

## 3. What is not imported

- **Phones.** Kiestoon provisions its own phones; add them on the **Phones** page. Phones
  that used a SIP account keep working once they point at Kiestoon with the new number and
  the same password.
- **Messages a queue plays** before it sends callers on (for example "nobody is available").
  They are imported as prompts, so you can use them in a voice menu.
- Spoken (text-to-speech) attendant greetings without a recording: add a prompt, for example
  with [text to speech](../prompts.md).
- Voicemail messages, call history, SMS, contacts and Talk-specific settings such as phone
  designs and paging.

After the import, check the voice menus in the IVR builder, make a test call
(**Test call** in the builder) and move the numbers to the new trunk at your provider.
