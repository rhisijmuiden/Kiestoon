import { useCallback, useEffect, useMemo, useRef, useState, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Background, BackgroundVariant, Controls, MiniMap, ReactFlow, ReactFlowProvider, useEdgesState, useNodesState, useReactFlow,
  type Connection, type Edge, type EdgeChange, type NodeChange,
} from '@xyflow/react'
import '@xyflow/react/dist/base.css'
import { ArrowLeft, LayoutGrid, PhoneCall, Redo2, Undo2 } from 'lucide-react'
import { Badge, Modal, Select, useApi } from '../../components/kit'
import { resetDestinations, useDestinations, useSaver } from '../../components/pbx'
import { Button, ErrorMessage } from '../../components/ui'
import { api } from '../../lib/api'
import { useSession } from '../../lib/session'
import { useTheme } from '../../lib/theme'
import type { Extension, InboundRoute, IVR, Prompt, TimeCondition } from '../../lib/types'
import { NodePanel } from './NodePanel'
import {
  NODE_W, autoLayout, blankFlow, canConnect, dataForKind, fromFlow, intentPort, kindTone, kinds, makeEdge, newId, toFlow, toneOf, tones,
  type Kind, type StepData, type StepNode,
} from './flow'
import { FlowInfoContext, kindIcon, nodeTypes, type FlowInfo } from './nodes'

type Snapshot = { nodes: StepNode[]; edges: Edge[] }
const DRAG_TYPE = 'application/x-kiestoon-step'

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

  const initial = useMemo(() => fromFlow(value.flow ?? blankFlow()), [value])
  const [ivr, setIvr] = useState<IVR>(value)
  const [nodes, setNodes, onNodesChangeBase] = useNodesState<StepNode>(initial.nodes)
  const [edges, setEdges, onEdgesChangeBase] = useEdgesState<Edge>(initial.edges)
  const [selected, setSelected] = useState<string | null>(() => initial.nodes.find((n) => n.data.type === 'entry')?.id ?? null)
  const [dirty, setDirty] = useState(!value.id)
  const [past, setPast] = useState<Snapshot[]>([])
  const [future, setFuture] = useState<Snapshot[]>([])
  const [testing, setTesting] = useState(false)
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
    if (!src || !isValidConnection(c)) return
    commit()
    const port = (c.sourceHandle ?? 'out') as 'out' | 'open' | 'closed'
    setEdges((es) => {
      // One line per port, except the entry, which has a line to every key.
      const rest = src.data.type === 'entry' ? es.filter((e) => !(e.source === c.source && e.target === c.target)) : es.filter((e) => !(e.source === c.source && (e.sourceHandle ?? 'out') === port))
      return [...rest, makeEdge(c.source, port, c.target, src.data)]
    })
  }, [byId, commit, isValidConnection, setEdges])

  const entryId = nodes.find((n) => n.data.type === 'entry')?.id
  const usedDigits = nodes.filter((n) => n.data.type === 'key').map((n) => n.data.digit ?? '')
  const hasTimeout = nodes.some((n) => n.data.type === 'timeout')

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
    if ((kind === 'key' || kind === 'timeout') && entryId) {
      const entry = byId.get(entryId)!
      setEdges((es) => [...es, makeEdge(entryId, 'out', id, entry.data)])
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
    if (data.type === 'key') data.digit = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '0', '*', '#'].find((k) => !usedDigits.includes(k))
    if (data.type === 'key' && !data.digit) return
    const nid = newId()
    setNodes((ns) => [...ns.map((x) => ({ ...x, selected: false })), { id: nid, type: 'step', position: { x: n.position.x + 30, y: n.position.y + 120 }, data, selected: true }])
    if (data.type === 'key' && entryId) setEdges((es) => [...es, makeEdge(entryId, 'out', nid, byId.get(entryId)!.data)])
    setSelected(nid)
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
    for (const n of nodes) {
      const d = n.data
      if ((d.type === 'key' || d.type === 'timeout') && !edges.some((e) => e.source === entryId && e.target === n.id)) m.set(n.id, t('ivr.problem.notInMenu'))
      else if (d.type !== 'entry' && d.type !== 'key' && d.type !== 'timeout' && !into.has(n.id)) m.set(n.id, t('ivr.problem.unreachable'))
      else if ((d.type === 'key' || d.type === 'audio' || d.type === 'timeout') && !from.has(n.id)) m.set(n.id, t('ivr.problem.noLine'))
      else if (d.type === 'time' && !d.destination?.target) m.set(n.id, t('ivr.problem.chooseTime'))
      else if (d.type === 'speech' && !(d.intents ?? []).length) m.set(n.id, t('ivr.problem.noIntents'))
      else if (d.type === 'speech' && (d.intents ?? []).some((i) => !i.label.trim() || !i.keywords.some((k) => k.trim()))) m.set(n.id, t('ivr.problem.intentIncomplete'))
      else if (d.type === 'destination' && d.destination?.type !== 'hangup' && !d.destination?.target) m.set(n.id, t('ivr.problem.chooseTarget'))
    }
    return m
  }, [nodes, edges, entryId, t])

  const promptMap = useMemo(() => new Map((prompts.data ?? []).map((p) => [p.id, p])), [prompts.data])
  const phone = ivr.id ? inbound.data?.find((r) => r.destinationType === 'ivr' && r.destination === String(ivr.id))?.did : undefined
  const info: FlowInfo = useMemo(() => ({
    prompts: promptMap,
    destLabel: (type, target) => dests?.options.find((o) => o.type === type && o.target === target)?.label
      ?? (type === 'timecondition' ? tcs.data?.find((c) => String(c.id) === target)?.name : undefined),
    problems, name: ivr.name, phone,
  }), [promptMap, dests, tcs.data, problems, ivr.name, phone])

  const sel = selected ? byId.get(selected) : undefined

  function doSave() {
    save({ ...ivr, flow: toFlow(nodes, edges) })
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
        <div role="toolbar" aria-label={t('ivr.palette')} className="flex flex-wrap items-center gap-1.5 rounded-2xl border border-line bg-surface-2 px-3 py-2">
          <span className="mr-1 text-xs text-subtle">{t('ivr.paletteHint')}</span>
          {kinds.map((k) => {
            const I = kindIcon[k]
            const disabled = (k === 'timeout' && hasTimeout) || (k === 'key' && usedDigits.length >= 12)
            return (
              <button key={k} type="button" draggable={!disabled} disabled={disabled}
                onDragStart={(e) => { e.dataTransfer.setData(DRAG_TYPE, k); e.dataTransfer.effectAllowed = 'move' }}
                onClick={() => addNode(k)} title={t('ivr.paletteClick')}
                className="inline-flex h-8 cursor-grab items-center gap-1.5 rounded-lg border border-line bg-field px-2.5 text-xs text-ink-2 transition hover:border-accent/50 hover:text-ink active:cursor-grabbing disabled:cursor-not-allowed disabled:opacity-40">
                <I className={`size-3.5 ${tones[kindTone(k)].text}`} aria-hidden />{t(`ivr.kind.${k}`)}
              </button>
            )
          })}
        </div>

        {/* Canvas + side panel */}
        <div className="relative flex h-[calc(100dvh-250px)] min-h-[560px] overflow-hidden rounded-2xl border border-line bg-bg">
          <div ref={wrapper} className="kt-flow min-w-0 flex-1" onDragOver={(e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move' }} onDrop={onDrop}>
            <ReactFlow<StepNode, Edge>
              nodes={nodes} edges={edges} nodeTypes={nodeTypes}
              onNodesChange={onNodesChange} onEdgesChange={onEdgesChange} onConnect={onConnect} isValidConnection={isValidConnection}
              onNodeClick={(_, n) => setSelected(n.id)} onPaneClick={() => setSelected(null)}
              onNodeDragStop={() => setDirty(true)}
              colorMode={theme} fitView fitViewOptions={{ padding: 0.08, maxZoom: 1 }} minZoom={0.2} maxZoom={1.6}
              deleteKeyCode={['Backspace', 'Delete']} proOptions={{ hideAttribution: true }}
              defaultEdgeOptions={{ type: 'smoothstep' }} connectionLineStyle={{ stroke: 'var(--color-accent)', strokeWidth: 2, strokeDasharray: '6 4' }}
              ariaLabelConfig={{ 'node.a11yDescription.default': t('ivr.a11yNode') }}>
              <Background variant={BackgroundVariant.Dots} gap={22} size={1.4} color="var(--color-grid-dot)" />
              <Controls showInteractive={false} position="bottom-right" />
              <MiniMap position="bottom-left" pannable zoomable nodeBorderRadius={6}
                nodeColor={(n) => tones[toneOf(n.data as StepData)].stroke} bgColor="var(--color-surface)" maskColor="rgb(from var(--color-bg) r g b / 0.6)" />
            </ReactFlow>
          </div>
          {sel && (
            <div className="absolute inset-0 z-10 bg-surface-2 lg:static lg:inset-auto lg:w-[380px] lg:flex-none lg:border-l lg:border-line">
              <NodePanel node={sel} ivr={ivr} setIvr={(fn) => { setIvr(fn); setDirty(true) }}
                prompts={prompts.data ?? []} onPromptsChanged={prompts.reload} timeConditions={tcs.data ?? []} usedDigits={usedDigits}
                onChange={(d) => changeData(sel.id, d)} onRemoveIntent={(i) => removeIntent(sel.id, i)} onDelete={() => remove(sel.id)} onDuplicate={() => duplicate(sel.id)} onClose={() => setSelected(null)} />
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
      <Select label={t('ivr.test.extension')} value={chosen} onChange={(e) => setExt(e.target.value)}>
        {(exts.data ?? []).map((e) => <option key={e.id} value={e.number}>{e.number} · {e.name}</option>)}
      </Select>
      {done && <p className="text-sm text-ok">{t('ivr.test.ringing', { ext: chosen })}</p>}
    </Modal>
  )
}
