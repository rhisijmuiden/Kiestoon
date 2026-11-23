# Settings

**Settings** holds the system-wide choices: the caller language, the number plan, emergency calls, network and
the phone engine.

![Settings](shot:settings)

## Caller language { #caller-language }

One setting for everything callers hear and phones show, in English or Dutch:

- queue announcements: the position of the caller ("you are caller number 3") and the hold time,
- voicemail menus and greetings, feature codes and other system messages,
- numbers, dates and times that are spoken,
- the default language of [spoken text](prompts.md) for IVR menus and voice prompts,
- voicemail, fax and other system e-mails,
- new users and their welcome e-mail.

**Phone screens follow the caller language** gives [provisioned phones](phones.md) the same language at their next
configuration update. Turn it off to keep a separate phone language under **Phones & provisioning > Provisioning
settings**.

The language of the web interface is not part of it: every user chooses their own under **Account**. An
administrator can work in English while callers hear Dutch.

A single [queue](queues.md) or [IVR menu](ivr.md) can use another language in its own settings (**Caller
language**), for example an English line for foreign customers. The language chosen during the first setup is
the first caller language.

## Number plan

Kiestoon uses fixed ranges per purpose, shown in the table: extensions, ring groups, call queues, conference rooms,
voicemail and features, and the emergency numbers.

**Change number length** switches between 3, 4 or 5 digit internal numbers. Existing extensions are renumbered
automatically. Provisioned phones pick up their new number by themselves; other
phones must be changed by hand. Kiestoon shows the numbers that cannot be shortened before you switch.

## Emergency calls { #emergency-calls }

112 and 911 always work, from every extension, without a route and even when a trunk limit is reached or the
[fraud protection](firewall.md#fraud-protection) blocks outbound calls.

- **Caller ID for emergency calls**: a number on which you can be called back, tied to your address. Empty: the
  trunk's caller ID.
- **Trunk order for emergency calls**: which trunk is tried first. No order set: every trunk is tried.

## Network and NAT

| Setting | What it does |
| --- | --- |
| **Public address** | Public IP or host name, when this server is behind NAT. Empty: no NAT. |
| **Local networks** | Networks that are not behind NAT, one per line. |
| **SIP port (UDP/TCP)** | Default 5060. |
| **SIP TLS port** | Default 5061. |
| **Audio ports from / to** | RTP range, default 10000–20000. |

Changing a port needs a restart of the phone engine.

## Calls

| Setting | What it does |
| --- | --- |
| **Time zone** | For time conditions, call history and schedules. |
| **Country code** | Without `+`; `31` is the Netherlands. |
| **Codecs for phones** | In order of preference, for example `g722,alaw,ulaw`. |
| **Show numbers in national format** | Shows `+31 20 …` as `020 …`, so calling back works. |

## Restart the phone engine

Some changes take effect after a restart of the phone engine (Asterisk); the portal then shows a banner. **When
idle** restarts as soon as no calls are active. **Restart now** ends every active call.
