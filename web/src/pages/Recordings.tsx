import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Disc3, Download, Sparkles, Trash2 } from 'lucide-react'
import { Empty, Table, formatBytes, formatDate, formatDuration, td, useApi } from '../components/kit'
import { IconButton } from '../components/pbx'
import { Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api, isAdmin } from '../lib/api'
import { useSession } from '../lib/session'
import type { Recording } from '../lib/types'
import { RecordingAI, SentimentBadge } from './ai/Summary'

/** A table of call recordings. base is /recordings (admin, reception) or /portal/recordings (own calls). */
export function RecordingList({ base, canDelete, canRun, query }: { base: string; canDelete?: boolean; canRun?: boolean; query?: string }) {
  const { t, i18n } = useTranslation()
  const [open, setOpen] = useState('')
  const list = useApi<Recording[]>(query ? `${base}?q=${encodeURIComponent(query)}` : base)
  async function remove(r: Recording) {
    if (!window.confirm(t('recordings.confirmDelete'))) return
    await api('DELETE', `${base}/${r.name}`)
    list.reload()
  }
  return (
    <>
      <ErrorMessage error={list.error} />
      <Table head={[t('history.time'), t('history.caller'), t('history.called'), t('history.duration'), t('recordings.listen'), '']}
        empty={list.data?.length === 0 && <Empty icon={<Disc3 className="size-7" />} title={t('recordings.emptyTitle')} text={t('recordings.emptyText')} />}>
        {list.data?.map((r) => (
          <Fragment key={r.name}>
          <tr>
            <td className={`${td} whitespace-nowrap font-mono text-xs`}>{formatDate(r.date, i18n.language)}</td>
            <td className={`${td} font-mono text-ink`}>{r.from || '–'}</td>
            <td className={`${td} font-mono`}>{r.to.startsWith('conf') ? `${t('recordings.conference')} ${r.to.slice(4)}` : r.to || '–'}</td>
            <td className={`${td} font-mono`}>
              {formatDuration(r.duration)}<span className="ml-2 text-xs text-subtle">{formatBytes(r.size)}</span>
              {r.ai?.topic && <span className="mt-1 flex flex-wrap items-center gap-1.5 font-sans text-xs text-muted">{r.ai.topic}<SentimentBadge value={r.ai.sentiment} /></span>}
            </td>
            <td className={td}><audio controls preload="none" src={`/api${base}/${r.name}`} className="h-9 w-56 max-w-full" /></td>
            <td className={`${td} whitespace-nowrap text-right`}>
              <a href={`/api${base}/${r.name}`} download={r.name} title={t('recordings.download')} aria-label={t('recordings.download')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-ink">
                <Download className="size-4" aria-hidden />
              </a>
              {(canRun || r.ai) && (
                <IconButton label={t('ai.show')} onClick={() => setOpen(open === r.name ? '' : r.name)}>
                  <Sparkles className={`size-4 ${r.ai?.status === 'done' ? 'text-accent' : ''}`} aria-hidden />
                </IconButton>
              )}
              {canDelete && <IconButton label={t('common.delete')} danger onClick={() => remove(r)}><Trash2 className="size-4" aria-hidden /></IconButton>}
            </td>
          </tr>
          {open === r.name && (
            <tr><td colSpan={6} className="px-3 pb-4"><RecordingAI base={base} name={r.name} canRun={canRun} /></td></tr>
          )}
          </Fragment>
        ))}
      </Table>
    </>
  )
}

export function Recordings() {
  const { t } = useTranslation()
  const { state } = useSession()
  const admin = isAdmin(state.kind === 'ready' ? state.me.user : undefined)
  // A link from the CRM (?play=<file>) filters on the call's caller number.
  const [q, setQ] = useState(() => new URLSearchParams(window.location.search).get('play')?.match(/-([0-9+]+)-[0-9+*#q]*\.wav$/)?.[1]?.replace(/\D/g, '') ?? '')
  return (
    <>
      <PageHeader title={t('recordings.title')} subtitle={t('recordings.subtitle')} />
      <Card>
        <div className="mb-4 max-w-xs">
          <Input label={t('recordings.filter')} value={q} onChange={(e) => setQ(e.target.value.replace(/\D/g, ''))} inputMode="numeric" placeholder="201" />
        </div>
        <RecordingList base="/recordings" canDelete={admin} canRun query={q} />
      </Card>
    </>
  )
}
