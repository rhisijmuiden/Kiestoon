# Feature codes

Dial these codes on any phone or softphone. They work the same on every brand, and the portal shows the result
right away (for example the do-not-disturb state). The list is also on the **Call features & fraud** page.

| Code | What it does |
| --- | --- |
| `*97` | Listen to your voicemail |
| `*98` | Listen to any mailbox (asks for the number and PIN) |
| `*43` | Echo test: hear yourself back, to check audio |
| `*72<number>` | Forward all calls to a number |
| `*73` | Stop forwarding |
| `*76` | Do not disturb on or off |
| `*45` | Sign in or out of all your queues |
| `*45<queue>` | Sign in or out of one queue, for example `*45700` |
| `*8` | Pick up a call ringing in your pickup group |
| `**<extension>` | Pick up a call ringing at that extension, for example `**203` |
| `70` | Park the current call (transfer it to 70); you hear the parking slot |
| `71` … `79` | Pick up a parked call from that slot |
| `*80<extension>` | Intercom: the other phone answers on the speaker |
| `*1` | Start or stop recording during a call (when recording is "on request") |
| `*28<id>` | Switch a time condition between day and night (the id is shown on the time condition) |
| `*2` | Attended transfer during a call |
| `##` | Blind transfer during a call |
| `112`, `911` | Emergency, always allowed |

!!! note "Parking needs the 7 range"
    Call parking uses `70` and `71`-`79`. That is why queues start at 700 in the 3-digit plan, and a linked system
    can not use the prefix 7.

Intercom (`*80` or an intercom key) only reaches known extensions that are free and not on do not disturb. How a
phone answers (automatically, with a warning tone, muted, or also during a call) is set per phone, see
[Phone keys and modules](phone-keys.md#intercom).
