// The bridge to the Windows shell (Tauri). The /desktop pages run inside the Kiestoon Softphone
// app, which exposes window.__TAURI__ to this origin only. In a normal browser every call is a
// no-op, so the pages still work there (handy for development).

type TauriGlobal = {
  core: {
    invoke: <T>(cmd: string, args?: Record<string, unknown>) => Promise<T>
    addPluginListener?: <T>(plugin: string, event: string, cb: (payload: T) => void) => Promise<{ unregister: () => Promise<void> }>
  }
  event: { listen: <T>(name: string, cb: (e: { payload: T }) => void) => Promise<() => void> }
}

function tauri(): TauriGlobal | null {
  return (window as unknown as { __TAURI__?: TauriGlobal }).__TAURI__ ?? null
}

/** True inside the Kiestoon Softphone app or the Kiestoon mobile app. */
export const inShell = () => tauri() !== null

/**
 * True inside the Kiestoon app for iPhone and Android. The mobile shell has no tray, popup or
 * headset; calls come in through push and the call screen of the phone instead.
 */
export const inMobile = () => tauri() !== null && /Android|iPhone|iPad|iPod/i.test(navigator.userAgent)

export async function invoke<T = void>(cmd: string, args?: Record<string, unknown>): Promise<T | null> {
  const t = tauri()
  if (!t) return null
  return t.core.invoke<T>(cmd, args)
}

/** Listens to an event of the shell; returns the function that stops listening. */
export function listen<T>(name: string, cb: (payload: T) => void): () => void {
  const t = tauri()
  if (!t) return () => undefined
  let stop: (() => void) | null = null
  let cancelled = false
  t.event.listen<T>(name, (e) => cb(e.payload)).then((u) => {
    if (cancelled) u()
    else stop = u
  })
  return () => {
    cancelled = true
    stop?.()
  }
}

export type Channel = 'production' | 'testing'
export type AppInfo = {
  version: string
  channel: Channel
  os: string
  autostart: boolean
  server: string
  fingerprint: string
}
export type UpdateRelease = { version: string; notes: string; publishedAt: string; url: string }
export type UpdateCheck = { current: string; channel: Channel; latest: UpdateRelease | null; available: boolean }
export type Headset = { name: string; vendor: string; buttons: boolean }

/** What the tray icon shows. */
export type TrayStatus = 'available' | 'dnd' | 'call' | 'ringing' | 'offline'

export const shell = {
  info: () => invoke<AppInfo>('app_info'),
  setChannel: (channel: Channel) => invoke('set_channel', { channel }),
  checkUpdate: () => invoke<UpdateCheck>('check_update'),
  installUpdate: () => invoke('install_update'),
  setAutostart: (enabled: boolean) => invoke('set_autostart', { enabled }),
  notify: (title: string, body: string) => invoke('notify', { title, body }),
  incomingShow: (name: string, number: string, via: string, lang: string) => invoke('incoming_show', { name, number, via, lang }),
  incomingHide: () => invoke('incoming_hide'),
  setStatus: (status: TrayStatus, tooltip: string, labels: Record<string, string>) => invoke('set_status', { status, tooltip, labels }),
  headsetState: (ringing: boolean, offhook: boolean, muted: boolean) => invoke('headset_state', { ringing, offhook, muted }),
  headsets: () => invoke<Headset[]>('headset_list'),
  openExternal: (url: string) => invoke('open_external', { url }),
  unpair: () => invoke('unpair'),
  focus: () => invoke('focus_main'),
}

/** The native call plugin of the mobile app (push, call screen, call kept alive in the background). */
export type MobileRegistration = {
  platform: 'apns' | 'fcm'
  token: string
  alertToken?: string
  sandbox: boolean
  appId: string
  name: string
  pushReady: boolean
}
export type MobileCallAction = { action: string; id: string }

const plugin = (cmd: string) => `plugin:kiestoon-call|${cmd}`

export const mobile = {
  registration: () => invoke<MobileRegistration>(plugin('registration')),
  setSecret: (secret: string) => invoke(plugin('set_secret'), { secret }),
  callUpdate: (state: 'ringing' | 'active' | 'ended', name: string, number: string) => invoke(plugin('call_update'), { state, name, number }),
  takePending: () => invoke<Partial<MobileCallAction>>(plugin('take_pending')),
  /** Listens to an event of the plugin (token, push, callAction); returns the function that stops. */
  listen<T>(event: string, cb: (payload: T) => void): () => void {
    const add = tauri()?.core.addPluginListener
    if (!add) return () => undefined
    let stop: (() => void) | null = null
    let cancelled = false
    add<T>('kiestoon-call', event, cb).then((l) => {
      if (cancelled) l.unregister().catch(() => undefined)
      else stop = () => { l.unregister().catch(() => undefined) }
    }).catch(() => undefined)
    return () => {
      cancelled = true
      stop?.()
    }
  },
}
