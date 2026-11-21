# Branch offices (Edge)

Kiestoon Edge is a small satellite for a branch office. It connects the office to the central Kiestoon (for example
in a data centre) through a WireGuard tunnel and runs a SIP proxy (Kamailio) and media relay (rtpengine) for the
phones in the office.

```
Office phones ──LAN── Kiestoon Edge ══ WireGuard (UDP 51820) ══ Kiestoon (data centre)
  SIP 5060, RTP         Kamailio, rtpengine,                       Asterisk, portal,
  provisioning :80      provisioning proxy                         all settings
```

Only the central Kiestoon needs a reachable UDP port. The edge connects out, so the office needs **no port
forwarding** and NAT problems between the office and the data centre are gone: only the tunnel crosses the NAT.
Extensions, voicemail and all settings stay on the central Kiestoon.

![Sites › Branch offices](shot:sites)

## Install

Pick one:

| Where | What to download from the release |
| --- | --- |
| Raspberry Pi 3, 4, 5 or Zero 2 W | `kiestoon-edge-<version>-raspberry-pi-arm64.img.xz` (Raspberry Pi OS Lite 64-bit with Kiestoon Edge) |
| Proxmox | `kiestoon-edge-<version>-debian-13-amd64.tar.zst` (LXC template) |
| Any Debian 13 machine | `kiestoon-edge_<version>_amd64.deb` or `_arm64.deb`: `apt install ./kiestoon-edge_*.deb` |

**Raspberry Pi:** write the image with Raspberry Pi Imager (*Use custom*). The Imager settings (user, SSH, Wi-Fi,
host name) work as usual. A wired connection is recommended for phones.

**Proxmox in one command:** run `bash -c "$(curl -fsSL https://raw.githubusercontent.com/rhisijmuiden/Kiestoon/testing/proxmox/kiestoon-lxc.sh)" -- --edge` in the shell of the Proxmox host. It
downloads the Edge template and creates the container (512 MB RAM, 4 GB disk). See [Installation](install.md).

**LXC:** the WireGuard module must be available in the host kernel (Proxmox has it). Unprivileged containers work.
Without the kernel module, `apt install wireguard-go` makes `wg-quick` use the (slower) userspace implementation.

## Pair

1. On the central Kiestoon, open **Sites → Branch offices**, switch on **Accept branch offices and linked systems**
   and enter the public address of the Kiestoon. Forward UDP port 51820 to it if it is behind a router.
2. Click **Add branch office**. Kiestoon shows a pairing code (valid 24 hours, works once).
3. Open the page of the edge in the office: `http://<edge address>/` and paste the code.
   - Without a screen: put the code in a file `kiestoon-edge.txt` on the boot partition of the SD card (the
     `bootfs` drive on your computer). The edge reads it on start, pairs and deletes the file.
   - On the command line: `kiestoon-edge pair <code>`.

The code holds the address of the portal and the SHA-256 fingerprint of its certificate. The edge only talks to a
portal with exactly that certificate, so a self-signed certificate is safe. The edge generates its own WireGuard key;
the private key never leaves the edge.

## Phones

Point the phones in the office at the edge:

- SIP server / registrar: `<edge address>`, port 5060 (UDP or TCP)
- Provisioning URL: `http://<edge address>/prov/` (for example in DHCP option 66)

The edge forwards provisioning to the central Kiestoon, which hands out the edge as SIP server. Phones register
through the edge (the edge adds a `Path` header); the central Asterisk sends calls back through the same edge.
Media flows phone → edge → tunnel → Asterisk; rtpengine anchors it on both sides.

## Status and maintenance

- The page of the edge shows the tunnel, the last contact and the services. The central portal shows every edge with
  its office network, version and services.
- The edge sends a heartbeat every 30 seconds through the tunnel. It picks up a new office address (DHCP) by itself.
- Update the edge like any Debian package (`apt install ./kiestoon-edge_<new>.deb`).
- To move an edge to another Kiestoon: `kiestoon-edge reset` on the edge, then pair again. Creating a new pairing code
  in the portal disconnects the old edge.
- `kiestoon-edge status` prints the state as JSON. Logs: `journalctl -u kiestoon-edge -u kamailio -u rtpengine-daemon`.

## Security

- Pairing from the local page is only possible while the edge is not paired, so nobody in the office can move a
  working edge to another system. Resetting needs a shell on the edge.
- The edge relays requests from the office to the central Kiestoon only, with flood protection (Kamailio pike).
  All phones of an office share the tunnel address of the edge, so fail2ban on the central system bans the whole
  office when one phone keeps failing to sign in.
- The edge holds no extension passwords; authentication happens on the central Asterisk.

## Ports

| Port | Where | What |
| --- | --- | --- |
| UDP 51820 | central Kiestoon, from the internet | WireGuard (changeable) |
| UDP/TCP 5060 | edge, office network | SIP for the phones |
| UDP 30000–39999 | edge, office network | RTP (media) |
| TCP 80 | edge, office network | status page and provisioning |
