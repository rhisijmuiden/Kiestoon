// Takes the documentation screenshots from the demo office (see demo.sh) with a headless browser.
// Usage: node capture.mjs <base-url> <work-dir> <out-dir> [--langs en,nl] [--themes light,dark] [--only name,name]
// Writes <out-dir>/<lang>/<theme>/<name>.png
import fs from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'
import { Client, loginAdmin } from './lib.mjs'
import { shots } from './shots.mjs'

// require() so a globally installed Playwright (NODE_PATH) works as well as a local one.
const { chromium } = createRequire(import.meta.url)('playwright')

const [base, work, out, ...rest] = process.argv.slice(2)
const opt = (k, d) => { const i = rest.indexOf(`--${k}`); return i >= 0 ? rest[i + 1].split(',') : d }
const langs = opt('langs', ['en', 'nl'])
const themes = opt('themes', ['light', 'dark'])
const only = opt('only', null)

const api = new Client(base)
const secret = fs.readFileSync(path.join(work, 'mfa-secret'), 'utf8').trim()
await loginAdmin(api, secret)
// The softphone pages turn the browser session into a desktop session, which ends the portal session.
// Sign in again before a pass when that happened.
async function signedIn() {
  api.quiet = true
  const r = await api.get('/api/me')
  api.quiet = false
  if (r.status === 401) await loginAdmin(api, secret)
  return api.cookie.split(';')[0].split('=')
}
const browser = await chromium.launch()
const host = new URL(base).hostname

let count = 0
for (const lang of langs) {
  for (const theme of themes) {
    const [name, value] = await signedIn()
    await api.put('/api/account/language', { language: lang })
    await api.put('/api/account/theme', { theme })
    const ctx = await browser.newContext({ viewport: { width: 1360, height: 850 }, deviceScaleFactor: 2, locale: lang === 'nl' ? 'nl-NL' : 'en-GB', timezoneId: 'Europe/Amsterdam', colorScheme: theme, reducedMotion: 'reduce' })
    await ctx.addCookies([{ name, value, domain: host, path: '/', httpOnly: true }])
    await ctx.addInitScript(([t]) => { try { localStorage.setItem('kiestoon.theme', t) } catch {} }, [theme])
    const page = await ctx.newPage()
    for (const s of shots) {
      if (only && !only.includes(s.name)) continue
      if (s.themes && !s.themes.includes(theme)) continue
      try {
        await page.setViewportSize(s.viewport ?? { width: 1360, height: 850 })
        // Pages poll the API every few seconds, so "network idle" may never come: wait briefly instead.
        await page.goto(base + s.path, { waitUntil: 'domcontentloaded' })
        await page.waitForLoadState('networkidle', { timeout: 2500 }).catch(() => {})
        await page.waitForTimeout(500)
        if (s.run) await s.run(page, { lang, theme })
        await page.waitForTimeout(s.wait ?? 350)
        const file = path.join(out, lang, theme, `${s.name}.png`)
        fs.mkdirSync(path.dirname(file), { recursive: true })
        const target = s.selector ? page.locator(s.selector).first() : page
        await target.screenshot({ path: file, fullPage: !s.selector && !!s.full, animations: 'disabled' })
        count++
      } catch (e) {
        console.warn(`  ! ${lang}/${theme}/${s.name}: ${e.message.split('\n')[0]}`)
      }
    }
    await ctx.close()
  }
}
await browser.close()
console.log(`${count} screenshots in ${out}`)
