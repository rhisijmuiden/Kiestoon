import { useEffect, useMemo, useRef, useState, type CSSProperties, type ReactNode, type Ref } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { BellRing, CheckCircle2, ImageIcon, PhoneForwarded } from 'lucide-react'
import { formatDuration } from '../../components/kit'
import { Mark } from '../../components/brand'
import rhisLogo from '../../assets/rhis-logo-wit.svg'
import type { WallAgent, WallData, WallDesign, WallTile } from '../../lib/types'
import { AlertBanner } from './WallView'
import {
  formatMetric, gridSize, hourly, metricInfo, metricTone, metricValue, periodTotals, thresholds, tileAgents, tileAlarmed, tileQueues, themeNow, type Tone,
} from './design'

const toneText: Record<Tone, string> = { ok: 'text-ok', warn: 'text-warn', danger: 'text-danger', muted: 'text-muted', ink: 'text-ink' }
const toneStroke: Record<Tone, string> = { ok: 'stroke-ok', warn: 'stroke-warn', danger: 'stroke-danger', muted: 'stroke-tint/20', ink: 'stroke-accent' }
const stateDot: Record<string, string> = {
  free: 'bg-ok', ringing: 'bg-accent animate-pulse', call: 'bg-danger', internal: 'bg-accent-2', paused: 'bg-warn', offline: 'bg-tint/25',
}

/** The default title of a tile: the figure it shows, or the name of the tile. */
export function tileTitle(tile: WallTile, t: TFunction): string {
  if (tile.title) return tile.title
  if ((tile.type === 'kpi' || tile.type === 'gauge' || tile.type === 'trend' || tile.type === 'compare') && tile.metric) {
    const m = t(`wallDesign.metric.${tile.metric}`)
    return metricInfo[tile.metric].live || tile.period === 'today' ? m : `${m} · ${t(`wallDesign.period.${tile.period}`)}`
  }
  return t(`wallDesign.tile.${tile.type}`)
}

/** The frame of a screen: sets the theme, accent and text size; the board inside scales with it. */
export function BoardFrame({ design, now, className = '', style, children }: {
  design: WallDesign; now: Date; className?: string; style?: CSSProperties; children: ReactNode
}) {
  const s = design.style
  const theme = themeNow(s.theme, now)
  const base = design.format === 'portrait' ? 1.67 : 0.94
  const bg = s.background === 'grid'
    ? 'bg-bg bg-[radial-gradient(var(--color-grid-dot)_1px,transparent_1.2px)] bg-[size:2.2em_2.2em]'
    : s.background === 'glow' ? 'bg-bg bg-[radial-gradient(70%_55%_at_75%_-10%,var(--page-glow),transparent_70%)]' : 'bg-bg'
  return (
    <div className={`kt-cq relative overflow-hidden ${className}`} style={style}>
      <div className={`kt-wall-${theme} ${s.accent ? 'kt-wall-accent' : ''} ${bg} absolute inset-0 font-sans text-ink`}
        style={{ fontSize: `calc(${(s.scale / 100) * base}cqw)`, ...(s.accent ? { '--wall-accent': s.accent } : {}) } as CSSProperties}>
        {children}
      </div>
    </div>
  )
}

const gaps = { roomy: '1.3em', normal: '0.9em', compact: '0.55em' }

/** One page of a designed screen: header, tiles on the grid, and "Powered by RHIS". */
export function Board({ data, design, name, page, pages, now, gridRef, wrapTile, alarmed }: {
  data: WallData; design: WallDesign; name: string; page: number; pages: number; now: Date
  gridRef?: Ref<HTMLDivElement>; wrapTile?: (tile: WallTile, content: ReactNode) => ReactNode; alarmed?: boolean
}) {
  const { t, i18n } = useTranslation()
  const s = design.style
  const [cols, rows] = gridSize(design.format)
  const p = design.pages[page % Math.max(1, design.pages.length)]
  const gap = gaps[s.density]
  const header = s.logo !== 'none' || s.name || s.clock || s.date || (s.dots && pages > 1)
  const ids = [...new Set(data.alerts.map((a) => a.queueId))]
  return (
    <div className="flex h-full flex-col" style={{ padding: `calc(${gap} * 1.6)`, gap }}>
      {header && (
        <header className="flex items-center gap-[1em]">
          {s.logo === 'kiestoon' && <span className="text-[1.4em]"><Mark size={44} /></span>}
          {s.logo === 'own' && (s.logoImage ? <img src={s.logoImage} alt="" className="block h-[2.6em] w-auto max-w-[14em] object-contain" /> : <span className="grid h-[2.6em] w-[6em] place-items-center rounded-[0.6em] border border-dashed border-line text-[0.7em] text-subtle"><ImageIcon className="size-[1.4em]" aria-hidden /></span>)}
          {s.name && (
            <div className="min-w-0">
              <div className="truncate font-display text-[1.7em] font-semibold leading-tight">{name}</div>
              {p?.name && <div className="truncate text-[0.8em] uppercase tracking-[0.12em] text-subtle">{p.name}</div>}
            </div>
          )}
          <span className="flex-1" />
          {s.dots && pages > 1 && (
            <div className="flex items-center gap-[0.4em]" aria-hidden>
              {Array.from({ length: pages }, (_, i) => <i key={i} className={`block h-[0.32em] w-[1.7em] rounded-full ${i === page % pages ? 'bg-accent' : 'bg-tint/15'}`} />)}
            </div>
          )}
          <span className="flex-1" />
          {(s.clock || s.date) && (
            <div className="text-right">
              {s.clock && <div className="font-mono text-[3em] leading-none tabular-nums">{now.toLocaleTimeString(i18n.language, { hour: '2-digit', minute: '2-digit' })}</div>}
              {s.date && <div className="mt-[0.25em] text-[0.95em] text-subtle">{now.toLocaleDateString(i18n.language, { weekday: 'long', day: 'numeric', month: 'long' })}</div>}
            </div>
          )}
        </header>
      )}
      {!data.live && <div className="rounded-[0.7em] border border-warn/40 bg-warn/10 px-[1em] py-[0.5em] text-warn">{t('wallboard.offline')}</div>}
      {s.alarmBanner && ids.map((id) => <AlertBanner key={id} list={data.alerts.filter((a) => a.queueId === id)} />)}
      <div ref={gridRef} className="relative grid min-h-0 flex-1" style={{ gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`, gridTemplateRows: `repeat(${rows}, minmax(0, 1fr))`, gap }}>
        {p?.tiles.map((tile) => {
          const content = <TileCard tile={tile} data={data} design={design} now={now} alarmed={alarmed !== false && tileAlarmed(data, tile)} />
          return (
            <div key={tile.id} className="relative min-h-0 min-w-0" style={{ gridColumn: `${tile.x + 1} / span ${tile.w}`, gridRow: `${tile.y + 1} / span ${tile.h}` }}>
              {wrapTile ? wrapTile(tile, content) : content}
            </div>
          )
        })}
      </div>
      <div className="flex justify-end">
        <span className="rhis-badge inline-flex items-center gap-[0.6em] rounded-[0.7em] px-[0.8em] py-[0.45em] text-[0.72em]">
          {t('app.poweredBy')}<img src={rhisLogo} alt="RHIS" className="block h-[1.35em] w-auto" />
        </span>
      </div>
    </div>
  )
}

function cardClass(style: WallDesign['style']['tiles'], alarm: boolean) {
  const base = 'relative flex h-full min-h-0 flex-col overflow-hidden rounded-[1em] p-[0.9em_1.1em] kt-cq'
  const edge = alarm ? 'border-danger/60 shadow-[inset_0_0_0_1px_color-mix(in_srgb,var(--color-danger)_25%,transparent)]' : ''
  switch (style) {
    case 'solid': return `${base} border ${alarm ? edge : 'border-transparent'} bg-surface`
    case 'outlined': return `${base} border-[1.5px] ${alarm ? edge : 'border-accent/35'} bg-transparent`
    default: return `${base} card ${edge}`
  }
}

function Label({ tile, children }: { tile: WallTile; children?: ReactNode }) {
  const { t } = useTranslation()
  if (tile.hideLabel) return null
  return (
    <div className="flex items-center gap-[0.6em]">
      <span className="min-w-0 flex-1 truncate text-[0.78em] uppercase tracking-[0.11em] text-subtle">{tileTitle(tile, t)}</span>
      {children}
    </div>
  )
}

function TileCard({ tile, data, design, now, alarmed }: { tile: WallTile; data: WallData; design: WallDesign; now: Date; alarmed: boolean }) {
  const plain = tile.type === 'ticker' || tile.type === 'image'
  if (plain) return <div className={cardClass(design.style.tiles, false) + (tile.type === 'ticker' ? ' !bg-accent/[0.08] !border-accent/30 justify-center !py-0' : ' !p-[0.5em]')}><TileBody tile={tile} data={data} now={now} /></div>
  return <div className={cardClass(design.style.tiles, alarmed)}><TileBody tile={tile} data={data} now={now} /></div>
}

function TileBody({ tile, data, now }: { tile: WallTile; data: WallData; now: Date }) {
  switch (tile.type) {
    case 'kpi': case 'gauge': case 'trend': return <Figure tile={tile} data={data} />
    case 'queue': return <QueueBody tile={tile} data={data} />
    case 'callers': return <CallersBody tile={tile} data={data} />
    case 'callbacks': return <CallersBody tile={tile} data={data} callbacks />
    case 'compare': return <CompareBody tile={tile} data={data} />
    case 'agents': return <AgentsBody tile={tile} data={data} now={now} />
    case 'agentTiles': return <AgentTilesBody tile={tile} data={data} now={now} />
    case 'leaderboard': return <LeaderBody tile={tile} data={data} />
    case 'hourly': return <HourlyBody tile={tile} data={data} now={now} />
    case 'donut': return <DonutBody tile={tile} data={data} />
    case 'clock': return <ClockBody tile={tile} now={now} />
    case 'text': return <TextBody tile={tile} />
    case 'ticker': return <TickerBody tile={tile} />
    case 'image': return <ImageBody tile={tile} />
    case 'alerts': return <AlertsBody tile={tile} data={data} />
  }
}

/** A figure: a big number, a number with a sparkline, or a ring. */
function Figure({ tile, data }: { tile: WallTile; data: WallData }) {
  const { t } = useTranslation()
  const metric = tile.metric || 'waiting'
  const v = metricValue(data, tile)
  const tone = metricTone(metric, v, thresholds(data, tile))
  const blink = tile.blink && tone === 'danger' ? 'kt-wall-blink' : ''
  const display = tile.type === 'gauge' ? 'gauge' : tile.type === 'trend' ? 'trend' : tile.display
  const showCompare = tile.compare && !metricInfo[metric].live && tile.period === 'today'
  const yest = showCompare ? metricValue(data, tile, metric, 'yesterday') : -1
  if (display === 'gauge') {
    const pct = v < 0 ? 0 : metricInfo[metric].kind === 'pct' ? Math.min(100, v) : Math.min(100, (v / Math.max(1, thresholds(data, tile).alarm || v || 1)) * 100)
    const r = 42
    const c = 2 * Math.PI * r
    return (
      <>
        <Label tile={tile} />
        <div className="flex min-h-0 flex-1 items-center justify-center">
          <svg viewBox="0 0 100 100" className="h-full max-h-[78cqh] w-auto max-w-full" role="img" aria-label={`${tileTitle(tile, t)}: ${formatMetric(metric, v)}`}>
            <circle cx="50" cy="50" r={r} fill="none" strokeWidth="9" className="stroke-tint/10" />
            <circle cx="50" cy="50" r={r} fill="none" strokeWidth="9" strokeLinecap="round" strokeDasharray={`${(pct / 100) * c} ${c}`} transform="rotate(-90 50 50)" className={toneStroke[tone]} />
            <text x="50" y="57" textAnchor="middle" className={`fill-current font-mono ${toneText[tone]} ${blink}`} style={{ fontSize: 21 }}>{formatMetric(metric, v)}</text>
          </svg>
        </div>
        {showCompare && <div className="text-center text-[0.75em] text-subtle">{t('wallDesign.yesterdayWas', { value: formatMetric(metric, yest) })}</div>}
      </>
    )
  }
  const hours = display === 'trend' ? hourly(data, tile) : []
  const series = hours.map((h) => (metric === 'abandoned' || metric === 'abandonedPct' ? h.abandoned : h.answered))
  const nowH = new Date(data.now).getHours()
  const last = Math.max(nowH, 8)
  const pts = series.slice(7, last + 1)
  const max = Math.max(1, ...pts)
  return (
    <>
      <Label tile={tile} />
      <div className="mt-auto flex min-h-0 items-end gap-[0.6em]">
        <span className={`font-mono font-medium leading-none tabular-nums ${toneText[tone]} ${blink}`} style={{ fontSize: display === 'trend' ? 'min(34cqh, 19cqw)' : 'min(48cqh, 24cqw)' }}>{formatMetric(metric, v)}</span>
      </div>
      {display === 'trend' && pts.length > 1 && (
        <svg viewBox={`0 0 ${(pts.length - 1) * 10} 30`} preserveAspectRatio="none" className="mt-[0.4em] h-[22cqh] w-full" aria-hidden>
          <polyline fill="none" vectorEffect="non-scaling-stroke" strokeWidth="2.5" className={metric === 'abandoned' ? 'stroke-danger' : 'stroke-accent'} points={pts.map((y, i) => `${i * 10},${28 - (y / max) * 26}`).join(' ')} />
        </svg>
      )}
      {showCompare && <div className="mt-[0.3em] text-[0.75em] text-subtle">{t('wallDesign.yesterdayWas', { value: formatMetric(metric, yest) })}</div>}
    </>
  )
}

function CallerRows({ list, max, maxWait }: { list: { position: number; number: string; name: string; wait: number; callback: boolean; queue?: string }[]; max: number; maxWait?: number }) {
  const { t } = useTranslation()
  return (
    <ol className="flex min-h-0 flex-col gap-[0.3em] overflow-hidden">
      {list.slice(0, max).map((c, i) => (
        <li key={i} className={`flex items-center gap-[0.7em] rounded-[0.5em] px-[0.7em] py-[0.3em] text-[1.05em] ${c.callback ? 'bg-accent-2/10' : 'bg-tint/[0.04]'}`}>
          <span className="font-mono text-subtle">#{c.position}</span>
          <span className="min-w-0 flex-1 truncate font-mono">{c.number || t('reception.unknownCaller')}{c.name && <span className="font-sans text-muted"> · {c.name}</span>}{c.queue && <span className="font-sans text-subtle"> · {c.queue}</span>}</span>
          {c.callback && <PhoneForwarded className="size-[0.9em] text-accent-2" aria-label={t('wallboard.callback')} />}
          <span className={`font-mono tabular-nums ${c.callback ? 'text-accent-2' : maxWait && c.wait > maxWait ? 'text-danger' : 'text-warn'}`}>{formatDuration(c.wait)}</span>
        </li>
      ))}
    </ol>
  )
}

function CallersBody({ tile, data, callbacks }: { tile: WallTile; data: WallData; callbacks?: boolean }) {
  const { t } = useTranslation()
  const qs = tileQueues(data, tile)
  const list = qs.flatMap((q) => q.callers.filter((c) => !callbacks || c.callback).map((c) => ({ ...c, queue: qs.length > 1 ? q.name : undefined })))
    .sort((a, b) => b.wait - a.wait)
  const maxWait = qs.length === 1 ? qs[0].sla.maxWait : undefined
  return (
    <>
      <Label tile={tile}><span className="font-mono text-[0.9em] text-muted">{list.length}</span></Label>
      <div className="mt-[0.5em] min-h-0 flex-1 overflow-hidden">
        {list.length ? <CallerRows list={list} max={14} maxWait={maxWait} /> : <div className="text-subtle">{t(callbacks ? 'wallDesign.noCallbacks' : 'wallboard.nobodyWaiting')}</div>}
      </div>
    </>
  )
}

function QueueBody({ tile, data }: { tile: WallTile; data: WallData }) {
  const { t } = useTranslation()
  const q = tileQueues(data, tile)[0]
  if (!q) return <div className="grid h-full place-items-center text-muted">{t('wallboard.emptyText')}</div>
  const one = { ...tile, queues: [q.id] }
  const th = thresholds(data, { ...one, fromSla: true, metric: 'serviceLevel' })
  const sl = q.today.serviceLevel
  const waitTone: Tone = q.waiting ? (q.sla.maxWaiting && q.waiting > q.sla.maxWaiting ? 'danger' : 'warn') : 'ok'
  const cell = (v: ReactNode, l: string, tn: Tone = 'ink') => (
    <div className="min-w-0"><div className={`font-mono text-[2.6em] leading-none tabular-nums ${toneText[tn]}`}>{v}</div><div className="mt-[0.35em] truncate text-[0.75em] uppercase tracking-[0.1em] text-subtle">{l}</div></div>
  )
  return (
    <div className="flex h-full min-h-0 flex-col gap-[0.9em]">
      <div className="flex items-baseline justify-between gap-[0.8em]">
        <h2 className="truncate font-display text-[1.8em] font-semibold">{tile.title || q.name}</h2>
        <span className="font-mono text-subtle">{q.number}</span>
      </div>
      <div className="grid grid-cols-4 gap-[0.8em]">
        {cell(q.waiting, t('wallboard.waiting'), waitTone)}
        {cell(formatDuration(q.longestWait), t('wallboard.longestWait'), q.sla.maxWait && q.longestWait > q.sla.maxWait ? 'danger' : 'ink')}
        {cell(sl < 0 ? '–' : `${Math.round(sl)}%`, t('wallboard.serviceLevel'), metricTone('serviceLevel', sl, th))}
        {cell(q.callbacks, t('wallboard.callbacks'), q.callbacks ? 'ink' : 'muted')}
      </div>
      <div className="min-h-0 flex-1 overflow-hidden">{q.callers.length ? <CallerRows list={q.callers} max={10} maxWait={q.sla.maxWait} /> : <div className="text-subtle">{t('wallboard.nobodyWaiting')}</div>}</div>
      <div className="grid grid-cols-4 gap-[0.6em] border-t border-line pt-[0.7em]">
        {([[q.today.answered, t('wallboard.answered')], [q.today.abandoned - q.today.shortAbandoned, t('wallboard.abandoned')], [formatDuration(q.today.avgWait), t('wallboard.avgWait')], [formatDuration(q.today.avgTalk), t('wallboard.avgTalk')]] as const).map(([v, l], i) => (
          <div key={i} className="min-w-0"><div className="font-mono text-[1.5em] tabular-nums">{v}</div><div className="truncate text-[0.7em] uppercase tracking-[0.08em] text-subtle">{l}</div></div>
        ))}
      </div>
    </div>
  )
}

function CompareBody({ tile, data }: { tile: WallTile; data: WallData }) {
  const metric = tile.metric || 'answered'
  const rows = tileQueues(data, tile).map((q) => {
    const one = { ...tile, queues: [q.id], metric }
    const v = metricValue(data, one)
    return { q, v, tone: metricTone(metric, v, thresholds(data, one)) }
  })
  const max = metricInfo[metric].kind === 'pct' ? 100 : Math.max(1, ...rows.map((r) => r.v))
  return (
    <>
      <Label tile={tile} />
      <div className="mt-[0.6em] flex min-h-0 flex-1 flex-col justify-center gap-[0.7em] overflow-hidden">
        {rows.map(({ q, v, tone }) => (
          <div key={q.id} className="grid grid-cols-[minmax(0,7em)_1fr_auto] items-center gap-[0.7em]">
            <span className="truncate">{q.name}</span>
            <span className="h-[0.7em] overflow-hidden rounded-full bg-tint/10"><span className={`block h-full rounded-full ${tone === 'danger' ? 'bg-danger' : tone === 'warn' ? 'bg-warn' : tone === 'ok' ? 'bg-ok' : 'bg-accent'}`} style={{ width: `${v < 0 ? 0 : Math.min(100, (v / max) * 100)}%` }} /></span>
            <span className={`font-mono tabular-nums ${toneText[tone]}`}>{formatMetric(metric, v)}</span>
          </div>
        ))}
      </div>
    </>
  )
}

function agentDetail(a: WallAgent, data: WallData, t: TFunction) {
  if (a.state === 'call') return [a.queue ? data.queues.find((q) => q.id === a.queue)?.name : '', a.caller].filter(Boolean).join(' · ') || t('wallboard.state.call')
  if (a.state === 'free') return t('wallboard.callsToday', { count: a.callsToday })
  return t(`wallboard.state.${a.state}`)
}

function since(unix: number, now: Date) { return Math.max(0, Math.floor(now.getTime() / 1000 - unix)) }

function AgentsBody({ tile, data, now }: { tile: WallTile; data: WallData; now: Date }) {
  const { t } = useTranslation()
  const agents = tileAgents(data, tile)
  const n = (s: string) => agents.filter((a) => a.state === s).length
  return (
    <>
      <Label tile={tile} />
      <div className="mb-[0.4em] text-[0.85em] text-muted">{t('wallboard.agentCounts', { free: n('free'), busy: n('call') + n('ringing') + n('internal'), paused: n('paused') })}</div>
      {!agents.length && <div className="text-subtle">{t('wallboard.noAgents')}</div>}
      <ul className="flex min-h-0 flex-1 flex-col overflow-hidden">
        {agents.map((a) => (
          <li key={a.number} className="flex items-center gap-[0.6em] border-b border-tint/[0.06] py-[0.4em] last:border-0">
            <span className={`size-[0.65em] shrink-0 rounded-full ${stateDot[a.state]}`} aria-hidden />
            <div className="min-w-0 flex-1">
              <div className="truncate text-[1.05em]">{a.name || a.number}</div>
              <div className="truncate text-[0.75em] text-subtle">{agentDetail(a, data, t)}</div>
            </div>
            <span className={`font-mono tabular-nums ${a.state === 'paused' ? 'text-warn' : 'text-muted'}`}>{a.state === 'offline' ? '' : formatDuration(since(a.since, now))}</span>
          </li>
        ))}
      </ul>
    </>
  )
}

function AgentTilesBody({ tile, data, now }: { tile: WallTile; data: WallData; now: Date }) {
  const { t } = useTranslation()
  const agents = tileAgents(data, tile)
  const border: Record<string, string> = { free: 'border-ok/40', ringing: 'border-accent/50', call: 'border-danger/45', internal: 'border-accent-2/45', paused: 'border-warn/45', offline: 'border-line opacity-60' }
  return (
    <>
      <Label tile={tile} />
      <div className="mt-[0.5em] grid min-h-0 flex-1 content-start gap-[0.6em] overflow-hidden" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(min(100%, 13em), 1fr))' }}>
        {agents.map((a) => (
          <div key={a.number} className={`flex flex-col gap-[0.25em] rounded-[0.8em] border bg-tint/[0.03] p-[0.7em] ${border[a.state]}`}>
            <div className="flex items-center gap-[0.5em]">
              <span className={`size-[0.65em] shrink-0 rounded-full ${stateDot[a.state]}`} aria-hidden />
              <span className="min-w-0 flex-1 truncate font-display text-[1.15em] font-semibold">{a.name || a.number}</span>
            </div>
            <div className="flex items-end justify-between gap-[0.5em]">
              <span className="truncate text-[0.8em] text-subtle">{agentDetail(a, data, t)}</span>
              <span className="font-mono text-[1.3em] tabular-nums">{a.state === 'offline' ? '' : formatDuration(since(a.since, now))}</span>
            </div>
          </div>
        ))}
      </div>
    </>
  )
}

function LeaderBody({ tile, data }: { tile: WallTile; data: WallData }) {
  const { t } = useTranslation()
  const list = [...tileAgents(data, tile)].filter((a) => a.callsToday > 0).sort((a, b) => b.callsToday - a.callsToday).slice(0, Math.max(3, tile.h * 2 - 1))
  return (
    <>
      <Label tile={tile} />
      {!list.length && <div className="mt-[0.5em] text-subtle">{t('wallDesign.noCallsYet')}</div>}
      <ol className="mt-[0.4em] flex min-h-0 flex-col gap-[0.35em] overflow-hidden">
        {list.map((a, i) => (
          <li key={a.number} className="flex items-center gap-[0.7em] text-[1.1em]">
            <span className={`w-[1.2em] font-mono ${i === 0 ? 'text-accent' : 'text-subtle'}`}>{i + 1}</span>
            <span className="min-w-0 flex-1 truncate">{a.name || a.number}</span>
            <span className="font-mono tabular-nums">{a.callsToday}</span>
          </li>
        ))}
      </ol>
    </>
  )
}

function HourlyBody({ tile, data, now }: { tile: WallTile; data: WallData; now: Date }) {
  const { t } = useTranslation()
  const hours = hourly(data, tile)
  const cur = now.getHours()
  let from = 8
  let to = Math.max(18, cur + 1)
  hours.forEach((h, i) => { if (h.answered + h.abandoned > 0) { from = Math.min(from, i); to = Math.max(to, i + 1) } })
  const list = hours.slice(from, Math.min(24, to))
  const max = Math.max(4, ...list.map((h) => Math.max(h.answered, h.abandoned)))
  return (
    <>
      <Label tile={tile}>
        <span className="inline-flex items-center gap-[0.35em] text-[0.75em] text-muted"><i className="block size-[0.7em] rounded-sm bg-accent" aria-hidden />{t('wallboard.answered')}</span>
        <span className="inline-flex items-center gap-[0.35em] text-[0.75em] text-muted"><i className="block size-[0.7em] rounded-sm bg-danger" aria-hidden />{t('wallboard.abandoned')}</span>
      </Label>
      <div className="mt-[0.6em] flex min-h-0 flex-1 items-end gap-[0.3em] border-b border-tint/15" role="img" aria-label={tileTitle(tile, t)}>
        {list.map((h, i) => (
          <div key={i} className="flex h-full flex-1 items-end justify-center gap-[2px]">
            <span className={`block w-[34%] rounded-t-[3px] bg-accent ${from + i === cur ? '' : 'opacity-80'}`} style={{ height: `${(h.answered / max) * 100}%` }} />
            <span className="block w-[34%] rounded-t-[3px] bg-danger opacity-85" style={{ height: `${(h.abandoned / max) * 100}%` }} />
          </div>
        ))}
      </div>
      <div className="mt-[0.25em] flex gap-[0.3em]">
        {list.map((_, i) => <span key={i} className={`flex-1 text-center font-mono text-[0.7em] ${from + i === cur ? 'text-ink' : 'text-subtle'}`}>{String(from + i).padStart(2, '0')}</span>)}
      </div>
    </>
  )
}

function DonutBody({ tile, data }: { tile: WallTile; data: WallData }) {
  const { t } = useTranslation()
  const tt = periodTotals(tileQueues(data, tile), tile.period)
  const ab = tt.abandoned - tt.shortAbandoned
  const total = tt.answered + ab
  const r = 38
  const c = 2 * Math.PI * r
  const part = total ? tt.answered / total : 0
  return (
    <>
      <Label tile={tile} />
      <div className="flex min-h-0 flex-1 items-center gap-[1em]">
        <svg viewBox="0 0 100 100" className="h-full max-h-[70cqh] w-auto" role="img" aria-label={`${t('wallboard.answered')} ${tt.answered}, ${t('wallboard.abandoned')} ${ab}`}>
          <circle cx="50" cy="50" r={r} fill="none" strokeWidth="14" className={total ? 'stroke-danger' : 'stroke-tint/10'} />
          {total > 0 && <circle cx="50" cy="50" r={r} fill="none" strokeWidth="14" strokeDasharray={`${part * c} ${c}`} transform="rotate(-90 50 50)" className="stroke-accent" />}
          <text x="50" y="56" textAnchor="middle" className="fill-current font-mono text-ink" style={{ fontSize: 17 }}>{total ? `${Math.round(part * 100)}%` : '–'}</text>
        </svg>
        <div className="flex min-w-0 flex-col gap-[0.5em]">
          <div><div className="font-mono text-[1.8em] leading-none text-accent">{tt.answered}</div><div className="text-[0.75em] uppercase tracking-[0.08em] text-subtle">{t('wallboard.answered')}</div></div>
          <div><div className="font-mono text-[1.8em] leading-none text-danger">{ab}</div><div className="text-[0.75em] uppercase tracking-[0.08em] text-subtle">{t('wallboard.abandoned')}</div></div>
        </div>
      </div>
    </>
  )
}

function ClockBody({ tile, now }: { tile: WallTile; now: Date }) {
  const { i18n } = useTranslation()
  return (
    <div className="flex h-full flex-col items-center justify-center text-center">
      <div className="font-mono leading-none tabular-nums" style={{ fontSize: 'min(46cqh, 26cqw)' }}>{now.toLocaleTimeString(i18n.language, { hour: '2-digit', minute: '2-digit' })}</div>
      {!tile.hideLabel && <div className="mt-[0.4em] text-subtle" style={{ fontSize: 'min(12cqh, 7cqw)' }}>{now.toLocaleDateString(i18n.language, { weekday: 'long', day: 'numeric', month: 'long' })}</div>}
    </div>
  )
}

function TextBody({ tile }: { tile: WallTile }) {
  const { t } = useTranslation()
  return (
    <>
      {tile.title && !tile.hideLabel && <div className="font-display text-[1.4em] font-semibold">{tile.title}</div>}
      <div className={`mt-[0.3em] min-h-0 flex-1 overflow-hidden whitespace-pre-wrap text-[1.3em] leading-snug ${tile.text ? '' : 'text-subtle'}`}>{tile.text || t('wallDesign.textPlaceholder')}</div>
    </>
  )
}

function TickerBody({ tile }: { tile: WallTile }) {
  const { t } = useTranslation()
  const text = tile.text.split('\n').map((s) => s.trim()).filter(Boolean).join('   ·   ') || t('wallDesign.textPlaceholder')
  const ref = useRef<HTMLDivElement>(null)
  const [time, setTime] = useState(30)
  useEffect(() => { if (ref.current) setTime(Math.max(12, ref.current.scrollWidth / 2 / 60)) }, [text])
  return (
    <div className="flex h-full items-center gap-[1em] overflow-hidden">
      <BellRing className="size-[1.1em] shrink-0 text-accent" aria-hidden />
      <div className="min-w-0 flex-1 overflow-hidden" title={text}>
        <div ref={ref} className="kt-wall-ticker flex w-max whitespace-nowrap text-[1.3em]" style={{ '--ticker-time': `${time}s` } as CSSProperties}>
          <span className="pr-[3em]">{text}</span><span className="pr-[3em]" aria-hidden>{text}</span>
        </div>
      </div>
    </div>
  )
}

function ImageBody({ tile }: { tile: WallTile }) {
  const { t } = useTranslation()
  if (!tile.image) return <div className="grid h-full place-items-center text-subtle"><span className="flex flex-col items-center gap-[0.4em]"><ImageIcon className="size-[2em]" aria-hidden />{t('wallDesign.noImage')}</span></div>
  return <img src={tile.image} alt={tile.title} className="h-full w-full rounded-[0.7em] object-contain" />
}

function AlertsBody({ tile, data }: { tile: WallTile; data: WallData }) {
  const { t, i18n } = useTranslation()
  const list = data.alerts.filter((a) => !tile.queues.length || tile.queues.includes(a.queueId))
  const value = (kind: string, v: number) => (kind === 'wait' ? formatDuration(Math.round(v)) : kind === 'level' || kind === 'abandoned' ? `${Math.round(v)}%` : String(Math.round(v)))
  return (
    <>
      <Label tile={tile} />
      {!list.length ? (
        <div className="flex flex-1 items-center gap-[0.6em] text-ok"><CheckCircle2 className="size-[1.4em]" aria-hidden /><span className="text-[1.15em]">{t('wallDesign.allWithinNorm')}</span></div>
      ) : (
        <ul className="mt-[0.5em] flex min-h-0 flex-col gap-[0.35em] overflow-hidden">
          {list.map((a, i) => (
            <li key={i} className="flex items-center gap-[0.6em] rounded-[0.5em] bg-danger/10 px-[0.7em] py-[0.3em]">
              <span className="size-[0.6em] shrink-0 animate-pulse rounded-full bg-danger" aria-hidden />
              <span className="min-w-0 flex-1 truncate"><b className="text-danger">{a.queue}</b> · {t(`wallboard.alert.${a.kind}`, { value: value(a.kind, a.value), limit: value(a.kind, a.limit) })}</span>
              <span className="font-mono text-[0.8em] text-muted">{new Date(a.since).toLocaleTimeString(i18n.language, { hour: '2-digit', minute: '2-digit' })}</span>
            </li>
          ))}
        </ul>
      )}
    </>
  )
}

/** Plays a short two-tone signal (when the browser allows sound). */
export function alarmSound() {
  try {
    const Ctx = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext
    const ctx = new Ctx()
    ;[880, 660].forEach((f, i) => {
      const o = ctx.createOscillator()
      const g = ctx.createGain()
      o.frequency.value = f
      g.gain.setValueAtTime(0.0001, ctx.currentTime + i * 0.25)
      g.gain.exponentialRampToValueAtTime(0.25, ctx.currentTime + i * 0.25 + 0.02)
      g.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + i * 0.25 + 0.22)
      o.connect(g).connect(ctx.destination)
      o.start(ctx.currentTime + i * 0.25)
      o.stop(ctx.currentTime + i * 0.25 + 0.24)
    })
    window.setTimeout(() => ctx.close().catch(() => undefined), 1000)
  } catch { /* no sound */ }
}

/** A designed screen on a tv: rotates the pages, jumps to an alarm and beeps when asked to. */
export function DesignedScreen({ data, design, name }: { data: WallData; design: WallDesign; name: string }) {
  const [now, setNow] = useState(() => new Date())
  const [page, setPage] = useState(0)
  const pages = design.pages.length
  useEffect(() => { const id = window.setInterval(() => setNow(new Date()), 1000); return () => window.clearInterval(id) }, [])
  const seconds = design.pages[page % Math.max(1, pages)]?.seconds ?? 30
  useEffect(() => {
    if (pages < 2) return
    const id = window.setTimeout(() => setPage((p) => (p + 1) % pages), seconds * 1000)
    return () => window.clearTimeout(id)
  }, [page, pages, seconds])
  const alertKey = useMemo(() => data.alerts.map((a) => `${a.queueId}:${a.kind}`).sort().join(','), [data.alerts])
  const prev = useRef('')
  useEffect(() => {
    const before = new Set(prev.current.split(',').filter(Boolean))
    const fresh = alertKey.split(',').filter((k) => k && !before.has(k))
    prev.current = alertKey
    if (!fresh.length) return
    if (design.style.alarmSound) alarmSound()
    if (design.style.alarmJump && pages > 1) {
      const qid = Number(fresh[0].split(':')[0])
      const i = design.pages.findIndex((p) => p.tiles.some((t) => (t.type === 'alerts' || t.type === 'queue' || t.fromSla) && (!t.queues.length || t.queues.includes(qid))))
      if (i >= 0) setPage(i)
    }
  }, [alertKey, design, pages])
  return (
    <BoardFrame design={design} now={now} className="h-screen w-screen">
      <Board data={data} design={design} name={name} page={page} pages={pages} now={now} />
    </BoardFrame>
  )
}
