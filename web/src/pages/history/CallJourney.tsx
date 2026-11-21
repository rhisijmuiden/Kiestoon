import { createContext, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  Background, BackgroundVariant, Controls, Handle, Position, ReactFlow, ReactFlowProvider,
  type Edge, type Node, type NodeProps,
} from '@xyflow/react'
import '@xyflow/react/dist/base.css'
import {
  ArrowLeft, ArrowRightLeft, Check, CircleParking, Clock, Disc3, FileText, GitBranch, Grid3x3, Hand, Hourglass, ListOrdered, Phone, PhoneCall,
  PhoneForwarded, PhoneIncoming, PhoneMissed, PhoneOff, PhoneOutgoing, Repeat, Route, TriangleAlert, Users, UsersRound, Voicemail, X,
  type LucideIcon,
} from 'lucide-react'
import { Badge, formatDate, useApi } from '../../components/kit'
import { Button, Card, ErrorMessage } from '../../components/ui'
import { useTheme } from '../../lib/theme'
import type { Journey, JourneyParty, JourneyStep, JourneySub, QualityCall, QualityDirection, QualityLeg, QualityLimits } from '../../lib/types'
import { MosPill, Strip, num, rating, ratingText } from '../../components/quality'
import { tones, type Tone } from '../ivr/flow'

// The call journey: how a call went, step by step, drawn as a flow in the style of the IVR
// builder, with the same steps as a list next to it.

const NODE_W = 260

/** 8s, 1m 05s, 1h 02m. */
export function fmtMs(ms: number) {
  const s = Math.round(ms / 1000)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m ${String(s % 60).padStart(2, '0')}s`
  return `${Math.floor(m / 60)}h ${String(m % 60).padStart(2, '0')}m`
}

const stepIcon: Record<string, LucideIcon> = {
  inbound: PhoneIncoming, internal: PhoneOutgoing, outbound: Route, timecondition: Clock, ivr: GitBranch, queue: ListOrdered,
  ringgroup: UsersRound, extension: PhoneForwarded, talk: PhoneCall, transfer: ArrowRightLeft, voicemail: Voicemail, forward: PhoneForwarded,
  park: CircleParking, pickup: Hand, conference: Users, fax: FileText, external: Phone, noroute: TriangleAlert, hangup: PhoneOff,
}

function toneOf(s: JourneyStep): Tone {
  switch (s.tone) {
    case 'ok':
      return 'ok'
    case 'warn':
      return 'warn'
    case 'danger':
      return 'danger'
    case 'accent':
      return s.kind === 'inbound' || s.kind === 'internal' || s.kind === 'timecondition' || s.kind === 'transfer' ? 'accent2' : 'accent'
    default:
      return 'accent2'
  }
}

export function partyLabel(t: TFunction, p?: JourneyParty) {
  if (!p) return ''
  if (p.kind === 'ext') return [p.number, p.name].filter(Boolean).join(' · ') || t('journey.party.ext')
  if (p.kind === 'caller') return t('journey.party.caller')
  if (p.kind === 'remote') return p.name || p.number || t('journey.party.remote')
  return t('journey.party.system')
}

/** The title of a step, e.g. "IVR menu · Main menu". */
function stepTitle(t: TFunction, s: JourneyStep) {
  const kind = t(`journey.step.${s.kind}`, { defaultValue: s.kind })
  switch (s.kind) {
    case 'inbound':
      return kind
    case 'internal':
      return t('journey.step.internal')
    case 'talk':
      return t('journey.talkWith', { name: partyLabel(t, s.party) })
    case 'extension':
    case 'voicemail':
    case 'pickup':
      return `${kind} ${[s.number, s.name].filter(Boolean).join(' · ')}`.trim()
    case 'forward':
    case 'external':
    case 'noroute':
      return `${kind} ${s.number ?? ''}`.trim()
    case 'transfer':
      return t(`journey.transfer.${s.result}`, { defaultValue: kind })
    case 'hangup':
      return kind
    default:
      return s.name ? `${kind} · ${s.name}` : s.ref ? `${kind} #${s.ref}` : kind
  }
}

/** The result line of a step, with its colour. */
function stepResult(t: TFunction, s: JourneyStep): { text: string; tone?: 'ok' | 'warn' | 'danger' | 'accent' } | null {
  const r = s.result
  switch (s.kind) {
    case 'timecondition':
      return r ? { text: t(`journey.tc.${r}`, { defaultValue: r }), tone: r === 'open' ? 'ok' : 'warn' } : null
    case 'ivr': {
      const key = s.subs?.filter((x) => x.kind === 'key' || x.kind === 'speech').at(-1)
      if (key?.kind === 'speech') return key.value ? { text: t('journey.ivr.said', { intent: key.value }), tone: 'accent' } : { text: t('journey.ivr.notUnderstood'), tone: 'warn' }
      if (key) return { text: t('journey.ivr.pressed', { key: key.value }), tone: 'accent' }
      return { text: t('journey.ivr.none'), tone: 'warn' }
    }
    case 'queue':
      if (!r) return null
      return { text: t(`journey.queueResult.${r}`, { defaultValue: r }), tone: r === 'answered' ? 'ok' : r === 'abandoned' ? 'danger' : r === 'callback' ? 'accent' : 'warn' }
    case 'extension':
    case 'ringgroup':
    case 'outbound':
      if (!r) return null
      return { text: t(`journey.extResult.${r}`, { defaultValue: r }), tone: r === 'answered' ? 'ok' : 'warn' }
    case 'voicemail':
      return r ? { text: t(`journey.vm.${r}`, { defaultValue: r }) } : null
    case 'transfer':
      return s.number || s.name ? { text: t('journey.transferTo', { to: [s.number, s.name].filter(Boolean).join(' · ') }), tone: 'accent' } : null
    case 'hangup':
      return { text: hangupText(t, s.party) }
    default:
      return null
  }
}

function hangupText(t: TFunction, p?: JourneyParty) {
  if (!p || p.kind === 'system') return t('journey.hangup.system')
  if (p.kind === 'ext') return t('journey.hangup.ext', { name: partyLabel(t, p) })
  return t(`journey.hangup.${p.kind}`)
}

/** Details below the title: trunk and number, queue position and wait, who transferred. */
function stepFacts(t: TFunction, s: JourneyStep): [string, string][] {
  const d = s.detail ?? {}
  const out: [string, string][] = []
  switch (s.kind) {
    case 'inbound':
      if (s.number) out.push([t('journey.from'), [s.number, s.name].filter(Boolean).join(' · ')])
      if (d.did) out.push([t('journey.did'), d.did])
      if (d.trunk) out.push([t('journey.trunk'), d.trunk])
      if (d.route) out.push([t('journey.route'), d.route])
      break
    case 'internal':
      out.push([t('journey.from'), partyLabel(t, s.party)])
      if (d.dialled) out.push([t('journey.dialled'), d.dialled])
      break
    case 'outbound':
      if (s.number) out.push([t('journey.dialled'), s.number])
      if (d.trunk) out.push([t('journey.trunk'), d.trunk])
      break
    case 'queue':
      if (d.position) out.push([t('journey.position'), d.position])
      if (d.holdMs) out.push([t('journey.waited'), fmtMs(Number(d.holdMs))])
      break
    case 'transfer':
      if (s.party) out.push([t('journey.by'), partyLabel(t, s.party)])
      break
    case 'park':
      if (s.party) out.push([t('journey.by'), partyLabel(t, s.party)])
      break
  }
  return out
}

function subText(t: TFunction, x: JourneySub) {
  switch (x.kind) {
    case 'key':
      return t('journey.ivr.pressed', { key: x.value })
    case 'timeout':
    case 'invalid':
      return t(`journey.ivr.${x.kind}`, { n: x.value })
    case 'repeat':
      return t('journey.ivr.repeat')
    case 'speech':
      return x.value ? t('journey.ivr.said', { intent: x.value }) : t('journey.ivr.notUnderstood')
    default:
      return partyLabel(t, x.party)
  }
}

const resCls = { ok: 'text-ok', warn: 'text-warn', danger: 'text-danger', accent: 'text-accent' } as const

const ringTone = (r?: string) => (r === 'answered' ? 'text-ok' : r === 'noanswer' || r === 'busy' || r === 'unavailable' ? 'text-danger' : 'text-subtle')

function SubRow({ x }: { x: JourneySub }) {
  const { t } = useTranslation()
  const Icon = x.kind === 'ring' ? (x.result === 'answered' ? Check : x.result === 'cancel' ? Disc3 : PhoneMissed)
    : x.kind === 'key' ? Grid3x3 : x.kind === 'repeat' ? Repeat : x.kind === 'timeout' ? Hourglass : X
  const tone = x.kind === 'ring' ? ringTone(x.result) : x.kind === 'key' ? 'text-accent' : 'text-warn'
  return (
    <li className="flex min-w-0 items-center gap-2 text-[12.5px]">
      <Icon className={`size-3.5 flex-none ${tone}`} aria-hidden />
      <span className="min-w-0 flex-1 truncate text-ink-2">{subText(t, x)}</span>
      {x.kind === 'ring' && x.result && <span className={`text-[11px] ${tone}`}>{t(`journey.ring.${x.result}`, { defaultValue: x.result })}</span>}
      <span className="font-mono text-[11px] text-subtle">{x.kind === 'ring' ? fmtMs(x.durationMs) : `+${fmtMs(x.durationMs)}`}</span>
    </li>
  )
}

type StepNodeData = { step: JourneyStep; in?: string; out?: string; [k: string]: unknown }
type JNode = Node<StepNodeData, 'step'>

const JourneyCtx = createContext<{ start: number }>({ start: 0 })

function StepCard({ s, compact }: { s: JourneyStep; compact?: boolean }) {
  const { t } = useTranslation()
  const { start } = useContext(JourneyCtx)
  const tone = tones[toneOf(s)]
  const Icon = stepIcon[s.kind] ?? Phone
  const res = stepResult(t, s)
  const facts = stepFacts(t, s)
  const offset = new Date(s.at).getTime() - start
  return (
    <>
      <div className="flex h-11 items-center gap-2 border-b border-line px-3">
        <span className={`inline-flex size-6 flex-none items-center justify-center rounded-md ${tone.bg} ${tone.text}`}><Icon className="size-3.5" aria-hidden /></span>
        <span className="min-w-0 flex-1 truncate text-[13px] font-semibold text-ink">{stepTitle(t, s)}</span>
        <span className="font-mono text-[11px] text-subtle" title={t('journey.offset')}>{offset > 0 ? `+${fmtMs(offset)}` : '0s'}</span>
      </div>
      <div className="flex flex-col gap-2 px-3 py-2.5">
        {res && (
          <div className="flex items-center justify-between gap-2">
            <span className={`text-[13px] font-medium ${res.tone ? resCls[res.tone] : 'text-ink-2'}`}>{res.text}</span>
            {s.kind !== 'hangup' && s.durationMs > 0 && <span className="font-mono text-xs text-muted">{fmtMs(s.durationMs)}</span>}
          </div>
        )}
        {!res && s.kind !== 'hangup' && s.kind !== 'inbound' && s.kind !== 'internal' && (
          <div className="flex items-center justify-between gap-2 text-[13px] text-ink-2">
            <span>{t('journey.duration')}</span><span className="font-mono text-xs text-muted">{fmtMs(s.durationMs)}</span>
          </div>
        )}
        {facts.length > 0 && (
          <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-0.5 text-[12px]">
            {facts.map(([k, v]) => <FactRow key={k} k={k} v={v} />)}
          </dl>
        )}
        {!compact && s.subs && s.subs.length > 0 && (
          <ul className="flex flex-col gap-1 border-t border-line pt-2" aria-label={s.kind === 'ivr' ? t('journey.choices') : t('journey.rang')}>
            {s.subs.map((x, i) => <SubRow key={i} x={x} />)}
          </ul>
        )}
      </div>
    </>
  )
}

function FactRow({ k, v }: { k: string; v: string }) {
  return (
    <>
      <dt className="text-subtle">{k}</dt>
      <dd className="min-w-0 truncate text-right font-mono text-ink-2">{v}</dd>
    </>
  )
}

type Side = 'top' | 'bottom' | 'left' | 'right'
const posOf: Record<Side, Position> = { top: Position.Top, bottom: Position.Bottom, left: Position.Left, right: Position.Right }

function JourneyNodeView({ data }: NodeProps<JNode>) {
  const tone = tones[toneOf(data.step)]
  const cls = `!size-2.5 !rounded-full !border-2 !border-surface ${tone.dot}`
  return (
    <div style={{ width: NODE_W }} className={`relative rounded-2xl border-[1.5px] bg-surface text-left shadow-[0_10px_30px_-18px_rgb(0_0_0_/_0.6)] ${tone.border}`}>
      {data.in && <Handle type="target" id="in" position={posOf[data.in as Side]} isConnectable={false} className={cls} />}
      <StepCard s={data.step} />
      {data.out && <Handle type="source" id="out" position={posOf[data.out as Side]} isConnectable={false} className={cls} />}
    </div>
  )
}

const nodeTypes = { step: JourneyNodeView }

/** A rough height per step, so the layout leaves room without measuring. */
function heightOf(t: TFunction, s: JourneyStep) {
  const facts = stepFacts(t, s).length
  const subs = s.subs?.length ?? 0
  return 44 + 20 + 26 + (facts ? facts * 18 + 6 : 0) + (subs ? subs * 22 + 12 : 0)
}

const GAP_X = 64
const GAP_Y = 64

/** How many steps fit next to each other on a canvas this wide (1 to 4). */
const colsFor = (width: number) => Math.max(1, Math.min(4, Math.floor((width - 16 + GAP_X) / (NODE_W + GAP_X))))

/**
 * Lays the steps out as a snake that fits the width of the canvas: left to right, then the next row
 * right to left, so every line stays short. A narrow screen gets one column, top to bottom.
 */
function layout(t: TFunction, steps: JourneyStep[], width: number): { nodes: JNode[]; edges: Edge[] } {
  const cols = colsFor(width)
  const rows: JourneyStep[][] = []
  for (let i = 0; i < steps.length; i += cols) rows.push(steps.slice(i, i + cols))
  const nodes: JNode[] = []
  const edges: Edge[] = []
  const sides = new Map<number, { in?: Side; out?: Side }>()
  // Which side each line leaves and enters.
  steps.forEach((s, i) => {
    if (i === 0) return
    const prev = steps[i - 1]
    const sameRow = Math.floor((i - 1) / cols) === Math.floor(i / cols)
    const ltr = Math.floor(i / cols) % 2 === 0
    const out: Side = sameRow ? (ltr ? 'right' : 'left') : 'bottom'
    const inn: Side = sameRow ? (ltr ? 'left' : 'right') : 'top'
    sides.set(prev.id, { ...sides.get(prev.id), out })
    sides.set(s.id, { ...sides.get(s.id), in: inn })
    const tone = tones[toneOf(prev)]
    edges.push({
      id: `${prev.id}-${s.id}`, source: String(prev.id), sourceHandle: 'out', target: String(s.id), targetHandle: 'in', type: 'smoothstep',
      label: prev.kind !== 'inbound' && prev.kind !== 'internal' && prev.durationMs >= 1000 ? fmtMs(prev.durationMs) : undefined,
      labelStyle: { fill: 'var(--color-ink-2)', fontSize: 11, fontFamily: 'var(--font-mono, monospace)' },
      labelBgStyle: { fill: 'var(--color-surface)' }, labelBgPadding: [6, 3], labelBgBorderRadius: 6,
      animated: prev.kind === 'talk' || s.kind === 'talk', style: { stroke: tone.stroke, strokeWidth: 2 },
    })
  })
  let y = 0
  rows.forEach((row, r) => {
    const h = Math.max(...row.map((s) => heightOf(t, s)))
    row.forEach((s, c) => {
      const col = r % 2 === 0 ? c : cols - 1 - c
      const sd = sides.get(s.id) ?? {}
      nodes.push({ id: String(s.id), type: 'step', position: { x: col * (NODE_W + GAP_X), y }, data: { step: s, in: sd.in, out: sd.out }, draggable: false, selectable: false })
    })
    y += h + GAP_Y
  })
  return { nodes, edges }
}

const resultTone = { answered: 'ok', voicemail: 'warn', abandoned: 'danger', missed: 'danger', busy: 'warn', failed: 'danger' } as const

/** The journey of one call. base is the API path the journey comes from. */
export function CallJourney({ id, base, recordingBase, onBack }: { id: string; base: string; recordingBase: string; onBack: () => void }) {
  const { t, i18n } = useTranslation()
  const { theme } = useTheme()
  const j = useApi<Journey>(`${base}/${encodeURIComponent(id)}`)
  const box = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(900)
  useEffect(() => {
    const el = box.current
    if (!el) return
    const ro = new ResizeObserver(([e]) => setWidth(Math.round(e.contentRect.width)))
    ro.observe(el)
    return () => ro.disconnect()
  }, [j.data])
  const flow = useMemo(() => (j.data ? layout(t, j.data.steps, width) : { nodes: [], edges: [] }), [j.data, t, width])
  const d = j.data
  const ctx = useMemo(() => ({ start: d ? new Date(d.start).getTime() : 0 }), [d])
  const DirIcon = d?.direction === 'inbound' ? PhoneIncoming : d?.direction === 'outbound' ? PhoneOutgoing : Repeat
  const from = d ? (d.from.kind === 'caller' ? [d.from.number, d.from.name].filter(Boolean).join(' · ') : partyLabel(t, d.from)) : ''

  return (
    <JourneyCtx.Provider value={ctx}>
      <div className="flex flex-col gap-5">
        <div className="flex flex-wrap items-center gap-3">
          <Button variant="ghost" onClick={onBack}><ArrowLeft className="size-4" aria-hidden /> {t('journey.back')}</Button>
          <div className="min-w-0 flex-1">
            <h1 className="flex min-w-0 items-center gap-2 font-display text-xl font-semibold text-ink sm:text-2xl">
              <DirIcon className="size-5 flex-none text-accent" aria-hidden />
              <span className="min-w-0 break-words">{d ? t('journey.heading', { from: from || '–', to: d.to || '–' }) : t('journey.title')}</span>
            </h1>
            {d && <p className="mt-0.5 text-sm text-muted">{formatDate(d.start, i18n.language)} · {t(`history.dir.${d.direction}`)}</p>}
          </div>
          {d && <Badge tone={resultTone[d.result]} dot>{t(`journey.result.${d.result}`)}</Badge>}
        </div>
        <ErrorMessage error={j.error} />
        {d && (
          <>
            <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
              <Fact label={t('journey.total')} value={fmtMs(d.durationMs)} />
              <Fact label={t('journey.wait')} value={d.result === 'answered' ? fmtMs(d.waitMs) : '–'} />
              <Fact label={t('journey.talk')} value={d.talkMs ? fmtMs(d.talkMs) : '–'} />
              <Fact label={t('journey.stepsCount')} value={String(d.steps.length)} />
            </div>
            {!d.complete && <p className="rounded-xl border border-accent/40 bg-accent/10 px-4 py-2.5 text-sm text-accent-ink" role="status">{t('journey.live')}</p>}
            {d.partial && <p className="rounded-xl border border-line bg-tint/5 px-4 py-2.5 text-sm text-muted">{t('journey.partial')}</p>}
            <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_380px]">
              <Card className="kt-flow h-[min(72vh,760px)] min-h-[420px] overflow-hidden !p-0">
                <div ref={box} className="size-full">
                <ReactFlowProvider>
                  <ReactFlow<JNode, Edge> key={colsFor(width)} nodes={flow.nodes} edges={flow.edges} nodeTypes={nodeTypes} colorMode={theme}
                    fitView fitViewOptions={{ padding: 0.15, maxZoom: 1 }} minZoom={0.2} maxZoom={1.6}
                    nodesDraggable={false} nodesConnectable={false} elementsSelectable={false} proOptions={{ hideAttribution: true }}
                    aria-label={t('journey.flow')}>
                    <Background variant={BackgroundVariant.Dots} gap={22} size={1.4} color="var(--color-grid-dot)" />
                    <Controls showInteractive={false} position="bottom-right" />
                  </ReactFlow>
                </ReactFlowProvider>
                </div>
              </Card>
              <div className="flex min-w-0 flex-col gap-5">
                {d.quality && <QualityCard q={d.quality} />}
                <Card>
                  <h2 className="mb-3 text-sm font-semibold text-ink">{t('journey.steps')}</h2>
                  <ol className="relative flex flex-col gap-3 border-l border-line pl-4">
                    {d.steps.map((s) => <TimelineItem key={s.id} s={s} lang={i18n.language} />)}
                  </ol>
                </Card>
                <Card>
                  <h2 className="mb-3 text-sm font-semibold text-ink">{t('journey.recordings')}</h2>
                  {d.recordings.length === 0 ? <p className="text-sm text-muted">{t('journey.noRecordings')}</p> : (
                    <ul className="flex flex-col gap-3">
                      {d.recordings.map((r) => (
                        <li key={r.name} className="flex flex-col gap-1.5">
                          <span className="text-xs text-muted">{r.from} → {r.to} · {fmtMs(r.durationMs)}</span>
                          <audio controls preload="none" src={`/api${recordingBase}/${r.name}`} className="h-9 w-full" />
                        </li>
                      ))}
                    </ul>
                  )}
                </Card>
              </div>
            </div>
          </>
        )}
      </div>
    </JourneyCtx.Provider>
  )
}

function Fact({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="rounded-2xl border border-line bg-surface px-4 py-3">
      <div className="text-[11px] font-medium uppercase tracking-[0.08em] text-subtle">{label}</div>
      <div className="mt-1 font-mono text-lg text-ink">{value}</div>
    </div>
  )
}

function TimelineItem({ s, lang }: { s: JourneyStep; lang: string }) {
  const { t } = useTranslation()
  const tone = tones[toneOf(s)]
  const Icon = stepIcon[s.kind] ?? Phone
  const res = stepResult(t, s)
  return (
    <li className="relative">
      <span className={`absolute -left-[25px] top-0.5 inline-flex size-[17px] items-center justify-center rounded-full border border-surface ${tone.bg} ${tone.text}`}>
        <Icon className="size-2.5" aria-hidden />
      </span>
      <div className="flex items-baseline justify-between gap-2">
        <span className="min-w-0 truncate text-[13px] font-medium text-ink">{stepTitle(t, s)}</span>
        <span className="flex-none font-mono text-[11px] text-subtle">{new Date(s.at).toLocaleTimeString(lang, { hour: '2-digit', minute: '2-digit', second: '2-digit' })}</span>
      </div>
      {(res || s.durationMs >= 1000) && (
        <div className="text-[12.5px] text-muted">
          {res?.text}{res && s.durationMs >= 1000 && s.kind !== 'hangup' ? ' · ' : ''}{s.kind !== 'hangup' && s.durationMs >= 1000 ? fmtMs(s.durationMs) : ''}
        </div>
      )}
      {s.subs && s.subs.length > 0 && (
        <ul className="mt-1 flex flex-col gap-0.5">
          {s.subs.map((x, i) => <SubRow key={i} x={x} />)}
        </ul>
      )}
    </li>
  )
}

/** The measured audio quality of the call: a diagnosis and both directions of every leg. */
function QualityCard({ q }: { q: QualityCall }) {
  const { t } = useTranslation()
  const worst = q.legs.reduce((a, l) => (l.mos < a.mos ? l : a), q.legs[0])
  const side = worst.kind === 'trunk' ? 'trunk' : 'phone'
  return (
    <Card>
      <div className="mb-3 flex items-center justify-between gap-2">
        <h2 className="text-sm font-semibold text-ink">{t('quality.callTitle')}</h2>
        <MosPill mos={q.mos} limits={q.limits} label="MOS" />
      </div>
      <p className="mb-3 rounded-r-lg border-l-[3px] border-accent bg-accent/[0.07] px-3 py-2 text-[13px] leading-relaxed text-ink-2">
        {q.rating === 'good' || q.cause === 'ok'
          ? t('quality.diagGood')
          : t(`quality.diag.${q.cause}`, { where: q.where, side: t(`quality.side.${side}`) })}
      </p>
      <div className="flex flex-col gap-3">
        {q.legs.map((l) => <LegView key={l.uniqueId} l={l} limits={q.limits} />)}
      </div>
      <p className="mt-3 text-[12px] text-subtle">{t('quality.stripHint')}</p>
    </Card>
  )
}

function LegView({ l, limits }: { l: QualityLeg; limits: QualityLimits }) {
  const { t } = useTranslation()
  const secs = Math.max(1, (new Date(l.end).getTime() - new Date(l.start).getTime()) / 1000)
  const other = l.kind === 'trunk' ? 'provider' : 'phone'
  return (
    <div className="rounded-xl border border-line bg-inset p-3">
      <div className="mb-2 flex flex-wrap items-baseline gap-x-2 gap-y-0.5 text-[13px]">
        <span className="font-semibold text-ink">{t(l.kind === 'trunk' ? 'quality.legTrunk' : 'quality.legPhone', { name: l.name })}</span>
        <span className="font-mono text-[11.5px] text-muted">{[l.codec.toUpperCase(), l.remote].filter(Boolean).join(' · ')}</span>
      </div>
      <div className="grid gap-2 sm:grid-cols-2 xl:grid-cols-1 2xl:grid-cols-2">
        <DirView title={t('quality.rx')} d={l.rx} limits={limits} />
        <DirView title={t(`quality.tx.${other}`)} d={l.tx} limits={limits} />
      </div>
      {l.rtt > 0 && (
        <div className="mt-2 flex justify-between text-[12px] text-muted"><span>{t('quality.rttAvgMax')}</span><span className="font-mono text-ink-2">{Math.round(l.rtt)} / {Math.round(l.rttMax)} ms</span></div>
      )}
      <Strip samples={l.samples} limits={limits} seconds={secs} />
    </div>
  )
}

function DirView({ title, d, limits }: { title: string; d: QualityDirection; limits: QualityLimits }) {
  const { t, i18n } = useTranslation()
  const lang = i18n.language
  if (!d.reports) {
    return <div className="rounded-lg border border-line bg-surface p-2.5"><div className="mb-1 text-[11.5px] text-subtle">{title}</div><p className="text-[12px] text-muted">{t('quality.noReports')}</p></div>
  }
  const bad = (v: number, lim: number) => (v >= lim ? 'text-danger-ink' : 'text-ink-2')
  return (
    <div className="rounded-lg border border-line bg-surface p-2.5">
      <div className="mb-1 text-[11.5px] text-subtle">{title}</div>
      <dl className="grid grid-cols-[auto_auto] justify-between gap-x-3 gap-y-0.5 whitespace-nowrap text-[12px]">
        <dt className="text-muted">MOS</dt><dd className={`text-right font-mono ${ratingText[rating(d.mos, limits)]}`}>{num(d.mos, 2, lang)}</dd>
        <dt className="text-muted">{t('quality.jitterAvgMax')}</dt><dd className={`text-right font-mono ${bad(d.jitter, limits.jitter)}`}>{Math.round(d.jitter)} / {Math.round(d.jitterMax)} ms</dd>
        <dt className="text-muted">{t('quality.loss')}</dt><dd className={`text-right font-mono ${bad(d.loss, limits.loss)}`}>{num(d.loss, 1, lang)}%</dd>
      </dl>
    </div>
  )
}
