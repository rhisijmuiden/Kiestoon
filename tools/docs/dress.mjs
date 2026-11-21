// Adds what the API cannot create on a machine without Asterisk and real sites: paired branch offices,
// an active linked system, voicemail, recordings, received faxes and a backup history.
// Usage: node dress.mjs <base-url> <data-dir> <wireguard-keys-file>
import crypto from 'node:crypto'
import fs from 'node:fs'
import path from 'node:path'
import { DatabaseSync } from 'node:sqlite'
import { Client, loginAdmin } from './lib.mjs'
import { wav } from './wav.mjs'

const [base, dataDir, keysFile] = process.argv.slice(2)
const db = new DatabaseSync(path.join(dataDir, 'kiestoon.db'))
db.exec('PRAGMA busy_timeout = 5000')
const iso = (msAgo) => new Date(Date.now() - msAgo).toISOString().replace(/\.\d+Z$/, 'Z')
const key = (s) => Buffer.from(s.padEnd(32, '=')).toString('base64').slice(0, 43) + '='

// Branch offices: paired, online, with their services running.
const edgeKeys = []
const edges = db.prepare('SELECT id, name FROM edge_sites ORDER BY id').all()
edges.forEach((e, i) => {
  const pub = key(`edge-${e.id}-docs`)
  edgeKeys.push(pub)
  db.prepare(`UPDATE edge_sites SET public_key = ?, tunnel_ip = ?, hostname = ?, version = ?, lan_address = ?, lan_net = ?, services = ?,
    last_seen = ?, paired_at = ?, pair_hash = '', pair_expires = '', token_hash = 'docs-demo' WHERE id = ?`).run(
    pub, `10.64.12.${10 + i}`, i ? 'edge-zwolle' : 'edge-groningen', 'v0.6.0-rc.12', `192.168.${20 + i}.2`, `192.168.${20 + i}.0/24`,
    JSON.stringify({ kamailio: 'active', rtpengine: 'active', wireguard: 'active' }), iso(25000), iso(12 * 864e5))
})

// Linked system: active, with its directory.
const peers = db.prepare('SELECT id FROM federation_peers ORDER BY id').all()
for (const p of peers) {
  const pub = key(`fed-${p.id}-docs`)
  edgeKeys.push(pub)
  db.prepare(`UPDATE federation_peers SET state = 'active', public_key = ?, tunnel_ip = '10.64.12.50', local_ip = '10.64.12.1',
    endpoint = '' , version = 'v0.6.0-rc.12', last_seen = ?, invite_hash = '', invite_expires = '', directory = ? WHERE id = ?`).run(
    pub, iso(40000), JSON.stringify({ name: 'Noordkade Antwerpen', timezone: 'Europe/Brussels', version: 'v0.6.0-rc.12', entries: [
      { number: '301', name: 'Lieve Peeters', kind: 'extension' }, { number: '302', name: 'Jonas Maes', kind: 'extension' },
      { number: '303', name: 'Elien Claes', kind: 'extension' }, { number: '360', name: 'Verkoop BE', kind: 'ringgroup' },
    ] }), p.id)
}
fs.writeFileSync(keysFile, JSON.stringify(edgeKeys))

// Voicemail for 204 (the admin's own extension) and 201.
const spool = path.join(dataDir, 'asterisk', 'spool')
const vm = [
  ['204', '"Jansen Bouw" <0612345678>', 3600 * 1000, 34], ['204', '"Gemeente Utrecht" <0301234567>', 5 * 3600 * 1000, 21],
  ['204', '<0687654321>', 26 * 3600 * 1000, 12], ['201', '"De Groot Transport" <0104567890>', 2 * 3600 * 1000, 46],
]
const count = {}
for (const [box, cid, ago, secs] of vm) {
  const dir = path.join(spool, 'voicemail', 'default', box, 'INBOX')
  fs.mkdirSync(dir, { recursive: true })
  const n = (count[box] = (count[box] ?? -1) + 1)
  const name = `msg${String(n).padStart(4, '0')}`
  fs.writeFileSync(path.join(dir, name + '.txt'), `;\n; Message Information file\n;\n[message]\norigmailbox=${box}\ncontext=kiestoon-in\ncallerid=${cid}\norigtime=${Math.floor((Date.now() - ago) / 1000)}\nduration=${secs}\n`)
  fs.writeFileSync(path.join(dir, name + '.wav'), Buffer.from(wav(secs, 3 + n), 'base64'))
}

// Call recordings.
const mon = path.join(spool, 'monitor')
fs.mkdirSync(mon, { recursive: true })
const stamp = (d) => d.toISOString().replace(/[-:]/g, '').replace('T', '-').slice(0, 15)
for (const [i, [from, to, secs]] of [['0612345678', '201', 312], ['201', '0205551234', 95], ['0104567890', '202', 187], ['0402233445', '205', 64], ['203', '0301234567', 141]].entries()) {
  const d = new Date(Date.now() - (i * 47 + 12) * 60000)
  fs.writeFileSync(path.join(mon, `kt-${stamp(d)}-17316${i}.${i}-${from}-${to}.wav`), Buffer.from(wav(Math.min(secs, 30), 11 + i), 'base64'))
}

// The recording of call 44 (see the call journeys in fake-ami.mjs).
fs.writeFileSync(path.join(mon, `kt-${stamp(new Date(Date.now() - 30 * 60000))}-1731644.1-0687654321-q700.wav`), Buffer.from(wav(30, 17), 'base64'))

// AI: transcripts and summaries of voicemail and recorded calls (as if Whisper and the model ran).
const setting = (key, value) => db.prepare('INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value').run(key, value)
setting('speech', JSON.stringify({ tts: { enabled: false, voiceEn: 'en_GB-cori-medium', voiceNl: 'nl_NL-pim-medium' }, stt: { enabled: true, model: 'small-q5_1', language: 'auto', voicemail: true } }))
setting('ai', JSON.stringify({ calls: true, voicemail: true, engine: 'cloud', llm: 'qwen3-4b-instruct', cloudModel: 'claude-opus-5-5', language: 'auto', keepDays: 90 }))
setting('ai_api_key', 'sk-ant-demo-not-a-real-key-7f3a')
const aiDir = path.join(dataDir, 'speech', 'ai')
const trDir = path.join(dataDir, 'speech', 'transcripts')
fs.mkdirSync(aiDir, { recursive: true })
fs.mkdirSync(trDir, { recursive: true })
const sha = (file) => crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex')
const at = new Date().toISOString()
const vmAI = [
  ['Goedemiddag, met Peter van Jansen Bouw. Ik bel over de offerte voor de nieuwe kantoorvleugel. Kunt u mij voor vrijdag terugbellen? Mijn nummer is 06 12 34 56 78.',
    { summary: 'Peter van Jansen Bouw belt over de offerte voor de nieuwe kantoorvleugel en vraagt om voor vrijdag teruggebeld te worden.', actions: ['Peter voor vrijdag terugbellen op 06 12 34 56 78'], topic: 'Offerte kantoorvleugel', sentiment: 'neutral' }],
  ['Goedendag, Gemeente Utrecht, afdeling vergunningen. Uw aanvraag is compleet en wordt deze week behandeld. U hoeft verder niets te doen.',
    { summary: 'De gemeente Utrecht meldt dat de vergunningsaanvraag compleet is en deze week wordt behandeld.', actions: [], topic: 'Vergunningsaanvraag', sentiment: 'positive' }],
]
fs.readdirSync(path.join(spool, 'voicemail', 'default', '204', 'INBOX')).filter((f) => f.endsWith('.wav')).sort().slice(0, 2).forEach((f, i) => {
  const h = sha(path.join(spool, 'voicemail', 'default', '204', 'INBOX', f))
  fs.writeFileSync(path.join(trDir, h + '.txt'), vmAI[i][0])
  fs.writeFileSync(path.join(aiDir, `vm-${h}.json`), JSON.stringify({ status: 'done', summary: vmAI[i][1], engine: 'claude-opus-5-5', at }))
})
const recAI = {
  '-0612345678-201.wav': ['Goedemorgen, met Peter Jansen. Ik bel over de levering van de kozijnen. Die zou vandaag komen, maar ik heb nog niets gezien. ... Ik zoek het voor u uit en bel u voor twaalf uur terug. ... Prima, dank u wel.',
    { summary: 'Peter Jansen mist de levering van de kozijnen die vandaag zou komen. De medewerker zoekt het uit en belt voor twaalf uur terug.', actions: ['Levering kozijnen nagaan bij de vervoerder', 'Peter Jansen voor 12:00 terugbellen'], topic: 'Levering kozijnen', sentiment: 'negative' }],
  '-0104567890-202.wav': ['Hallo, De Groot Transport. We willen de rit van donderdag een uur vervroegen, kan dat? ... Ja, dat lukt, ik zet het om naar acht uur. ... Top, bedankt.',
    { summary: 'De Groot Transport vraagt de rit van donderdag een uur te vervroegen; de medewerker zet hem om naar 8:00.', actions: ['Planning donderdag aanpassen naar 8:00'], topic: 'Rit vervroegen', sentiment: 'positive' }],
}
for (const f of fs.readdirSync(mon)) {
  const k = Object.keys(recAI).find((s) => f.endsWith(s))
  if (k) fs.writeFileSync(path.join(aiDir, `rec-${f}.json`), JSON.stringify({ status: 'done', transcript: recAI[k][0], summary: recAI[k][1], engine: 'claude-opus-5-5', at }))
}

// Received faxes.
const box = db.prepare('SELECT id FROM fax_boxes ORDER BY id LIMIT 1').get()
for (const [ago, caller, name, pages, status, err] of [
  [40 * 60000, '0503344556', 'Elektro Noord', 2, 'success', ''], [3 * 3600000, '0402233445', 'VAN DIJK INSTALL', 1, 'success', ''],
  [27 * 3600000, '0201112233', '', 0, 'failed', 'No fax signal (the caller was not a fax machine)'], [50 * 3600000, '0104567890', 'DE GROOT TRANSPORT', 4, 'success', ''],
]) {
  db.prepare(`INSERT INTO faxes(box_id, received_at, caller, caller_name, did, remote_station, pages, status, error, file, size, mailed)
    VALUES (?, ?, ?, ?, '0507001250', ?, ?, ?, ?, '', ?, ?)`).run(box?.id ?? null, iso(ago), caller, name, name, pages, status, err, pages * 48211, status === 'success' ? 1 : 0)
}

// Queue statistics for the wallboard and the reports: today until now, and the five weeks before on workdays,
// busier around 10:00 and 14:00.
{
  const now = new Date()
  let seed = 7
  const rnd = () => ((seed = (seed * 16807) % 2147483647) / 2147483647)
  const agents = { 1: [['201', 'Sanne Bakker'], ['202', 'Thomas Jansen'], ['204', 'Emma de Boer'], ['209', 'Milan Vos'], ['205', 'Noor Hendriks']], 2: [['203', 'Daan Visser'], ['207', 'Lotte Smit']] }
  const ins = db.prepare(`INSERT INTO queue_calls(queue_id, uniqueid, caller, joined_at, result, hold, talk, agent, agent_name, ended_at) VALUES (?,?,?,?,?,?,?,?,?,?)`)
  const stamp = (d) => d.toISOString().replace(/\.\d+Z$/, 'Z')
  let n = 0
  db.exec('BEGIN')
  for (let back = 35; back >= 0; back--) {
    const start = new Date(now)
    start.setDate(start.getDate() - back)
    start.setHours(8, 0, 0, 0)
    let end = new Date(start)
    end.setHours(18, 0, 0, 0)
    if (back === 0) {
      if (now - start < 3 * 3600000) start.setTime(Math.max(now.getTime() - 8 * 3600000, new Date(now).setHours(0, 5, 0, 0)))
      end = new Date(now.getTime() - 120000)
    } else if (start.getDay() === 0 || start.getDay() === 6) continue
    // Monday mornings are a little busier and slower, so the reports have something to compare.
    const extra = start.getDay() === 1 ? 0.15 : 0
    for (let t = start.getTime(); t < end.getTime(); t += 60000) {
      const h = new Date(t).getHours() + new Date(t).getMinutes() / 60
      const busy = 0.25 + extra + 0.5 * Math.exp(-((h - 10) ** 2) / 2) + 0.35 * Math.exp(-((h - 14) ** 2) / 3)
      for (const q of [1, 2]) {
        if (rnd() > busy * (q === 1 ? 1 : 0.45)) continue
        const r = rnd()
        const result = r < 0.03 ? 'abandoned' : r < 0.045 ? 'left' : 'answered'
        const hold = result === 'answered' ? Math.round(4 + rnd() ** 8 * (q === 1 ? 80 + extra * 200 : 45)) : Math.round(20 + rnd() * 120)
        const [ag, name] = result === 'answered' ? agents[q][Math.floor(rnd() * agents[q].length)] : ['', '']
        const at = new Date(t + rnd() * 50000)
        ins.run(q, `demo.${n++}`, '06' + String(10000000 + Math.floor(rnd() * 89999999)), stamp(at), result, hold,
          result === 'answered' ? Math.round(60 + rnd() * 400) : 0, ag, name, stamp(new Date(at.getTime() + 300000)))
      }
    }
  }
  // A few SLA alerts in the past weeks, all at Support on Monday mornings.
  for (let back = 1; back <= 35; back++) {
    const d = new Date(now)
    d.setDate(d.getDate() - back)
    if (d.getDay() !== 1) continue
    d.setHours(9, 40, 0, 0)
    db.prepare(`INSERT INTO sla_alerts(queue_id, kind, at) VALUES (1, 'wait', ?)`).run(stamp(d))
  }
  db.exec('COMMIT')
}

// Scheduled reports.
for (const [name, period, time, weekday, workdays, queues, sections, rcpt, lang] of [
  ['Weekly overview support', 'weekly', '08:00', 1, 0, '[1,2]', '["queues","agents","hours","trend"]', '["teamlead@example.com"]', 'en'],
  ['Daily summary', 'daily', '18:00', 1, 1, '[]', '["queues","trend"]', '["ops@example.com"]', 'en'],
  ['Maandrapport directie', 'monthly', '07:00', 1, 0, '[]', '["queues","trend"]', '["directie@example.com"]', 'nl'],
]) {
  db.prepare(`INSERT INTO wallboard_reports(name, period, time, weekday, workdays, queues, sections, recipients, language, enabled, last_sent)
    VALUES (?,?,?,?,?,?,?,?,?,1,?)`).run(name, period, time, weekday, workdays, queues, sections, rcpt, lang, iso(3600000))
}

// Wallboard tv links and who gets SLA alerts.
db.prepare(`INSERT INTO wallboard_screens(name, key, queues, layouts, rotate, theme, hide_names, last_seen) VALUES (?,?,?,?,?,?,?,?)`)
  .run('Support room', 'k7Q2m-X9vLr-3e8sW-dPz4a', '[]', '["overview","queue","agents"]', 30, 'dark', 0, iso(60000))
db.prepare(`INSERT INTO wallboard_screens(name, key, queues, layouts, rotate, theme, hide_names, last_seen) VALUES (?,?,?,?,?,?,?,?)`)
  .run('Reception lobby', 'Lb8vR-2qPxN-7mWc4-ZtK9e', '[1]', '["queue"]', 0, 'light', 1, iso(3 * 60000))
{
  const tc = db.prepare('SELECT id FROM time_conditions ORDER BY id LIMIT 1').get()
  db.prepare(`INSERT INTO settings(key, value) VALUES ('wallboard_alerts', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`)
    .run(JSON.stringify({ screens: true, email: true, recipients: ['teamlead@example.com', 'support@example.com'], quiet: 15, timeCondition: tc?.id ?? 0, recovered: true }))
}

// The mobile app: switched on, with a few linked phones.
db.prepare(`INSERT INTO settings(key, value) VALUES ('mobile', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`)
  .run(JSON.stringify({ enabled: true, relayURL: 'https://push.rhis.nl' }))
{
  const users = db.prepare('SELECT id, username FROM users WHERE extension_id IS NOT NULL ORDER BY id').all()
  const phones = [['iPhone 16 Pro', 'apns'], ['Pixel 9', 'fcm'], ['iPhone 15', 'apns'], ['Galaxy S25', 'fcm']]
  users.slice(0, 4).forEach((u, i) => {
    const [name, platform] = phones[i]
    db.prepare(`INSERT INTO mobile_devices(user_id, app_id, name, platform, handle, secret, created_at, last_seen, last_push)
      VALUES (?,?,?,?,?,?,?,?,?)`).run(u.id, `demo-${i}`, name, platform, 'h1.demo', 'demo', iso((12 + i * 5) * 86400000), iso((i + 1) * 1800000), iso((i + 2) * 3600000))
  })
}

// Call quality: a week of measured call legs. Voys is fine until the last two hours, when audio
// from the provider starts to drop (an active alert); phone 204 on Wi-Fi loses audio towards the
// phone and the softphone of 207 at home sends jittery audio. Today's calls of the call history
// (linked ids 17316<i>.1, see fake-ami.mjs) get their own legs, call 44 with a full timeline.
{
  const qs = (d) => d.toISOString().replace(/\.\d+Z$/, 'Z')
  let seed = 11
  const rnd = () => ((seed = (seed * 16807) % 2147483647) / 2147483647)
  // The E-model of the portal (server/internal/quality/mos.go), so the figures agree with each other.
  const mos = (jit, loss, rtt, codec) => {
    const delay = rtt / 2 + jit * 2 + 10
    let r = 93.2 - (delay < 160 ? delay / 40 : (delay - 120) / 10)
    const bpl = codec === 'g729' ? 19 : 25.1
    r -= 95 * loss / (loss + bpl)
    const m = r <= 0 ? 1 : r >= 100 ? 4.5 : 1 + 0.035 * r + 0.000007 * r * (r - 60) * (100 - r)
    return Math.round(Math.max(1, Math.min(4.5, m)) * 100) / 100
  }
  const ins = db.prepare(`INSERT INTO call_quality(linkedid, uniqueid, channel, endpoint, kind, trunk_id, ext, codec, remote, started_at, ended_at,
    mos, mos_min, rx_mos, rx_jitter, rx_jitter_max, rx_loss, rx_reports, tx_mos, tx_jitter, tx_jitter_max, tx_loss, tx_reports, rtt, rtt_max, samples)
    VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
  const leg = (linked, n, endpoint, end, secs, p) => {
    const kind = endpoint.startsWith('trunk-') ? 'trunk' : 'extension'
    const codec = kind === 'trunk' ? 'alaw' : 'g722'
    const rtt = p.rtt * (0.8 + rnd() * 0.4)
    const d = (jit, loss) => {
      const j = jit * (0.7 + rnd() * 0.6), l = Math.max(0, loss * (0.6 + rnd() * 0.8))
      return { jit: Math.round(j * 10) / 10, max: Math.round(j * (1.8 + rnd()) * 10) / 10, loss: Math.round(l * 100) / 100, mos: mos(j, l, rtt, codec) }
    }
    const rx = d(p.rx[0], p.rx[1]), tx = d(p.tx[0], p.tx[1])
    const worst = Math.min(rx.mos, tx.mos)
    const reports = Math.max(1, Math.round(secs / 5))
    const samples = Array.from({ length: Math.min(reports, 120) }, (_, i) => {
      const bad = p.bursts && ((i > reports * 0.25 && i < reports * 0.42) || (i > reports * 0.7 && i < reports * 0.8))
      return Math.round(Math.min(4.41, Math.max(1.2, (bad ? worst - 0.7 - rnd() * 0.4 : worst + 0.25 - rnd() * 0.3))) * 100)
    })
    const start = new Date(end.getTime() - secs * 1000)
    ins.run(linked, `${linked}.${n}`, `PJSIP/${endpoint}-0000${n}a`, endpoint, kind, kind === 'trunk' ? Number(endpoint.slice(6)) : 0,
      kind === 'trunk' ? '' : endpoint.replace(/-(web|app)$/, ''), codec, kind === 'trunk' ? '185.103.76.21' : `192.168.10.${40 + Number(endpoint.slice(0, 3)) - 200}`,
      qs(start), qs(end), worst, Math.min(...samples) / 100, rx.mos, rx.jit, rx.max, rx.loss, reports, tx.mos, tx.jit, tx.max, tx.loss, reports,
      Math.round(rtt * 10) / 10, Math.round(rtt * 1.6 * 10) / 10, JSON.stringify(samples))
  }
  const phone = { '204': { rx: [6, 0.2], tx: [14, 9], rtt: 9 }, '207': { rx: [36, 0.6], tx: [8, 0.2], rtt: 38 } }
  const good = { rx: [3, 0.05], tx: [4, 0.08], rtt: 6 }
  const voys = (end) => (Date.now() - end.getTime() < 2 * 3600000 ? { rx: [8, 8], tx: [5, 0.2], rtt: 24 } : { rx: [4, 0.1], tx: [5, 0.1], rtt: 22 })
  const exts = ['201', '202', '203', '204', '205', '207', '209']
  const now = Date.now()
  db.exec('BEGIN')
  let k = 0
  for (let back = 7 * 24 * 60; back > 12; back -= 6 + Math.floor(rnd() * 14)) {
    const end = new Date(now - back * 60000)
    const h = new Date(end).getHours()
    if (h < 7 || h > 19) { back -= 60; continue }
    const ext = exts[Math.floor(rnd() * exts.length)]
    const secs = 40 + Math.floor(rnd() * 420)
    const linked = `q${k++}.1`
    if (rnd() < 0.82) leg(linked, 1, rnd() < 0.85 ? 'trunk-1' : 'trunk-2', end, secs, rnd() < 0.85 ? voys(end) : { rx: [6, 0.2], tx: [6, 0.2], rtt: 31 })
    leg(linked, 2, ext === '204' ? '204-web' : ext, end, secs, phone[ext] ?? good)
  }
  // Today's calls of the call history; call 44 went from the trunk to 201 and was transferred to 204.
  for (let i = 0; i < 44; i++) {
    const end = new Date(now - (46 - i) * 10 * 60000)
    const ext = exts[(i * 3) % exts.length]
    if (i % 7 !== 3) leg(`17316${i}.1`, 1, 'trunk-1', end, 60 + i * 7, voys(end))
    leg(`17316${i}.1`, 2, ext, end, 60 + i * 7, phone[ext] ?? good)
  }
  const end44 = new Date(now - 25 * 60000)
  leg('1731644.1', 1, 'trunk-1', end44, 190, { rx: [4, 0.1], tx: [5, 0.1], rtt: 22 })
  leg('1731644.1', 2, '201', new Date(end44.getTime() - 60000), 100, good)
  leg('1731644.1', 3, '204-web', end44, 60, { ...phone['204'], bursts: true })
  db.prepare(`INSERT INTO quality_alerts(key, kind, ref, name, calls, poor, mos, jitter, loss, rtt, cause, started_at, mailed_at)
    VALUES ('trunk/1', 'trunk', '1', 'Voys', 6, 5, 3.52, 8, 8.1, 24, 'lossFrom', ?, ?)`).run(qs(new Date(now - 38 * 60000)), qs(new Date(now - 38 * 60000)))
  db.prepare(`INSERT INTO quality_alerts(key, kind, ref, name, calls, poor, mos, jitter, loss, rtt, cause, started_at, ended_at, mailed_at)
    VALUES ('phone/204', 'phone', '204', '204 Emma de Boer', 5, 3, 3.41, 14, 9, 9, 'lossTo', ?, ?, ?)`).run(qs(new Date(now - 26 * 3600000)), qs(new Date(now - 24 * 3600000)), qs(new Date(now - 26 * 3600000)))
  db.exec('COMMIT')
  setting('call_quality', JSON.stringify({ good: 4, poor: 3.6, jitter: 30, loss: 2, rtt: 300, trunk: { enabled: true, minutes: 15, count: 3 },
    phone: { enabled: true, minutes: 60, count: 3 }, eachCall: false, recovered: true, email: true, recipients: ['it@noordkade.example'], quiet: 30, keepDays: 90 }))
}

// A backup history through the (fake) helper.
const c = new Client(base)
await loginAdmin(c, fs.readFileSync(path.join(dataDir, '..', 'mfa-secret'), 'utf8').trim())
const { data: targets } = await c.get('/api/backups/targets')
for (const t of targets) {
  await c.post('/api/backups/run', { targetId: t.id, kind: 'full' })
  await c.post('/api/backups/run', { targetId: t.id, kind: 'incremental' })
}
await new Promise((r) => setTimeout(r, 1500))
console.log('dressed')
