# Kiestoon roadmap

Kiestoon is an open source, single-tenant VoIP phone system (PBX) for small businesses, built on
Debian 13 "Trixie". It ships as a Proxmox LXC template and as an installer for a plain Debian 13 machine.

Every phase ends with a release on the **testing** channel. After it has been tested it is promoted
to the **production** channel (`main`).

Legend: `[x]` done · `[ ]` planned

## Phase 0 · Foundation

Goal: a clean install on Debian 13 works and the portal can be updated from the UI.

- [x] Repository, licence (GPL-3.0), roadmap, house style in `branding/`
- [x] Go backend as a single binary with SQLite (WAL) and embedded web UI on port 8080
- [x] First-run setup wizard: create the first administrator
- [x] Login with sessions, login rate limiting, audit log
- [x] Mandatory MFA (TOTP) for administrators, with recovery codes
- [x] Portal layout: left admin menu, dark high-tech theme, "Powered by RHIS" bottom-right
- [x] English by default, full Dutch translation, language per user
- [x] Info button: version, check for updates, update from the UI
- [x] Production and testing channels, switchable in the UI
- [x] Updater service (systemd) with automatic pre-update backup
- [x] Debian package (`.deb`), installer script, LXC template build
- [x] CI: tests and builds on every pull request, releases on tags
- [ ] Passkeys / WebAuthn as a second MFA method

## Phase 1 · Core PBX

Goal: the first real phone call.

- [ ] Asterisk 22 LTS built as our own `.deb` in CI, configuration generated from the database
- [ ] Number plan wizard: 3, 4 or 5 digits, with suggested ranges and a migration tool
- [ ] Extensions / users, SIP credentials (strong, generated), per-device TLS + SRTP option
- [ ] SIP trunks (registration and IP authentication) with presets for common Dutch providers
- [ ] Inbound routes per DID, outbound routes with prefixes and failover
- [ ] Emergency number 112 always allowed, with the right caller ID
- [ ] Voicemail with voicemail-to-email (audio attachment)
- [ ] Custom SMTP mail server: STARTTLS / TLS, username + password or OAuth2 (Microsoft 365, Google), test button
- [ ] Call detail records (CDR): search, filters, CSV export
- [ ] Firewall (nftables) managed from the UI: SIP IP whitelist (IPs and CIDR ranges), RTP range, safe mode with automatic rollback
- [ ] fail2ban for SIP and portal logins, bans visible and removable in the UI
- [ ] Phone provisioning: Yealink, Snom, Grandstream (HTTPS, DHCP option 66, per-MAC config, visual key layout)
- [ ] Backups: full and incremental (restic), to FTP, FTPS, SFTP and SCP, with schedules, retention and restore from the remote location
- [ ] Live dashboard via AMI/ARI: active calls, registered phones, trunk status

## Phase 2 · Business features

Goal: a complete business phone system, including the reception.

- [ ] Call queues: ring strategies, music on hold, position announcements, overflow, agent login/logout
- [ ] Hunt groups / ring groups: simultaneous, sequential, rotating
- [ ] Visual IVR builder (digital receptionist): menus, prompts, time conditions, transfer, voicemail, queue, dial-by-extension
- [ ] Time conditions and holiday calendar (Dutch holidays preloaded), manual day/night switch
- [ ] SSO for admins and users: OpenID Connect (Microsoft Entra ID, Google Workspace, Keycloak, Authentik), IdP groups mapped to roles
- [ ] Roles: superadmin, admin, reception, user, with per-role permissions
- [ ] User portal (SSO):
  - [ ] call forwarding (always, busy, no answer), follow-me to mobile, do not disturb, time-based profiles
  - [ ] voicemail in the browser, greetings, email notifications
  - [ ] WebRTC softphone: call and be called with a headset from the browser
  - [ ] hunt group status, queue sign-in / sign-out and live queue status
  - [ ] own call history and recordings, BLF overview of colleagues with click-to-call
  - [ ] own keys (BLF, speed dial), SIP details / QR code for a mobile softphone
- [ ] Call recording: per user, per queue or on demand, with retention period (GDPR)
- [ ] BLF / presence, call pickup groups, call parking, intercom and paging
- [ ] Conference rooms with PIN
- [ ] Outbound fraud protection: block premium and international destinations per user, max concurrent outbound calls, alert mail on unusual traffic

## Phase 3 · Integrations

Goal: safely connected to the internet and to the rest of the business.

- [ ] Internet mode: Caddy with automatic Let's Encrypt certificates on 443, WebRTC over WSS
- [ ] Geo-blocking (country lists, updated monthly)
- [ ] SAML 2.0 next to OIDC
- [ ] OneDrive as backup destination
- [ ] Phone provisioning: Fanvil, Gigaset (N670/N870 Pro DECT), Poly (VVX, Edge)
- [ ] Fax-to-email: receive faxes (T.38 / G.711 via SpanDSP), convert to PDF, mail per fax number
- [ ] Queue wallboard for the reception
- [ ] Central phone book with XML phone book for phones and LDAP
- [ ] REST API with tokens, webhooks (for example incoming call to CRM)
- [ ] Passkeys / WebAuthn for users

## Phase 4 · Extensions

Ongoing.

- [ ] Support for running behind your own reverse proxy (trusted proxies)
- [ ] Zero-touch provisioning via vendor redirection services (Yealink RPS, Snom SRAPS, Grandstream GDMS, Fanvil FDPS)
- [ ] Firmware management per phone model
- [ ] Provisioning: Htek, Akuvox, Cisco 8800 MPP
- [ ] Local text-to-speech (Piper) for IVR prompts, speech-to-text for voicemail
- [ ] Email-to-fax / send a fax from the portal (depends on T.38 support of the SIP provider)
- [ ] More backup destinations: S3, WebDAV, SMB
- [ ] Optional Prometheus metrics
- [ ] Research: multi-tenant with Kamailio in front

## Phase 5 · Multi-site (after v1.0)

Goal: one phone system across several offices and data centres.

- [ ] Kiestoon Edge: a satellite for a branch office, as Raspberry Pi image (arm64), LXC template or `.deb`
  - [ ] WireGuard tunnel to the central Kiestoon in the data centre, paired with a code from the portal
  - [ ] Local SIP proxy and media relay (Kamailio + rtpengine) to solve NAT problems
  - [ ] Local provisioning server for the phones in that office
- [ ] Federation of two or more Kiestoon systems over a tunnel (WireGuard + SIP-TLS)
  - [ ] Shared phone book and number plan with a location prefix
  - [ ] Internal transfers and BLF between locations
  - [ ] Agents from several locations in one queue
  - [ ] Follow-the-sun routing based on time zone
- [ ] High availability: an active/passive pair
  - [ ] Virtual IP with keepalived (VRRP), DNS SRV records as an alternative
  - [ ] Continuous database replication (for example Litestream) and sync of voicemail, recordings and prompts
  - [ ] Cluster status and manual failover in the UI
  - [ ] Note: calls in progress are dropped on failover

Design rule from phase 0 on: all state lives in the data directory (`/var/lib/kiestoon`: database plus files),
and every generated configuration (Asterisk, firewall, provisioning) can be rebuilt from the database.
That keeps backups, restores and high availability simple.
