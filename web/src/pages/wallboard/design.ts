import type { WallAgent, WallData, WallDesign, WallMetric, WallPage, WallPeriod, WallQueue, WallStyle, WallTile, WallTileType, WallTotals } from '../../lib/types'
import { formatDuration } from '../../components/kit'
import { sumTotals } from './WallView'

/** Columns and rows of the grid per screen format. */
export function gridSize(format: WallDesign['format']): [number, number] {
  return format === 'portrait' ? [6, 12] : [12, 8]
}

export const uid = () => Math.random().toString(36).slice(2, 10)

export type TileGroup = 'kpi' | 'queues' | 'agents' | 'charts' | 'other'

/** The tiles of the library: group, default size and whether they show a figure. */
export const tileCatalog: { type: WallTileType; group: TileGroup; w: number; h: number }[] = [
  { type: 'kpi', group: 'kpi', w: 3, h: 2 },
  { type: 'gauge', group: 'kpi', w: 2, h: 2 },
  { type: 'trend', group: 'kpi', w: 3, h: 2 },
  { type: 'queue', group: 'queues', w: 4, h: 3 },
  { type: 'callers', group: 'queues', w: 4, h: 3 },
  { type: 'compare', group: 'queues', w: 4, h: 3 },
  { type: 'callbacks', group: 'queues', w: 3, h: 3 },
  { type: 'agents', group: 'agents', w: 4, h: 5 },
  { type: 'agentTiles', group: 'agents', w: 8, h: 3 },
  { type: 'leaderboard', group: 'agents', w: 3, h: 3 },
  { type: 'hourly', group: 'charts', w: 4, h: 3 },
  { type: 'donut', group: 'charts', w: 3, h: 3 },
  { type: 'clock', group: 'other', w: 2, h: 2 },
  { type: 'text', group: 'other', w: 4, h: 2 },
  { type: 'ticker', group: 'other', w: 12, h: 1 },
  { type: 'image', group: 'other', w: 2, h: 2 },
  { type: 'alerts', group: 'other', w: 6, h: 2 },
]

/** Tiles whose figure, thresholds and display can be chosen. */
export const figureTiles: WallTileType[] = ['kpi', 'gauge', 'trend']
/** Tiles that can be limited to some queues. */
export const queueTiles: WallTileType[] = ['kpi', 'gauge', 'trend', 'queue', 'callers', 'compare', 'callbacks', 'agents', 'agentTiles', 'leaderboard', 'hourly', 'donut', 'alerts']

type MetricInfo = { live: boolean; kind: 'count' | 'time' | 'pct'; better: 'higher' | 'lower' }
export const metricInfo: Record<WallMetric, MetricInfo> = {
  waiting: { live: true, kind: 'count', better: 'lower' },
  longestWait: { live: true, kind: 'time', better: 'lower' },
  callbacks: { live: true, kind: 'count', better: 'lower' },
  free: { live: true, kind: 'count', better: 'higher' },
  busy: { live: true, kind: 'count', better: 'lower' },
  paused: { live: true, kind: 'count', better: 'lower' },
  offered: { live: false, kind: 'count', better: 'higher' },
  answered: { live: false, kind: 'count', better: 'higher' },
  abandoned: { live: false, kind: 'count', better: 'lower' },
  abandonedPct: { live: false, kind: 'pct', better: 'lower' },
  serviceLevel: { live: false, kind: 'pct', better: 'higher' },
  avgWait: { live: false, kind: 'time', better: 'lower' },
  avgTalk: { live: false, kind: 'time', better: 'lower' },
  maxWait: { live: false, kind: 'time', better: 'lower' },
}
export const metrics = Object.keys(metricInfo) as WallMetric[]
export const periods: WallPeriod[] = ['today', 'hour', 'yesterday', 'week']

export function newTile(type: WallTileType, x: number, y: number, w?: number, h?: number): WallTile {
  const c = tileCatalog.find((t) => t.type === type)!
  const metric: WallMetric | '' = type === 'gauge' ? 'serviceLevel' : type === 'trend' ? 'answered' : type === 'kpi' ? 'waiting' : ''
  return {
    id: uid(), type, x, y, w: w ?? c.w, h: h ?? c.h, title: '', metric, queues: [], period: 'today', warn: 0, alarm: 0,
    fromSla: type === 'kpi' || type === 'gauge', display: type === 'gauge' ? 'gauge' : type === 'trend' ? 'trend' : 'big',
    blink: true, hideLabel: false, compare: false, text: '', image: '',
  }
}

export const defaultStyle: WallStyle = {
  theme: 'dark', accent: '', logo: 'kiestoon', logoImage: '', background: 'glow', tiles: 'glass', scale: 100, density: 'normal',
  clock: true, date: true, name: true, dots: true, language: 'browser', alarmBanner: true, alarmSound: false, alarmJump: true,
}

export type TemplateName = 'overview' | 'queue' | 'agents' | 'reception' | 'kpi' | 'blank'
export const templates: TemplateName[] = ['overview', 'queue', 'agents', 'reception', 'kpi', 'blank']

const t = (type: WallTileType, x: number, y: number, w: number, h: number, p: Partial<WallTile> = {}): WallTile => ({ ...newTile(type, x, y, w, h), ...p })

/** Ready-made pages; the classic layouts of the wallboard are among them. */
export function templatePages(name: TemplateName, pageName: string): WallPage[] {
  const page = (tiles: WallTile[], seconds = 30): WallPage => ({ id: uid(), name: pageName, seconds, tiles })
  switch (name) {
    case 'overview':
      return [page([
        t('kpi', 0, 0, 3, 2, { metric: 'waiting' }), t('kpi', 3, 0, 3, 2, { metric: 'longestWait' }), t('gauge', 6, 0, 2, 2),
        t('agents', 8, 0, 4, 5), t('callers', 0, 2, 4, 3), t('hourly', 4, 2, 4, 3),
        t('kpi', 0, 5, 3, 2, { metric: 'abandoned', fromSla: false }), t('trend', 3, 5, 3, 2), t('kpi', 6, 5, 3, 2, { metric: 'avgWait', fromSla: false }),
        t('leaderboard', 9, 5, 3, 2), t('ticker', 0, 7, 12, 1),
      ])]
    case 'queue':
      return [page([t('queue', 0, 0, 8, 6), t('agents', 8, 0, 4, 6), t('hourly', 0, 6, 12, 2)])]
    case 'agents':
      return [page([t('agentTiles', 0, 0, 12, 6), t('kpi', 0, 6, 3, 2, { metric: 'free', fromSla: false }), t('kpi', 3, 6, 3, 2, { metric: 'busy', fromSla: false }),
        t('kpi', 6, 6, 3, 2, { metric: 'paused', fromSla: false }), t('leaderboard', 9, 6, 3, 2)])]
    case 'reception':
      return [page([t('clock', 0, 0, 3, 2), t('kpi', 3, 0, 3, 2, { metric: 'waiting' }), t('kpi', 6, 0, 3, 2, { metric: 'longestWait' }), t('gauge', 9, 0, 3, 2),
        t('callers', 0, 2, 6, 5), t('agents', 6, 2, 6, 5), t('ticker', 0, 7, 12, 1)])]
    case 'kpi':
      return [page([
        t('kpi', 0, 0, 3, 3, { metric: 'waiting' }), t('kpi', 3, 0, 3, 3, { metric: 'longestWait' }), t('gauge', 6, 0, 3, 3), t('kpi', 9, 0, 3, 3, { metric: 'abandonedPct' }),
        t('trend', 0, 3, 4, 3), t('compare', 4, 3, 4, 3), t('donut', 8, 3, 4, 3),
        t('kpi', 0, 6, 4, 2, { metric: 'answered', period: 'week', fromSla: false }), t('kpi', 4, 6, 4, 2, { metric: 'avgTalk', fromSla: false }), t('alerts', 8, 6, 4, 2),
      ])]
    default:
      return [page([])]
  }
}

/** A new design: the overview template, or the classic layouts of an existing screen. */
export function initialDesign(layouts: string[], pageNames: Record<string, string>, theme: 'dark' | 'light'): WallDesign {
  const pages: WallPage[] = []
  for (const l of layouts.length ? layouts : ['overview']) {
    const name: TemplateName = l === 'queue' ? 'queue' : l === 'agents' ? 'agents' : 'overview'
    pages.push(...templatePages(name, pageNames[name] ?? name))
  }
  return { format: 'landscape', pages, style: { ...defaultStyle, theme } }
}

// ---- figures -------------------------------------------------------------------

export function tileQueues(data: WallData, tile: WallTile): WallQueue[] {
  return tile.queues.length ? data.queues.filter((q) => tile.queues.includes(q.id)) : data.queues
}

export function tileAgents(data: WallData, tile: WallTile): WallAgent[] {
  if (!tile.queues.length) return data.agents
  return data.agents.filter((a) => a.queues.some((id) => tile.queues.includes(id)))
}

const periodKey: Record<WallPeriod, 'today' | 'lastHour' | 'yesterday' | 'week'> = { today: 'today', hour: 'lastHour', yesterday: 'yesterday', week: 'week' }

export function periodTotals(qs: WallQueue[], period: WallPeriod): WallTotals {
  return sumTotals(qs.map((q) => q[periodKey[period]] ?? q.today))
}

/** The value of a metric for the queues of a tile; -1 when there is nothing to count. */
export function metricValue(data: WallData, tile: WallTile, metric: WallMetric = tile.metric || 'waiting', period: WallPeriod = tile.period): number {
  const qs = tileQueues(data, tile)
  const agents = tileAgents(data, tile)
  switch (metric) {
    case 'waiting': return qs.reduce((a, q) => a + q.waiting, 0)
    case 'longestWait': return qs.reduce((a, q) => Math.max(a, q.longestWait), 0)
    case 'callbacks': return qs.reduce((a, q) => a + q.callbacks, 0)
    case 'free': return agents.filter((a) => a.state === 'free').length
    case 'busy': return agents.filter((a) => a.state === 'call' || a.state === 'ringing' || a.state === 'internal').length
    case 'paused': return agents.filter((a) => a.state === 'paused').length
  }
  const tt = periodTotals(qs, period)
  switch (metric) {
    case 'offered': return tt.offered
    case 'answered': return tt.answered
    case 'abandoned': return tt.abandoned - tt.shortAbandoned
    case 'abandonedPct': return tt.abandonedPct
    case 'serviceLevel': return tt.serviceLevel
    case 'avgWait': return tt.answered ? tt.avgWait : -1
    case 'avgTalk': return tt.answered ? tt.avgTalk : -1
    case 'maxWait': return tt.longestWait
  }
  return -1
}

export function formatMetric(metric: WallMetric | '', v: number): string {
  if (v < 0) return '–'
  const kind = metric ? metricInfo[metric].kind : 'count'
  if (kind === 'time') return formatDuration(Math.round(v))
  if (kind === 'pct') return `${Math.round(v)}%`
  return String(Math.round(v))
}

/** The thresholds of a tile: its own, or taken from the SLA of its (first monitored) queue. */
export function thresholds(data: WallData, tile: WallTile): { warn: number; alarm: number } {
  if (!tile.fromSla) return { warn: tile.warn, alarm: tile.alarm }
  const q = tileQueues(data, tile).find((x) => x.sla && (x.sla.norm || x.sla.maxWait || x.sla.maxWaiting || x.sla.maxAbandoned))
  if (!q) return { warn: 0, alarm: 0 }
  const s = q.sla
  switch (tile.metric) {
    case 'waiting': return s.maxWaiting ? { warn: Math.max(1, Math.round(s.maxWaiting * 0.66)), alarm: s.maxWaiting } : { warn: 0, alarm: 0 }
    case 'longestWait': case 'maxWait': return s.maxWait ? { warn: Math.round(s.maxWait / 2), alarm: s.maxWait } : { warn: 0, alarm: 0 }
    case 'serviceLevel': { const n = s.norm || 80; return { warn: n, alarm: n - 10 } }
    case 'abandonedPct': return s.maxAbandoned ? { warn: s.maxAbandoned / 2, alarm: s.maxAbandoned } : { warn: 0, alarm: 0 }
    case 'avgWait': return s.target ? { warn: Math.round(s.target * 0.75), alarm: s.target } : { warn: 0, alarm: 0 }
  }
  return { warn: 0, alarm: 0 }
}

export type Tone = 'ok' | 'warn' | 'danger' | 'muted' | 'ink'

/** Green, orange or red by the thresholds; ink when no thresholds are set. */
export function metricTone(metric: WallMetric | '', v: number, th: { warn: number; alarm: number }): Tone {
  if (v < 0) return 'muted'
  if (!metric || (!th.warn && !th.alarm)) return 'ink'
  if (metricInfo[metric].better === 'higher') {
    if (th.alarm && v < th.alarm) return 'danger'
    if (th.warn && v < th.warn) return 'warn'
    return 'ok'
  }
  if (th.alarm && v > th.alarm) return 'danger'
  if (th.warn && v > th.warn) return 'warn'
  return 'ok'
}

/** The hourly series of a tile (answered or abandoned) for a sparkline. */
export function hourly(data: WallData, tile: WallTile): { answered: number; abandoned: number }[] {
  const qs = tileQueues(data, tile)
  return Array.from({ length: 24 }, (_, h) => ({
    answered: qs.reduce((a, q) => a + (q.hours[h]?.answered ?? 0), 0),
    abandoned: qs.reduce((a, q) => a + (q.hours[h]?.abandoned ?? 0), 0),
  }))
}

/** Whether a tile gets the red alarm edge: a queue tile whose queue has an SLA alarm, or a figure that is red. */
export function tileAlarmed(data: WallData, tile: WallTile): boolean {
  if (tile.type === 'queue') {
    const q = tileQueues(data, tile)[0]
    return !!q && data.alerts.some((a) => a.queueId === q.id)
  }
  if (figureTiles.includes(tile.type) && tile.metric) return metricTone(tile.metric, metricValue(data, tile), thresholds(data, tile)) === 'danger'
  return false
}

/** Tiles may not overlap: the first free spot for a w×h tile, scanning rows. */
export function overlaps(tiles: WallTile[], r: { x: number; y: number; w: number; h: number }, skip?: string) {
  return tiles.some((t) => t.id !== skip && r.x < t.x + t.w && t.x < r.x + r.w && r.y < t.y + t.h && t.y < r.y + r.h)
}

export function freeSpot(tiles: WallTile[], w: number, h: number, cols: number, rows: number): { x: number; y: number; w: number; h: number } | null {
  for (let hh = h; hh >= 1; hh--) {
    for (let ww = w; ww >= 1; ww--) {
      for (let y = 0; y + hh <= rows; y++) {
        for (let x = 0; x + ww <= cols; x++) if (!overlaps(tiles, { x, y, w: ww, h: hh })) return { x, y, w: ww, h: hh }
      }
    }
  }
  return null
}

/** The theme a screen shows now: auto is light from 07:00 to 19:00. */
export function themeNow(theme: WallStyle['theme'], now: Date): 'dark' | 'light' {
  if (theme !== 'auto') return theme
  const h = now.getHours()
  return h >= 7 && h < 19 ? 'light' : 'dark'
}
