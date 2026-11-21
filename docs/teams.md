# Microsoft Teams

With **Direct Routing**, people who work in Microsoft Teams make and receive normal phone calls through Kiestoon and
your own trunks. Kiestoon acts as the **SBC** (session border controller) of your Microsoft 365 tenant: Teams sends
its calls to Kiestoon, and Kiestoon rings Teams users like any extension.

![Microsoft Teams overview](shot:teams)

## What you need

- **Internet access** switched on, with **Use the certificate for SIP**. Microsoft only accepts a public certificate
  (Let's Encrypt is fine) with an **RSA** key; Kiestoon asks Let's Encrypt for RSA when the certificate is used for SIP.
- A **domain verified in Microsoft 365** for the SBC name (for example `pbx.example.com` on the verified domain
  `example.com`), with at least one licensed user on that domain.
- A **Teams Phone** licence for every Teams user who calls through Kiestoon (in Microsoft 365 E5, or as an add-on).
- The **MicrosoftTeams PowerShell module** and a Teams administrator account.

## Setting it up

1. Go to **Telephony > Microsoft Teams** and switch on **Microsoft Teams Direct Routing**. The SBC domain name is
   filled in from Internet access.
2. Check the **SIP TLS port** (5067 by default). It is separate from the TLS port of the phones, because Microsoft
   needs the domain name in every SIP message. The firewall opens this port for Microsoft's address ranges only
   (`52.112.0.0/14` and `52.122.0.0/15`).
3. Choose how **outside calls from Teams** go out: through the normal outbound routes, or through one trunk. Choose
   the **caller ID**: the user's own Teams number, or the caller ID of the route or trunk.
4. Save, then **restart the phone system** once (Settings): Asterisk opens the new port only on a restart.
5. Add the Teams users (below), then run the PowerShell script from the **Microsoft 365 setup** tab.

**Setup progress** on the overview shows what is still missing. **Microsoft SIP proxies** shows whether Kiestoon
reaches Microsoft's three proxies (`sip`, `sip2` and `sip3.pstnhub.microsoft.com`); calls go to the next one when one
fails.

## Teams users

![Teams users](shot:teams-users)

Each Teams user gets a **phone number** (E.164, for example `+31 20 123 4567`) and optionally an **extension**:

| Rings on | What happens |
| --- | --- |
| **Teams + desk phone** | Calls to the extension ring the desk phone, the softphone and Teams together. First to answer gets the call. |
| **Teams only** | Calls to the extension ring only in Teams. |
| No extension | Calls to the number ring in Teams, unless an inbound route sends that number elsewhere. |

With an extension, Teams users call colleagues by extension number and show as that extension; queues, ring groups
and the switchboard reach them like any other extension. Their outgoing rights (national, international) are the
rights of the extension. Teams numbers without an extension may call out nationally. 112 always works.

## The PowerShell script

![Microsoft 365 setup](shot:teams-setup)

Kiestoon writes the script with your domain name, port and users. Copy or download it and run it in PowerShell:

```powershell
Install-Module MicrosoftTeams     # once
.\kiestoon-teams.ps1
```

The script adds Kiestoon as **PSTN gateway**, a **voice route** that sends every number to Kiestoon, the **voice
routing policy** "Kiestoon", and gives every user their number and the policy. It is safe to run again after you add
or change users. Microsoft can take up to an hour before a new gateway or number works.

## Troubleshooting

- **Certificate not ready**: check Internet access. The certificate must cover the SBC domain name and use RSA. If it
  still uses ECDSA, save Internet access again so Let's Encrypt issues an RSA certificate.
- **Proxies unreachable**: check that port 5067 (or your port) is forwarded to Kiestoon in your router, and that the
  phone system was restarted after switching Teams on.
- In the **Teams admin center**, under *Voice > Direct Routing*, Microsoft shows the TLS and SIP OPTIONS status of the
  gateway.
- Audio always flows through Microsoft's media processors (no media bypass); the RTP port range must be reachable.
