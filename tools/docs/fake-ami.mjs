// A stand-in for the Asterisk Manager Interface, only for documentation screenshots.
// It answers the read-only actions the portal uses with a believable, fixed office scene:
// a few calls in progress, registered phones, a trunk, BLF states and a busy support queue.
// It also sends a day of call detail records when Kiestoon connects, so the history is filled.
// Usage: node fake-ami.mjs [port]   (point Kiestoon at it with KIESTOON_AMI_ADDR=127.0.0.1:<port>)
import net from 'node:net'

const port = Number(process.argv[2] || 5038)
const started = Date.now()
const up = () => Math.floor((Date.now() - started) / 1000)

// Extension states: 0 idle, 1 in use, 8 ringing, 16 on hold, -1 unavailable, 2 busy.
const extState = {
  201: 1, 202: 0, 203: 1, 204: 8, 205: 0, 206: 0, 207: 1, 208: -1, 209: 0, 210: 16, 211: 0, 212: 0,
}
const ua = {
  201: 'Yealink SIP-T54W 96.86.0.70', 202: 'Yealink SIP-T46U 108.86.0.90', 203: 'snomD785/10.1.175.12', 204: 'Kiestoon Web',
  205: 'Grandstream GRP2614 1.0.11.33', 206: 'Yealink SIP-T54W 96.86.0.70', 207: 'Kiestoon Softphone 0.6.0', 209: 'Fanvil X5U 2.12.8',
  210: 'Yealink SIP-T57W 96.86.0.70', 211: 'Gigaset N870 IP PRO', 212: 'Poly Edge E350 8.1.4',
}

function channels() {
  const t = up()
  const chans = []
  const call = (id, legs) => legs.forEach((l, i) => chans.push({ ...l, Linkedid: id, Uniqueid: `${id}.${i}` }))
  call('1731686000.11', [
    { Channel: 'PJSIP/trunk-1-0000001a', CallerIDNum: '0612345678', CallerIDName: 'Jansen Bouw', ConnectedLineNum: '201', ConnectedLineName: 'Sanne Bakker', Exten: '201', ChannelStateDesc: 'Up', Application: 'Dial', Duration: 312 + t, Context: 'kiestoon-from-trunk' },
    { Channel: 'PJSIP/201-0000001b', CallerIDNum: '201', CallerIDName: 'Sanne Bakker', ConnectedLineNum: '0612345678', ConnectedLineName: 'Jansen Bouw', Exten: '', ChannelStateDesc: 'Up', Application: 'AppDial', Duration: 305 + t, Context: 'kiestoon-extensions' },
  ])
  call('1731686100.21', [
    { Channel: 'PJSIP/203-0000002a', CallerIDNum: '203', CallerIDName: 'Daan Visser', ConnectedLineNum: '0205551234', ConnectedLineName: '', Exten: '0205551234', ChannelStateDesc: 'Up', Application: 'Dial', Duration: 128 + t, Context: 'kiestoon-internal' },
    { Channel: 'PJSIP/trunk-1-0000002b', CallerIDNum: '0205551234', CallerIDName: '', ConnectedLineNum: '203', ConnectedLineName: 'Daan Visser', Exten: '', ChannelStateDesc: 'Up', Application: 'AppDial', Duration: 121 + t, Context: 'kiestoon-to-trunk' },
  ])
  call('1731686200.31', [
    { Channel: 'PJSIP/207-0000003a', CallerIDNum: '207', CallerIDName: 'Lotte Smit', ConnectedLineNum: '210', ConnectedLineName: 'Ruben Mulder', Exten: '210', ChannelStateDesc: 'Up', Application: 'Dial', Duration: 45 + t, Context: 'kiestoon-internal' },
    { Channel: 'PJSIP/210-0000003b', CallerIDNum: '210', CallerIDName: 'Ruben Mulder', ConnectedLineNum: '207', ConnectedLineName: 'Lotte Smit', Exten: '', ChannelStateDesc: 'Up', Application: 'AppDial', Duration: 40 + t, Context: 'kiestoon-internal' },
  ])
  call('1731686300.41', [
    { Channel: 'PJSIP/trunk-1-0000004a', CallerIDNum: '0301234567', CallerIDName: 'Gemeente Utrecht', ConnectedLineNum: '204', ConnectedLineName: 'Emma de Boer', Exten: '204', ChannelStateDesc: 'Ring', Application: 'Dial', Duration: 6 + (t % 20), Context: 'kiestoon-from-trunk' },
    { Channel: 'PJSIP/204-0000004b', CallerIDNum: '204', CallerIDName: 'Emma de Boer', ConnectedLineNum: '0301234567', ConnectedLineName: 'Gemeente Utrecht', Exten: '', ChannelStateDesc: 'Ringing', Application: 'AppDial', Duration: 5 + (t % 20), Context: 'kiestoon-extensions' },
  ])
  call('1731686400.51', [
    { Channel: 'PJSIP/trunk-1-0000005a', CallerIDNum: '0687654321', CallerIDName: '', ConnectedLineNum: '700', ConnectedLineName: '', Exten: '700', ChannelStateDesc: 'Up', Application: 'Queue', Duration: 74 + t, Context: 'kiestoon-queues' },
  ])
  call('1731686500.61', [
    { Channel: 'PJSIP/trunk-1-0000006a', CallerIDNum: '0104567890', CallerIDName: 'De Groot Transport', ConnectedLineNum: '700', ConnectedLineName: '', Exten: '700', ChannelStateDesc: 'Up', Application: 'Queue', Duration: 31 + t, Context: 'kiestoon-queues' },
  ])
  return chans
}

function contacts() {
  return Object.entries(ua).map(([ext, agent], i) => {
    const endpoint = agent === 'Kiestoon Web' ? `${ext}-web` : ext
    return { Endpoint: endpoint, Uri: `sip:${endpoint}@192.168.10.${40 + i}:5060`, Status: 'Reachable', RoundtripUsec: String(1800 + i * 950), UserAgent: agent }
  })
}

// Call records are made relative to this moment, so every reconnect sends the same records.
const STARTED = Date.now()
let cdrsSent = false

const queueMembers = [
  { ext: '201', name: 'Sanne Bakker', status: 2, inCall: 1, taken: 14, paused: 0 },
  { ext: '202', name: 'Thomas Jansen', status: 1, inCall: 0, taken: 11, paused: 0 },
  { ext: '204', name: 'Emma de Boer', status: 6, inCall: 0, taken: 5, paused: 0 },
  { ext: '205', name: 'Noor Hendriks', status: 1, inCall: 0, taken: 9, paused: 1 },
  { ext: '209', name: 'Milan Vos', status: 1, inCall: 0, taken: 7, paused: 0 },
]

function queueEvents() {
  const t = up()
  const ev = [
    { Event: 'QueueParams', Queue: 'kiestoon-q-1', Calls: '2', Holdtime: '38', TalkTime: '214', Completed: '41', Abandoned: '3', ServicelevelPerf: '92.7' },
    ...queueMembers.map((m) => ({ Event: 'QueueMember', Queue: 'kiestoon-q-1', Name: m.name, Location: `Local/${m.ext}@kiestoon-ring/n`, Status: String(m.status), Paused: String(m.paused), InCall: String(m.inCall), CallsTaken: String(m.taken), LastCall: String(Math.floor(Date.now() / 1000) - 240) })),
    { Event: 'QueueEntry', Queue: 'kiestoon-q-1', Position: '1', CallerIDNum: '0687654321', CallerIDName: '', Wait: String(74 + t) },
    { Event: 'QueueEntry', Queue: 'kiestoon-q-1', Position: '2', CallerIDNum: '0104567890', CallerIDName: 'De Groot Transport', Wait: String(31 + t) },
    { Event: 'QueueParams', Queue: 'kiestoon-q-2', Calls: '0', Holdtime: '22', TalkTime: '180', Completed: '17', Abandoned: '1', ServicelevelPerf: '96.1' },
    { Event: 'QueueMember', Queue: 'kiestoon-q-2', Name: 'Daan Visser', Location: 'Local/203@kiestoon-ring/n', Status: '2', Paused: '0', InCall: '1', CallsTaken: '8', LastCall: '0' },
    { Event: 'QueueMember', Queue: 'kiestoon-q-2', Name: 'Lotte Smit', Location: 'Local/207@kiestoon-ring/n', Status: '2', Paused: '0', InCall: '1', CallsTaken: '6', LastCall: '0' },
  ]
  return ev
}

function cdrs() {
  const out = []
  const people = [['201', 'Sanne Bakker'], ['202', 'Thomas Jansen'], ['203', 'Daan Visser'], ['204', 'Emma de Boer'], ['205', 'Noor Hendriks'], ['207', 'Lotte Smit'], ['209', 'Milan Vos']]
  const outside = [['0612345678', 'Jansen Bouw'], ['0205551234', ''], ['0301234567', 'Gemeente Utrecht'], ['0104567890', 'De Groot Transport'], ['0687654321', ''], ['0402233445', 'Van Dijk Installatie']]
  // 46 calls of today, ending a few minutes ago; spaced closer together when the build runs early in the day.
  const now = STARTED, midnight = new Date(STARTED); midnight.setHours(0, 5, 0, 0)
  const begin = Math.max(midnight.getTime(), now - 46 * 11 * 60000 - 300000)
  const step = (now - 300000 - begin) / 46
  const day = new Date(begin)
  const fmt = (d) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')} ${d.toTimeString().slice(0, 8)}`
  for (let i = 0; i < 46; i++) {
    const start = new Date(day.getTime() + i * step + (i * 37 % 60) * 1000 * (step / 660000))
    if (start > new Date()) break
    const p = people[(i * 3 + Math.floor(i / 7)) % people.length], o = outside[(i * 5) % outside.length]
    const kind = i % 7 === 3 ? 'internal' : i % 3 === 0 ? 'outbound' : 'inbound'
    const missed = kind === 'inbound' && i % 9 === 4
    const bill = missed ? 0 : 35 + ((i * 53) % 420)
    const ans = new Date(start.getTime() + 7000)
    const end = new Date(ans.getTime() + bill * 1000)
    let src, dst, clid, chan, dchan, trunk = ''
    if (kind === 'inbound') { src = o[0]; dst = p[0]; clid = `"${o[1]}" <${o[0]}>`; chan = `PJSIP/trunk-1-000${i}a`; dchan = `PJSIP/${p[0]}-000${i}b` }
    else if (kind === 'outbound') { src = p[0]; dst = o[0]; clid = `"${p[1]}" <${p[0]}>`; chan = `PJSIP/${p[0]}-000${i}a`; dchan = `PJSIP/trunk-1-000${i}b`; trunk = 'Voys' }
    else { const q = people[(i + 2) % people.length]; src = p[0]; dst = q[0]; clid = `"${p[1]}" <${p[0]}>`; chan = `PJSIP/${p[0]}-000${i}a`; dchan = `PJSIP/${q[0]}-000${i}b` }
    out.push({
      Event: 'Cdr', UniqueID: `17316${i}.1`, LinkedID: `17316${i}.1`, Source: src, Destination: dst, CallerID: clid, Channel: chan,
      DestinationChannel: dchan, DestinationContext: 'kiestoon-internal', LastApplication: 'Dial', Disposition: missed ? 'NO ANSWER' : 'ANSWERED',
      StartTime: fmt(start), AnswerTime: missed ? '' : fmt(ans), EndTime: fmt(missed ? new Date(start.getTime() + 25000) : end),
      Duration: String(missed ? 25 : bill + 7), BillableSeconds: String(bill), Trunk: trunk,
    })
  }
  return out
}

function write(sock, obj) {
  sock.write(Object.entries(obj).map(([k, v]) => `${k}: ${v}`).join('\r\n') + '\r\n\r\n')
}

function list(sock, id, events, complete) {
  write(sock, { Response: 'Success', ActionID: id, EventList: 'start', Message: 'List follows' })
  for (const e of events) write(sock, { ...e, ActionID: id })
  write(sock, { Event: complete, ActionID: id, EventList: 'Complete', ListItems: events.length })
}

net.createServer((sock) => {
  sock.write('Asterisk Call Manager/11.0.0\r\n')
  let buf = ''
  sock.on('error', () => {})
  sock.on('data', (d) => {
    buf += d.toString()
    let i
    while ((i = buf.indexOf('\r\n\r\n')) >= 0) {
      const raw = buf.slice(0, i); buf = buf.slice(i + 4)
      const m = {}
      for (const line of raw.split('\r\n')) { const j = line.indexOf(':'); if (j > 0) m[line.slice(0, j).trim().toLowerCase()] = line.slice(j + 1).trim() }
      const id = m.actionid, action = (m.action || '').toLowerCase()
      switch (action) {
        case 'login':
          write(sock, { Response: 'Success', ActionID: id, Message: 'Authentication accepted' })
          // Asterisk sends a call record once; the portal stores it, also across a restart of the portal.
          if (!cdrsSent) { cdrsSent = true; setTimeout(() => cdrs().forEach((e) => write(sock, e)), 500) }
          break
        case 'coreshowchannels': list(sock, id, channels().map((c) => ({ Event: 'CoreShowChannel', ...c, Duration: new Date(c.Duration * 1000).toISOString().slice(11, 19) })), 'CoreShowChannelsComplete'); break
        case 'pjsipshowcontacts': list(sock, id, contacts().map((c) => ({ Event: 'ContactList', ...c })), 'ContactListComplete'); break
        case 'pjsipshowregistrationsoutbound': list(sock, id, [1, 2].map((n) => ({ Event: 'OutboundRegistrationDetail', ObjectName: `trunk-${n}`, Status: 'Registered' })), 'OutboundRegistrationDetailComplete'); break
        case 'extensionstatelist': list(sock, id, Object.entries(extState).map(([ext, s]) => ({ Event: 'ExtensionStatus', Exten: ext, Context: 'kiestoon-extensions', Hint: `PJSIP/${ext}`, Status: String(s) })), 'ExtensionStateListComplete'); break
        case 'queuestatus': list(sock, id, queueEvents(), 'QueueStatusComplete'); break
        case 'coresettings': write(sock, { Response: 'Success', ActionID: id, AsteriskVersion: '22.6.0', AMIversion: '11.0.0' }); break
        case 'corestatus': {
          const d = new Date(started)
          write(sock, { Response: 'Success', ActionID: id, CoreStartupDate: d.toISOString().slice(0, 10), CoreStartupTime: d.toTimeString().slice(0, 8), CoreCurrentCalls: '6' })
          break
        }
        case 'command': write(sock, { Response: 'Success', ActionID: id, Output: '' }); break
        default: write(sock, { Response: 'Success', ActionID: id, Message: 'OK' })
      }
    }
  })
}).listen(port, '127.0.0.1', () => console.log(`fake AMI on 127.0.0.1:${port}`))
