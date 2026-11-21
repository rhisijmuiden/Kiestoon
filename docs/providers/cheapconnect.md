# Provider: Cheapconnect

[Cheapconnect](https://www.cheapconnect.nl) is a Dutch VoIP provider with SIP accounts that work with your own
phone system. Kiestoon has a ready-made **Cheapconnect** template.

## What the template sets

| Setting | Value | Why |
|---|---|---|
| Authentication | Registration | Cheapconnect only routes incoming calls to a registered PBX |
| Host / port | `sip.cheapconnect.net`, `5060` | Cheapconnect's SIP server |
| Transport | UDP | As in Cheapconnect's Asterisk example |
| Codecs | `alaw,ulaw,g722` | Cheapconnect offers G.711 (A-law, µ-law), G.722 and G.729 |
| Called number from | To header | Works with Kiestoon's registration; switch to Request URI if calls land on the wrong route |
| User agent | `Kiestoon PBX` | Cheapconnect reserves the user agent `asterisk`; Kiestoon already sends its own name |

!!! note "Check the details in your Cheapconnect account"
    Always use the exact values shown in your own Cheapconnect dashboard: they win over this page.

## 1. At Cheapconnect

1. Sign in to your Cheapconnect account and open the SIP account (the phone number) you want to use.
2. Note the **SIP username**: the number in international format without `+`, for example `31851234567`.
3. Note the **SIP password**.
4. Optional, more secure: limit the account to the public IP address of your Kiestoon server and set a spending
   limit.

## 2. In Kiestoon

1. **Trunks & routes › Trunks › Add trunk**, provider **Cheapconnect**. The guide is shown in the form as well.
2. **Username**: the number, for example `31851234567`. **Password**: the SIP password.
3. **Caller ID**: the same Cheapconnect number.
4. **T.38 fax** (Advanced): on, if you receive faxes on this number.
5. Save. The status turns **Registered**.
6. Add an [inbound route](../routes.md) for the number and put the trunk in an [outbound route](../routes.md),
   for example as the second trunk so it takes over when your main provider fails.
7. Behind NAT? Fill in the public address under **Settings → Network and NAT** and forward UDP 5060 and the
   RTP range.

## If it does not work

| Symptom | Fix |
| --- | --- |
| Status **Rejected** | Wrong username or password; copy them again from Cheapconnect |
| Status stays **Not registered** | Check that outgoing UDP 5060 is allowed |
| Incoming calls reach the wrong route | Set **Called number comes from** to **Request URI** |
| Calls connect but there is no audio | NAT: set the public address under Settings and forward the RTP range |

Source: [Cheapconnect: SIP Trunk Setup, Asterisk & FreePBX](https://www.cheapconnect.nl/en/business-voip/sip-trunk-setup/).
