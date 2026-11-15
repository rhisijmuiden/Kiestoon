# Separate host names

Kiestoon can use two addresses on the same server:

| Address | Example | Used for |
| --- | --- | --- |
| **PBX address** | `https://sip.example.com` or `https://voip.example.com` | Administration, phone provisioning, firmware, phone book, SIP registration, SAML |
| **Employee portal address** | `https://voipcentrale.example.com` | The [user portal](portal.md), welcome e-mails, set-password links, the [Windows softphone](softphone.md) |

Both addresses reach the same Kiestoon. Without an employee portal address everybody uses the PBX address, as
before.

![Host names](shot:internet)

## With the built-in internet access (Caddy)

1. Create DNS records for **both** names, pointing to your public IP address.
2. Go to **Internet access**, fill in the **Domain name** (the PBX address, e.g. `sip.example.com`) and the
   **Employee portal host name** (e.g. `voipcentrale.example.com`), and save.
3. Caddy gets a separate Let's Encrypt certificate for each name. The **Certificate** block shows both.

When **Use the certificate for SIP over TLS too** is on, the certificate of the PBX address is used for SIP.

## Behind your own reverse proxy

1. Go to **Internet access › Own reverse proxy**, set the **Public address** (PBX) and the
   **Employee portal address**.
2. Point both host names at the same Kiestoon in your proxy. Pass the `Host` header unchanged (or set
   `X-Forwarded-Host`; Kiestoon only reads it from a trusted proxy).

nginx example with two server blocks:

```nginx
server {
    listen 443 ssl http2;
    server_name sip.example.com voipcentrale.example.com;
    ssl_certificate     /etc/ssl/example.com/fullchain.pem;   # a certificate for both names
    ssl_certificate_key /etc/ssl/example.com/privkey.pem;
    location / {
        proxy_pass http://10.0.0.20:8080;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade           $http_upgrade;
        proxy_set_header Connection        $connection_upgrade;
    }
}
```

## Blocking administration on the employee portal address

In the **Host names** block, switch on **Block administration on the employee portal address**. On that address:

- administrators get the user portal and the switchboard, never the administration pages or the admin API;
- the menu shows **Administration is closed on this address** with a link to the PBX address;
- employees and reception work as usual.

You can only switch this on from the PBX address, so you cannot lock yourself out.

!!! warning "Keep administration off the internet completely"
    Kiestoon tells the addresses apart by host name. Anyone who knows the PBX address and can reach the server can
    still try it. For full separation, only publish the employee portal address in your proxy or public DNS, and
    reach the PBX address from your own network or a VPN. Combine this with
    [Firewall](firewall.md) rules for the admin networks.

## What goes where

| Link or setting | Address |
| --- | --- |
| Welcome e-mail, set-password link | Employee portal |
| Pairing the Windows softphone | Employee portal |
| Browser softphone (WebRTC) | The address the page was opened on |
| SIP server shown in the portal and welcome e-mail | PBX host name |
| Phone provisioning, firmware, phone book XML, zero-touch | PBX address |
| OpenID Connect sign-in | The address you signed in on (register both callbacks, shown in **Host names**) |
| SAML sign-in | PBX address |
| Passkeys | Per address: a passkey made on one address does not work on the other |

SIP (5060/5061) and audio (RTP) never go through a web proxy: phones register directly with the PBX host name,
so its DNS record must point to the address where those ports are forwarded.
