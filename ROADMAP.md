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

Goal: the first real phone call. Released as `v0.2.0-rc.1` on the testing channel.

- [x] Asterisk 22 LTS built as our own `.deb` in CI, configuration generated from the database
- [x] Number plan wizard: 3, 4 or 5 digits, with suggested ranges and a migration tool
- [x] Extensions / users, SIP credentials (strong, generated), per-device TLS + SRTP option
- [x] SIP trunks (registration and IP authentication) with presets for common Dutch providers
- [x] Inbound routes per DID, outbound routes with prefixes and failover
- [x] Emergency number 112 always allowed, with the right caller ID
- [x] Voicemail with voicemail-to-email (audio attachment)
- [x] Custom SMTP mail server: STARTTLS / TLS, username + password or OAuth2 (Microsoft 365, Google), test button
- [x] Call detail records (CDR): search, filters, CSV export
- [x] Firewall (nftables) managed from the UI: SIP IP whitelist (IPs and CIDR ranges), RTP range, safe mode with automatic rollback
- [x] fail2ban for SIP and portal logins, bans visible and removable in the UI
- [x] Phone provisioning: Yealink, Snom, Grandstream (HTTPS, DHCP option 66, per-MAC config, visual key layout)
- [x] Backups: full and incremental (restic), to FTP, FTPS, SFTP and SCP, with schedules, retention and restore from the remote location
- [x] Live dashboard via AMI: active calls, registered phones, trunk status, calls today

Notes on phase 1:

- Asterisk 22.12 is built in CI for amd64 and arm64 as `kiestoon-asterisk` and published with every release.
  The updater installs both packages.
- SCP backups use the same SSH connection as SFTP (restic over rclone). Backups are always encrypted;
  the key per location can be shown in the UI and is needed to restore on a new server.
- Firewall, fail2ban, backups and restore run through a small root helper (`kiestoon-helper.service`);
  the portal itself runs unprivileged.
- Grandstream phones get their accounts now; the visual key layout for Grandstream followed in phase 2.
- Voice prompts were English in phase 1. Dutch prompts followed with the IVR builder in phase 2.

## Phase 2 · Business features

Goal: a complete business phone system, including the reception. Released as `v0.3.0-rc.1` on the testing channel.

- [x] Call queues: ring strategies, music on hold, position announcements, overflow, agent login/logout
- [x] Hunt groups / ring groups: simultaneous, sequential, rotating (and paging)
- [x] Visual IVR builder (digital receptionist): menus, prompts, time conditions, transfer, voicemail, queue, dial-by-extension
- [x] Time conditions and holiday calendar (Dutch holidays preloaded), manual day/night switch
- [x] SSO for admins and users: OpenID Connect (Microsoft Entra ID, Google Workspace, Keycloak, Authentik), IdP groups mapped to roles
- [x] Roles: superadmin, admin, reception, user, with per-role permissions
- [x] User portal (SSO):
  - [x] call forwarding (always, busy, no answer), follow-me to mobile, do not disturb, time-based profiles
  - [x] voicemail in the browser, greetings, email notifications
  - [x] WebRTC softphone: call and be called with a headset from the browser
  - [x] hunt group status, queue sign-in / sign-out and live queue status
  - [x] own call history and recordings, BLF overview of colleagues with click-to-call
  - [x] own keys (BLF, speed dial), SIP details / QR code for a mobile softphone
- [x] Call recording: per user, per queue or on demand, with retention period (GDPR)
- [x] BLF / presence, call pickup groups, call parking, intercom and paging
- [x] Conference rooms with PIN
- [x] Outbound fraud protection: block premium and international destinations per user, max concurrent outbound calls, alert mail on unusual traffic
- [x] Dutch voice prompts (`kiestoon-sounds-nl`, generated with Piper from an open CC0 voice)
- [x] Visual key layout for Grandstream (memory keys 1 to 7)

Notes on phase 2:

- Everything is stored in the database; queues, IVRs, time conditions and feature codes are generated into the
  Asterisk dialplan. Feature codes (*76, *72, *45, *28 …) report back to the portal, so the portal and the phone
  always show the same state.
- The WebRTC softphone connects through the portal itself (`/api/webrtc/ws`), so no extra port is opened.
  Browsers only allow the microphone over https: use the portal on port 8443 (or 443 with internet mode in phase 3).
- Single sign-on uses OpenID Connect with PKCE. Admins who sign in with SSO can skip the Kiestoon MFA code
  when the provider enforces MFA ("trust the provider's MFA").
- Dutch prompts cover voicemail, queues, conference rooms, feature codes, numbers and dates. Anything missing
  falls back to English. Own IVR and queue prompts are uploaded or recorded in the browser.
- The Grandstream memory-key mapping (P-values) follows the vendor documentation and still needs a check on real hardware.
- Fraud alerts count the outbound calls of the last hour after every outbound call; "block on alert" stops all outbound calls except 112 until an admin unblocks.

## Phase 3 · Integrations

Goal: safely connected to the internet and to the rest of the business. Released as `v0.4.0-rc.1` on the testing channel.

- [x] Internet mode: Caddy with automatic Let's Encrypt certificates on 443, WebRTC over WSS
- [x] Geo-blocking (country lists, updated monthly)
- [x] SAML 2.0 next to OIDC
- [x] OneDrive as backup destination
- [x] Phone provisioning: Fanvil, Gigaset (N670/N870 Pro DECT), Poly (VVX, Edge)
- [x] Fax-to-email: receive faxes (T.38 / G.711 via SpanDSP), convert to PDF, mail per fax number
- [x] Queue wallboard for the reception
- [x] Central phone book with XML phone book for phones and LDAP
- [x] REST API with tokens, webhooks (for example incoming call to CRM)
- [x] Passkeys / WebAuthn for users

Notes on phase 3:

- Internet mode: the root helper writes `/etc/caddy/Caddyfile` and starts Caddy on 80/443 in front of the portal.
  Caddy stays off until it is switched on in the portal. Behind Caddy the portal trusts `X-Forwarded-For` only from
  localhost. The softphone then works on `https://<domain>` (WSS through Caddy), so port 8443 is no longer needed
  for it. The Let's Encrypt certificate can also be used for SIP over TLS; it is copied after every renewal.
- Geo-blocking uses the free DB-IP Lite country database (CC BY 4.0), downloaded monthly. Private networks, the trunk
  providers and the SIP/admin allow lists are always let through, and without country data no country rule is
  applied, so nobody is locked out by an empty list. Port 80 stays open for Let's Encrypt.
- SAML 2.0 is a provider kind next to OIDC (metadata by URL or pasted XML). Kiestoon signs its requests with its own
  key; the metadata and ACS addresses are shown on the SSO page.
- OneDrive signs in with the Microsoft device code flow (an Entra app registration with public client flows).
  The refresh token is renewed before every backup run.
- Fax: a fax box is a destination for inbound routes. Faxes arrive as TIFF through SpanDSP, are converted to PDF with
  `tiff2pdf` and mailed through the mail queue. T.38 is switched on per trunk. Sending faxes followed in phase 4.
- Phone book: contacts are served as XML phone books (Yealink, Snom, Grandstream, Fanvil, through provisioning) and as
  a read-only LDAP directory (389/636, own network only). Incoming callers in the phone book get their name shown.
- The REST API is the same API the portal uses. Tokens are stored as hashes, never have more rights than their owner
  and cannot manage users, tokens, webhooks, updates or restores. Webhooks are signed with HMAC-SHA256.
- Passkeys (WebAuthn, discoverable credentials with user verification) count as the second factor. They need a host
  name, not an IP address.
- Not yet tested on real hardware: Fanvil, Gigaset and Poly provisioning (built after the vendor documentation), and
  still the Grandstream memory keys from phase 2.

## Phase 4 · Extensions

Goal: fit into existing networks and save work for the installer. Released as `v0.5.0-rc.1` on the testing channel.

- [x] Support for running behind your own reverse proxy (trusted proxies)
- [x] Zero-touch provisioning via vendor redirection services (Yealink RPS automatic; Snom SRAPS, Grandstream GDMS, Fanvil FDPS, Htek, Poly, Gigaset and Cisco guided)
- [x] Firmware management per phone model
- [x] Provisioning: Htek, Akuvox, Cisco 8800 MPP
- [x] Local text-to-speech (Piper) for own IVR prompts typed in the portal, speech-to-text for voicemail
- [x] Email-to-fax / send a fax from the portal (depends on T.38 support of the SIP provider)
- [x] More backup destinations: S3, WebDAV, SMB
- [x] Optional Prometheus metrics
- [x] Research: multi-tenant with Kamailio in front ([docs/development/multi-tenant-research.md](docs/development/multi-tenant-research.md))

Notes on phase 4:

- Reverse proxy: trusted proxy addresses and a public URL under Internet access. `X-Forwarded-For` is only read from
  those addresses (walked from the right), loopback is always trusted. Examples for nginx, Traefik, HAProxy and Caddy
  are in [docs/reverse-proxy.md](docs/reverse-proxy.md).
- Zero-touch: only Yealink RPS has a documented public API; Kiestoon registers and removes Yealink phones there
  automatically when an AccessKey is set. The other vendors have no public API, so the portal shows the address to
  enter in their portal and exports the MAC addresses as CSV for a bulk import.
- Firmware: uploaded once (or downloaded from the vendor site), linked to models and activated. Phones get it from
  `/prov/fw/<random token>/` on their next provisioning, without internet access. A `.zip` is unpacked.
- Speech: the separate `kiestoon-speech` package holds Piper and whisper.cpp (~66 MB). Voices and Whisper models are
  downloaded from the portal (Speech page) with pinned SHA-256 checksums. Typed text becomes a prompt; voicemail is
  transcribed and the text is added to the voicemail e-mail and the user portal. No audio leaves the server.
- Fax sending: a PDF from the portal or by e-mail (IMAP mailbox, allowed senders per fax box, optional PIN, SPF/DMARC
  failures refused) is converted with Ghostscript and sent with SpanDSP, at most two at once, three attempts each.
  The sender gets the result by e-mail.
- Backups: S3 (AWS and compatible providers), WebDAV (Nextcloud, ownCloud) and SMB through rclone, encrypted by
  restic as before.
- Prometheus: `/metrics` with a bearer token or an allow list, off by default. Metrics are listed in
  [docs/api.md](docs/api.md).
- Not yet tested on real hardware or accounts: Htek, Akuvox and Cisco provisioning, firmware rollout per vendor,
  Yealink RPS with a real account, and T.38 fax sending through a provider.

## Phase 5 · Multi-site & high availability

Goal: one phone system across several offices and data centres. Released as `v0.6.0-rc.1` on the testing channel.

- [x] Kiestoon Edge: a satellite for a branch office, as Raspberry Pi image (arm64), LXC template or `.deb`
  - [x] WireGuard tunnel to the central Kiestoon in the data centre, paired with a code from the portal
  - [x] Local SIP proxy and media relay (Kamailio + rtpengine) to solve NAT problems
  - [x] Local provisioning for the phones in that office (proxied to the central Kiestoon)
- [x] Federation of two or more Kiestoon systems over a WireGuard tunnel
  - [x] Shared phone book and number plan with a site prefix
  - [x] Internal calls, transfers and BLF between sites
  - [x] Agents from several sites in one queue
  - [x] Follow-the-sun routing based on time zone
- [x] High availability: an active/passive pair
  - [x] Virtual IP with keepalived (VRRP), DNS (by hand) as an alternative
  - [x] Continuous database replication and sync of voicemail, recordings, prompts, fax and certificates
  - [x] Cluster status, switch-over and manual promotion in the UI
  - [x] Note: calls in progress are dropped on failover

Notes on phase 5:

- Everything between sites runs over WireGuard. The central Kiestoon is the hub (`kt0`, UDP 51820) for edges and
  linked systems; only the hub needs an open port. The HA pair has its own tunnel (`kt-ha`, UDP 51821).
- Pairing codes (`KT1-E-…`, `KT1-F-…`, `KT1-H-…`) hold the portal address, the SHA-256 fingerprint of its certificate
  and a one-time secret (24 hours). Self-signed certificates are safe because the other side pins exactly that one.
- Edge: Kamailio adds a `Path` header on REGISTER, so the central Asterisk reaches office phones back through the edge;
  rtpengine anchors media between the office network and the tunnel. Provisioning through the edge names the edge as
  SIP server. Headless pairing with `kiestoon-edge.txt` on the boot partition. See [docs/edge.md](docs/edge.md).
- Federation: each link is a PJSIP endpoint over TCP through the tunnel; a linked system only routes to its own
  internal numbers, never to its trunks. Phone book every minute, BLF pushed on change. Remote queue agents are Local
  channels with a custom device state. See [docs/multi-site.md](docs/multi-site.md).
- HA: no Litestream: the primary takes a consistent SQLite snapshot (`VACUUM INTO`) within seconds of every change and
  the standby pulls it, plus the files by checksum, through the tunnel. Generated configuration is rebuilt from the
  database. Split brain and data-loss window are described in [docs/high-availability.md](docs/high-availability.md).
- Tested in CI: an edge is paired with a real Kiestoon in Docker (WireGuard, Kamailio, rtpengine), a phone registers
  through it and Asterisk reaches the phone back through the edge. The Raspberry Pi image is built on an arm64 runner.
- Not yet tested on real hardware: the Raspberry Pi image on a Pi, audio through rtpengine with real phones, a VRRP
  failover between two Proxmox hosts, and federation between two systems on different networks.

## Improvements after phase 5

- [x] IVR flow canvas (`v0.6.0-rc.5`): steps are dragged onto a canvas and connected by drawing lines. Every step
  has its own voice prompt with a waveform player: generated with the local text-to-speech, recorded, uploaded or
  picked from the prompts. Steps: keypress, play audio, transfer (extension, ring group, queue, voicemail, submenu,
  external number), time condition with open/closed lines, repeat menu, hang up and no input. Undo/redo, tidy up,
  minimap and a test call that rings your own phone. Existing menus are converted automatically.
- [x] Transfer by drag and drop (`v0.6.0-rc.6`): in the user portal (Colleagues) an employee drags an active call
  onto a colleague, ring group or queue. Transfer directly (blind) or ask first (attended: the target is called while
  the caller waits, then complete or go back to the caller). Works for desk phones and the browser softphone (AMI
  BlindTransfer/Atxfer). The switchboard drags any live caller onto a target (redirect). A Transfer button and pick
  mode make it work with keyboard and touch too.
- [x] Welcome e-mail for new extensions (`v0.6.0-rc.7`): an extension created with an e-mail address gets a portal
  account (role user) linked to it, and the employee gets a welcome e-mail in English or Dutch with a one-time link
  to set a password (72 hours, no password in the mail), the portal addresses (http, https for the softphone, or the
  public domain), the username and extension, the SIP details for a mobile app and voicemail info, plus a note when
  SSO is enabled. Admins can send it again from Extensions or Users & roles. Without a mail server the admin gets
  the link to hand over.
- [x] Link provisioned phones to extensions (`v0.6.0-rc.8`): a phone that fetches its configuration with the
  provisioning password but whose MAC address is not known yet shows up under "Waiting for an extension", with its
  vendor and model recognised from the file name and User-Agent. Pick an extension and link it; phones without an
  extension show up there too. Any phone can be moved to another extension or unlinked, from Phones or from the
  phone button on Extensions. After a change Kiestoon sends the vendor's SIP NOTIFY (check-sync, resync for Cisco)
  through the account the phone is registered with, so it fetches the new configuration right away; a phone that is
  not registered yet picks it up at its next provisioning check or after a reboot.
- [ ] Windows softphone (`v0.6.0-rc.11`, Kiestoon Softphone, `desktop/`): a Tauri app around new compact
  `/desktop` pages of the portal. Colleagues with live presence, phone book and favourites, call history, keypad,
  voicemail, queue sign-in/out and pause, transfer by dragging the call onto a colleague (direct or ask first),
  incoming-call popup, tray icon with presence and queues, notifications, USB headset buttons and lights (HID
  telephony), tel:/sip: links, start with Windows, Info with update check and install (Production/Testing).
  Linking: "Link this computer" in the portal (pairing link or code), signing in inside the app, or in the browser
  for SSO; a desktop session lasts 30 days and can only use the user portal API. Self-signed servers are pinned
  by certificate fingerprint. The MSI is built by CI, attached to the release and served by the portal at
  `/download/kiestoon-softphone.msi`. Signing needs Azure Trusted Signing secrets. Not yet tested on Windows
  hardware with real headsets.
- [x] Phone keys, expansion modules and intercom (`v0.6.0-rc.12`): a visual key editor per model (keys left and
  right of the display, pages, drag a key onto another to swap) with line keys per account, BLF, speed dial, park,
  parking slot, intercom, do not disturb and voicemail keys. Desk phones carry extra lines (Yealink, Snom,
  Grandstream accounts 2 and up). Expansion modules: Yealink EXP40/EXP43/EXP50, Snom D7 and Grandstream GBX20, with
  their own keys. Key templates per model are applied to many phones at once; phones fetch new keys right away
  (check-sync). Intercom (*80 or a key) now only reaches known extensions that are free and not on DND, on all their
  devices including the browser, and each phone has intercom options (auto-answer, warning tone, mute, during a call).
  Employees edit the keys of their own phone and modules in the portal.
- [x] Intercom auto-answer on every brand (`v0.6.0-rc.13`): the phone that is called with *80 or an intercom key
  answers on its speaker by itself, pushed through provisioning and switched per phone. Yealink, Snom and Fanvil
  (warning tone, muted microphone, also during a call), Poly (muted microphone), Grandstream, Htek, Cisco MPP and
  Akuvox door and loading bay stations (on or off). Snom now gets the correct answer_after_policy values.
- [x] Manual and Help button (`v0.6.0-rc.14`): a complete manual in English and Dutch on GitHub Pages
  (https://rhisijmuiden.github.io/Kiestoon/, MkDocs Material in the Kiestoon house style) covering install, first
  setup, every telephony feature, the user portal, the Windows softphone, branch offices, linked systems, high
  availability, security and the system pages. Screenshots in light and dark, per language, are made in every docs
  build from a demo office (`tools/docs/`). Every portal page has a Help button that opens the matching page and
  section of the manual in the user's language. The README (also in Dutch, `README.nl.md`) shows the highlights. A
  backup location can now be added with an existing encryption key, to restore backups on a new server.
- [x] Call journey (`v0.6.0-rc.16`): click a call in the call history (or under My calls in the portal) to see
  every step as a flow in the style of the IVR builder: trunk and number called, time condition, IVR keys and
  timeouts, queue position and wait, every phone that rang and who missed it, who answered and the talk time,
  transfers (also drag and drop), voicemail, recordings and who hung up. Built from Asterisk CEL over AMI, dial and
  queue events and dialplan markers, stored per linkedid; retention setting under Call features (default 90 days,
  0 = off). Older calls get a short version from the call record.
- [x] Trusted networks (`v0.6.0-rc.17`): on the Firewall page, IP ranges (IPv4/IPv6 CIDR with a description) from
  which password sign-ins skip the MFA step, per role (super admins off by default). Off by default, every skip in
  the audit log, the session loses the skip when it is used from outside the network, X-Forwarded-For only from
  trusted proxies, ranges wider than /8 refused and a warning for public ranges. SSO and API tokens are not affected.

## Phase 6 · AI, wallboard and mobile

Goal: smarter calls, insight for the team lead and the phone system in your pocket. Released as `v0.7.0-rc.N` on
the testing channel.

- [x] Supervisor (`v0.7.0-rc.1`): listen, whisper (coach) and barge in on queue calls, from the switchboard and
  with `*33` / `*34` / `*35` + extension on desk phones; per queue switched on, for reception and admins with an
  extension, only queue calls (never other calls of the agent), every use in the audit log
- [x] Queue callback (`v0.7.0-rc.1`): the caller presses 1, confirms or types a number, hangs up and keeps their
  place; when an agent is free the agent is connected first and then the caller is called back, with retries, an
  expiry time, own caller ID and a callback list on the switchboard (cancel, retry, add by hand)
- [x] Live wallboard v2 for a TV (`v0.7.0-rc.4`): per queue waiting (with callbacks), longest wait, service level
  ring, today's figures and calls per hour, agents with state timers, three rotating layouts, TV links without
  login (secret key, new key, revoke, hide caller names), dark and light; supervision buttons in the portal
- [x] SLA alerts (`v0.7.0-rc.4`): thresholds per queue (target time, service level norm, waiting callers, longest
  wait, abandoned share) shown on the wallboard and switchboard and sent by e-mail with a quiet period, only during
  opening hours and a mail when recovered (push follows with the mobile app)
- [x] Scheduled reports by e-mail (`v0.7.0-rc.5`): daily, weekly or monthly per queue, agent and hour (answered,
  abandoned, waiting time, service level, talk time, SLA alerts) compared with the period before, as HTML mail with
  CSV; preview and "send now to me"
- [x] AI (`v0.7.0-rc.2`): transcription of recorded calls (whisper.cpp, local), summaries, topic, mood and
  follow-ups of calls and voicemail, by a local model (llama.cpp with Qwen3, default) or Claude with the
  installation's own API key; shown with recordings, in the portal, the softphone and the voicemail mail
- [x] Speech IVR (`v0.7.0-rc.3`): a "speech question" step in the IVR builder asks what the caller wants,
  transcribes the answer locally (Whisper, via a FastAGI listener on localhost) and follows the line of the
  answer whose keywords match, optionally helped by the AI engine; "not understood" as fallback
- [ ] Mobile app (iOS and Android), the counterpart of the Windows softphone: calls with push notifications when
  the app is closed (CallKit / Android ConnectionService), colleagues and presence, voicemail, history, queues
  - [x] Push relay and server side (`v0.7.0-rc.6`): stateless relay `kiestoon-push` (APNs VoIP + FCM, sealed
    handles, ed25519-signed requests per installation, content end-to-end encrypted per phone), QR linking in
    the portal, wake-up dialplan (`<number>-app` endpoint, waits for the woken app), voicemail push with AI summary
  - [x] App shell (`v0.7.0-rc.7`): Tauri 2 app in `mobile/` that reuses the softphone pages with a tab bar;
    native plugin for push (FCM, PushKit), the call screen (Android full-screen call notification, iPhone
    CallKit) and calls kept alive in the background; separate iPhone tokens for calls and notifications;
    CI builds (debug APK, iPhone simulator)
  - [ ] Store release (needs the RHIS Apple and Google developer accounts, Firebase project, push.rhis.nl)
- [ ] WhatsApp and SMS in the portal next to calls: a shared inbox per team or number, conversations linked to the
  phone book and the call history, notifications
- [x] Yealink T42G, T42S and T42U (`v0.7.0-rc.8`): provisioning, 15 line keys in 3 pages of 5, extra lines
  (12 on T42S/T42U, 3 on T42G), model detection from the User-Agent

Design rule from phase 0 on: all state lives in the data directory (`/var/lib/kiestoon`: database plus files),
and every generated configuration (Asterisk, firewall, provisioning) can be rebuilt from the database.
That keeps backups, restores and high availability simple.
