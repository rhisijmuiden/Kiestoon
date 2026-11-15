import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import type { RTCSession } from 'jssip/lib/RTCSession'
import type { UA } from 'jssip/lib/UA'
import { api } from './api'
import type { WebRTCAccount } from './types'

// The browser softphone. It registers over a WebSocket that the portal proxies to Asterisk on
// the same origin, so it needs no extra port and only works for a signed-in user.

export type PhoneStatus = 'off' | 'connecting' | 'registered' | 'failed' | 'insecure' | 'nomic'
export type CallState = {
  direction: 'incoming' | 'outgoing'
  remote: string
  remoteName: string
  state: 'ringing' | 'calling' | 'active'
  muted: boolean
  held: boolean
  startedAt: number
}

type Ctx = {
  available: boolean
  status: PhoneStatus
  call: CallState | null
  open: boolean
  setOpen: (v: boolean) => void
  enable: () => void
  dial: (n: string) => void
  answer: () => void
  hangup: () => void
  toggleMute: () => void
  toggleHold: () => void
  dtmf: (d: string) => void
  /** Rings the desk phone first (click-to-call) when the softphone is not registered. */
  callAny: (n: string) => Promise<'softphone' | 'deskphone'>
}

const SoftphoneContext = createContext<Ctx | null>(null)

function ringer() {
  let ctx: AudioContext | null = null
  let timer = 0
  const beep = () => {
    if (!ctx) return
    const o = ctx.createOscillator()
    const g = ctx.createGain()
    o.frequency.value = 440
    o.connect(g)
    g.connect(ctx.destination)
    g.gain.setValueAtTime(0.08, ctx.currentTime)
    o.start()
    o.stop(ctx.currentTime + 0.9)
  }
  return {
    start() {
      try {
        ctx = new AudioContext()
        beep()
        timer = window.setInterval(beep, 3000)
      } catch {
        /* no audio */
      }
    },
    stop() {
      window.clearInterval(timer)
      ctx?.close()
      ctx = null
    },
  }
}

export function SoftphoneProvider({ webrtc, children }: { webrtc: boolean; children: ReactNode }) {
  const [status, setStatus] = useState<PhoneStatus>('off')
  const [call, setCall] = useState<CallState | null>(null)
  const [open, setOpen] = useState(false)
  const ua = useRef<UA | null>(null)
  const session = useRef<RTCSession | null>(null)
  const account = useRef<WebRTCAccount | null>(null)
  const audio = useRef<HTMLAudioElement | null>(null)
  const ring = useRef(ringer())

  const attach = useCallback((s: RTCSession) => {
    const play = (pc: RTCPeerConnection) => {
      pc.addEventListener('track', (ev) => {
        if (audio.current && ev.streams[0]) {
          audio.current.srcObject = ev.streams[0]
          audio.current.play().catch(() => undefined)
        }
      })
    }
    if (s.connection) play(s.connection as unknown as RTCPeerConnection)
    else s.on('peerconnection', (e) => play(e.peerconnection as unknown as RTCPeerConnection))
    const end = () => {
      ring.current.stop()
      session.current = null
      setCall(null)
    }
    s.on('accepted', () => { ring.current.stop(); setCall((c) => (c ? { ...c, state: 'active', startedAt: Date.now() } : c)) })
    s.on('ended', end)
    s.on('failed', end)
    s.on('hold', () => setCall((c) => (c ? { ...c, held: true } : c)))
    s.on('unhold', () => setCall((c) => (c ? { ...c, held: false } : c)))
    s.on('muted', () => setCall((c) => (c ? { ...c, muted: true } : c)))
    s.on('unmuted', () => setCall((c) => (c ? { ...c, muted: false } : c)))
  }, [])

  const enable = useCallback(async () => {
    if (ua.current) return
    if (!window.isSecureContext) return setStatus('insecure')
    setStatus('connecting')
    try {
      const acc = await api<WebRTCAccount>('GET', '/portal/webrtc')
      account.current = acc
      const JsSIP = await import('jssip')
      const url = `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}${acc.wsPath}`
      const socket = new JsSIP.WebSocketInterface(url)
      const u = new JsSIP.UA({
        sockets: [socket], uri: `sip:${acc.username}@${acc.domain}`, password: acc.password, display_name: acc.name,
        register: true, session_timers: false, user_agent: 'Kiestoon Web',
      })
      u.on('registered', () => setStatus('registered'))
      u.on('unregistered', () => setStatus('connecting'))
      u.on('registrationFailed', () => setStatus('failed'))
      u.on('disconnected', () => setStatus('connecting'))
      u.on('newRTCSession', (e: { session: RTCSession; originator: string }) => {
        const s = e.session as RTCSession
        if (e.originator === 'local') return
        if (session.current) {
          s.terminate({ status_code: 486 })
          return
        }
        session.current = s
        attach(s)
        const from = s.remote_identity
        setCall({ direction: 'incoming', remote: from.uri.user, remoteName: from.display_name ?? '', state: 'ringing', muted: false, held: false, startedAt: 0 })
        setOpen(true)
        ring.current.start()
      })
      u.start()
      ua.current = u
    } catch {
      setStatus('failed')
    }
  }, [attach])

  useEffect(() => {
    let auto = false
    try {
      auto = localStorage.getItem('kiestoon.softphone') === 'on'
    } catch {
      /* ignore */
    }
    if (webrtc && auto) enable()
    return () => {
      ua.current?.stop()
      ua.current = null
    }
  }, [webrtc, enable])

  const remember = () => {
    try {
      localStorage.setItem('kiestoon.softphone', 'on')
    } catch {
      /* ignore */
    }
  }

  const dial = useCallback((n: string) => {
    const u = ua.current
    const acc = account.current
    if (!u || !acc || session.current || !n) return
    try {
      const s = u.call(`sip:${n}@${acc.domain}`, { mediaConstraints: { audio: true, video: false }, pcConfig: { iceServers: [] }, rtcOfferConstraints: { offerToReceiveAudio: true, offerToReceiveVideo: false } })
      session.current = s
      attach(s)
      setCall({ direction: 'outgoing', remote: n, remoteName: '', state: 'calling', muted: false, held: false, startedAt: 0 })
      setOpen(true)
    } catch {
      setStatus('nomic')
    }
  }, [attach])

  const ctx: Ctx = {
    available: webrtc,
    status,
    call,
    open,
    setOpen,
    enable: () => { remember(); enable() },
    dial,
    answer: () => {
      ring.current.stop()
      session.current?.answer({ mediaConstraints: { audio: true, video: false }, pcConfig: { iceServers: [] } })
    },
    hangup: () => {
      ring.current.stop()
      try {
        session.current?.terminate()
      } catch {
        session.current = null
        setCall(null)
      }
    },
    toggleMute: () => {
      const s = session.current
      if (!s) return
      if (s.isMuted().audio) s.unmute({ audio: true })
      else s.mute({ audio: true })
    },
    toggleHold: () => {
      const s = session.current
      if (!s) return
      if (s.isOnHold().local) s.unhold()
      else s.hold()
    },
    dtmf: (d) => session.current?.sendDTMF(d),
    callAny: async (n) => {
      if (status === 'registered') {
        dial(n)
        return 'softphone'
      }
      await api('POST', '/portal/call', { number: n })
      return 'deskphone'
    },
  }

  return (
    <SoftphoneContext.Provider value={ctx}>
      {children}
      <audio ref={audio} autoPlay className="hidden" />
    </SoftphoneContext.Provider>
  )
}

export function useSoftphone() {
  const c = useContext(SoftphoneContext)
  if (!c) throw new Error('useSoftphone outside SoftphoneProvider')
  return c
}
