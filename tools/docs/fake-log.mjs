// Writes an Asterisk messages.log and security.log for the demo office, so the log viewer and the
// "why does this trunk not register" explanation have something real-looking to show.
// Usage: node fake-log.mjs <dir>
import fs from 'node:fs'
import path from 'node:path'

const dir = process.argv[2]
fs.mkdirSync(dir, { recursive: true })
const day = new Date().toISOString().slice(0, 10)
let pid = 4100
const line = (hms, level, mod, text) => `[${day} ${hms}] ${level}[${pid++}] ${mod}: ${text}`

const messages = [
  line('06:58:02', 'NOTICE', 'loader.c', '512 modules will be loaded.'),
  line('06:58:04', 'NOTICE', 'res_pjsip_outbound_registration.c', "Outbound registration to 'sip:ha.voys.nl' for trunk-1 succeeded"),
  line('06:58:04', 'NOTICE', 'res_pjsip_outbound_registration.c', "Outbound registration to 'sip:sip.cheapconnect.net:5060' for trunk-2 succeeded"),
  line('07:12:40', 'WARNING', 'chan_pjsip.c', "Endpoint 214 is not reachable: no contact registered"),
  line('07:41:09', 'NOTICE', 'app_queue.c', "Member 'PJSIP/203' of queue 'kiestoon-queue-1' is now paused (Lunch)"),
  line('08:02:13', 'WARNING', 'res_pjsip_outbound_registration.c', "No response received from 'sip:sip.speakup.example:5060' on registration attempt to 'sip:0507001300@sip.speakup.example:5060', retrying in '60'"),
  line('08:03:13', 'WARNING', 'res_pjsip_outbound_registration.c', "Fatal response '401' received from 'sip:sip.speakup.example:5060' on registration attempt to 'sip:0507001300@sip.speakup.example:5060', stopping outbound registration"),
  line('08:03:13', 'ERROR', 'res_pjsip_outbound_authenticator_digest.c', "trunk-3: Failed to authenticate with 'sip:sip.speakup.example:5060': wrong user name or password (realm 'speakup')"),
  line('08:21:55', 'WARNING', 'app_dial.c', "Unable to create channel of type 'PJSIP' (cause 20 - Subscriber absent)"),
  line('08:30:01', 'NOTICE', 'res_pjsip/pjsip_distributor.c', "Request 'REGISTER' from '\"201\" <sip:201@pbx.noordkade.example>' failed for '192.168.10.41:5060' (callid: 3c2a) - Failed to authenticate"),
]
const security = [
  line('03:14:22', 'SECURITY', 'res_security_log.c', 'SecurityEvent="InvalidAccountID",EventTV="' + day + 'T03:14:22",Service="PJSIP",AccountID="admin",RemoteAddress="IPV4/UDP/185.53.88.21/5071"'),
  line('03:14:23', 'SECURITY', 'res_security_log.c', 'SecurityEvent="InvalidAccountID",EventTV="' + day + 'T03:14:23",Service="PJSIP",AccountID="100",RemoteAddress="IPV4/UDP/185.53.88.21/5071"'),
  line('08:30:01', 'SECURITY', 'res_security_log.c', 'SecurityEvent="InvalidPassword",EventTV="' + day + 'T08:30:01",Service="PJSIP",AccountID="201",RemoteAddress="IPV4/UDP/192.168.10.41/5060"'),
]
fs.writeFileSync(path.join(dir, 'messages.log'), messages.join('\n') + '\n')
fs.writeFileSync(path.join(dir, 'security.log'), security.join('\n') + '\n')
