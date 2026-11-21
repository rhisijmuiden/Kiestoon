import { Suspense, lazy, useState } from 'react'
import { useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ChevronRight, Download, Loader2, PhoneIncoming, PhoneOutgoing, Repeat } from 'lucide-react'
import { Badge, Select, Table, formatDate, formatDuration, td, useApi } from '../components/kit'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { MosPill } from '../components/quality'
import type { Cdr, QualityLimits } from '../lib/types'

const PAGE = 50

const defaultLimits: QualityLimits = { good: 4, poor: 3.6, jitter: 30, loss: 2, rtt: 300 }

const CallJourney = lazy(() => import('./history/CallJourney').then((m) => ({ default: m.CallJourney })))

export function JourneyFallback() {
  return <div className="flex justify-center py-16 text-muted" role="status"><Loader2 className="size-5 animate-spin text-accent" aria-hidden /></div>
}

/** The call to show the journey of (?call=<linked id>), and how to open and close it. */
export function useCallParam() {
  const [params, setParams] = useSearchParams()
  const call = params.get('call')
  const open = (id: string) => setParams((p) => { p.set('call', id); return p })
  const close = () => setParams((p) => { p.delete('call'); return p })
  return { call, open, close }
}

/** Opens a row's journey with a click or Enter. */
export function rowOpen(open: () => void, label: string) {
  return {
    onClick: (e: { target: EventTarget }) => { if (!(e.target as HTMLElement).closest('a,button,audio')) open() },
    onKeyDown: (e: { key: string; preventDefault: () => void }) => { if (e.key === 'Enter') { e.preventDefault(); open() } },
    tabIndex: 0, role: 'link', 'aria-label': label,
    className: 'cursor-pointer transition-colors hover:bg-tint/[0.04] focus-visible:bg-tint/[0.06] focus-visible:outline-none',
  }
}

export function History() {
  const { t, i18n } = useTranslation()
  const [f, setF] = useState({ q: '', from: '', to: '', direction: '', disposition: '' })
  const [offset, setOffset] = useState(0)
  const params = new URLSearchParams({ ...f, limit: String(PAGE), offset: String(offset) })
  const list = useApi<{ records: Cdr[]; total: number; quality?: Record<string, number>; qualityLimits?: QualityLimits }>(`/cdr?${params}`)
  const exportUrl = `/api/cdr/export?${new URLSearchParams(f)}`
  const set = (k: keyof typeof f, v: string) => { setOffset(0); setF({ ...f, [k]: v }) }
  const dispTone = (d: string) => (d === 'ANSWERED' ? 'ok' : d === 'NO ANSWER' || d === 'BUSY' ? 'warn' : 'danger') as 'ok' | 'warn' | 'danger'
  const total = list.data?.total ?? 0
  const { call, open, close } = useCallParam()
  if (call) {
    return <Suspense fallback={<JourneyFallback />}><CallJourney id={call} base="/cdr/journey" recordingBase="/recordings" onBack={close} /></Suspense>
  }
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
        <Table head={[t('history.time'), '', t('history.caller'), t('history.called'), t('history.duration'), t('history.result'), t('history.trunk'), 'MOS', '']}
          empty={list.data?.records.length === 0 && <p className="py-10 text-center text-sm text-muted">{t('history.empty')}</p>}>
          {list.data?.records.map((r) => (
            <tr key={r.id} {...rowOpen(() => open(r.linkedId || r.uniqueId), t('journey.open', { time: formatDate(r.start, i18n.language) }))}>
              <td className={`${td} whitespace-nowrap font-mono text-xs`}>{formatDate(r.start, i18n.language)}</td>
              <td className={td} title={t(`history.dir.${r.direction}`)}>
                {r.direction === 'inbound' ? <PhoneIncoming className="size-4 text-accent" aria-label={t('history.dir.inbound')} /> : r.direction === 'outbound' ? <PhoneOutgoing className="size-4 text-accent-2" aria-label={t('history.dir.outbound')} /> : <Repeat className="size-4 text-subtle" aria-label={t('history.dir.internal')} />}
              </td>
              <td className={td}><span className="font-mono text-ink">{r.src || '–'}</span> <span className="text-xs text-subtle">{r.clid.replace(/<.*>/, '').replace(/"/g, '').trim()}</span></td>
              <td className={`${td} font-mono`}>{r.dst}</td>
              <td className={`${td} font-mono`}>{formatDuration(r.billsec)}</td>
              <td className={td}><Badge tone={dispTone(r.disposition)}>{t(`history.disp.${r.disposition.replace(' ', '_')}`, { defaultValue: r.disposition })}</Badge></td>
              <td className={td}>{r.trunk || '–'}</td>
              <td className={td}>{list.data?.quality?.[r.linkedId] ? <MosPill mos={list.data.quality[r.linkedId]} limits={list.data.qualityLimits ?? defaultLimits} /> : <span className="text-subtle">–</span>}</td>
              <td className={`${td} text-right`}><ChevronRight className="inline size-4 text-subtle" aria-hidden /></td>
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
