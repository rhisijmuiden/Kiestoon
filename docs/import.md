# Import from another phone system

**System › Import** takes over the configuration of your old phone system: extensions, ring
groups, queues, voice menus with their recordings, opening hours, trunks and phone numbers.
You upload a backup or the configuration files, check a preview, tick what you want and
Kiestoon creates it. Nothing changes in Kiestoon until you press **Import**.

![Import](shot:import)

| System | What you upload |
|---|---|
| [UniFi Talk](import/unifi-talk.md) | The Talk backup (`.unifi`) from UniFi OS |
| [Asterisk](#asterisk) | `/etc/asterisk` as a `.tar.gz` or zip, or the loose `.conf` files |
| [FreePBX, Issabel, PBXact](#freepbx-issabel-pbxact) | A FreePBX backup (`.tar.gz` or `.tgz`) or a mysqldump of the `asterisk` database |
| [FreeSWITCH and FusionPBX](#freeswitch-and-fusionpbx) | `/etc/freeswitch` as an archive, or a FusionPBX backup with its database dump |
| [3CX](#3cx) | A 3CX backup (`.zip`) without a password, or the extension list exported as CSV |

## The steps

1. **Upload.** Pick the system (or leave it on *Detect automatically*) and drop the files. You
   can upload up to 50 loose files at once, or one archive of up to 2 GB.
2. **Preview.** Kiestoon shows everything it found, grouped by kind, with a free number in
   your number plan for each item. The old number is kept when it fits your
   [number plan](concepts.md#the-number-plan). You can change every number and untick what you do not want.
   Voice menus are shown as a tree with their keys, and every recording can be played.
   Remarks about things that could not be taken over are listed under **Notes**.
3. **Import.** The recordings are converted to telephone quality (8 kHz) in your browser and
   saved as prompts. Then the items are created: first the extensions, ring groups, queues,
   trunks and opening hours, then the voice menus and inbound routes that point at them.

When a destination was not ticked, the call hangs up at that point and the preview says so.
Things that already exist in Kiestoon with the same name are reused, so you can run the
import again without duplicates. The uploaded files are kept for at most two hours and are
removed when you close the import.

!!! warning "Backups contain passwords"
    A backup of a phone system holds SIP passwords of phones and trunks. Kiestoon keeps a SIP
    password when it is strong enough, so a phone only needs the new server address. Weak
    passwords are replaced by a new one, which the preview shows.

## Asterisk

Upload the folder `/etc/asterisk` as a `.tar.gz` or zip. To bring the recordings along, add
`/var/lib/asterisk/sounds/custom` (or wherever your recordings are) to the same archive, or
upload them as loose files next to the configuration.

```bash
tar czf asterisk-config.tar.gz /etc/asterisk /var/lib/asterisk/sounds/custom
```

Kiestoon reads:

| From Asterisk | Becomes in Kiestoon |
|---|---|
| `pjsip.conf` endpoints (also the pjsip wizard) and `sip.conf` peers, `users.conf` | Extensions with their SIP password |
| `pjsip.conf` registrations and endpoints with a remote host, `sip.conf` peers with `register =>` | Trunks |
| `voicemail.conf` | Voicemail on/off, PIN and e-mail per extension |
| `queues.conf` | Queues with their members and settings |
| `extensions.conf`: `Dial` to several phones | Ring groups |
| `extensions.conf`: `Background` + `WaitExten` with keys | Voice menus with their recordings |
| `extensions.conf`: `GotoIfTime` | Time conditions |
| `extensions.conf`: numbers in the context of the trunk | Inbound routes |

Templates (`[name](!)`), `#include` files and `(+)` additions are followed. Dialplan steps
that Kiestoon has no equivalent for (AGI scripts, database lookups, custom macros) are named in
the notes. Recordings in `.wav`, `.ulaw`, `.alaw`, `.sln` and `.gsm` are recognised; GSM
cannot be converted in the browser, so upload those again as WAV or MP3 under **Prompts**.

## FreePBX, Issabel, PBXact

In FreePBX 15, 16 or 17, go to **Admin › Backup & Restore**, make a backup with all modules
and download the `.tar.gz`. Backups of FreePBX 2.x to 14 (`.tgz`) and of Issabel or Elastix
work as well; they are read from the database dump inside. A loose mysqldump of the
`asterisk` database also works:

```bash
mysqldump asterisk > asterisk.sql
```

Kiestoon reads the extensions (with voicemail settings), ring groups, queues, IVRs with their
system recordings, announcements, time conditions with their time groups, misc destinations,
trunks and inbound routes. Feature codes are not imported, because Kiestoon has its own
(see [Feature codes](feature-codes.md)).

## FreeSWITCH and FusionPBX

For **FreeSWITCH**, upload `/etc/freeswitch` as an archive. Kiestoon reads the users and
groups of the directory, the gateways, the callcenter queues, the IVR menus with their sound
files and the dialplan: the `public` context gives the inbound numbers, conditions on
`wday`/`time-of-day` become time conditions. Add the recordings folder (for example
`/usr/share/freeswitch/sounds/custom`) to the archive to bring them along.

For **FusionPBX**, upload a FusionPBX backup, or a database dump next to the recordings:

```bash
pg_dump -U fusionpbx fusionpbx > fusionpbx.sql
tar czf fusionpbx.tar.gz fusionpbx.sql /var/lib/freeswitch/recordings
```

Both the plain (`.sql`) and the custom format (`pg_dump -Fc`) are read. Kiestoon takes the
extensions, voicemail, ring groups, call center queues, IVR menus, gateways, destinations
(inbound numbers) and recordings of the first domain. FusionPBX time conditions are not
imported; set them up again on the [Time conditions](time-conditions.md) page.

## 3CX

In 3CX, go to **Admin › Backup**, make a backup **without a password** and download the
`.zip`. Do not unpack and re-pack it. 3CX encrypts the configuration of a backup with a
password, and only 3CX itself can read that; Kiestoon then asks for a backup without one.

3CX does not publish the layout of its backups, so Kiestoon looks for the extensions, ring
groups, queues, digital receptionists, trunks and inbound rules by name, and takes the
recordings from the `httpprompts` folder. Check the preview carefully before you import.

The extension list from **Users › Export** (a `.csv` file) can be uploaded as well, on its
own or next to the backup. Since 3CX 20 that export no longer contains the SIP passwords, so
the extensions get a new password.

## After the import

- Check the voice menus in the [IVR builder](ivr.md) and make a test call.
- Add the phones on the **Phones** page; phones are not imported, because Kiestoon
  provisions its own.
- Move the phone numbers to the new trunk at your provider.
