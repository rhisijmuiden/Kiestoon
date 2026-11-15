# Phones and provisioning

Kiestoon configures desk phones for you: accounts, keys, language, time server, phone book and firmware. A phone
only needs to know where Kiestoon is. You then link it to an extension in the portal.

![Phones & provisioning](shot:phones)

## Supported brands

| Brand | Models | Status |
| --- | --- | --- |
| **Yealink** | T3x, T4xU, T5xW, with EXP40, EXP43 and EXP50 modules | Tested |
| **Snom** | D7xx, D8xx, with D7 module | Tested |
| **Grandstream** | GRP26xx, GXP2170, with GBX20 module | Built after the vendor documentation |
| **Fanvil** | X3U, X4U, X5U and more | Built after the vendor documentation |
| **Gigaset** | N670 / N870 IP PRO DECT, with several handsets | Built after the vendor documentation |
| **Poly** | VVX, Edge E | Built after the vendor documentation |
| **Htek**, **Akuvox**, **Cisco 8800** (3PCC firmware) | | Built after the vendor documentation |

Brands marked "built after the vendor documentation" have not been tested on a real device yet. Let us know when
something does not work.

## 1. Switch provisioning on

Open **Phones & provisioning** and click **Provisioning settings**:

| Field | What |
| --- | --- |
| **Provisioning on** | Phones may fetch their configuration |
| **Server address for phones** | Host name or IP address phones use to reach Kiestoon |
| **Username / Password** | Protect the provisioning address. Generated; keep them private |
| **Phone admin password** | Set on every phone, for its own web interface |
| **Time server** | NTP server for the phones, for example `nl.pool.ntp.org` |
| **Language** | Language on the phone display |

The card then shows the **provisioning URLs** to copy:

- `http://user:password@<server>:8080/prov/` for Yealink, Grandstream, Fanvil, Poly, Htek, Akuvox and Cisco,
- the same address with `snom{model}.htm` for Snom,
- the same address with `gigaset/` for the Gigaset base station.

## 2. Point the phones at Kiestoon

Pick what suits your network:

- **DHCP option 66** on your router or DHCP server, with the URL above. New phones find Kiestoon by themselves.
- **By hand** in the phone's web interface (*Auto provisioning* / *Setting server*).
- **Zero-touch** through the vendor's redirection service, also for phones outside the office. See below.

## 3. Link a phone to an extension

A phone that asks for its configuration, but whose MAC address is unknown, appears under **Waiting for an
extension**, with its brand and model recognised from what it sent. Choose an extension and click **Link**.

Kiestoon writes the configuration and sends the vendor's SIP NOTIFY (check-sync; resync for Cisco), so a registered
phone fetches its new configuration within seconds. A phone that is not registered yet picks it up at its next
provisioning check or right away after a reboot.

You can also add a phone in advance with **Add phone** (MAC address from the label at the back, model, extension
and label), move a phone to another extension, or unlink it. **Fetch configuration now** asks a phone to reload.
**Preview config** shows the exact file the phone gets.

## Multi-handset DECT (Gigaset)

A Gigaset N670 or N870 base station carries several handsets. Handset 1 uses the extension of the phone; give each
further handset its own extension under **Handsets**.

## Firmware

On the **Firmware** tab, upload the firmware once (or download it from a link to the vendor's site) and choose the
models it is for. A `.zip` is unpacked. Click **Roll out to phones**: phones of those models install it on their
next check, usually at night or after a reboot. Phones fetch it from Kiestoon (`/prov/fw/<random token>/`), so they
need no internet access.

!!! warning
    Only choose models a file is meant for. A wrong file can make a phone unusable.

## Zero-touch provisioning

On the **Zero-touch** tab, Kiestoon shows the address to register with the vendor's redirection service. A new
phone then asks that service where its settings are and configures itself out of the box, also at home.

- **Yealink RPS** works automatically: enter an AccessKey ID and secret from the Yealink Device Management portal
  (*API*). Phones are registered and removed when you add or delete them.
- **Snom SRAPS, Grandstream GDMS, Fanvil FDPS, Htek, Poly, Gigaset and Cisco** have no public API. Sign in to their
  portal, enter the server address shown, and import the MAC addresses with the **MAC list (CSV)** export.

Phones outside your network need HTTPS: switch on [internet access](internet.md) or set a public address.

## Keys and expansion modules

Every phone has a visual key editor with extra lines, BLF, speed dials, parking, intercom and expansion modules. That
has its own chapter: [Phone keys and modules](phone-keys.md).
