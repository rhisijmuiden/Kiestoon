import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, Grid3x3, GripVertical, Mic, MicOff, Pause, Phone, PhoneForwarded, PhoneOff, Play, Undo2 } from 'lucide-react'
import { formatDuration, useApi } from '../components/kit'
import { ErrorMessage } from '../components/ui'
import { api } from '../lib/api'
import { useSoftphone } from '../lib/softphone'
import { useTransfer, type TransferSource } from '../lib/transfer'
import type { ActiveCall } from '../lib/types'
import { Avatar } from './Avatar'
import { useDesktop } from './data'
import { presenceOf, presenceText } from './presence'

function useTimer(start: number) {
  const [now, setNow] = useState(Date.now())
  useEffect(() => {
    if (!start) return
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [start])
  return start ? Math.max(0, Math.floor((now - start) / 1000)) : 0
}

const DTMF = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '*', '0', '#']

/** The call in progress: answer, mute, hold, keypad, hang up, and transfer by dragging it onto a colleague. */
export function CallPanel() {
  const { t } = useTranslation()
  const phone = useSoftphone()
  const x = useTransfer()
  const { targets, me } = useDesktop()
  const call = phone.call!
  const secs = useTimer(call.state === 'active' ? call.startedAt : 0)
  const [keys, setKeys] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const active = useApi<{ connected: boolean; calls: ActiveCall[] }>(call.state === 'active' ? '/portal/active' : null, 1500)
  // The server-side leg of this softphone call: what transfers move.
  const leg = (active.data?.calls ?? []).find((c) => c.device === 'web') ?? null
  const contact = targets.data?.find((e) => e.kind === 'extension' && e.number === call.remote)
  const name = call.remoteName || contact?.name || leg?.peerName || ''
  const src: TransferSource | null = leg && !leg.consult ? { channel: leg.channel, label: name || call.remote, scope: 'portal' } : null

  async function transfer(target: string, targetName: string, mode: 'blind' | 'attended') {
    if (!leg) return
    setError(null)
    try {
      await api('POST', '/portal/transfer', { channel: leg.channel, target, mode })
      if (mode === 'attended') active.reload()
    } catch (e) {
      setError(e)
    }
    void targetName
  }
  async function step(what: 'complete' | 'cancel') {
    if (!leg) return
    setError(null)
    try {
      await api('POST', `/portal/transfer/${what}`, { channel: leg.channel })
    } catch (e) {
      setError(e)
    }
    active.reload()
  }

  const ringing = call.direction === 'incoming' && call.state === 'ringing'
  const people = (targets.data ?? []).filter((e) => e.kind === 'extension' && e.number !== me.data?.number && presenceOf(e) !== 'offline')
  const btn = 'flex h-14 flex-col items-center justify-center gap-1 rounded-xl border text-[10.5px] transition disabled:opacity-40'
  const off = 'border-line bg-field text-muted hover:text-ink'
  const on = 'border-accent text-accent bg-accent/10'

  return (
    <section aria-label={t('desktop.call.title')} className="flex flex-col gap-3 border-b border-line p-3.5">
      <div {...(src ? x.dragHandlers(src) : {})}
        className={`rounded-2xl border border-line bg-gradient-to-b from-surface to-surface-2 p-4 text-center ${src ? 'cursor-grab touch-none select-none active:cursor-grabbing' : ''}`}>
        <div className="text-xs text-muted">
          {ringing ? t('desktop.call.incoming') : call.state === 'calling' ? t('desktop.call.calling') : call.held ? t('desktop.call.onHold') : t('desktop.call.inCall')}
        </div>
        <div className="my-2.5 flex justify-center"><Avatar name={name || call.remote} size="lg" /></div>
        <div className="truncate text-base font-semibold text-ink">{name || call.remote || t('reception.unknownCaller')}</div>
        {name && <div className="font-mono text-xs text-muted">{call.remote}</div>}
        {call.state === 'active' && <div className="mt-1.5 font-mono text-[22px] text-accent">{formatDuration(secs)}</div>}

        {ringing ? (
          <div className="mt-4 grid grid-cols-2 gap-2">
            <button type="button" onClick={phone.answer} className="flex h-11 items-center justify-center gap-2 rounded-xl bg-ok font-semibold text-white">
              <Phone className="size-4" aria-hidden /> {t('desktop.call.answer')}
            </button>
            <button type="button" onClick={phone.hangup} className="flex h-11 items-center justify-center gap-2 rounded-xl bg-danger font-semibold text-white">
              <PhoneOff className="size-4" aria-hidden /> {t('desktop.call.decline')}
            </button>
          </div>
        ) : (
          <>
            <div className="mt-4 grid grid-cols-4 gap-2">
              <button type="button" className={`${btn} ${call.muted ? on : off}`} onClick={phone.toggleMute} aria-pressed={call.muted} disabled={call.state !== 'active'}>
                {call.muted ? <MicOff className="size-[18px]" aria-hidden /> : <Mic className="size-[18px]" aria-hidden />}{t('desktop.call.mute')}
              </button>
              <button type="button" className={`${btn} ${call.held ? on : off}`} onClick={phone.toggleHold} aria-pressed={call.held} disabled={call.state !== 'active'}>
                {call.held ? <Play className="size-[18px]" aria-hidden /> : <Pause className="size-[18px]" aria-hidden />}{call.held ? t('desktop.call.resume') : t('desktop.call.hold')}
              </button>
              <button type="button" className={`${btn} ${x.active ? on : off}`} disabled={!src} onClick={() => x.pick(x.active ? null : src)} aria-pressed={!!x.active}>
                <PhoneForwarded className="size-[18px]" aria-hidden />{t('desktop.call.transfer')}
              </button>
              <button type="button" className={`${btn} ${keys ? on : off}`} onClick={() => setKeys(!keys)} aria-pressed={keys} disabled={call.state !== 'active'}>
                <Grid3x3 className="size-[18px]" aria-hidden />{t('desktop.call.keypad')}
              </button>
            </div>
            {keys && (
              <div className="mt-2 grid grid-cols-3 gap-1.5">
                {DTMF.map((d) => (
                  <button key={d} type="button" onClick={() => phone.dtmf(d)} className="h-9 rounded-lg border border-line bg-field font-mono text-ink hover:border-accent/50">{d}</button>
                ))}
              </div>
            )}
            <button type="button" onClick={phone.hangup} aria-label={t('desktop.call.hangup')} title={t('desktop.call.hangup')}
              className="mx-auto mt-4 flex h-11 w-16 items-center justify-center rounded-full bg-danger text-white hover:brightness-110">
              <PhoneOff className="size-5" aria-hidden />
            </button>
          </>
        )}
      </div>

      <ErrorMessage error={error} />

      {leg?.consult && (
        <div className="flex flex-col gap-2 rounded-xl border border-warn/40 bg-warn/[0.06] p-3 text-sm">
          <span className="text-warn-ink">{leg.consult.answered ? t('transfer.consultTalking', { target: leg.consult.targetName || leg.consult.target }) : t('transfer.consultRinging', { target: leg.consult.targetName || leg.consult.target })}</span>
          <div className="grid grid-cols-2 gap-2">
            <button type="button" onClick={() => step('complete')} className="flex h-9 items-center justify-center gap-1.5 rounded-lg bg-gradient-to-br from-accent to-accent-2 text-xs font-semibold text-on-accent"><Check className="size-4" aria-hidden />{t('transfer.complete')}</button>
            <button type="button" onClick={() => step('cancel')} className="flex h-9 items-center justify-center gap-1.5 rounded-lg border border-line text-xs text-ink"><Undo2 className="size-4" aria-hidden />{t('transfer.cancel')}</button>
          </div>
        </div>
      )}

      {src && people.length > 0 && (
        <div className="rounded-2xl border-[1.5px] border-dashed border-accent/70 bg-accent/[0.05] p-2.5">
          <div className="mb-1.5 flex items-center gap-1.5 px-1 text-[11.5px] font-semibold text-accent"><GripVertical className="size-3.5" aria-hidden />{t('desktop.call.dragHint')}</div>
          <div className="flex max-h-56 flex-col gap-1 overflow-y-auto">
            {people.map((p) => {
              const pr = presenceOf(p)
              const over = x.dragging && x.hover === p.number
              return (
                <div key={p.number} data-xfer-target={p.number} data-xfer-name={p.name}
                  className={`flex items-center gap-2.5 rounded-xl border px-2 py-1.5 transition ${over ? 'scale-[1.02] border-accent bg-accent/15' : x.active && !x.dragging ? 'border-dashed border-accent/50 bg-surface' : 'border-transparent'}`}>
                  <Avatar name={p.name} presence={pr} />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-[13px] font-medium text-ink">{p.name}</span>
                    <span className="block truncate text-[11px] text-muted">{p.number} · <span className={presenceText[pr]}>{t(`desktop.presence.${pr}`)}</span></span>
                  </span>
                  <button type="button" onClick={() => transfer(p.number, p.name, 'blind')} className="h-7 rounded-lg border border-line bg-surface px-2 text-[11px] font-semibold text-ink hover:border-accent/50">{t('desktop.call.direct')}</button>
                  <button type="button" onClick={() => transfer(p.number, p.name, 'attended')} className="h-7 rounded-lg bg-gradient-to-br from-accent to-accent-2 px-2 text-[11px] font-semibold text-on-accent">{t('desktop.call.consult')}</button>
                </div>
              )
            })}
          </div>
          <p className="mt-1.5 px-1 text-[10.5px] text-subtle">{t('desktop.call.dragMode', { mode: t(`transfer.mode.${x.mode}`) })}</p>
        </div>
      )}
    </section>
  )
}
