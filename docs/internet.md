# Internet access and SSL

On your own network the portal is always at `http://<server>:8080` (and `https://<server>:8443` with a
self-signed certificate). To reach the portal, the softphone and passkeys from anywhere with a trusted
certificate, switch on **Internet access**. Kiestoon then starts **Caddy** on ports 80 and 443 and gets a free
certificate from **Let's Encrypt** for your domain name, and renews it on its own.

![Internet access](shot:internet)

## Step by step

1. **DNS**: create an `A` record (and `AAAA` for IPv6) for the domain name, for example `pbx.example.com`, pointing
   to your public IP address. Kiestoon shows where the name points now, and warns when it does not resolve yet.
2. **Router**: forward ports **80** and **443** (TCP) to the Kiestoon server.
3. In Kiestoon, go to **Internet access**, switch on **Direct internet access**, fill in the **Domain name** and an
   **E-mail for Let's Encrypt** (it gets a message if renewing fails), and save.
4. Caddy gets the certificate within about a minute. The **Certificate** box shows who it is for, the issuer and
   the expiry date. Without a certificate yet, Caddy tries again every few minutes.

The firewall opens ports 80 and 443 automatically.

| Option | What it does |
| --- | --- |
| **Use the certificate for SIP over TLS too** | Phones and softphones trust the encrypted SIP connection (port 5061) without installing a certificate. |
| **Test certificates (Let's Encrypt staging)** | For trying things out without hitting the Let's Encrypt rate limits. Browsers do not trust these certificates. |

## What works over the internet address

- The admin portal and the [user portal](portal.md), with a valid padlock.
- The [browser softphone](portal.md#softphone-devices) and the [Windows softphone](softphone.md) (WebRTC over
  secure WebSockets).
- [Passkeys](users.md#passkeys), which need https and a host name.
- Links in welcome e-mails, voicemail e-mails and zero-touch [provisioning](phones.md).

!!! tip "Already a reverse proxy?"
    Do you already have nginx, Traefik, HAProxy or a firewall with a web proxy in front? Leave internet access off
    and follow [Own reverse proxy](reverse-proxy.md).

!!! warning "Telephones from outside"
    Internet access is for the web portal and the softphones. Desk phones and SIP apps outside the office still need
    SIP to be allowed in the [firewall](firewall.md). For branch offices, use an [Edge](edge.md): it connects
    through an encrypted tunnel, without opening SIP to the internet.
