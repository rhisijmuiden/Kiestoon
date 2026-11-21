# Provider: Speakup

[Speakup](https://speakup.nl/telefonie/trunk/) offers SIP trunks for your own phone system. Kiestoon has a
ready-made **Speakup** template.

## What the template sets

| Setting | Value | Why |
|---|---|---|
| Authentication | Registration | Speakup trunks use a login name and password |
| Host / port | `sip.speakup.nl`, `5060` | Speakup's SIP domain; no separate proxy or registrar |
| Transport | UDP | |
| Codecs | `alaw,ulaw` | Speakup supports G.711 A-law, µ-law and G.729 and does no transcoding |
| DTMF | RFC 2833 / 4733 | Kiestoon's standard |
| Called number from | To header | Numbers are in `31...` format; Kiestoon also matches `0...` and `+31...` |

!!! warning "Speakup does not publish a PBX manual"
    These values come from interop guides by Swyx and Tuxis (a Speakup partner). Always use the details Speakup
    gives you for your trunk: they win over this page.

## 1. At Speakup

1. Ask Speakup for the trunk details: **login name**, **password** and the **numbers** of the trunk (`31...`).
2. Optional, more secure: ask Speakup to only accept the trunk from the public IP address of your Kiestoon server.

## 2. In Kiestoon

1. **Trunks & routes › Trunks › Add trunk**, provider **Speakup**.
2. **Username**: the main number of the trunk, for example `31201234567`.
3. **Authentication username**: the Speakup login name, when it differs from the number. **Password**: the trunk
   password.
4. **Caller ID**: one of the trunk numbers. CLIP no screening (showing a number that is not yours) is not
   allowed in the Netherlands.
5. **T.38 fax**: Speakup passes T.38 end to end. For a dedicated fax trunk Speakup uses `t38-sbc.speakup.nl`:
   add a second trunk with that host and turn on T.38.
6. Save, then add [inbound routes](../routes.md) for the numbers and an [outbound route](../routes.md).

## If it does not work

| Symptom | Fix |
| --- | --- |
| Status **Rejected** | Check the login name and password; put the login name in Authentication username |
| Incoming calls reach the wrong route | Set **Called number comes from** to **Request URI** |
| No audio | NAT: check the public address and the RTP port forward |

Sources: [Swyx: SIP Provider SpeakUp (NL)](https://service.swyx.net/hc/en-gb/articles/360010409039-SIP-Provider-SpeakUp-NL),
[Tuxis: Speakup trunk instellen](https://support.tuxis.nl/en/knowledgebase/article/speakup-trunk-instellen-kerio-operator).
