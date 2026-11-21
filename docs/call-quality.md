# Call quality

**Call quality** (Telephony menu) shows how calls sound: MOS, jitter, packet loss and round trip, per call, per trunk
and per phone. When a trunk or phone goes wrong, Kiestoon mails you, and you see on which side of the connection the
problem is.

![Call quality](shot:call-quality)

## Where the figures come from

During a call, Asterisk exchanges an RTCP report about every 5 seconds with each phone and each provider. Kiestoon
reads both directions of every call leg:

- **Received by Kiestoon**: the audio Kiestoon gets from the phone or provider, measured by Kiestoon itself.
- **Reported by the phone / provider**: the audio Kiestoon sends, as the other side reports it arrived.

From jitter, packet loss, round trip and codec, Kiestoon estimates the **MOS** (mean opinion score) with the ITU
E-model, on a scale of 1 to 4.5. A normal phone call scores about 4.4. The MOS of a call leg is its worst direction,
and the MOS of a call is its worst leg. Nothing has to be set up: every call is measured.

| MOS | Rating |
| --- | --- |
| 4.0 and up | Good |
| 3.6 to 4.0 | Fair: people notice it, but can talk |
| under 3.6 | Poor: words drop out, people repeat themselves |

## The page

- **Alerts** at the top: trunks and phones that are over their limit right now.
- **Average MOS**, **Poor calls**, **Weakest trunk** and **Active alerts** for the chosen period (last 24 hours, 7 days
  or 30 days).
- **Live calls**: the calls going on with their current MOS, jitter, loss and round trip, updated every 5 seconds.
  Click a call to open its [call journey](call-journey.md).
- **MOS by trunk**: a line per trunk over the period; the shaded band is the poor range.
- **Phones with the weakest quality**: the ten phones with the lowest MOS and where it goes wrong, for example
  "Loss towards the phone (Wi-Fi or local network)".
- **Trunks**: per trunk the number of calls, average MOS, share of poor calls, jitter and loss in both directions
  (received by Kiestoon / reported by the provider), round trip and the trend.

Values over a limit are red. The **Where it goes wrong** column reads the direction: loss on the audio *towards* a
phone points at the network of that phone; loss on the audio *from* a provider points at the provider or your
internet line.

## In the call history

The [call history](history.md) has a **MOS** column, and the [call journey](call-journey.md) of a call shows a
**Call quality** card: a short diagnosis, every leg with both directions, the round trip, and a strip with the MOS of
each report interval, so you see when during the call it went wrong. The dashboard shows the MOS of the calls going
on and the active alerts.

## Alerts

Under **Call quality › Settings** you set the limits and when you get a mail.

![Call quality settings](shot:call-quality-settings)

- **Limits**: good from and poor under (MOS), and the limits for jitter, packet loss and round trip.
- **Trunk with poor quality**: the average MOS of the last 15 minutes is under the poor limit, with at least 3 calls.
- **Phone with poor calls**: at least 3 poor calls on one extension in the last 60 minutes.
- **Every poor call**: a mail for each call under the poor limit. Busy installations get many mails.
- **Back to normal**: a mail when the trunk or phone is back within its limits.
- **Mail to**, the **Quiet period** before the same alert is mailed again, and how long measurements are kept
  (90 days by default). **Send test mail** sends an example alert.

Alerts use the [mail server](mail.md) of Kiestoon and are written to the audit log.

## Prometheus

With [metrics](api.md) switched on, `/metrics` also has, over the last 15 minutes:

| Metric | What |
| --- | --- |
| `kiestoon_call_mos` | Average MOS of the call legs |
| `kiestoon_call_poor_total` | Calls under the poor limit |
| `kiestoon_trunk_mos{trunk}` | Average MOS per trunk |
| `kiestoon_trunk_jitter_ms{trunk}`, `kiestoon_trunk_packet_loss_percent{trunk}`, `kiestoon_trunk_rtt_ms{trunk}` | Jitter, loss and round trip per trunk |
| `kiestoon_active_call_mos_min` | Lowest current MOS of the calls going on |
| `kiestoon_quality_alerts_active` | Active call quality alerts |

## Good to know

- Phones and providers that send no RTCP reports only get the direction Kiestoon measures itself.
- A call that is still going on shows its figures so far.
- The MOS is an estimate from network figures, not a recording of what people heard. It is a reliable guide to
  where problems are, and how bad they are.
