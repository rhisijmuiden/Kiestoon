// Makes the Google Play phone screenshots of the Kiestoon app from the documentation demo office.
// The /desktop pages are the screens of the app; a stand-in for the Tauri bridge and an Android
// user agent make them render exactly as in the app (tab bar at the bottom).
//
// Usage (see mobile/store/README.md):
//   tools/docs/demo.sh build/kiestoon-linux-amd64 /tmp/kiestoon-demo 8090
//   node mobile/store/screenshots.mjs http://127.0.0.1:8090 /tmp/kiestoon-demo mobile/store/android
// Writes <out>/<en-US|nl-NL>/images/phoneScreenshots/<n>_<name>.png at 1080 x 1920.
import fs from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'
import { Client, loginAdmin } from '../../tools/docs/lib.mjs'

const { chromium } = createRequire(import.meta.url)('playwright')
const [base, work, out] = process.argv.slice(2)

const shots = [
  { name: 'colleagues', path: '/desktop', theme: 'dark' },
  { name: 'keypad', path: '/desktop/keypad', theme: 'dark', run: async (p) => {
    for (const d of '0201234567') await p.keyboard.press(d).catch(() => {})
  } },
  { name: 'queues', path: '/desktop/queues', theme: 'light' },
  { name: 'voicemail', path: '/desktop/voicemail', theme: 'dark' },
  { name: 'history', path: '/desktop/history', theme: 'light' },
]
const langs = [['en', 'en-US', 'en-GB'], ['nl', 'nl-NL', 'nl-NL']]

const fakeShell = () => {
  const answers = {
    app_info: { version: '0.7.0', channel: 'production', os: 'android', autostart: false, server: location.origin, fingerprint: '' },
    'plugin:kiestoon-call|registration': { platform: 'fcm', token: 'demo', sandbox: false, appId: 'nl.rhis.kiestoon', name: 'Pixel', pushReady: true },
    'plugin:kiestoon-call|take_pending': {},
  }
  window.__TAURI__ = {
    core: {
      invoke: async (cmd) => answers[cmd] ?? null,
      addPluginListener: async () => ({ unregister: async () => {} }),
    },
    event: { listen: async () => () => {} },
  }
}

// The demo has no Asterisk: answer the SIP registration of the softphone so the app shows "Available".
async function fakeRegistrar(ctx) {
  await ctx.routeWebSocket(/\/ws|sip/i, (ws) => {
    ws.onMessage((msg) => {
      const text = String(msg)
      if (!text.startsWith('REGISTER ')) return
      const lines = text.split('\r\n')
      const keep = lines.filter((l) => /^(Via|From|Call-ID|CSeq):/i.test(l))
      const to = lines.find((l) => /^To:/i.test(l)) ?? ''
      const contact = lines.find((l) => /^Contact:/i.test(l)) ?? ''
      ws.send(['SIP/2.0 200 OK', ...keep, `${to};tag=demo`, `${contact.replace(/;expires=\d+/, '')};expires=600`, 'Content-Length: 0', '', ''].join('\r\n'))
    })
  })
}

const api = new Client(base)
const secret = fs.readFileSync(path.join(work, 'mfa-secret'), 'utf8').trim()
const browser = await chromium.launch()
const host = new URL(base).hostname
let count = 0
for (const [lang, dir, locale] of langs) {
  const folder = path.join(out, dir, 'images', 'phoneScreenshots')
  fs.rmSync(folder, { recursive: true, force: true })
  fs.mkdirSync(folder, { recursive: true })
  for (const theme of ['dark', 'light']) {
    // The softphone pages turn the session into a desktop session: sign in fresh for every pass.
    api.cookie = ''
    await loginAdmin(api, secret)
    await api.put('/api/account/language', { language: lang })
    await api.put('/api/account/theme', { theme })
    const [name, value] = api.cookie.split(';')[0].split('=')
    const ctx = await browser.newContext({
      viewport: { width: 360, height: 640 }, deviceScaleFactor: 3, isMobile: true, hasTouch: true,
      userAgent: 'Mozilla/5.0 (Linux; Android 15; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0 Mobile Safari/537.36',
      locale, timezoneId: 'Europe/Amsterdam', colorScheme: theme, reducedMotion: 'reduce',
    })
    await ctx.addCookies([{ name, value, domain: host, path: '/', httpOnly: true }])
    await ctx.addInitScript(fakeShell)
    await fakeRegistrar(ctx)
    await ctx.addInitScript(([t]) => { try { localStorage.setItem('kiestoon.theme', t) } catch {} }, [theme])
    const page = await ctx.newPage()
    for (const [i, s] of shots.entries()) {
      if (s.theme !== theme) continue
      await page.goto(base + s.path, { waitUntil: 'domcontentloaded' })
      await page.waitForLoadState('networkidle', { timeout: 3000 }).catch(() => {})
      await page.waitForTimeout(1500)
      if (s.run) await s.run(page)
      await page.waitForTimeout(500)
      await page.screenshot({ path: path.join(folder, `${i + 1}_${s.name}.png`), animations: 'disabled' })
      count++
    }
    await ctx.close()
  }
}
await browser.close()
console.log(`${count} screenshots in ${out}`)
