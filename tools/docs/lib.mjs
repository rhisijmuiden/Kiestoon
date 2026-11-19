// Small helpers for the documentation tools: an API client with a cookie and TOTP codes.
import crypto from 'node:crypto'

export function totp(secret, t = Date.now()) {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  let bits = ''
  for (const c of secret.replace(/=+$/, '').toUpperCase()) bits += alphabet.indexOf(c).toString(2).padStart(5, '0')
  const key = Buffer.from(bits.match(/.{8}/g).map((b) => parseInt(b, 2)))
  const counter = Buffer.alloc(8)
  counter.writeBigUInt64BE(BigInt(Math.floor(t / 30000)))
  const h = crypto.createHmac('sha1', key).update(counter).digest()
  const o = h[h.length - 1] & 15
  return String((h.readUInt32BE(o) & 0x7fffffff) % 1e6).padStart(6, '0')
}

export class Client {
  constructor(base) { this.base = base; this.cookie = '' }
  async req(method, path, body) {
    const res = await fetch(this.base + path, {
      method,
      headers: { ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}), ...(this.cookie ? { Cookie: this.cookie } : {}) },
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
    const set = res.headers.getSetCookie?.() ?? []
    if (set.length) this.cookie = set.map((c) => c.split(';')[0]).join('; ')
    const text = await res.text()
    let data = text
    try { data = JSON.parse(text) } catch {}
    if (!res.ok && !this.quiet) console.warn(`  ! ${method} ${path} -> ${res.status} ${text.slice(0, 200)}`)
    return { status: res.status, data }
  }
  get(p) { return this.req('GET', p) }
  post(p, b = {}) { return this.req('POST', p, b) }
  put(p, b = {}) { return this.req('PUT', p, b) }
}

// Signs in as the demo admin. A TOTP code is accepted once, so after a refusal wait for the next 30 seconds.
export async function loginAdmin(c, secret, user = { username: 'admin', password: 'Kiestoon-docs-2026' }) {
  await c.post('/api/auth/login', user)
  for (let i = 0; i < 3; i++) {
    c.quiet = true
    const r = await c.post('/api/auth/mfa', { code: totp(secret) })
    c.quiet = false
    if (r.status === 200) return
    await new Promise((res) => setTimeout(res, 30000 - (Date.now() % 30000) + 200))
  }
  throw new Error('admin sign-in failed')
}
