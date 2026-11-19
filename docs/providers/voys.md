# Provider: Voys

Kiestoon has a ready-made template for [Voys](https://www.voys.nl). Voys runs on the
VoIPGRID platform and calls a SIP trunk to your own phone system a **VoIP trunk**.

## What the template sets

| Setting | Value | Why |
|---|---|---|
| Authentication | Registration | Voys gives an Account ID and password per VoIP trunk |
| Host | `ha.voys.nl` | Voys' high-availability SIP domain |
| Port | `0` (DNS SRV) | `ha.voys.nl` publishes SRV records: UDP 6060 first, 5060 as fallback, on two SIP proxies (Amsterdam and Groningen). Port 0 makes Asterisk follow them, so a failing proxy is skipped |
| Transport | UDP | Voys supports UDP and TCP; no TLS/SRTP for VoIP trunks |
| Codecs | `alaw,g722` | Voys: preferred codecs G.711 A-law and G.722 |
| Provider IP addresses | `195.35.114.0/23`, `185.103.76.0/22` | Voys sends SIP and media from these ranges. Kiestoon accepts calls from them and opens the firewall for them |
| Called number from | Request URI | Voys: "Use destination URL from: Request line"; numbers arrive as `+31201234567` |
| Caller ID | `P-Asserted-Identity` and `Remote-Party-ID` | Kiestoon sends both; set the CLI header in Freedom to Remote-Party-ID |
| DTMF | RFC 4733 | Standard |

Inbound INVITEs from Voys are not challenged for a password (the "insecure=invite" of old
`sip.conf` guides); they are recognised by source address instead.

## 1. In Voys Freedom

1. Sign in at [freedom.voys.nl](https://freedom.voys.nl) and open **Admin → VoIP trunks**
   (direct: `freedom.voys.nl/redirect/TrunkAccount`). Click **Add**.
2. **Description**: for example `Kiestoon`. Voys also uses it as the outgoing name.
3. **Number of lines**: the maximum number of simultaneous calls.
4. **Internal number**: a free three-digit number, for example `900`.
5. **Call permissions**: allow only the destinations you need (for example the Netherlands
   and Europe). This limits damage if the system is ever misused.
6. Turn on **Use the caller ID of the trunk**, so the number Kiestoon sends is shown.
   Voys only accepts numbers that are assigned to you. With this option on, it overrides
   "Suppress outgoing number".
7. **Save**. Voys generates the **Account ID** and **password**. You need both in Kiestoon.
8. Open **Advanced (SIP) settings** and set the **CLI header** to `Remote-Party-ID`.
   Leave the static hostname empty: Kiestoon registers itself.
9. Optional, more secure: under **Extra security**, allow only the public IP address of
   your Kiestoon server.
10. Open the call flow (dial plan) of each Voys number and send incoming calls to this
    VoIP trunk.

## 2. In Kiestoon

1. Go to **Trunks & routes → Trunks → Add trunk** and choose provider **Voys**. The
   step-by-step guide is shown in the form as well.
2. **Username**: the Account ID. **Password**: the trunk password. Leave the
   authentication username empty.
3. **Caller ID**: one of your Voys numbers, `0201234567` or `+31201234567`.
4. **Maximum simultaneous calls**: the number of lines from Freedom.
5. Save. Within a minute the status changes to **Registered**.
6. **Inbound routes**: add a route per Voys number. Voys sends `+31201234567`;
   Kiestoon also matches `0201234567` and `31201234567`.
7. **Outbound routes**: add a route with the Dutch numbers preset and the Voys trunk.
8. Behind NAT? Fill in the public address under **Settings → Network and NAT** and
   forward the SIP port and the RTP range to the server. The Voys ranges are allowed in
   the Kiestoon firewall automatically.

Anonymous calls: Voys hides your number when you dial `31*` before it. Add an outbound
pattern with that prefix if you want this.

## Troubleshooting

| Symptom | Fix |
|---|---|
| Status **Rejected** | Wrong Account ID or password. Copy the password again from Freedom |
| Status stays **Not registered** | Your network may block DNS SRV or port 6060. Set host `sipproxy.voipgrid.nl` and port `5060` |
| Incoming calls reach the wrong route | Set **Called number comes from** to **To header** |
| One-way or no audio | NAT: check the public address and the RTP port forward |
| Wrong number shown | Turn on "Use the caller ID of the trunk" in Freedom; the number must be yours |

## Voys terms for your own PBX

- Voys supports connecting a PBX through a VoIP trunk, but **does not support configuring
  the PBX itself**.
- A fraud detection system deactivates a trunk on suspicious traffic. Keep call
  permissions tight and use the Kiestoon fraud limits.
- Voys supports every SIP compatible PBX.

Sources: [VoIP-trunk: Mijn eigen PBX instellen](https://help.voys.nl/voip-trunk-configuratie),
[Asterisk](https://help.voys.nl/asterisk) (Voys help centre), and the DNS SRV records of
`ha.voys.nl` (checked November 2026).

