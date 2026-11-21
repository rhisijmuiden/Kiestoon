import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link, useNavigate, useParams } from 'react-router'
import {
  ArrowLeft, BarChart3, BellRing, Clock, Copy, Eye, Gauge as GaugeIcon, GripVertical, Hash, Image as ImageIcon, KeyRound, LayoutTemplate,
  ListOrdered, Minimize2, Palette, PhoneForwarded, PieChart, Plus, Redo2, Rows3, Search, Send, ShieldCheck, Text, Trash2, TrendingUp, Trophy, Tv,
  Undo2, Upload, Users, LayoutGrid, Megaphone, SquareStack, type LucideIcon,
} from 'lucide-react'
import { Badge, CopyField, useApi } from '../../components/kit'
import { Alert, Button, ErrorMessage } from '../../components/ui'
import { api } from '../../lib/api'
import type { WallData, WallDesign, WallMetric, WallPage, WallScreen, WallSettings, WallStyle, WallTile, WallTileType } from '../../lib/types'
import { Board, BoardFrame, DesignedScreen, tileTitle } from './Board'
import {
  defaultStyle, figureTiles, freeSpot, gridSize, initialDesign, metricInfo, metrics, newTile, overlaps, periods, queueTiles, templatePages, templates,
  tileCatalog, uid, type TemplateName, type TileGroup,
} from './design'

const tileIcon: Record<WallTileType, LucideIcon> = {
  kpi: Hash, gauge: GaugeIcon, trend: TrendingUp, queue: SquareStack, callers: ListOrdered, compare: Rows3, callbacks: PhoneForwarded,
  agents: Users, agentTiles: LayoutGrid, leaderboard: Trophy, hourly: BarChart3, donut: PieChart, clock: Clock, text: Text, ticker: Megaphone,
  image: ImageIcon, alerts: BellRing,
}
const groups: TileGroup[] = ['kpi', 'queues', 'agents', 'charts', 'other']
const metricTiles: WallTileType[] = ['kpi', 'gauge', 'trend', 'compare']
const periodTiles: WallTileType[] = ['kpi', 'gauge', 'trend', 'compare', 'donut']

export const wallAddress = (slug: string) => `${window.location.origin}/wall/${slug}`

type Tab = 'layout' | 'style' | 'access'
type Drag = { id: string; mode: 'move' | 'resize'; dx: number; dy: number; x: number; y: number; w: number; h: number }

/** Small segmented control for the dense panels of the designer. */
function Seg<T extends string>({ value, options, onChange, label }: { value: T; options: { key: T; label: string }[]; onChange: (v: T) => void; label: string }) {
  return (
    <div role="radiogroup" aria-label={label} className="grid overflow-hidden rounded-[9px] border border-line" style={{ gridTemplateColumns: `repeat(${options.length}, minmax(0, 1fr))` }}>
      {options.map((o, i) => (
        <button key={o.key} type="button" role="radio" aria-checked={value === o.key} onClick={() => onChange(o.key)}
          className={`min-h-9 px-1.5 text-[12.5px] transition ${i ? 'border-l border-line' : ''} ${value === o.key ? 'bg-accent/15 font-medium text-ink' : 'text-muted hover:text-ink'}`}>{o.label}</button>
      ))}
    </div>
  )
}

function Field({ label, children, hint }: { label: string; children: ReactNode; hint?: ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5">
      <span className="text-[11px] uppercase tracking-[0.1em] text-subtle">{label}</span>
      {children}
      {hint && <span className="text-xs text-subtle">{hint}</span>}
    </div>
  )
}

const small = 'min-h-9 w-full rounded-[9px] border border-line bg-field px-2.5 text-[13px] text-ink outline-none focus:border-accent/60 focus:ring-2 focus:ring-accent/20'

function Switch({ label, checked, onChange }: { label: string; checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="flex min-h-8 cursor-pointer items-center justify-between gap-3 text-[13px] text-ink-2">
      {label}
      <button type="button" role="switch" aria-checked={checked} aria-label={label} onClick={() => onChange(!checked)}
        className={`relative inline-flex h-5 w-9 flex-none items-center rounded-full border transition ${checked ? 'border-accent/60 bg-accent/30' : 'border-line bg-tint/5'}`}>
        <span className={`inline-block size-3.5 rounded-full transition ${checked ? 'translate-x-[18px] bg-accent' : 'translate-x-0.5 bg-muted'}`} />
      </button>
    </label>
  )
}

function chip(on: boolean) {
  return `inline-flex min-h-8 items-center gap-1 rounded-full border px-2.5 text-xs transition ${on ? 'border-accent/50 bg-accent/12 text-ink' : 'border-line text-muted hover:text-ink'}`
}

/** Reads an image file as a data: URL; photos are scaled down so the design stays small. */
async function readImage(file: File): Promise<string> {
  const url = await new Promise<string>((res, rej) => { const r = new FileReader(); r.onload = () => res(String(r.result)); r.onerror = rej; r.readAsDataURL(file) })
  if (file.type === 'image/svg+xml') {
    if (url.length > 380_000) throw new Error('too_large')
    return url
  }
  const img = await new Promise<HTMLImageElement>((res, rej) => { const i = new Image(); i.onload = () => res(i); i.onerror = rej; i.src = url })
  const scale = Math.min(1, 900 / Math.max(img.width, img.height))
  const c = document.createElement('canvas')
  c.width = Math.round(img.width * scale)
  c.height = Math.round(img.height * scale)
  c.getContext('2d')!.drawImage(img, 0, 0, c.width, c.height)
  let out = c.toDataURL('image/png')
  if (out.length > 380_000) out = c.toDataURL('image/webp', 0.85)
  if (out.length > 380_000) out = c.toDataURL('image/jpeg', 0.8)
  if (out.length > 380_000) throw new Error('too_large')
  return out
}

function ImagePick({ label, onPick, onError }: { label: string; onPick: (url: string) => void; onError: (e: unknown) => void }) {
  const ref = useRef<HTMLInputElement>(null)
  return (
    <>
      <button type="button" onClick={() => ref.current?.click()} className="flex min-h-9 w-full items-center justify-center gap-2 rounded-[9px] border border-dashed border-accent/45 text-[13px] text-accent-ink hover:bg-accent/5">
        <Upload className="size-4" aria-hidden />{label}
      </button>
      <input ref={ref} type="file" accept="image/png,image/jpeg,image/webp,image/svg+xml" className="hidden"
        onChange={(e) => { const f = e.target.files?.[0]; e.target.value = ''; if (f) readImage(f).then(onPick, onError) }} />
    </>
  )
}

/** The wallboard designer: tiles on a grid, pages that rotate, the house style and the address of a screen. */
export function WallDesigner() {
  const { t } = useTranslation()
  const { id } = useParams()
  const nav = useNavigate()
  const settings = useApi<WallSettings>('/wallboard/settings')
  const live = useApi<WallData>('/reception/wallboard', 4000)
  const screen = settings.data?.screens.find((s) => String(s.id) === id)
  if (settings.error) return <ErrorMessage error={settings.error} />
  if (!settings.data || !live.data) return <div className="p-8 text-muted">{t('app.loading')}</div>
  if (!screen) return <Alert>{t('wallDesign.notFound')}</Alert>
  return <Editor key={screen.id} screen={screen} settings={settings.data} live={live.data} onSaved={settings.reload} onSwitch={(sid) => nav(`/wallboard/designer/${sid}`)} />
}

function Editor({ screen, settings, live, onSaved, onSwitch }: { screen: WallScreen; settings: WallSettings; live: WallData; onSaved: () => Promise<void>; onSwitch: (id: number) => void }) {
  const { t } = useTranslation()
  const pageNames = useMemo(() => Object.fromEntries(templates.map((n) => [n, t(`wallDesign.template.${n}`)])), [t])
  const [s, setS] = useState<WallScreen>(screen)
  const [design, setDesignRaw] = useState<WallDesign>(() => screen.design ?? initialDesign(screen.layouts, pageNames, screen.theme))
  const [past, setPast] = useState<WallDesign[]>([])
  const [future, setFuture] = useState<WallDesign[]>([])
  const [dirty, setDirty] = useState(!screen.design)
  const [tab, setTab] = useState<Tab>('layout')
  const [page, setPage] = useState(0)
  const [sel, setSel] = useState<string | null>(null)
  const [drag, setDrag] = useState<Drag | null>(null)
  const [ghost, setGhost] = useState<{ x: number; y: number; w: number; h: number; ok: boolean } | null>(null)
  const [busy, setBusy] = useState<'' | 'save' | 'publish'>('')
  const [error, setError] = useState<unknown>(null)
  const [notice, setNotice] = useState('')
  const [preview, setPreview] = useState(false)
  const [search, setSearch] = useState('')
  const [now, setNow] = useState(() => new Date())
  const gridRef = useRef<HTMLDivElement>(null)
  useEffect(() => { const i = window.setInterval(() => setNow(new Date()), 1000); return () => window.clearInterval(i) }, [])

  const [cols, rows] = gridSize(design.format)
  const cur = design.pages[Math.min(page, design.pages.length - 1)]
  const tile = cur.tiles.find((x) => x.id === sel) ?? null
  const unpublished = dirty || JSON.stringify(screen.design) !== JSON.stringify(screen.published)

  // The tv only gets the queues of the screen; the designer shows the same.
  const data = useMemo<WallData>(() => {
    if (!s.queues.length) return live
    const keep = (id: number) => s.queues.includes(id)
    return { ...live, queues: live.queues.filter((q) => keep(q.id)), agents: live.agents.filter((a) => a.queues.some(keep)), alerts: live.alerts.filter((a) => keep(a.queueId)) }
  }, [live, s.queues])
  const queues = settings.queues.filter((q) => !s.queues.length || s.queues.includes(q.id))

  const setDesign = useCallback((next: WallDesign | ((d: WallDesign) => WallDesign), record = true) => {
    setDesignRaw((d) => {
      const n = typeof next === 'function' ? next(d) : next
      if (record) { setPast((p) => [...p.slice(-49), d]); setFuture([]) }
      return n
    })
    setDirty(true)
    setNotice('')
  }, [])
  const undo = () => { if (!past.length) return; setFuture((f) => [design, ...f]); setDesignRaw(past[past.length - 1]); setPast(past.slice(0, -1)); setDirty(true) }
  const redo = () => { if (!future.length) return; setPast((p) => [...p, design]); setDesignRaw(future[0]); setFuture(future.slice(1)); setDirty(true) }
  const setPageTiles = (fn: (tiles: WallTile[]) => WallTile[], record = true) =>
    setDesign((d) => ({ ...d, pages: d.pages.map((p, i) => (i === page ? { ...p, tiles: fn(p.tiles) } : p)) }), record)
  const updTile = (p: Partial<WallTile>) => sel && setPageTiles((ts) => ts.map((x) => (x.id === sel ? { ...x, ...p } : x)))
  const updPage = (p: Partial<WallPage>) => setDesign((d) => ({ ...d, pages: d.pages.map((x, i) => (i === page ? { ...x, ...p } : x)) }))
  const updStyle = (p: Partial<WallStyle>) => setDesign((d) => ({ ...d, style: { ...d.style, ...p } }))

  function addTile(type: WallTileType, at?: { x: number; y: number }) {
    const c = tileCatalog.find((x) => x.type === type)!
    let spot: { x: number; y: number; w: number; h: number } | null = null
    if (at) {
      const r = { x: Math.min(at.x, cols - Math.min(c.w, cols)), y: Math.min(at.y, rows - Math.min(c.h, rows)), w: Math.min(c.w, cols), h: Math.min(c.h, rows) }
      if (!overlaps(cur.tiles, r)) spot = r
    }
    spot ??= freeSpot(cur.tiles, Math.min(c.w, cols), Math.min(c.h, rows), cols, rows)
    if (!spot) { setNotice(t('wallDesign.full')); return }
    const nt = newTile(type, spot.x, spot.y, spot.w, spot.h)
    setPageTiles((ts) => [...ts, nt])
    setSel(nt.id)
  }
  function removeTile() { if (!sel) return; setPageTiles((ts) => ts.filter((x) => x.id !== sel)); setSel(null) }
  function duplicateTile() {
    if (!tile) return
    const spot = freeSpot(cur.tiles, tile.w, tile.h, cols, rows)
    if (!spot) { setNotice(t('wallDesign.full')); return }
    const nt = { ...tile, ...spot, id: uid() }
    setPageTiles((ts) => [...ts, nt])
    setSel(nt.id)
  }

  // Grid cell under the pointer.
  const cellAt = (cx: number, cy: number) => {
    const r = gridRef.current?.getBoundingClientRect()
    if (!r) return { x: 0, y: 0 }
    return { x: Math.max(0, Math.min(cols - 1, Math.floor(((cx - r.left) / r.width) * cols))), y: Math.max(0, Math.min(rows - 1, Math.floor(((cy - r.top) / r.height) * rows))) }
  }

  function startDrag(e: React.PointerEvent, tl: WallTile, mode: Drag['mode']) {
    if (e.button !== 0) return
    e.preventDefault()
    e.stopPropagation()
    ;(e.currentTarget.closest('[role=button]') as HTMLElement | null)?.focus({ preventScroll: true })
    setSel(tl.id)
    const c = cellAt(e.clientX, e.clientY)
    setDrag({ id: tl.id, mode, dx: c.x - tl.x, dy: c.y - tl.y, x: tl.x, y: tl.y, w: tl.w, h: tl.h })
  }
  useEffect(() => {
    if (!drag) return
    const move = (e: PointerEvent) => {
      const c = cellAt(e.clientX, e.clientY)
      setDrag((d) => {
        if (!d) return d
        if (d.mode === 'move') return { ...d, x: Math.max(0, Math.min(cols - d.w, c.x - d.dx)), y: Math.max(0, Math.min(rows - d.h, c.y - d.dy)) }
        return { ...d, w: Math.max(1, Math.min(cols - d.x, c.x - d.x + 1)), h: Math.max(1, Math.min(rows - d.y, c.y - d.y + 1)) }
      })
    }
    const up = () => {
      setDrag((d) => {
        if (d) {
          const tl = cur.tiles.find((x) => x.id === d.id)
          const changed = tl && (tl.x !== d.x || tl.y !== d.y || tl.w !== d.w || tl.h !== d.h)
          if (changed && !overlaps(cur.tiles, d, d.id)) setPageTiles((ts) => ts.map((x) => (x.id === d.id ? { ...x, x: d.x, y: d.y, w: d.w, h: d.h } : x)))
        }
        return null
      })
    }
    window.addEventListener('pointermove', move)
    window.addEventListener('pointerup', up, { once: true })
    return () => { window.removeEventListener('pointermove', move); window.removeEventListener('pointerup', up) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [drag?.id, drag?.mode, cols, rows, cur])

  function onKey(e: React.KeyboardEvent) {
    if (!tile || (e.target as HTMLElement).closest('input,textarea,select')) return
    const d = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }[e.key]
    if (e.key === 'Delete' || e.key === 'Backspace') { e.preventDefault(); removeTile(); return }
    if (!d) return
    e.preventDefault()
    const r = e.shiftKey
      ? { x: tile.x, y: tile.y, w: Math.max(1, Math.min(cols - tile.x, tile.w + d[0])), h: Math.max(1, Math.min(rows - tile.y, tile.h + d[1])) }
      : { x: Math.max(0, Math.min(cols - tile.w, tile.x + d[0])), y: Math.max(0, Math.min(rows - tile.h, tile.y + d[1])), w: tile.w, h: tile.h }
    if (!overlaps(cur.tiles, r, tile.id)) updTile(r)
  }

  // The design shown: with the tile being dragged at its new place.
  const shown = useMemo<WallDesign>(() => {
    if (!drag) return design
    return { ...design, pages: design.pages.map((p, i) => (i !== page ? p : { ...p, tiles: p.tiles.map((x) => (x.id === drag.id ? { ...x, x: drag.x, y: drag.y, w: drag.w, h: drag.h } : x)) })) }
  }, [design, drag, page])
  const dragBad = !!drag && overlaps(cur.tiles, drag, drag.id)

  async function save(publish = false) {
    setBusy(publish ? 'publish' : 'save')
    setError(null)
    try {
      await api('PUT', `/wallboard/screens/${s.id}`, { ...s, design })
      if (publish) await api('POST', `/wallboard/screens/${s.id}/publish`, {})
      setDirty(false)
      setS((x) => ({ ...x, pin: '' }))
      setNotice(t(publish ? 'wallDesign.published' : 'wallDesign.saved'))
      await onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy('')
    }
  }
  async function newKey() {
    if (!window.confirm(t('wallboard.newKeyConfirm', { name: s.name }))) return
    try { const r = await api<{ key: string }>('POST', `/wallboard/screens/${s.id}/key`, {}); setS((x) => ({ ...x, key: r.key })); await onSaved() } catch (e) { setError(e) }
  }
  function applyTemplate(name: TemplateName, replace: boolean) {
    const p = templatePages(name, pageNames[name])[0]
    if (design.format === 'portrait') p.tiles = []
    if (replace) {
      if (cur.tiles.length && !window.confirm(t('wallDesign.replaceConfirm'))) return
      setDesign((d) => ({ ...d, pages: d.pages.map((x, i) => (i === page ? { ...p, id: x.id, seconds: x.seconds } : x)) }))
    } else {
      setDesign((d) => ({ ...d, pages: [...d.pages, p] }))
      setPage(design.pages.length)
    }
    setSel(null)
  }
  function setFormat(format: WallDesign['format']) {
    if (format === design.format) return
    const [c, r] = gridSize(format)
    setDesign((d) => ({
      ...d, format,
      pages: d.pages.map((p) => {
        const placed: WallTile[] = []
        for (const x of p.tiles) {
          const spot = freeSpot(placed, Math.min(x.w, c), Math.min(x.h, r), c, r)
          if (spot) placed.push({ ...x, ...spot })
        }
        return { ...p, tiles: placed }
      }),
    }))
  }

  const tabs: { key: Tab; label: string; icon: LucideIcon }[] = [
    { key: 'layout', label: t('wallDesign.tab.layout'), icon: LayoutTemplate }, { key: 'style', label: t('wallDesign.tab.style'), icon: Palette },
    { key: 'access', label: t('wallDesign.tab.access'), icon: ShieldCheck },
  ]
  const address = wallAddress(s.slug || '…')
  const lib = tileCatalog.filter((c) => !search || t(`wallDesign.tile.${c.type}`).toLowerCase().includes(search.toLowerCase()))

  const canvas = (
    <BoardFrame design={shown} now={now} className={`w-full rounded-xl border border-line ${design.format === 'portrait' ? 'mx-auto aspect-[9/16] max-h-[78vh] !w-auto' : 'aspect-video'}`}>
      <Board data={data} design={shown} name={s.name} page={page} pages={design.pages.length} now={now} gridRef={gridRef} alarmed
        wrapTile={(tl, content) => {
          const on = tl.id === sel
          return (
            <div className={`group h-full cursor-grab touch-none rounded-[1em] ${on ? `outline-2 outline-offset-2 ${dragBad && drag?.id === tl.id ? 'outline-danger' : 'outline-accent'} outline` : 'hover:outline hover:outline-1 hover:outline-accent/40'}`}
              onPointerDown={(e) => startDrag(e, tl, 'move')} onClick={(e) => { e.stopPropagation(); setSel(tl.id) }}
              role="button" tabIndex={0} aria-label={tileTitle(tl, t)} aria-pressed={on} onFocus={() => setSel(tl.id)}>
              <div className="pointer-events-none h-full">{content}</div>
              {on && (
                <span onPointerDown={(e) => startDrag(e, tl, 'resize')} aria-hidden
                  className="absolute -bottom-1.5 -right-1.5 z-10 size-4 cursor-nwse-resize rounded-[4px] border-2 border-bg bg-accent shadow" />
              )}
            </div>
          )
        }} />
      {ghost && (
        <div className="pointer-events-none absolute inset-0" aria-hidden>
          <GhostLayer grid={gridRef} g={ghost} cols={cols} rows={rows} label={t('wallDesign.dropHere')} />
        </div>
      )}
    </BoardFrame>
  )

  return (
    <div className="flex flex-col gap-4" onKeyDown={onKey}>
      <div className="flex flex-wrap items-start gap-4">
        <div className="min-w-0 flex-1">
          <Link to="/wallboard/settings" className="mb-1 inline-flex items-center gap-1.5 text-sm text-muted hover:text-ink"><ArrowLeft className="size-4" aria-hidden />{t('wallboard.settingsTitle')}</Link>
          <h1 className="font-display text-[26px] font-semibold tracking-tight">{t('wallDesign.title')}</h1>
          <p className="mt-0.5 text-sm text-muted">{t('wallDesign.subtitle')}</p>
        </div>
        <div role="tablist" aria-label={t('wallboard.screens')} className="flex flex-wrap gap-1 rounded-xl border border-line bg-surface-2/70 p-1">
          {settings.screens.map((x) => (
            <button key={x.id} type="button" role="tab" aria-selected={x.id === s.id} onClick={() => x.id !== s.id && (!dirty || window.confirm(t('wallDesign.leaveConfirm'))) && onSwitch(x.id)}
              className={`inline-flex min-h-9 items-center gap-1.5 rounded-lg px-3 text-[13px] ${x.id === s.id ? 'bg-accent/15 text-ink shadow-[inset_0_0_0_1px_color-mix(in_srgb,var(--color-accent)_35%,transparent)]' : 'text-muted hover:text-ink'}`}>
              {x.id === s.id && <Tv className="size-3.5" aria-hidden />}{x.name}
            </button>
          ))}
        </div>
      </div>
      <div className="flex flex-wrap items-center gap-2.5">
        <div role="tablist" className="flex flex-wrap gap-1 rounded-xl border border-line bg-surface-2/70 p-1">
          {tabs.map((tb) => (
            <button key={tb.key} type="button" role="tab" aria-selected={tab === tb.key} onClick={() => setTab(tb.key)}
              className={`inline-flex min-h-9 items-center gap-1.5 rounded-lg px-3.5 text-[13px] ${tab === tb.key ? 'bg-accent/15 text-ink shadow-[inset_0_0_0_1px_color-mix(in_srgb,var(--color-accent)_35%,transparent)]' : 'text-muted hover:text-ink'}`}>
              <tb.icon className="size-3.5" aria-hidden />{tb.label}
            </button>
          ))}
        </div>
        <span className="flex-1" />
        {unpublished ? <Badge tone="warn" dot>{t('wallDesign.unpublished')}</Badge> : <Badge tone="ok" dot>{t('wallDesign.live')}</Badge>}
        <Button variant="secondary" onClick={() => setPreview(true)}><Eye className="size-4" aria-hidden />{t('wallDesign.preview')}</Button>
        <Button variant="secondary" busy={busy === 'save'} onClick={() => save(false)}>{t('common.save')}</Button>
        <Button busy={busy === 'publish'} onClick={() => save(true)}><Send className="size-4" aria-hidden />{t('wallDesign.publish')}</Button>
      </div>
      <ErrorMessage error={error} />
      {notice && <Alert tone="ok">{notice}</Alert>}

      {tab === 'layout' && (
        <div className="grid gap-4 xl:grid-cols-[230px_minmax(0,1fr)_290px]">
          <section className="card flex flex-col gap-2.5 p-3.5 xl:max-h-[calc(100vh-220px)] xl:overflow-y-auto" aria-label={t('wallDesign.library')}>
            <div className="flex items-center justify-between"><b className="font-display text-[15px]">{t('wallDesign.library')}</b><Badge>{tileCatalog.length}</Badge></div>
            <label className="relative">
              <Search className="pointer-events-none absolute left-2.5 top-2.5 size-4 text-subtle" aria-hidden />
              <input value={search} onChange={(e) => setSearch(e.target.value)} placeholder={t('wallDesign.searchTiles')} aria-label={t('wallDesign.searchTiles')} className={`${small} pl-8`} />
            </label>
            <p className="text-xs text-subtle">{t('wallDesign.libraryHint')}</p>
            {groups.map((g) => {
              const items = lib.filter((c) => c.group === g)
              if (!items.length) return null
              return (
                <div key={g} className="flex flex-col gap-1">
                  <span className="mt-1 text-[11px] uppercase tracking-[0.1em] text-subtle">{t(`wallDesign.group.${g}`)}</span>
                  {items.map((c) => {
                    const Icon = tileIcon[c.type]
                    return (
                      <button key={c.type} type="button" draggable onClick={() => addTile(c.type)}
                        onDragStart={(e) => { e.dataTransfer.setData('text/x-kt-tile', c.type); e.dataTransfer.effectAllowed = 'copy' }}
                        onDragEnd={() => setGhost(null)}
                        className="flex min-h-9 items-center gap-2 rounded-[10px] border border-line bg-tint/[0.02] px-2 text-left text-[13px] text-ink-2 transition hover:border-accent/45 hover:text-ink">
                        <span className="grid size-6 flex-none place-items-center rounded-md bg-accent/12 text-accent"><Icon className="size-3.5" aria-hidden /></span>
                        <span className="min-w-0 flex-1 truncate">{t(`wallDesign.tile.${c.type}`)}</span>
                        <GripVertical className="size-3.5 text-subtle" aria-hidden />
                      </button>
                    )
                  })}
                </div>
              )
            })}
          </section>

          <section className="card flex min-w-0 flex-col gap-3 p-3.5">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-[11px] uppercase tracking-[0.1em] text-subtle">{t('wallDesign.pages')}</span>
              {design.pages.map((p, i) => (
                <button key={p.id} type="button" onClick={() => { setPage(i); setSel(null) }} aria-pressed={i === page}
                  className={`inline-flex min-h-8 items-center gap-2 rounded-[9px] border px-2.5 text-[12.5px] ${i === page ? 'border-accent/50 bg-accent/10 text-ink' : 'border-line text-ink-2 hover:text-ink'}`}>
                  {i + 1} · {p.name || t('wallDesign.page')}{design.pages.length > 1 && <span className="font-mono text-[11px] text-subtle">{p.seconds}s</span>}
                </button>
              ))}
              <button type="button" onClick={() => { setDesign((d) => ({ ...d, pages: [...d.pages, { id: uid(), name: '', seconds: 30, tiles: [] }] })); setPage(design.pages.length); setSel(null) }}
                disabled={design.pages.length >= 12} className="inline-flex min-h-8 items-center gap-1 rounded-[9px] border border-dashed border-line px-2.5 text-[12.5px] text-accent-ink disabled:opacity-50">
                <Plus className="size-3.5" aria-hidden />{t('wallDesign.addPage')}
              </button>
              <span className="flex-1" />
              <button type="button" onClick={undo} disabled={!past.length} aria-label={t('wallDesign.undo')} title={t('wallDesign.undo')} className="inline-flex size-8 items-center justify-center rounded-lg border border-line text-ink-2 disabled:opacity-40"><Undo2 className="size-4" aria-hidden /></button>
              <button type="button" onClick={redo} disabled={!future.length} aria-label={t('wallDesign.redo')} title={t('wallDesign.redo')} className="inline-flex size-8 items-center justify-center rounded-lg border border-line text-ink-2 disabled:opacity-40"><Redo2 className="size-4" aria-hidden /></button>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <select aria-label={t('wallDesign.template.label')} className={`${small} !w-auto`} value="" onChange={(e) => { if (e.target.value) applyTemplate(e.target.value as TemplateName, true) }}>
                <option value="">{t('wallDesign.template.label')}…</option>
                {templates.map((n) => <option key={n} value={n}>{pageNames[n]}</option>)}
              </select>
              <select aria-label={t('wallDesign.format')} className={`${small} !w-auto`} value={design.format} onChange={(e) => setFormat(e.target.value as WallDesign['format'])}>
                <option value="landscape">{t('wallDesign.landscape')}</option>
                <option value="portrait">{t('wallDesign.portrait')}</option>
              </select>
              <Badge>{t('wallDesign.grid', { cols, rows })}</Badge>
              <span className="flex-1" />
              <span className="text-xs text-subtle">{t('wallDesign.canvasHint')}</span>
            </div>
            <div className="relative" onClick={() => setSel(null)}
              onDragOver={(e) => {
                if (!e.dataTransfer.types.includes('text/x-kt-tile')) return
                e.preventDefault()
                const c = cellAt(e.clientX, e.clientY)
                setGhost((g) => (g && g.x === c.x && g.y === c.y ? g : { x: c.x, y: c.y, w: 3, h: 2, ok: true }))
              }}
              onDragLeave={(e) => { if (!e.currentTarget.contains(e.relatedTarget as Node)) setGhost(null) }}
              onDrop={(e) => {
                const type = e.dataTransfer.getData('text/x-kt-tile') as WallTileType
                setGhost(null)
                if (!type) return
                e.preventDefault()
                addTile(type, cellAt(e.clientX, e.clientY))
              }}>
              {canvas}
            </div>
          </section>

          <section className="card flex flex-col gap-3.5 p-3.5" aria-label={t('wallDesign.properties')}>
            {tile ? <TileProps tile={tile} queues={queues} onChange={updTile} onDelete={removeTile} onDuplicate={duplicateTile} onError={setError} cols={cols} rows={rows} others={cur.tiles} /> : (
              <>
                <b className="font-display text-[15px]">{t('wallDesign.pageProps')}</b>
                <Field label={t('wallDesign.pageName')}><input className={small} value={cur.name} maxLength={60} onChange={(e) => updPage({ name: e.target.value })} placeholder={t('wallDesign.page')} /></Field>
                <Field label={t('wallDesign.seconds')} hint={design.pages.length < 2 ? t('wallDesign.secondsHint') : undefined}>
                  <input type="number" min={5} max={600} className={small} value={cur.seconds} onChange={(e) => updPage({ seconds: Math.max(5, Math.min(600, Number(e.target.value) || 30)) })} />
                </Field>
                <div className="flex gap-2">
                  <Button variant="secondary" className="!min-h-9 flex-1" disabled={page === 0} onClick={() => { setDesign((d) => { const ps = [...d.pages];[ps[page - 1], ps[page]] = [ps[page], ps[page - 1]]; return { ...d, pages: ps } }); setPage(page - 1) }}>←</Button>
                  <Button variant="secondary" className="!min-h-9 flex-1" disabled={page >= design.pages.length - 1} onClick={() => { setDesign((d) => { const ps = [...d.pages];[ps[page + 1], ps[page]] = [ps[page], ps[page + 1]]; return { ...d, pages: ps } }); setPage(page + 1) }}>→</Button>
                </div>
                <Button variant="ghost" className="!min-h-9 text-danger" disabled={design.pages.length < 2}
                  onClick={() => { if (window.confirm(t('wallDesign.deletePageConfirm'))) { setDesign((d) => ({ ...d, pages: d.pages.filter((_, i) => i !== page) })); setPage(Math.max(0, page - 1)) } }}>
                  <Trash2 className="size-4" aria-hidden />{t('wallDesign.deletePage')}
                </Button>
                <p className="rounded-xl border border-line bg-tint/[0.03] p-3 text-xs leading-relaxed text-muted">{t('wallDesign.selectHint')}</p>
              </>
            )}
          </section>
        </div>
      )}

      {tab === 'style' && <StylePanel design={design} data={data} name={s.name} now={now} onStyle={updStyle} onTemplate={(n) => { applyTemplate(n, false); setTab('layout') }} onError={setError} />}

      {tab === 'access' && <AccessPanel s={s} set={(p) => { setS((x) => ({ ...x, ...p })); setDirty(true) }} settings={settings} address={address} onNewKey={newKey} />}

      {preview && (
        <div className="fixed inset-0 z-[100] bg-black" role="dialog" aria-modal aria-label={t('wallDesign.preview')}>
          <DesignedScreen data={data} design={design} name={s.name} />
          <Button variant="secondary" className="absolute right-4 top-4" onClick={() => setPreview(false)} autoFocus><Minimize2 className="size-4" aria-hidden />{t('common.close')}</Button>
        </div>
      )}
    </div>
  )
}

function GhostLayer({ grid, g, cols, rows, label }: { grid: React.RefObject<HTMLDivElement | null>; g: { x: number; y: number; w: number; h: number }; cols: number; rows: number; label: string }) {
  const el = grid.current
  const host = el?.closest('.kt-cq')
  if (!el || !host) return null
  const r = el.getBoundingClientRect()
  const h = host.getBoundingClientRect()
  const w = Math.min(g.w, cols - g.x)
  const hh = Math.min(g.h, rows - g.y)
  return (
    <div className="absolute grid place-items-center rounded-xl border-2 border-dashed border-accent/70 bg-accent/10 text-xs font-medium text-accent-ink"
      style={{ left: r.left - h.left + (g.x / cols) * r.width, top: r.top - h.top + (g.y / rows) * r.height, width: (w / cols) * r.width - 6, height: (hh / rows) * r.height - 6 }}>
      {label}
    </div>
  )
}

function TileProps({ tile, queues, onChange, onDelete, onDuplicate, onError, cols, rows, others }: {
  tile: WallTile; queues: WallSettings['queues']; onChange: (p: Partial<WallTile>) => void; onDelete: () => void; onDuplicate: () => void; onError: (e: unknown) => void
  cols: number; rows: number; others: WallTile[]
}) {
  const { t } = useTranslation()
  const Icon = tileIcon[tile.type]
  const metric = (tile.metric || (tile.type === 'compare' ? 'answered' : 'waiting')) as WallMetric
  const info = metricInfo[metric]
  const showPeriod = periodTiles.includes(tile.type) && (tile.type === 'donut' || !info.live)
  const unit = info.kind === 'time' ? 's' : info.kind === 'pct' ? '%' : ''
  const better = info.better === 'higher'
  const pos = (k: 'x' | 'y' | 'w' | 'h', v: number) => {
    const r = { x: tile.x, y: tile.y, w: tile.w, h: tile.h, [k]: v }
    r.w = Math.max(1, Math.min(cols, r.w)); r.h = Math.max(1, Math.min(rows, r.h))
    r.x = Math.max(0, Math.min(cols - r.w, r.x)); r.y = Math.max(0, Math.min(rows - r.h, r.y))
    if (!overlaps(others, r, tile.id)) onChange(r)
  }
  return (
    <>
      <div className="flex items-center gap-2">
        <b className="flex-1 font-display text-[15px]">{t('wallDesign.properties')}</b>
        <span className="inline-flex items-center gap-1 rounded-full border border-accent/45 bg-accent/12 px-2.5 py-0.5 text-xs text-ink"><Icon className="size-3" aria-hidden />{t(`wallDesign.tile.${tile.type}`)}</span>
      </div>
      <Field label={t('wallDesign.titleField')}><input className={small} value={tile.title} maxLength={60} placeholder={tileTitle({ ...tile, title: '' }, t)} onChange={(e) => onChange({ title: e.target.value })} /></Field>
      {metricTiles.includes(tile.type) && (
        <Field label={t('wallDesign.metricField')}>
          <select className={small} value={metric} onChange={(e) => onChange({ metric: e.target.value as WallMetric })}>
            <optgroup label={t('wallDesign.liveMetrics')}>{metrics.filter((m) => metricInfo[m].live).map((m) => <option key={m} value={m}>{t(`wallDesign.metric.${m}`)}</option>)}</optgroup>
            <optgroup label={t('wallDesign.periodMetrics')}>{metrics.filter((m) => !metricInfo[m].live).map((m) => <option key={m} value={m}>{t(`wallDesign.metric.${m}`)}</option>)}</optgroup>
          </select>
        </Field>
      )}
      {queueTiles.includes(tile.type) && (
        <Field label={t('wallboard.queues')} hint={tile.type === 'queue' ? t('wallDesign.queueTileHint') : undefined}>
          <div className="flex flex-wrap gap-1.5">
            <button type="button" aria-pressed={!tile.queues.length} className={chip(!tile.queues.length)} onClick={() => onChange({ queues: [] })}>{t('wallboard.allQueues')}</button>
            {queues.map((q) => {
              const on = tile.queues.includes(q.id)
              return <button key={q.id} type="button" aria-pressed={on} className={chip(on)} onClick={() => onChange({ queues: on ? tile.queues.filter((x) => x !== q.id) : [...tile.queues, q.id] })}>{q.name}</button>
            })}
          </div>
        </Field>
      )}
      {showPeriod && (
        <Field label={t('wallDesign.periodField')}>
          <Seg label={t('wallDesign.periodField')} value={tile.period} onChange={(v) => onChange({ period: v })} options={periods.map((p) => ({ key: p, label: t(`wallDesign.period.${p}`) }))} />
        </Field>
      )}
      {figureTiles.includes(tile.type) && (
        <Field label={t('wallDesign.thresholds')}>
          <Switch label={t('wallDesign.fromSla')} checked={tile.fromSla} onChange={(v) => onChange({ fromSla: v })} />
          {!tile.fromSla && (
            <div className="flex flex-col gap-1.5">
              <div className="grid grid-cols-[12px_1fr_92px] items-center gap-2 text-[13px] text-ink-2">
                <i className="block size-2.5 rounded-[3px] bg-warn" />{t(better ? 'wallDesign.warnBelow' : 'wallDesign.warnAbove')}
                <span className="relative"><input type="number" min={0} className={`${small} pr-6 font-mono`} value={tile.warn || ''} placeholder="–" onChange={(e) => onChange({ warn: Math.max(0, Number(e.target.value) || 0) })} /><span className="absolute right-2 top-2 text-xs text-subtle">{unit}</span></span>
              </div>
              <div className="grid grid-cols-[12px_1fr_92px] items-center gap-2 text-[13px] text-ink-2">
                <i className="block size-2.5 rounded-[3px] bg-danger" />{t(better ? 'wallDesign.alarmBelow' : 'wallDesign.alarmAbove')}
                <span className="relative"><input type="number" min={0} className={`${small} pr-6 font-mono`} value={tile.alarm || ''} placeholder="–" onChange={(e) => onChange({ alarm: Math.max(0, Number(e.target.value) || 0) })} /><span className="absolute right-2 top-2 text-xs text-subtle">{unit}</span></span>
              </div>
              <span className="text-xs text-subtle">{t('wallDesign.thresholdHint')}</span>
            </div>
          )}
        </Field>
      )}
      {tile.type === 'kpi' && (
        <Field label={t('wallDesign.display')}>
          <Seg label={t('wallDesign.display')} value={tile.display} onChange={(v) => onChange({ display: v })} options={[{ key: 'big', label: t('wallDesign.displayBig') }, { key: 'trend', label: t('wallDesign.displayTrend') }, { key: 'gauge', label: t('wallDesign.displayGauge') }]} />
        </Field>
      )}
      {(tile.type === 'text' || tile.type === 'ticker') && (
        <Field label={t('wallDesign.textField')} hint={tile.type === 'ticker' ? t('wallDesign.tickerHint') : undefined}>
          <textarea rows={4} maxLength={1000} className={`${small} py-2`} value={tile.text} onChange={(e) => onChange({ text: e.target.value })} />
        </Field>
      )}
      {tile.type === 'image' && (
        <Field label={t('wallDesign.imageField')}>
          {tile.image && <img src={tile.image} alt="" className="max-h-24 w-full rounded-lg border border-line object-contain" />}
          <ImagePick label={t('wallDesign.upload')} onPick={(image) => onChange({ image })} onError={onError} />
          {tile.image && <button type="button" className="text-left text-xs text-danger" onClick={() => onChange({ image: '' })}>{t('wallDesign.removeImage')}</button>}
        </Field>
      )}
      <Field label={t('wallDesign.options')}>
        {figureTiles.includes(tile.type) && <Switch label={t('wallDesign.blink')} checked={tile.blink} onChange={(v) => onChange({ blink: v })} />}
        <Switch label={t('wallDesign.showLabel')} checked={!tile.hideLabel} onChange={(v) => onChange({ hideLabel: !v })} />
        {figureTiles.includes(tile.type) && !info.live && <Switch label={t('wallDesign.compare')} checked={tile.compare} onChange={(v) => onChange({ compare: v })} />}
      </Field>
      <Field label={t('wallDesign.position')}>
        <div className="grid grid-cols-4 gap-1.5">
          {(['x', 'y', 'w', 'h'] as const).map((k) => (
            <label key={k} className="relative">
              <span className="absolute left-2 top-2 font-mono text-xs text-subtle">{t(`wallDesign.pos.${k}`)}</span>
              <input type="number" aria-label={t(`wallDesign.posLabel.${k}`)} className={`${small} pl-6 font-mono`} value={k === 'x' || k === 'y' ? tile[k] + 1 : tile[k]}
                onChange={(e) => pos(k, (Number(e.target.value) || 1) - (k === 'x' || k === 'y' ? 1 : 0))} />
            </label>
          ))}
        </div>
      </Field>
      <div className="mt-auto flex gap-2">
        <Button variant="secondary" className="!min-h-9 flex-1" onClick={onDuplicate}><Copy className="size-4" aria-hidden />{t('wallDesign.duplicate')}</Button>
        <Button variant="secondary" className="!min-h-9 flex-1 !text-danger" onClick={onDelete}><Trash2 className="size-4" aria-hidden />{t('wallDesign.remove')}</Button>
      </div>
    </>
  )
}

const accents = ['', '#818CF8', '#34D399', '#F472B6', '#FB923C', '#FACC15']

function OptionCards<T extends string>({ value, options, onChange, label }: { value: T; options: { key: T; label: string; preview: string }[]; onChange: (v: T) => void; label: string }) {
  return (
    <div role="radiogroup" aria-label={label} className="grid grid-cols-3 gap-2">
      {options.map((o) => (
        <button key={o.key} type="button" role="radio" aria-checked={value === o.key} onClick={() => onChange(o.key)}
          className={`flex flex-col gap-2 rounded-xl border p-2.5 text-left text-[13px] transition ${value === o.key ? 'border-accent/60 text-ink shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-accent)_14%,transparent)]' : 'border-line text-ink-2 hover:text-ink'}`}>
          <span className={`block h-11 rounded-lg border border-line ${o.preview}`} />{o.label}
        </button>
      ))}
    </div>
  )
}

function StylePanel({ design, data, name, now, onStyle, onTemplate, onError }: {
  design: WallDesign; data: WallData; name: string; now: Date; onStyle: (p: Partial<WallStyle>) => void; onTemplate: (n: TemplateName) => void; onError: (e: unknown) => void
}) {
  const { t } = useTranslation()
  const s = { ...defaultStyle, ...design.style }
  const [hex, setHex] = useState(s.accent)
  useEffect(() => setHex(s.accent), [s.accent])
  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,1.05fr)]">
      <section className="card flex flex-col gap-5 p-5">
        <b className="font-display text-base">{t('wallDesign.styleTitle')}</b>
        <Field label={t('wallDesign.theme')} hint={t('wallDesign.themeHint')}>
          <OptionCards label={t('wallDesign.theme')} value={s.theme} onChange={(v) => onStyle({ theme: v })} options={[
            { key: 'dark', label: t('wallDesign.themeDark'), preview: 'bg-[#070B14] bg-[radial-gradient(60px_30px_at_70%_0,rgb(34_211_238/.3),transparent)]' },
            { key: 'light', label: t('wallDesign.themeLight'), preview: 'bg-[#F4F7FB]' },
            { key: 'auto', label: t('wallDesign.themeAuto'), preview: 'bg-[linear-gradient(100deg,#F4F7FB_50%,#070B14_50%)]' },
          ]} />
        </Field>
        <Field label={t('wallDesign.accent')} hint={t('wallDesign.accentHint')}>
          <div className="flex flex-wrap items-center gap-2">
            {accents.map((c) => (
              <button key={c || 'default'} type="button" aria-pressed={s.accent === c} aria-label={c || t('wallDesign.accentDefault')} title={c || t('wallDesign.accentDefault')} onClick={() => onStyle({ accent: c })}
                className={`size-8 rounded-[9px] border-2 ${s.accent === c ? 'border-ink' : 'border-transparent'}`} style={{ background: c || 'linear-gradient(135deg,#22D3EE 50%,#0891B2 50%)' }} />
            ))}
            <input aria-label={t('wallDesign.accentHex')} value={hex} placeholder="#22D3EE" maxLength={7} className={`${small} !w-28 font-mono`}
              onChange={(e) => { setHex(e.target.value); if (/^#[0-9a-fA-F]{6}$/.test(e.target.value)) onStyle({ accent: e.target.value.toUpperCase() }) }} />
          </div>
        </Field>
        <div className="grid gap-5 sm:grid-cols-2">
          <Field label={t('wallDesign.logo')}>
            <Seg label={t('wallDesign.logo')} value={s.logo} onChange={(v) => onStyle({ logo: v })} options={[{ key: 'kiestoon', label: 'Kiestoon' }, { key: 'own', label: t('wallDesign.logoOwn') }, { key: 'none', label: t('wallDesign.logoNone') }]} />
            {s.logo === 'own' && <ImagePick label={t('wallDesign.uploadLogo')} onPick={(logoImage) => onStyle({ logoImage })} onError={onError} />}
          </Field>
          <Field label={t('wallDesign.language')}>
            <Seg label={t('wallDesign.language')} value={s.language} onChange={(v) => onStyle({ language: v })} options={[{ key: 'nl', label: 'Nederlands' }, { key: 'en', label: 'English' }, { key: 'browser', label: t('wallDesign.langBrowser') }]} />
          </Field>
        </div>
        <Field label={t('wallDesign.background')}>
          <OptionCards label={t('wallDesign.background')} value={s.background} onChange={(v) => onStyle({ background: v })} options={[
            { key: 'glow', label: t('wallDesign.bgGlow'), preview: 'bg-bg bg-[radial-gradient(70px_30px_at_70%_0,color-mix(in_srgb,var(--color-accent)_35%,transparent),transparent)]' },
            { key: 'grid', label: t('wallDesign.bgGrid'), preview: 'bg-inset bg-[radial-gradient(var(--color-subtle)_1px,transparent_1.3px)] bg-[size:8px_8px]' },
            { key: 'plain', label: t('wallDesign.bgPlain'), preview: 'bg-bg' },
          ]} />
        </Field>
        <Field label={t('wallDesign.tileStyle')}>
          <OptionCards label={t('wallDesign.tileStyle')} value={s.tiles} onChange={(v) => onStyle({ tiles: v })} options={[
            { key: 'glass', label: t('wallDesign.tilesGlass'), preview: 'card' },
            { key: 'solid', label: t('wallDesign.tilesSolid'), preview: 'bg-surface' },
            { key: 'outlined', label: t('wallDesign.tilesOutlined'), preview: '!border-accent/50 !border-[1.5px]' },
          ]} />
        </Field>
        <div className="grid gap-5 sm:grid-cols-2">
          <Field label={t('wallDesign.scale')}>
            <div className="flex items-center gap-3">
              <input type="range" min={60} max={180} step={5} value={s.scale} aria-label={t('wallDesign.scale')} onChange={(e) => onStyle({ scale: Number(e.target.value) })} className="flex-1 accent-[var(--color-accent)]" />
              <span className="w-12 text-right font-mono text-sm">{s.scale}%</span>
            </div>
          </Field>
          <Field label={t('wallDesign.density')}>
            <Seg label={t('wallDesign.density')} value={s.density} onChange={(v) => onStyle({ density: v })} options={[{ key: 'roomy', label: t('wallDesign.roomy') }, { key: 'normal', label: t('wallDesign.normal') }, { key: 'compact', label: t('wallDesign.compact') }]} />
          </Field>
        </div>
        <div className="grid gap-5 sm:grid-cols-2">
          <Field label={t('wallDesign.header')}>
            <Switch label={t('wallDesign.showClock')} checked={s.clock} onChange={(v) => onStyle({ clock: v })} />
            <Switch label={t('wallDesign.showDate')} checked={s.date} onChange={(v) => onStyle({ date: v })} />
            <Switch label={t('wallDesign.showName')} checked={s.name} onChange={(v) => onStyle({ name: v })} />
            <Switch label={t('wallDesign.showDots')} checked={s.dots} onChange={(v) => onStyle({ dots: v })} />
          </Field>
          <Field label={t('wallDesign.onAlarm')}>
            <Switch label={t('wallDesign.alarmBanner')} checked={s.alarmBanner} onChange={(v) => onStyle({ alarmBanner: v })} />
            <Switch label={t('wallDesign.alarmSound')} checked={s.alarmSound} onChange={(v) => onStyle({ alarmSound: v })} />
            <Switch label={t('wallDesign.alarmJump')} checked={s.alarmJump} onChange={(v) => onStyle({ alarmJump: v })} />
          </Field>
        </div>
        <p className="text-xs text-subtle">{t('wallDesign.rhisNote')}</p>
      </section>
      <section className="flex flex-col gap-4">
        <div className="card p-4">
          <div className="mb-3 flex items-center justify-between"><b className="font-display text-[15px]">{t('wallDesign.livePreview')}</b><Badge>{design.format === 'portrait' ? '1080×1920' : '1920×1080'}</Badge></div>
          <BoardFrame design={design} now={now} className={`rounded-xl border border-line ${design.format === 'portrait' ? 'mx-auto aspect-[9/16] max-h-[60vh]' : 'aspect-video w-full'}`}>
            <Board data={data} design={design} name={name} page={0} pages={design.pages.length} now={now} />
          </BoardFrame>
        </div>
        <div className="card p-4">
          <b className="font-display text-[15px]">{t('wallDesign.templates')}</b>
          <p className="mt-0.5 text-xs text-subtle">{t('wallDesign.templatesHint')}</p>
          <div className="mt-3 grid grid-cols-2 gap-2.5 sm:grid-cols-3">
            {templates.map((n) => {
              const p = templatePages(n, '')[0]
              return (
                <button key={n} type="button" onClick={() => onTemplate(n)} className="flex flex-col gap-1.5 rounded-xl border border-line p-2 text-left text-[12.5px] text-ink-2 transition hover:border-accent/50 hover:text-ink">
                  <span className="relative block aspect-video rounded-md bg-inset">
                    {p.tiles.map((x) => <i key={x.id} className="absolute rounded-[3px] bg-accent/25" style={{ left: `${(x.x / 12) * 100 + 1}%`, top: `${(x.y / 8) * 100 + 2}%`, width: `${(x.w / 12) * 100 - 2}%`, height: `${(x.h / 8) * 100 - 4}%` }} />)}
                  </span>
                  {t(`wallDesign.template.${n}`)}
                </button>
              )
            })}
          </div>
        </div>
      </section>
    </div>
  )
}

function AccessPanel({ s, set, settings, address, onNewKey }: { s: WallScreen; set: (p: Partial<WallScreen>) => void; settings: WallSettings; address: string; onNewKey: () => void }) {
  const { t } = useTranslation()
  const [nets, setNets] = useState(s.networks.join('\n'))
  const toggleQ = (id: number) => set({ queues: s.queues.includes(id) ? s.queues.filter((x) => x !== id) : [...s.queues, id] })
  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <section className="card flex flex-col gap-5 p-5">
        <b className="font-display text-base">{t('wallDesign.addressTitle')}</b>
        <Field label={t('common.name')}><input className={small} value={s.name} maxLength={80} onChange={(e) => set({ name: e.target.value })} /></Field>
        <Field label={t('wallDesign.ownAddress')} hint={t('wallDesign.ownAddressHint')}>
          <div className="flex min-h-9 items-center overflow-hidden rounded-[9px] border border-line bg-field focus-within:border-accent/60">
            <span className="truncate pl-2.5 font-mono text-[13px] text-subtle">{window.location.origin}/wall/</span>
            <input name="slug" aria-label={t('wallDesign.ownAddress')} className="min-h-9 min-w-0 flex-1 bg-transparent pr-2.5 font-mono text-[13px] text-ink outline-none" value={s.slug} maxLength={40}
              onChange={(e) => set({ slug: e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, '-') })} />
          </div>
        </Field>
        <Field label={t('wallDesign.access')}>
          <div role="radiogroup" aria-label={t('wallDesign.access')} className="flex flex-col gap-2">
            {(['open', 'pin', 'link'] as const).map((a) => (
              <button key={a} type="button" role="radio" aria-checked={s.access === a} onClick={() => set({ access: a })}
                className={`flex flex-col gap-0.5 rounded-xl border p-3 text-left ${s.access === a ? 'border-accent/60 bg-accent/[0.06]' : 'border-line hover:border-accent/35'}`}>
                <span className="text-sm font-medium text-ink">{t(`wallDesign.accessMode.${a}`)}</span>
                <span className="text-xs text-muted">{t(`wallDesign.accessHint.${a}`)}</span>
              </button>
            ))}
          </div>
        </Field>
        {s.access === 'pin' && (
          <Field label={t('wallDesign.pin')} hint={s.hasPin ? t('wallDesign.pinKept') : t('wallDesign.pinHint')}>
            <input name="pin" inputMode="numeric" autoComplete="off" className={`${small} !w-40 font-mono tracking-[0.3em]`} value={s.pin ?? ''} maxLength={10} placeholder={s.hasPin ? '••••' : ''}
              onChange={(e) => set({ pin: e.target.value.replace(/\D/g, '') })} />
          </Field>
        )}
        {s.access !== 'link' && <CopyField label={t('wallDesign.yourAddress')} value={address} />}
        <Field label={t('wallDesign.networks')} hint={t('wallDesign.networksHint')}>
          <textarea name="networks" rows={3} className={`${small} py-2 font-mono`} value={nets} placeholder="192.168.10.0/24"
            onChange={(e) => { setNets(e.target.value); set({ networks: e.target.value.split(/[\s,;]+/).filter(Boolean) }) }} />
        </Field>
      </section>
      <section className="card flex flex-col gap-5 p-5">
        <b className="font-display text-base">{t('wallDesign.dataTitle')}</b>
        <Field label={t('wallboard.queues')} hint={t('wallDesign.screenQueuesHint')}>
          <div className="flex flex-wrap gap-1.5">
            <button type="button" aria-pressed={!s.queues.length} className={chip(!s.queues.length)} onClick={() => set({ queues: [] })}>{t('wallboard.allQueues')}</button>
            {settings.queues.map((q) => <button key={q.id} type="button" aria-pressed={s.queues.includes(q.id)} className={chip(s.queues.includes(q.id))} onClick={() => toggleQ(q.id)}>{q.name}</button>)}
          </div>
        </Field>
        <Switch label={t('wallboard.hideNames')} checked={s.hideNames} onChange={(v) => set({ hideNames: v })} />
        <p className="-mt-3 text-xs text-subtle">{t('wallboard.hideNamesHint')}</p>
        <div className="flex flex-col gap-2 rounded-xl border border-line p-4">
          <div className="flex items-center gap-2 text-sm font-medium text-ink"><KeyRound className="size-4 text-accent" aria-hidden />{t('wallDesign.secretLink')}</div>
          <p className="text-xs text-muted">{t('wallDesign.secretLinkHint')}</p>
          <CopyField label={t('wallboard.link')} value={`${window.location.origin}/wall/${s.key}`} secret />
          <div><Button variant="ghost" className="!min-h-9" onClick={onNewKey}>{t('wallboard.newKey')}</Button></div>
        </div>
        <p className="text-xs text-subtle">{t('wallDesign.saveAccessHint')}</p>
      </section>
    </div>
  )
}
