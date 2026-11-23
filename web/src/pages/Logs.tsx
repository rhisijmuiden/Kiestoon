import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useSearchParams } from 'react-router'
import { Download, FileText, RefreshCw, Search } from 'lucide-react'
import { Badge, Empty, Select, Toggle, useApi } from '../components/kit'
import { Segmented } from '../components/pbx'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'

export type LogEntry = { at: string; level: string; module?: string; text: string }
type Source = { id: string; kind: 'file' | 'journal'; available: boolean; size?: number; modified?: string }

export function levelTone(l: string): 'danger' | 'warn' | 'accent' | 'muted' {
  if (l === 'error') return 'danger'
  if (l === 'warning') return 'warn'
  if (l === 'security' || l === 'notice') return 'accent'
  return 'muted'
}

/** Log lines, newest first, with the search words highlighted. */
export function LogLines({ entries, highlight = '' }: { entries: LogEntry[]; highlight?: string }) {
  const { t } = useTranslation()
  const words = highlight.trim().split(/\s+/).filter(Boolean)
  const re = words.length ? new RegExp(`(${words.map((w) => w.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|')})`, 'gi') : null
  const mark = (s: string) => re ? s.split(re).map((p, i) => i % 2 ? <mark key={i} className="rounded bg-accent/25 px-0.5 text-ink">{p}</mark> : p) : s
  return (
    <ol className="flex flex-col font-mono text-[12.5px] leading-relaxed">
      {entries.map((e, i) => (
        <li key={i} className="grid grid-cols-1 gap-x-3 border-t border-tint/[0.06] py-1.5 first:border-t-0 sm:grid-cols-[150px_112px_1fr]">
          <span className="whitespace-nowrap text-subtle">{e.at || '–'}</span>
          <span><Badge tone={levelTone(e.level)}>{t(`logs.level.${e.level}`, { defaultValue: e.level })}</Badge></span>
          <span className="min-w-0 whitespace-pre-wrap break-words text-ink-2">
            {e.module && <span className="text-accent">{e.module}: </span>}
            {mark(e.text)}
          </span>
        </li>
      ))}
    </ol>
  )
}

export function Logs() {
  const { t } = useTranslation()
  const [params, setParams] = useSearchParams()
  const sources = useApi<Source[]>('/logs/sources')
  const source = params.get('source') || 'asterisk'
  const [q, setQ] = useState(params.get('q') ?? '')
  const [query, setQuery] = useState(params.get('q') ?? '')
  const [level, setLevel] = useState(params.get('level') ?? '')
  const [follow, setFollow] = useState(false)
  const [entries, setEntries] = useState<LogEntry[] | null>(null)
  const [scanned, setScanned] = useState(0)
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState(false)

  // Typing waits a moment before it searches.
  useEffect(() => {
    const id = window.setTimeout(() => setQuery(q), 300)
    return () => window.clearTimeout(id)
  }, [q])

  const load = useCallback(async () => {
    setBusy(true)
    try {
      const qs = new URLSearchParams({ source, q: query, level, limit: '1000' })
      const r = await api<{ entries: LogEntry[]; scanned: number }>('GET', `/logs?${qs}`)
      setEntries(r.entries)
      setScanned(r.scanned)
      setError(null)
    } catch (e) {
      setError(e)
      setEntries([])
    } finally {
      setBusy(false)
    }
  }, [source, query, level])

  useEffect(() => { load() }, [load])
  useEffect(() => {
    if (!follow) return
    const id = window.setInterval(load, 5000)
    return () => window.clearInterval(id)
  }, [follow, load])

  function pick(id: string) {
    const p = new URLSearchParams(params)
    p.set('source', id)
    p.delete('q')
    setParams(p, { replace: true })
    setQ('')
    setQuery('')
  }
  const src = useMemo(() => sources.data?.find((s) => s.id === source), [sources.data, source])
  const ids = sources.data?.map((s) => s.id) ?? ['asterisk', 'security', 'portal', 'helper', 'engine']

  return (
    <>
      <PageHeader title={t('logs.title')} subtitle={t('logs.subtitle')} />
      <Card>
        <div className="flex flex-col gap-4">
          <Segmented<string> label={t('logs.source')} value={source} onChange={pick}
            options={ids.map((id) => ({ key: id, label: t(`logs.sources.${id}`, { defaultValue: id }) }))} />
          <p className="-mt-2 text-xs text-subtle">{t(`logs.sourceHint.${source}`, { defaultValue: '' })}</p>
          <div className="grid items-end gap-3 sm:grid-cols-[1fr_200px_auto]">
            <div className="relative">
              <Input label={t('logs.search')} value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('logs.searchPlaceholder')} className="pl-10" />
              <Search className="pointer-events-none absolute bottom-3.5 left-3.5 size-4 text-subtle" aria-hidden />
            </div>
            <Select label={t('logs.levelFilter')} value={level} onChange={(e) => setLevel(e.target.value)}>
              <option value="">{t('logs.levels.all')}</option>
              <option value="notice">{t('logs.levels.notice')}</option>
              <option value="warning">{t('logs.levels.warning')}</option>
              <option value="error">{t('logs.levels.error')}</option>
            </Select>
            <div className="flex flex-wrap gap-2">
              <Button variant="ghost" onClick={load} busy={busy}><RefreshCw className="size-4" aria-hidden /> {t('logs.reload')}</Button>
              <a href={`/api/logs/download?source=${encodeURIComponent(source)}`} download
                className="inline-flex min-h-11 items-center gap-2 rounded-[10px] border border-line px-4 text-sm text-ink-2 transition hover:border-accent/50 hover:text-ink">
                <Download className="size-4" aria-hidden /> {t('logs.download')}
              </a>
            </div>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Toggle name="follow" label={t('logs.follow')} checked={follow} onChange={setFollow} />
            {entries && !error && (
              <span className="text-xs text-subtle">
                {t('logs.count', { count: entries.length, scanned })}
                {src?.modified && <> · {t('logs.updated', { at: new Date(src.modified).toLocaleString() })}</>}
              </span>
            )}
          </div>
        </div>
      </Card>
      <ErrorMessage error={error} />
      <Card className="mt-4">
        {entries && entries.length === 0 && !error
          ? <Empty icon={<FileText className="size-7" />} title={t('logs.emptyTitle')} text={query || level ? t('logs.emptyFiltered') : t('logs.emptyText')} />
          : <LogLines entries={entries ?? []} highlight={query} />}
      </Card>
    </>
  )
}
