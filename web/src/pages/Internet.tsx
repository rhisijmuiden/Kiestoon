import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Activity, Globe, Lock, Network, RefreshCw, ShieldCheck } from 'lucide-react'
import { Badge, CopyField, SectionTitle, Textarea, Toggle, formatDate, useApi } from '../components/kit'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { InternetData, InternetSettings, MetricsSettings, ProxyData, ProxySettings } from '../lib/types'

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
      <div className="grid gap-4 xl:grid-cols-2">
        <ReverseProxy />
        <Metrics />
      </div>
    </>
  )
}

const lines = (v: string) => v.split(/[\s,]+/).map((x) => x.trim()).filter(Boolean)

/** Running behind your own reverse proxy (nginx, Traefik, HAProxy …): which addresses may set X-Forwarded-For. */
function ReverseProxy() {
  const { t } = useTranslation()
  const d = useApi<ProxyData>('/system/proxy')
  const [p, setP] = useState<ProxySettings | null>(null)
  const [list, setList] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  useEffect(() => {
    if (d.data && !p) { setP(d.data.settings); setList(d.data.settings.trustedProxies.join('\n')) }
  }, [d.data, p])
  if (!p || !d.data) return <ErrorMessage error={d.error} />
  async function save() {
    setBusy(true)
    setError(null)
    setSaved(false)
    try {
      await api('PUT', '/system/proxy', { ...p, trustedProxies: lines(list) })
      setSaved(true)
      d.reload()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const x = d.data
  return (
    <Card>
      <SectionTitle><span className="inline-flex items-center gap-2"><Network className="size-5 text-accent" aria-hidden />{t('proxy.title')}</span></SectionTitle>
      <div className="flex flex-col gap-4">
        <p className="text-sm text-muted">{t('proxy.intro')}</p>
        <Toggle label={t('proxy.enable')} hint={t('proxy.enableHint')} checked={p.enabled} onChange={(v) => { setSaved(false); setP({ ...p, enabled: v }) }} />
        <Textarea label={t('proxy.trusted')} value={list} onChange={(e) => { setSaved(false); setList(e.target.value) }} placeholder={'10.0.0.5\n192.168.10.0/24'} hint={t('proxy.trustedHint')} />
        <Input label={t('proxy.publicUrl')} value={p.publicUrl} onChange={(e) => { setSaved(false); setP({ ...p, publicUrl: e.target.value.trim() }) }} placeholder="https://pbx.example.com" className="font-mono" hint={t('proxy.publicUrlHint')} />
        <div className="rounded-xl border border-line bg-surface-2/60 p-4">
          <div className="mb-2 flex items-center justify-between gap-2">
            <h3 className="text-sm font-medium text-ink">{t('proxy.check')}</h3>
            <button type="button" onClick={d.reload} aria-label={t('proxy.recheck')} title={t('proxy.recheck')} className="inline-flex size-9 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-accent"><RefreshCw className="size-4" aria-hidden /></button>
          </div>
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm">
            <dt className="text-muted">{t('proxy.remoteAddr')}</dt><dd className="font-mono text-ink-2">{x.remoteAddr}</dd>
            <dt className="text-muted">X-Forwarded-For</dt><dd className="break-all font-mono text-ink-2">{x.forwardedFor || '–'}</dd>
            <dt className="text-muted">X-Forwarded-Proto</dt><dd className="font-mono text-ink-2">{x.forwardedProto || '–'}</dd>
            <dt className="text-muted">{t('proxy.clientIp')}</dt><dd className="font-mono text-ink">{x.clientIp}</dd>
            <dt className="text-muted">{t('proxy.secure')}</dt><dd>{x.secure ? <Badge tone="ok" dot>HTTPS</Badge> : <Badge tone="warn" dot>HTTP</Badge>}</dd>
          </dl>
          {x.forwardedFor && x.clientIp === x.remoteAddr && <p className="mt-2 text-xs text-warn">{t('proxy.notTrusted')}</p>}
        </div>
        <p className="text-xs text-subtle">{t('proxy.docs')}</p>
        {saved && <Alert tone="ok">{t('proxy.saved')}</Alert>}
        <ErrorMessage error={error} />
        <div className="flex justify-end"><Button busy={busy} onClick={save}>{t('common.save')}</Button></div>
      </div>
    </Card>
  )
}

function randomToken() {
  const b = new Uint8Array(24)
  crypto.getRandomValues(b)
  return Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('')
}

/** Optional Prometheus endpoint at /metrics. */
function Metrics() {
  const { t } = useTranslation()
  const d = useApi<{ settings: MetricsSettings; url: string }>('/system/metrics')
  const [m, setM] = useState<MetricsSettings | null>(null)
  const [allow, setAllow] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  useEffect(() => {
    if (d.data && !m) { setM(d.data.settings); setAllow((d.data.settings.allowFrom ?? []).join('\n')) }
  }, [d.data, m])
  if (!m || !d.data) return <ErrorMessage error={d.error} />
  async function save() {
    if (!m) return
    setBusy(true)
    setError(null)
    setSaved(false)
    const body = { ...m, token: m.enabled && !m.token ? randomToken() : m.token, allowFrom: lines(allow) }
    try {
      const r = await api<{ settings: MetricsSettings; url: string }>('PUT', '/system/metrics', body)
      setM(r.settings)
      setSaved(true)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Card>
      <SectionTitle><span className="inline-flex items-center gap-2"><Activity className="size-5 text-accent" aria-hidden />{t('metrics.title')}</span></SectionTitle>
      <div className="flex flex-col gap-4">
        <p className="text-sm text-muted">{t('metrics.intro')}</p>
        <Toggle label={t('metrics.enable')} checked={m.enabled} onChange={(v) => { setSaved(false); setM({ ...m, enabled: v }) }} />
        {m.enabled && <>
          <CopyField label={t('metrics.url')} value={d.data.url} />
          <div className="flex items-end gap-2">
            <div className="flex-1"><CopyField label={t('metrics.token')} value={m.token || t('metrics.tokenAuto')} secret={!!m.token} /></div>
            <Button variant="ghost" onClick={() => { setSaved(false); setM({ ...m, token: randomToken() }) }}><RefreshCw className="size-4" aria-hidden />{t('metrics.newToken')}</Button>
          </div>
          <Textarea label={t('metrics.allowFrom')} value={allow} onChange={(e) => { setSaved(false); setAllow(e.target.value) }} placeholder="10.0.0.20/32" hint={t('metrics.allowFromHint')} />
          <pre className="overflow-x-auto rounded-xl border border-line bg-field p-3 font-mono text-xs text-ink-2">{`- job_name: kiestoon
  scheme: ${d.data.url.startsWith('https') ? 'https' : 'http'}
  authorization:
    credentials: <token>
  static_configs:
    - targets: ['${d.data.url.replace(/^https?:\/\//, '').replace(/\/metrics$/, '')}']`}</pre>
        </>}
        {saved && <Alert tone="ok">{t('metrics.saved')}</Alert>}
        <ErrorMessage error={error} />
        <div className="flex justify-end"><Button busy={busy} onClick={save}>{t('common.save')}</Button></div>
      </div>
    </Card>
  )
}
