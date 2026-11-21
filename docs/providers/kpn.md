# Provider: KPN

KPN delivers business SIP trunks (KPN EEN Vast / VoIP Connect) as an **IP trunk**: there is no registration,
KPN recognises your PBX by its fixed public IP address. Kiestoon has a ready-made **KPN SIP trunk** template.

## What the template sets

| Setting | Value | Why |
|---|---|---|
| Authentication | IP address | KPN only accepts SIP from the PBX addresses on your order |
| Host / port | `213.162.171.164`, `5060` | KPN's standard SBC |
| Provider IP addresses | `213.162.171.164`, `213.162.171.180` | The second SBC is used with *Verhoogde Beschikbaarheid* (high availability), load-shared |
| Transport | TCP | KPN supports UDP and TCP and prefers TCP |
| Codecs | `alaw` | G.711 A-law (20 ms) is mandatory to the PSTN; G.722/G.729 only work on-net |
| DTMF | RFC 2833 / 4733 | SIP INFO and KPML are not supported |
| Called number from | Request URI | KPN sends numbers in international (E.164) notation by default |

!!! warning "Your KPN order wins"
    KPN hands out the SBC addresses and SIP domain per order and can deviate from the standard addresses.
    The values here come from KPN's configuration guidelines (2017). Always check them against your order.

## 1. At KPN

1. Give KPN the **fixed public IP address** of your Kiestoon server when ordering (KTOF form).
2. Note the SBC address(es) and SIP domain from your order.
3. Keep the standard profile with numbers in international notation (`+31...`).

## 2. In Kiestoon

1. **Trunks & routes › Trunks › Add trunk**, provider **KPN SIP trunk**. Change the host and provider IP
   addresses when your order lists other SBCs.
2. **Caller ID**: a number from your KPN block. KPN does not allow other numbers.
3. **T.38 fax** (Advanced): on when you receive faxes. KPN offers T.38 first and falls back to G.711.
4. Save, then add [inbound routes](../routes.md) for your numbers and an [outbound route](../routes.md).
5. Emergency calls: KPN recognises `112`.

## If it does not work

| Symptom | Fix |
| --- | --- |
| No calls at all | Check that KPN has the right public IP of this server and that TCP 5060 is open both ways |
| Calls to KPN mobile fail | Make sure only `alaw` is offered |
| Wrong number shown | The caller ID must be a number of your KPN trunk; KPN strips P-Asserted-Identity |
| Audio drops on large packets | KPN does not allow IP fragmentation; check the MTU on your router |

Sources: [KPN EEN dienst Vast (VoIP Connect) configuration guidelines](https://wiki.innovaphone.com/images/4/4b/KPN_EEN_dienst_Vast_%28VoIP_Connect%29_Configuration_Guidelines_innovaphone_PBX_12r2.pdf),
[innovaphone: KPN VoIP Connect SIP provider](https://wiki.innovaphone.com/?i=9983).
