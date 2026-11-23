import { createContext, useContext, type CSSProperties, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import {
  Clock, GitBranch, Grid3x3, LayoutList, Hourglass, ListOrdered, Mic, Phone, PhoneForwarded, PhoneIncoming, PhoneOff, Repeat, TriangleAlert, UsersRound, Voicemail, Volume2,
  type LucideIcon,
} from 'lucide-react'
import { WavePlayer, promptSrc } from '../../components/WavePlayer'
import type { IvrLive, Prompt } from '../../lib/types'
import { NODE_W, intentPort, type Kind, type StepData, type StepNode, type Tone, toneOf, tones } from './flow'

/** What the nodes need from the editor: names of prompts and destinations, problems to flag and live state. */
export type FlowInfo = {
  prompts: Map<number, Prompt>
  destLabel: (type: string, target: string) => string | undefined
  problems: Map<string, string>
  name: string
  phones?: string[]
  live?: IvrLive | null
}
export const FlowInfoContext = createContext<FlowInfo>({ prompts: new Map(), destLabel: () => undefined, problems: new Map(), name: '' })

export const kindIcon: Record<Kind, LucideIcon> = {
  menu: LayoutList, key: Grid3x3, audio: Volume2, speech: Mic, extension: PhoneForwarded, ringgroup: UsersRound, queue: ListOrdered, voicemail: Voicemail, ivr: GitBranch,
  external: Phone, time: Clock, repeat: Repeat, hangup: PhoneOff, timeout: Hourglass,
}

/** The CSS class that sets --tone for the block styles in index.css. */
export const toneClass = (t: Tone) => `kt-tone-${t}`

export function iconOf(d: StepData): LucideIcon {
  switch (d.type) {
    case 'entry':
      return PhoneIncoming
    case 'destination': {
      const k = (d.destination?.type || 'hangup') as Kind
      return kindIcon[k] ?? PhoneForwarded
    }
    default:
      return kindIcon[d.type as Kind]
  }
}

/** The small type label above the title, such as "Key press" or "Queue". */
export function kindLabel(d: StepData, t: (k: string) => string): string {
  if (d.type === 'destination') return t(`ivr.kind.${d.destination?.type || 'hangup'}`)
  return t(`ivr.kind.${d.type}`)
}

/** The title line of a node, also used by the side panel. */
export function NodeTitle({ d }: { d: StepData }) {
  const { t } = useTranslation()
  const info = useContext(FlowInfoContext)
  const tone = tones[toneOf(d)].text
  switch (d.type) {
    case 'entry':
      return <>{info.name || t('ivr.untitled')}</>
    case 'key':
      return (
        <>
          {t('ivr.node.press')} <span className={tone}>{d.digit}</span>
          {d.label && <> {t('ivr.node.for')} <span className={tone}>{d.label}</span></>}
        </>
      )
    case 'destination': {
      const dt = d.destination?.type || 'hangup'
      const raw = d.destination && dt !== 'hangup' && dt !== 'external' ? info.destLabel(dt, d.destination.target) : undefined
      return <>{d.label || (raw && splitLabel(raw).name) || t(`ivr.kind.${dt}`)}</>
    }
    default:
      return <>{d.label || t(`ivr.kind.${d.type}`)}</>
  }
}

/** An output port: a glossy sphere on the right edge. `tone` overrides the block colour. */
function Port({ id, tone, label }: { id: string; tone?: Tone; label?: string }) {
  return (
    <Handle type="source" position={Position.Right} id={id} aria-label={label}
      className="kt-port" style={tone ? ({ '--pt': tones[tone].stroke } as CSSProperties) : undefined} />
  )
}

/** Splits "201 Anna de Vries" into the number and the name. */
function splitLabel(label: string): { num: string; name: string } {
  const m = /^(\+?[\d*#]+)\s+(.*)$/.exec(label)
  return m ? { num: m[1], name: m[2] } : { num: '', name: label }
}
function initials(name: string): string {
  const w = name.split(/\s+/).filter((x) => /^\p{L}/u.test(x))
  if (!w.length) return ''
  return (w.length === 1 ? w[0].slice(0, 2) : w[0][0] + w[w.length - 1][0]).toUpperCase()
}

const stateTone: Record<string, string> = { available: 'bg-ok', inCall: 'bg-warn', ringing: 'bg-accent animate-pulse', offline: 'bg-subtle' }
function extState(s: string | undefined): string | undefined {
  switch (s) {
    case undefined:
    case '':
      return undefined
    case 'idle':
      return 'available'
    case 'ringing':
    case 'ringinuse':
      return 'ringing'
    case 'inuse':
    case 'busy':
    case 'onhold':
      return 'inCall'
    default:
      return 'offline'
  }
}

/** The audio of a step in a recessed well, with the prompt name underneath. */
function AudioWell({ d, tone }: { d: StepData; tone: string }) {
  const { t } = useTranslation()
  const info = useContext(FlowInfoContext)
  const prompt = d.promptId ? info.prompts.get(d.promptId) : undefined
  return (
    <div className="flex min-w-0 flex-col gap-1.5">
      <div className="kt-well flex min-h-10 items-center px-2.5 py-1" title={prompt?.name}>
        {d.promptId
          ? <div className="min-w-0 flex-1"><WavePlayer src={promptSrc(d.promptId, prompt?.durationMs)} tone={tone} label={prompt?.name} glossy /></div>
          : <span className="text-xs text-subtle">{t('ivr.node.noAudio')}</span>}
      </div>
    </div>
  )
}

/** Where a destination step sends the call, with who answers and their live state. */
function DestWell({ d }: { d: StepData }) {
  const { t } = useTranslation()
  const info = useContext(FlowInfoContext)
  const dest = d.destination
  const type = dest?.type || 'hangup'
  const Icon = iconOf(d)
  if (type === 'hangup') {
    return (
      <div className="kt-well flex min-h-10 items-center gap-2.5 px-2.5 py-1.5 text-[12.5px] text-ink-2">
        <span className="kt-avatar"><Icon className="size-3.5" aria-hidden /></span>{t('ivr.node.hangupText')}
      </div>
    )
  }
  const raw = dest ? info.destLabel(type, dest.target) ?? dest.target : ''
  if (!raw) {
    return <div className="kt-well flex min-h-10 items-center px-2.5 py-1.5 text-[12.5px] text-warn">{t('ivr.node.choose')}</div>
  }
  const { num, name } = type === 'external' ? { num: raw, name: '' } : splitLabel(raw)
  const live = dest ? info.live?.dests[`${type}:${dest.target}`] : undefined
  let sub: ReactNode = t(`ivr.kind.${type}`)
  if (type === 'extension') {
    const st = extState(live?.state)
    if (st) sub = <span className="inline-flex items-center gap-1.5"><span className={`size-1.5 rounded-full ${stateTone[st]}`} />{t(`ivr.node.state.${st}`)}</span>
  } else if (type === 'voicemail') {
    sub = t('ivr.node.mailbox')
  } else if (type === 'ringgroup' && live?.members) {
    sub = t('ivr.node.members', { count: live.members })
  } else if (type === 'queue' && live) {
    sub = live.online !== undefined
      ? <span className="inline-flex items-center gap-1.5"><span className={`size-1.5 rounded-full ${live.online ? 'bg-ok' : 'bg-subtle'}`} />{t('ivr.node.online', { online: live.online, waiting: live.waiting ?? 0 })}</span>
      : t('ivr.node.members', { count: live.members ?? 0 })
  }
  const ini = initials(name)
  return (
    <div className="kt-well flex min-h-10 items-center gap-2.5 px-2.5 py-1.5">
      <span className="kt-avatar">{ini || <Icon className="size-3.5" aria-hidden />}</span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[12.5px] font-semibold text-ink-2">
          {num && <span className="font-mono text-[11px] font-medium" style={{ color: 'color-mix(in srgb, var(--tone) 75%, var(--color-ink))' }}>{num} </span>}
          {name}
        </span>
        <span className="mt-0.5 block truncate text-[11px] text-subtle">{sub}</span>
      </span>
    </div>
  )
}

/** A row inside a block with its own output port (open/closed, speech answers). */
function PortRow({ id, tone, label, children }: { id: string; tone?: Tone; label?: string; children: ReactNode }) {
  return (
    <div className="kt-well relative flex min-h-8 items-center gap-2 px-2.5 text-[12px] text-ink-2">
      {children}
      <Port id={id} tone={tone} label={label} />
    </div>
  )
}

export function StepNodeView({ id, data, selected }: NodeProps<StepNode>) {
  const { t } = useTranslation()
  const info = useContext(FlowInfoContext)
  const toneKey = toneOf(data)
  const tone = tones[toneKey]
  const Icon = iconOf(data)
  const problem = info.problems.get(id)

  let body: ReactNode = null
  if (data.type === 'speech') {
    const intents = data.intents ?? []
    body = (
      <div className="flex flex-col gap-1.5">
        <AudioWell d={data} tone={tone.text} />
        {intents.map((it, i) => (
          <PortRow key={i} id={intentPort(i + 1)} label={it.label}>
            <span className={`font-mono text-[11px] ${tone.text}`}>“</span>
            <span className="min-w-0 flex-1 truncate">{it.label || <span className="text-warn">{t('ivr.node.intentUnnamed')}</span>}</span>
            <span className="max-w-[40%] truncate text-[11px] text-subtle">{it.keywords.slice(0, 2).join(', ')}</span>
          </PortRow>
        ))}
        <PortRow id="nomatch" tone="warn" label={t('ivr.node.notUnderstood')}>
          <span className="text-warn">{t('ivr.node.notUnderstood')}</span>
        </PortRow>
      </div>
    )
  } else if (data.type === 'entry') {
    body = (
      <div className="flex flex-col gap-1.5">
        {!!info.phones?.length && <span className="truncate px-0.5 font-mono text-[11px] text-muted" title={info.phones.join(', ')}>{info.phones.join(' · ')}</span>}
        <AudioWell d={data} tone={tone.text} />
      </div>
    )
  } else if (data.type === 'key' || data.type === 'timeout' || data.type === 'audio' || data.type === 'menu') {
    body = <AudioWell d={data} tone={tone.text} />
  } else if (data.type === 'destination') {
    body = <DestWell d={data} />
  } else if (data.type === 'time') {
    const target = data.destination?.target ?? ''
    const tcName = target ? info.destLabel('timecondition', target) : undefined
    const open = target ? info.live?.open[target] : undefined
    body = (
      <div className="flex flex-col gap-1.5">
        <div className="flex min-h-5 items-center justify-between gap-2 px-0.5 text-[11px] text-subtle">
          <span className="truncate">{tcName ?? <span className="text-warn">{t('ivr.node.choose')}</span>}</span>
          {open !== undefined && (
            <span className={`inline-flex flex-none items-center gap-1.5 rounded-md px-1.5 py-0.5 text-[10.5px] font-semibold ${open ? 'bg-ok/15 text-ok' : 'bg-warn/15 text-warn'}`}>
              <span className={`size-1.5 rounded-full ${open ? 'bg-ok glow-ok' : 'bg-warn'}`} />{t(open ? 'ivr.node.openNow' : 'ivr.node.closedNow')}
            </span>
          )}
        </div>
        <PortRow id="open" tone="accent2" label={t('ivr.node.open')}><span className="flex-1 first-letter:uppercase">{t('ivr.node.open')}</span></PortRow>
        <PortRow id="closed" tone="warn" label={t('ivr.node.closed')}><span className="flex-1 first-letter:uppercase">{t('ivr.node.closed')}</span></PortRow>
      </div>
    )
  } else if (data.type === 'repeat') {
    body = <div className="kt-well flex min-h-10 items-center px-2.5 py-1.5 text-[12px] text-ink-2">{t('ivr.node.repeatText')}</div>
  }

  const singleOut = data.type === 'entry' || data.type === 'menu' || data.type === 'key' || data.type === 'timeout' || data.type === 'audio'
  return (
    <div style={{ width: NODE_W }} className={`kt-node ${toneClass(toneKey)} ${selected ? 'is-selected' : ''}`}>
      {data.type !== 'entry' && <Handle type="target" position={Position.Left} id="in" className="kt-port" />}
      <div className="flex items-center gap-2.5 px-3 pb-2 pt-2.5">
        <span className="kt-chip size-8 rounded-[10px]"><Icon className="size-4" aria-hidden /></span>
        <span className="grid min-w-0 flex-1 gap-0.5">
          <span className="kt-kind truncate">{kindLabel(data, t)}</span>
          <span className="truncate text-[13.5px] font-semibold text-ink"><NodeTitle d={data} /></span>
        </span>
        {problem && <span title={problem} className="inline-flex flex-none text-warn"><TriangleAlert className="size-4" aria-label={problem} /></span>}
        {data.type === 'key' && <span className="kt-keycap" aria-hidden>{data.digit}</span>}
      </div>
      <div className="px-3 pb-2.5">{body}</div>
      {singleOut && <Port id="out" />}
    </div>
  )
}

export const nodeTypes = { step: StepNodeView }
