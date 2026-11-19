# Hot desks

A **hot desk** is a desk phone that anyone can use as their own: they log in with their extension number and PIN,
and the phone loads their account. Calls to their extension ring there, and calls they make show their number.

## Setting up a hot desk

1. Create an extension for the desk, for example **290 Hot desk 1**. This is the guest account the phone uses while
   nobody is logged in. You can give it limited outgoing rights.
2. Go to **Phones & provisioning**, click **Edit** on the phone and choose that extension.
3. Under **Hot desk**, switch on **This phone is a hot desk**. Switch on **Log out every night** if desks should be
   free every morning (everyone is logged out at 03:00).

Hot desking works on every provisioned desk phone. It has been built for Yealink.

## Logging in and out

| Code | What it does |
| --- | --- |
| `*55` | Log in: enter your extension number and `#`, then your PIN and `#` |
| `*56` | Log out: the phone goes back to its guest account |

The PIN is the **voicemail PIN** of the extension. An extension without a PIN cannot log in.

After you log in or out, the phone reloads its settings within a few seconds. You can put `*55` and `*56` on a
**speed dial** key to make it one press.

- An extension is logged in on **one hot desk at a time**: logging in on another desk logs it out on the first one.
- The keys of the phone stay the same; the account on line 1 changes.
- Your own desk phone and the softphone keep working. If the extension allows only one device at a time
  (**Devices at the same time** 1), the hot desk takes over from the other device.
- After 5 wrong PINs, the desk is locked for 10 minutes.

## In the admin console

The phone list shows who is logged in on each hot desk. Click the **log out** icon to free a desk. Every login and
logout is in the audit log.
