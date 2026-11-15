export type Extension = {
  id: number
  number: string
  name: string
  email: string
  sipSecret?: string
  transport: 'udp' | 'tcp' | 'tls'
  srtp: boolean
  maxContacts: number
  outboundCallerId: string
  ringSeconds: number
  voicemail: boolean
  voicemailPin?: string
  voicemailEmail: boolean
  voicemailDelete: boolean
  enabled: boolean
}

export type Trunk = {
  id: number
  name: string
  provider: string
  authMode: 'register' | 'ip'
  host: string
  port: number
  transport: 'udp' | 'tcp' | 'tls'
  username: string
  authUsername: string
  secret?: string
  secretSet: boolean
  fromDomain: string
  fromUser: string
  matchIps: string
  didSource: 'ruri' | 'to'
  codecs: string
  maxChannels: number
  callerId: string
  srtp: boolean
  enabled: boolean
}

export type TrunkPreset = {
  id: string; name: string; authMode: 'register' | 'ip'; host: string; port: number; transport: 'udp' | 'tcp' | 'tls'
  didSource: 'ruri' | 'to'; codecs: string; note: string
}

export type InboundRoute = {
  id: number; name: string; did: string; destinationType: 'extension' | 'voicemail' | 'hangup'; destination: string; cidPrefix: string; enabled: boolean
}

export type DialPattern = { prefix: string; match: string; prepend: string }
export type OutboundRoute = { id: number; name: string; position: number; patterns: DialPattern[]; trunkIds: number[]; callerId: string; enabled: boolean }

export type PbxSettings = {
  digits: number
  countryCode: string
  emergencyCallerId: string
  emergencyTrunkIds: number[]
  externalAddress: string
  localNets: string[]
  sipPort: number
  tlsPort: number
  rtpStart: number
  rtpEnd: number
  codecs: string
  language: 'en' | 'nl'
  nationalCallerId: boolean
  timezone: string
}

export type Range = { purpose: string; from: string; to: string }
export type NumberPlan = { digits: number; ranges: Range[] }

export type PbxStatus = { connected: boolean; version: string; lastApply: string; lastError: string; restartNeeded: boolean }

export type LiveCall = { id: string; from: string; fromName: string; to: string; direction: string; state: string; duration: number }
export type LiveContact = { endpoint: string; uri: string; status: string; rttMs: number; userAgent: string }
export type Live = {
  connected: boolean
  calls: LiveCall[]
  contacts: LiveContact[]
  trunks: { id: number; name: string; mode: string; status: string }[]
  extensions: number
  registered: number
  stats: { today: number; answered: number; missed: number; inbound: number; outbound: number }
}

export type Cdr = {
  id: number; start: string; answer: string; end: string; duration: number; billsec: number; src: string; dst: string; clid: string
  srcChannel: string; dstChannel: string; disposition: string; direction: string; trunk: string
}

export type MailSettings = {
  enabled: boolean; host: string; port: number; security: 'tls' | 'starttls' | 'none'
  auth: 'none' | 'password' | 'oauth2_m365' | 'oauth2_google'; username: string; password?: string; passwordSet: boolean
  fromAddress: string; fromName: string; tenantId: string; clientId: string; clientSecret?: string; clientSecretSet: boolean
  refreshToken?: string; refreshTokenSet: boolean; skipVerify: boolean; notifyAddress: string
}
export type MailLog = { id: number; at: string; recipient: string; subject: string; kind: string; ok: boolean; error: string }

export type FirewallConfig = {
  enabled: boolean; sipWhitelist: string[]; allowPrivateSip: boolean; sipOpen: boolean; adminNetworks: string[]
  fail2banEnabled: boolean; banMinutes: number; findMinutes: number; maxRetry: number; ignoreIps: string[]
}
export type FirewallStatus = { active: boolean; pending: boolean; secondsLeft: number; ruleset: string; rolledBack: boolean }
export type Ban = { jail: string; ip: string }

export type BackupTarget = {
  id: number; name: string; kind: 'ftp' | 'ftps' | 'sftp' | 'scp'; host: string; port: number; username: string
  password?: string; passwordSet: boolean; privateKey?: string; keySet: boolean; path: string; implicitTls: boolean
  repoPassword?: string; enabled: boolean
}
export type BackupRun = {
  id: number; target: string; kind: string; trigger: string; startedAt: string; finishedAt: string; state: string
  message: string; snapshotId: string; bytesAdded: number; files: number
}
export type Snapshot = { id: string; shortId: string; time: string; hostname: string; tags: string[] }
export type Schedule = { enabled: boolean; time: string; fullWeekday: number; retention: { daily: number; weekly: number; monthly: number } }

export type ProvSettings = { enabled: boolean; username: string; password: string; server: string; adminPassword: string; ntpServer: string; language: 'en' | 'nl' }
export type PhoneModel = { vendor: 'yealink' | 'snom' | 'grandstream'; id: string; name: string; keys: number }
export type PhoneKey = { type: 'none' | 'line' | 'blf' | 'speed'; value: string; label: string }
export type Phone = {
  id: number; mac: string; vendor: PhoneModel['vendor']; model: string; extensionId: number | null; label: string
  keys: PhoneKey[]; lastSeenAt: string; lastIp: string; userAgent: string
}
