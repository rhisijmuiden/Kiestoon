// A stand-in for the root helper (kiestoon-helper), only for documentation screenshots.
// It accepts every command and answers with a healthy system: firewall active, a few fail2ban bans,
// Caddy with a certificate, WireGuard tunnels up for the paired sites and successful backups.
// Usage: node fake-helper.mjs <socket-path> <wireguard-public-keys-file>
import fs from 'node:fs'
import net from 'node:net'

const [sock, keysFile] = process.argv.slice(2)
try { fs.unlinkSync(sock) } catch {}

const ruleset = `table inet kiestoon {
  set sip_allow { type ipv4_addr; flags interval; elements = { 185.103.76.0/22, 195.35.114.0/23, 213.211.0.0/16 } }
  chain input {
    type filter hook input priority 0; policy drop;
    ct state established,related accept
    iif lo accept
    tcp dport { 80, 443, 8080, 8443 } accept
    ip saddr @sip_allow udp dport 5060 accept
    ip saddr @sip_allow tcp dport { 5060, 5061 } accept
    udp dport 10000-20000 accept
    udp dport 51820 accept
  }
}`
const fw = { active: true, pending: false, secondsLeft: 0, ruleset, rolledBack: false }
const now = () => new Date().toISOString()
let snapshots = 0

function answer(cmd, args) {
  switch (cmd) {
    case 'firewall.apply': case 'firewall.status': case 'firewall.confirm': return fw
    case 'fail2ban.bans': return [{ jail: 'kiestoon-sip', ip: '185.53.88.21' }, { jail: 'kiestoon-sip', ip: '45.134.144.7' }, { jail: 'kiestoon-portal', ip: '193.32.162.88' }]
    case 'internet.status': case 'internet.apply': {
      const d = args?.settings?.domain || args?.domain || 'pbx.example.com'
      return { installed: true, active: true, copied: false, certificate: { subject: d, issuer: "Let's Encrypt R11", notAfter: new Date(Date.now() + 71 * 864e5).toISOString(), dnsNames: [d] } }
    }
    case 'wireguard.status': case 'wireguard.apply': {
      let keys = []
      try { keys = JSON.parse(fs.readFileSync(keysFile, 'utf8')) } catch {}
      return {
        installed: true, up: ['kt0'], errors: [],
        peers: keys.map((k, i) => ({ interface: 'kt0', publicKey: k, endpoint: `81.204.${12 + i}.${30 + i}:51820`, lastHandshake: new Date(Date.now() - (20 + i * 31) * 1000).toISOString(), rxBytes: 48211234 + i, txBytes: 51123321 + i })),
      }
    }
    case 'cluster.status': return { keepalivedInstalled: true, keepalivedActive: true, vrrpState: 'MASTER', vipHere: true, interface: 'eth0', asteriskActive: true }
    case 'backup.test': return { created: false }
    case 'backup.run': snapshots++; return { snapshotId: 'a3f9c2d1' + String(snapshots).padStart(56, '0'), bytesAdded: args?.kind === 'full' ? 412_334_112 : 3_482_119, files: 1843 }
    case 'backup.snapshots': return Array.from({ length: 6 }, (_, i) => {
      const t = new Date(Date.now() - i * 864e5); t.setHours(2, 30, 0, 0)
      const id = (0xa3f9c2d1 - i * 7919).toString(16).padEnd(64, '0')
      return { id, shortId: id.slice(0, 8), time: t.toISOString(), hostname: 'kiestoon', tags: [i % 7 === 0 ? 'full' : 'incremental'] }
    })
    default: return null
  }
}

net.createServer((c) => {
  let buf = ''
  c.on('error', () => {})
  c.on('data', (d) => {
    buf += d
    const i = buf.indexOf('\n')
    if (i < 0) return
    let req = {}
    try { req = JSON.parse(buf.slice(0, i)) } catch {}
    const data = answer(req.cmd, req.args)
    c.end(JSON.stringify({ ok: true, ...(data !== null ? { data } : {}) }) + '\n')
  })
}).listen(sock, () => console.log(`fake helper on ${sock} (${now()})`))
