import type { Edge, Node } from '@xyflow/react'
import type { DestType, Destination, Flow, FlowNode, FlowNodeType, FlowPort, IVR } from '../../lib/types'

// The canvas works with React Flow nodes and edges; the server stores a plain Flow. These helpers
// convert between the two and hold the rules for which lines are allowed (the server checks the same).

export type StepData = Omit<FlowNode, 'id' | 'x' | 'y'> & { [k: string]: unknown }
export type StepNode = Node<StepData, 'step'>

export const KEYS = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '0', '*', '#']

export type Tone = 'accent' | 'accent2' | 'ok' | 'warn' | 'danger'

/** Tailwind classes per colour, written out so the build keeps them. */
export const tones: Record<Tone, { text: string; border: string; ring: string; bg: string; dot: string; stroke: string }> = {
  accent: { text: 'text-accent', border: 'border-accent/55', ring: 'shadow-[0_0_0_4px_rgb(34_211_238_/_0.18),0_0_36px_-10px_var(--color-accent)] border-accent', bg: 'bg-accent/12', dot: '!bg-accent', stroke: 'var(--color-accent)' },
  accent2: { text: 'text-accent-2', border: 'border-accent-2/55', ring: 'shadow-[0_0_0_4px_rgb(129_140_248_/_0.2),0_0_36px_-10px_var(--color-accent-2)] border-accent-2', bg: 'bg-accent-2/12', dot: '!bg-accent-2', stroke: 'var(--color-accent-2)' },
  ok: { text: 'text-ok', border: 'border-ok/55', ring: 'shadow-[0_0_0_4px_rgb(52_211_153_/_0.18),0_0_36px_-10px_var(--color-ok)] border-ok', bg: 'bg-ok/12', dot: '!bg-ok', stroke: 'var(--color-ok)' },
  warn: { text: 'text-warn', border: 'border-warn/55', ring: 'shadow-[0_0_0_4px_rgb(251_191_36_/_0.18),0_0_36px_-10px_var(--color-warn)] border-warn', bg: 'bg-warn/12', dot: '!bg-warn', stroke: 'var(--color-warn)' },
  danger: { text: 'text-danger', border: 'border-danger/55', ring: 'shadow-[0_0_0_4px_rgb(248_113_113_/_0.18),0_0_36px_-10px_var(--color-danger)] border-danger', bg: 'bg-danger/12', dot: '!bg-danger', stroke: 'var(--color-danger)' },
}

export function toneOf(d: StepData): Tone {
  switch (d.type) {
    case 'entry':
    case 'time':
      return 'accent2'
    case 'key':
    case 'repeat':
    case 'speech':
      return 'accent'
    case 'audio':
      return 'ok'
    case 'timeout':
      return 'warn'
    case 'destination':
      return d.destination?.type === 'hangup' ? 'danger' : 'warn'
  }
}

/** Which node types can hold a voice prompt. */
export const hasAudio = (t: FlowNodeType) => t === 'entry' || t === 'key' || t === 'timeout' || t === 'audio' || t === 'speech'

/** The port of intent i (from 1) of a speech step. */
export const intentPort = (i: number): FlowPort => `i${i}`

/** The output ports of a node type. */
export function portsOf(t: FlowNodeType, intents = 0): FlowPort[] {
  switch (t) {
    case 'speech':
      return [...Array.from({ length: intents }, (_, i) => intentPort(i + 1)), 'nomatch']
    case 'entry':
    case 'key':
    case 'timeout':
    case 'audio':
      return ['out']
    case 'time':
      return ['open', 'closed']
    default:
      return []
  }
}

/** Whether a line from a node to another is allowed. */
export function canConnect(from: StepData | undefined, to: StepData | undefined): boolean {
  if (!from || !to || to.type === 'entry') return false
  if (from.type === 'entry') return to.type === 'key' || to.type === 'timeout'
  if (from.type === 'key' || from.type === 'timeout' || from.type === 'audio' || from.type === 'time' || from.type === 'speech') return to.type !== 'key' && to.type !== 'timeout'
  return false
}

/** The building blocks offered in the palette. */
export type Kind = 'key' | 'audio' | 'speech' | 'extension' | 'ringgroup' | 'queue' | 'voicemail' | 'ivr' | 'external' | 'time' | 'repeat' | 'hangup' | 'timeout'
export const kinds: Kind[] = ['key', 'audio', 'speech', 'extension', 'ringgroup', 'queue', 'voicemail', 'time', 'ivr', 'external', 'repeat', 'hangup', 'timeout']

export function kindTone(k: Kind): Tone {
  switch (k) {
    case 'key':
    case 'repeat':
    case 'speech':
      return 'accent'
    case 'audio':
      return 'ok'
    case 'time':
    case 'ivr':
      return 'accent2'
    case 'hangup':
      return 'danger'
    default:
      return 'warn'
  }
}

export function dataForKind(k: Kind, usedDigits: string[]): StepData {
  switch (k) {
    case 'key':
      return { type: 'key', digit: KEYS.find((d) => !usedDigits.includes(d)) ?? '1', label: '' }
    case 'audio':
      return { type: 'audio', label: '' }
    case 'time':
      return { type: 'time', destination: { type: 'timecondition', target: '' } }
    case 'repeat':
      return { type: 'repeat' }
    case 'speech':
      return { type: 'speech', label: '', intents: [], ai: false }
    case 'timeout':
      return { type: 'timeout' }
    case 'hangup':
      return { type: 'destination', destination: { type: 'hangup', target: '' } }
    default:
      return { type: 'destination', destination: { type: k as DestType, target: '' } }
  }
}

let seq = 0
export const newId = () => `n${Date.now().toString(36)}${(seq++).toString(36)}`

export function fromFlow(f: Flow): { nodes: StepNode[]; edges: Edge[] } {
  const nodes: StepNode[] = f.nodes.map((n) => {
    const { id, x, y, ...data } = n
    return { id, type: 'step', position: { x, y }, data: data as StepData, deletable: n.type !== 'entry' }
  })
  const byId = new Map(nodes.map((n) => [n.id, n]))
  const edges = f.edges.filter((e) => byId.has(e.from) && byId.has(e.to)).map((e) => makeEdge(e.from, e.port, e.to, byId.get(e.from)!.data))
  return { nodes, edges }
}

export function makeEdge(from: string, port: FlowPort, to: string, src: StepData): Edge {
  const tone = port === 'closed' || port === 'nomatch' ? 'warn' : toneOf(src)
  return {
    id: `${from}:${port}:${to}`, source: from, sourceHandle: port, target: to, targetHandle: 'in',
    type: 'smoothstep', pathOptions: { borderRadius: 14 },
    style: { stroke: tones[tone].stroke, strokeWidth: 2 },
  } as Edge
}

export function toFlow(nodes: StepNode[], edges: Edge[]): Flow {
  return {
    nodes: nodes.map((n) => {
      const d = n.data
      const out: FlowNode = { id: n.id, type: d.type, x: Math.round(n.position.x), y: Math.round(n.position.y) }
      if (d.digit) out.digit = d.digit
      if (d.label) out.label = d.label
      if (d.promptId) out.promptId = d.promptId
      if (d.destination) out.destination = d.destination
      if (d.type === 'speech') {
        out.intents = (d.intents ?? []).map((i) => ({ label: i.label.trim(), keywords: i.keywords.map((k) => k.trim()).filter(Boolean) }))
        if (d.ai) out.ai = true
      }
      return out
    }),
    edges: edges.map((e) => ({ from: e.source, port: (e.sourceHandle ?? 'out') as FlowPort, to: e.target })),
  }
}

/** A new menu: the entry, key 1 and the timeout branch. */
export function blankFlow(): Flow {
  return {
    nodes: [
      { id: 'entry', type: 'entry', x: 0, y: 80 },
      { id: 'k1', type: 'key', x: 340, y: 0, digit: '1' },
      { id: 'timeout', type: 'timeout', x: 340, y: 160 },
      { id: 'hangup', type: 'destination', x: 680, y: 160, destination: { type: 'hangup', target: '' } },
    ],
    edges: [
      { from: 'entry', port: 'out', to: 'k1' },
      { from: 'entry', port: 'out', to: 'timeout' },
      { from: 'timeout', port: 'out', to: 'hangup' },
    ],
  }
}

export const blankIVR = (): IVR => ({
  id: 0, name: '', promptId: 0, timeout: 5, retries: 2, directDial: false, options: [], timeoutDestination: { type: 'hangup', target: '' }, flow: blankFlow(),
})

export const NODE_W = 260
const COL = NODE_W + 90
const ROW = 128

/**
 * Tidy layout: columns by distance from the entry, rows in key order. Nodes nobody reaches go in a
 * last column.
 */
export function autoLayout(nodes: StepNode[], edges: Edge[]): StepNode[] {
  const out = new Map<string, string[]>()
  for (const e of edges) out.set(e.source, [...(out.get(e.source) ?? []), e.target])
  const byId = new Map(nodes.map((n) => [n.id, n]))
  const rank = (id: string) => {
    const d = byId.get(id)?.data
    if (d?.type === 'key') return KEYS.indexOf(d.digit ?? '')
    if (d?.type === 'timeout') return 99
    return 50
  }
  const depth = new Map<string, number>()
  const entry = nodes.find((n) => n.data.type === 'entry')
  const order: string[] = []
  if (entry) {
    depth.set(entry.id, 0)
    const queue = [entry.id]
    while (queue.length) {
      const id = queue.shift()!
      order.push(id)
      const next = [...(out.get(id) ?? [])].sort((a, b) => rank(a) - rank(b))
      for (const n of next) {
        if (!depth.has(n)) {
          depth.set(n, depth.get(id)! + 1)
          queue.push(n)
        }
      }
    }
  }
  const maxDepth = Math.max(0, ...depth.values())
  for (const n of nodes) if (!depth.has(n.id)) { depth.set(n.id, maxDepth + 1); order.push(n.id) }
  const cols = new Map<number, string[]>()
  for (const id of order) cols.set(depth.get(id)!, [...(cols.get(depth.get(id)!) ?? []), id])
  const tallest = Math.max(...[...cols.values()].map((c) => c.length))
  const pos = new Map<string, { x: number; y: number }>()
  for (const [col, ids] of cols) {
    const offset = ((tallest - ids.length) * ROW) / 2
    ids.forEach((id, i) => pos.set(id, { x: col * COL, y: offset + i * ROW }))
  }
  return nodes.map((n) => ({ ...n, position: pos.get(n.id) ?? n.position }))
}

export function destinationKind(d: Destination | undefined): string {
  return d?.type || 'hangup'
}
