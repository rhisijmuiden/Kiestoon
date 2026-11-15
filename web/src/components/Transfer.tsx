import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, GripVertical, Headset, ListOrdered, Monitor, Phone, PhoneForwarded, Undo2, Users, UsersRound, X } from 'lucide-react'
import { Badge, Empty, SectionTitle, formatDuration, useApi } from './kit'
import { Segmented, stateTone } from './pbx'
import { Alert, Button, Card, ErrorMessage, Input } from './ui'
import { api } from '../lib/api'
import { useTransfer, type TransferSource } from '../lib/transfer'
import type { ActiveCall, LiveLeg, TransferTarget } from '../lib/types'

/** Success or error of the last transfer. */
export function TransferNotice() {
  const x = useTransfer()
  if (!x.notice) return null
  if (x.notice.tone === 'danger') return <ErrorMessage error={x.notice.error} />
  return <Alert tone="ok">{x.notice.text}</Alert>
}

/** The user's own calls, each one a card that can be dragged onto a colleague or a group. */
export function ActiveCalls({ hint }: { hint?: boolean }) {
  const { t } = useTranslation()
  const x = useTransfer()
  const active = useApi<{ connected: boolean; calls: ActiveCall[] }>('/portal/active', 1500)
  const [error, setError] = useState<unknown>(null)
  const calls = active.data?.calls ?? []
  async function step(c: ActiveCall, what: 'complete' | 'cancel') {
    setError(null)
    try {
      await api('POST', `/portal/transfer/${what}`, { channel: c.channel })
    } catch (e) {
      setError(e)
    }
    active.reload()
  }
  useEffect(() => {
    if (!x.busy) active.reload()
  }, [x.busy])

  return (
    <Card className="lg:sticky lg:top-4 lg:z-10">
      <SectionTitle action={
        <Segmented options={[{ key: 'blind', label: t('transfer.mode.blind') }, { key: 'attended', label: t('transfer.mode.attended') }]}
          value={x.mode} onChange={x.setMode} />
      }>{t('transfer.myCalls')}</SectionTitle>
      <ErrorMessage error={error} />
      {calls.length === 0 ? (
        <p className="text-sm text-muted">{t('transfer.noCalls')}</p>
      ) : (
        <div className="flex flex-col gap-2">
          {calls.map((c) => {
            const src: TransferSource = { channel: c.channel, label: c.peer || c.peerName || '?', scope: 'portal' }
            const picking = x.active?.channel === c.channel
            const canDrag = c.state === 'talking' && !c.consult
            return (
              <div key={c.channel}
                {...(canDrag ? x.dragHandlers(src) : {})}
                className={`flex flex-wrap items-center gap-3 rounded-xl border px-3 py-2.5 transition ${canDrag ? 'cursor-grab touch-none select-none active:cursor-grabbing' : ''} ${picking ? 'border-accent bg-accent/10 shadow-[0_0_24px_-8px_var(--color-accent)]' : c.consult ? 'border-warn/40 bg-warn/[0.06]' : 'border-line bg-surface-2/60'}`}>
                {canDrag && <GripVertical className="size-4 flex-none text-subtle" aria-hidden />}
                <span className={`size-2.5 flex-none rounded-full ${c.state === 'talking' ? 'bg-ok glow-ok' : 'bg-accent animate-pulse'}`} aria-hidden />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-ink"><span className={c.peerName ? 'font-medium' : 'font-mono'}>{c.peerName || c.peer || t('reception.unknownCaller')}</span></span>
                  <span className="flex flex-wrap items-center gap-x-2 text-xs text-subtle">
                    {c.peerName && c.peer && <span className="font-mono">{c.peer}</span>}
                    <span className="inline-flex items-center gap-1">{c.device === 'web' ? <Monitor className="size-3.5" aria-hidden /> : <Phone className="size-3.5" aria-hidden />}{t(`transfer.device.${c.device}`)}</span>
                    <span className="font-mono">{c.state === 'talking' ? formatDuration(c.duration) : t('transfer.ringing')}</span>
                  </span>
                </span>
                {c.consult ? (
                  <span className="flex w-full flex-wrap items-center gap-2 sm:w-auto">
                    <Badge tone={c.consult.answered ? 'ok' : 'warn'} dot>
                      {c.consult.answered ? t('transfer.consultTalking', { target: c.consult.targetName || c.consult.target }) : t('transfer.consultRinging', { target: c.consult.targetName || c.consult.target })}
                    </Badge>
                    <Button onClick={() => step(c, 'complete')}><Check className="size-4" aria-hidden /> {t('transfer.complete')}</Button>
                    <Button variant="ghost" onClick={() => step(c, 'cancel')}><Undo2 className="size-4" aria-hidden /> {t('transfer.cancel')}</Button>
                  </span>
                ) : canDrag && (
                  picking && !x.dragging
                    ? <Button variant="ghost" className="w-full sm:w-auto" onClick={() => x.pick(null)}><X className="size-4" aria-hidden /> {t('common.cancel')}</Button>
                    : <Button variant="ghost" className="w-full sm:w-auto" onClick={() => x.pick(src)} disabled={x.busy}><PhoneForwarded className="size-4" aria-hidden /> {t('transfer.transfer')}</Button>
                )}
              </div>
            )
          })}
        </div>
      )}
      {hint && calls.some((c) => c.state === 'talking' && !c.consult) && (
        <p className="mt-3 text-xs text-subtle">{x.active && !x.dragging ? t('transfer.pickHint') : t('transfer.dragHint')}</p>
      )}
    </Card>
  )
}

/** Live calls of the whole PBX for the switchboard. Every party can be dragged elsewhere. */
export function SwitchboardCalls({ calls }: { calls: { id: string; from: string; fromName: string; to: string; state: string; duration: number; legs?: LiveLeg[] }[] }) {
  const { t } = useTranslation()
  const x = useTransfer()
  return (
    <Card>
      <SectionTitle>{t('transfer.liveCalls')}</SectionTitle>
      {calls.length === 0 ? <p className="text-sm text-muted">{t('transfer.noLiveCalls')}</p> : (
        <div className="grid gap-2 lg:grid-cols-2">
          {calls.map((c) => (
            <div key={c.id} className="flex flex-col gap-2 rounded-xl border border-line bg-surface-2/60 p-3">
              <div className="flex items-center justify-between text-xs text-subtle">
                <span>{t(`transfer.state.${c.state}`, { defaultValue: c.state })}</span>
                <span className="font-mono">{formatDuration(c.duration)}</span>
              </div>
              <div className="flex flex-wrap gap-2">
                {(c.legs ?? []).filter((l) => l.channel.startsWith('PJSIP/')).map((l) => {
                  const external = l.endpoint.startsWith('trunk-')
                  const num = external ? l.callerNum : l.endpoint.replace(/-web$/, '')
                  const name = external ? l.callerName : l.callerName && l.callerName !== num ? l.callerName : ''
                  const src: TransferSource = { channel: l.channel, label: num || '?', scope: 'reception' }
                  const picking = x.active?.channel === l.channel
                  return (
                    <div key={l.channel} {...x.dragHandlers(src)}
                      className={`flex min-h-11 min-w-0 flex-1 cursor-grab touch-none select-none items-center gap-2 rounded-lg border px-2.5 active:cursor-grabbing ${picking ? 'border-accent bg-accent/10' : 'border-line bg-surface'}`}>
                      <GripVertical className="size-4 flex-none text-subtle" aria-hidden />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate font-mono text-sm text-ink">{num || t('reception.unknownCaller')}</span>
                        <span className="block truncate text-xs text-subtle">{name || (external ? t('transfer.external') : t('transfer.internal'))}</span>
                      </span>
                      <button type="button" onClick={() => x.pick(picking ? null : src)} aria-label={t('transfer.transfer')} title={t('transfer.transfer')}
                        className={`inline-flex size-9 flex-none items-center justify-center rounded-lg transition ${picking ? 'bg-accent/20 text-accent' : 'text-muted hover:bg-tint/5 hover:text-ink'}`}>
                        {picking ? <X className="size-4" aria-hidden /> : <PhoneForwarded className="size-4" aria-hidden />}
                      </button>
                    </div>
                  )
                })}
              </div>
            </div>
          ))}
        </div>
      )}
      {calls.length > 0 && <p className="mt-3 text-xs text-subtle">{x.active && !x.dragging ? t('transfer.pickHint') : t('transfer.dragHintReception')}</p>}
    </Card>
  )
}

const glow: Record<string, string> = { ok: 'bg-ok glow-ok', danger: 'bg-danger shadow-[0_0_10px_var(--color-danger)]', accent: 'bg-accent animate-pulse shadow-[0_0_10px_var(--color-accent)]', warn: 'bg-warn', muted: 'bg-subtle' }

/** Colleagues, ring groups and queues as drop targets. A plain click calls the target. */
export function TransferTargets({ path, self, onCall }: { path: string; self?: string; onCall?: (n: string) => void }) {
  const { t } = useTranslation()
  const x = useTransfer()
  const list = useApi<TransferTarget[]>(path, 3000)
  const [q, setQ] = useState('')
  const all = (list.data ?? []).filter((e) => !q || e.number.includes(q) || e.name.toLowerCase().includes(q.toLowerCase()))
  const armed = !!x.active
  const groups: { kind: TransferTarget['kind']; title: string; icon: ReactNode }[] = [
    { kind: 'extension', title: t('transfer.colleagues'), icon: <UsersRound className="size-4" aria-hidden /> },
    { kind: 'ringgroup', title: t('transfer.ringGroups'), icon: <Users className="size-4" aria-hidden /> },
    { kind: 'queue', title: t('transfer.queues'), icon: <ListOrdered className="size-4" aria-hidden /> },
  ]
  function activate(e: TransferTarget) {
    if (armed) x.drop(e.number, e.name)
    else onCall?.(e.number)
  }
  return (
    <Card>
      <ErrorMessage error={list.error} />
      <div className="mb-4 max-w-xs"><Input label={t('common.search')} value={q} onChange={(e) => setQ(e.target.value)} /></div>
      {all.length === 0 && <Empty icon={<Headset className="size-7" />} title={t('transfer.noTargets')} />}
      <div className="flex flex-col gap-5">
        {groups.map((g) => {
          const items = all.filter((e) => e.kind === g.kind)
          if (!items.length) return null
          return (
            <section key={g.kind} aria-label={g.title}>
              <h3 className="mb-2 flex items-center gap-2 text-[11px] font-medium uppercase tracking-[0.08em] text-subtle">{g.icon} {g.title}</h3>
              <div className="grid grid-cols-[repeat(auto-fill,minmax(min(100%,190px),1fr))] gap-2">
                {items.map((e) => {
                  const mine = e.kind === 'extension' && e.number === self
                  const tone = e.kind !== 'extension' ? 'accent' : e.dnd && e.state === 'idle' ? 'warn' : stateTone(e.state ?? 'unknown')
                  const over = x.hover === e.number && x.dragging
                  const clickable = !mine && (armed || !!onCall)
                  const status = e.kind === 'extension'
                    ? (e.dnd ? t('states.dnd') : t(`states.${e.state}`, { defaultValue: e.state }))
                    : e.kind === 'queue' && e.waiting ? t('transfer.waiting', { count: e.waiting }) : t('transfer.members', { count: e.members ?? 0 })
                  const inner = (
                    <>
                      <span className={`size-2.5 flex-none rounded-full ${e.kind === 'extension' ? glow[tone] : 'bg-accent-2'}`} aria-hidden />
                      <span className="min-w-0 flex-1 text-left">
                        <span className="flex items-center justify-between gap-2">
                          <span className="font-mono text-sm text-ink">{e.number}</span>
                          <span className="truncate text-[11px] text-subtle">{over ? <PhoneForwarded className="size-4 text-accent" aria-hidden /> : status}</span>
                        </span>
                        <span className="block truncate text-xs text-muted" title={e.name}>{e.name}</span>
                      </span>
                    </>
                  )
                  const cls = `flex min-h-14 items-center gap-3 rounded-xl border px-3 transition ${
                    over ? 'scale-[1.02] border-accent bg-accent/15 shadow-[0_0_28px_-8px_var(--color-accent)]'
                      : mine ? 'border-accent/40 bg-accent/[0.05]'
                        : armed ? 'border-dashed border-accent/50 bg-accent/[0.04] hover:border-accent hover:bg-accent/10'
                          : 'border-line bg-surface-2/60 hover:border-accent/50 hover:bg-accent/[0.06]'}`
                  return clickable ? (
                    <button key={e.kind + e.number} type="button" data-xfer-target={e.number} data-xfer-name={e.name} onClick={() => activate(e)} className={cls}
                      title={armed ? t('transfer.transferTo', { target: e.name || e.number }) : t('portal.callNumber', { number: e.number })}>
                      {inner}
                    </button>
                  ) : (
                    <div key={e.kind + e.number} className={cls} data-xfer-target={mine ? undefined : e.number} data-xfer-name={e.name}>{inner}</div>
                  )
                })}
              </div>
            </section>
          )
        })}
      </div>
    </Card>
  )
}
