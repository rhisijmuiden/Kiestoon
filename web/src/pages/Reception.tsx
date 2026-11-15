import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Headset, Pause, Play, LogIn, LogOut } from 'lucide-react'
import { Badge, Empty, SectionTitle, formatDuration, useApi } from '../components/kit'
import { IconButton, stateTone } from '../components/pbx'
import { Card, ErrorMessage, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { BlfEntry, Live, QueueView, TimeCondition } from '../lib/types'
import { ActiveCalls, SwitchboardCalls, TransferNotice, TransferTargets } from '../components/Transfer'
import { useSession } from '../lib/session'
import { TransferProvider } from '../lib/transfer'
import { ModeSwitch } from './TimeConditions'

/**
 * The switchboard: day/night switches, live calls that can be dragged onto a colleague, ring
 * group or queue, live queues with their agents and the busy lamp field.
 */
export function Reception() {
  const { t } = useTranslation()
  const { state } = useSession()
  const hasExt = state.kind === 'ready' && !!state.me.extension
  const tcs = useApi<TimeCondition[]>('/pbx/timeconditions', 15000)
  const queues = useApi<QueueView[]>('/reception/queues', 3000)
  const live = useApi<Live>('/live', 2000)
  async function agent(q: number, ext: number, action: string) {
    await api('POST', `/reception/queues/${q}/agents/${ext}`, { action })
    queues.reload()
  }
  return (
    <>
      <PageHeader title={t('reception.title')} subtitle={t('reception.subtitle')} />
      <ErrorMessage error={queues.error || live.error} />
      {(tcs.data?.length ?? 0) > 0 && (
        <Card>
          <SectionTitle>{t('reception.dayNight')}</SectionTitle>
          <div className="grid gap-3 lg:grid-cols-2 2xl:grid-cols-3">
            {tcs.data!.map((tc) => (
              <div key={tc.id} className="flex flex-col gap-2 rounded-xl border border-line p-4">
                <div className="flex items-center justify-between"><span className="font-medium text-ink">{tc.name}</span><span className="font-mono text-xs text-subtle">*28{tc.id}</span></div>
                <ModeSwitch tc={tc} onChanged={tcs.reload} />
              </div>
            ))}
          </div>
        </Card>
      )}
      <TransferProvider onDone={live.reload}>
        <TransferNotice />
        {hasExt && <ActiveCalls />}
        <SwitchboardCalls calls={live.data?.calls ?? []} />
        <TransferTargets path="/reception/targets" />
      </TransferProvider>
      <QueueBoard queues={queues.data ?? []} onAgent={agent} />
    </>
  )
}

/** Live queues with their callers and agents. onAgent is set when the viewer may steer agents. */
export function QueueBoard({ queues, onAgent, myExtensionId }: { queues: QueueView[]; onAgent?: (q: number, ext: number, action: string) => void; myExtensionId?: number }) {
  const { t } = useTranslation()
  if (!queues.length) return <Card><Empty icon={<Headset className="size-7" />} title={t('reception.noQueues')} /></Card>
  return (
    <div className="grid gap-6 xl:grid-cols-2">
      {queues.map((q) => {
        const live = q.live
        const agents = new Map((live?.agents ?? []).map((a) => [a.number, a]))
        return (
          <Card key={q.id}>
            <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
              <div>
                <h2 className="font-display text-lg font-semibold">{q.name}</h2>
                <span className="font-mono text-xs text-subtle">{q.number}</span>
              </div>
              <div className="flex gap-4 text-center">
                <Metric label={t('queues.waiting')} value={live?.calls ?? 0} warn={(live?.calls ?? 0) > 0} />
                <Metric label={t('reception.longest')} value={formatDuration(live?.longestWait ?? 0)} />
                <Metric label={t('reception.answered')} value={live?.completed ?? 0} />
                <Metric label={t('reception.abandoned')} value={live?.abandoned ?? 0} />
              </div>
            </div>
            {(live?.callers?.length ?? 0) > 0 && (
              <div className="mb-4 flex flex-col gap-1.5">
                {live!.callers!.map((c) => (
                  <div key={c.position} className="flex items-center gap-3 rounded-lg border border-warn/30 bg-warn/[0.06] px-3 py-2 text-sm">
                    <span className="font-mono text-xs text-warn">#{c.position}</span>
                    <span className="flex-1 font-mono text-ink">{c.callerId || t('reception.unknownCaller')} <span className="font-sans text-xs text-subtle">{c.name}</span></span>
                    <span className="font-mono text-xs text-warn">{formatDuration(c.wait)}</span>
                  </div>
                ))}
              </div>
            )}
            <div className="flex flex-col gap-1.5">
              {q.members.map((m) => {
                const a = agents.get(m.number)
                const signedIn = !m.dynamic || m.loggedIn
                const paused = a?.paused ?? m.paused
                const mine = myExtensionId === m.extensionId
                const steer = onAgent && (!myExtensionId || mine)
                return (
                  <div key={m.extensionId} className={`flex min-h-11 items-center gap-3 rounded-lg border px-3 ${mine ? 'border-accent/40 bg-accent/[0.05]' : 'border-line'} ${signedIn ? '' : 'opacity-60'}`}>
                    <span className="font-mono text-ink">{m.number}</span>
                    <span className="flex-1 truncate text-sm text-muted">{m.name}</span>
                    {!signedIn ? <Badge>{t('reception.signedOut')}</Badge>
                      : paused ? <Badge tone="warn" dot>{t('reception.paused')}</Badge>
                        : a?.inCall ? <Badge tone="danger" dot>{t('reception.onCall')}</Badge>
                          : <Badge tone={stateTone(a?.status ?? 'unknown')} dot>{t(`states.${a?.status ?? 'unknown'}`, { defaultValue: a?.status })}</Badge>}
                    {a && <span className="hidden font-mono text-xs text-subtle sm:inline" title={t('reception.callsTaken')}>{a.callsTaken}</span>}
                    {steer && (
                      <span className="flex">
                        {m.dynamic && (signedIn
                          ? <IconButton label={t('reception.signOut')} onClick={() => onAgent!(q.id, m.extensionId, 'logout')}><LogOut className="size-4" aria-hidden /></IconButton>
                          : <IconButton label={t('reception.signIn')} onClick={() => onAgent!(q.id, m.extensionId, 'login')}><LogIn className="size-4" aria-hidden /></IconButton>)}
                        {signedIn && (paused
                          ? <IconButton label={t('reception.resume')} onClick={() => onAgent!(q.id, m.extensionId, 'unpause')}><Play className="size-4" aria-hidden /></IconButton>
                          : <IconButton label={t('reception.pause')} onClick={() => onAgent!(q.id, m.extensionId, 'pause')}><Pause className="size-4" aria-hidden /></IconButton>)}
                      </span>
                    )}
                  </div>
                )
              })}
            </div>
          </Card>
        )
      })}
    </div>
  )
}

function Metric({ label, value, warn }: { label: string; value: ReactNode; warn?: boolean }) {
  return (
    <div>
      <div className={`font-mono text-xl ${warn ? 'text-warn' : 'text-ink'}`}>{value}</div>
      <div className="text-[11px] uppercase tracking-[0.08em] text-subtle">{label}</div>
    </div>
  )
}

/** Busy lamp field: one tile per extension. onCall makes the tiles clickable. */
export function BlfGrid({ entries, onCall, self }: { entries: BlfEntry[]; onCall?: (n: string) => void; self?: string }) {
  const { t } = useTranslation()
  const glow: Record<string, string> = { ok: 'bg-ok glow-ok', danger: 'bg-danger shadow-[0_0_10px_var(--color-danger)]', accent: 'bg-accent animate-pulse shadow-[0_0_10px_var(--color-accent)]', warn: 'bg-warn', muted: 'bg-subtle' }
  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-6">
      {entries.map((e) => {
        const tone = e.dnd && e.state === 'idle' ? 'warn' : stateTone(e.state)
        const inner = (
          <>
            <span className={`size-2.5 flex-none rounded-full ${glow[tone]}`} aria-hidden />
            <span className="min-w-0 flex-1 text-left">
              <span className="block font-mono text-sm text-ink">{e.number}</span>
              <span className="block truncate text-xs text-muted">{e.name}</span>
            </span>
            <span className="text-[11px] text-subtle">{e.dnd ? t('states.dnd') : t(`states.${e.state}`, { defaultValue: e.state })}</span>
          </>
        )
        return onCall && e.number !== self ? (
          <button key={e.number} type="button" onClick={() => onCall(e.number)} title={t('portal.callNumber', { number: e.number })}
            className="flex min-h-14 items-center gap-3 rounded-xl border border-line bg-surface-2/60 px-3 transition hover:border-accent/50 hover:bg-accent/[0.06]">
            {inner}
          </button>
        ) : (
          <div key={e.number} className={`flex min-h-14 items-center gap-3 rounded-xl border px-3 ${e.number === self ? 'border-accent/40 bg-accent/[0.05]' : 'border-line bg-surface-2/60'}`}>{inner}</div>
        )
      })}
    </div>
  )
}
