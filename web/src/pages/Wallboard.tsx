import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Maximize2, Minimize2, MonitorPlay } from 'lucide-react'
import { Empty, formatDuration, useApi } from '../components/kit'
import { Button, Card, ErrorMessage, PageHeader } from '../components/ui'
import type { QueueView } from '../lib/types'

/** A large-print view of the queues for a screen on the wall. */
export function Wallboard() {
  const { t, i18n } = useTranslation()
  const queues = useApi<QueueView[]>('/reception/queues', 3000)
  const [full, setFull] = useState(false)
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    const id = window.setInterval(() => setNow(new Date()), 1000)
    const onFs = () => setFull(!!document.fullscreenElement)
    document.addEventListener('fullscreenchange', onFs)
    return () => { window.clearInterval(id); document.removeEventListener('fullscreenchange', onFs) }
  }, [])
  const toggle = () => (document.fullscreenElement ? document.exitFullscreen() : document.getElementById('wallboard')?.requestFullscreen())
  const list = (queues.data ?? []).filter((q) => q.enabled)
  const totals = list.reduce((a, q) => ({
    waiting: a.waiting + (q.live?.calls ?? 0),
    answered: a.answered + (q.live?.completed ?? 0),
    abandoned: a.abandoned + (q.live?.abandoned ?? 0),
  }), { waiting: 0, answered: 0, abandoned: 0 })

  return (
    <>
      {!full && (
        <PageHeader title={t('wallboard.title')} subtitle={t('wallboard.subtitle')}>
          <Button variant="secondary" onClick={toggle}><Maximize2 className="size-4" aria-hidden />{t('wallboard.fullscreen')}</Button>
        </PageHeader>
      )}
      <ErrorMessage error={queues.error} />
      <div id="wallboard" className={`flex flex-col gap-5 ${full ? 'min-h-screen overflow-auto bg-bg p-8' : ''}`}>
        <div className="flex flex-wrap items-end justify-between gap-4">
          <div className="flex flex-wrap gap-8">
            <Big label={t('wallboard.waitingNow')} value={totals.waiting} tone={totals.waiting > 0 ? 'warn' : undefined} />
            <Big label={t('wallboard.answeredToday')} value={totals.answered} tone="ok" />
            <Big label={t('wallboard.abandonedToday')} value={totals.abandoned} tone={totals.abandoned > 0 ? 'danger' : undefined} />
          </div>
          <div className="flex items-center gap-3">
            <div className="text-right font-mono text-4xl text-ink tabular-nums">{now.toLocaleTimeString(i18n.language, { hour: '2-digit', minute: '2-digit' })}</div>
            {full && <Button variant="ghost" onClick={toggle} aria-label={t('wallboard.exitFullscreen')}><Minimize2 className="size-4" aria-hidden /></Button>}
          </div>
        </div>
        {queues.data && list.length === 0 && <Card><Empty icon={<MonitorPlay className="size-7" />} title={t('wallboard.emptyTitle')} text={t('wallboard.emptyText')} /></Card>}
        <div className="grid gap-5 lg:grid-cols-2 2xl:grid-cols-3">
          {list.map((q) => <QueueTile key={q.id} q={q} />)}
        </div>
      </div>
    </>
  )
}

function Big({ label, value, tone }: { label: string; value: number | string; tone?: 'ok' | 'warn' | 'danger' }) {
  const color = tone === 'ok' ? 'text-ok' : tone === 'warn' ? 'text-warn' : tone === 'danger' ? 'text-danger' : 'text-ink'
  return (
    <div>
      <div className="text-sm uppercase tracking-[0.1em] text-muted">{label}</div>
      <div className={`font-mono text-6xl font-medium tabular-nums ${color}`}>{value}</div>
    </div>
  )
}

function QueueTile({ q }: { q: QueueView }) {
  const { t } = useTranslation()
  const l = q.live
  const agents = l?.agents ?? []
  const free = agents.filter((a) => !a.paused && !a.inCall && a.status !== 'unavailable').length
  const busy = agents.filter((a) => a.inCall).length
  const paused = agents.filter((a) => a.paused).length
  const waiting = l?.calls ?? 0
  const longest = l?.longestWait ?? 0
  const alarm = waiting > 0 && (free === 0 || (q.maxWait > 0 && longest > q.maxWait * 0.7))
  const sl = l?.serviceLevel ?? 0
  return (
    <section className={`card flex flex-col gap-4 p-6 ${alarm ? 'shadow-[inset_0_0_0_2px_rgb(248_113_113_/_0.6)]' : ''}`}>
      <div className="flex items-baseline justify-between gap-3">
        <h2 className="font-display text-2xl font-semibold">{q.name}</h2>
        <span className="font-mono text-lg text-subtle">{q.number}</span>
      </div>
      <div className="grid grid-cols-2 gap-4">
        <Metric label={t('wallboard.waiting')} value={waiting} tone={waiting ? (alarm ? 'danger' : 'warn') : 'ok'} />
        <Metric label={t('wallboard.longestWait')} value={formatDuration(longest)} tone={alarm ? 'danger' : undefined} />
      </div>
      <div className="grid grid-cols-3 gap-3 rounded-xl border border-line bg-surface-2/60 p-3 text-center">
        <Small label={t('wallboard.free')} value={free} tone="ok" />
        <Small label={t('wallboard.inCall')} value={busy} tone="accent" />
        <Small label={t('wallboard.paused')} value={paused} tone="warn" />
      </div>
      <div className="grid grid-cols-3 gap-3 text-center">
        <Small label={t('wallboard.answered')} value={l?.completed ?? 0} />
        <Small label={t('wallboard.abandoned')} value={l?.abandoned ?? 0} tone={(l?.abandoned ?? 0) > 0 ? 'danger' : undefined} />
        {(l?.completed ?? 0) + (l?.abandoned ?? 0) === 0
          ? <Small label={t('wallboard.serviceLevel')} value="–" />
          : <Small label={t('wallboard.serviceLevel')} value={`${Math.round(sl)}%`} tone={sl >= 80 ? 'ok' : sl >= 60 ? 'warn' : 'danger'} />}
      </div>
      {(l?.callers?.length ?? 0) > 0 && (
        <ol className="flex flex-col gap-1 border-t border-line pt-3 text-sm">
          {l!.callers!.slice(0, 5).map((c) => (
            <li key={c.position} className="flex justify-between gap-3 font-mono">
              <span className="truncate text-ink-2">{c.position}. {c.name || c.callerId}</span>
              <span className="text-warn tabular-nums">{formatDuration(c.wait)}</span>
            </li>
          ))}
        </ol>
      )}
    </section>
  )
}

const toneCls = (tone?: string) => (tone === 'ok' ? 'text-ok' : tone === 'warn' ? 'text-warn' : tone === 'danger' ? 'text-danger' : tone === 'accent' ? 'text-accent' : 'text-ink')

function Metric({ label, value, tone }: { label: string; value: number | string; tone?: string }) {
  return (
    <div>
      <div className="text-xs uppercase tracking-[0.1em] text-muted">{label}</div>
      <div className={`font-mono text-5xl font-medium tabular-nums ${toneCls(tone)}`}>{value}</div>
    </div>
  )
}

function Small({ label, value, tone }: { label: string; value: number | string; tone?: string }) {
  return (
    <div>
      <div className={`font-mono text-2xl tabular-nums ${toneCls(tone)}`}>{value}</div>
      <div className="text-[11px] uppercase tracking-[0.08em] text-subtle">{label}</div>
    </div>
  )
}
