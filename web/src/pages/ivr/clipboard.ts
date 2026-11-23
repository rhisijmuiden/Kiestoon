import type { Edge } from '@xyflow/react'
import type { FlowPort } from '../../lib/types'
import { KEYS, canConnect, canRingTogether, isMenu, makeEdge, newId, portsOf, type StepData, type StepNode } from './flow'

// Copy and paste of steps on the IVR canvas. The clipboard lives in this browser (localStorage), so
// steps can be pasted into another menu too. Only lines between copied steps are copied.

export type Clip = {
  nodes: { id: string; position: { x: number; y: number }; data: StepData }[]
  edges: { source: string; port: FlowPort; target: string }[]
}

const KEY = 'kt-ivr-clipboard'
let memory: Clip | null = null

export function readClip(): Clip | null {
  try {
    const raw = localStorage.getItem(KEY)
    if (raw) return JSON.parse(raw) as Clip
  } catch { /* storage blocked */ }
  return memory
}

export function writeClip(c: Clip) {
  memory = c
  try {
    localStorage.setItem(KEY, JSON.stringify(c))
  } catch { /* storage blocked */ }
}

/** A step and every step after it (following the lines), never the incoming call itself. */
export function branchOf(id: string, nodes: StepNode[], edges: Edge[]): string[] {
  const byId = new Map(nodes.map((n) => [n.id, n]))
  const seen = new Set<string>([id])
  const queue = [id]
  while (queue.length) {
    const cur = queue.shift()!
    for (const e of edges) {
      if (e.source === cur && !seen.has(e.target) && byId.get(e.target)?.data.type !== 'entry') {
        seen.add(e.target)
        queue.push(e.target)
      }
    }
  }
  return [...seen].filter((x) => byId.get(x)?.data.type !== 'entry')
}

/** Copies the steps with these ids and the lines between them. */
export function makeClip(ids: string[], nodes: StepNode[], edges: Edge[]): Clip {
  const keep = new Set(ids)
  const picked = nodes.filter((n) => keep.has(n.id) && n.data.type !== 'entry')
  const inClip = new Set(picked.map((n) => n.id))
  return {
    nodes: picked.map((n) => ({ id: n.id, position: { ...n.position }, data: structuredClone(n.data) })),
    edges: edges.filter((e) => inClip.has(e.source) && inClip.has(e.target))
      .map((e) => ({ source: e.source, port: (e.sourceHandle ?? 'out') as FlowPort, target: e.target })),
  }
}

/**
 * Pastes a clip. With a target step the copy is connected to it where that makes sense:
 * - a copied choice menu pasted on a menu (another choice menu or the incoming call) puts the copied
 *   keys and no-input step into that menu, so it gets the same choices;
 * - copied keys or no-input pasted on a menu join that menu;
 * - one copied step pasted on a step with a free "out" dot is connected to it.
 * Keys whose digit the menu already has stay unconnected (the canvas flags them).
 */
export function pasteClip(clip: Clip, target: StepNode | undefined, nodes: StepNode[], edges: Edge[], at?: { x: number; y: number }) {
  const ids = new Map(clip.nodes.map((n) => [n.id, newId()]))
  const incoming = new Set(clip.edges.map((e) => e.target))
  const roots = clip.nodes.filter((n) => !incoming.has(n.id))
  const minX = Math.min(...clip.nodes.map((n) => n.position.x))
  const minY = Math.min(...clip.nodes.map((n) => n.position.y))

  const menuMerge = !!target && isMenu(target.data) && roots.length === 1 && roots[0].data.type === 'menu'
  const skip = menuMerge ? roots[0].id : ''
  // Where the copy goes: in place of a merged menu, right of the target, at the mouse or offset.
  let dx: number, dy: number
  if (menuMerge) {
    dx = target!.position.x - roots[0].position.x
    dy = target!.position.y - roots[0].position.y
  } else if (target) {
    dx = target.position.x + 350 - minX
    dy = target.position.y - minY
  } else if (at) {
    dx = at.x - minX
    dy = at.y - minY
  } else {
    dx = 60
    dy = 60
  }

  const newNodes: StepNode[] = clip.nodes.filter((n) => n.id !== skip).map((n) => {
    const data = structuredClone(n.data)
    // "Back to" a copied menu points to its copy (or to the menu it was merged into).
    if (data.menu) data.menu = data.menu === skip && target ? target.id : ids.get(data.menu) ?? (nodes.some((x) => x.id === data.menu) ? data.menu : undefined)
    return { id: ids.get(n.id)!, type: 'step', position: { x: n.position.x + dx, y: n.position.y + dy }, data, selected: true }
  })
  const dataOf = (id: string) => newNodes.find((n) => n.id === id)?.data
  const newEdges: Edge[] = clip.edges.filter((e) => e.source !== skip)
    .map((e) => makeEdge(ids.get(e.source)!, e.port, ids.get(e.target)!, dataOf(ids.get(e.source)!)!))

  if (target) {
    const byId = new Map(nodes.map((n) => [n.id, n]))
    const into = (menu: StepNode, stepIds: string[]) => {
      const taken = new Set(edges.filter((e) => e.source === menu.id && byId.get(e.target)?.data.type === 'key').map((e) => byId.get(e.target)!.data.digit))
      let hasTimeout = edges.some((e) => e.source === menu.id && byId.get(e.target)?.data.type === 'timeout')
      for (const id of stepIds) {
        const d = dataOf(id)
        if (!d) continue
        if (d.type === 'key' && KEYS.includes(d.digit ?? '') && !taken.has(d.digit)) {
          taken.add(d.digit)
          newEdges.push(makeEdge(menu.id, 'out', id, menu.data))
        } else if (d.type === 'timeout' && !hasTimeout) {
          hasTimeout = true
          newEdges.push(makeEdge(menu.id, 'out', id, menu.data))
        }
      }
    }
    if (menuMerge) {
      into(target, clip.edges.filter((e) => e.source === skip).map((e) => ids.get(e.target)!))
    } else if (isMenu(target.data) && roots.every((r) => r.data.type === 'key' || r.data.type === 'timeout')) {
      into(target, roots.map((r) => ids.get(r.id)!))
    } else if (roots.length === 1 && portsOf(target.data.type).includes('out') && !isMenu(target.data) && canConnect(target.data, roots[0].data)) {
      const used = edges.filter((e) => e.source === target.id && (e.sourceHandle ?? 'out') === 'out')
      const together = used.length > 0 && canRingTogether(roots[0].data) && used.every((e) => canRingTogether(byId.get(e.target)?.data))
      if (!used.length || together) newEdges.push(makeEdge(target.id, 'out', ids.get(roots[0].id)!, target.data))
    } else if (roots.length === 1 && target.data.type === 'entry' && canConnect(target.data, roots[0].data)
      && !edges.some((e) => e.source === target.id)) {
      newEdges.push(makeEdge(target.id, 'out', ids.get(roots[0].id)!, target.data))
    }
  }
  return { nodes: newNodes, edges: newEdges }
}
