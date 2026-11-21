import { createContext, useContext } from 'react'
import { BaseEdge, getBezierPath, getSmoothStepPath, useNodesData, type Edge, type EdgeProps } from '@xyflow/react'
import { toneOf, tones, type StepData, type StepNode, type Tone } from './flow'

/** How lines are drawn: flowing curves (default) or angular with rounded corners. */
export type LineStyle = 'curve' | 'step'
export const LineStyleContext = createContext<LineStyle>('curve')

type FlowEdgeType = Edge<{ tone: Tone }, 'flow'>

/**
 * A line between two steps: a soft glow, the line itself with a colour gradient from the step it
 * leaves to the step it enters, and small dots running in the direction of the call.
 */
export function FlowEdge({ id, source, target, sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, data, markerEnd }: EdgeProps<FlowEdgeType>) {
  const lineStyle = useContext(LineStyleContext)
  const ends = useNodesData<StepNode>([source, target])
  const to = ends.find((n) => n.id === target)?.data as StepData | undefined
  const from = data?.tone ?? 'accent'
  const c1 = tones[from].stroke
  const c2 = to ? tones[toneOf(to)].stroke : c1
  const [path] = lineStyle === 'step'
    ? getSmoothStepPath({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, borderRadius: 14 })
    : getBezierPath({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, curvature: 0.35 })
  const gid = `kt-g-${id.replace(/[^a-zA-Z0-9_-]/g, '-')}`
  // A gradient in user space needs some width; a perfectly straight line would otherwise not render it.
  const x2 = Math.abs(targetX - sourceX) < 1 ? targetX + 1 : targetX
  const stroke = `url(#${gid})`
  return (
    <>
      <defs>
        <linearGradient id={gid} gradientUnits="userSpaceOnUse" x1={sourceX} y1={sourceY} x2={x2} y2={targetY}>
          <stop offset="0" style={{ stopColor: c1 }} />
          <stop offset="1" style={{ stopColor: c2 }} />
        </linearGradient>
      </defs>
      <path d={path} className="kt-edge-glow" stroke={stroke} />
      <BaseEdge id={id} path={path} className="kt-edge-main" style={{ stroke }} markerEnd={markerEnd} interactionWidth={18} />
      <path d={path} className="kt-edge-flow" />
    </>
  )
}

export const edgeTypes = { flow: FlowEdge }
