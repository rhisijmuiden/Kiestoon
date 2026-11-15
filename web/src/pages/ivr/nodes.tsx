import { createContext, useContext, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import {
  Clock, GitBranch, Grid3x3, Hourglass, ListOrdered, Mic, Phone, PhoneForwarded, PhoneIncoming, PhoneOff, Repeat, TriangleAlert, UsersRound, Voicemail, Volume2,
  type LucideIcon,
} from 'lucide-react'
import { WavePlayer, promptSrc } from '../../components/WavePlayer'
import type { Prompt } from '../../lib/types'
import { NODE_W, intentPort, type Kind, type StepData, type StepNode, toneOf, tones } from './flow'

/** What the nodes need from the editor: names of prompts and destinations, and problems to flag. */
export type FlowInfo = {
  prompts: Map<number, Prompt>
  destLabel: (type: string, target: string) => string | undefined
  problems: Map<string, string>
  name: string
  phone?: string
}
export const FlowInfoContext = createContext<FlowInfo>({ prompts: new Map(), destLabel: () => undefined, problems: new Map(), name: '' })

export const kindIcon: Record<Kind, LucideIcon> = {
  key: Grid3x3, audio: Volume2, speech: Mic, extension: PhoneForwarded, ringgroup: UsersRound, queue: ListOrdered, voicemail: Voicemail, ivr: GitBranch,
  external: Phone, time: Clock, repeat: Repeat, hangup: PhoneOff, timeout: Hourglass,
}

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
    case 'destination':
      return <>{d.label || t(`ivr.kind.${d.destination?.type || 'hangup'}`)}</>
    default:
      return <>{d.label || t(`ivr.kind.${d.type}`)}</>
  }
}

function Port({ id, top, label, tone }: { id: string; top?: string; label?: string; tone: string }) {
  return (
    <>
      <Handle type="source" position={Position.Right} id={id} style={top ? { top } : undefined}
        className={`!size-3 !rounded-full !border-2 !border-surface ${tone}`} aria-label={label} />
      {label && <span className="pointer-events-none absolute -right-1 translate-x-full text-[10px] text-subtle" style={{ top: `calc(${top} - 18px)` }}>{label}</span>}
    </>
  )
}

export function StepNodeView({ id, data, selected }: NodeProps<StepNode>) {
  const { t } = useTranslation()
  const info = useContext(FlowInfoContext)
  const tone = tones[toneOf(data)]
  const Icon = iconOf(data)
  const problem = info.problems.get(id)
  const prompt = data.promptId ? info.prompts.get(data.promptId) : undefined

  let body: ReactNode = null
  if (data.type === 'speech') {
    // One row per intent with its own port, then "not understood".
    const intents = data.intents ?? []
    body = (
      <div className="-mx-3 flex flex-col">
        <div className="px-3 pb-2">
          {data.promptId
            ? <WavePlayer src={promptSrc(data.promptId, prompt?.durationMs)} tone={tone.text} label={prompt?.name} />
            : <span className="text-xs text-subtle">{t('ivr.node.noAudio')}</span>}
        </div>
        {intents.map((it, i) => (
          <div key={i} className="relative flex min-h-8 items-center gap-2 border-t border-line px-3 text-[12.5px]">
            <span className={`font-mono text-[11px] ${tone.text}`}>“</span>
            <span className="min-w-0 flex-1 truncate text-ink-2">{it.label || <span className="text-warn">{t('ivr.node.intentUnnamed')}</span>}</span>
            <span className="truncate text-[11px] text-subtle">{it.keywords.slice(0, 2).join(', ')}</span>
            <Handle type="source" position={Position.Right} id={intentPort(i + 1)} className={`!size-3 !rounded-full !border-2 !border-surface ${tone.dot}`} aria-label={it.label} />
          </div>
        ))}
        <div className="relative flex min-h-8 items-center border-t border-line px-3 text-[12.5px] text-warn">
          {t('ivr.node.notUnderstood')}
          <Handle type="source" position={Position.Right} id="nomatch" className={`!size-3 !rounded-full !border-2 !border-surface ${tones.warn.dot}`} aria-label={t('ivr.node.notUnderstood')} />
        </div>
      </div>
    )
  } else if (data.type === 'entry' || data.type === 'key' || data.type === 'timeout' || data.type === 'audio') {
    body = data.promptId
      ? <WavePlayer src={promptSrc(data.promptId, prompt?.durationMs)} tone={tone.text} label={prompt?.name} />
      : <span className="text-xs text-subtle">{t('ivr.node.noAudio')}</span>
  } else if (data.type === 'destination' || data.type === 'time') {
    const d = data.destination
    const label = d && d.type !== 'hangup' ? info.destLabel(d.type, d.target) ?? d.target : ''
    body = (
      <span className="flex min-w-0 items-center gap-2 text-[13px] text-ink-2">
        <span className={`inline-flex size-6 flex-none items-center justify-center rounded-md ${tone.bg} ${tone.text}`}><Icon className="size-3.5" aria-hidden /></span>
        <span className="truncate">{d?.type === 'hangup' ? t('ivr.node.hangupText') : label || <span className="text-warn">{t('ivr.node.choose')}</span>}</span>
      </span>
    )
  } else if (data.type === 'repeat') {
    body = <span className="text-[13px] text-ink-2">{t('ivr.node.repeatText')}</span>
  }

  return (
    <div style={{ width: NODE_W }}
      className={`group relative rounded-2xl border-[1.5px] bg-surface text-left shadow-[0_10px_30px_-18px_rgb(0_0_0_/_0.6)] transition-shadow ${selected ? tone.ring : tone.border}`}>
      {data.type !== 'entry' && <Handle type="target" position={Position.Left} id="in" className={`!size-3 !rounded-full !border-2 !border-surface ${tone.dot}`} />}
      <div className="flex h-11 items-center gap-2 border-b border-line px-3">
        <Icon className={`size-4 flex-none ${tone.text}`} aria-hidden />
        <span className="min-w-0 flex-1 truncate text-[13px] font-semibold text-ink"><NodeTitle d={data} /></span>
        {problem && <span title={problem} className="inline-flex text-warn"><TriangleAlert className="size-4" aria-label={problem} /></span>}
      </div>
      <div className={`flex min-h-11 items-center px-3 ${data.type === 'speech' ? 'pt-2' : 'py-2'}`}>
        {data.type === 'entry' && info.phone ? (
          <div className="flex min-w-0 flex-1 flex-col gap-1.5">
            <span className="font-mono text-[11px] text-muted">{info.phone}</span>
            {body}
          </div>
        ) : <div className="min-w-0 flex-1">{body}</div>}
      </div>
      {data.type === 'time' ? (
        <>
          <Port id="open" top="38%" label={t('ivr.node.open')} tone={tones.accent2.dot} />
          <Port id="closed" top="78%" label={t('ivr.node.closed')} tone={tones.warn.dot} />
        </>
      ) : (data.type === 'entry' || data.type === 'key' || data.type === 'timeout' || data.type === 'audio') && <Port id="out" tone={tone.dot} />}
    </div>
  )
}

export const nodeTypes = { step: StepNodeView }
