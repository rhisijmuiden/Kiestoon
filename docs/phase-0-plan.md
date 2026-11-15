# Phase 0 build plan

**Status: built.** Everything below is implemented; kept as a record of the design.

Work in progress on branch `feat/phase-0-foundation`. PRs target `testing`; `main` is production.
Branch flow: feature branch → PR into `testing` → CI green → merge → tag `v0.1.0-rc.N` on testing →
after Reinder tests, merge `testing` into `main` and tag `v0.1.0`.

## Done on this branch

- `ROADMAP.md`, `LICENSE` (GPL-3.0), `branding/` (tokens, README, Kiestoon mark)
- `server/` Go module (Go 1.26): `internal/version`, `internal/config`, `internal/store` (SQLite via
  modernc.org/sqlite, WAL, migrations with `PRAGMA user_version`), `internal/auth` (bcrypt cost 12,
  min 12 chars, sessions with sha256-hashed tokens, TOTP via pquerna/otp, 8 recovery codes, IP limiter)

## Still to build

### Backend (`server/`)
- `internal/updates`: query `https://api.github.com/repos/<repo>/releases`; channel `production` =
  newest non-prerelease, `testing` = newest including prereleases; compare with `golang.org/x/mod/semver`.
  Apply = write `<DataDir>/update-request.json` ({version, channel, requestedAt, requestedBy}); a root
  systemd path unit picks it up. Status from `<DataDir>/update-status.json`.
- `internal/api`: chi router, JSON helpers, security headers (CSP `default-src 'self'`, frame DENY,
  nosniff, referrer no-referrer), cookie `kiestoon_session` (HttpOnly, SameSite=Strict, Secure when TLS
  or `KIESTOON_SECURE_COOKIES=true`). Mutating requests must have `Content-Type: application/json`.
  Endpoints:
  - `GET /api/setup/status`, `POST /api/setup` (only when no users; creates superadmin, logs in)
  - `POST /api/auth/login` → `{mfaRequired}`; `POST /api/auth/mfa {code}`; `POST /api/auth/logout`
  - `GET /api/me` → user + `mfaEnrollmentRequired` (admin without TOTP)
  - `POST /api/account/mfa/enroll` → `{secret, otpauthUrl}`; `POST /api/account/mfa/confirm {code}` → `{recoveryCodes}`
  - `PUT /api/account/language {language}`
  - `GET /api/system/info`; `PUT /api/system/channel {channel}` (admin)
  - `POST /api/system/updates/check`; `POST /api/system/updates/apply` (admin); `GET /api/system/updates/status`
  - Admins without MFA get 403 `mfa_enrollment_required` on everything except me/enroll/confirm/logout/info.
  - Login limiter: 5 failures per IP per 10 minutes → 429. Audit log entries for login, MFA, channel, update.
- `internal/web`: `//go:embed all:dist`, SPA fallback to index.html; `make` copies `web/dist` into it.
- `cmd/kiestoon/main.go`: load config, open DB, hourly session purge, graceful shutdown.

### Frontend (`web/`)
Vite + React + TypeScript, Tailwind v4 (`@tailwindcss/vite`), react-router, i18next (en default, nl),
lucide-react icons, `qrcode` for the TOTP QR, fonts bundled via @fontsource (Inter, Space Grotesk,
JetBrains Mono). Look = the approved UI sketch https://claude.ai/artifact/UmKQFN3z6qx7Czwy6xthEm
(dark, cyan/indigo, left menu, "Powered by RHIS" bottom-right with `rhis-logo-wit.svg` from the RHIS
house style artifact https://claude.ai/artifact/JphZ8zRMCYKVhtPw69GkV3).
Screens: setup wizard, login + MFA step, MFA enrollment (forced for admins, shows recovery codes),
layout with full left menu (items for later phases open a "coming in phase N" page), dashboard
skeleton, Info dialog (version, commit, channel switch, check for updates, update now, status).

### Packaging
- `packaging/nfpm.yaml` → `.deb` (binary in `/usr/bin/kiestoon`, systemd units, env file
  `/etc/kiestoon/kiestoon.env`, user `kiestoon`, data `/var/lib/kiestoon`)
- `packaging/systemd/`: `kiestoon.service` (hardened), `kiestoon-updater.path` + `.service`,
  `packaging/kiestoon-updater` script (download .deb from release, pre-update tar.zst backup of data dir,
  `apt install`, restart, write status)
- `install.sh` for Debian 13 (`--channel production|testing`)
- `lxc/build-lxc.sh`: debootstrap trixie rootfs + .deb → Proxmox `.tar.zst` template
- `.github/workflows/ci.yml` (go vet/test, web build, deb build) and `release.yml` (on tag `v*`:
  build, attach .deb + LXC template + checksums, prerelease when tag has `-`)
