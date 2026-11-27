// Renders the Google Play icon (512 x 512) and feature graphic (1024 x 500, per language) from the
// Kiestoon house style (branding/tokens.json). Run after screenshots.mjs, which the feature graphic uses.
// Usage: node mobile/store/graphics.mjs mobile/store/android
import fs from 'node:fs'
import path from 'node:path'
import { createRequire } from 'node:module'

const { chromium } = createRequire(import.meta.url)('playwright')
const out = process.argv[2] ?? 'mobile/store/android'
const tokens = JSON.parse(fs.readFileSync(new URL('../../branding/tokens.json', import.meta.url), 'utf8'))
const c = tokens.color
const wave = (stroke, width) => `<svg viewBox="0 0 34 34" width="${width}" height="${width}"><path d="M9 17h3l2-6 4 12 2-6h5" fill="none" stroke="${stroke}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>`
const fonts = `<link href="https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@600;700&family=Inter:wght@400;500&display=swap" rel="stylesheet">`

// Google Play rounds the corners itself: the icon is a full square.
const icon = `<html><head>${fonts}</head><body style="margin:0">
<div style="width:512px;height:512px;display:grid;place-items:center;background:radial-gradient(circle at 30% 20%, #12304a, ${c['surface-sidebar']} 70%)">
${wave(c.accent, 440)}</div></body></html>`

const texts = {
  'en-US': { title: 'Your office phone, in your pocket', sub: 'Calls, colleagues, queues and voicemail. Rings even when the app is closed.' },
  'nl-NL': { title: 'Je kantoortelefoon, in je broekzak', sub: 'Bellen, collega’s, wachtrijen en voicemail. Gaat over, ook als de app dicht is.' },
}
const feature = (lang, shot) => `<html><head>${fonts}</head><body style="margin:0">
<div style="position:relative;width:1024px;height:500px;overflow:hidden;background:radial-gradient(ellipse at 15% 0%, #12304a, ${c.bg} 65%);font-family:Inter,sans-serif;color:${c.text}">
  <div style="position:absolute;left:64px;top:96px;width:540px">
    <div style="display:flex;align-items:center;gap:16px">
      <div style="width:64px;height:64px;border-radius:18px;background:${c['surface-sidebar']};border:2px solid ${c.accent};display:grid;place-items:center">${wave(c.accent, 56)}</div>
      <span style="font:700 40px 'Space Grotesk';letter-spacing:-.5px">Kiestoon</span>
    </div>
    <h1 style="font:700 44px/1.1 'Space Grotesk';margin:36px 0 16px;letter-spacing:-.8px">${texts[lang].title}</h1>
    <p style="font:400 21px/1.45 Inter;color:${c['text-muted']};margin:0">${texts[lang].sub}</p>
  </div>
  <div style="position:absolute;right:70px;top:40px;width:300px;height:533px;border-radius:34px;border:8px solid #1e293b;overflow:hidden;box-shadow:0 30px 80px rgba(34,211,238,.18)">
    <img src="data:image/png;base64,${shot}" style="width:100%;display:block">
  </div>
</div></body></html>`

const browser = await chromium.launch()
const page = await browser.newPage({ deviceScaleFactor: 1 })
async function render(html, w, h, file) {
  await page.setViewportSize({ width: w, height: h })
  await page.setContent(html, { waitUntil: 'networkidle' })
  await page.evaluate(() => document.fonts.ready)
  fs.mkdirSync(path.dirname(file), { recursive: true })
  await page.screenshot({ path: file, clip: { x: 0, y: 0, width: w, height: h } })
}
for (const lang of Object.keys(texts)) {
  await render(icon, 512, 512, path.join(out, lang, 'images', 'icon.png'))
  const shot = fs.readFileSync(path.join(out, lang, 'images', 'phoneScreenshots', '1_colleagues.png')).toString('base64')
  await render(feature(lang, shot), 1024, 500, path.join(out, lang, 'images', 'featureGraphic.png'))
}
await browser.close()
console.log(`icon and feature graphics in ${out}`)
