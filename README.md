<p align="center"><img src="branding/kiestoon-mark.svg" width="72" alt=""></p>

# Kiestoon

Open source, lightweight single-tenant VoIP phone system (PBX) for Debian 13, with a modern web portal.
English by default, fully translated into Dutch. Powered by RHIS.

> Status: **phase 0, foundation** (v0.1). Calling arrives in phase 1. See the [roadmap](ROADMAP.md).

## Install

### Proxmox LXC template

Download `kiestoon-<version>-debian-13-amd64.tar.zst` from the [releases](https://github.com/rhisijmuiden/kiestoon/releases),
upload it to Proxmox (*storage › CT Templates › Upload*) and create a container from it
(unprivileged is fine; 1 CPU, 1 GB RAM, 8 GB disk to start). Log in on the console as root:
the address of the portal and the one-time setup token are shown.

### Debian 13 server or VM

```sh
curl -fsSL https://raw.githubusercontent.com/rhisijmuiden/kiestoon/main/install.sh | sudo bash
# or follow the testing channel:
curl -fsSL https://raw.githubusercontent.com/rhisijmuiden/kiestoon/main/install.sh | sudo bash -s -- --channel testing
```

Open `http://<server>:8080`, enter the setup token and create the first administrator.
Administrators must set up two-factor authentication (TOTP) on first sign-in.

Show the setup token again with `sudo kiestoon setup-token`.

## Updates and channels

| Channel | Branch | Releases |
| --- | --- | --- |
| Production | `main` | Stable versions, `vX.Y.Z` |
| Testing | `testing` | Release candidates, `vX.Y.Z-rc.N` |

In the portal, open **Info & updates** (bottom of the left menu) to see the version, switch channel,
check for updates and install one. The updater makes a backup of `/var/lib/kiestoon` to
`/var/lib/kiestoon-backups` before every update (the newest 5 are kept).

## Layout

| Path | What |
| --- | --- |
| `server/` | Go backend: API, auth, SQLite store, updater client. Single binary with the UI embedded |
| `web/` | React + TypeScript + Tailwind portal, translations in `web/src/i18n/` |
| `branding/` | House style: tokens, logos, usage rules |
| `packaging/` | `.deb` (nfpm), systemd units, updater script |
| `lxc/` | Proxmox LXC template build |
| `install.sh` | Installer for Debian 13 |

On an installed system:

| Path | What |
| --- | --- |
| `/etc/kiestoon/kiestoon.env` | Settings (port, data dir, default channel) |
| `/var/lib/kiestoon/` | All state: database and files |
| `/var/lib/kiestoon-backups/` | Automatic pre-update backups |

## Development

Needs Go (see `server/go.mod`) and Node.js 22.

```sh
make run                 # API on :8080 with a local data dir
cd web && npm run dev    # UI with hot reload on :5173, proxies /api to :8080
make test                # Go tests, translation check, type check
make deb                 # .deb in dist/ (needs nfpm)
```

Branches: work on a feature branch, open a pull request into `testing`. Releases are made by pushing a tag
(`vX.Y.Z-rc.N` on `testing`, `vX.Y.Z` on `main`); the release workflow builds the packages and the LXC template.

## Licence

GPL-3.0. See [LICENSE](LICENSE).
