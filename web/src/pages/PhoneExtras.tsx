import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CloudDownload, Download, ExternalLink, HardDriveDownload, Pencil, Radar, Trash2, Upload } from 'lucide-react'
import { Badge, CopyField, Empty, Modal, SectionTitle, Select, Table, Toggle, formatBytes, formatDate, td, useApi } from '../components/kit'
import { IconButton } from '../components/pbx'
import { Alert, Button, Card, ErrorMessage, Input } from '../components/ui'
import { ApiError, api } from '../lib/api'
import type { Firmware, PhoneModel, ZtpData, ZtpSettings } from '../lib/types'

export const vendorName = (v: string) => ({ poly: 'Poly', htek: 'Htek', akuvox: 'Akuvox', cisco: 'Cisco' } as Record<string, string>)[v] ?? v[0].toUpperCase() + v.slice(1)

/**
 * Sends a firmware file as the raw request body with upload progress. Errors say where it went
 * wrong: Kiestoon answers with a JSON error; anything else came from a proxy in front of it, and a
 * broken connection has no answer at all.
 */
function uploadFirmware(url: string, file: File, onProgress: (pct: number) => void): Promise<Firmware> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', url)
    xhr.withCredentials = true
    xhr.setRequestHeader('Content-Type', 'application/octet-stream')
    xhr.setRequestHeader('X-Kiestoon-Upload', '1')
    let sent = 0
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) {
        sent = e.loaded
        onProgress(Math.min(100, Math.floor((e.loaded / e.total) * 100)))
      }
    }
    xhr.onload = () => {
      let data: { error?: string; field?: string; message?: string } & Partial<Firmware> = {}
      let isJSON = true
      try {
        data = JSON.parse(xhr.responseText)
      } catch {
        isJSON = false
      }
      if (xhr.status >= 200 && xhr.status < 300 && isJSON) return resolve(data as Firmware)
      if (isJSON && data.error) return reject(new ApiError(xhr.status, data.error, data.field, data.message))
      // Not from Kiestoon: a reverse proxy refused or cut off the upload.
      const code = xhr.status === 413 ? 'proxy_too_large' : xhr.status === 502 || xhr.status === 504 ? 'proxy_timeout' : 'internal'
      reject(new ApiError(xhr.status, code, undefined, `HTTP ${xhr.status}`))
    }
    xhr.onerror = () => reject(new ApiError(0, 'upload_interrupted', undefined, `${Math.round(sent / 1048576)} / ${Math.round(file.size / 1048576)} MB`))
    xhr.ontimeout = xhr.onerror
    xhr.send(file)
  })
}

/** Firmware files per phone model. Phones fetch the active firmware for their model on the next provisioning. */
export function FirmwarePanel({ models }: { models: PhoneModel[] }) {
  const { t, i18n } = useTranslation()
  const list = useApi<{ firmware: Firmware[]; vendors: string[] }>('/firmware')
  const [vendor, setVendor] = useState('yealink')
  const [url, setUrl] = useState('')
  const [busy, setBusy] = useState('')
  const [error, setError] = useState<unknown>(null)
  const [edit, setEdit] = useState<Firmware | null>(null)

  const [progress, setProgress] = useState(0)

  async function upload(file: File) {
    setBusy('upload')
    setError(null)
    setProgress(0)
    try {
      const data = await uploadFirmware(`/api/firmware/upload?vendor=${encodeURIComponent(vendor)}&name=${encodeURIComponent(file.name)}`, file, setProgress)
      list.reload()
      setEdit(data)
    } catch (e) {
      setError(e)
    } finally {
      setBusy('')
    }
  }
  async function fetchUrl() {
    setBusy('fetch')
    setError(null)
    try {
      const f = await api<Firmware>('POST', '/firmware/fetch', { vendor, url })
      setUrl('')
      list.reload()
      setEdit(f)
    } catch (e) {
      setError(e)
    } finally {
      setBusy('')
    }
  }
  async function remove(f: Firmware) {
    if (!window.confirm(t('common.confirmDelete', { name: f.filename }))) return
    await api('DELETE', `/firmware/${f.id}`)
    list.reload()
  }
  const modelName = (vendor: string, id: string) => models.find((m) => m.vendor === vendor && m.id === id)?.name ?? id

  return (
    <>
      <Card>
        <SectionTitle><span className="inline-flex items-center gap-2"><HardDriveDownload className="size-5 text-accent" aria-hidden />{t('firmware.title')}</span></SectionTitle>
        <p className="mb-4 max-w-3xl text-sm text-muted">{t('firmware.intro')}</p>
        <div className="grid items-end gap-3 lg:grid-cols-[180px_1fr_auto_auto]">
          <Select label={t('firmware.vendor')} value={vendor} onChange={(e) => setVendor(e.target.value)}>
            {(list.data?.vendors ?? ['yealink']).map((v) => <option key={v} value={v}>{vendorName(v)}</option>)}
          </Select>
          <Input label={t('firmware.url')} value={url} onChange={(e) => setUrl(e.target.value.trim())} placeholder="https://…/T54W-96.86.0.70.rom" className="font-mono" />
          <Button variant="secondary" busy={busy === 'fetch'} disabled={!url || !!busy} onClick={fetchUrl}><CloudDownload className="size-4" aria-hidden />{t('firmware.fetch')}</Button>
          <label className={`inline-flex min-h-11 cursor-pointer items-center justify-center gap-2 rounded-[10px] bg-gradient-to-br from-accent to-accent-2 px-4 text-sm font-semibold text-on-accent shadow-[0_8px_30px_-10px_rgb(34_211_238_/_0.6)] hover:brightness-110 focus-within:ring-2 focus-within:ring-accent/40 ${busy ? 'pointer-events-none opacity-60' : ''}`}>
            <Upload className="size-4" aria-hidden />{busy === 'upload' ? `${t('firmware.uploading')} ${progress}%` : t('firmware.upload')}
            <input type="file" className="sr-only" accept=".rom,.bin,.zip,.loads,.ld,.img,.fw,.z" onChange={(e) => { const f = e.target.files?.[0]; e.target.value = ''; if (f) upload(f) }} />
          </label>
        </div>
        {busy === 'upload' && (
          <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-tint/10" role="progressbar" aria-valuenow={progress} aria-valuemin={0} aria-valuemax={100} aria-label={t('firmware.uploading')}>
            <div className="h-full rounded-full bg-gradient-to-r from-accent to-accent-2 transition-[width]" style={{ width: `${progress}%` }} />
          </div>
        )}
        <p className="mt-2 text-xs text-subtle">{t('firmware.hint')}</p>
        <div className="mt-3"><ErrorMessage error={error || list.error} /></div>
      </Card>
      <Card>
        <Table head={[t('firmware.vendor'), t('firmware.file'), t('firmware.version'), t('firmware.models'), t('firmware.added'), '', '']}
          empty={list.data?.firmware.length === 0 && <Empty icon={<HardDriveDownload className="size-7" />} title={t('firmware.emptyTitle')} text={t('firmware.emptyText')} />}>
          {list.data?.firmware.map((f) => (
            <tr key={f.id}>
              <td className={td}>{vendorName(f.vendor)}</td>
              <td className={`${td} text-ink`}>
                <span className="font-mono text-xs">{f.filename}</span>
                <span className="block text-xs text-subtle">{formatBytes(f.size)}{f.files > 1 ? ` · ${t('firmware.files', { count: f.files })}` : ''}</span>
              </td>
              <td className={`${td} font-mono text-xs`}>{f.version || '–'}</td>
              <td className={`${td} text-xs`}>{f.models.length ? f.models.map((m) => modelName(f.vendor, m)).join(', ') : <span className="text-subtle">{t('firmware.noModels')}</span>}</td>
              <td className={`${td} text-xs`}>{formatDate(f.createdAt, i18n.language)}</td>
              <td className={td}>{f.active ? <Badge tone="ok" dot>{t('firmware.active')}</Badge> : <Badge>{t('firmware.inactive')}</Badge>}</td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <IconButton label={t('common.edit')} onClick={() => setEdit(f)}><Pencil className="size-4" aria-hidden /></IconButton>
                <IconButton label={t('common.delete')} danger onClick={() => remove(f)}><Trash2 className="size-4" aria-hidden /></IconButton>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
      {edit && <FirmwareForm value={edit} models={models.filter((m) => m.vendor === edit.vendor)} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); list.reload() }} />}
    </>
  )
}

function FirmwareForm({ value, models, onClose, onSaved }: { value: Firmware; models: PhoneModel[]; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [f, setF] = useState(value)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const toggle = (id: string) => setF((o) => ({ ...o, models: o.models.includes(id) ? o.models.filter((m) => m !== id) : [...o.models, id] }))
  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api('PUT', `/firmware/${f.id}`, { version: f.version, models: f.models, active: f.active })
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={t('firmware.edit')} subtitle={`${vendorName(f.vendor)} · ${f.filename}`} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <Input label={t('firmware.version')} value={f.version} onChange={(e) => setF({ ...f, version: e.target.value })} placeholder="96.86.0.70" className="font-mono" hint={t('firmware.versionHint')} />
      <div>
        <h3 className="mb-1 text-sm font-medium text-ink-2">{t('firmware.models')}</h3>
        <p className="mb-3 text-xs text-subtle">{t('firmware.modelsHint')}</p>
        <div className="grid gap-2 sm:grid-cols-3">
          {models.map((m) => (
            <label key={m.id} className={`flex min-h-10 cursor-pointer items-center gap-2 rounded-lg border px-3 text-sm ${f.models.includes(m.id) ? 'border-accent/50 bg-accent/10 text-ink' : 'border-line text-muted'}`}>
              <input type="checkbox" checked={f.models.includes(m.id)} onChange={() => toggle(m.id)} className="accent-[var(--color-accent)]" />{m.name}
            </label>
          ))}
        </div>
      </div>
      <Toggle label={t('firmware.activate')} hint={t('firmware.activateHint')} checked={f.active} onChange={(v) => setF({ ...f, active: v })} />
      <p className="break-all font-mono text-[11px] text-subtle">SHA-256 {f.sha256}</p>
    </Modal>
  )
}

/** Zero-touch provisioning: Yealink RPS through its API, the other vendors by hand with a guide and a MAC export. */
export function ZtpPanel() {
  const { t } = useTranslation()
  const d = useApi<ZtpData>('/provisioning/ztp')
  const [edit, setEdit] = useState<ZtpSettings | null>(null)
  if (!d.data) return <ErrorMessage error={d.error} />
  const z = d.data
  const withCreds = (path: string) => {
    try {
      const u = new URL(z.effectiveUrl)
      u.username = z.username
      u.password = z.password
      return u.toString() + path
    } catch {
      return z.effectiveUrl + path
    }
  }
  return (
    <>
      <Card>
        <SectionTitle action={<Button variant="secondary" onClick={() => setEdit(z.settings)}>{t('ztp.settings')}</Button>}>
          <span className="inline-flex items-center gap-2"><Radar className="size-5 text-accent" aria-hidden />{t('ztp.title')}</span>
        </SectionTitle>
        <p className="mb-4 max-w-3xl text-sm text-muted">{t('ztp.intro')}</p>
        <div className="grid gap-3 lg:grid-cols-2">
          <CopyField label={t('ztp.serverUrl')} value={withCreds('')} secret />
          <div className="flex flex-col gap-1">
            <span className="text-xs text-muted">Yealink RPS</span>
            <div className="flex min-h-10 items-center gap-2">
              {z.settings.yealink.enabled ? <Badge tone="ok" dot>{t('ztp.yealinkOn')}</Badge> : <Badge dot>{t('ztp.yealinkOff')}</Badge>}
              <span className="text-xs text-subtle">{t('ztp.yealinkHint')}</span>
            </div>
          </div>
        </div>
        {!z.effectiveUrl.startsWith('https://') && <div className="mt-3"><Alert tone="warn">{t('ztp.httpWarn')}</Alert></div>}
      </Card>
      <Card>
        <SectionTitle>{t('ztp.guides')}</SectionTitle>
        <p className="mb-4 max-w-3xl text-sm text-muted">{t('ztp.guidesIntro')}</p>
        <Table head={[t('firmware.vendor'), t('ztp.service'), t('ztp.enterUrl'), '']}>
          {z.guides.map((g) => (
            <tr key={g.vendor}>
              <td className={`${td} text-ink`}>{vendorName(g.vendor)}</td>
              <td className={td}>
                <a href={g.portal} target="_blank" rel="noreferrer noopener" className="inline-flex items-center gap-1.5 text-accent hover:underline">{g.service}<ExternalLink className="size-3.5" aria-hidden /></a>
              </td>
              <td className={`${td} max-w-[420px]`}><code className="break-all font-mono text-xs text-ink-2">{z.effectiveUrl}{g.path}</code></td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <a href={`/api/provisioning/ztp/export?vendor=${g.vendor}`} className="inline-flex min-h-10 items-center gap-1.5 rounded-lg px-3 text-sm text-muted hover:bg-tint/5 hover:text-accent">
                  <Download className="size-4" aria-hidden />{t('ztp.exportCsv')}
                </a>
              </td>
            </tr>
          ))}
        </Table>
        <p className="mt-3 text-xs text-subtle">{t('ztp.credsHint', { user: z.username })}</p>
      </Card>
      {edit && <ZtpForm value={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); d.reload() }} />}
    </>
  )
}

function ZtpForm({ value, onClose, onSaved }: { value: ZtpSettings; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [x, setX] = useState<ZtpSettings>(value)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const y = x.yealink
  const setY = (p: Partial<ZtpSettings['yealink']>) => setX({ ...x, yealink: { ...y, ...p } })
  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api('PUT', '/provisioning/ztp', x)
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={t('ztp.settings')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <Input label={t('ztp.serverUrlOverride')} value={x.serverUrl} onChange={(e) => setX({ ...x, serverUrl: e.target.value.trim() })} placeholder="https://pbx.example.com/prov/" className="font-mono" hint={t('ztp.serverUrlHint')} />
      <Toggle label={t('ztp.yealinkEnable')} hint={t('ztp.yealinkEnableHint')} checked={y.enabled} onChange={(v) => setY({ enabled: v })} />
      {y.enabled && <>
        <div className="grid gap-4 sm:grid-cols-2">
          <Input label={t('ztp.accessKey')} value={y.accessKey} onChange={(e) => setY({ accessKey: e.target.value.trim() })} className="font-mono" autoComplete="off" />
          <Input label={t('ztp.secret')} type="password" value={y.secret ?? ''} onChange={(e) => setY({ secret: e.target.value.trim() })} placeholder={y.secretSet ? t('common.keepSecret') : ''} autoComplete="new-password" />
        </div>
        <Input label={t('ztp.api')} value={y.api} onChange={(e) => setY({ api: e.target.value.trim() })} placeholder="https://api-dm.yealink.com:8443" className="font-mono" hint={t('ztp.apiHint')} />
      </>}
    </Modal>
  )
}

/** Shows the RPS state of a Yealink phone and registers it again on request. */
export function ZtpBadge({ phoneId, state, error, onDone }: { phoneId: number; state?: string; error?: string; onDone: () => void }) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState<unknown>(null)
  async function register() {
    setBusy(true)
    setErr(null)
    try {
      await api('POST', `/phones/${phoneId}/ztp`)
      onDone()
    } catch (e) {
      setErr(e)
    } finally {
      setBusy(false)
    }
  }
  const tone = state === 'registered' ? 'ok' : state === 'error' || state === 'elsewhere' ? 'danger' : 'muted'
  const code = err instanceof ApiError ? err.code : ''
  return (
    <span className="inline-flex flex-wrap items-center gap-1.5">
      <button type="button" onClick={register} disabled={busy} title={error || t('ztp.registerNow')} className="disabled:opacity-50">
        <Badge tone={tone} dot>{busy ? t('ztp.registering') : t(`ztp.states.${state || 'none'}`)}</Badge>
      </button>
      {code && <span className="text-xs text-danger">{t(`errors.${code}`, { defaultValue: t('errors.internal') })}</span>}
    </span>
  )
}
