# Provider: Vodafone Ziggo

Vodafone Ziggo Zakelijk delivers SIP trunks with a username and password per trunk. Kiestoon has a ready-made
**Vodafone Ziggo Zakelijk** template.

## What the template sets

| Setting | Value | Why |
|---|---|---|
| Authentication | Registration | Ziggo supplies a username and password; registration every hour |
| Host / port | `trunk.sipnl.net`, `5060` | Ziggo's SIP trunk server |
| Transport | UDP | |
| Codecs | `alaw,ulaw` | G.711; check with Ziggo when you want other codecs |
| Called number from | To header | Numbers are in `31...` format; Kiestoon also matches `0...` and `+31...` |
| Fax | T.38 | Ziggo advises T.38 for fax; turn it on under Advanced |

!!! warning "Vodafone Ziggo does not publish a PBX manual"
    These values come from Panasonic's Ziggo SIP trunk setup guide (2017). Always use the details Ziggo gives you
    for your trunk: they win over this page.

## 1. At Vodafone Ziggo

1. Note the **username** (for example `31123450000`, without `@trunk.sipnl.net`) and the **password**.
2. Note the numbers of your number block, also in `31...` format.

## 2. In Kiestoon

1. **Trunks & routes › Trunks › Add trunk**, provider **Vodafone Ziggo Zakelijk**.
2. **Username** and **authentication username**: the Ziggo username. **Password**: the Ziggo password.
3. **Caller ID**: a number from your block. Ziggo always adds a P-Asserted-Identity header on incoming calls.
4. **T.38 fax** (Advanced): on when you fax.
5. Save. The status turns **Registered**. Add [inbound routes](../routes.md) and an [outbound route](../routes.md).
6. Behind NAT? Fill in the public address under **Settings → Network and NAT**, forward UDP 5060 and the RTP range,
   and limit the forward to the Ziggo source addresses.

## If it does not work

| Symptom | Fix |
| --- | --- |
| Status **Rejected** | Check the username (without `@trunk.sipnl.net`) and the password |
| Incoming calls reach the wrong route | Set **Called number comes from** to **Request URI** |
| Calls drop behind NAT | Check the NAT settings and the port forward |

Source: [Panasonic: KX-NS setup guide, Ziggo SIP trunk](https://www.psn-web.net/cs-im/Global/PBX/pbx/ns_nsx/manual/SIP_Trunk/Netherlands/NS%20Series%20Setup%20Guidance%20Ver_1-2%20Ziggo.pdf).
