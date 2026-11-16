import { useEffect, useRef } from 'react'
import { useNavigate } from 'react-router'
import { api } from '../lib/api'
import { useSoftphone } from '../lib/softphone'
import type { WebRTCAccount } from '../lib/types'
import { mobile, type MobileCallAction } from './bridge'
import { useDesktop } from './data'

// The mobile app (iPhone and Android) reuses the /desktop pages. The native part receives the
// push of an incoming call while the app is closed, shows the phone's own call screen and starts
// the app; the softphone in these pages then registers and takes the call over SIP.

let registered: Promise<boolean> | null = null

/** Registers this phone for push with the server, once per start (and again on a new token). */
function register(force = false): Promise<boolean> {
  if (registered && !force) return registered
  registered = (async () => {
    const r = await mobile.registration()
    if (!r || !r.pushReady || !r.token) return false
    const res = await api<{ id: number; secret: string }>('POST', '/portal/mobile/register', {
      appId: r.appId, name: r.name, platform: r.platform, token: r.token, alertToken: r.alertToken ?? '', sandbox: r.sandbox,
    })
    await mobile.setSecret(res.secret)
    return true
  })().catch(() => false)
  return registered
}

/** True when the server rings this phone through push (and the phone's call screen rings). */
export const pushActive = () => register()

/**
 * The SIP account of the app: its own endpoint (n-app), which the server rings through push. When
 * push is off on the server, or the app was built without push, it falls back to the account of
 * the browser softphone, so the app still works while it is open.
 */
export async function mobileAccount(): Promise<WebRTCAccount> {
  if (await register()) {
    try {
      return await api<WebRTCAccount>('GET', '/portal/mobile/sip')
    } catch {
      /* fall back */
    }
  }
  return api<WebRTCAccount>('GET', '/portal/webrtc')
}

/** How long an answer or decline from the call screen waits for the call to arrive over SIP. */
const PENDING_MS = 20_000

/** Keeps the phone's call screen in step with the softphone, and the other way round. */
export function MobileSync() {
  const phone = useSoftphone()
  const { voicemail } = useDesktop()
  const navigate = useNavigate()
  const latest = useRef(phone)
  latest.current = phone
  const pending = useRef<{ action: string; at: number } | null>(null)

  const call = phone.call
  const state = !call ? 'none' : call.direction === 'incoming' && call.state === 'ringing' ? 'ringing' : 'active'
  const name = call?.remoteName ?? ''
  const number = call?.remote ?? ''

  // Tell the native side: it keeps the call alive in the background and ends the call screen.
  const was = useRef('none')
  useEffect(() => {
    if (state === was.current) return
    const before = was.current
    was.current = state
    if (state === 'none') {
      if (before !== 'none') mobile.callUpdate('ended', '', '').catch(() => undefined)
      return
    }
    mobile.callUpdate(state, name, number).catch(() => undefined)
  }, [state, name, number])

  // An answer or decline from the call screen that came before the call itself.
  useEffect(() => {
    const p = pending.current
    if (state !== 'ringing' || !p) return
    pending.current = null
    if (Date.now() - p.at > PENDING_MS) return
    if (p.action === 'answer') {
      phone.answer()
      navigate('/desktop')
    } else if (p.action === 'decline' || p.action === 'end') phone.hangup()
  }, [state, phone, navigate])

  useEffect(() => {
    const act = ({ action }: Partial<MobileCallAction>) => {
      if (!action) return
      const p = latest.current
      const c = p.call
      const ringing = c?.direction === 'incoming' && c.state === 'ringing'
      switch (action) {
        case 'answer':
          if (ringing) {
            p.answer()
            navigate('/desktop')
          } else if (!c) pending.current = { action, at: Date.now() }
          break
        case 'decline':
        case 'end':
          if (c) p.hangup()
          else pending.current = { action, at: Date.now() }
          break
        case 'mute':
        case 'unmute':
          if (c && c.muted !== (action === 'mute')) p.toggleMute()
          break
        case 'hold':
        case 'unhold':
          if (c && c.held !== (action === 'hold')) p.toggleHold()
          break
        case 'show':
          navigate('/desktop')
          break
      }
    }
    mobile.takePending().then((p) => p && act(p)).catch(() => undefined)
    const offs = [
      mobile.listen<MobileCallAction>('callAction', act),
      mobile.listen<{ token: string }>('token', () => { register(true).catch(() => undefined) }),
      mobile.listen<{ kind: string }>('push', ({ kind }) => {
        if (kind === 'voicemail') voicemail.reload()
      }),
    ]
    return () => offs.forEach((o) => o())
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return null
}
