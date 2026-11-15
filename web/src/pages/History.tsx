import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Download, PhoneIncoming, PhoneOutgoing, Repeat } from 'lucide-react'
import { Badge, Select, Table, formatDate, formatDuration, td, useApi } from '../components/kit'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import type { Cdr } from '../lib/types'

const PAGE = 50

export function History() {
  const { t, i18n } = useTranslation()
  const [f, setF] = useState({ q: '', from: '', to: '', direction: '', disposition: '' })
  const [offset, setOffset] = useState(0)
  const params = new URLSearchParams({ ...f, limit: String(PAGE), offset: String(offset) })
  const list = useApi<{ records: Cdr[]; total: number }>(`/cdr?${params}`)
  const exportUrl = `/api/cdr/export?${new URLSearchParams(f)}`
  const set = (k: keyof typeof f, v: string) => { setOffset(0); setF({ ...f, [k]: v }) }
  const dispTone = (d: string) => (d === 'ANSWERED' ? 'ok' : d === 'NO ANSWER' || d === 'BUSY' ? 'warn' : 'danger') as 'ok' | 'warn' | 'danger'
  const total = list.data?.total ?? 0
  return (
    <>
      <PageHeader title={t('history.title')} subtitle={t('history.subtitle')}>
        <a href={exportUrl} className="inline-flex min-h-11 items-center gap-2 rounded-[10px] border border-accent/45 bg-accent/10 px-4 text-sm font-medium text-accent-ink hover:bg-accent/15">
          <Download className="size-4" aria-hidden /> {t('history.export')}
        </a>
      </PageHeader>
      <Card>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
          <Input label={t('history.search')} value={f.q} onChange={(e) => set('q', e.target.value)} placeholder="201, 06…" />
          <Input label={t('history.from')} type="date" value={f.from} onChange={(e) => set('from', e.target.value)} />
          <Input label={t('history.to')} type="date" value={f.to} onChange={(e) => set('to', e.target.value)} />
          <Select label={t('history.direction')} value={f.direction} onChange={(e) => set('direction', e.target.value)}>
            <option value="">{t('history.all')}</option>
            <option value="inbound">{t('history.dir.inbound')}</option>
            <option value="outbound">{t('history.dir.outbound')}</option>
            <option value="internal">{t('history.dir.internal')}</option>
          </Select>
          <Select label={t('history.result')} value={f.disposition} onChange={(e) => set('disposition', e.target.value)}>
            <option value="">{t('history.all')}</option>
            {['ANSWERED', 'NO ANSWER', 'BUSY', 'FAILED', 'CONGESTION'].map((d) => <option key={d} value={d}>{t(`history.disp.${d.replace(' ', '_')}`)}</option>)}
          </Select>
        </div>
      </Card>
      <ErrorMessage error={list.error} />
      <Card>
        <Table head={[t('history.time'), '', t('history.caller'), t('history.called'), t('history.duration'), t('history.result'), t('history.trunk')]}
          empty={list.data?.records.length === 0 && <p className="py-10 text-center text-sm text-muted">{t('history.empty')}</p>}>
          {list.data?.records.map((r) => (
            <tr key={r.id}>
              <td className={`${td} whitespace-nowrap font-mono text-xs`}>{formatDate(r.start, i18n.language)}</td>
              <td className={td} title={t(`history.dir.${r.direction}`)}>
                {r.direction === 'inbound' ? <PhoneIncoming className="size-4 text-accent" aria-label={t('history.dir.inbound')} /> : r.direction === 'outbound' ? <PhoneOutgoing className="size-4 text-accent-2" aria-label={t('history.dir.outbound')} /> : <Repeat className="size-4 text-subtle" aria-label={t('history.dir.internal')} />}
              </td>
              <td className={td}><span className="font-mono text-ink">{r.src || '–'}</span> <span className="text-xs text-subtle">{r.clid.replace(/<.*>/, '').replace(/"/g, '').trim()}</span></td>
              <td className={`${td} font-mono`}>{r.dst}</td>
              <td className={`${td} font-mono`}>{formatDuration(r.billsec)}</td>
              <td className={td}><Badge tone={dispTone(r.disposition)}>{t(`history.disp.${r.disposition.replace(' ', '_')}`, { defaultValue: r.disposition })}</Badge></td>
              <td className={td}>{r.trunk || '–'}</td>
            </tr>
          ))}
        </Table>
        {total > PAGE && (
          <div className="mt-4 flex items-center justify-between text-sm text-muted">
            <span>{t('history.showing', { from: offset + 1, to: Math.min(offset + PAGE, total), total })}</span>
            <div className="flex gap-2">
              <Button variant="ghost" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - PAGE))}>{t('common.previous')}</Button>
              <Button variant="ghost" disabled={offset + PAGE >= total} onClick={() => setOffset(offset + PAGE)}>{t('common.next')}</Button>
            </div>
          </div>
        )}
      </Card>
    </>
  )
}
