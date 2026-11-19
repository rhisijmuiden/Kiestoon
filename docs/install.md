# Install

Kiestoon runs on **Debian 13 "Trixie"** (amd64 or arm64). Pick one of two ways: the ready-made Proxmox LXC
template, or the installer on a Debian 13 server or virtual machine. Both end with the same result: a running
portal and a one-time **setup token** for the first administrator.

## What you need

| | Minimum | Comfortable for 50+ users |
| --- | --- | --- |
| CPU | 1 core | 2 cores |
| Memory | 1 GB | 2 GB (4 GB with local speech-to-text) |
| Disk | 8 GB | 20 GB or more for recordings and voicemail |
| Network | A fixed IP address in the office network | Plus a public host name for remote phones and the softphone |

## Option 1: Proxmox LXC template

1. Download `kiestoon-<version>-debian-13-amd64.tar.zst` from the
   [releases](https://github.com/rhisijmuiden/Kiestoon/releases) on GitHub.
2. In Proxmox, open *your storage › CT Templates › Upload* and upload the file.
3. Create a container from the template. Unprivileged is fine. Start with 1 CPU, 1 GB RAM and 8 GB disk, and give
   it a fixed IP address (or a DHCP reservation).
4. Start the container and log in on the console as `root`. The console shows the address of the portal and the
   setup token.

!!! note "Testing channel"
    Release candidates (`vX.Y.Z-rc.N`) come with a template too. A container made from an rc template follows the
    **testing** channel; a stable template follows **production**. You can switch later, see [Updates](updates.md).

## Option 2: Debian 13 server or VM

Run the installer as root on a clean Debian 13 machine:

```sh
curl -fsSL https://raw.githubusercontent.com/rhisijmuiden/Kiestoon/testing/install.sh | sudo bash
```

To follow the testing channel (release candidates):

```sh
curl -fsSL https://raw.githubusercontent.com/rhisijmuiden/Kiestoon/testing/install.sh | sudo bash -s -- --channel testing
```

!!! note ""
    Until the first stable release, only the testing channel has releases, so use `--channel testing` for now.

Installer options:

| Option | What it does |
| --- | --- |
| `--channel production|testing` | Update channel to follow (default `production`) |
| `--version vX.Y.Z` | Install exactly this release instead of the newest |
| `--deb`, `--asterisk-deb`, `--sounds-deb`, `--speech-deb` | Install local packages (for development) |
| `--dry-run` | Only show which release would be installed, change nothing |

The installer adds the packages `kiestoon` (portal and API), `kiestoon-asterisk` (Asterisk 22 LTS, built for
Kiestoon), `kiestoon-sounds-nl` (Dutch voice prompts) and the services `kiestoon-helper` (a small root helper for
the firewall, backups and tunnels) and `kiestoon-updater`. At the end it prints the portal address and the setup
token.

## Open the portal

| Address | When |
| --- | --- |
| `http://<server>:8080` | Always, in your own network |
| `https://<server>:8443` | Same portal over HTTPS with a self-signed certificate. Needed for the browser softphone (microphone) |
| `https://<your domain>` | After you switch on [internet access](internet.md) (Let's Encrypt on port 443) |

Lost the setup token? Show it again on the server:

```sh
sudo kiestoon setup-token
```

## Ports

Open these in a firewall in front of Kiestoon. The [firewall page](firewall.md) in the portal opens exactly these on
the server itself.

| Port | What |
| --- | --- |
| 8080/tcp, 8443/tcp | Portal (HTTP, HTTPS) and phone provisioning (`/prov/`) |
| 80/tcp, 443/tcp | Only with internet access switched on (Caddy, Let's Encrypt) |
| 5060/udp+tcp, 5061/tcp | SIP, SIP over TLS |
| 10000-20000/udp | Audio (RTP) |
| 51820/udp | WireGuard for branch offices and linked systems (only when switched on) |
| 51821/udp | WireGuard between the two nodes of a high-availability pair (only when switched on) |
| 389/tcp, 636/tcp | LDAP phone book, own network only (only when switched on) |

## Where things live

| Path | What |
| --- | --- |
| `/etc/kiestoon/kiestoon.env` | Settings: listen port, data directory, default channel |
| `/var/lib/kiestoon/` | All state: database, generated Asterisk config, voicemail, recordings, faxes, mail queue |
| `/etc/asterisk/` | Static Asterisk config that includes the generated files. Do not edit by hand |
| `/var/lib/kiestoon-backups/` | Automatic backups made before every update (the newest 5 are kept) |

!!! info "One rule behind everything"
    All state lives in `/var/lib/kiestoon`, and every generated configuration (Asterisk, firewall, provisioning) can
    be rebuilt from the database. That keeps backups, restores and high availability simple.

Next: [first setup](first-setup.md).
