import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ListChecks, Loader2, Sparkles } from 'lucide-react'
import { Badge, useApi } from '../../components/kit'
import { Button, ErrorMessage } from '../../components/ui'
import { api } from '../../lib/api'
import type { AIResult, AISentiment, AISummary } from '../../lib/types'

const sentimentTone = { positive: 'ok', neutral: 'muted', negative: 'danger' } as const

export function SentimentBadge({ value }: { value?: AISentiment }) {
  const { t } = useTranslation()
  if (!value) return null
  return <Badge tone={sentimentTone[value]} dot>{t(`ai.sentiment.${value}`)}</Badge>
}

/** A summary with its follow-ups. */
export function SummaryView({ summary, compact }: { summary: AISummary; compact?: boolean }) {
  const { t } = useTranslation()
  return (
    <div className={`flex flex-col gap-2 ${compact ? 'text-xs' : 'text-sm'}`}>
      <div className="flex flex-wrap items-center gap-2">
        <Sparkles className="size-4 text-accent" aria-hidden />
        {summary.topic && <span className="font-medium text-ink">{summary.topic}</span>}
        <SentimentBadge value={summary.sentiment} />
      </div>
      <p className="text-ink/90">{summary.summary}</p>
      {summary.actions.length > 0 && (
        <div>
          <span className="inline-flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide text-subtle"><ListChecks className="size-3.5" aria-hidden />{t('ai.actions')}</span>
          <ul className="mt-1 list-disc pl-5 text-muted">{summary.actions.map((a, i) => <li key={i}>{a}</li>)}</ul>
        </div>
      )}
    </div>
  )
}

/** The AI result of one recording: summary, transcript and (staff only) a button to make it now. */
export function RecordingAI({ base, name, canRun }: { base: string; name: string; canRun?: boolean }) {
  const { t } = useTranslation()
  const r = useApi<AIResult>(`${base}/${name}/ai`)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [open, setOpen] = useState(false)
  async function run() {
    setBusy(true)
    setError(null)
    try {
      r.setData(await api<AIResult>('POST', `${base}/${name}/ai`))
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const res = r.data
  if (!res) return <div className="p-3"><ErrorMessage error={r.error} />{!r.error && <Loader2 className="size-4 animate-spin text-muted" aria-hidden />}</div>
  return (
    <div className="flex flex-col gap-3 rounded-xl border border-line bg-surface-2/60 p-4">
      {res.summary ? <SummaryView summary={res.summary} /> : (
        <p className="text-sm text-muted">
          {res.status === 'none' ? t('ai.noResult') : t(`ai.errors.${res.error?.split(':')[0] ?? 'failed'}`, { defaultValue: t('ai.errors.failed') })}
        </p>
      )}
      {res.transcript && (
        <div>
          <button type="button" onClick={() => setOpen(!open)} aria-expanded={open} className="inline-flex items-center gap-1 text-xs font-medium text-accent hover:underline">
            <ChevronDown className={`size-3.5 transition ${open ? 'rotate-180' : ''}`} aria-hidden />{t('ai.transcript')}
          </button>
          {open && <p className="mt-2 max-h-64 overflow-y-auto whitespace-pre-wrap text-xs text-muted">{res.transcript}</p>}
        </div>
      )}
      <div className="flex flex-wrap items-center gap-3">
        {canRun && <Button variant="secondary" busy={busy} onClick={run}><Sparkles className="size-4" aria-hidden />{res.status === 'none' ? t('ai.run') : t('ai.rerun')}</Button>}
        {busy && <span className="text-xs text-subtle">{t('ai.running')}</span>}
        {res.engine && <span className="text-xs text-subtle">{t('ai.madeBy', { engine: t(`ai.models.${res.engine}`, { defaultValue: res.engine }) })}</span>}
      </div>
      <ErrorMessage error={error} />
    </div>
  )
}
