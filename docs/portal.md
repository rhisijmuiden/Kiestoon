# User portal

Every employee with a [user account](users.md) linked to an extension gets the **user portal**. It is the same
address as the admin portal: Kiestoon shows the menu that fits the role. Employees manage their own phone here,
without bothering the administrator.

| Page | What you do there |
| --- | --- |
| **My phone** | Do not disturb, forwarding, follow me, your own hours |
| **Voicemail** | Listen to messages, record greetings, change the PIN |
| **Queues & groups** | Sign in and out of queues, see your ring groups |
| **Colleagues** | See who is available, call, and transfer calls by drag and drop |
| **My calls** | Your own call history, call back with one click |
| **Softphone & devices** | Browser softphone, phone app with QR code, keys of your desk phone |

## My phone

![My phone](shot:my-phone)

| Setting | What it does |
| --- | --- |
| **Do not disturb** | Callers hear busy or go to your voicemail. Also `*76` on the phone. |
| **Forward to my mobile** | One click: every call goes to your mobile until you press **Stop**. |
| **Always forward to** | Your phone does not ring at all; calls go straight to this number. |
| **When busy, forward to** | When you are already on a call. |
| **When not answered, forward to** | After your ring time. Replaces voicemail. |

A forward can go to a colleague, a ring group or an external number. External forwards use an outbound route, so
the [fraud protection](firewall.md#fraud-protection) limits still apply.

### Follow me

With **Also ring other numbers** your own phone rings first, then the numbers you add ring as well, for example your
mobile and your home office phone.

- **Ring them after (seconds)**: `0` rings everything at once.
- **Ask to press 1**: the person who answers has to press 1 first. This stops the voicemail of a mobile from taking
  the call.

### My hours

Pick the [time condition](time-conditions.md) you follow under **Follow the hours of**, for example "Office hours".
Outside those hours your calls go to **Outside these hours, forward to**, or to your voicemail when that is empty.
**Always available** switches it off.

## Voicemail

See [Voicemail](voicemail.md): messages, greetings and the PIN are all managed on this page.

## Queues & groups

![Queues & groups](shot:my-queues)

Sign in to the [queues](queues.md) you are a member of, one by one or with **Sign in to all** / **Sign out of
all**. Signed in means you receive callers from that queue. Fixed members are always signed in. On the desk phone
`*45` does the same. Under **My ring groups** you see the [ring groups](ring-groups.md) you are in and who in them
is available.

## Colleagues and drag-and-drop transfer

![Colleagues with live status](shot:colleagues)

The colleagues page shows every extension, ring group and queue with its live status: available, ringing, in a
call or do not disturb. Click a colleague to call them. **Call a number** dials any number; with the softphone on
the call starts in the browser, otherwise your desk phone rings first and sets up the call when you pick it up.

**Transferring a call** works the same for every employee:

1. Answer the call on your desk phone or the softphone. It appears under **My active calls**.
2. Choose **Transfer directly** (blind) or **Ask first** (attended).
3. Drag the call onto a colleague, ring group or queue. Or press **Transfer** and pick a target; `Esc` cancels.
4. With **Ask first** the caller waits on hold while you talk to the colleague. Then press **Complete transfer**,
   or **Back to caller** when the colleague cannot take it.

!!! tip "Reception"
    The [switchboard](switchboard.md) works the same way, but shows every live call of the office instead of only
    your own.

## My calls

![My calls](shot:my-calls)

Your own incoming, outgoing and missed calls, with **Call back** on every row. Administrators see all calls under
[Call history](history.md).

## Softphone & devices

![Softphone & devices](shot:devices)

**Softphone in the browser.** Press **Turn on softphone** and call and answer with any headset, right in the
browser. The browser asks for the microphone once. The softphone keeps running while you use the rest of the portal
and docks at the bottom of the screen. It needs **https**: open the portal through its [internet
address](internet.md) or your [own reverse proxy](reverse-proxy.md). The administrator can switch the softphone off
per extension.

**Phone app.** **Show my settings** shows the SIP account and a QR code for an app such as Linphone, Zoiper or
Groundwire. Scan the code in the app and it is set up.

**My desk phones.** Phones that are [provisioned](phones.md) for your extension appear here. Employees can set their
own [keys](phone-keys.md), for example a speed dial or a busy lamp field for a colleague. The phone picks up the new
keys at its next restart or provisioning check.

**Windows.** For a full softphone in the taskbar, see the [Windows softphone](softphone.md).
