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
  features: Features
  recordMode: 'never' | 'ondemand' | 'always'
  pickupGroup: string
  outboundClass: 'internal' | 'national' | 'international'
  allowPremium: boolean
  webrtc: boolean
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
  id: number; name: string; did: string; destinationType: DestType; destination: string; cidPrefix: string; enabled: boolean
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

// ---- phase 2: business features ---------------------------------------------

export type DestType = 'extension' | 'voicemail' | 'ringgroup' | 'queue' | 'ivr' | 'timecondition' | 'conference' | 'external' | 'hangup'
export type Destination = { type: DestType | ''; target: string }
export type DestOption = { type: DestType; target: string; label: string }
export type Destinations = { types: DestType[]; options: DestOption[] }

export type FollowMe = { enabled: boolean; numbers: string[]; delay: number; confirm: boolean }
export type Features = {
  dnd: boolean; forwardAlways: string; forwardBusy: string; forwardNoAnswer: string; followMe: FollowMe
  scheduleId: number; closedForward: string
}
export const blankFeatures: Features = {
  dnd: false, forwardAlways: '', forwardBusy: '', forwardNoAnswer: '', followMe: { enabled: false, numbers: [], delay: 0, confirm: false },
  scheduleId: 0, closedForward: '',
}

export type RingGroup = {
  id: number; number: string; name: string; strategy: 'simultaneous' | 'sequential' | 'rotating' | 'paging'; ringSeconds: number
  members: string[]; cidPrefix: string; skipBusy: boolean; failover: Destination; enabled: boolean
}
export type QueueMember = { extensionId: number; number: string; name: string; penalty: number; dynamic: boolean; loggedIn: boolean; paused: boolean }
export type Queue = {
  id: number; number: string; name: string; strategy: string; music: string; announcePosition: boolean; announceFrequency: number
  agentTimeout: number; retry: number; wrapup: number; maxWait: number; maxCallers: number; joinEmpty: boolean; record: boolean
  cidPrefix: string; greetingPrompt: number; overflow: Destination; enabled: boolean; members: QueueMember[]
}
export type QueueAgentLive = { number: string; name: string; status: string; paused: boolean; inCall: boolean; callsTaken: number; lastCall: number }
export type QueueCallerLive = { position: number; callerId: string; name: string; wait: number }
export type QueueView = Queue & {
  live: null | {
    calls: number; longestWait: number; holdtime: number; completed: number; abandoned: number; serviceLevel: number
    callers: QueueCallerLive[] | null; agents: QueueAgentLive[] | null
  }
}

export type IVROption = { digit: string; destination: Destination }
export type IVR = {
  id: number; name: string; promptId: number; timeout: number; retries: number; directDial: boolean; options: IVROption[]
  timeoutDestination: Destination; layout: unknown
}

export type TimeRange = { days: number[]; from: string; to: string }
export type TimeCondition = {
  id: number; name: string; schedule: TimeRange[]; useHolidays: boolean; mode: 'auto' | 'open' | 'closed'
  open: Destination; closed: Destination; holiday: Destination
}
export type CustomHoliday = { date: string; name: string; annual: boolean }
export type HolidayCalendar = { preset: string[]; custom: CustomHoliday[] }
export type HolidaysView = { calendar: HolidayCalendar; year: number; days: { date: string; key?: string; name?: string }[]; presets: string[] }

export type Conference = {
  id: number; number: string; name: string; pin: string; adminPin: string; maxMembers: number; record: boolean; waitAdmin: boolean; enabled: boolean
}
export type Prompt = { id: number; name: string; durationMs: number; createdAt: string }

export type FraudSettings = {
  premiumPrefixes: string[]; blockedCountries: string[]; maxOutbound: number; alertEmail: string; alertOutboundPerHour: number
  alertInternationalPerHour: number; blockOnAlert: boolean
}
export type FeatureSettings = { parkingSlots: number; parkingTimeout: number; recordingDays: number; recordingBeep: boolean; fraud: FraudSettings }
export type FeatureCode = { code: string; key: string }

export type Recording = { name: string; date: string; uniqueId: string; from: string; to: string; size: number; duration: number }
export type BlfEntry = { number: string; name: string; state: string; dnd: boolean }

export type RoleMapping = { group: string; role: string }
export type SsoProvider = {
  id: number; name: string; kind: 'entra' | 'google' | 'keycloak' | 'authentik' | 'generic'; issuer: string; clientId: string
  clientSecret?: string; secretSet: boolean; scopes: string; groupsClaim: string; roleMap: RoleMapping[]; defaultRole: string
  autoCreate: boolean; trustMfa: boolean; enabled: boolean
}

export type PortalMe = {
  id: number; number: string; name: string; email: string; features: Features; voicemail: boolean; voicemailEmail: boolean
  webrtc: boolean; recordMode: string; outboundClass: string; schedules: { id: number; name: string }[]; codes: FeatureCode[]
}
export type VoicemailMessage = { folder: string; id: number; callerId: string; date: string; duration: number }
export type RingGroupStatus = { id: number; number: string; name: string; strategy: string; members: { number: string; name: string; state: string }[] }
export type WebRTCAccount = { wsPath: string; username: string; password: string; number: string; name: string; domain: string; secureContext: boolean }
