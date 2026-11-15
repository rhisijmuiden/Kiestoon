import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ShieldAlert } from 'lucide-react'
import { SectionTitle, Table, Toggle, formatDate, td, useApi } from '../components/kit'
import { splitList } from '../components/pbx'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { FeatureCode, FeatureSettings } from '../lib/types'

type View = { settings: FeatureSettings; fraudLockedAt: string; codes: FeatureCode[] }

export function Features() {
  const { t, i18n } = useTranslation()
  const view = useApi<View>('/pbx/features')
  const [f, setF] = useState<FeatureSettings | null>(null)
  const [premium, setPremium] = useState('')
  const [blocked, setBlocked] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  useEffect(() => {
    if (!view.data) return
    setF(view.data.settings)
    setPremium(view.data.settings.fraud.premiumPrefixes.join(', '))
    setBlocked(view.data.settings.fraud.blockedCountries.join(', '))
  }, [view.data])
  if (!f || !view.data) return <><PageHeader title={t('features.title')} /><ErrorMessage error={view.error} /></>
  const fraud = f.fraud
  const setFraud = <K extends keyof FeatureSettings['fraud']>(k: K, v: FeatureSettings['fraud'][K]) => setF({ ...f, fraud: { ...fraud, [k]: v } })
  async function save() {
    setBusy(true)
    setError(null)
    setSaved(false)
    try {
      await api('PUT', '/pbx/features', { ...f, fraud: { ...fraud, premiumPrefixes: splitList(premium), blockedCountries: splitList(blocked).map((c) => c.replace(/^\+/, '')) } })
      setSaved(true)
      view.reload()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  async function unlock() {
    await api('POST', '/pbx/fraud/unlock')
    view.reload()
  }
  return (
    <>
      <PageHeader title={t('features.title')} subtitle={t('features.subtitle')}>
        <Button busy={busy} onClick={save}>{t('common.save')}</Button>
      </PageHeader>
      <ErrorMessage error={error} />
      {saved && <Alert tone="ok">{t('common.saved')}</Alert>}
      {view.data.fraudLockedAt && (
        <Alert>
          <div className="flex flex-wrap items-center gap-3">
            <span><strong>{t('features.lockedTitle')}</strong> {t('features.lockedText', { at: formatDate(view.data.fraudLockedAt, i18n.language) })}</span>
            <Button variant="danger" onClick={unlock}>{t('features.unlock')}</Button>
          </div>
        </Alert>
      )}
      <div className="grid gap-6 lg:grid-cols-2">
        <Card className="flex flex-col gap-4">
          <SectionTitle>{t('features.parking')}</SectionTitle>
          <div className="grid gap-4 sm:grid-cols-2">
            <Input label={t('features.parkingSlots')} type="number" min={1} max={9} value={f.parkingSlots} onChange={(e) => setF({ ...f, parkingSlots: Number(e.target.value) })} hint={t('features.parkingSlotsHint', { last: 70 + f.parkingSlots })} />
            <Input label={t('features.parkingTimeout')} type="number" min={30} max={900} value={f.parkingTimeout} onChange={(e) => setF({ ...f, parkingTimeout: Number(e.target.value) })} hint={t('features.parkingTimeoutHint')} />
          </div>
          <SectionTitle>{t('features.recording')}</SectionTitle>
          <Input label={t('features.recordingDays')} type="number" min={0} max={3650} value={f.recordingDays} onChange={(e) => setF({ ...f, recordingDays: Number(e.target.value) })} hint={t('features.recordingDaysHint')} />
          <Toggle label={t('features.recordingBeep')} hint={t('features.recordingBeepHint')} checked={f.recordingBeep} onChange={(v) => setF({ ...f, recordingBeep: v })} />
          <SectionTitle>{t('features.journey')}</SectionTitle>
          <Input label={t('features.journeyDays')} type="number" min={0} max={3650} value={f.journeyDays} onChange={(e) => setF({ ...f, journeyDays: Number(e.target.value) })} hint={t('features.journeyDaysHint')} />
        </Card>
        <Card className="flex flex-col gap-4">
          <SectionTitle><span className="inline-flex items-center gap-2"><ShieldAlert className="size-5 text-warn" aria-hidden />{t('features.fraud')}</span></SectionTitle>
          <p className="text-sm text-muted">{t('features.fraudText')}</p>
          <Input label={t('features.premiumPrefixes')} value={premium} onChange={(e) => setPremium(e.target.value)} className="font-mono text-sm" hint={t('features.premiumPrefixesHint')} />
          <Input label={t('features.blockedCountries')} value={blocked} onChange={(e) => setBlocked(e.target.value)} className="font-mono text-sm" placeholder="7, 234, 882, 883" hint={t('features.blockedCountriesHint')} />
          <Input label={t('features.maxOutbound')} type="number" min={0} max={500} value={fraud.maxOutbound} onChange={(e) => setFraud('maxOutbound', Number(e.target.value))} hint={t('features.maxOutboundHint')} />
          <div className="grid gap-4 sm:grid-cols-2">
            <Input label={t('features.alertOutbound')} type="number" min={0} value={fraud.alertOutboundPerHour} onChange={(e) => setFraud('alertOutboundPerHour', Number(e.target.value))} hint={t('features.perHour')} />
            <Input label={t('features.alertInternational')} type="number" min={0} value={fraud.alertInternationalPerHour} onChange={(e) => setFraud('alertInternationalPerHour', Number(e.target.value))} hint={t('features.perHour')} />
          </div>
          <Input label={t('features.alertEmail')} type="email" value={fraud.alertEmail} onChange={(e) => setFraud('alertEmail', e.target.value)} hint={t('features.alertEmailHint')} />
          <Toggle label={t('features.blockOnAlert')} hint={t('features.blockOnAlertHint')} checked={fraud.blockOnAlert} onChange={(v) => setFraud('blockOnAlert', v)} />
        </Card>
      </div>
      <Card>
        <SectionTitle>{t('features.codes')}</SectionTitle>
        <FeatureCodeTable codes={view.data.codes} />
      </Card>
    </>
  )
}

export function FeatureCodeTable({ codes }: { codes: FeatureCode[] }) {
  const { t } = useTranslation()
  return (
    <Table head={[t('features.code'), t('features.action')]}>
      {codes.map((c) => (
        <tr key={c.code}>
          <td className={`${td} whitespace-nowrap font-mono text-ink`}>{c.code}</td>
          <td className={td}>{t(`codes.${c.key}`)}</td>
        </tr>
      ))}
    </Table>
  )
}
