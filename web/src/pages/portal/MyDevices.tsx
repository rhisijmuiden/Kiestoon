import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import QRCode from 'qrcode'
import { Headset, Smartphone } from 'lucide-react'
import { Badge, CopyField, Empty, SectionTitle, formatDate, useApi } from '../../components/kit'
import { Alert, Button, Card, ErrorMessage, PageHeader } from '../../components/ui'
import { api } from '../../lib/api'
import type { BlfEntry, ModuleType, Phone, PhoneKey, PhoneModule, PortalMe } from '../../lib/types'
import { useSoftphone } from '../../lib/softphone'
import { KeyLayout, fillKeys } from '../../components/KeyLayout'
import { WindowsSoftphoneCard } from '../../components/WindowsSoftphone'

type PortalPhone = Phone & { maxKeys: number; maxLines: number; moduleTypes: ModuleType[] }
type Sip = { server: string; port: number; transport: string; username: string; password: string; srtp: boolean }

/** SIP details for apps, the browser softphone and the keys of the user's own desk phones. */
export function MyDevices() {
  const { t, i18n } = useTranslation()
  const me = useApi<PortalMe>('/portal/me')
  const phones = useApi<PortalPhone[]>('/portal/phones')
  const blf = useApi<BlfEntry[]>('/portal/colleagues')
  const softphone = useSoftphone()
  const [sip, setSip] = useState<Sip | null>(null)
  const [qr, setQr] = useState('')
  const [error, setError] = useState<unknown>(null)
  async function reveal() {
    setError(null)
    try {
      const s = await api<Sip>('GET', '/portal/sip')
      setSip(s)
      const uri = `sip:${s.username}@${s.server}:${s.port};transport=${s.transport}`
      setQr(await QRCode.toDataURL(uri, { margin: 1, width: 180, color: { dark: '#070B14', light: '#FFFFFF' } }))
    } catch (e) {
      setError(e)
    }
  }
  return (
    <>
      <PageHeader title={t('portal.devices')} subtitle={t('portal.devicesHint')} />
      <ErrorMessage error={error || me.error} />
      <div className="grid gap-6 lg:grid-cols-2">
        <Card className="flex flex-col gap-3">
          <SectionTitle><span className="inline-flex items-center gap-2"><Headset className="size-5 text-accent" aria-hidden />{t('softphone.title')}</span></SectionTitle>
          <p className="text-sm text-muted">{me.data?.webrtc ? t('portal.softphoneText') : t('portal.softphoneDisabled')}</p>
          {me.data?.webrtc && (
            <div className="flex flex-wrap items-center gap-3">
              <Badge tone={softphone.status === 'registered' ? 'ok' : softphone.status === 'connecting' ? 'warn' : 'muted'} dot>{t(`softphone.status.${softphone.status}`)}</Badge>
              {softphone.status !== 'registered' && <Button variant="secondary" onClick={() => { softphone.enable(); softphone.setOpen(true) }}>{t('softphone.enable')}</Button>}
            </div>
          )}
        </Card>
        <WindowsSoftphoneCard />
        <Card className="flex flex-col gap-3">
          <SectionTitle><span className="inline-flex items-center gap-2"><Smartphone className="size-5 text-accent" aria-hidden />{t('portal.sipApp')}</span></SectionTitle>
          <p className="text-sm text-muted">{t('portal.sipAppText')}</p>
          {!sip ? <div><Button variant="ghost" onClick={reveal}>{t('portal.showSip')}</Button></div> : (
            <div className="grid gap-4 sm:grid-cols-[1fr_auto]">
              <div className="grid gap-2">
                <CopyField label={t('extensions.server')} value={sip.server} />
                <CopyField label={t('extensions.port')} value={`${sip.port} (${sip.transport.toUpperCase()})`} />
                <CopyField label={t('extensions.username')} value={sip.username} />
                <CopyField label={t('extensions.password')} value={sip.password} secret />
              </div>
              {qr && <img src={qr} alt={t('portal.qrAlt')} className="size-[180px] rounded-xl" />}
            </div>
          )}
        </Card>
      </div>
      <Card>
        <SectionTitle>{t('portal.deskPhones')}</SectionTitle>
        {phones.data?.length === 0 && <Empty icon={<Smartphone className="size-7" />} title={t('portal.noPhones')} text={t('portal.noPhonesText')} />}
        <div className="flex flex-col gap-6">
          {phones.data?.map((p) => <PhoneKeys key={p.id} phone={p} exts={blf.data ?? []} lastSeen={p.lastSeenAt ? formatDate(p.lastSeenAt, i18n.language) : ''} onSaved={phones.reload} />)}
        </div>
      </Card>
    </>
  )
}

function PhoneKeys({ phone, exts, lastSeen, onSaved }: { phone: PortalPhone; exts: { number: string; name: string }[]; lastSeen: string; onSaved: () => void }) {
  const { t } = useTranslation()
  const make = () => fillKeys(phone.keys, phone.maxKeys)
  const [keys, setKeys] = useState<PhoneKey[]>(make)
  const [modules, setModules] = useState<PhoneModule[]>(phone.modules ?? [])
  useEffect(() => { setKeys(make()); setModules(phone.modules ?? []) }, [phone]) // eslint-disable-line react-hooks/exhaustive-deps
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  async function save() {
    setBusy(true)
    setError(null)
    setSaved(false)
    try {
      await api('PUT', `/portal/phones/${phone.id}/keys`, { keys, modules })
      setSaved(true)
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-3">
        <span className="font-medium text-ink">{phone.label || `${phone.vendor} ${phone.model}`}</span>
        <span className="font-mono text-xs text-subtle">{phone.mac}</span>
        {lastSeen && <span className="text-xs text-subtle">{t('portal.lastSeen', { at: lastSeen })}</span>}
      </div>
      <ErrorMessage error={error} />
      {saved && <Alert tone="ok">{t('portal.keysSaved')}</Alert>}
      {phone.maxKeys === 0 && modules.length === 0 ? <p className="text-sm text-subtle">{t('phones.noKeys')}</p> : (
        <>
          <KeyLayout vendor={phone.vendor} keys={keys} modules={modules} moduleTypes={phone.moduleTypes ?? []} lines={1 + (phone.lines?.length ?? 0)} exts={exts}
            onKeys={setKeys} onModules={setModules} canEditModules={false} />
          <div className="flex justify-end"><Button busy={busy} onClick={save}>{t('common.save')}</Button></div>
        </>
      )}
    </div>
  )
}
