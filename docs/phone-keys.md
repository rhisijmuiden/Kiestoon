# Phone keys and modules

Every desk phone has a **visual key editor**: a picture of the phone with its keys left and right of the display,
in pages, plus any expansion modules. Click a key to set it up; drag a key onto another to swap them, also onto
another page or module.

![The key editor of a Yealink T54W with an EXP50 module](shot:phone-keys)

## Open the editor

- Administrators: **Phones & provisioning**, click **Edit** on a phone.
- Employees: in the user portal under **Softphone & devices** they edit the keys of their own phone and modules.

## Key types

| Type | What the key does | Value |
| --- | --- | --- |
| **Line** | A line (account) on the phone | Line number |
| **BLF (colleague)** | Shows whether a colleague is free, ringing or busy; press to call, or to pick up when it rings | Extension |
| **Speed dial** | Calls a number | Phone number |
| **Park call** | Parks the current call; the caller hears music until someone picks it up at a parking slot (71-79) | |
| **Parking slot** | Lights up when a call is parked there; press to pick it up | 71 … 79 |
| **Intercom** | Calls a colleague who answers on the speaker | Extension |
| **Do not disturb** | Switches DND on and off (`*76`), with the lamp showing the state | |
| **Voicemail** | Calls your voicemail (`*97`) | |
| **Empty** | Does nothing | |

Each key also has a **label** for the display.

## Extra lines

Line 1 is the extension of the phone. Desk phones of Yealink, Snom and Grandstream can carry **extra lines**
(accounts 2 and up), for example a shared sales line. Choose the extensions under **Extra lines** and put them on
line keys.

## Expansion modules

Click **Expansion module** and choose the module. All modules on a phone are of the same type.

| Module | Keys | For | Max |
| --- | --- | --- | --- |
| Yealink EXP40 | 40 (2 pages of 20) | T46U, T48U | 6 |
| Yealink EXP43 | 60 (3 pages of 20) | T43U, T46U, T48U | 3 |
| Yealink EXP50 | 60 (3 pages of 20) | T53W, T54W, T57W | 3 |
| Snom D7 | 18 | D717, D735, D785 | 3 |
| Grandstream GBX20 | 40 (2 pages of 20) | GXP2170, GRP2616 | 4 |

Module keys are set up exactly like phone keys. A reception module full of BLF keys shows the whole office at a
glance.

## Key templates

A **key template** is a key layout for one phone model, including its expansion modules. Use it to give all
reception phones, or all phones of a team, the same keys.

1. Set up one phone and click **Save as template**, or open the **Key templates** tab and click **New template**.
2. On the template, click **Apply to phones**, choose the phones of that model and confirm. Their keys and modules
   are replaced.
3. Phones that are online fetch their new keys right away; the others at their next provisioning check.

**Load template…** in the editor of a single phone copies a template onto that phone before you save it.

## Intercom

Intercom (`*80<extension>` or an intercom key) only reaches known extensions that are free and not on do not
disturb. It rings all their devices, including the browser softphone. Per phone you choose how it answers:

| Option | Effect |
| --- | --- |
| **Answer intercom calls automatically** | The phone picks up on the speaker |
| **Play a warning tone first** | A short beep before the caller is heard |
| **Start with the microphone muted** | The callee is only heard after pressing mute |
| **Also during another call** | The intercom call is answered while the user is on the phone |

The phone form only shows the options the brand supports:

| Brand | Options |
| --- | --- |
| Yealink, Snom, Fanvil | All four |
| Poly | Automatic answer and muted microphone |
| Grandstream, Htek, Cisco MPP, Akuvox door stations | Automatic answer on or off |

## When do phones get their keys?

After you save, Kiestoon tells a registered phone to fetch its configuration (check-sync). The keys change within a
few seconds. A phone that is not registered picks the change up at its next provisioning check or after a reboot.
