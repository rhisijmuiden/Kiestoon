// Adds what the API cannot create on a machine without Asterisk and real sites: paired branch offices,
// an active linked system, voicemail, recordings, received faxes and a backup history.
// Usage: node dress.mjs <base-url> <data-dir> <wireguard-keys-file>
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

// Received faxes.
const box = db.prepare('SELECT id FROM fax_boxes ORDER BY id LIMIT 1').get()
for (const [ago, caller, name, pages, status, err] of [
  [40 * 60000, '0503344556', 'Elektro Noord', 2, 'success', ''], [3 * 3600000, '0402233445', 'VAN DIJK INSTALL', 1, 'success', ''],
  [27 * 3600000, '0201112233', '', 0, 'failed', 'No fax signal (the caller was not a fax machine)'], [50 * 3600000, '0104567890', 'DE GROOT TRANSPORT', 4, 'success', ''],
]) {
  db.prepare(`INSERT INTO faxes(box_id, received_at, caller, caller_name, did, remote_station, pages, status, error, file, size, mailed)
    VALUES (?, ?, ?, ?, '0507001250', ?, ?, ?, ?, '', ?, ?)`).run(box?.id ?? null, iso(ago), caller, name, name, pages, status, err, pages * 48211, status === 'success' ? 1 : 0)
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
