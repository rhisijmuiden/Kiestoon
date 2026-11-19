# Firewall and fraud protection

A phone system on the internet is attacked within minutes: bots try SIP passwords day and night, and a hacked
extension can call expensive numbers abroad. Kiestoon protects you in layers: a firewall, automatic blocking,
geo-blocking and fraud protection on outbound calls.

## Firewall

![Firewall](shot:firewall)

Kiestoon manages an **nftables** firewall on the server.

| Setting | What it does |
| --- | --- |
| **Firewall on** | Blocks everything except the portal, SIP and audio ports. |
| **SIP open for everyone** | For phones that connect from anywhere. Not recommended: use the whitelist or [Edge](edge.md) instead. |
| **Allow SIP from local networks** | Phones in the office network can always register. |
| **SIP whitelist** | IP addresses or networks allowed to use SIP, one per line. The addresses of your [trunks](trunks.md) are added automatically. |
| **Portal access** | Networks that may open the portal. Empty: everyone. |

**Show the generated rules** shows the exact nftables rules.

!!! warning "Safe mode"
    Every change is applied in **safe mode**: Kiestoon checks that you can still reach the portal, and undoes the
    change automatically within 90 seconds if you cannot. Kiestoon also warns before you save a portal access list
    without your own IP address.

## fail2ban

With **fail2ban on**, an address that fails to sign in too often on SIP or the portal is blocked.

- **Attempts**, **Within (minutes)** and **Block for (minutes)** set how strict it is.
- **Never block**: trusted addresses, such as your office IP.
- **Blocked addresses** shows the current blocks; **Unblock** lifts one.

## Trusted networks (skip MFA)

People who sign in with their password from a known network, such as the office LAN or the company VPN, can skip
the MFA step there. Outside those networks the code is asked as usual.

1. Open **Firewall** and scroll to **Trusted networks**.
2. Add each network as an IP range in CIDR notation (`192.168.1.0/24`, `10.8.0.0/16`, `fd00:1::/64`) with a
   description. A single address (`203.0.113.10`) is stored as `/32` or `/128`. **Add my address** fills in the
   address the portal sees for you right now.
3. Choose the **roles that may skip MFA**. Leave **Super admin** off (the default), so super admins always use their
   code.
4. Switch on **Skip MFA on trusted networks** and click **Save**. This is saved straight away; it does not change
   the firewall rules and needs no **Apply**.

![Trusted networks](shot:trusted-networks)

How it is kept safe:

- **Off by default.** Every skipped MFA step is written to the [audit log](users.md#audit-log) as `login.mfa_skipped`, with
  the network and its description.
- **Only where you are.** A session that skipped MFA keeps that only while it is used from a trusted network. Take
  the laptop home and the portal asks for the code before you can continue (audit: `session.mfa_required`). Taking
  a role or network off has the same effect on running sessions.
- **The real address.** Kiestoon uses the address of the connection. `X-Forwarded-For` is only believed from Caddy
  on this machine ([internet access](internet.md)) or from the trusted proxies of your own reverse proxy, so a
  browser cannot claim to be in the office.
- **No internet-wide ranges.** Ranges wider than `/8`, such as `0.0.0.0/0` or `::/0`, are refused. A public range is
  allowed (an office with a fixed IP address) but the page warns about it: never add a range of a provider, a
  mobile network or a cloud.
- **Password sign-in only.** [Single sign-on](users.md#single-sign-on) leaves MFA to the identity provider,
  [API tokens](api.md) never ask for MFA, and passkeys already are a second factor. Administrators who have not set
  up MFA yet still have to do so, so it works when they are away from the office.

!!! warning "A trusted network is as safe as its weakest device"
    Anyone on that network only needs a password. Use it for networks you control, keep guest Wi-Fi out of the
    range, and use strong passwords.

## Geo-blocking

Only let telephony (SIP), and if you want the portal, in from certain countries.

- **Only allow these countries**, for example with **+ Netherlands & Belgium** or **+ Europe**, or
  **Block these countries**.
- **Also for the web portal**: port 80 stays open for Let's Encrypt.
- Phones on your own network, the trunk providers and the whitelists always get in.
- Colleagues who travel abroad cannot reach the phone system from countries outside the allow list.

The country data is the free "IP to Country Lite" database of DB-IP (CC BY 4.0). Kiestoon downloads it once a
month; **Update country data** fetches it right away.

## Fraud protection

![Call features & fraud](shot:features)

Under **Call features & fraud › Toll-fraud protection** you limit what can be dialled, and Kiestoon raises the
alarm on unusual traffic, so a stolen password does not cost a fortune.

| Setting | What it does |
| --- | --- |
| **Premium-rate prefixes** | Blocked for extensions without **Allow premium-rate numbers**, such as 0900 and 0906. |
| **Blocked country codes** | Never dialable, for example 7, 234 or 882. |
| **Maximum simultaneous outbound calls** | A hard limit for the whole system. 0 is unlimited. |
| **Alert above outbound calls** / **Alert above international calls** | Per hour. 0 turns the alert off. |
| **Alert email** | Who gets the alarm. Empty: the notification address under [Mail server](mail.md). |
| **Block all outbound calls on an alert** | Stops all outbound calls until an administrator clicks **Unblock**. Emergency calls stay possible. |

Per extension, **Outbound calls** sets what it may dial: **Internal only**, **National** or **International**.

!!! tip "Emergency calls always work"
    112 and 911 work from every extension, without a route, during a fraud block and even when a trunk limit is
    reached. See [Settings](settings.md#emergency-calls).

## The same page: parking and recording

- **Call parking**: dial `70` to park a call; it goes to slot 71 and up. A parked call that nobody picks up rings
  back at the person who parked it.
- **Recording**: how long [recordings](history.md#recordings) are kept (mind the GDPR), and an optional beep every
  15 seconds so callers know they are being recorded.
