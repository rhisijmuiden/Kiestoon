import type { Role } from './api'

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
  t38: boolean
  enabled: boolean
}

export type TrunkPreset = {
  id: string; name: string; authMode: 'register' | 'ip'; host: string; port: number; transport: 'udp' | 'tcp' | 'tls'
  didSource: 'ruri' | 'to'; codecs: string; matchIps: string; note: string; guide: string; docsUrl?: string
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

export type LiveCall = { id: string; from: string; fromName: string; to: string; direction: string; state: string; duration: number; legs?: LiveLeg[] }
export type LiveContact = { endpoint: string; uri: string; status: string; rttMs: number; userAgent: string }
export type Live = {
  connected: boolean
  calls: LiveCall[]
  contacts: LiveContact[]
  trunks: { id: number; name: string; mode: string; status: string }[]
  extensions: number
  registered: number
  stats: { today: number; answered: number; missed: number; inbound: number; outbound: number }
  quality?: { calls: Record<string, number>; alerts: QualityAlert[]; limits: QualityLimits }
}

export type Cdr = {
  id: number; uniqueId: string; linkedId: string; start: string; answer: string; end: string; duration: number; billsec: number; src: string; dst: string; clid: string
  srcChannel: string; dstChannel: string; disposition: string; direction: string; trunk: string
}

export type MailSettings = {
  enabled: boolean; host: string; port: number; security: 'tls' | 'starttls' | 'none'
  auth: 'none' | 'password' | 'oauth2_m365' | 'oauth2_google'; username: string; password?: string; passwordSet: boolean
  fromAddress: string; fromName: string; tenantId: string; clientId: string; clientSecret?: string; clientSecretSet: boolean
  refreshToken?: string; refreshTokenSet: boolean; skipVerify: boolean; notifyAddress: string
}
export type MailLog = { id: number; at: string; recipient: string; subject: string; kind: string; ok: boolean; error: string }

export type TrustedNetwork = { cidr: string; description: string }
export type TrustedNetworks = { enabled: boolean; networks: TrustedNetwork[]; roles: Role[] }
export type TrustedNetworksData = { settings: TrustedNetworks; clientIp: string; public: string[]; clientNetwork: string }

export type FirewallConfig = {
  enabled: boolean; sipWhitelist: string[]; allowPrivateSip: boolean; sipOpen: boolean; adminNetworks: string[]
  fail2banEnabled: boolean; banMinutes: number; findMinutes: number; maxRetry: number; ignoreIps: string[]
  geoMode: 'off' | 'allow' | 'block' | ''; geoCountries: string[]; geoPortal: boolean
}
export type FirewallStatus = { active: boolean; pending: boolean; secondsLeft: number; ruleset: string; rolledBack: boolean }
export type Ban = { jail: string; ip: string }

export type BackupTarget = {
  id: number; name: string; kind: 'ftp' | 'ftps' | 'sftp' | 'scp' | 'onedrive' | 's3' | 'webdav' | 'smb'; host: string; port: number; username: string
  password?: string; passwordSet: boolean; privateKey?: string; keySet: boolean; path: string; implicitTls: boolean
  repoPassword?: string; enabled: boolean; onedrive?: OneDrive
  /** s3: provider and region; webdav: vendor; smb: Windows domain; plain: http instead of https. */
  provider?: string; region?: string; domain?: string; plain?: boolean
}
export type OneDrive = {
  tenant: string; clientId: string; clientSecret?: string; secretSet?: boolean; connected?: boolean; driveType?: string; account?: string
}
export type BackupRun = {
  id: number; target: string; kind: string; trigger: string; startedAt: string; finishedAt: string; state: string
  message: string; snapshotId: string; bytesAdded: number; files: number
}
export type Snapshot = { id: string; shortId: string; time: string; hostname: string; tags: string[] }
export type Schedule = { enabled: boolean; time: string; fullWeekday: number; retention: { daily: number; weekly: number; monthly: number } }

export type ProvSettings = { enabled: boolean; username: string; password: string; server: string; adminPasswordMode: 'perPhone' | 'shared'; adminPassword: string; ntpServer: string; language: 'en' | 'nl' }
export type PhoneModel = { vendor: 'yealink' | 'snom' | 'grandstream' | 'fanvil' | 'gigaset' | 'poly' | 'htek' | 'akuvox' | 'cisco'; id: string; name: string; keys: number; lines?: number; perPage?: number }
export type KeyType = 'none' | 'line' | 'blf' | 'speed' | 'park' | 'parkslot' | 'intercom' | 'dnd' | 'voicemail'
export type PhoneKey = { type: KeyType; value: string; label: string }
export type PhoneModule = { type: string; keys: PhoneKey[] }
export type ModuleType = { vendor: PhoneModel['vendor']; id: string; name: string; keys: number; perPage: number; max: number; models: string[] }
export type IntercomOptions = { allow: boolean; mute: boolean; tone: boolean; barge: boolean }
export type HotdeskOptions = { enabled: boolean; autoLogout: boolean }
export type KeyTemplate = { id: number; name: string; vendor: PhoneModel['vendor']; model: string; keys: PhoneKey[]; modules: PhoneModule[] }
export type Phone = {
  id: number; mac: string; vendor: PhoneModel['vendor']; model: string; extensionId: number | null; label: string
  keys: PhoneKey[]; lines: number[]; lastSeenAt: string; lastIp: string; userAgent: string
  ztpState?: '' | 'registered' | 'error' | 'elsewhere'; ztpError?: string; ztpAt?: string
  modules?: PhoneModule[]; options?: { intercom: IntercomOptions; hotdesk?: HotdeskOptions }
  hotdeskExtensionId?: number | null; hotdeskAt?: string
}

// ---- phase 2: business features ---------------------------------------------

export type DestType = 'extension' | 'voicemail' | 'ringgroup' | 'queue' | 'ivr' | 'timecondition' | 'conference' | 'fax' | 'external' | 'hangup' | 'remote' | 'followsun'
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
  remoteMembers?: RemoteMember[]
  options: QueueOptions
}
export type CallbackOptions = { enabled: boolean; offerEvery: number; retries: number; retryDelay: number; expire: number; callerId: string }
export type SLAOptions = { target: number; norm: number; maxWaiting: number; maxWait: number; maxAbandoned: number }
export type QueueOptions = { supervision: boolean; callback: CallbackOptions; sla?: SLAOptions }
export type SLAKind = 'waiting' | 'wait' | 'level' | 'abandoned'
export type SLAAlert = { queueId: number; queue: string; kind: SLAKind; value: number; limit: number; since: string }
export type CallbackStatus = 'scheduled' | 'waiting' | 'calling' | 'done' | 'failed' | 'cancelled'
export type QueueCallback = {
  id: number; queueId: number; number: string; name: string; position: number; status: CallbackStatus; attempts: number
  agent: string; note: string; nextAt: string; createdAt: string; updatedAt: string
}
export type RemoteMember = { peerId: number; number: string; name: string; site: string; penalty: number }
export type QueueAgentLive = { number: string; name: string; status: string; paused: boolean; inCall: boolean; callsTaken: number; lastCall: number }
export type QueueCallerLive = { position: number; callerId: string; name: string; wait: number }
export type QueueView = Queue & {
  live: null | {
    calls: number; longestWait: number; holdtime: number; completed: number; abandoned: number; serviceLevel: number
    callers: QueueCallerLive[] | null; agents: QueueAgentLive[] | null
  }
  alerts?: SLAAlert[]
}

export type WallTotals = {
  offered: number; answered: number; abandoned: number; shortAbandoned: number; left: number; callbacks: number; inTarget: number
  avgWait: number; avgTalk: number; longestWait: number; serviceLevel: number; abandonedPct: number
}
export type WallHour = { answered: number; abandoned: number }
export type WallCaller = { position: number; number: string; name: string; wait: number; callback: boolean }
export type WallQueue = {
  id: number; name: string; number: string; sla: SLAOptions; supervision: boolean; live: boolean; waiting: number; callbacks: number
  longestWait: number; callers: WallCaller[]; free: number; busy: number; paused: number; today: WallTotals; hours: WallHour[]
}
export type WallAgentState = 'free' | 'ringing' | 'call' | 'internal' | 'paused' | 'offline'
export type WallAgent = {
  number: string; name: string; state: WallAgentState; since: number; queues: number[]; queue?: number; caller?: string; callsToday: number
}
export type WallLayout = 'overview' | 'queue' | 'agents'
export type WallData = {
  now: string; live: boolean; queues: WallQueue[]; agents: WallAgent[]; alerts: SLAAlert[]
  screen?: { name: string; layouts: WallLayout[]; rotate: number; theme: 'dark' | 'light' }
}
export type WallScreen = {
  id: number; name: string; key: string; queues: number[]; layouts: WallLayout[]; rotate: number; theme: 'dark' | 'light'
  hideNames: boolean; createdAt: string; lastSeen: string
}
export type AlertSettings = { screens: boolean; email: boolean; recipients: string[]; quiet: number; timeCondition: number; recovered: boolean }
export type WallSettings = {
  screens: WallScreen[]; alerts: AlertSettings; timeConditions: { id: number; name: string }[]
  queues: { id: number; name: string; number: string; sla: SLAOptions; alerts: SLAKind[] }[]
}

export type IVROption = { digit: string; destination: Destination }
export type FlowNodeType = 'entry' | 'key' | 'timeout' | 'audio' | 'destination' | 'time' | 'repeat' | 'speech'
export type SpeechIntent = { label: string; keywords: string[] }
export type FlowNode = {
  id: string; type: FlowNodeType; x: number; y: number; digit?: string; label?: string; promptId?: number; destination?: Destination
  intents?: SpeechIntent[]; ai?: boolean
}
export type FlowPort = 'out' | 'open' | 'closed' | 'nomatch' | `i${number}`
export type FlowEdge = { from: string; port: FlowPort; to: string }
export type Flow = { nodes: FlowNode[]; edges: FlowEdge[] }
export type IVR = {
  id: number; name: string; promptId: number; timeout: number; retries: number; directDial: boolean; options: IVROption[]
  timeoutDestination: Destination; flow: Flow | null
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
/** Live state for the IVR canvas: time conditions open now, and destinations keyed "type:target". */
export type IvrLive = {
  open: Record<string, boolean>
  dests: Record<string, { state?: string; members?: number; online?: number; waiting?: number }>
}
export type Prompt = { id: number; name: string; durationMs: number; createdAt: string }

export type FraudSettings = {
  premiumPrefixes: string[]; blockedCountries: string[]; maxOutbound: number; alertEmail: string; alertOutboundPerHour: number
  alertInternationalPerHour: number; blockOnAlert: boolean
}
export type FeatureSettings = { parkingSlots: number; parkingTimeout: number; recordingDays: number; recordingBeep: boolean; journeyDays: number; fraud: FraudSettings }
export type FeatureCode = { code: string; key: string }

export type Recording = {
  name: string; date: string; uniqueId: string; from: string; to: string; size: number; duration: number
  ai?: { status: 'done' | 'failed'; topic?: string; sentiment?: AISentiment }
}
export type BlfEntry = { number: string; name: string; state: string; dnd: boolean }
export type TransferTarget = { kind: 'extension' | 'ringgroup' | 'queue'; number: string; name: string; state?: string; dnd?: boolean; members?: number; waiting?: number }
export type ActiveCall = {
  channel: string; peer: string; peerName: string; state: 'ringing' | 'talking'; duration: number; device: 'phone' | 'web' | 'app'
  consult?: { target: string; targetName: string; answered: boolean }
}
export type LiveLeg = { channel: string; endpoint: string; callerNum: string; callerName: string; connected: string; connectedName: string; state: string; duration: number }

export type RoleMapping = { group: string; role: string }
export type SsoProvider = {
  id: number; name: string; kind: 'entra' | 'google' | 'keycloak' | 'authentik' | 'generic' | 'saml'; issuer: string; clientId: string
  clientSecret?: string; secretSet: boolean; scopes: string; groupsClaim: string; roleMap: RoleMapping[]; defaultRole: string
  autoCreate: boolean; trustMfa: boolean; enabled: boolean; protocol?: 'oidc' | 'saml'; samlMetadata?: string
}

export type PortalMe = {
  id: number; number: string; name: string; email: string; features: Features; voicemail: boolean; voicemailEmail: boolean
  webrtc: boolean; recordMode: string; outboundClass: string; schedules: { id: number; name: string }[]; codes: FeatureCode[]
}
export type VoicemailMessage = { folder: string; id: number; callerId: string; date: string; duration: number; transcript?: string; summary?: AISummary }
export type RingGroupStatus = { id: number; number: string; name: string; strategy: string; members: { number: string; name: string; state: string }[] }
export type WebRTCAccount = { wsPath: string; username: string; password: string; number: string; name: string; domain: string; secureContext: boolean }

// ---- phase 3: integrations --------------------------------------------------

export type InternetSettings = { enabled: boolean; domain: string; portalDomain: string; email: string; staging: boolean; useForSip: boolean }
export type CertInfo = { subject: string; issuer: string; notAfter: string; dnsNames: string[] }
export type InternetData = {
  settings: InternetSettings; status: { installed: boolean; active: boolean; certificate: CertInfo | null; portalCertificate?: CertInfo | null }; helperAvailable: boolean; dns?: string[]; portalDns?: string[]
}
export type GeoStatus = { available: boolean; updatedAt?: string; attribution: string }

export type FaxBox = {
  id: number; name: string; emails: string; keepDays: number; ecm: boolean; enabled: boolean
  number: string; sendEnabled: boolean; senders: string; sendPin?: string; sendPinSet?: boolean
}
export type Fax = {
  id: number; boxId: number | null; boxName: string; receivedAt: string; caller: string; callerName: string; did: string
  remoteStation: string; pages: number; status: 'success' | 'failed'; error: string; size: number; mailed: boolean
}

export type Contact = { id: number; name: string; company: string; work: string; mobile: string; other: string; email: string; notes: string }
export type PhonebookSettings = {
  includeExtensions: boolean; callerIdLookup: boolean; xmlEnabled: boolean; ldapEnabled: boolean; ldapPort: number; ldapsPort: number; ldapAuth: boolean
}
export type PhonebookInfo = { settings: PhonebookSettings; baseDn: string; bindDn: string; server: string; ldapRunning: boolean; xmlBase: string }

export type ApiToken = {
  id: number; name: string; prefix: string; userId: number; username: string; role: string; readOnly: boolean; expiresAt: string
  lastUsedAt: string; lastIp: string; createdAt: string
}
export type Webhook = { id: number; name: string; url: string; events: string[]; secret: string; enabled: boolean; createdAt: string }
export type WebhookDelivery = { id: number; at: string; event: string; attempt: number; status: number; error: string; durationMs: number }

export type Passkey = { id: number; name: string; createdAt: string; lastUsedAt: string }

// ---- phase 4: extensions -----------------------------------------------------

export type Firmware = {
  id: number; vendor: string; models: string[]; version: string; filename: string; size: number; sha256: string
  active: boolean; createdAt: string; files: number
}
export type FirmwareRelease = { vendor: string; models: string[]; version: string; url: string; sha256?: string; notes?: string; source: string }
export type FirmwareModelState = {
  vendor: string; model: string; name: string; phones: number; behind: number
  versions: { version: string; phones: number }[]
  latest: FirmwareRelease | null; active: Firmware | null
  state: 'current' | 'update' | 'rolling' | 'nosource'
}
export type FirmwareLatest = {
  mode: 'off' | 'notify' | 'auto'; catalogUrl: string; defaultCatalogUrl: string; checkedAt: string; errors: string[]
  models: FirmwareModelState[]
}
export type ZtpGuide = { vendor: string; service: string; portal: string; path: string }
export type ZtpSettings = { serverUrl: string; yealink: { enabled: boolean; accessKey: string; secret?: string; secretSet: boolean; api: string } }
export type ZtpData = { settings: ZtpSettings; guides: ZtpGuide[]; effectiveUrl: string; username: string; password: string }

export type SpeechProgress = { done: number; total: number; error?: string }
export type SpeechItem = {
  id: string; name: string; language?: string; licence?: string; speed?: string; size: number; downloaded: boolean; progress?: SpeechProgress
}
export type SpeechSettings = {
  tts: { enabled: boolean; voiceEn: string; voiceNl: string }
  stt: { enabled: boolean; model: string; language: 'auto' | 'nl' | 'en'; voicemail: boolean }
}
export type AISentiment = 'positive' | 'neutral' | 'negative'
export type AISummary = { summary: string; actions: string[]; topic: string; sentiment: AISentiment }
export type AISettings = {
  calls: boolean; voicemail: boolean; engine: 'local' | 'cloud'; llm: string; cloudModel: string; language: 'auto' | 'nl' | 'en'; keepDays: number
}
export type AIData = {
  settings: AISettings; llms: (SpeechItem & { ram: number })[]; llmInstalled: boolean; cloudModels: string[]
  keySet: boolean; keyHint: string; sttReady: boolean
}
export type AIResult = { status: 'none' | 'done' | 'failed'; transcript?: string; summary?: AISummary; engine?: string; error?: string; at?: string }
export type SpeechData = { settings: SpeechSettings; ttsInstalled: boolean; sttInstalled: boolean; voices: SpeechItem[]; models: SpeechItem[] }

export type FaxJob = {
  id: number; boxId: number | null; boxName: string; userId: number | null; source: 'portal' | 'email'; sender: string; number: string
  document: string; pages: number; status: 'queued' | 'sending' | 'success' | 'failed'; attempts: number; error: string
  createdAt: string; updatedAt: string; nextAt: string
}
export type FaxSendBox = { id: number; name: string; number: string }
export type FaxMail = {
  enabled: boolean; host: string; port: number; security: 'tls' | 'starttls'; username: string; password?: string; passwordSet: boolean
  folder: string; delete: boolean; lastCheck?: string; lastError?: string
}

export type ProxySettings = { enabled: boolean; trustedProxies: string[]; publicUrl: string; portalUrl: string }
export type Hostnames = {
  pbxUrl: string; portalUrl: string; blockAdmin: boolean; pbxHost: string; portalHost: string; separate: boolean; requestHost: string; onPortal: boolean
}
export type HostnamesData = { hostnames: Hostnames; ssoCallbacks: string[] }
export type ProxyData = {
  settings: ProxySettings; remoteAddr: string; clientIp: string; forwardedFor: string; forwardedProto: string; secure: boolean; host: string
}
export type MetricsSettings = { enabled: boolean; token: string; allowFrom: string[] }

// ---- Multi-site and high availability ----------------------------------------

export type HubSettings = { enabled: boolean; subnet: string; publicKey: string; listenPort: number; host: string }
export type EdgeSite = {
  id: number; name: string; paired: boolean; pairExpires: string; publicKey: string; tunnelIp: string; hostname: string; version: string
  lanAddress: string; lanNet: string; services: Record<string, string> | null; lastSeen: string; pairedAt: string; createdAt: string
  connected: boolean; remote: string
}
export type SitesView = {
  hub: HubSettings; edges: EdgeSite[]; helperAvailable: boolean; wireguardInstalled: boolean; tunnelsUp: string[] | null
  tunnelErrors: Record<string, string> | null; portalUrl: string; pinned: boolean
}
export type DirectoryEntry = { number: string; name: string; kind: string }
export type FedPeer = {
  id: number; name: string; prefix: string; role: 'hosted' | 'joined'; state: string; inviteExpires: string; publicKey: string
  tunnelIp: string; localIp: string; endpoint: string; sipPort: number; portalPort: number; directDial: boolean
  directory: { name: string; timezone: string; version: string; entries: DirectoryEntry[] | null }
  version: string; lastSeen: string; createdAt: string; connected: boolean; states: Record<string, string> | null
}
export type FederationView = { name: string; hubEnabled: boolean; peers: FedPeer[]; helperAvailable: boolean }
export type SunRule = { label: string; timezone: string; days: number[]; from: string; to: string; destination: Destination }
export type SunRoute = { id: number; name: string; rules: SunRule[]; fallback: Destination; enabled: boolean }

export type ClusterNode = {
  enabled: boolean; mode: 'vip' | 'manual' | ''; name: string; address: string; interface: string; vip: string; tunnel: string
  peer: { name: string; address: string; publicKey: string; tunnel: string } | null; joinOpen: boolean
}
export type ClusterRole = { role: 'primary' | 'standby'; epoch: number; lastSeq: number; changedAt: string; reason: string }
export type SnapshotMeta = { seq: number; epoch: number; at: string; size: number; sha256: string }
export type ClusterStatusHelper = { keepalivedInstalled: boolean; keepalivedActive: boolean; vrrpState: string; vipHere: boolean; interface: string; asteriskActive: boolean }
export type ClusterView = {
  node: ClusterNode; role: ClusterRole; status: ClusterStatusHelper; helperAvailable: boolean; busy: boolean; lastError: string; version: string
  defaultAddress: string; snapshot: SnapshotMeta
  peerState: null | { lastSeen: string; version: string; replica: SnapshotMeta; lastFileSync: string; files: number; lagSeconds: number; inSync: boolean; online: boolean }
}
export type StandbyView = {
  node: ClusterNode; role: ClusterRole; status: ClusterStatusHelper; helperAvailable: boolean; version: string; hostname: string
  replication: {
    primaryReachable: boolean; primaryName: string; primaryVersion: string; lastContact: string; lastDbSync: string; lastFileSync: string
    files: number; replica: SnapshotMeta; primary: SnapshotMeta; lastError: string
  }
}

/** What happened to a welcome e-mail. The link is only there when the mail could not be sent. */
export type WelcomeResult = { sent: boolean; created: boolean; username: string; email: string; error?: string; link?: string }

// ---- linking phones to extensions -------------------------------------------

/** A phone that asked for its configuration but is not known yet. */
export type DiscoveredPhone = {
  mac: string; vendor: PhoneModel['vendor'] | ''; model: string; ip: string; userAgent: string; file: string
  hits: number; firstSeen: string; lastSeen: string
}
export type ResyncState = 'sent' | 'not_registered' | 'failed' | 'skipped'
export type ResyncResult = { resync: ResyncState; message?: string }

export type JourneyParty = { kind: 'ext' | 'caller' | 'remote' | 'system'; number?: string; name?: string }
export type JourneySub = {
  kind: 'key' | 'timeout' | 'invalid' | 'repeat' | 'speech' | 'ring'; at: string; durationMs: number; value?: string; party?: JourneyParty; result?: string
}
export type JourneyStep = {
  id: number; kind: string; at: string; durationMs: number; ref?: string; name?: string; number?: string; result?: string
  tone: 'ok' | 'warn' | 'danger' | 'info' | 'accent'; party?: JourneyParty; detail?: Record<string, string>; subs?: JourneySub[]
}
export type Journey = {
  linkedId: string; start: string; end: string; durationMs: number; talkMs: number; waitMs: number
  direction: 'inbound' | 'outbound' | 'internal'; from: JourneyParty; to: string
  result: 'answered' | 'voicemail' | 'abandoned' | 'missed' | 'busy' | 'failed'; complete: boolean; partial: boolean
  steps: JourneyStep[]; recordings: { name: string; durationMs: number; from: string; to: string }[]
  quality?: QualityCall
}

export type ReportPeriod = 'daily' | 'weekly' | 'monthly'
export type ReportSection = 'queues' | 'agents' | 'hours' | 'trend'
export type Report = {
  id: number; name: string; period: ReportPeriod; time: string; weekday: number; workdays: boolean; queues: number[]
  sections: ReportSection[]; recipients: string[]; language: 'en' | 'nl'; enabled: boolean; lastSent: string
}

export type MobileDevice = {
  id: number; userId: number; user: string; userName: string; extension: string; name: string
  platform: 'apns' | 'fcm'; createdAt: string; lastSeen: string; lastPush: string
}
export type MobileAdmin = { enabled: boolean; relayURL: string; defaultRelayURL: string; publicKey: string; devices: MobileDevice[] }

// ---- call quality ----

export type QualityLimits = { good: number; poor: number; jitter: number; loss: number; rtt: number }
export type QualityCause = 'ok' | 'lossTo' | 'lossFrom' | 'jitterTo' | 'jitterFrom' | 'rtt' | 'mixed'
export type QualityDirection = { mos: number; jitter: number; jitterMax: number; loss: number; reports: number }
export type QualityLeg = {
  linkedId: string; uniqueId: string; channel: string; endpoint: string; kind: 'trunk' | 'extension' | 'other'; trunkId?: number; ext?: string
  codec: string; remote: string; start: string; end: string; rx: QualityDirection; tx: QualityDirection; rtt: number; rttMax: number
  mos: number; mosMin: number; samples: number[]; name: string; rating: 'good' | 'fair' | 'poor'
}
export type QualityCall = { mos: number; rating: 'good' | 'fair' | 'poor'; cause: QualityCause; where: string; legs: QualityLeg[]; limits: QualityLimits }
export type QualityFigures = {
  legs: number; calls: number; mos: number; poor: number; poorPct: number; rxJitter: number; txJitter: number; rxLoss: number; txLoss: number
  rtt: number; jitter: number; loss: number; cause: QualityCause
}
export type QualityGroup = QualityFigures & { key: string; name: string; remote: string; buckets: number[] }
export type QualityAlert = {
  id: number; key: string; kind: 'trunk' | 'phone'; ref: string; name: string; calls: number; poor: number; mos: number; jitter: number
  loss: number; rtt: number; cause: QualityCause; started: string; ended?: string; mailed?: string
}
export type QualityLive = {
  id: string; from: string; fromName: string; to: string; direction: string; route: string; codec: string; duration: number
  mos: number; jitter: number; loss: number; rtt: number
}
export type QualityOverview = {
  overview: { from: string; bucket: number; total: QualityFigures; trunks: QualityGroup[]; phones: QualityGroup[]; buckets: number[] }
  live: QualityLive[]; alerts: QualityAlert[]; lastCleared: string; limits: QualityLimits; connected: boolean
}
export type QualityRule = { enabled: boolean; minutes: number; count: number }
export type QualitySettings = QualityLimits & {
  trunk: QualityRule; phone: QualityRule; eachCall: boolean; recovered: boolean; email: boolean; recipients: string[]; quiet: number; keepDays: number
}
