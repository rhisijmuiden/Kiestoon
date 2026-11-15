# Kiestoon Softphone for Windows

A small Windows app (Tauri 2, WebView2) around the `/desktop` pages of a Kiestoon portal. The calling
itself (SIP over a secure WebSocket, WebRTC audio with Opus) runs in those pages, served by the
user's own Kiestoon server, so the softphone always matches its server. The app adds:

- **Linking**: a pairing link or code from the portal (`kiestoon://pair?server=…&code=…&fp=…`), signing
  in inside the app (password, MFA, passkey, SSO) or in the normal browser (`/desktop-link`, for SSO).
  The server gives the app a desktop session (30 days, extended while used) that can only use the
  user portal API.
- **Certificate pinning** for servers with a self-signed certificate (LAN): the app pins exactly
  that certificate (SHA-256 of the DER, as the portal shows it). Servers with a public certificate
  are checked normally.
- Tray icon with presence and queue sign-in/out, incoming-call popup, notifications for missed calls
  and voicemail, headset buttons and lights (HID telephony: Jabra, Poly, EPOS, Yealink, …), `tel:`,
  `sip:` and `callto:` links, start with Windows.
- **Updates** from the GitHub releases (Production: stable releases, Testing: release candidates),
  checked against `SHA256SUMS` before the MSI runs.

## Build

Needs Rust, Node 22 and, for the MSI, Windows with the WiX toolset (Tauri downloads it).

```sh
cd desktop
npm ci
KIESTOON_VERSION=v0.7.0 npm run build   # -> src-tauri/target/release/bundle/msi/*.msi
```

CI builds the MSI on every release (`.github/workflows/release.yml`), puts it in the release and
in the Kiestoon server package, so the portal offers it for download at
`/download/kiestoon-softphone.msi`.

### Code signing

Without a certificate the MSI is unsigned and Windows SmartScreen warns once. To sign, add these
repository secrets and the release workflow signs the app and the MSI with Azure Trusted Signing:
`AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET`, `AZURE_SIGNING_ENDPOINT`,
`AZURE_SIGNING_ACCOUNT`, `AZURE_SIGNING_PROFILE`.

## Tests

```sh
cd desktop/src-tauri && cargo test
```
