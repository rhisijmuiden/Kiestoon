// The bridge to the Windows shell (Tauri). The /desktop pages run inside the Kiestoon Softphone
// app, which exposes window.__TAURI__ to this origin only. In a normal browser every call is a
// no-op, so the pages still work there (handy for development).

type TauriGlobal = {
  core: { invoke: <T>(cmd: string, args?: Record<string, unknown>) => Promise<T> }
  event: { listen: <T>(name: string, cb: (e: { payload: T }) => void) => Promise<() => void> }
}

function tauri(): TauriGlobal | null {
  return (window as unknown as { __TAURI__?: TauriGlobal }).__TAURI__ ?? null
}

/** True inside the Kiestoon Softphone app. */
export const inShell = () => tauri() !== null

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
