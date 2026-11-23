import { useCallback, useEffect, useMemo, useRef, useState, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Background, BackgroundVariant, ConnectionLineType, Controls, MiniMap, ReactFlow, ReactFlowProvider, useEdgesState, useNodesState, useReactFlow,
  type Connection, type Edge, type EdgeChange, type NodeChange,
} from '@xyflow/react'
import '@xyflow/react/dist/base.css'
import { Activity, ArrowLeft, ClipboardPaste, Copy, CopyPlus, GitFork, LayoutGrid, PhoneCall, Redo2, Spline, Trash2, Undo2, Waypoints } from 'lucide-react'
import { Badge, Modal, Select, useApi } from '../../components/kit'
import { resetDestinations, useDestinations, useSaver } from '../../components/pbx'
import { Button, ErrorMessage } from '../../components/ui'
import { api } from '../../lib/api'
import { useSession } from '../../lib/session'
import { useTheme } from '../../lib/theme'
import type { Extension, InboundRoute, IVR, IvrLive, Prompt, TimeCondition } from '../../lib/types'
import { NodePanel } from './NodePanel'
import { branchOf, makeClip, pasteClip, readClip, writeClip, type Clip } from './clipboard'
import {
  MAX_PARALLEL, NODE_W, autoLayout, blankFlow, canConnect, canRingTogether, dataForKind, isMenu, fromFlow, intentPort, kindTone, makeEdge, newId, toFlow, toneOf, tones,
  type Kind, type StepData, type StepNode,
} from './flow'
import { LineStyleContext, edgeTypes, type LineStyle } from './edges'
import { FlowInfoContext, kindIcon, nodeTypes, toneClass, type FlowInfo } from './nodes'

type Snapshot = { nodes: StepNode[]; edges: Edge[] }
const DRAG_TYPE = 'application/x-kiestoon-step'

/** The building blocks in the palette, grouped as a menu designer thinks about them. */
const GROUPS: { key: string; kinds: Kind[] }[] = [
  { key: 'menu', kinds: ['menu', 'key', 'timeout', 'repeat'] },
  { key: 'audio', kinds: ['audio', 'speech'] },
  { key: 'route', kinds: ['extension', 'ringgroup', 'queue', 'voicemail', 'ivr', 'external', 'hangup'] },
  { key: 'logic', kinds: ['time'] },
]

/** A per-browser view preference (line style, call flow animation). */
function usePref<T extends string>(key: string, initial: T): [T, (v: T) => void] {
  const [v, setV] = useState<T>(() => {
    try {
      return (localStorage.getItem(key) as T | null) ?? initial
    } catch {
      return initial
    }
  })
  const set = (x: T) => {
    setV(x)
    try {
      localStorage.setItem(key, x)
    } catch { /* private mode */ }
  }
  return [v, set]
}

export function FlowEditor(props: { value: IVR; onClose: () => void; onSaved: (v: IVR) => void }) {
  return (
    <ReactFlowProvider>
      <Editor {...props} />
    </ReactFlowProvider>
  )
}

function Editor({ value, onClose, onSaved }: { value: IVR; onClose: () => void; onSaved: (v: IVR) => void }) {
  const { t } = useTranslation()
  const { theme } = useTheme()
  const rf = useReactFlow()
  const prompts = useApi<Prompt[]>('/pbx/prompts')
  const tcs = useApi<TimeCondition[]>('/pbx/timeconditions')
  const inbound = useApi<InboundRoute[]>('/pbx/inbound')
  const dests = useDestinations()
  const live = useApi<IvrLive>('/pbx/ivrs/live', 15000)
  const [lineStyle, setLineStyle] = usePref<LineStyle>('kt-ivr-lines', 'curve')
  const [motion, setMotion] = usePref<'on' | 'off'>('kt-ivr-motion', 'on')

  const initial = useMemo(() => fromFlow(value.flow ?? blankFlow()), [value])
  const [ivr, setIvr] = useState<IVR>(value)
  const [nodes, setNodes, onNodesChangeBase] = useNodesState<StepNode>(initial.nodes)
  const [edges, setEdges, onEdgesChangeBase] = useEdgesState<Edge>(initial.edges)
  const [selected, setSelected] = useState<string | null>(() => initial.nodes.find((n) => n.data.type === 'entry')?.id ?? null)
  const [dirty, setDirty] = useState(!value.id)
  const [past, setPast] = useState<Snapshot[]>([])
  const [future, setFuture] = useState<Snapshot[]>([])
  const [testing, setTesting] = useState(false)
  const [clip, setClip] = useState<Clip | null>(() => readClip())
  const [notice, setNotice] = useState('')
  const [menu, setMenu] = useState<{ x: number; y: number; node?: string; at: { x: number; y: number } } | null>(null)
  const lastCommit = useRef(0)
  const wrapper = useRef<HTMLDivElement>(null)

  const { busy, error, save } = useSaver<IVR>('/pbx/ivrs', (v) => {
    setIvr(v)
    setDirty(false)
    resetDestinations()
    onSaved(v)
  })

  /** Remembers the current state for undo. Rapid changes (typing) count as one step. */
  const commit = useCallback((coalesce = false) => {
    const now = Date.now()
    if (coalesce && now - lastCommit.current < 1000) {
      lastCommit.current = now
      return
    }
    lastCommit.current = now
    setPast((p) => [...p.slice(-49), { nodes, edges }])
    setFuture([])
    setDirty(true)
  }, [nodes, edges])

  function undo() {
    const prev = past.at(-1)
    if (!prev) return
    setFuture((f) => [{ nodes, edges }, ...f])
    setPast((p) => p.slice(0, -1))
    setNodes(prev.nodes)
    setEdges(prev.edges)
    setDirty(true)
  }
  function redo() {
    const next = future[0]
    if (!next) return
    setPast((p) => [...p, { nodes, edges }])
    setFuture((f) => f.slice(1))
    setNodes(next.nodes)
    setEdges(next.edges)
    setDirty(true)
  }
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement
      if (el.closest('input, textarea, select, [contenteditable]')) return
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'z') {
        e.preventDefault()
        if (e.shiftKey) redo()
        else undo()
      }
      if (!wrapper.current?.closest('body')) return
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'c' && !window.getSelection()?.toString()) {
        if (copySelection()) e.preventDefault()
      }
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'v') {
        e.preventDefault()
        paste(selected ? byId.get(selected) : undefined)
      }
      if (e.key === 'Escape') setMenu(null)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })
  useEffect(() => {
    if (!dirty) return
    const warn = (e: BeforeUnloadEvent) => e.preventDefault()
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [dirty])

  const onNodesChange = useCallback((ch: NodeChange<StepNode>[]) => {
    if (ch.some((c) => c.type === 'remove')) commit()
    if (ch.some((c) => c.type === 'position' && c.dragging)) commit(true)
    for (const c of ch) if (c.type === 'remove' && c.id === selected) setSelected(null)
    onNodesChangeBase(ch)
  }, [commit, onNodesChangeBase, selected])
  const onEdgesChange = useCallback((ch: EdgeChange<Edge>[]) => {
    if (ch.some((c) => c.type === 'remove')) commit()
    onEdgesChangeBase(ch)
  }, [commit, onEdgesChangeBase])

  const byId = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes])
  const isValidConnection = useCallback((c: Connection | Edge) => c.source !== c.target && canConnect(byId.get(c.source)?.data, byId.get(c.target)?.data), [byId])
  const onConnect = useCallback((c: Connection) => {
    const src = byId.get(c.source)
    const dst = byId.get(c.target)
    if (!src || !dst || !isValidConnection(c)) return
    commit()
    const port = (c.sourceHandle ?? 'out') as 'out' | 'open' | 'closed'
    setEdges((es) => {
      const others = es.filter((e) => !(e.source === c.source && e.target === c.target))
      let rest: Edge[]
      if (isMenu(src.data)) {
        // A menu has a line to every key and its timeout; a key or timeout is in one menu only.
        // The entry has at most one start step (for example a time check).
        rest = dst.data.type === 'key' || dst.data.type === 'timeout'
          ? others.filter((e) => !(e.target === c.target && isMenu(byId.get(e.source)?.data)))
          : others.filter((e) => !(e.source === c.source && byId.get(e.target)?.data.type !== 'key' && byId.get(e.target)?.data.type !== 'timeout'))
        if (dst.data.type === 'timeout') rest = rest.filter((e) => !(e.source === c.source && byId.get(e.target)?.data.type === 'timeout'))
      } else {
        // One line per port, except destinations that ring at the same time.
        const same = others.filter((e) => e.source === c.source && (e.sourceHandle ?? 'out') === port)
        const together = canRingTogether(dst.data) && same.length < MAX_PARALLEL && same.every((e) => canRingTogether(byId.get(e.target)?.data))
        rest = together ? others : others.filter((e) => !same.includes(e))
      }
      return [...rest, makeEdge(c.source, port, c.target, src.data)]
    })
  }, [byId, commit, isValidConnection, setEdges])

  const entryId = nodes.find((n) => n.data.type === 'entry')?.id
  // The menu (entry or menu step) a key or timeout belongs to.
  const menuOf = (id: string) => edges.find((e) => e.target === id && isMenu(byId.get(e.source)?.data))?.source
  const keysOf = (menu: string | undefined) => edges.filter((e) => e.source === menu && byId.get(e.target)?.data.type === 'key').map((e) => byId.get(e.target)!)
  // New keys go into the selected menu (or the menu of the selected key), else the entry.
  const selData = selected ? byId.get(selected)?.data : undefined
  const targetMenu = selected && isMenu(selData) ? selected : selected && (selData?.type === 'key' || selData?.type === 'timeout') ? menuOf(selected) ?? entryId : entryId
  const usedDigits = keysOf(targetMenu).map((n) => n.data.digit ?? '')
  const hasTimeout = edges.some((e) => e.source === targetMenu && byId.get(e.target)?.data.type === 'timeout')

  function addNode(kind: Kind, pos?: { x: number; y: number }) {
    if (kind === 'timeout' && hasTimeout) return
    if (kind === 'key' && usedDigits.length >= 12) return
    commit()
    let position = pos
    if (!position) {
      const r = wrapper.current?.getBoundingClientRect()
      const c = r ? rf.screenToFlowPosition({ x: r.left + r.width / 2, y: r.top + r.height / 2 }) : { x: 0, y: 0 }
      position = { x: c.x - NODE_W / 2 + (nodes.length % 5) * 18, y: c.y - 44 + (nodes.length % 5) * 18 }
    }
    const data = dataForKind(kind, usedDigits)
    const id = newId()
    setNodes((ns) => [...ns.map((n) => ({ ...n, selected: false })), { id, type: 'step', position, data, selected: true }])
    if ((kind === 'key' || kind === 'timeout') && targetMenu) {
      const menu = byId.get(targetMenu)!
      setEdges((es) => [...es, makeEdge(targetMenu, 'out', id, menu.data)])
    }
    setSelected(id)
  }

  function onDrop(e: DragEvent) {
    const kind = e.dataTransfer.getData(DRAG_TYPE) as Kind
    if (!kind) return
    e.preventDefault()
    const p = rf.screenToFlowPosition({ x: e.clientX, y: e.clientY })
    addNode(kind, { x: p.x - NODE_W / 2, y: p.y - 30 })
  }

  function changeData(id: string, d: Partial<StepData>) {
    commit(true)
    setNodes((ns) => ns.map((n) => (n.id === id ? { ...n, data: { ...n.data, ...d } } : n)))
    // Lines take the colour of their start node.
    if (d.destination) setEdges((es) => es.map((e) => (e.source === id ? makeEdge(e.source, (e.sourceHandle ?? 'out') as 'out', e.target, { ...byId.get(id)!.data, ...d }) : e)))
  }
  // Removing intent i (from 1) of a speech step drops its line and renumbers the later ports.
  function removeIntent(id: string, i: number) {
    const n = byId.get(id)
    if (!n) return
    commit()
    const intents = (n.data.intents ?? []).filter((_, j) => j !== i - 1)
    const data = { ...n.data, intents }
    setNodes((ns) => ns.map((x) => (x.id === id ? { ...x, data } : x)))
    setEdges((es) => es.flatMap((e) => {
      if (e.source !== id || !e.sourceHandle?.startsWith('i')) return [e]
      const k = Number(e.sourceHandle.slice(1))
      if (k === i) return []
      return [k > i ? makeEdge(id, intentPort(k - 1), e.target, data) : e]
    }))
  }
  function remove(id: string) {
    commit()
    setNodes((ns) => ns.filter((n) => n.id !== id))
    setEdges((es) => es.filter((e) => e.source !== id && e.target !== id))
    setSelected(null)
  }
  function duplicate(id: string) {
    const n = byId.get(id)
    if (!n) return
    commit()
    const data = { ...n.data }
    const menu = data.type === 'key' ? menuOf(id) ?? entryId : undefined
    const taken = keysOf(menu).map((k) => k.data.digit ?? '')
    if (data.type === 'key') data.digit = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '0', '*', '#'].find((k) => !taken.includes(k))
    if (data.type === 'key' && !data.digit) return
    const nid = newId()
    setNodes((ns) => [...ns.map((x) => ({ ...x, selected: false })), { id: nid, type: 'step', position: { x: n.position.x + 30, y: n.position.y + 120 }, data, selected: true }])
    if (data.type === 'key' && menu) setEdges((es) => [...es, makeEdge(menu, 'out', nid, byId.get(menu)!.data)])
    setSelected(nid)
  }
  function flash(msg: string) {
    setNotice(msg)
    window.setTimeout(() => setNotice((m) => (m === msg ? '' : m)), 2500)
  }
  /** Copies the selected steps (several with Ctrl/Shift-click or a selection box); true when something was copied. */
  function copySelection(only?: string) {
    const picked = nodes.filter((n) => n.selected).map((n) => n.id)
    const ids = only && !picked.includes(only) ? [only] : picked.length ? picked : selected ? [selected] : []
    return copyIds(ids)
  }
  function copyIds(ids: string[]) {
    const c = makeClip(ids, nodes, edges)
    if (!c.nodes.length) return false
    writeClip(c)
    setClip(c)
    flash(t('ivr.clip.copied', { count: c.nodes.length }))
    return true
  }
  function copyBranch(id: string) {
    copyIds(branchOf(id, nodes, edges))
  }
  function paste(target?: StepNode, at?: { x: number; y: number }) {
    const c = readClip() ?? clip
    if (!c?.nodes.length) return
    commit()
    const add = pasteClip(c, target, nodes, edges, at)
    setNodes((ns) => [...ns.map((n) => ({ ...n, selected: false })), ...add.nodes])
    setEdges((es) => [...es, ...add.edges])
    setSelected(add.nodes[0]?.id ?? null)
    flash(t('ivr.clip.pasted', { count: add.nodes.length }))
  }
  function removeMany(ids: string[]) {
    const del = new Set(ids.filter((id) => byId.get(id)?.data.type !== 'entry'))
    if (!del.size) return
    commit()
    setNodes((ns) => ns.filter((n) => !del.has(n.id)))
    setEdges((es) => es.filter((e) => !del.has(e.source) && !del.has(e.target)))
    setSelected(null)
  }
  function tidy() {
    commit()
    setNodes((ns) => autoLayout(ns, edges))
    window.requestAnimationFrame(() => rf.fitView({ padding: 0.15, duration: 300 }))
  }

  // Problems shown on the nodes (the menu still saves; the server only refuses invalid lines).
  const problems = useMemo(() => {
    const m = new Map<string, string>()
    const from = new Map<string, Edge[]>()
    const into = new Set<string>()
    for (const e of edges) {
      from.set(e.source, [...(from.get(e.source) ?? []), e])
      into.add(e.target)
    }
    const startStep = edges.some((e) => e.source === entryId && byId.get(e.target)?.data.type !== 'key' && byId.get(e.target)?.data.type !== 'timeout')
    for (const n of nodes) {
      const d = n.data
      const parent = edges.find((e) => e.target === n.id && isMenu(byId.get(e.source)?.data))?.source
      if ((d.type === 'key' || d.type === 'timeout') && !parent) m.set(n.id, t('ivr.problem.notInMenu'))
      else if ((d.type === 'key' || d.type === 'timeout') && parent === entryId && startStep) m.set(n.id, t('ivr.problem.skippedByStart'))
      else if (d.type !== 'entry' && d.type !== 'key' && d.type !== 'timeout' && !into.has(n.id)) m.set(n.id, t('ivr.problem.unreachable'))
      else if ((d.type === 'key' || d.type === 'audio' || d.type === 'timeout') && !from.has(n.id)) m.set(n.id, t('ivr.problem.noLine'))
      else if (d.type === 'menu' && !(from.get(n.id) ?? []).some((e) => byId.get(e.target)?.data.type === 'key')) m.set(n.id, t('ivr.problem.noKeys'))
      else if (d.type === 'time' && !d.destination?.target) m.set(n.id, t('ivr.problem.chooseTime'))
      else if (d.type === 'speech' && !(d.intents ?? []).length) m.set(n.id, t('ivr.problem.noIntents'))
      else if (d.type === 'speech' && (d.intents ?? []).some((i) => !i.label.trim() || !i.keywords.some((k) => k.trim()))) m.set(n.id, t('ivr.problem.intentIncomplete'))
      else if (d.type === 'destination' && d.destination?.type !== 'hangup' && !d.destination?.target) m.set(n.id, t('ivr.problem.chooseTarget'))
      else if (d.type === 'destination' && !canRingTogether(d) && edges.some((e) => e.target === n.id && edges.some((o) => o !== e && o.source === e.source && o.sourceHandle === e.sourceHandle && !isMenu(byId.get(e.source)?.data)))) m.set(n.id, t('ivr.problem.cannotRingTogether'))
    }
    return m
  }, [nodes, edges, entryId, byId, t])

  const promptMap = useMemo(() => new Map((prompts.data ?? []).map((p) => [p.id, p])), [prompts.data])
  const phones = useMemo(() => (ivr.id ? (inbound.data ?? []).filter((r) => r.destinationType === 'ivr' && r.destination === String(ivr.id)).map((r) => r.did) : []), [ivr.id, inbound.data])
  const info: FlowInfo = useMemo(() => ({
    prompts: promptMap,
    destLabel: (type, target) => dests?.options.find((o) => o.type === type && o.target === target)?.label
      ?? (type === 'timecondition' ? tcs.data?.find((c) => String(c.id) === target)?.name : undefined),
    problems, name: ivr.name, phones, live: live.data,
  }), [promptMap, dests, tcs.data, problems, ivr.name, phones, live.data])

  const sel = selected ? byId.get(selected) : undefined

  function doSave() {
    save({ ...ivr, flow: { ...toFlow(nodes, edges), language: ivr.flow?.language } })
  }
  function back() {
    if (dirty && !window.confirm(t('ivr.leaveUnsaved'))) return
    onClose()
  }

  return (
    <FlowInfoContext.Provider value={info}>
      <div className="flex min-h-0 flex-col gap-3">
        {/* Top bar */}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-3">
            <Button variant="ghost" onClick={back}><ArrowLeft className="size-4" aria-hidden /> {t('common.back')}</Button>
            <h1 className="min-w-0 truncate font-display text-[24px] font-semibold tracking-tight">{ivr.name || t('ivr.untitled')}</h1>
            {dirty && <Badge tone="warn" dot>{t('ivr.unsaved')}</Badge>}
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <button type="button" onClick={undo} disabled={!past.length} aria-label={t('ivr.undo')} title={t('ivr.undo')}
              className="inline-flex size-10 items-center justify-center rounded-[10px] border border-line text-muted hover:bg-tint/5 hover:text-ink disabled:opacity-40"><Undo2 className="size-4" aria-hidden /></button>
            <button type="button" onClick={redo} disabled={!future.length} aria-label={t('ivr.redo')} title={t('ivr.redo')}
              className="inline-flex size-10 items-center justify-center rounded-[10px] border border-line text-muted hover:bg-tint/5 hover:text-ink disabled:opacity-40"><Redo2 className="size-4" aria-hidden /></button>
            <Button variant="ghost" onClick={tidy}><LayoutGrid className="size-4" aria-hidden />{t('ivr.tidy')}</Button>
            <Button variant="ghost" onClick={() => setTesting(true)} disabled={!ivr.id || dirty} title={!ivr.id || dirty ? t('ivr.test.saveFirst') : undefined}>
              <PhoneCall className="size-4" aria-hidden />{t('ivr.test.button')}
            </Button>
            <Button busy={busy} onClick={doSave} disabled={!ivr.name.trim()} title={!ivr.name.trim() ? t('ivr.nameFirst') : undefined}>{t('common.save')}</Button>
          </div>
        </div>
        <ErrorMessage error={error} />

        {/* Palette */}
        <div role="toolbar" aria-label={t('ivr.palette')} className="flex flex-wrap items-center gap-x-5 gap-y-2 rounded-2xl border border-line bg-surface-2 px-3 py-2.5">
          {GROUPS.map((g) => (
            <div key={g.key} className="flex flex-wrap items-center gap-1.5">
              <span className="mr-0.5 text-[10.5px] font-semibold uppercase tracking-[0.12em] text-subtle">{t(`ivr.group.${g.key}`)}</span>
              {g.kinds.map((k) => {
                const I = kindIcon[k]
                const disabled = (k === 'timeout' && hasTimeout) || (k === 'key' && usedDigits.length >= 12)
                return (
                  <button key={k} type="button" draggable={!disabled} disabled={disabled}
                    onDragStart={(e) => { e.dataTransfer.setData(DRAG_TYPE, k); e.dataTransfer.effectAllowed = 'move' }}
                    onClick={() => addNode(k)} title={t('ivr.paletteClick')}
                    className={`kt-tile ${toneClass(kindTone(k))} inline-flex h-9 cursor-grab items-center gap-2 rounded-[11px] border border-line pl-1 pr-3 text-[12.5px] text-ink-2 hover:text-ink active:cursor-grabbing disabled:cursor-not-allowed disabled:opacity-40`}>
                    <span className="kt-chip size-7 rounded-lg"><I className="size-[15px]" aria-hidden /></span>{t(`ivr.kind.${k}`)}
                  </button>
                )
              })}
            </div>
          ))}
          <div className="ml-auto flex items-center gap-1.5">
            <div role="group" aria-label={t('ivr.lines.label')} className="inline-flex gap-0.5 rounded-[10px] border border-line bg-field p-0.5">
              {([['curve', Spline], ['step', Waypoints]] as const).map(([v, I]) => (
                <button key={v} type="button" aria-pressed={lineStyle === v} onClick={() => setLineStyle(v)} title={t(`ivr.lines.${v}`)} aria-label={t(`ivr.lines.${v}`)}
                  className={`inline-flex size-7 items-center justify-center rounded-lg transition ${lineStyle === v ? 'bg-accent/15 text-ink ring-1 ring-accent/45' : 'text-muted hover:text-ink'}`}>
                  <I className="size-4" aria-hidden />
                </button>
              ))}
            </div>
            <button type="button" aria-pressed={motion === 'on'} onClick={() => setMotion(motion === 'on' ? 'off' : 'on')} title={t('ivr.motion')} aria-label={t('ivr.motion')}
              className={`inline-flex size-8 items-center justify-center rounded-[10px] border border-line transition ${motion === 'on' ? 'bg-accent/15 text-ink' : 'bg-field text-muted hover:text-ink'}`}>
              <Activity className="size-4" aria-hidden />
            </button>
          </div>
        </div>

        {/* Canvas + side panel */}
        <div className="relative flex h-[calc(100dvh-250px)] min-h-[560px] overflow-hidden rounded-2xl border border-line bg-bg">
          <LineStyleContext.Provider value={lineStyle}>
          <div ref={wrapper} className={`kt-flow kt-ivr min-w-0 flex-1 ${motion === 'off' ? 'no-motion' : ''}`} onDragOver={(e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move' }} onDrop={onDrop}>
            <ReactFlow<StepNode, Edge>
              nodes={nodes} edges={edges} nodeTypes={nodeTypes} edgeTypes={edgeTypes}
              onNodesChange={onNodesChange} onEdgesChange={onEdgesChange} onConnect={onConnect} isValidConnection={isValidConnection}
              onNodeClick={(_, n) => { setSelected(n.id); setMenu(null) }} onPaneClick={() => { setSelected(null); setMenu(null) }}
              onNodeContextMenu={(e, n) => {
                e.preventDefault()
                const r = wrapper.current!.getBoundingClientRect()
                setMenu({ x: e.clientX - r.left, y: e.clientY - r.top, node: n.id, at: rf.screenToFlowPosition({ x: e.clientX, y: e.clientY }) })
              }}
              onPaneContextMenu={(e) => {
                e.preventDefault()
                const r = wrapper.current!.getBoundingClientRect()
                setMenu({ x: e.clientX - r.left, y: e.clientY - r.top, at: rf.screenToFlowPosition({ x: e.clientX, y: e.clientY }) })
              }}
              onMoveStart={() => setMenu(null)}
              onNodeDragStop={() => setDirty(true)}
              colorMode={theme} fitView fitViewOptions={{ padding: 0.08, maxZoom: 1 }} minZoom={0.2} maxZoom={1.6}
              deleteKeyCode={['Backspace', 'Delete']} proOptions={{ hideAttribution: true }}
              defaultEdgeOptions={{ type: 'flow' }} connectionLineType={lineStyle === 'step' ? ConnectionLineType.SmoothStep : ConnectionLineType.Bezier}
              connectionLineStyle={{ stroke: 'var(--color-accent)', strokeWidth: 2.5, strokeDasharray: '6 5', strokeLinecap: 'round' }}
              ariaLabelConfig={{ 'node.a11yDescription.default': t('ivr.a11yNode') }}>
              <Background id="dots" variant={BackgroundVariant.Dots} gap={22} size={1.5} color="var(--kt-canvas-dot)" />
              <Controls showInteractive={false} position="bottom-right" />
              <MiniMap position="bottom-left" pannable zoomable nodeBorderRadius={8}
                nodeColor={(n) => tones[toneOf(n.data as StepData)].stroke} bgColor="transparent" maskColor="rgb(from var(--kt-canvas) r g b / 0.55)" />
            </ReactFlow>
            {notice && <div role="status" className="pointer-events-none absolute left-1/2 top-3 z-20 -translate-x-1/2 rounded-full border border-line bg-surface/95 px-3.5 py-1.5 text-xs font-medium text-ink shadow-lg backdrop-blur">{notice}</div>}
            {menu && (
              <StepMenu x={menu.x} y={menu.y} onClose={() => setMenu(null)} items={(() => {
                const n = menu.node ? byId.get(menu.node) : undefined
                const many = nodes.filter((x) => x.selected).length
                const items: MenuItem[] = []
                if (n) {
                  if (n.data.type !== 'entry') items.push({ icon: Copy, label: many > 1 && n.selected ? t('ivr.clip.copySelected', { count: many }) : t('ivr.clip.copy'), hint: 'Ctrl+C', run: () => copySelection(n.id) })
                  if (edges.some((e) => e.source === n.id)) items.push({ icon: GitFork, label: t('ivr.clip.copyBranch'), run: () => copyBranch(n.id) })
                  if (clip) items.push({ icon: ClipboardPaste, label: isMenu(n.data) ? t('ivr.clip.pasteInto') : t('ivr.clip.pasteAfter'), hint: 'Ctrl+V', run: () => paste(n) })
                  if (n.data.type !== 'entry' && n.data.type !== 'timeout') items.push({ icon: CopyPlus, label: t('ivr.panel.duplicate'), run: () => duplicate(n.id) })
                  if (n.data.type !== 'entry') items.push({ icon: Trash2, label: many > 1 && n.selected ? t('ivr.clip.deleteSelected', { count: many }) : t('common.delete'), danger: true, run: () => removeMany(many > 1 && n.selected ? nodes.filter((x) => x.selected).map((x) => x.id) : [n.id]) })
                } else {
                  items.push({ icon: ClipboardPaste, label: t('ivr.clip.pasteHere'), hint: 'Ctrl+V', disabled: !clip, run: () => paste(undefined, menu.at) })
                }
                return items
              })()} />
            )}
          </div>
          </LineStyleContext.Provider>
          {sel && (
            <div className="absolute inset-0 z-10 bg-surface-2 lg:static lg:inset-auto lg:w-[380px] lg:flex-none lg:border-l lg:border-line">
              <NodePanel node={sel} ivr={ivr} setIvr={(fn) => { setIvr(fn); setDirty(true) }}
                prompts={prompts.data ?? []} onPromptsChanged={prompts.reload} timeConditions={tcs.data ?? []} usedDigits={usedDigits}
                onChange={(d) => changeData(sel.id, d)} onRemoveIntent={(i) => removeIntent(sel.id, i)} onDelete={() => remove(sel.id)} onDuplicate={() => duplicate(sel.id)} onClose={() => setSelected(null)}
                onCopy={() => copySelection(sel.id)} onCopyBranch={edges.some((e) => e.source === sel.id) ? () => copyBranch(sel.id) : undefined}
                onPaste={clip ? () => paste(sel) : undefined} pasteLabel={isMenu(sel.data) ? t('ivr.clip.pasteInto') : t('ivr.clip.pasteAfter')}
                inbound={inbound.data ?? []} onInboundChanged={inbound.reload}
                onQueueSaved={live.reload} />
            </div>
          )}
        </div>
      </div>
      {testing && ivr.id > 0 && <TestCall ivr={ivr} onClose={() => setTesting(false)} />}
    </FlowInfoContext.Provider>
  )
}

/** Rings an extension and plays the saved menu when it is answered. */
function TestCall({ ivr, onClose }: { ivr: IVR; onClose: () => void }) {
  const { t } = useTranslation()
  const { state } = useSession()
  const me = state.kind === 'ready' ? state.me : null
  const exts = useApi<Extension[]>('/pbx/extensions')
  const mine = exts.data?.find((e) => e.id === me?.user.extensionId)?.number
  const [ext, setExt] = useState('')
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const chosen = ext || mine || exts.data?.[0]?.number || ''
  async function call() {
    setBusy(true)
    setError(null)
    try {
      await api('POST', `/pbx/ivrs/${ivr.id}/test`, { extension: chosen })
      setDone(true)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={t('ivr.test.title')} subtitle={t('ivr.test.subtitle')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.close')}</Button><Button busy={busy} disabled={!chosen} onClick={call}><PhoneCall className="size-4" aria-hidden />{t('ivr.test.call')}</Button></>}>
      <ErrorMessage error={error} />
      <Select name="extension" label={t('ivr.test.extension')} value={chosen} onChange={(e) => setExt(e.target.value)}>
        {(exts.data ?? []).map((e) => <option key={e.id} value={e.number}>{e.number} · {e.name}</option>)}
      </Select>
      {done && <p className="text-sm text-ok">{t('ivr.test.ringing', { ext: chosen })}</p>}
    </Modal>
  )
}

type MenuItem = { icon: typeof Copy; label: string; hint?: string; danger?: boolean; disabled?: boolean; run: () => void }

/** The right-click menu of the canvas. */
function StepMenu({ x, y, items, onClose }: { x: number; y: number; items: MenuItem[]; onClose: () => void }) {
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => {
    ref.current?.querySelector<HTMLButtonElement>('button:not([disabled])')?.focus()
    const away = (e: MouseEvent) => { if (!ref.current?.contains(e.target as Node)) onClose() }
    window.addEventListener('mousedown', away)
    return () => window.removeEventListener('mousedown', away)
  }, [onClose])
  if (!items.length) return null
  return (
    <div ref={ref} role="menu" style={{ left: x, top: y }}
      className="absolute z-30 min-w-[220px] overflow-hidden rounded-xl border border-line bg-surface/95 p-1 shadow-2xl backdrop-blur">
      {items.map((it) => (
        <button key={it.label} type="button" role="menuitem" disabled={it.disabled} onClick={() => { it.run(); onClose() }}
          className={`flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-[13px] disabled:opacity-40 ${it.danger ? 'text-danger hover:bg-danger/10' : 'text-ink hover:bg-tint/6'}`}>
          <it.icon className="size-4 flex-none opacity-80" aria-hidden />
          <span className="flex-1">{it.label}</span>
          {it.hint && <span className="font-mono text-[10.5px] text-subtle">{it.hint}</span>}
        </button>
      ))}
    </div>
  )
}
