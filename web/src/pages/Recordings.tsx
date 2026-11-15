import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Disc3, Download, Trash2 } from 'lucide-react'
import { Empty, Table, formatBytes, formatDate, formatDuration, td, useApi } from '../components/kit'
import { IconButton } from '../components/pbx'
import { Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api, isAdmin } from '../lib/api'
import { useSession } from '../lib/session'
import type { Recording } from '../lib/types'

/** A table of call recordings. base is /recordings (admin, reception) or /portal/recordings (own calls). */
export function RecordingList({ base, canDelete, query }: { base: string; canDelete?: boolean; query?: string }) {
  const { t, i18n } = useTranslation()
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
          <tr key={r.name}>
            <td className={`${td} whitespace-nowrap font-mono text-xs`}>{formatDate(r.date, i18n.language)}</td>
            <td className={`${td} font-mono text-ink`}>{r.from || '–'}</td>
            <td className={`${td} font-mono`}>{r.to.startsWith('conf') ? `${t('recordings.conference')} ${r.to.slice(4)}` : r.to || '–'}</td>
            <td className={`${td} font-mono`}>{formatDuration(r.duration)}<span className="ml-2 text-xs text-subtle">{formatBytes(r.size)}</span></td>
            <td className={td}><audio controls preload="none" src={`/api${base}/${r.name}`} className="h-9 w-56 max-w-full" /></td>
            <td className={`${td} whitespace-nowrap text-right`}>
              <a href={`/api${base}/${r.name}`} download={r.name} title={t('recordings.download')} aria-label={t('recordings.download')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-ink">
                <Download className="size-4" aria-hidden />
              </a>
              {canDelete && <IconButton label={t('common.delete')} danger onClick={() => remove(r)}><Trash2 className="size-4" aria-hidden /></IconButton>}
            </td>
          </tr>
        ))}
      </Table>
    </>
  )
}

export function Recordings() {
  const { t } = useTranslation()
  const { state } = useSession()
  const admin = isAdmin(state.kind === 'ready' ? state.me.user : undefined)
  const [q, setQ] = useState('')
  return (
    <>
      <PageHeader title={t('recordings.title')} subtitle={t('recordings.subtitle')} />
      <Card>
        <div className="mb-4 max-w-xs">
          <Input label={t('recordings.filter')} value={q} onChange={(e) => setQ(e.target.value.replace(/\D/g, ''))} inputMode="numeric" placeholder="201" />
        </div>
        <RecordingList base="/recordings" canDelete={admin} query={q} />
      </Card>
    </>
  )
}
