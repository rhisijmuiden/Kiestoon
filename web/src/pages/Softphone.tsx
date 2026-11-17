import { useTranslation } from 'react-i18next'
import { Download, Monitor, Rocket, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { CopyField, Empty, SectionTitle, Table, formatBytes, formatDate, td, useApi } from '../components/kit'
import { IconButton } from '../components/pbx'
import { Card, ErrorMessage, PageHeader } from '../components/ui'
import type { DesktopInfo } from '../components/WindowsSoftphone'
import { api } from '../lib/api'

type DesktopClient = {
  id: number
  userId: number
  username: string
  displayName: string
  extension: string
  linkedAt: string
  expiresAt: string
}

/** Windows softphone: download the MSI for employees or a rollout, and see the linked softphones. */
export function Softphone() {
  const { t, i18n } = useTranslation()
  const info = useApi<DesktopInfo>('/desktop/info')
  const clients = useApi<DesktopClient[]>('/desktop/clients')
  const [error, setError] = useState<unknown>(null)
  const inst = info.data?.installer
  const url = inst ? new URL(inst.url, window.location.origin).toString() : ''
  const server = info.data?.server || window.location.origin

  async function remove(c: DesktopClient) {
    if (!window.confirm(t('windowsSoftphone.removeConfirm', { user: c.displayName || c.username }))) return
    try {
      await api('DELETE', `/desktop/clients/${c.id}`, {})
      clients.reload()
    } catch (e) {
      setError(e)
    }
  }

  return (
    <>
      <PageHeader title={t('windowsSoftphone.title')} subtitle={t('windowsSoftphone.subtitle')}>
        {inst && (
          <a href={inst.url} download className="inline-flex min-h-11 items-center justify-center gap-2 rounded-[10px] bg-gradient-to-br from-accent to-accent-2 px-4 text-sm font-semibold text-on-accent shadow-[0_8px_30px_-10px_rgb(34_211_238_/_0.6)] hover:brightness-110">
            <Download className="size-4" aria-hidden />{t('windowsSoftphone.download')}
          </a>
        )}
      </PageHeader>
      <ErrorMessage error={error || info.error || clients.error} />
      <div className="grid gap-4 xl:grid-cols-2">
        <Card>
          <SectionTitle><span className="inline-flex items-center gap-2"><Monitor className="size-5 text-accent" aria-hidden />{t('windowsSoftphone.installer')}</span></SectionTitle>
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted">{t('windowsSoftphone.intro')}</p>
            {inst ? (
              <>
                <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-sm">
                  <dt className="text-muted">{t('windowsSoftphone.version')}</dt><dd className="font-mono text-ink">{info.data?.version}</dd>
                  <dt className="text-muted">{t('windowsSoftphone.file')}</dt><dd className="break-all font-mono text-ink">{inst.name}</dd>
                  <dt className="text-muted">{t('windowsSoftphone.size')}</dt><dd className="text-ink">{formatBytes(inst.size)}</dd>
                </dl>
                <a href={inst.url} download className="inline-flex min-h-11 w-fit items-center justify-center gap-2 rounded-[10px] border border-line px-4 text-sm font-medium text-ink-2 hover:bg-tint/5">
                  <Download className="size-4" aria-hidden />{t('windowsSoftphone.downloadMsi')} <span className="text-xs text-subtle">({formatBytes(inst.size)})</span>
                </a>
                <CopyField label={t('windowsSoftphone.url')} value={url} />
                <CopyField label="SHA-256" value={inst.sha256} />
              </>
            ) : info.data && (
              <p className="rounded-xl border border-line bg-surface-2/60 p-4 text-sm text-muted">{t('windowsSoftphone.noInstaller')}</p>
            )}
          </div>
        </Card>
        <Card>
          <SectionTitle><span className="inline-flex items-center gap-2"><Rocket className="size-5 text-accent" aria-hidden />{t('windowsSoftphone.rollout')}</span></SectionTitle>
          <div className="flex flex-col gap-4">
            <ol className="flex flex-col gap-2 text-sm text-ink-2">
              {(['step1', 'step2', 'step3'] as const).map((s, i) => (
                <li key={s} className="flex gap-3">
                  <span className="grid size-6 shrink-0 place-items-center rounded-full bg-accent/15 font-mono text-xs text-accent">{i + 1}</span>
                  <span>{t(`windowsSoftphone.${s}`)}</span>
                </li>
              ))}
            </ol>
            <p className="text-sm text-muted">{t('windowsSoftphone.silentText')}</p>
            <CopyField label={t('windowsSoftphone.silent')} value={`msiexec /i "${inst?.name ?? 'Kiestoon-Softphone.msi'}" /qn`} />
            <CopyField label={t('windowsSoftphone.server')} value={server} />
            {info.data?.fingerprint && (
              <p className="break-all font-mono text-[11px] text-subtle">{t('windowsSoftphone.fingerprint')}: {info.data.fingerprint.match(/.{2}/g)?.join(':')}</p>
            )}
          </div>
        </Card>
      </div>
      <Card>
        <SectionTitle>{t('windowsSoftphone.clients')}</SectionTitle>
        <Table head={[t('windowsSoftphone.user'), t('windowsSoftphone.extension'), t('windowsSoftphone.linkedAt'), t('windowsSoftphone.expiresAt'), '']}
          empty={clients.data && clients.data.length === 0 ? <Empty icon={<Monitor className="size-6" />} title={t('windowsSoftphone.noClients')} text={t('windowsSoftphone.noClientsText')} /> : undefined}>
          {clients.data?.map((c) => (
            <tr key={c.id}>
              <td className={td}>
                <div className="font-medium text-ink">{c.displayName || c.username}</div>
                {c.displayName && <div className="text-xs text-subtle">{c.username}</div>}
              </td>
              <td className={`${td} font-mono`}>{c.extension || '–'}</td>
              <td className={td}>{formatDate(c.linkedAt, i18n.language)}</td>
              <td className={td}>{formatDate(c.expiresAt, i18n.language)}</td>
              <td className={`${td} text-right`}>
                <IconButton label={t('windowsSoftphone.remove')} danger onClick={() => remove(c)}><Trash2 className="size-4" /></IconButton>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
    </>
  )
}
