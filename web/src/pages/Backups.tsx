import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Archive, Cloud, DatabaseBackup, ExternalLink, Loader2, History, KeyRound, Pencil, Play, Plus, PlugZap, RotateCcw, Trash2 } from 'lucide-react'
import { Badge, CopyField, Empty, Modal, SectionTitle, Select, Table, Textarea, Toggle, formatBytes, formatDate, td, useApi } from '../components/kit'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { BackupRun, BackupTarget, Schedule, Snapshot } from '../lib/types'

const blank: BackupTarget = {
  id: 0, name: '', kind: 'sftp', host: '', port: 22, username: '', passwordSet: false, keySet: false, path: 'kiestoon',
  implicitTls: false, enabled: true,
}
const defaultPorts: Record<BackupTarget['kind'], number> = { ftp: 21, ftps: 21, sftp: 22, scp: 22, onedrive: 443, s3: 443, webdav: 443, smb: 445 }
const s3Providers = ['AWS', 'Minio', 'Wasabi', 'Ceph', 'DigitalOcean', 'Scaleway', 'Hetzner', 'Cloudflare', 'Other']
const davVendors = ['nextcloud', 'owncloud', 'sharepoint-ntlm', 'other']

type RestoreStatus = { state: '' | 'running' | 'success' | 'failed'; snapshot: string; message: string; at: string }

export function Backups() {
  const { t, i18n } = useTranslation()
  const targets = useApi<BackupTarget[]>('/backups/targets')
  const runs = useApi<BackupRun[]>('/backups/runs', 5000)
  const [connect, setConnect] = useState<BackupTarget | null>(null)
  const restore = useApi<RestoreStatus>('/backups/restore/status', 5000)
  const helper = useApi<{ available: boolean }>('/system/helper')
  const [edit, setEdit] = useState<BackupTarget | null>(null)
  const [snapsFor, setSnapsFor] = useState<BackupTarget | null>(null)
  const [keyFor, setKeyFor] = useState<{ name: string; key: string } | null>(null)
  const [msg, setMsg] = useState<{ tone: 'ok' | 'danger'; text: string } | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState('')

  async function act(key: string, fn: () => Promise<void>) {
    setBusy(key)
    setError(null)
    setMsg(null)
    try {
      await fn()
    } catch (e) {
      setError(e)
    } finally {
      setBusy('')
    }
  }
  const test = (tg: BackupTarget) => act(`test${tg.id}`, async () => {
    const r = await api<{ created: boolean }>('POST', `/backups/targets/${tg.id}/test`)
    setMsg({ tone: 'ok', text: r.created ? t('backups.testCreated', { name: tg.name }) : t('backups.testOk', { name: tg.name }) })
  })
  const run = (tg: BackupTarget, kind: 'full' | 'incremental') => act(`${kind}${tg.id}`, async () => {
    await api('POST', '/backups/run', { targetId: tg.id, kind })
    setMsg({ tone: 'ok', text: t('backups.started') })
    runs.reload()
  })
  const reveal = (tg: BackupTarget) => act(`key${tg.id}`, async () => {
    const r = await api<{ repoPassword: string }>('GET', `/backups/targets/${tg.id}/key`)
    setKeyFor({ name: tg.name, key: r.repoPassword })
  })
  async function remove(tg: BackupTarget) {
    if (!window.confirm(t('backups.confirmDelete', { name: tg.name }))) return
    await act(`del${tg.id}`, async () => {
      await api('DELETE', `/backups/targets/${tg.id}`)
      targets.reload()
    })
  }
  const running = runs.data?.some((r) => r.state === 'running')

  return (
    <>
      <PageHeader title={t('backups.title')} subtitle={t('backups.subtitle')}>
        <Button onClick={() => setEdit(blank)}>
          <Plus className="size-4" aria-hidden /> {t('backups.addTarget')}
        </Button>
      </PageHeader>
      {helper.data && !helper.data.available && <Alert tone="warn">{t('backups.noHelper')}</Alert>}
      {restore.data?.state === 'running' && <Alert tone="warn">{t('backups.restoreRunning')}</Alert>}
      {restore.data?.state === 'success' && <Alert tone="ok">{t('backups.restoreDone', { snapshot: restore.data.snapshot.slice(0, 8), at: formatDate(restore.data.at, i18n.language) })}</Alert>}
      {restore.data?.state === 'failed' && <Alert>{t('backups.restoreFailed', { message: restore.data.message })}</Alert>}
      {msg && <Alert tone={msg.tone}>{msg.text}</Alert>}
      <ErrorMessage error={error} />

      <Card>
        <SectionTitle>{t('backups.targets')}</SectionTitle>
        <Table
          head={[t('backups.name'), t('backups.location'), t('backups.state'), '']}
          empty={targets.data?.length === 0 && <Empty icon={<Archive className="size-7" />} title={t('backups.emptyTitle')} text={t('backups.emptyText')} />}
        >
          {targets.data?.map((tg) => (
            <tr key={tg.id} className={tg.enabled ? '' : 'opacity-50'}>
              <td className={`${td} text-ink`}>{tg.name}</td>
              <td className={td}>
                <span className="font-mono text-xs uppercase text-accent">{tg.kind}</span>{' '}
                {tg.kind === 'onedrive' ? (
                  <span className="font-mono text-xs">{tg.onedrive?.connected ? `${tg.onedrive.account || 'OneDrive'}/${tg.path}` : t('backups.onedrive.notConnected')}</span>
                ) : (
                  <span className="font-mono text-xs">{tg.username}@{tg.host}:{tg.port}/{tg.path}</span>
                )}
              </td>
              <td className={td}>{tg.enabled ? <Badge tone="ok" dot>{t('common.enabled')}</Badge> : <Badge dot>{t('backups.disabled')}</Badge>}</td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <div className="inline-flex flex-wrap justify-end gap-1.5">
                  {tg.kind === 'onedrive' && (
                    <Button variant={tg.onedrive?.connected ? 'ghost' : 'secondary'} onClick={() => setConnect(tg)}><Cloud className="size-4" />{tg.onedrive?.connected ? t('backups.onedrive.reconnect') : t('backups.onedrive.connect')}</Button>
                  )}
                  <Button variant="ghost" busy={busy === `test${tg.id}`} onClick={() => test(tg)}><PlugZap className="size-4" />{t('backups.test')}</Button>
                  <Button variant="secondary" busy={busy === `full${tg.id}`} disabled={running} onClick={() => run(tg, 'full')}><DatabaseBackup className="size-4" />{t('backups.full')}</Button>
                  <Button variant="secondary" busy={busy === `incremental${tg.id}`} disabled={running} onClick={() => run(tg, 'incremental')}><Play className="size-4" />{t('backups.incremental')}</Button>
                  <Button variant="ghost" onClick={() => setSnapsFor(tg)}><History className="size-4" />{t('backups.snapshots')}</Button>
                  <IconBtn label={t('backups.repoKey')} onClick={() => reveal(tg)}><KeyRound className="size-4" /></IconBtn>
                  <IconBtn label={t('common.edit')} onClick={() => setEdit(tg)}><Pencil className="size-4" /></IconBtn>
                  <IconBtn label={t('common.delete')} danger onClick={() => remove(tg)}><Trash2 className="size-4" /></IconBtn>
                </div>
              </td>
            </tr>
          ))}
        </Table>
      </Card>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
        <ScheduleCard />
        <Card>
          <SectionTitle>{t('backups.runs')}</SectionTitle>
          <Table
            head={[t('backups.started'), t('backups.target'), t('backups.kind'), t('backups.state'), t('backups.size'), '']}
            empty={runs.data?.length === 0 && <p className="py-6 text-center text-sm text-muted">{t('backups.noRuns')}</p>}
          >
            {runs.data?.map((r) => (
              <tr key={r.id}>
                <td className={`${td} whitespace-nowrap`}>{formatDate(r.startedAt, i18n.language)}</td>
                <td className={td}>{r.target}</td>
                <td className={td}>{t(`backups.kinds.${r.kind}`)} <span className="text-xs text-subtle">· {t(`backups.triggers.${r.trigger}`)}</span></td>
                <td className={td}>
                  {r.state === 'success' ? <Badge tone="ok" dot>{t('backups.states.success')}</Badge>
                    : r.state === 'failed' ? <Badge tone="danger" dot>{t('backups.states.failed')}</Badge>
                    : <Badge tone="accent" dot>{t('backups.states.running')}</Badge>}
                </td>
                <td className={`${td} font-mono text-xs`}>{r.state === 'success' ? `+${formatBytes(r.bytesAdded)}` : ''}</td>
                <td className={`${td} max-w-xs truncate text-xs text-subtle`} title={r.message}>{r.snapshotId ? r.snapshotId.slice(0, 8) : r.message}</td>
              </tr>
            ))}
          </Table>
        </Card>
      </div>

      {edit && <TargetForm value={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); targets.reload() }} />}
      {connect && <OneDriveConnect target={connect} onClose={() => { setConnect(null); targets.reload() }} />}
      {snapsFor && <SnapshotsModal target={snapsFor} onClose={() => setSnapsFor(null)} onRestore={() => { setSnapsFor(null); restore.reload() }} />}
      {keyFor && (
        <Modal title={t('backups.repoKey')} subtitle={keyFor.name} onClose={() => setKeyFor(null)}>
          <Alert tone="warn">{t('backups.repoKeyHint')}</Alert>
          <CopyField label={t('backups.repoKey')} value={keyFor.key} secret />
        </Modal>
      )}
    </>
  )
}

function IconBtn({ label, onClick, danger, children }: { label: string; onClick: () => void; danger?: boolean; children: React.ReactNode }) {
  return (
    <button type="button" onClick={onClick} title={label} aria-label={label}
      className={`inline-flex size-11 items-center justify-center rounded-lg text-muted hover:bg-tint/5 ${danger ? 'hover:bg-danger/10 hover:text-danger' : 'hover:text-ink'}`}>
      {children}
    </button>
  )
}

function ScheduleCard() {
  const { t } = useTranslation()
  const sc = useApi<Schedule>('/backups/schedule')
  const [s, setS] = useState<Schedule | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  useEffect(() => { if (sc.data && !s) setS(sc.data) }, [sc.data, s])
  if (!s) return <Card><ErrorMessage error={sc.error} /></Card>
  const days = [0, 1, 2, 3, 4, 5, 6]

  async function save() {
    setBusy(true)
    setError(null)
    setSaved(false)
    try {
      setS(await api<Schedule>('PUT', '/backups/schedule', s))
      setSaved(true)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const ret = (k: keyof Schedule['retention'], v: number) => setS({ ...s, retention: { ...s.retention, [k]: v } })
  return (
    <Card>
      <SectionTitle>{t('backups.schedule')}</SectionTitle>
      <div className="flex flex-col gap-3">
        <Toggle label={t('backups.scheduleOn')} hint={t('backups.scheduleHint')} checked={s.enabled} onChange={(v) => setS({ ...s, enabled: v })} />
        <div className="grid gap-3 sm:grid-cols-2">
          <Input label={t('backups.time')} type="time" value={s.time} onChange={(e) => setS({ ...s, time: e.target.value })} />
          <Select label={t('backups.fullDay')} value={s.fullWeekday} onChange={(e) => setS({ ...s, fullWeekday: Number(e.target.value) })}>
            {days.map((d) => <option key={d} value={d}>{t(`backups.weekdays.${d}`)}</option>)}
          </Select>
        </div>
        <div className="grid grid-cols-3 gap-3">
          <Input label={t('backups.keepDaily')} type="number" min={0} value={s.retention.daily} onChange={(e) => ret('daily', Number(e.target.value))} />
          <Input label={t('backups.keepWeekly')} type="number" min={0} value={s.retention.weekly} onChange={(e) => ret('weekly', Number(e.target.value))} />
          <Input label={t('backups.keepMonthly')} type="number" min={0} value={s.retention.monthly} onChange={(e) => ret('monthly', Number(e.target.value))} />
        </div>
        <p className="text-xs text-subtle">{t('backups.allTargets')}</p>
        <ErrorMessage error={error} />
        {saved && <Alert tone="ok">{t('common.saved')}</Alert>}
        <div className="flex justify-end"><Button busy={busy} onClick={save}>{t('common.save')}</Button></div>
      </div>
    </Card>
  )
}

function TargetForm({ value, onClose, onSaved }: { value: BackupTarget; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [v, setV] = useState<BackupTarget>(value)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const set = <K extends keyof BackupTarget>(k: K, x: BackupTarget[K]) => setV((o) => ({ ...o, [k]: x }))
  const ssh = v.kind === 'sftp' || v.kind === 'scp'

  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api(v.id ? 'PUT' : 'POST', v.id ? `/backups/targets/${v.id}` : '/backups/targets', v)
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={v.id ? t('backups.editTarget') : t('backups.addTarget')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={save}>{t('common.save')}</Button></>}>
      <Input label={t('backups.name')} value={v.name} onChange={(e) => set('name', e.target.value)} placeholder="NAS office" />
        <Select label={t('backups.protocol')} value={v.kind} onChange={(e) => {
          const k = e.target.value as BackupTarget['kind']
          setV((o) => ({
            ...o, kind: k, port: o.port === defaultPorts[o.kind] ? defaultPorts[k] : o.port,
            onedrive: k === 'onedrive' ? o.onedrive ?? { tenant: 'common', clientId: '' } : o.onedrive,
            provider: k === 's3' ? (s3Providers.includes(o.provider ?? '') ? o.provider : 'AWS') : k === 'webdav' ? (davVendors.includes(o.provider ?? '') ? o.provider : 'nextcloud') : '',
            region: k === 's3' ? o.region || 'us-east-1' : '',
            path: k === 's3' && o.path === blank.path ? 'kiestoon-backup' : k !== 's3' && o.path === 'kiestoon-backup' ? blank.path : o.path,
          }))
        }}>
          <option value="sftp">SFTP</option>
          <option value="scp">SCP (SSH)</option>
          <option value="ftps">FTPS</option>
          <option value="ftp">FTP</option>
          <option value="smb">SMB / Windows share</option>
          <option value="webdav">WebDAV (Nextcloud)</option>
          <option value="s3">S3 (object storage)</option>
          <option value="onedrive">Microsoft OneDrive</option>
        </Select>
      {v.kind !== 'onedrive' && (
        <div className="grid gap-3 sm:grid-cols-[minmax(0,3fr)_minmax(0,1fr)]">
          <Input label={v.kind === 's3' ? t('backups.endpoint') : t('backups.host')} value={v.host} onChange={(e) => set('host', e.target.value)}
            placeholder={v.kind === 's3' ? 's3.eu-central-1.amazonaws.com' : v.kind === 'smb' ? 'nas.office.lan' : 'backup.example.com'} />
          <Input label={t('backups.port')} type="number" value={v.port} onChange={(e) => set('port', Number(e.target.value))} />
        </div>
      )}
      {v.kind === 'onedrive' && v.onedrive && <>
        <p className="text-xs text-subtle">{t('backups.onedrive.intro')}</p>
        <div className="grid gap-3 sm:grid-cols-2">
          <Input label={t('backups.onedrive.clientId')} value={v.onedrive.clientId} className="font-mono"
            onChange={(e) => set('onedrive', { ...v.onedrive!, clientId: e.target.value.trim() })} placeholder="00000000-0000-0000-0000-000000000000" />
          <Input label={t('backups.onedrive.tenant')} value={v.onedrive.tenant} className="font-mono" hint={t('backups.onedrive.tenantHint')}
            onChange={(e) => set('onedrive', { ...v.onedrive!, tenant: e.target.value.trim() })} placeholder="common" />
        </div>
        <Input label={t('backups.onedrive.clientSecret')} type="password" value={v.onedrive.clientSecret ?? ''} autoComplete="new-password"
          placeholder={v.onedrive.secretSet ? t('common.keepSecret') : ''} hint={t('backups.onedrive.clientSecretHint')}
          onChange={(e) => set('onedrive', { ...v.onedrive!, clientSecret: e.target.value })} />
        {!v.id && <Alert tone="warn">{t('backups.onedrive.connectAfterSave')}</Alert>}
      </>}
      {v.kind === 's3' && <>
        <div className="grid gap-3 sm:grid-cols-2">
          <Select label={t('backups.s3.provider')} value={v.provider ?? 'AWS'} onChange={(e) => set('provider', e.target.value)}>
            {s3Providers.map((p) => <option key={p} value={p}>{p === 'Other' ? t('backups.s3.other') : p}</option>)}
          </Select>
          <Input label={t('backups.s3.region')} value={v.region ?? ''} onChange={(e) => set('region', e.target.value.trim())} placeholder="eu-central-1" className="font-mono" hint={t('backups.s3.regionHint')} />
        </div>
        <p className="text-xs text-subtle">{t('backups.s3.hint')}</p>
      </>}
      {v.kind === 'webdav' && <>
        <Select label={t('backups.webdav.vendor')} value={v.provider ?? 'nextcloud'} onChange={(e) => set('provider', e.target.value)}>
          {davVendors.map((p) => <option key={p} value={p}>{t(`backups.webdav.vendors.${p}`)}</option>)}
        </Select>
        <p className="text-xs text-subtle">{t('backups.webdav.hint')}</p>
      </>}
      {v.kind === 'smb' && <>
        <Input label={t('backups.smb.domain')} value={v.domain ?? ''} onChange={(e) => set('domain', e.target.value.trim())} placeholder="WORKGROUP" hint={t('backups.smb.domainHint')} />
        <p className="text-xs text-subtle">{t('backups.smb.hint')}</p>
      </>}
      {(v.kind === 's3' || v.kind === 'webdav') && (
        <Toggle label={t('backups.plain')} hint={t('backups.plainHint')} checked={!!v.plain}
          onChange={(x) => setV((o) => ({ ...o, plain: x, port: o.port === 443 && x ? 80 : o.port === 80 && !x ? 443 : o.port }))} />
      )}
      {v.kind === 'ftp' && <Alert tone="warn">{t('backups.ftpWarn')}</Alert>}
      {v.kind === 'scp' && <p className="text-xs text-subtle">{t('backups.scpHint')}</p>}
      {v.kind === 'ftps' && <Toggle label={t('backups.implicitTls')} hint={t('backups.implicitTlsHint')} checked={v.implicitTls} onChange={(x) => set('implicitTls', x)} />}
      {v.kind !== 'onedrive' && (
        <div className="grid gap-3 sm:grid-cols-2">
          <Input label={v.kind === 's3' ? t('backups.s3.accessKey') : t('backups.username')} value={v.username} onChange={(e) => set('username', e.target.value)} autoComplete="off" className={v.kind === 's3' ? 'font-mono' : undefined} />
          <Input label={v.kind === 's3' ? t('backups.s3.secretKey') : t('backups.password')} type="password" value={v.password ?? ''} onChange={(e) => set('password', e.target.value)}
            placeholder={v.passwordSet ? t('common.keepSecret') : ''} autoComplete="new-password" hint={ssh ? t('backups.passwordOrKey') : undefined} />
        </div>
      )}
      {ssh && (
        <Textarea label={t('backups.privateKey')} value={v.privateKey ?? ''} onChange={(e) => set('privateKey', e.target.value)}
          placeholder={v.keySet ? t('common.keepSecret') : '-----BEGIN OPENSSH PRIVATE KEY-----'} hint={t('backups.privateKeyHint')} />
      )}
      <Input label={v.kind === 's3' ? t('backups.s3.bucket') : v.kind === 'smb' ? t('backups.smb.share') : t('backups.path')} value={v.path} onChange={(e) => set('path', e.target.value)}
        placeholder={v.kind === 'smb' ? 'Backups/kiestoon' : v.kind === 'webdav' ? 'kiestoon' : undefined}
        hint={v.kind === 's3' ? t('backups.s3.bucketHint') : v.kind === 'smb' ? t('backups.smb.shareHint') : v.kind === 'webdav' ? t('backups.webdav.pathHint') : t('backups.pathHint')} />
      <Toggle label={t('common.enabled')} hint={t('backups.enabledHint')} checked={v.enabled} onChange={(x) => set('enabled', x)} />
      {!v.id && <p className="text-xs text-subtle">{t('backups.encryption')}</p>}
      <ErrorMessage error={error} />
    </Modal>
  )
}

function SnapshotsModal({ target, onClose, onRestore }: { target: BackupTarget; onClose: () => void; onRestore: () => void }) {
  const { t, i18n } = useTranslation()
  const snaps = useApi<Snapshot[]>(`/backups/targets/${target.id}/snapshots`)
  const [pick, setPick] = useState<Snapshot | null>(null)
  const [confirm, setConfirm] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)

  async function restore() {
    if (!pick) return
    setBusy(true)
    setError(null)
    try {
      await api('POST', '/backups/restore', { targetId: target.id, snapshot: pick.id, confirm })
      onRestore()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const sorted = [...(snaps.data ?? [])].sort((a, b) => b.time.localeCompare(a.time))
  return (
    <Modal wide title={t('backups.snapshots')} subtitle={target.name} onClose={onClose}>
      {snaps.loading && <p className="text-sm text-muted">{t('app.loading')}</p>}
      <ErrorMessage error={snaps.error} />
      {!pick && (
        <Table head={[t('backups.time'), t('backups.kind'), 'ID', '']}
          empty={!snaps.loading && sorted.length === 0 && <p className="py-6 text-center text-sm text-muted">{t('backups.noSnapshots')}</p>}>
          {sorted.map((s) => (
            <tr key={s.id}>
              <td className={`${td} whitespace-nowrap`}>{formatDate(s.time, i18n.language)}</td>
              <td className={td}>{(s.tags ?? []).filter((x) => x === 'full' || x === 'incremental').map((x) => <Badge key={x} tone={x === 'full' ? 'accent' : 'muted'}>{t(`backups.kinds.${x}`)}</Badge>)}</td>
              <td className={`${td} font-mono text-xs`}>{s.shortId}</td>
              <td className={`${td} text-right`}><Button variant="danger" onClick={() => { setPick(s); setConfirm('') }}><RotateCcw className="size-4" />{t('backups.restore')}</Button></td>
            </tr>
          ))}
        </Table>
      )}
      {pick && (
        <div className="flex flex-col gap-4">
          <Alert tone="warn">{t('backups.restoreWarn', { time: formatDate(pick.time, i18n.language) })}</Alert>
          <Input label={t('backups.typeRestore')} value={confirm} onChange={(e) => setConfirm(e.target.value)} placeholder="RESTORE" autoComplete="off" />
          <ErrorMessage error={error} />
          <div className="flex justify-end gap-2.5">
            <Button variant="ghost" onClick={() => setPick(null)}>{t('common.back')}</Button>
            <Button variant="danger" busy={busy} disabled={confirm !== 'RESTORE'} onClick={restore}><RotateCcw className="size-4" />{t('backups.restoreNow')}</Button>
          </div>
        </div>
      )}
    </Modal>
  )
}

/** Signs in to Microsoft with the device code flow: the administrator enters a code on microsoft.com/devicelogin. */
function OneDriveConnect({ target, onClose }: { target: BackupTarget; onClose: () => void }) {
  const { t } = useTranslation()
  const [code, setCode] = useState<{ userCode: string; verificationUri: string; interval: number } | null>(null)
  const [done, setDone] = useState<{ account: string } | null>(null)
  const [error, setError] = useState<unknown>(null)
  useEffect(() => {
    let stop = false
    api<{ userCode: string; verificationUri: string; interval: number }>('POST', `/backups/targets/${target.id}/onedrive/start`)
      .then((c) => { if (!stop) setCode(c) }).catch((e) => !stop && setError(e))
    return () => { stop = true }
  }, [target.id])
  useEffect(() => {
    if (!code || done) return
    const id = window.setInterval(async () => {
      try {
        const r = await api<{ connected: boolean; account?: string }>('POST', `/backups/targets/${target.id}/onedrive/poll`)
        if (r.connected) setDone({ account: r.account ?? '' })
      } catch (e) {
        setError(e)
        window.clearInterval(id)
      }
    }, code.interval * 1000)
    return () => window.clearInterval(id)
  }, [code, done, target.id])
  return (
    <Modal title={t('backups.onedrive.connectTitle', { name: target.name })} onClose={onClose}
      footer={<Button variant={done ? 'primary' : 'ghost'} onClick={onClose}>{done ? t('common.close') : t('common.cancel')}</Button>}>
      <ErrorMessage error={error} />
      {done ? (
        <Alert tone="ok">{t('backups.onedrive.connected', { account: done.account })}</Alert>
      ) : code ? (
        <>
          <p className="text-sm text-ink-2">{t('backups.onedrive.step1')}</p>
          <a href={code.verificationUri} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1.5 text-accent hover:underline">
            {code.verificationUri} <ExternalLink className="size-4" aria-hidden />
          </a>
          <p className="text-sm text-ink-2">{t('backups.onedrive.step2')}</p>
          <CopyField label={t('backups.onedrive.code')} value={code.userCode} />
          <p className="flex items-center gap-2 text-xs text-subtle"><Loader2 className="size-4 animate-spin" aria-hidden />{t('backups.onedrive.waiting')}</p>
        </>
      ) : !error && <p className="flex items-center gap-2 text-sm text-muted"><Loader2 className="size-4 animate-spin" aria-hidden />{t('app.loading')}</p>}
    </Modal>
  )
}
