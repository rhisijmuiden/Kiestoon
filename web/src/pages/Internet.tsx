import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Globe, Lock, ShieldCheck } from 'lucide-react'
import { Badge, CopyField, SectionTitle, Toggle, formatDate, useApi } from '../components/kit'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { InternetData, InternetSettings } from '../lib/types'

/** Direct internet access: Caddy with a Let's Encrypt certificate in front of the portal. */
export function Internet() {
  const { t, i18n } = useTranslation()
  const data = useApi<InternetData>('/internet')
  const [s, setS] = useState<InternetSettings | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  useEffect(() => { if (data.data && !s) setS(data.data.settings) }, [data.data, s])
  if (!s || !data.data) return <><PageHeader title={t('internet.title')} /><ErrorMessage error={data.error} /></>
  const set = <K extends keyof InternetSettings>(k: K, v: InternetSettings[K]) => { setSaved(false); setS({ ...s, [k]: v }) }
  const st = data.data.status
  const cert = st.certificate

  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api('PUT', '/internet', s)
      setSaved(true)
      data.reload()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <PageHeader title={t('internet.title')} subtitle={t('internet.subtitle')}>
        {s.enabled && st.active ? <Badge tone="ok" dot>{t('internet.online')}</Badge> : <Badge dot>{t('internet.off')}</Badge>}
      </PageHeader>
      {!data.data.helperAvailable && <Alert tone="warn">{t('internet.noHelper')}</Alert>}
      {data.data.helperAvailable && !st.installed && <Alert tone="warn">{t('errors.caddy_missing')}</Alert>}
      {saved && <Alert tone="ok">{s.enabled ? t('internet.savedOn', { domain: s.domain }) : t('internet.savedOff')}</Alert>}
      <ErrorMessage error={error} />
      <div className="grid gap-4 xl:grid-cols-[3fr_2fr]">
        <Card>
          <SectionTitle><span className="inline-flex items-center gap-2"><Globe className="size-5 text-accent" aria-hidden />{t('internet.settings')}</span></SectionTitle>
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted">{t('internet.intro')}</p>
            <Toggle label={t('internet.enable')} hint={t('internet.enableHint')} checked={s.enabled} onChange={(v) => set('enabled', v)} />
            <div className="grid gap-4 sm:grid-cols-2">
              <Input label={t('internet.domain')} value={s.domain} onChange={(e) => set('domain', e.target.value.trim().toLowerCase())} placeholder="pbx.example.com" className="font-mono" hint={t('internet.domainHint')} />
              <Input label={t('internet.email')} type="email" value={s.email} onChange={(e) => set('email', e.target.value.trim())} placeholder="it@example.com" hint={t('internet.emailHint')} />
            </div>
            <Toggle label={t('internet.useForSip')} hint={t('internet.useForSipHint')} checked={s.useForSip} onChange={(v) => set('useForSip', v)} />
            <Toggle label={t('internet.staging')} hint={t('internet.stagingHint')} checked={s.staging} onChange={(v) => set('staging', v)} />
            {s.enabled && data.data.dns && data.data.dns.length === 0 && s.domain && <Alert tone="warn">{t('internet.noDns', { domain: s.domain })}</Alert>}
            <div className="flex justify-end">
              <Button busy={busy} disabled={!data.data.helperAvailable} onClick={save}>{t('common.save')}</Button>
            </div>
          </div>
        </Card>
        <div className="flex flex-col gap-4">
          <Card>
            <SectionTitle><span className="inline-flex items-center gap-2"><Lock className="size-5 text-accent" aria-hidden />{t('internet.certificate')}</span></SectionTitle>
            {cert ? (
              <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
                <dt className="text-muted">{t('internet.certFor')}</dt><dd className="font-mono text-ink">{cert.dnsNames?.join(', ') || cert.subject}</dd>
                <dt className="text-muted">{t('internet.certIssuer')}</dt><dd className="text-ink-2">{cert.issuer}</dd>
                <dt className="text-muted">{t('internet.certValid')}</dt><dd className="text-ink-2">{formatDate(cert.notAfter, i18n.language)}</dd>
              </dl>
            ) : (
              <p className="text-sm text-muted">{s.enabled ? t('internet.certPending') : t('internet.certNone')}</p>
            )}
          </Card>
          <Card>
            <SectionTitle><span className="inline-flex items-center gap-2"><ShieldCheck className="size-5 text-accent" aria-hidden />{t('internet.checklist')}</span></SectionTitle>
            <ol className="flex list-decimal flex-col gap-2 pl-5 text-sm text-ink-2">
              <li>{t('internet.step1')}</li>
              <li>{t('internet.step2')}</li>
              <li>{t('internet.step3')}</li>
            </ol>
            {data.data.dns && data.data.dns.length > 0 && <div className="mt-4"><CopyField label={t('internet.dnsNow', { domain: s.domain })} value={data.data.dns.join(', ')} /></div>}
            {s.enabled && s.domain && <div className="mt-3"><CopyField label={t('internet.address')} value={`https://${s.domain}`} /></div>}
          </Card>
        </div>
      </div>
    </>
  )
}
