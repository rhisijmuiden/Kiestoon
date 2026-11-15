# Call journey

Click any call in **Call history** (Reception menu) to see what happened to it, step by step: the number it came in
on, the time condition, which option the caller chose in the IVR, how long they waited in the queue, whose phones
rang and who missed it, who answered and for how long, transfers, voicemail and who hung up. It works for inbound,
outbound and internal calls.

![Call journey](shot:call-journey)

## What you see

- **The summary** at the top: caller and number called, the result (answered, voicemail, hung up while waiting,
  missed, busy, failed), the total time, the waiting time until someone answered and the talk time.
- **The flow**: one box per step, in the style of the [IVR builder](ivr.md). The lines carry how long the caller spent
  in the previous step. Zoom with the buttons or the mouse wheel; on a wide screen the boxes sit side by side.
- **Steps**: the same journey as a list with the clock time of each step, also handy on a phone.
- **Recordings** of the call, to play right there (see [recordings](history.md#recordings)).

| Step | Shows |
| --- | --- |
| **Incoming call** | Caller, number called, trunk and [inbound route](routes.md) |
| **Call from an extension** / **Outbound route** | Who called, the number dialled, the route and trunk used |
| **Time condition** | Open, closed or holiday at that moment |
| **IVR menu** | Every key pressed, and every time the caller made no choice or an invalid one |
| **Queue** | Position in line, waiting time, each agent that was rung (answered, not answered, busy) and why the caller left the queue (answered, hung up, maximum waiting time, nobody signed in) |
| **Ring group** / **Extension** | Each phone that rang and how it ended; do not disturb and outside the schedule |
| **Talking with** | Who answered and the talk time |
| **Transferred** | Directly or after consulting, by whom and to whom (also when transferred by drag and drop) |
| **Voicemail**, **Forwarded to**, **Parked**, **Picked up by**, **Conference**, **Fax** | Where the call went |
| **Call ended** | Who hung up: the caller, the other party, an extension or the phone system |

![A call that ended in voicemail](shot:call-journey-voicemail)

Employees see the journey of their own calls by clicking a call under **My calls** in the user portal. They only see
calls they took part in, and only their own recordings.

## Older calls

Kiestoon records journeys from version 0.6.0-rc.15 on. For older calls, and for calls whose journey was removed after
the retention period, the page shows a short version built from the call record: who called, which phones rang and
who answered.

## Retention and the GDPR

The journey holds phone numbers and names, so it is personal data. Under **Call features & fraud › Call journey**:

| Setting | |
| --- | --- |
| **Keep call journeys (days)** | Journeys older than this are removed every night. Default 90 days. **0** records no journeys at all (the call history itself stays) |

Pick the shortest period you need, and mention it in your privacy statement.

## How it works

Asterisk reports every step of a call to the portal over the manager interface: Channel Event Logging (CEL) for
channels, answers, bridges, transfers and hang-ups, dial and queue events for the phones that rang, and markers the
generated dialplan adds at each step (route, time condition, IVR key, queue, ring group, voicemail). The portal stores
them per call (the Asterisk *linkedid*) in its own database; nothing is written to log files on disk.
