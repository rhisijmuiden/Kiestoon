import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BellRing, CheckCircle2, LockKeyhole, Smartphone, Trash2, XCircle } from 'lucide-react'
import { Badge, CopyField, Empty, SectionTitle, Table, Toggle, formatDate, td, useApi } from '../components/kit'
import { IconButton } from '../components/pbx'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { MobileAdmin, MobileDevice } from '../lib/types'

/** Mobile app: switch it on, choose the push relay and see the linked phones. */
export function MobileApp() {
  const { t, i18n } = useTranslation()
  const d = useApi<MobileAdmin>('/mobile/settings')
  const [enabled, setEnabled] = useState(false)
  const [relay, setRelay] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  const [test, setTest] = useState<{ ok: boolean; platforms?: string[]; error?: string } | null>(null)
  const [testing, setTesting] = useState(false)
  useEffect(() => {
    if (d.data) {
      setEnabled(d.data.enabled)
      setRelay(d.data.relayURL)
    }
  }, [d.data])

  async function save() {
    setBusy(true)
    setError(null)
    setSaved(false)
    try {
      await api('PUT', '/mobile/settings', { enabled, relayURL: relay })
      setSaved(true)
      d.reload()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  async function runTest() {
    setTesting(true)
    setTest(null)
    try {
      setTest(await api('POST', '/mobile/relay-test', { relayURL: relay }))
    } catch (e) {
      setTest({ ok: false, error: String((e as Error).message ?? e) })
    } finally {
      setTesting(false)
    }
  }
  async function remove(dev: MobileDevice) {
    if (!window.confirm(t('mobile.removeConfirm', { name: dev.name || dev.platform, user: dev.userName || dev.user }))) return
    try {
      await api('DELETE', `/mobile/devices/${dev.id}`)
      d.reload()
    } catch (e) {
      setError(e)
    }
  }

  return (
    <>
      <PageHeader title={t('mobile.title')} subtitle={t('mobile.subtitle')}>
        <Button busy={busy} onClick={save} disabled={!d.data}>{t('common.save')}</Button>
      </PageHeader>
      {saved && <Alert tone="ok">{t('mobile.saved')}</Alert>}
      <ErrorMessage error={error || d.error} />
      <div className="grid gap-4 xl:grid-cols-2">
        <Card>
          <SectionTitle><span className="inline-flex items-center gap-2"><Smartphone className="size-5 text-accent" aria-hidden />{t('mobile.app')}</span></SectionTitle>
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted">{t('mobile.intro')}</p>
            <Toggle label={t('mobile.enabled')} hint={t('mobile.enabledHint')} checked={enabled} onChange={(v) => { setSaved(false); setEnabled(v) }} />
            <ol className="flex flex-col gap-2 text-sm text-ink-2">
              {(['step1', 'step2', 'step3'] as const).map((s, i) => (
                <li key={s} className="flex gap-3">
                  <span className="grid size-6 shrink-0 place-items-center rounded-full bg-accent/15 font-mono text-xs text-accent">{i + 1}</span>
                  <span>{t(`mobile.${s}`)}</span>
                </li>
              ))}
            </ol>
          </div>
        </Card>
        <Card>
          <SectionTitle><span className="inline-flex items-center gap-2"><BellRing className="size-5 text-accent" aria-hidden />{t('mobile.relay')}</span></SectionTitle>
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted">{t('mobile.relayText')}</p>
            <div className="flex flex-wrap items-end gap-2">
              <div className="min-w-56 flex-1">
                <Input label={t('mobile.relayURL')} value={relay} placeholder={d.data?.defaultRelayURL} hint={t('mobile.relayHint')}
                  onChange={(e) => { setSaved(false); setRelay(e.target.value) }} />
              </div>
              <Button variant="secondary" busy={testing} onClick={runTest}>{t('mobile.test')}</Button>
            </div>
            {test && (test.ok ? (
              <p className="inline-flex items-center gap-2 text-sm text-ok"><CheckCircle2 className="size-4" aria-hidden />
                {t('mobile.testOk', { platforms: (test.platforms ?? []).map((p) => t(`mobile.platforms.${p}`, { defaultValue: p })).join(', ') || '–' })}</p>
            ) : (
              <p className="inline-flex items-center gap-2 text-sm text-danger"><XCircle className="size-4" aria-hidden />{t(`mobile.testErrors.${test.error}`, { defaultValue: test.error })}</p>
            ))}
            <div className="flex flex-col gap-2 rounded-xl border border-line bg-surface-2/60 p-4">
              <span className="inline-flex items-center gap-2 text-sm font-medium text-ink"><LockKeyhole className="size-4 text-accent" aria-hidden />{t('mobile.privacy')}</span>
              <p className="text-xs text-muted">{t('mobile.privacyText')}</p>
              {d.data && <CopyField label={t('mobile.publicKey')} value={d.data.publicKey} />}
            </div>
          </div>
        </Card>
      </div>
      <Card>
        <SectionTitle>{t('mobile.devices')}</SectionTitle>
        <Table head={[t('mobile.device'), t('mobile.user'), t('mobile.lastSeen'), t('mobile.lastPush'), '']}
          empty={d.data && d.data.devices.length === 0 ? <Empty icon={<Smartphone className="size-6" />} title={t('mobile.noDevices')} text={t('mobile.noDevicesText')} /> : undefined}>
          {d.data?.devices.map((dev) => (
            <tr key={dev.id}>
              <td className={td}>
                <div className="flex items-center gap-2">
                  <span className="font-medium text-ink">{dev.name || t(`mobile.platforms.${dev.platform}`)}</span>
                  <Badge>{t(`mobile.platforms.${dev.platform}`)}</Badge>
                </div>
                <div className="text-xs text-subtle">{t('mobile.linkedOn', { date: formatDate(dev.createdAt, i18n.language) })}</div>
              </td>
              <td className={td}>{dev.userName || dev.user}{dev.extension && <span className="ml-1.5 font-mono text-xs text-subtle">{dev.extension}</span>}</td>
              <td className={td}>{dev.lastSeen ? formatDate(dev.lastSeen, i18n.language) : '–'}</td>
              <td className={td}>{dev.lastPush ? formatDate(dev.lastPush, i18n.language) : '–'}</td>
              <td className={`${td} text-right`}>
                <IconButton label={t('mobile.remove')} danger onClick={() => remove(dev)}><Trash2 className="size-4" /></IconButton>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
    </>
  )
}
