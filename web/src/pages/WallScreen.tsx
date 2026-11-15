import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2, MonitorX } from 'lucide-react'
import { Mark } from '../components/brand'
import { api, ApiError } from '../lib/api'
import type { WallData } from '../lib/types'
import { WallView } from './wallboard/WallView'

/** The wallboard on a tv: /wall/<key> shows it without signing in. */
export function WallScreen({ wallKey }: { wallKey: string }) {
  const { t } = useTranslation()
  const [data, setData] = useState<WallData | null>(null)
  const [gone, setGone] = useState(false)
  const [stale, setStale] = useState(false)
  useEffect(() => {
    let stop = false
    const load = async () => {
      try {
        const d = await api<WallData>('GET', `/wall/${encodeURIComponent(wallKey)}`)
        if (stop) return
        setData(d)
        setGone(false)
        setStale(false)
      } catch (e) {
        if (stop) return
        if (e instanceof ApiError && (e.status === 404 || e.status === 429)) setGone(true)
        else setStale(true)
      }
    }
    load()
    const id = window.setInterval(load, 3000)
    return () => { stop = true; window.clearInterval(id) }
  }, [wallKey])
  // The tv link chooses its own theme; keep the screen awake when the browser allows it.
  useEffect(() => {
    if (data?.screen?.theme) document.documentElement.dataset.theme = data.screen.theme
  }, [data?.screen?.theme])
  useEffect(() => {
    let lock: { release: () => Promise<void> } | null = null
    const nav = navigator as Navigator & { wakeLock?: { request: (t: 'screen') => Promise<{ release: () => Promise<void> }> } }
    const req = () => nav.wakeLock?.request('screen').then((l) => { lock = l }).catch(() => undefined)
    req()
    const onVis = () => { if (document.visibilityState === 'visible') req() }
    document.addEventListener('visibilitychange', onVis)
    return () => { document.removeEventListener('visibilitychange', onVis); lock?.release().catch(() => undefined) }
  }, [])
  useEffect(() => { document.title = data?.screen?.name ? `${data.screen.name} · Kiestoon` : 'Kiestoon wallboard' }, [data?.screen?.name])

  if (gone) {
    return (
      <main className="grid min-h-screen place-items-center bg-bg p-8 text-center text-ink">
        <div className="flex max-w-md flex-col items-center gap-4">
          <MonitorX className="size-10 text-muted" aria-hidden />
          <h1 className="font-display text-2xl font-semibold">{t('wallboard.linkGone')}</h1>
          <p className="text-muted">{t('wallboard.linkGoneText')}</p>
        </div>
      </main>
    )
  }
  if (!data) {
    return <main className="flex min-h-screen items-center justify-center gap-3 bg-bg text-muted"><Mark /><Loader2 className="size-5 animate-spin text-accent" aria-hidden /></main>
  }
  return (
    <main className="h-screen overflow-hidden bg-bg bg-[radial-gradient(var(--color-grid-dot)_1px,transparent_1px)] bg-[size:22px_22px] p-[1.6vw] text-[clamp(11px,0.85vw,28px)]">
      {stale && <div className="fixed left-1/2 top-2 z-10 -translate-x-1/2 rounded-full border border-warn/40 bg-surface px-4 py-1 text-sm text-warn">{t('wallboard.reconnecting')}</div>}
      <WallView data={data} layouts={data.screen?.layouts ?? ['overview']} rotate={data.screen?.rotate ?? 0} tv />
    </main>
  )
}
