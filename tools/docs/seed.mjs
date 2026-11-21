// Fills a fresh Kiestoon with a fictional office ("Noordkade Installatietechniek") for the documentation screenshots.
// Usage: node seed.mjs <base-url> <data-dir>   (run by demo.sh)
// Creates the first admin (admin / Kiestoon-docs-2026) and writes the TOTP secret to <data-dir>/../mfa-secret.
import fs from 'node:fs'
import path from 'node:path'
import { Client, totp } from './lib.mjs'
import { wav } from './wav.mjs'

const [base = 'http://127.0.0.1:8090', dataDir] = process.argv.slice(2)
export const ADMIN = { username: 'admin', password: 'Kiestoon-docs-2026' }
const c = new Client(base)

// ---- first admin with MFA ----
const token = fs.readFileSync(path.join(dataDir, 'setup-token'), 'utf8').trim()
await c.post('/api/setup', { token, ...ADMIN, language: 'en', displayName: 'Anna de Vries' })
const { data: enr } = await c.post('/api/account/mfa/enroll', {})
await c.post('/api/account/mfa/confirm', { code: totp(enr.secret) })
fs.writeFileSync(path.join(dataDir, '..', 'mfa-secret'), enr.secret)
await c.put('/api/account/theme', { theme: 'light' })
await c.put('/api/system/channel', { channel: 'testing' })

const prompts = {}
for (const [i, [key, name, secs]] of [
  ['welcome', 'Welcome - main menu', 9], ['closed', 'Closed - outside office hours', 7], ['holiday', 'Holiday message', 6],
  ['queue', 'Support queue - please hold', 5], ['sales', 'Sales - one moment please', 4],
].entries()) {
  const r = await c.post('/api/pbx/prompts', { name, audio: wav(secs, 7 + i * 13) })
  prompts[key] = r.data.id
}

// ---- extensions ----
const people = [
  ['201', 'Sanne Bakker', 'sanne.bakker'], ['202', 'Thomas Jansen', 'thomas.jansen'], ['203', 'Daan Visser', 'daan.visser'],
  ['204', 'Emma de Boer', 'emma.deboer'], ['205', 'Noor Hendriks', 'noor.hendriks'], ['206', 'Bram Willems', 'bram.willems'],
  ['207', 'Lotte Smit', 'lotte.smit'], ['208', 'Jesse Meijer', 'jesse.meijer'], ['209', 'Milan Vos', 'milan.vos'],
  ['210', 'Ruben Mulder', 'ruben.mulder'], ['211', 'Warehouse DECT', ''], ['212', 'Meeting room', ''],
]
const blankFeatures = { dnd: false, forwardAlways: '', forwardBusy: '', forwardNoAnswer: '', followMe: { enabled: false, numbers: [], delay: 0, confirm: false }, scheduleId: 0, closedForward: '' }
const ext = {}
for (const [number, name, mail] of people) {
  const r = await c.post('/api/pbx/extensions', {
    number, name, email: mail ? `${mail}@noordkade.example` : '', transport: 'udp', srtp: false, maxContacts: 3, outboundCallerId: '',
    ringSeconds: 25, voicemail: !!mail, voicemailEmail: !!mail, voicemailDelete: false, enabled: true,
    features: { ...blankFeatures, dnd: number === '206', forwardNoAnswer: number === '209' ? '0612398765' : '' },
    recordMode: number === '201' ? 'always' : 'ondemand', pickupGroup: '1', outboundClass: number === '211' ? 'internal' : 'national',
    allowPremium: false, webrtc: true,
  })
  ext[number] = r.data.id ?? r.data.extension?.id
}

// ---- trunks and routes ----
const trunk = await c.post('/api/pbx/trunks', {
  name: 'Voys', provider: 'voys', authMode: 'register', host: 'ha.voys.nl', port: 0, transport: 'udp', username: '155012345',
  authUsername: '155012345', secret: 'docs-example-secret', fromDomain: '', fromUser: '', matchIps: '195.35.114.0/23,185.103.76.0/22',
  didSource: 'ruri', codecs: 'alaw,g722', maxChannels: 10, callerId: '0507001234', srtp: false, t38: false, enabled: true,
})
const trunkId = trunk.data.id
await c.post('/api/pbx/trunks', {
  name: 'Cheapconnect (backup)', provider: 'generic', authMode: 'register', host: 'sip.cheapconnect.net', port: 5060, transport: 'udp',
  username: '0507001299', authUsername: '0507001299', secret: 'docs-example-secret', fromDomain: '', fromUser: '', matchIps: '',
  didSource: 'to', codecs: 'alaw,ulaw', maxChannels: 4, callerId: '0507001299', srtp: false, t38: true, enabled: true,
})
await c.put('/api/pbx/settings', {
  digits: 3, countryCode: '31', emergencyCallerId: '0507001234', emergencyTrunkIds: [trunkId], externalAddress: 'pbx.noordkade.example',
  localNets: ['192.168.10.0/24', '10.0.0.0/8'], sipPort: 5060, tlsPort: 5061, rtpStart: 10000, rtpEnd: 20000, codecs: 'g722,alaw,ulaw',
  language: 'nl', nationalCallerId: true, timezone: 'Europe/Amsterdam',
})

// ---- ring group, queues, conference ----
const sales = await c.post('/api/pbx/ringgroups', {
  number: '600', name: 'Sales', strategy: 'simultaneous', ringSeconds: 20, members: ['202', '206', '208'], cidPrefix: 'Sales: ',
  skipBusy: true, failover: { type: 'voicemail', target: '202' }, enabled: true,
})
await c.post('/api/pbx/ringgroups', {
  number: '601', name: 'Warehouse', strategy: 'sequential', ringSeconds: 15, members: ['211', '210'], cidPrefix: '', skipBusy: false,
  failover: { type: 'hangup', target: '' }, enabled: true,
})
const qBase = { music: 'default', announcePosition: true, announceFrequency: 60, agentTimeout: 15, retry: 5, wrapup: 10, maxWait: 300, maxCallers: 0, joinEmpty: true, record: false, enabled: true }
const support = await c.post('/api/pbx/queues', {
  ...qBase, number: '700', name: 'Support', strategy: 'rrmemory', cidPrefix: 'Support: ', greetingPrompt: prompts.queue,
  overflow: { type: 'voicemail', target: '201' }, record: true,
  options: { supervision: true, callback: { enabled: true, offerEvery: 60, retries: 2, retryDelay: 120, expire: 240, callerId: '' },
    sla: { target: 30, norm: 80, maxWaiting: 3, maxWait: 60, maxAbandoned: 10 } },
  members: ['201', '202', '204', '205', '209'].map((n) => ({ extensionId: ext[n], penalty: 0, dynamic: n !== '201' })),
})
await c.post('/api/pbx/queues', {
  ...qBase, number: '701', name: 'Planning', strategy: 'leastrecent', cidPrefix: 'Planning: ', greetingPrompt: 0,
  overflow: { type: 'ringgroup', target: String(sales.data.id) },
  options: { supervision: true, callback: { enabled: false, offerEvery: 60, retries: 2, retryDelay: 120, expire: 240, callerId: '' },
    sla: { target: 30, norm: 90, maxWaiting: 2, maxWait: 90, maxAbandoned: 0 } },
  members: ['203', '207'].map((n) => ({ extensionId: ext[n], penalty: 0, dynamic: false })),
})
await c.post('/api/reception/callbacks', { queueId: support.data.id, number: '0612345678', name: 'J. Jansen' })
await c.post('/api/reception/callbacks', { queueId: support.data.id, number: '0503141592', name: 'Bakkerij De Korrel' })
await c.post('/api/pbx/conferences', { number: '800', name: 'Team meeting', pin: '4321', adminPin: '9876', maxMembers: 20, record: false, waitAdmin: false, enabled: true })
await c.post('/api/pbx/conferences', { number: '801', name: 'Customer calls', pin: '2580', adminPin: '1357', maxMembers: 8, record: true, waitAdmin: true, enabled: true })

// ---- IVR flow, time condition, inbound ----
const ivr = await c.post('/api/pbx/ivrs', {
  name: 'Main menu', promptId: prompts.welcome, timeout: 6, retries: 2, directDial: true, options: [], timeoutDestination: { type: '', target: '' },
  flow: {
    nodes: [
      { id: 'entry', type: 'entry', x: 40, y: 220, label: 'Main menu', promptId: prompts.welcome },
      { id: 'k1', type: 'key', x: 360, y: 40, digit: '1', label: 'Sales' },
      { id: 'k2', type: 'key', x: 360, y: 170, digit: '2', label: 'Support' },
      { id: 'k3', type: 'key', x: 360, y: 300, digit: '3', label: 'Reception' },
      { id: 'to', type: 'timeout', x: 360, y: 430 },
      { id: 'a1', type: 'audio', x: 690, y: 40, label: 'Sales intro', promptId: prompts.sales },
      { id: 'd1', type: 'destination', x: 1020, y: 40, destination: { type: 'ringgroup', target: String(sales.data.id) } },
      { id: 'd2', type: 'destination', x: 690, y: 170, destination: { type: 'queue', target: String(support.data.id) } },
      { id: 'd3', type: 'destination', x: 690, y: 300, destination: { type: 'extension', target: '203' } },
      { id: 'r1', type: 'repeat', x: 690, y: 430 },
    ],
    edges: [
      { from: 'entry', port: 'out', to: 'k1' }, { from: 'entry', port: 'out', to: 'k2' }, { from: 'entry', port: 'out', to: 'k3' },
      { from: 'entry', port: 'out', to: 'to' }, { from: 'k1', port: 'out', to: 'a1' }, { from: 'a1', port: 'out', to: 'd1' },
      { from: 'k2', port: 'out', to: 'd2' }, { from: 'k3', port: 'out', to: 'd3' }, { from: 'to', port: 'out', to: 'r1' },
    ],
  },
})
// A menu that asks what the caller wants (speech step).
await c.post('/api/pbx/ivrs', {
  name: 'Speech menu', promptId: prompts.welcome, timeout: 3, retries: 0, directDial: false, options: [], timeoutDestination: { type: '', target: '' },
  flow: {
    nodes: [
      { id: 'entry', type: 'entry', x: 40, y: 160, promptId: prompts.welcome },
      { id: 'k1', type: 'key', x: 360, y: 20, digit: '1', label: 'Support' },
      { id: 'to', type: 'timeout', x: 360, y: 200 },
      { id: 'sp', type: 'speech', x: 680, y: 160, label: 'What are you calling about?', promptId: prompts.sales, ai: true, intents: [
        { label: 'Sales', keywords: ['quote', 'offerte', 'price', 'prijs', 'bestel'] },
        { label: 'Support', keywords: ['fault', 'storing', 'broken', 'kapot', 'werkt niet'] },
        { label: 'Invoices', keywords: ['invoice', 'factu', 'payment', 'betaling'] },
      ] },
      { id: 'd1', type: 'destination', x: 1020, y: 40, destination: { type: 'ringgroup', target: String(sales.data.id) } },
      { id: 'd2', type: 'destination', x: 1020, y: 170, destination: { type: 'queue', target: String(support.data.id) } },
      { id: 'd3', type: 'destination', x: 1020, y: 300, destination: { type: 'extension', target: '203' } },
      { id: 'd4', type: 'destination', x: 1020, y: 430, destination: { type: 'voicemail', target: '201' } },
    ],
    edges: [
      { from: 'entry', port: 'out', to: 'k1' }, { from: 'entry', port: 'out', to: 'to' }, { from: 'k1', port: 'out', to: 'd2' },
      { from: 'to', port: 'out', to: 'sp' }, { from: 'sp', port: 'i1', to: 'd1' }, { from: 'sp', port: 'i2', to: 'd2' },
      { from: 'sp', port: 'i3', to: 'd3' }, { from: 'sp', port: 'nomatch', to: 'd4' },
    ],
  },
})
const tc = await c.post('/api/pbx/timeconditions', {
  name: 'Office hours', schedule: [{ days: [1, 2, 3, 4], from: '08:30', to: '17:30' }, { days: [5], from: '08:30', to: '16:00' }],
  useHolidays: true, mode: 'auto', open: { type: 'ivr', target: String(ivr.data.id) }, closed: { type: 'voicemail', target: '201' },
  holiday: { type: 'voicemail', target: '201' },
})
await c.post('/api/pbx/timeconditions', {
  name: 'Warehouse lunch', schedule: [{ days: [1, 2, 3, 4, 5], from: '12:00', to: '12:45' }], useHolidays: false, mode: 'auto',
  open: { type: 'ringgroup', target: '2' }, closed: { type: 'extension', target: '210' }, holiday: { type: '', target: '' },
})
const { data: hol } = await c.get('/api/pbx/holidays')
await c.put('/api/pbx/holidays', { preset: hol.calendar.preset, custom: [{ date: '2026-12-24', name: 'Christmas Eve (afternoon closed)', annual: true }] })
const fax = await c.post('/api/pbx/faxboxes', { name: 'Administration', emails: 'admin@noordkade.example', keepDays: 90, ecm: true, enabled: true, number: '0507001250', sendEnabled: true, senders: 'anna.devries@noordkade.example, sanne.bakker@noordkade.example' })
await c.post('/api/pbx/inbound', { name: 'Main number', did: '0507001234', destinationType: 'timecondition', destination: String(tc.data.id), cidPrefix: '', enabled: true })
await c.post('/api/pbx/inbound', { name: 'Support direct', did: '0507001240', destinationType: 'queue', destination: String(support.data.id), cidPrefix: 'Direct: ', enabled: true })
await c.post('/api/pbx/inbound', { name: 'Fax', did: '0507001250', destinationType: 'fax', destination: String(fax.data.id), cidPrefix: '', enabled: true })
await c.post('/api/pbx/inbound', { name: 'Sanne direct', did: '0507001201', destinationType: 'extension', destination: '201', cidPrefix: '', enabled: true })
await c.post('/api/pbx/outbound', { name: 'National and mobile', patterns: [{ prefix: '', match: '0[1-9]XXXXXXXX', prepend: '' }], trunkIds: [trunkId, 2], callerId: '', enabled: true })
await c.post('/api/pbx/outbound', { name: 'International', patterns: [{ prefix: '', match: '00X.', prepend: '' }], trunkIds: [trunkId], callerId: '', enabled: true })

// ---- portal users ----
const users = [
  ['sanne.bakker', 'Sanne Bakker', 'reception', '201'], ['thomas.jansen', 'Thomas Jansen', 'user', '202'],
  ['daan.visser', 'Daan Visser', 'user', '203'], ['emma.deboer', 'Emma de Boer', 'user', '204'], ['ruben.mulder', 'Ruben Mulder', 'admin', '210'],
]
for (const [username, displayName, role, n] of users) {
  await c.post('/api/users', { username, displayName, email: `${username}@noordkade.example`, role, extensionId: ext[n], language: n === '202' ? 'en' : 'nl', enabled: true, password: 'Kiestoon-docs-2026' })
}
const { data: me } = await c.get('/api/users')
const adminUser = (Array.isArray(me) ? me : me.users ?? []).find((u) => u.username === 'admin')
if (adminUser) await c.put(`/api/users/${adminUser.id}`, { username: 'admin', displayName: 'Anna de Vries', email: 'anna.devries@noordkade.example', role: 'superadmin', extensionId: ext['204'], language: 'en', enabled: true, password: '' })

// ---- phones, key template ----
const blf = (n, l) => ({ type: 'blf', value: n, label: l })
const keys = [
  { type: 'line', value: '1', label: '' }, blf('202', 'Thomas'), blf('203', 'Daan'), blf('205', 'Noor'), { type: 'park', value: '70', label: 'Park' },
  { type: 'parkslot', value: '71', label: 'Slot 71' }, { type: 'speed', value: '0612345678', label: 'Jansen Bouw' }, { type: 'intercom', value: '210', label: 'Intercom Ruben' },
  { type: 'dnd', value: '', label: 'DND' }, { type: 'voicemail', value: '', label: 'Voicemail' },
]
const phones = [
  ['805ec0a1b201', 'yealink', 'T54W', '201', 'Reception desk', keys, [{ type: 'EXP50', keys: people.slice(0, 10).map(([n, nm]) => blf(n, nm.split(' ')[0])) }]],
  ['805ec0a1b202', 'yealink', 'T46U', '202', 'Thomas', keys.slice(0, 6), []],
  ['000413a2c203', 'snom', 'D785', '203', 'Planning', keys.slice(0, 8), []],
  ['c074ad11d205', 'grandstream', 'GRP2614', '205', 'Support 2', keys.slice(0, 4), []],
  ['0c383e44e209', 'fanvil', 'X5U', '209', 'Support 3', keys.slice(0, 6), []],
  ['805ec0a1b210', 'yealink', 'T57W', '210', 'Ruben', keys, []],
  ['7c2f80aa0211', 'gigaset', 'N870', '211', 'Warehouse DECT', [], []],
]
for (const [mac, vendor, model, n, label, k, modules] of phones) {
  await c.post('/api/phones', { mac, vendor, model, extensionId: ext[n], label, keys: k, modules, lines: [], options: { intercom: { allow: true, mute: false, tone: true, barge: false } } })
}
await c.post('/api/key-templates', { name: 'Support desk', vendor: 'yealink', model: 'T54W', keys, modules: [] })
await c.put('/api/provisioning/settings', { enabled: true, username: 'prov', password: 'docs-example-pass', server: 'pbx.noordkade.example', adminPassword: 'docs-admin', ntpServer: 'nl.pool.ntp.org', language: 'nl' })
// Two phones that asked for their configuration but are not linked yet.
for (const [mac, uaStr, file] of [['805ec0a1b2f1', 'Yealink SIP-T54W 96.86.0.70 80:5e:c0:a1:b2:f1', '805ec0a1b2f1.cfg'], ['000413a2c2f2', 'Mozilla/4.0 (compatible; snomD735-SIP 10.1.175.12)', 'snomD735-000413A2C2F2.htm']]) {
  await fetch(`${base}/prov/${file}`, { headers: { 'User-Agent': uaStr, Authorization: 'Basic ' + Buffer.from('prov:docs-example-pass').toString('base64') } }).catch(() => {})
}

// ---- phone book ----
for (const [name, company, work, mobile, email] of [
  ['Peter Jansen', 'Jansen Bouw', '0501234567', '0612345678', 'peter@jansenbouw.example'],
  ['Inge de Groot', 'De Groot Transport', '0104567890', '0623456789', 'inge@degroot.example'],
  ['Klantcontact', 'Gemeente Utrecht', '0301234567', '', ''],
  ['Marco van Dijk', 'Van Dijk Installatie', '0402233445', '0634567890', 'marco@vandijk.example'],
  ['Servicedesk', 'Hollands Glas', '0205551234', '', 'service@hollandsglas.example'],
  ['Fatima El Amrani', 'Elektro Noord', '0503344556', '0645678901', 'fatima@elektronoord.example'],
]) await c.post('/api/phonebook', { name, company, work, mobile, other: '', email, notes: '' })

// ---- mail, backups ----
await c.put('/api/mail/settings', {
  enabled: true, host: 'smtp.office365.com', port: 587, security: 'starttls', auth: 'password', username: 'pbx@noordkade.example',
  password: 'docs-example-secret', fromAddress: 'pbx@noordkade.example', fromName: 'Noordkade telefonie', tenantId: '', clientId: '',
  skipVerify: false, notifyAddress: 'it@noordkade.example',
})
await c.post('/api/backups/targets', { name: 'NAS office (SFTP)', kind: 'sftp', host: 'nas.noordkade.example', port: 22, username: 'kiestoon', password: 'docs-example-secret', path: '/backups/kiestoon', implicitTls: false, enabled: true })
await c.post('/api/backups/targets', { name: 'Offsite FTPS', kind: 'ftps', host: 'backup.hosting.example', port: 21, username: 'noordkade', password: 'docs-example-secret', path: '/kiestoon', implicitTls: false, enabled: true })
await c.put('/api/backups/schedule', { enabled: true, time: '02:30', fullWeekday: 0, retention: { daily: 7, weekly: 4, monthly: 12 } })

// ---- security, SSO, integrations ----
await c.post('/api/sso/providers', {
  name: 'Microsoft', kind: 'entra', issuer: 'https://login.microsoftonline.com/0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b/v2.0', clientId: '6b7c8d9e-0f1a-2b3c-4d5e-6f7a8b9c0d1e',
  clientSecret: 'docs-example-secret', scopes: 'openid profile email', groupsClaim: 'groups',
  roleMap: [{ group: 'PBX-Admins', role: 'admin' }, { group: 'Reception', role: 'reception' }], defaultRole: 'user', autoCreate: true, trustMfa: true, enabled: true,
})
await c.put('/api/firewall', {
  enabled: true, sipWhitelist: ['195.35.114.0/23', '185.103.76.0/22', '213.211.0.0/16'], allowPrivateSip: true, sipOpen: false, adminNetworks: ['192.168.10.0/24'],
  fail2banEnabled: true, banMinutes: 60, findMinutes: 10, maxRetry: 5, ignoreIps: ['192.168.10.0/24'], geoMode: 'allow', geoCountries: ['NL', 'BE', 'DE'], geoPortal: false,
})
await c.put('/api/firewall/trusted', {
  enabled: true, roles: ['user', 'reception', 'admin'],
  networks: [{ cidr: '192.168.10.0/24', description: 'Office LAN Noordkade' }, { cidr: '10.8.0.0/16', description: 'Company VPN' }],
})
await c.post('/api/tokens', { name: 'CRM integration', role: 'reception', readOnly: true, expiresDays: 365 })
await c.post('/api/webhooks', { name: 'CRM - incoming calls', url: 'https://crm.noordkade.example/hooks/kiestoon', events: ['call.ringing', 'call.answered', 'call.ended'], secret: '', enabled: true })
await c.put('/api/internet', { enabled: true, domain: 'pbx.noordkade.example', email: 'it@noordkade.example', staging: false, useForSip: true })

// ---- multi-site ----
await c.put('/api/sites/hub', { enabled: true, listenPort: 51820, host: 'pbx.noordkade.example' })
await c.post('/api/sites/edges', { name: 'Groningen office', url: '' })
await c.post('/api/sites/edges', { name: 'Zwolle warehouse', url: '' })
await c.post('/api/federation/invites', { name: 'Noordkade Antwerpen', prefix: '3' })
await c.post('/api/pbx/sun-routes', {
  name: 'Support follow-the-sun', enabled: true, fallback: { type: 'voicemail', target: '201' },
  rules: [
    { label: 'Netherlands', timezone: 'Europe/Amsterdam', days: [1, 2, 3, 4, 5], from: '08:00', to: '18:00', destination: { type: 'queue', target: String(support.data.id) } },
    { label: 'Curaçao', timezone: 'America/Curacao', days: [1, 2, 3, 4, 5], from: '08:00', to: '17:00', destination: { type: 'external', target: '+59994651234' } },
  ],
})
await c.put('/api/pbx/features', {
  parkingSlots: 8, parkingTimeout: 120, recordingDays: 90, recordingBeep: true,
  fraud: { premiumPrefixes: ['0900', '0906', '0909', '084', '087', '18'], blockedCountries: ['53', '850'], maxOutbound: 10, alertEmail: 'it@noordkade.example', alertOutboundPerHour: 60, alertInternationalPerHour: 10, blockOnAlert: true },
})
console.log('seeded')
