import { useEffect, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2, LockKeyhole, MonitorX } from 'lucide-react'
import { Mark } from '../components/brand'
import { api, ApiError } from '../lib/api'
import type { WallData } from '../lib/types'
import { WallView } from './wallboard/WallView'
import { DesignedScreen } from './wallboard/Board'
import { themeNow } from './wallboard/design'

const keyStore = (ref: string) => `kiestoon.wall.${ref}`
function storedKey(ref: string) { try { return localStorage.getItem(keyStore(ref)) ?? '' } catch { return '' } }

/** The wallboard on a tv: /wall/<key> or the screen's own address /wall/<name> shows it without signing in. */
export function WallScreen({ wallKey }: { wallKey: string }) {
  const { t, i18n } = useTranslation()
  const [data, setData] = useState<WallData | null>(null)
  const [gone, setGone] = useState<'' | 'gone' | 'network'>('')
  const [pin, setPin] = useState(false)
  const [stale, setStale] = useState(false)
  const [tick, setTick] = useState(0)
  useEffect(() => {
    let stop = false
    const load = async () => {
      const key = storedKey(wallKey)
      try {
        const d = await api<WallData>('GET', `/wall/${encodeURIComponent(wallKey)}`, undefined, key ? { 'X-Wall-Key': key } : undefined)
        if (stop) return
        setData(d)
        setGone('')
        setPin(false)
        setStale(false)
      } catch (e) {
        if (stop) return
        if (e instanceof ApiError && e.status === 401) { try { localStorage.removeItem(keyStore(wallKey)) } catch { /* storage blocked */ } setPin(true) }
        else if (e instanceof ApiError && e.status === 403) setGone('network')
        else if (e instanceof ApiError && (e.status === 404 || e.status === 429)) setGone('gone')
        else setStale(true)
      }
    }
    load()
    const id = window.setInterval(load, 3000)
    return () => { stop = true; window.clearInterval(id) }
  }, [wallKey, tick])
  const design = data?.screen?.design
  // A tv that never chose a language follows its browser (also on the pin screen).
  useEffect(() => {
    let saved = ''
    try { saved = localStorage.getItem('kiestoon.lang') ?? '' } catch { /* storage blocked */ }
    if (!saved && navigator.languages?.some((l) => l.toLowerCase().startsWith('nl'))) i18n.changeLanguage('nl')
  }, [i18n])
  // The tv link chooses its own theme and language; keep the screen awake when the browser allows it.
  useEffect(() => {
    const theme = design ? themeNow(design.style.theme, new Date()) : data?.screen?.theme
    if (theme) document.documentElement.dataset.theme = theme
  }, [data?.screen?.theme, design, data?.now])
  useEffect(() => {
    let lang = design?.style.language
    // "Browser" follows the language of the tv's browser (Dutch or else English).
    if (lang === 'browser') lang = navigator.languages?.some((l) => l.toLowerCase().startsWith('nl')) ? 'nl' : 'en'
    if (lang && i18n.language !== lang) i18n.changeLanguage(lang)
  }, [design?.style.language, i18n])
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
          <h1 className="font-display text-2xl font-semibold">{t(gone === 'network' ? 'wallDesign.networkBlocked' : 'wallboard.linkGone')}</h1>
          <p className="text-muted">{t(gone === 'network' ? 'wallDesign.networkBlockedText' : 'wallboard.linkGoneText')}</p>
        </div>
      </main>
    )
  }
  if (pin) return <PinScreen wallRef={wallKey} onOk={() => { setPin(false); setTick((x) => x + 1) }} />
  if (!data) {
    return <main className="flex min-h-screen items-center justify-center gap-3 bg-bg text-muted"><Mark /><Loader2 className="size-5 animate-spin text-accent" aria-hidden /></main>
  }
  if (design) {
    return (
      <main className="h-screen overflow-hidden bg-bg">
        {stale && <div className="fixed left-1/2 top-2 z-10 -translate-x-1/2 rounded-full border border-warn/40 bg-surface px-4 py-1 text-sm text-warn">{t('wallboard.reconnecting')}</div>}
        <DesignedScreen data={data} design={design} name={data.screen?.name ?? ''} />
      </main>
    )
  }
  return (
    <main className="h-screen overflow-hidden bg-bg bg-[radial-gradient(var(--color-grid-dot)_1px,transparent_1px)] bg-[size:22px_22px] p-[1.6vw] text-[clamp(11px,0.85vw,28px)]">
      {stale && <div className="fixed left-1/2 top-2 z-10 -translate-x-1/2 rounded-full border border-warn/40 bg-surface px-4 py-1 text-sm text-warn">{t('wallboard.reconnecting')}</div>}
      <WallView data={data} layouts={data.screen?.layouts ?? ['overview']} rotate={data.screen?.rotate ?? 0} tv />
    </main>
  )
}

/** Asks for the pin code of a screen once; the tv then remembers the screen's key. */
function PinScreen({ wallRef, onOk }: { wallRef: string; onOk: () => void }) {
  const { t } = useTranslation()
  const [pin, setPin] = useState('')
  const [busy, setBusy] = useState(false)
  const [wrong, setWrong] = useState(false)
  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setWrong(false)
    try {
      const r = await api<{ key: string }>('POST', `/wall/${encodeURIComponent(wallRef)}/pin`, { pin })
      try { localStorage.setItem(keyStore(wallRef), r.key) } catch { /* storage blocked: ask again next time */ }
      onOk()
    } catch {
      setWrong(true)
      setPin('')
    } finally {
      setBusy(false)
    }
  }
  return (
    <main className="grid min-h-screen place-items-center bg-bg p-8 text-ink">
      <form onSubmit={submit} className="card flex w-full max-w-sm flex-col items-center gap-4 p-8 text-center">
        <Mark size={44} />
        <LockKeyhole className="size-6 text-accent" aria-hidden />
        <h1 className="font-display text-2xl font-semibold">{t('wallDesign.pinTitle')}</h1>
        <p className="text-sm text-muted">{t('wallDesign.pinText')}</p>
        <input autoFocus inputMode="numeric" autoComplete="off" aria-label={t('wallDesign.pin')} value={pin} maxLength={10}
          onChange={(e) => setPin(e.target.value.replace(/\D/g, ''))}
          className="min-h-14 w-48 rounded-xl border border-line bg-field text-center font-mono text-2xl tracking-[0.4em] text-ink outline-none focus:border-accent/60 focus:ring-2 focus:ring-accent/20" />
        {wrong && <p role="alert" className="text-sm text-danger">{t('wallDesign.pinWrong')}</p>}
        <button type="submit" disabled={busy || pin.length < 4} className="min-h-11 w-48 rounded-[10px] bg-accent font-medium text-on-accent disabled:opacity-50">{t('wallDesign.pinGo')}</button>
      </form>
    </main>
  )
}
