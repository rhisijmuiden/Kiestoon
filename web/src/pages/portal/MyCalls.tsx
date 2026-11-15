import { Suspense, lazy, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Phone, PhoneIncoming, PhoneOutgoing, Repeat } from 'lucide-react'
import { Badge, Table, Tabs, formatDate, formatDuration, td, useApi } from '../../components/kit'
import { IconButton } from '../../components/pbx'
import { Button, Card, ErrorMessage, PageHeader } from '../../components/ui'
import type { Cdr, PortalMe } from '../../lib/types'
import { useSoftphone } from '../../lib/softphone'
import { RecordingList } from '../Recordings'
import { JourneyFallback, rowOpen, useCallParam } from '../History'

const CallJourney = lazy(() => import('../history/CallJourney').then((m) => ({ default: m.CallJourney })))

const PAGE = 50

export function MyCalls() {
  const { t, i18n } = useTranslation()
  const [tab, setTab] = useState<'calls' | 'recordings'>('calls')
  const [offset, setOffset] = useState(0)
  const me = useApi<PortalMe>('/portal/me')
  const list = useApi<{ records: Cdr[]; total: number }>(`/portal/history?limit=${PAGE}&offset=${offset}`)
  const phone = useSoftphone()
  const [error, setError] = useState<unknown>(null)
  const total = list.data?.total ?? 0
  const other = (r: Cdr) => (r.src === me.data?.number ? r.dst : r.src)
  const { call, open, close } = useCallParam()
  if (call) {
    return <Suspense fallback={<JourneyFallback />}><CallJourney id={call} base="/portal/history/journey" recordingBase="/portal/recordings" onBack={close} /></Suspense>
  }
  return (
    <>
      <PageHeader title={t('portal.myCalls')} />
      <Tabs value={tab} onChange={setTab} tabs={[{ key: 'calls', label: t('portal.calls') }, { key: 'recordings', label: t('recordings.title') }]} />
      <ErrorMessage error={list.error || error} />
      <Card>
        {tab === 'recordings' ? <RecordingList base="/portal/recordings" /> : (
          <>
            <Table head={[t('history.time'), '', t('portal.with'), t('history.duration'), t('history.result'), '']}
              empty={list.data?.records.length === 0 && <p className="py-10 text-center text-sm text-muted">{t('history.empty')}</p>}>
              {list.data?.records.map((r) => {
                const out = r.src === me.data?.number
                return (
                  <tr key={r.id} {...rowOpen(() => open(r.linkedId || r.uniqueId), t('journey.open', { time: formatDate(r.start, i18n.language) }))}>
                    <td className={`${td} whitespace-nowrap font-mono text-xs`}>{formatDate(r.start, i18n.language)}</td>
                    <td className={td}>{r.direction === 'internal' ? <Repeat className="size-4 text-subtle" aria-hidden /> : out ? <PhoneOutgoing className="size-4 text-accent-2" aria-hidden /> : <PhoneIncoming className="size-4 text-accent" aria-hidden />}</td>
                    <td className={td}><span className="font-mono text-ink">{other(r) || '–'}</span> {!out && <span className="text-xs text-subtle">{r.clid.replace(/<.*>/, '').replace(/"/g, '').trim()}</span>}</td>
                    <td className={`${td} font-mono`}>{formatDuration(r.billsec)}</td>
                    <td className={td}><Badge tone={r.disposition === 'ANSWERED' ? 'ok' : 'warn'}>{t(`history.disp.${r.disposition.replace(' ', '_')}`, { defaultValue: r.disposition })}</Badge></td>
                    <td className={`${td} text-right`}>
                      {other(r) && <IconButton label={t('portal.callBack')} onClick={() => phone.callAny(other(r)).catch(setError)}><Phone className="size-4" aria-hidden /></IconButton>}
                    </td>
                  </tr>
                )
              })}
            </Table>
            {total > PAGE && (
              <div className="mt-4 flex justify-end gap-2">
                <Button variant="ghost" disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - PAGE))}>{t('common.previous')}</Button>
                <Button variant="ghost" disabled={offset + PAGE >= total} onClick={() => setOffset(offset + PAGE)}>{t('common.next')}</Button>
              </div>
            )}
          </>
        )}
      </Card>
    </>
  )
}
