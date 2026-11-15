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

// Queue statistics of today for the wallboard: calls from the morning until now, busier around 10:00 and 14:00.
{
  const now = new Date(), start = new Date(now)
  start.setHours(8, 0, 0, 0)
  if (now - start < 3 * 3600000) start.setTime(Math.max(now.getTime() - 8 * 3600000, new Date(now).setHours(0, 5, 0, 0)))
  let seed = 7
  const rnd = () => ((seed = (seed * 16807) % 2147483647) / 2147483647)
  const agents = { 1: [['201', 'Sanne Bakker'], ['202', 'Thomas Jansen'], ['204', 'Emma de Boer'], ['209', 'Milan Vos'], ['205', 'Noor Hendriks']], 2: [['203', 'Daan Visser'], ['207', 'Lotte Smit']] }
  const ins = db.prepare(`INSERT INTO queue_calls(queue_id, uniqueid, caller, joined_at, result, hold, talk, agent, agent_name, ended_at) VALUES (?,?,?,?,?,?,?,?,?,?)`)
  let n = 0
  for (let t = start.getTime(); t < now.getTime() - 120000; t += 60000) {
    const h = new Date(t).getHours() + new Date(t).getMinutes() / 60
    const busy = 0.25 + 0.5 * Math.exp(-((h - 10) ** 2) / 2) + 0.35 * Math.exp(-((h - 14) ** 2) / 3)
    for (const q of [1, 2]) {
      if (rnd() > busy * (q === 1 ? 1 : 0.45)) continue
      const r = rnd()
      const result = r < 0.03 ? 'abandoned' : r < 0.045 ? 'left' : 'answered'
      const hold = result === 'answered' ? Math.round(4 + rnd() ** 8 * (q === 1 ? 80 : 45)) : Math.round(20 + rnd() * 120)
      const [ag, name] = result === 'answered' ? agents[q][Math.floor(rnd() * agents[q].length)] : ['', '']
      const at = new Date(t + rnd() * 50000)
      ins.run(q, `demo.${n++}`, '06' + String(10000000 + Math.floor(rnd() * 89999999)), at.toISOString().replace(/\.\d+Z$/, 'Z'), result, hold,
        result === 'answered' ? Math.round(60 + rnd() * 400) : 0, ag, name, new Date(at.getTime() + 300000).toISOString().replace(/\.\d+Z$/, 'Z'))
    }
  }
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
