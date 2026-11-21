import { useEffect, useRef } from 'react'
import { useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { api } from '../lib/api'
import { refreshHeadset, useSoftphone } from '../lib/softphone'
import { useCrmLookup, useCrmStatus } from '../lib/crm'
import { listen, shell, type TrayStatus } from './bridge'
import { myQueues, useDesktop } from './data'

/**
 * Keeps the Windows shell in step with the softphone: the incoming-call popup, the tray icon and
 * menu, the headset lights, notifications for missed calls and new voicemail. It also handles
 * what comes back: answer/decline from the popup, headset buttons, tray menu choices and tel: links.
 */
export function ShellSync() {
  const { t, i18n } = useTranslation()
  const phone = useSoftphone()
  const { me, queues, voicemail, targets } = useDesktop()
  const navigate = useNavigate()
  const latest = useRef({ phone, me: me.data, queues: queues.data })
  latest.current = { phone, me: me.data, queues: queues.data }

  const call = phone.call
  const ringing = call?.direction === 'incoming' && call.state === 'ringing'
  // A caller from outside gets the name (and company) the CRM knows.
  const crmOn = !!useCrmStatus()?.active
  const crm = useCrmLookup(ringing && call && call.remote.replace(/\D/g, '').length >= 7 ? call.remote : null, crmOn)
  const crmContact = crm?.contacts?.[0]
  const callerName = call ? crmContact?.name || call.remoteName || targets.data?.find((e) => e.number === call.remote)?.name || '' : ''
  const callerVia = crmContact?.company && crmContact.company !== crmContact.name ? crmContact.company : ''

  // Incoming-call popup, and a notification when it was missed.
  const wasRinging = useRef<{ name: string; number: string } | null>(null)
  useEffect(() => {
    if (ringing && call) {
      wasRinging.current = { name: callerName, number: call.remote }
      shell.incomingShow(callerName, call.remote, callerVia, i18n.language).catch(() => undefined)
      return
    }
    shell.incomingHide().catch(() => undefined)
    const r = wasRinging.current
    wasRinging.current = null
    if (r && !call) shell.notify(t('desktop.notify.missed'), r.name ? `${r.name} (${r.number})` : r.number || t('reception.unknownCaller')).catch(() => undefined)
  }, [ringing, call, callerName, callerVia, i18n.language, t])

  // Device names only show after the microphone was opened once (the shell allows it without a
  // prompt). Open it briefly at start, so a plugged-in headset is known before the first call.
  useEffect(() => {
    navigator.mediaDevices?.getUserMedia({ audio: true })
      .then((s) => s.getTracks().forEach((tr) => tr.stop()))
      .then(refreshHeadset)
      .catch(() => undefined)
  }, [])

  // Headset lights.
  const offhook = !!call && !ringing
  useEffect(() => {
    shell.headsetState(ringing, offhook, !!call?.muted).catch(() => undefined)
  }, [ringing, offhook, call?.muted])

  // Tray icon, tooltip and menu labels.
  const dnd = !!me.data?.features.dnd
  const q = myQueues(queues.data, me.data?.id)
  const status: TrayStatus = ringing ? 'ringing' : call ? 'call' : phone.status !== 'registered' ? 'offline' : dnd ? 'dnd' : 'available'
  useEffect(() => {
    const line = status === 'offline' ? t(`desktop.status.${phone.status}`) : status === 'call' || status === 'ringing' ? t('desktop.call.inCall') : t(`desktop.presence.${status}`)
    const tip = `Kiestoon · ${me.data?.number ?? ''} · ${line}` + (q.mine.length ? ` · ${t('desktop.queuesSignedIn', { count: q.signedIn.length, total: q.mine.length })}` : '')
    shell.setStatus(status, tip, {
      open: t('desktop.tray.open'), available: t('desktop.presence.available'), dnd: t('desktop.presence.dnd'),
      queuesIn: t('portal.signInAll'), queuesOut: t('portal.signOutAll'), info: t('desktop.nav.info'), quit: t('desktop.tray.quit'),
      hasQueues: q.dynamic.length ? '1' : '',
    }).catch(() => undefined)
  }, [status, phone.status, me.data?.number, q.mine.length, q.signedIn.length, q.dynamic.length, t])

  // New voicemail.
  const inbox = (voicemail.data ?? []).filter((m) => m.folder === 'INBOX').length
  const lastInbox = useRef<number | null>(null)
  useEffect(() => {
    if (voicemail.data === null) return
    if (lastInbox.current !== null && inbox > lastInbox.current) shell.notify(t('desktop.notify.voicemail'), t('desktop.notify.voicemailText', { count: inbox })).catch(() => undefined)
    lastInbox.current = inbox
  }, [inbox, voicemail.data, t])

  // What the shell sends back.
  useEffect(() => {
    const setDnd = async (v: boolean) => {
      const m = latest.current.me
      if (!m) return
      await api('PUT', '/portal/features', { ...m.features, dnd: v }).catch(() => undefined)
      me.reload()
    }
    const offs = [
      listen<{ action: string }>('incoming', ({ action }) => {
        const p = latest.current.phone
        if (action === 'answer') {
          p.answer()
          shell.focus()
          navigate('/desktop')
        } else if (action === 'decline') p.hangup()
      }),
      listen<{ action: string }>('headset', ({ action }) => {
        const p = latest.current.phone
        const c = p.call
        if (action === 'hook-off' && c?.direction === 'incoming' && c.state === 'ringing') p.answer()
        else if (action === 'hook-on' && c) p.hangup()
        else if (action === 'mute' && c?.state === 'active') p.toggleMute()
      }),
      listen<{ action: string }>('tray', ({ action }) => {
        if (action === 'available' || action === 'dnd') setDnd(action === 'dnd')
        else if (action === 'queues-in' || action === 'queues-out') {
          api('POST', '/portal/queues/0', { action: action === 'queues-in' ? 'login' : 'logout' }).then(() => queues.reload()).catch(() => undefined)
        } else if (action === 'info') navigate('/desktop/info')
      }),
      listen<{ number: string }>('dial', ({ number }) => {
        const p = latest.current.phone
        const n = number.replace(/[^0-9+*#]/g, '')
        if (n && !p.call && p.status === 'registered') {
          navigate('/desktop')
          p.dial(n)
        } else if (n) navigate('/desktop/keypad')
      }),
    ]
    return () => offs.forEach((o) => o())
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return null
}
