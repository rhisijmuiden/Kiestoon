# Documentation tools

The manual in `docs/` (MkDocs Material, English `page.md` and Dutch `page.nl.md`) is published on GitHub Pages by
`.github/workflows/docs.yml`. Its screenshots are not committed: every build makes them from a demo office, so they
always match the portal of that commit.

| File | What |
| --- | --- |
| `demo.sh` | Starts a throwaway Kiestoon with a fake Asterisk manager and a fake root helper, and fills it with the demo office "Noordkade" |
| `fake-ami.mjs` | Answers the Asterisk manager actions the portal uses: live calls, registrations, trunks, queues, call records |
| `fake-helper.mjs` | Answers the root helper: firewall, fail2ban, internet access, WireGuard, cluster, backups |
| `seed.mjs` | Creates the demo office through the API: extensions, trunks, queues, IVR, phones, users and more |
| `dress.mjs` | Adds what the API cannot make: paired edges, a linked system, voicemail, recordings, faxes, backup runs |
| `shots.mjs` | The list of screenshots: page, element and the clicks before the shot |
| `capture.mjs` | Takes the screenshots with Playwright, per language and theme |
| `check-help.mjs` | Checks that every Help link of the portal (`web/src/components/help.tsx`) exists in the built site |
| `site/hooks.py` | MkDocs hook: `![Caption](shot:name)` becomes a light and a dark screenshot in the reader's language |

All names, numbers and addresses in the demo office are made up (`*.example`).

## Preview locally

```sh
make build VERSION="$(cat VERSION)"
tools/docs/demo.sh build/kiestoon-linux-amd64 /tmp/kiestoon-demo 8090
npm install --no-save --prefix tools/docs playwright && npx --prefix tools/docs playwright install chromium
node tools/docs/capture.mjs http://127.0.0.1:8090 /tmp/kiestoon-demo docs/assets/screenshots   # about 8 minutes
pip install -r tools/docs/requirements.txt
mkdocs serve                                      # http://127.0.0.1:8000
```

`capture.mjs` takes `--langs en`, `--themes light` and `--only dashboard,ivr-canvas` to make a subset. The demo
admin is `admin` / `Kiestoon-docs-2026`; the MFA secret is in `<work-dir>/mfa-secret`.

## Writing pages

- Every page exists twice: `page.md` and `page.nl.md`. Use the exact labels of the portal (`web/src/i18n/`).
- A new portal page needs a screenshot in `shots.mjs`, a Help link in `web/src/components/help.tsx`, and a nav entry
  plus Dutch translation in `mkdocs.yml`.
- Section ids that the Help button links to are listed in `help.tsx`; the docs build fails when one goes missing.
