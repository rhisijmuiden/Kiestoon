import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Ear, Headset, Pause, Play, LogIn, LogOut, PhoneCall, RotateCcw, Speech, Users, X } from 'lucide-react'
import { Badge, Empty, SectionTitle, Select, Table, formatDuration, td, useApi } from '../components/kit'
import { IconButton, stateTone } from '../components/pbx'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { BlfEntry, CallbackStatus, Live, QueueCallback, QueueView, TimeCondition } from '../lib/types'
import { ActiveCalls, SwitchboardCalls, TransferNotice, TransferTargets } from '../components/Transfer'
import { useSession } from '../lib/session'
import { TransferProvider } from '../lib/transfer'
import { CrmIncoming } from '../components/CrmIncoming'
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
  const [spyError, setSpyError] = useState<unknown>(null)
  async function agent(q: number, ext: number, action: string) {
    await api('POST', `/reception/queues/${q}/agents/${ext}`, { action })
    queues.reload()
  }
  async function spy(agentNumber: string, mode: SpyMode) {
    setSpyError(null)
    try {
      await api('POST', '/reception/supervise', { agent: agentNumber, mode })
    } catch (e) {
      setSpyError(e)
    }
  }
  const myNumber = state.kind === 'ready' ? state.me.extension?.number : undefined
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
        <CrmIncoming />
        {hasExt && <ActiveCalls />}
        <SwitchboardCalls calls={live.data?.calls ?? []} />
        <TransferTargets path="/reception/targets" />
      </TransferProvider>
      <ErrorMessage error={spyError} />
      <QueueBoard queues={queues.data ?? []} onAgent={agent} onSpy={hasExt ? spy : undefined} self={myNumber} />
      <Callbacks queues={queues.data ?? []} />
    </>
  )
}

export type SpyMode = 'listen' | 'whisper' | 'barge'

/** An SLA value as shown in an alert: wait times as m:ss, percentages with %. */
export function slaValue(kind: string, v: number) {
  if (kind === 'wait') return formatDuration(Math.round(v))
  if (kind === 'level' || kind === 'abandoned') return `${Math.round(v)}%`
  return String(Math.round(v))
}

/**
 * Live queues with their callers and agents. onAgent is set when the viewer may steer agents,
 * onSpy when they may listen, whisper or barge in (queues with supervision on; self is skipped).
 */
export function QueueBoard({ queues, onAgent, myExtensionId, onSpy, self }: {
  queues: QueueView[]; onAgent?: (q: number, ext: number, action: string) => void; myExtensionId?: number
  onSpy?: (agent: string, mode: SpyMode) => void; self?: string
}) {
  const { t } = useTranslation()
  if (!queues.length) return <Card><Empty icon={<Headset className="size-7" />} title={t('reception.noQueues')} /></Card>
  return (
    <div className="grid gap-6 xl:grid-cols-2">
      {queues.map((q) => {
        const live = q.live
        const agents = new Map((live?.agents ?? []).map((a) => [a.number, a]))
        return (
          <Card key={q.id} className={q.alerts?.length ? 'shadow-[inset_0_0_0_1px_rgb(248_113_113_/_0.55)]' : ''}>
            {q.alerts?.map((a) => (
              <div key={a.kind} role="alert" className="mb-3 flex items-center gap-2 rounded-lg border border-danger/45 bg-danger/10 px-3 py-2 text-sm text-danger-ink">
                <span className="size-2 shrink-0 animate-pulse rounded-full bg-danger" aria-hidden />
                <b>SLA</b> {t(`wallboard.alert.${a.kind}`, { value: slaValue(a.kind, a.value), limit: slaValue(a.kind, a.limit) })}
              </div>
            ))}
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
                    {onSpy && q.options?.supervision && a?.inCall && m.number !== self && (
                      <span className="flex border-l border-line pl-1">
                        <IconButton label={t('reception.spyListen', { name: m.name })} onClick={() => onSpy(m.number, 'listen')}><Ear className="size-4" aria-hidden /></IconButton>
                        <IconButton label={t('reception.spyWhisper', { name: m.name })} onClick={() => onSpy(m.number, 'whisper')}><Speech className="size-4" aria-hidden /></IconButton>
                        <IconButton label={t('reception.spyBarge', { name: m.name })} onClick={() => onSpy(m.number, 'barge')}><Users className="size-4" aria-hidden /></IconButton>
                      </span>
                    )}
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

const cbTone: Record<CallbackStatus, 'ok' | 'warn' | 'danger' | 'accent' | 'muted'> = {
  scheduled: 'warn', waiting: 'warn', calling: 'accent', done: 'ok', failed: 'danger', cancelled: 'muted',
}

/** The queue callback list: callers who asked to be called back, and adding one by hand. */
function Callbacks({ queues }: { queues: QueueView[] }) {
  const { t, i18n } = useTranslation()
  const list = useApi<QueueCallback[]>('/reception/callbacks', 5000)
  const withCb = queues.filter((q) => q.options?.callback?.enabled)
  const [queueId, setQueueId] = useState(0)
  const [number, setNumber] = useState('')
  const [name, setName] = useState('')
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState(false)
  if (!withCb.length && !list.data?.length) return null
  const qName = (id: number) => queues.find((q) => q.id === id)?.name ?? `#${id}`
  const time = (iso: string) => new Date(iso).toLocaleTimeString(i18n.language, { hour: '2-digit', minute: '2-digit' })
  async function act(c: QueueCallback, action: 'cancel' | 'retry') {
    setError(null)
    try { await api('POST', `/reception/callbacks/${c.id}/${action}`, {}) } catch (e) { setError(e) }
    list.reload()
  }
  async function add() {
    setError(null)
    setBusy(true)
    try {
      await api('POST', '/reception/callbacks', { queueId: queueId || withCb[0]?.id, number, name })
      setNumber('')
      setName('')
      list.reload()
    } catch (e) { setError(e) } finally { setBusy(false) }
  }
  return (
    <Card>
      <SectionTitle>{t('reception.callbacks')}</SectionTitle>
      <p className="-mt-2 mb-4 text-sm text-muted">{t('reception.callbacksHint')}</p>
      <ErrorMessage error={error || list.error} />
      <Table head={[t('reception.cbCaller'), t('reception.cbQueue'), t('reception.cbStatus'), t('reception.cbAttempts'), t('reception.cbAsked'), '']}
        empty={list.data?.length === 0 && <Empty icon={<PhoneCall className="size-7" />} title={t('reception.callbacksEmpty')} />}>
        {list.data?.map((c) => (
          <tr key={c.id}>
            <td className={td}><span className="font-mono text-ink">{c.number}</span> <span className="text-xs text-subtle">{c.name}</span></td>
            <td className={td}>{qName(c.queueId)}</td>
            <td className={td}>
              <Badge tone={cbTone[c.status]} dot>{t(`reception.cb.${c.status}`)}</Badge>
              {c.status === 'calling' && c.agent && <span className="ml-2 text-xs text-subtle">{c.agent}</span>}
              {c.note && c.status !== 'calling' && c.status !== 'done' && <span className="ml-2 text-xs text-subtle">{t(`reception.cbNote.${c.note}`, { defaultValue: c.note })}</span>}
            </td>
            <td className={`${td} font-mono`}>{c.attempts}</td>
            <td className={`${td} font-mono text-xs`}>{time(c.createdAt)}</td>
            <td className={`${td} whitespace-nowrap text-right`}>
              {(c.status === 'scheduled' || c.status === 'waiting') && <IconButton danger label={t('reception.cbCancel')} onClick={() => act(c, 'cancel')}><X className="size-4" aria-hidden /></IconButton>}
              {(c.status === 'failed' || c.status === 'cancelled') && <IconButton label={t('reception.cbRetry')} onClick={() => act(c, 'retry')}><RotateCcw className="size-4" aria-hidden /></IconButton>}
            </td>
          </tr>
        ))}
      </Table>
      {withCb.length > 0 && (
        <div className="mt-4 grid items-end gap-3 sm:grid-cols-[1fr_1fr_1fr_auto]">
          <Select label={t('reception.cbQueue')} value={queueId || withCb[0].id} onChange={(e) => setQueueId(Number(e.target.value))}>
            {withCb.map((q) => <option key={q.id} value={q.id}>{q.name}</option>)}
          </Select>
          <Input label={t('reception.cbNumber')} value={number} onChange={(e) => setNumber(e.target.value.replace(/[^0-9+]/g, ''))} inputMode="tel" className="font-mono" />
          <Input label={t('reception.cbName')} value={name} onChange={(e) => setName(e.target.value)} />
          <Button busy={busy} disabled={number.length < 6} onClick={add}><PhoneCall className="size-4" aria-hidden />{t('reception.cbAdd')}</Button>
        </div>
      )}
    </Card>
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
