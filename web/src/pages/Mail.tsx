import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Send } from 'lucide-react'
import { Badge, SectionTitle, Select, Table, Toggle, formatDate, td, useApi } from '../components/kit'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { MailLog, MailSettings } from '../lib/types'
import { useSession } from '../lib/session'

export function Mail() {
  const { t, i18n } = useTranslation()
  const { state } = useSession()
  const data = useApi<{ settings: MailSettings; pending: number }>('/mail/settings')
  const log = useApi<MailLog[]>('/mail/log', 15000)
  const [m, setM] = useState<MailSettings | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  const [to, setTo] = useState('')
  const [testing, setTesting] = useState(false)
  const [testResult, setTestResult] = useState<'ok' | unknown | null>(null)

  useEffect(() => { if (data.data && !m) setM(data.data.settings) }, [data.data, m])
  useEffect(() => { if (state.kind === 'ready' && !to) setTo(data.data?.settings.notifyAddress ?? '') }, [state, data.data, to])
  if (!m) return <><PageHeader title={t('mail.title')} /><ErrorMessage error={data.error} /></>
  const set = <K extends keyof MailSettings>(k: K, v: MailSettings[K]) => { setSaved(false); setM({ ...m, [k]: v }) }

  function preset(p: 'm365' | 'google' | 'custom') {
    if (p === 'm365') setM((x) => x && { ...x, host: 'smtp.office365.com', port: 587, security: 'starttls', auth: 'oauth2_m365' })
    if (p === 'google') setM((x) => x && { ...x, host: 'smtp.gmail.com', port: 465, security: 'tls', auth: 'oauth2_google' })
    if (p === 'custom') setM((x) => x && { ...x, auth: 'password' })
  }
  async function save() {
    setBusy(true)
    setError(null)
    try {
      setM(await api<MailSettings>('PUT', '/mail/settings', m))
      setSaved(true)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  async function test() {
    setTesting(true)
    setTestResult(null)
    try {
      await api('POST', '/mail/test', { to })
      setTestResult('ok')
    } catch (e) {
      setTestResult(e)
    } finally {
      setTesting(false)
      log.reload()
    }
  }
  const securityPort = (s: MailSettings['security']) => setM({ ...m, security: s, port: s === 'tls' ? 465 : s === 'starttls' ? 587 : 25 })

  return (
    <>
      <PageHeader title={t('mail.title')} subtitle={t('mail.subtitle')}>
        {data.data?.pending ? <Badge tone="warn">{t('mail.queued', { count: data.data.pending })}</Badge> : null}
      </PageHeader>
      <div className="grid gap-4 xl:grid-cols-[1.4fr_1fr]">
        <Card>
          <SectionTitle>{t('mail.server')}</SectionTitle>
          <div className="flex flex-col gap-4">
            <Toggle label={t('mail.enabled')} hint={t('mail.enabledHint')} checked={m.enabled} onChange={(v) => set('enabled', v)} />
            <div className="flex flex-wrap gap-2">
              <span className="self-center text-xs text-subtle">{t('mail.presets')}:</span>
              <Button variant="ghost" onClick={() => preset('m365')}>Microsoft 365</Button>
              <Button variant="ghost" onClick={() => preset('google')}>Google Workspace</Button>
              <Button variant="ghost" onClick={() => preset('custom')}>{t('mail.custom')}</Button>
            </div>
            <div className="grid gap-4 sm:grid-cols-[1fr_110px]">
              <Input label={t('mail.host')} value={m.host} onChange={(e) => set('host', e.target.value.trim())} className="font-mono" placeholder="smtp.example.com" />
              <Input label={t('mail.port')} type="number" value={m.port} onChange={(e) => set('port', Number(e.target.value))} />
            </div>
            <div className="grid gap-4 sm:grid-cols-2">
              <Select label={t('mail.security')} value={m.security} onChange={(e) => securityPort(e.target.value as MailSettings['security'])}>
                <option value="starttls">STARTTLS (587)</option>
                <option value="tls">{t('mail.implicitTls')} (465)</option>
                <option value="none">{t('mail.noTls')} (25)</option>
              </Select>
              <Select label={t('mail.auth')} value={m.auth} onChange={(e) => set('auth', e.target.value as MailSettings['auth'])}>
                <option value="password">{t('mail.authPassword')}</option>
                <option value="oauth2_m365">OAuth2 · Microsoft 365</option>
                <option value="oauth2_google">OAuth2 · Google Workspace</option>
                <option value="none">{t('mail.authNone')}</option>
              </Select>
            </div>
            {m.security === 'none' && <Alert tone="warn">{t('mail.noTlsWarn')}</Alert>}
            {m.auth !== 'none' && (
              <Input label={m.auth === 'password' ? t('mail.username') : t('mail.mailbox')} value={m.username} onChange={(e) => set('username', e.target.value.trim())} autoComplete="off" />
            )}
            {m.auth === 'password' && (
              <Input label={t('mail.password')} type="password" value={m.password ?? ''} placeholder={m.passwordSet ? '••••••••' : ''} hint={m.passwordSet ? t('common.keepSecret') : undefined} onChange={(e) => set('password', e.target.value)} autoComplete="new-password" />
            )}
            {(m.auth === 'oauth2_m365' || m.auth === 'oauth2_google') && (
              <div className="grid gap-4 rounded-xl border border-line p-4">
                <p className="text-xs text-subtle">{t(m.auth === 'oauth2_m365' ? 'mail.m365Hint' : 'mail.googleHint')}</p>
                {m.auth === 'oauth2_m365' && <Input label={t('mail.tenantId')} value={m.tenantId} onChange={(e) => set('tenantId', e.target.value.trim())} className="font-mono" />}
                <Input label={t('mail.clientId')} value={m.clientId} onChange={(e) => set('clientId', e.target.value.trim())} className="font-mono" />
                <Input label={t('mail.clientSecret')} type="password" value={m.clientSecret ?? ''} placeholder={m.clientSecretSet ? '••••••••' : ''} onChange={(e) => set('clientSecret', e.target.value)} autoComplete="new-password" />
                {m.auth === 'oauth2_google' && <Input label={t('mail.refreshToken')} type="password" value={m.refreshToken ?? ''} placeholder={m.refreshTokenSet ? '••••••••' : ''} onChange={(e) => set('refreshToken', e.target.value)} autoComplete="new-password" />}
              </div>
            )}
            <div className="grid gap-4 sm:grid-cols-2">
              <Input label={t('mail.fromAddress')} type="email" value={m.fromAddress} onChange={(e) => set('fromAddress', e.target.value.trim())} />
              <Input label={t('mail.fromName')} value={m.fromName} onChange={(e) => set('fromName', e.target.value)} />
            </div>
            <Input label={t('mail.notify')} type="email" value={m.notifyAddress} onChange={(e) => set('notifyAddress', e.target.value.trim())} hint={t('mail.notifyHint')} />
            <Toggle label={t('mail.skipVerify')} hint={t('mail.skipVerifyHint')} checked={m.skipVerify} onChange={(v) => set('skipVerify', v)} />
            <ErrorMessage error={error} />
            {saved && <Alert tone="ok">{t('common.saved')}</Alert>}
            <div className="flex justify-end"><Button busy={busy} onClick={save}>{t('common.save')}</Button></div>
          </div>
        </Card>
        <div className="flex flex-col gap-4">
          <Card>
            <SectionTitle>{t('mail.test')}</SectionTitle>
            <p className="mb-3 text-sm text-muted">{t('mail.testText')}</p>
            <div className="flex flex-col gap-3">
              <Input label={t('mail.testTo')} type="email" value={to} onChange={(e) => setTo(e.target.value)} />
              <Button variant="secondary" busy={testing} disabled={!m.enabled || !to} onClick={test}><Send className="size-4" />{t('mail.sendTest')}</Button>
              {testResult === 'ok' && <Alert tone="ok">{t('mail.testOk', { to })}</Alert>}
              {testResult !== null && testResult !== 'ok' && <ErrorMessage error={testResult} />}
            </div>
          </Card>
          <Card>
            <SectionTitle>{t('mail.log')}</SectionTitle>
            <Table head={[t('mail.when'), t('mail.to'), t('mail.result')]}
              empty={log.data?.length === 0 && <p className="py-6 text-center text-sm text-muted">{t('mail.noLog')}</p>}>
              {log.data?.slice(0, 15).map((l) => (
                <tr key={l.id}>
                  <td className={`${td} whitespace-nowrap font-mono text-xs`}>{formatDate(l.at, i18n.language)}</td>
                  <td className={td}><div className="truncate">{l.recipient}</div><div className="truncate text-xs text-subtle">{l.subject}</div></td>
                  <td className={td} title={l.error}>{l.ok ? <Badge tone="ok">{t('mail.sent')}</Badge> : <Badge tone="danger">{t('mail.failed')}</Badge>}</td>
                </tr>
              ))}
            </Table>
          </Card>
        </div>
      </div>
    </>
  )
}
