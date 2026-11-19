# Provider: Cheapconnect

[Cheapconnect](https://www.cheapconnect.net) is a Dutch VoIP provider with SIP accounts that work with your own
phone system. There is no dedicated template yet; the **Generic (registration)** template does the job.

!!! note "Check the details in your Cheapconnect account"
    The server and the account format below are what Cheapconnect publishes for SIP accounts. Always use the exact
    values shown in your own Cheapconnect dashboard: they win over this page.

## 1. At Cheapconnect

1. Sign in to your Cheapconnect account and open the SIP account (the phone number) you want to use.
2. Note the **SIP username** (usually the account or phone number), the **password** and the **SIP server**.
3. Optional, more secure: limit the account to the public IP address of your Kiestoon server and set a spending
   limit.

## 2. In Kiestoon

1. **Trunks & routes › Trunks › Add trunk**, provider **Generic (registration)**.
2. **Name**: `Cheapconnect`.
3. **Host**: the SIP server from your account (`sip.cheapconnect.net`), **port** `5060`, **transport** UDP.
4. **Username** and **password**: from the SIP account.
5. **Called number comes from**: *To header* (the default of the generic template). Switch to *Request URI* when
   incoming calls land on the wrong route.
6. **Codecs**: `alaw,ulaw`.
7. **Caller ID**: the Cheapconnect number of this account.
8. **T.38 fax**: on, if you receive faxes on this number.
9. Save. The status turns **Registered**.

Then add an [inbound route](../routes.md) for the number and put the trunk in your [outbound route](../routes.md),
for example as the second trunk so it takes over when your main provider fails.

## If it does not work

| Symptom | Fix |
| --- | --- |
| Status **Rejected** | Wrong username or password; copy them again from Cheapconnect |
| Status stays **Not registered** | Check the server name and that outgoing UDP 5060 is allowed |
| Calls connect but there is no audio | NAT: set the public address under Settings and forward the RTP range |
