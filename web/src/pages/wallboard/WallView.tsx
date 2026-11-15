import { useEffect, useState, type ReactNode } from 'react'
import type { TFunction } from 'i18next'
import { useTranslation } from 'react-i18next'
import { Ear, PhoneForwarded as PhoneCallback, Speech, Users } from 'lucide-react'
import { formatDuration } from '../../components/kit'
import { Mark, PoweredBy } from '../../components/brand'
import type { SLAAlert, WallAgent, WallData, WallHour, WallLayout, WallQueue, WallTotals } from '../../lib/types'

export type SpyMode = 'listen' | 'whisper' | 'barge'

/** Sums the figures of several queues (percentages are worked out again from the counts). */
export function sumTotals(list: WallTotals[]): WallTotals {
  const t = list.reduce<WallTotals>((a, b) => ({
    offered: a.offered + b.offered, answered: a.answered + b.answered, abandoned: a.abandoned + b.abandoned,
    shortAbandoned: a.shortAbandoned + b.shortAbandoned, left: a.left + b.left, callbacks: a.callbacks + b.callbacks,
    inTarget: a.inTarget + b.inTarget, avgWait: a.avgWait + b.avgWait * b.answered, avgTalk: a.avgTalk + b.avgTalk * b.answered,
    longestWait: Math.max(a.longestWait, b.longestWait), serviceLevel: -1, abandonedPct: -1,
  }), { offered: 0, answered: 0, abandoned: 0, shortAbandoned: 0, left: 0, callbacks: 0, inTarget: 0, avgWait: 0, avgTalk: 0, longestWait: 0, serviceLevel: -1, abandonedPct: -1 })
  if (t.answered) { t.avgWait = Math.round(t.avgWait / t.answered); t.avgTalk = Math.round(t.avgTalk / t.answered) }
  const counted = t.abandoned - t.shortAbandoned
  if (t.answered + counted > 0) t.serviceLevel = (t.inTarget * 100) / (t.answered + counted)
  if (t.offered - t.shortAbandoned > 0) t.abandonedPct = (counted * 100) / (t.offered - t.shortAbandoned)
  return t
}

const norm = (q: WallQueue) => q.sla.norm || 80

/** Green at or above the norm, orange up to 10 points under it, red below that. */
function levelTone(sl: number, n: number) {
  if (sl < 0) return 'muted'
  return sl >= n ? 'ok' : sl >= n - 10 ? 'warn' : 'danger'
}

const tone: Record<string, string> = { ok: 'text-ok', warn: 'text-warn', danger: 'text-danger', accent: 'text-accent', muted: 'text-muted', ink: 'text-ink' }

/** Seconds since a unix time, kept ticking by the clock. */
function since(unix: number, now: number) { return Math.max(0, Math.floor(now / 1000 - unix)) }

/** The rotating wallboard: overview, one queue large, or the agents. */
export function WallView({ data, layouts, rotate, tv, onSpy, self }: {
  data: WallData; layouts: WallLayout[]; rotate: number; tv?: boolean; onSpy?: (agent: string, mode: SpyMode) => void; self?: string
}) {
  const { t, i18n } = useTranslation()
  const [now, setNow] = useState(() => Date.now())
  const [step, setStep] = useState(0)
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [])
  // Each rotation step shows the next layout; "one queue large" steps through the queues.
  const slots: { layout: WallLayout; queue?: WallQueue }[] = []
  for (const l of layouts) {
    if (l === 'queue') data.queues.forEach((q) => slots.push({ layout: l, queue: q }))
    else slots.push({ layout: l })
  }
  if (!slots.length) slots.push({ layout: 'overview' })
  useEffect(() => {
    if (!rotate || slots.length < 2) return
    const id = window.setInterval(() => setStep((s) => s + 1), rotate * 1000)
    return () => window.clearInterval(id)
  }, [rotate, slots.length])
  const slot = slots[step % slots.length]
  const breached = new Set(data.alerts.map((a) => a.queueId))
  const date = new Date(now)
  const layoutName = slot.layout === 'queue' ? slot.queue?.name : t(`wallboard.layout.${slot.layout}`)

  return (
    <div className="wall flex h-full min-h-0 flex-col gap-[1.2em] text-ink">
      <header className="flex flex-wrap items-center justify-between gap-[1em]">
        <div className="flex items-center gap-[0.8em]">
          <Mark size={tv ? 44 : 36} />
          <div>
            <div className="font-display text-[1.6em] font-semibold leading-tight">{data.screen?.name || t('wallboard.title')}</div>
            <div className="text-[0.85em] uppercase tracking-[0.12em] text-subtle">{layoutName}</div>
          </div>
        </div>
        {slots.length > 1 && (
          <div className="flex items-center gap-[0.4em]" aria-hidden>
            {slots.map((_, i) => <i key={i} className={`block h-[0.3em] w-[1.6em] rounded-full ${i === step % slots.length ? 'bg-accent' : 'bg-tint/15'}`} />)}
          </div>
        )}
        <div className="text-right">
          <div className="font-mono text-[3em] leading-none tabular-nums">{date.toLocaleTimeString(i18n.language, { hour: '2-digit', minute: '2-digit' })}</div>
          <div className="mt-[0.3em] text-[0.9em] text-subtle">{date.toLocaleDateString(i18n.language, { weekday: 'long', day: 'numeric', month: 'long' })}</div>
        </div>
      </header>
      {!data.live && <div className="rounded-xl border border-warn/40 bg-warn/10 px-[1em] py-[0.6em] text-[1.05em] text-warn">{t('wallboard.offline')}</div>}
      {[...new Set(data.alerts.map((a) => a.queueId))].map((id) => <AlertBanner key={id} list={data.alerts.filter((a) => a.queueId === id)} />)}
      <div className="min-h-0 flex-1">
        {slot.layout === 'overview' && <Overview data={data} breached={breached} now={now} onSpy={onSpy} self={self} />}
        {slot.layout === 'queue' && slot.queue && <QueueLarge q={data.queues.find((x) => x.id === slot.queue!.id) ?? slot.queue} data={data} breached={breached.has(slot.queue.id)} now={now} onSpy={onSpy} self={self} />}
        {slot.layout === 'agents' && <AgentGrid data={data} now={now} onSpy={onSpy} self={self} />}
      </div>
      {tv && <div className="flex justify-end"><PoweredBy className="inline-flex" /></div>}
    </div>
  )
}

function alertValue(kind: string, v: number) {
  if (kind === 'wait') return formatDuration(Math.round(v))
  if (kind === 'level' || kind === 'abandoned') return `${Math.round(v)}%`
  return String(Math.round(v))
}

function AlertBanner({ list }: { list: SLAAlert[] }) {
  const { t, i18n } = useTranslation()
  const first = list.reduce((a, b) => (a.since < b.since ? a : b))
  return (
    <div role="alert" className="flex flex-wrap items-center gap-[0.9em] rounded-xl border border-danger/55 bg-danger/[0.12] px-[1.1em] py-[0.6em] text-[1.1em]">
      <span className="size-[0.8em] shrink-0 animate-pulse rounded-full bg-danger shadow-[0_0_14px_var(--color-danger)]" aria-hidden />
      <span className="min-w-0 flex-1"><b className="text-danger">SLA</b> {first.queue} · {list.map((a) => t(`wallboard.alert.${a.kind}`, { value: alertValue(a.kind, a.value), limit: alertValue(a.kind, a.limit) })).join(' · ')}</span>
      <span className="font-mono text-[0.85em] text-muted">{t('wallboard.since', { time: new Date(first.since).toLocaleTimeString(i18n.language, { hour: '2-digit', minute: '2-digit' }) })}</span>
    </div>
  )
}

function Big({ value, label, t: tn = 'ink' }: { value: ReactNode; label: string; t?: string }) {
  return (
    <div className="min-w-0">
      <div className={`font-mono text-[3.6em] font-medium leading-none tabular-nums ${tone[tn]}`}>{value}</div>
      <div className="mt-[0.4em] text-[0.8em] uppercase tracking-[0.1em] text-subtle">{label}</div>
    </div>
  )
}

/** The service level as a ring: the share answered within the target time. */
function Gauge({ q, size = 7 }: { q: WallQueue; size?: number }) {
  const { t } = useTranslation()
  const sl = q.today.serviceLevel
  const tn = levelTone(sl, norm(q))
  const r = 42
  const c = 2 * Math.PI * r
  const pct = sl < 0 ? 0 : Math.min(100, sl)
  return (
    <div className="flex flex-col items-center gap-[0.3em]" title={t('wallboard.levelHint', { target: q.sla.target, norm: norm(q) })}>
      <svg viewBox="0 0 100 100" style={{ width: `${size}em`, height: `${size}em` }} role="img" aria-label={`${t('wallboard.serviceLevel')}: ${sl < 0 ? '–' : Math.round(sl) + '%'}`}>
        <circle cx="50" cy="50" r={r} fill="none" strokeWidth="9" className="stroke-tint/10" />
        <circle cx="50" cy="50" r={r} fill="none" strokeWidth="9" strokeLinecap="round" strokeDasharray={`${(pct / 100) * c} ${c}`} transform="rotate(-90 50 50)"
          className={tn === 'ok' ? 'stroke-ok' : tn === 'warn' ? 'stroke-warn' : tn === 'danger' ? 'stroke-danger' : 'stroke-tint/20'} />
        <text x="50" y="56" textAnchor="middle" className={`fill-current font-mono ${tone[tn]}`} style={{ fontSize: 22 }}>{sl < 0 ? '–' : `${Math.round(sl)}%`}</text>
      </svg>
      <div className="text-center text-[0.8em] uppercase tracking-[0.08em] text-subtle">{t('wallboard.within', { s: q.sla.target })}</div>
    </div>
  )
}

function Callers({ q, max }: { q: WallQueue; max: number }) {
  const { t } = useTranslation()
  if (!q.callers.length) return <div className="py-[0.6em] text-[1em] text-subtle">{t('wallboard.nobodyWaiting')}</div>
  return (
    <ol className="flex flex-col gap-[0.3em]">
      {q.callers.slice(0, max).map((c) => (
        <li key={c.position} className={`flex items-center gap-[0.7em] rounded-lg px-[0.7em] py-[0.35em] text-[1.05em] ${c.callback ? 'bg-accent-2/10' : 'bg-tint/[0.04]'}`}>
          <span className="font-mono text-subtle">#{c.position}</span>
          <span className="min-w-0 flex-1 truncate font-mono">{c.number || t('reception.unknownCaller')}{c.name && <span className="font-sans text-muted"> · {c.name}</span>}</span>
          {c.callback && <span className="inline-flex items-center gap-[0.3em] text-[0.8em] text-accent-2"><PhoneCallback className="size-[1em]" aria-hidden />{t('wallboard.callback')}</span>}
          <span className={`font-mono tabular-nums ${c.callback ? 'text-accent-2' : q.sla.maxWait && c.wait > q.sla.maxWait ? 'text-danger' : 'text-warn'}`}>{formatDuration(c.wait)}</span>
        </li>
      ))}
      {q.callers.length > max && <li className="px-[0.7em] text-[0.9em] text-subtle">{t('wallboard.more', { count: q.callers.length - max })}</li>}
    </ol>
  )
}

function Footer({ t: tt, extra }: { t: WallTotals; extra?: boolean }) {
  const { t } = useTranslation()
  const cells: [ReactNode, string, string?][] = [
    [tt.answered, t('wallboard.answered')],
    [tt.abandoned - tt.shortAbandoned, t('wallboard.abandoned'), tt.abandoned - tt.shortAbandoned > 0 ? 'danger' : undefined],
    [formatDuration(tt.avgWait), t('wallboard.avgWait')],
    [formatDuration(tt.avgTalk), t('wallboard.avgTalk')],
  ]
  if (extra) cells.push([tt.abandonedPct < 0 ? '–' : `${Math.round(tt.abandonedPct)}%`, t('wallboard.abandonedPct')])
  return (
    <div className="grid gap-[0.6em] border-t border-line pt-[0.8em]" style={{ gridTemplateColumns: `repeat(${cells.length}, minmax(0, 1fr))` }}>
      {cells.map(([v, l, tn], i) => (
        <div key={i} className="min-w-0">
          <div className={`font-mono text-[1.6em] tabular-nums ${tone[tn ?? 'ink']}`}>{v}</div>
          <div className="truncate text-[0.75em] uppercase tracking-[0.08em] text-subtle">{l}</div>
        </div>
      ))}
    </div>
  )
}

function QueueTile({ q, breached }: { q: WallQueue; breached: boolean }) {
  const { t } = useTranslation()
  const waitTone = q.waiting ? (breached ? 'danger' : 'warn') : 'ok'
  return (
    <section className={`flex min-h-0 flex-col gap-[1em] rounded-2xl border bg-surface/90 p-[1.3em] ${breached ? 'border-danger/60 shadow-[inset_0_0_0_1px_rgb(248_113_113_/_0.25)]' : 'border-line'}`}>
      <div className="flex items-baseline justify-between gap-[0.8em]">
        <h2 className="truncate font-display text-[1.7em] font-semibold">{q.name}</h2>
        <span className="font-mono text-[0.95em] text-subtle">{q.number}</span>
      </div>
      <div className="grid grid-cols-[1fr_1fr_auto] items-center gap-[0.8em]">
        <Big value={q.waiting} label={t('wallboard.waiting')} t={waitTone} />
        <Big value={formatDuration(q.longestWait)} label={t('wallboard.longestWait')} t={q.sla.maxWait && q.longestWait > q.sla.maxWait ? 'danger' : 'ink'} />
        <Gauge q={q} size={6.5} />
      </div>
      <div className="min-h-0 flex-1 overflow-hidden"><Callers q={q} max={3} /></div>
      <Footer t={q.today} />
    </section>
  )
}

function Overview({ data, breached, now, onSpy, self }: { data: WallData; breached: Set<number>; now: number; onSpy?: (a: string, m: SpyMode) => void; self?: string }) {
  const { t } = useTranslation()
  const names = new Map(data.queues.map((q) => [q.id, q.name]))
  const hours = data.queues.length ? data.queues[0].hours.map((_, h) => ({
    answered: data.queues.reduce((a, q) => a + (q.hours[h]?.answered ?? 0), 0),
    abandoned: data.queues.reduce((a, q) => a + (q.hours[h]?.abandoned ?? 0), 0),
  })) : []
  if (!data.queues.length) return <div className="grid h-full place-items-center text-[1.3em] text-muted">{t('wallboard.emptyText')}</div>
  return (
    <div className="grid h-full min-h-0 gap-[1.1em] lg:grid-cols-[minmax(0,1fr)_minmax(16em,22em)]">
      <div className="flex min-h-0 flex-col gap-[1.1em]">
        <div className="grid min-h-0 flex-1 gap-[1.1em]" style={{ gridTemplateColumns: `repeat(auto-fit, minmax(min(100%, 24em), 1fr))` }}>
          {data.queues.map((q) => <QueueTile key={q.id} q={q} breached={breached.has(q.id)} />)}
        </div>
        <HourChart hours={hours} now={new Date(now).getHours()} />
      </div>
      <AgentPanel agents={data.agents} names={names} now={now} onSpy={onSpy} self={self} queues={data.queues} />
    </div>
  )
}

function QueueLarge({ q, data, breached, now, onSpy, self }: { q: WallQueue; data: WallData; breached: boolean; now: number; onSpy?: (a: string, m: SpyMode) => void; self?: string }) {
  const { t } = useTranslation()
  const agents = data.agents.filter((a) => a.queues.includes(q.id))
  const names = new Map(data.queues.map((x) => [x.id, x.name]))
  return (
    <div className="grid h-full min-h-0 gap-[1.1em] lg:grid-cols-[minmax(0,1fr)_minmax(16em,24em)]">
      <section className={`flex min-h-0 flex-col gap-[1.3em] rounded-2xl border bg-surface/90 p-[1.6em] ${breached ? 'border-danger/60' : 'border-line'}`}>
        <div className="flex items-baseline justify-between">
          <h2 className="font-display text-[2.4em] font-semibold">{q.name}</h2>
          <span className="font-mono text-[1.2em] text-subtle">{q.number}</span>
        </div>
        <div className="grid grid-cols-2 items-center gap-[1.2em] sm:grid-cols-4">
          <Big value={q.waiting} label={t('wallboard.waiting')} t={q.waiting ? (breached ? 'danger' : 'warn') : 'ok'} />
          <Big value={formatDuration(q.longestWait)} label={t('wallboard.longestWait')} t={q.sla.maxWait && q.longestWait > q.sla.maxWait ? 'danger' : 'ink'} />
          <Big value={q.today.serviceLevel < 0 ? '–' : `${Math.round(q.today.serviceLevel)}%`} label={t('wallboard.levelNorm', { norm: norm(q) })} t={levelTone(q.today.serviceLevel, norm(q))} />
          <Big value={q.callbacks} label={t('wallboard.callbacks')} t={q.callbacks ? 'accent' : 'ink'} />
        </div>
        <div className="min-h-0 flex-1 overflow-hidden text-[1.15em]"><Callers q={q} max={8} /></div>
        <Footer t={q.today} extra />
      </section>
      <AgentPanel agents={agents} names={names} now={now} onSpy={onSpy} self={self} queues={data.queues} title={q.name} />
    </div>
  )
}

const stateDot: Record<string, string> = {
  free: 'bg-ok', ringing: 'bg-accent animate-pulse', call: 'bg-danger', internal: 'bg-accent-2', paused: 'bg-warn', offline: 'bg-tint/25',
}

function agentDetail(a: WallAgent, names: Map<number, string>, t: TFunction) {
  switch (a.state) {
    case 'call': return [a.queue ? names.get(a.queue) : '', a.caller].filter(Boolean).join(' · ') || t('wallboard.state.call')
    case 'free': return t('wallboard.callsToday', { count: a.callsToday })
    default: return t(`wallboard.state.${a.state}`)
  }
}

function SpyButtons({ a, queues, onSpy, self }: { a: WallAgent; queues: WallQueue[]; onSpy?: (a: string, m: SpyMode) => void; self?: string }) {
  const { t } = useTranslation()
  if (!onSpy || a.state !== 'call' || a.number === self) return null
  if (!queues.some((q) => q.supervision && a.queues.includes(q.id))) return null
  const btn = 'inline-flex size-9 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-ink'
  return (
    <span className="flex">
      <button type="button" className={btn} title={t('reception.spyListen', { name: a.name })} aria-label={t('reception.spyListen', { name: a.name })} onClick={() => onSpy(a.number, 'listen')}><Ear className="size-4" aria-hidden /></button>
      <button type="button" className={btn} title={t('reception.spyWhisper', { name: a.name })} aria-label={t('reception.spyWhisper', { name: a.name })} onClick={() => onSpy(a.number, 'whisper')}><Speech className="size-4" aria-hidden /></button>
      <button type="button" className={btn} title={t('reception.spyBarge', { name: a.name })} aria-label={t('reception.spyBarge', { name: a.name })} onClick={() => onSpy(a.number, 'barge')}><Users className="size-4" aria-hidden /></button>
    </span>
  )
}

function AgentPanel({ agents, names, now, onSpy, self, queues, title }: {
  agents: WallAgent[]; names: Map<number, string>; now: number; onSpy?: (a: string, m: SpyMode) => void; self?: string; queues: WallQueue[]; title?: string
}) {
  const { t } = useTranslation()
  const n = (s: string) => agents.filter((a) => a.state === s).length
  return (
    <section className="flex min-h-0 flex-col gap-[0.8em] rounded-2xl border border-line bg-surface/90 p-[1.2em]">
      <div>
        <h2 className="font-display text-[1.3em] font-semibold">{title ? t('wallboard.agentsOf', { name: title }) : t('wallboard.agents')}</h2>
        <div className="text-[0.85em] text-muted">{t('wallboard.agentCounts', { free: n('free'), busy: n('call') + n('ringing') + n('internal'), paused: n('paused') })}</div>
      </div>
      {!agents.length && <div className="text-[1em] text-subtle">{t('wallboard.noAgents')}</div>}
      <ul className="flex min-h-0 flex-1 flex-col gap-[0.2em] overflow-hidden">
        {agents.map((a) => (
          <li key={a.number} className="flex items-center gap-[0.6em] border-b border-tint/[0.06] py-[0.35em] last:border-0">
            <span className={`size-[0.65em] shrink-0 rounded-full ${stateDot[a.state]}`} aria-hidden />
            <div className="min-w-0 flex-1">
              <div className="truncate text-[1.05em]">{a.name || a.number}</div>
              <div className="truncate text-[0.8em] text-subtle">{agentDetail(a, names, t)}</div>
            </div>
            <SpyButtons a={a} queues={queues} onSpy={onSpy} self={self} />
            <span className={`font-mono text-[1em] tabular-nums ${a.state === 'paused' ? 'text-warn' : 'text-muted'}`}>{a.state === 'offline' ? '' : formatDuration(since(a.since, now))}</span>
          </li>
        ))}
      </ul>
    </section>
  )
}

function AgentGrid({ data, now, onSpy, self }: { data: WallData; now: number; onSpy?: (a: string, m: SpyMode) => void; self?: string }) {
  const { t } = useTranslation()
  const names = new Map(data.queues.map((q) => [q.id, q.name]))
  if (!data.agents.length) return <div className="grid h-full place-items-center text-[1.3em] text-muted">{t('wallboard.noAgents')}</div>
  const border: Record<string, string> = { free: 'border-ok/40', ringing: 'border-accent/50', call: 'border-danger/45', internal: 'border-accent-2/45', paused: 'border-warn/45', offline: 'border-line opacity-60' }
  return (
    <div className="grid content-start gap-[0.9em]" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(min(100%, 17em), 1fr))' }}>
      {data.agents.map((a) => (
        <div key={a.number} className={`flex flex-col gap-[0.4em] rounded-2xl border bg-surface/90 p-[1em] ${border[a.state]}`}>
          <div className="flex items-center gap-[0.6em]">
            <span className={`size-[0.75em] shrink-0 rounded-full ${stateDot[a.state]}`} aria-hidden />
            <span className="min-w-0 flex-1 truncate font-display text-[1.3em] font-semibold">{a.name || a.number}</span>
            <span className="font-mono text-[0.9em] text-subtle">{a.number}</span>
          </div>
          <div className="flex items-end justify-between gap-[0.6em]">
            <div className="min-w-0">
              <div className="text-[0.95em]">{t(`wallboard.state.${a.state}`)}</div>
              <div className="truncate text-[0.8em] text-subtle">{a.state === 'call' ? agentDetail(a, names, t) : t('wallboard.callsToday', { count: a.callsToday })}</div>
            </div>
            <SpyButtons a={a} queues={data.queues} onSpy={onSpy} self={self} />
            <span className="font-mono text-[1.8em] tabular-nums">{a.state === 'offline' ? '' : formatDuration(since(a.since, now))}</span>
          </div>
        </div>
      ))}
    </div>
  )
}

/** Answered and abandoned calls per hour of today: bars from the baseline, two series with a legend. */
function HourChart({ hours, now }: { hours: WallHour[]; now: number }) {
  const { t } = useTranslation()
  let from = 8
  let to = Math.max(18, now + 1)
  hours.forEach((h, i) => { if (h.answered + h.abandoned > 0) { from = Math.min(from, i); to = Math.max(to, i + 1) } })
  to = Math.min(24, to)
  const list = hours.slice(from, to)
  const max = Math.max(4, ...list.map((h) => Math.max(h.answered, h.abandoned)))
  return (
    <section className="rounded-2xl border border-line bg-surface/90 px-[1.2em] py-[0.9em]">
      <div className="mb-[0.5em] flex flex-wrap items-center gap-[1.2em] text-[0.85em] text-muted">
        <span className="font-medium text-ink-2">{t('wallboard.perHour')}</span>
        <span className="inline-flex items-center gap-[0.4em]"><i className="block size-[0.7em] rounded-sm bg-accent" aria-hidden />{t('wallboard.answered')}</span>
        <span className="inline-flex items-center gap-[0.4em]"><i className="block size-[0.7em] rounded-sm bg-danger" aria-hidden />{t('wallboard.abandoned')}</span>
      </div>
      <div className="flex h-[6em] items-end gap-[0.3em] border-b border-tint/15" role="img" aria-label={t('wallboard.perHour')}>
        {list.map((h, i) => {
          const hour = String(from + i).padStart(2, '0')
          return (
            <div key={i} className="group relative flex h-full flex-1 items-end justify-center gap-[2px]" title={`${hour}:00 · ${t('wallboard.answered')} ${h.answered} · ${t('wallboard.abandoned')} ${h.abandoned}`}>
              <span className={`block w-[32%] rounded-t-[4px] bg-accent ${from + i === now ? '' : 'opacity-80'}`} style={{ height: `${(h.answered / max) * 100}%` }} />
              <span className="block w-[32%] rounded-t-[4px] bg-danger opacity-85" style={{ height: `${(h.abandoned / max) * 100}%` }} />
            </div>
          )
        })}
      </div>
      <div className="mt-[0.3em] flex gap-[0.3em]">
        {list.map((_, i) => <span key={i} className={`flex-1 text-center font-mono text-[0.75em] ${from + i === now ? 'text-ink' : 'text-subtle'}`}>{String(from + i).padStart(2, '0')}</span>)}
      </div>
    </section>
  )
}
