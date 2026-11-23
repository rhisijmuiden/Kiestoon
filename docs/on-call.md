# On-call

An **on-call team** is an internal number that rings the engineer who is on call at that moment, on their own mobile
number. Kiestoon works out who that is from the schedule: you do not change any routing by hand when the duty
changes. When the engineer does not answer, the call moves down the **escalation** steps: the backup, the team lead,
everyone, a fixed number, and finally a destination such as a voicemail box. Numbers are in the ring group range.

Engineers see their shifts under **My on-call shifts** in the portal and can swap or hand over shifts with
colleagues there. A swap goes around the regular schedule for that period only.

![On-call overview](shot:on-call)

## Create an on-call team

1. Open **On-call** and click **New on-call team**. Give the team a name and a number.
2. On the **Schedule** tab, choose how the duty rotates:
    - **Weekly rotation**: the engineers take turns per week. Set the **hand-over day** and **time** (for example
      Monday 08:00) and the **start date**: the first engineer in the list is on call from the hand-over on or
      before that date.
    - **Daily rotation**: the same, per day.
    - **Planned shifts**: no rotation; you plan every shift on the overview with **Temporary override**.
3. Add the **engineers** in the order of the rotation. Each engineer is called on the mobile number in their own
   profile. A missing number shows a warning; click the number to change it.
4. **Backup (2nd line)**: the next engineer in the rotation, a fixed person, or nobody.
5. **Team lead**: called in the team lead step, gets mails about gaps in the schedule and approves swaps.
6. **Swaps and hand-overs**: approved right away once the colleague accepts, or after approval by the team lead or
   an administrator.
7. **Reminders**: engineers get a mail the day before their shift starts.

![On-call schedule](shot:on-call-schedule)

To have a caller reach the on-call team, pick **On-call team** as the destination of an
[inbound route](routes.md), a [time condition](time-conditions.md) (for example outside office hours) or a key in
an [IVR menu](ivr.md).

## Escalation

The **Escalation** tab lists the steps that run from top to bottom until someone answers. Per step you choose who is
called and for how many seconds:

| Step | Who is called |
|---|---|
| Engineer on duty (1st line) | The engineer on call according to the schedule, swaps included |
| Backup (2nd line) | The backup at this moment |
| Team lead | The team lead of the team |
| Everyone | All engineers of the team at once |
| A user | One fixed person |
| A number | A fixed phone number, for example an external service desk |

A step without anyone to call (no backup, or a gap in the schedule) is skipped. **Repeat steps** runs the whole list
again, up to five times. After the last step the call goes to **If nobody answers**.

- **Engineer presses 1 to accept**: the engineer hears that this is an on-call call and presses 1. A mobile voicemail
  that picks up can then never take the call; without the 1 the next step follows.
- **Also ring the extension and app**: the engineer's own phone, softphone and Kiestoon app ring next to the mobile.
- **Music while waiting**: the caller hears music on hold instead of ringing.
- **Greeting for the caller**: a [prompt](prompts.md) played first, such as "You are being connected to the engineer
  on duty".
- **Caller ID prefix**: put in front of the caller's name, so engineers see that it is an on-call call.

![On-call escalation](shot:on-call-escalation)

Every on-call call shows in the [call journey](call-journey.md): which step rang whom and who accepted.

## Temporary override

On the overview, **Temporary override** puts someone on call for a while, for example when an engineer is ill. An
override beats the regular schedule and the swaps made before it. The **Swaps and changes** tab lists all overrides,
swaps and hand-overs; you can cancel them there.

The overview also shows **Attention** points: gaps in the next 14 days where nobody is on call, and engineers without
a mobile number. The team lead gets a mail about gaps.

## Calendar link

Under **Schedule**, **Calendar link (iCal)** gives a link you subscribe to in Outlook, Google or Apple Calendar.
Anyone with the link can read the schedule; **New link** stops the old one. Engineers find the same link on their
own page.

## My on-call shifts

Engineers open **My on-call shifts** in the portal. They see:

- their **mobile number**, which they can change themselves;
- whether they are on call now, and their **upcoming shifts**;
- the schedule of the next 14 days.

**Swap** next to a shift asks a colleague to take it:

- **Hand over**: the colleague takes your shift (or part of it, from a time until a time).
- **Swap**: the colleague takes your shift and you take one of theirs in return.

The colleague accepts or declines in their own portal and gets a mail. Depending on the team's setting, the team lead
then approves. Until then the regular schedule applies. **Take over now** puts yourself on call right away, for
example when the engineer on duty asked you by phone.

![My on-call shifts](shot:my-on-call)
