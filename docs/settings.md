# Settings

**Settings** holds the system-wide choices: the number plan, emergency calls, network and the phone engine.

![Settings](shot:settings)

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
| **Language of prompts** | English or Dutch, also used for voicemail e-mails. |
| **Time zone** | For time conditions, call history and schedules. |
| **Country code** | Without `+`; `31` is the Netherlands. |
| **Codecs for phones** | In order of preference, for example `g722,alaw,ulaw`. |
| **Show numbers in national format** | Shows `+31 20 …` as `020 …`, so calling back works. |

## Restart the phone engine

Some changes take effect after a restart of the phone engine (Asterisk); the portal then shows a banner. **When
idle** restarts as soon as no calls are active. **Restart now** ends every active call.
