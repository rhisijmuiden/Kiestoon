import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Eye, Pencil, Plus, Smartphone, Trash2 } from 'lucide-react'
import { Badge, CopyField, Empty, Modal, Select, SectionTitle, Table, Tabs, Toggle, formatDate, td, useApi } from '../components/kit'
import { FirmwarePanel, ZtpBadge, ZtpPanel, vendorName } from './PhoneExtras'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { Extension, Phone, PhoneKey, PhoneModel, ProvSettings } from '../lib/types'

const vendors = ['yealink', 'snom', 'grandstream', 'fanvil', 'gigaset', 'poly', 'htek', 'akuvox', 'cisco'] as const
const untested = new Set<string>(['grandstream', 'fanvil', 'gigaset', 'poly', 'htek', 'akuvox', 'cisco'])

export function Phones() {
  const { t, i18n } = useTranslation()
  const list = useApi<Phone[]>('/phones')
  const models = useApi<PhoneModel[]>('/provisioning/models')
  const exts = useApi<Extension[]>('/pbx/extensions')
  const [edit, setEdit] = useState<Phone | null>(null)
  const [preview, setPreview] = useState<{ phone: Phone; body: string } | null>(null)
  const [tab, setTab] = useState<'phones' | 'firmware' | 'ztp'>('phones')
  const ztp = useApi<{ settings: { yealink: { enabled: boolean } } }>('/provisioning/ztp')
  const rps = !!ztp.data?.settings.yealink.enabled
  const extLabel = (id: number | null) => {
    const e = exts.data?.find((x) => x.id === id)
    return e ? `${e.number} · ${e.name}` : '–'
  }
  async function remove(p: Phone) {
    if (!window.confirm(t('phones.confirmDelete', { mac: p.mac }))) return
    await api('DELETE', `/phones/${p.id}`)
    list.reload()
  }
  async function showPreview(p: Phone) {
    const r = await api<{ body: string }>('GET', `/phones/${p.id}/preview`)
    setPreview({ phone: p, body: r.body })
  }
  return (
    <>
      <PageHeader title={t('phones.title')} subtitle={t('phones.subtitle')}>
        <Button onClick={() => setEdit({ id: 0, mac: '', vendor: 'yealink', model: 'T54W', extensionId: exts.data?.[0]?.id ?? null, label: '', keys: [], lines: [], lastSeenAt: '', lastIp: '', userAgent: '' })}>
          <Plus className="size-4" aria-hidden /> {t('phones.add')}
        </Button>
      </PageHeader>
      <Tabs value={tab} onChange={setTab} tabs={[{ key: 'phones', label: t('phones.tabPhones') }, { key: 'firmware', label: t('phones.tabFirmware') }, { key: 'ztp', label: t('phones.tabZtp') }]} />
      {tab === 'firmware' && models.data && <FirmwarePanel models={models.data} />}
      {tab === 'ztp' && <ZtpPanel />}
      {tab === 'phones' && <>
      <ProvisioningCard />
      <Card>
        <ErrorMessage error={list.error} />
        <Table head={[t('phones.label'), t('phones.mac'), t('phones.model'), t('phones.extension'), t('phones.lastSeen'), t('phones.ztp'), '']}
          empty={list.data?.length === 0 && <Empty icon={<Smartphone className="size-7" />} title={t('phones.emptyTitle')} text={t('phones.emptyText')} />}>
          {list.data?.map((p) => (
            <tr key={p.id}>
              <td className={`${td} text-ink`}>{p.label || '–'}</td>
              <td className={`${td} font-mono text-xs`}>{p.mac.match(/.{2}/g)?.join(':').toUpperCase()}</td>
              <td className={td}>
                {models.data?.find((m) => m.vendor === p.vendor && m.id === p.model)?.name ?? p.model}
                {p.lines?.length > 0 && <span className="ml-1.5 text-xs text-subtle">{t('phones.handsetCount', { count: p.lines.length + 1 })}</span>}
              </td>
              <td className={td}>{extLabel(p.extensionId)}</td>
              <td className={td}>{p.lastSeenAt ? <><Badge tone="ok" dot>{formatDate(p.lastSeenAt, i18n.language)}</Badge> <span className="font-mono text-xs text-subtle">{p.lastIp}</span></> : <Badge>{t('phones.never')}</Badge>}</td>
              <td className={td}>{p.vendor === 'yealink' && rps ? <ZtpBadge phoneId={p.id} state={p.ztpState} error={p.ztpError} onDone={list.reload} /> : <span className="text-xs text-subtle">–</span>}</td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <button type="button" onClick={() => showPreview(p)} aria-label={t('phones.preview')} title={t('phones.preview')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-white/5 hover:text-accent"><Eye className="size-4" /></button>
                <button type="button" onClick={() => setEdit(p)} aria-label={t('common.edit')} title={t('common.edit')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-white/5 hover:text-ink"><Pencil className="size-4" /></button>
                <button type="button" onClick={() => remove(p)} aria-label={t('common.delete')} title={t('common.delete')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-danger/10 hover:text-danger"><Trash2 className="size-4" /></button>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
      </>}
      {edit && models.data && <PhoneForm value={edit} models={models.data} exts={exts.data ?? []} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); list.reload() }} />}
      {preview && (
        <Modal wide title={t('phones.preview')} subtitle={preview.phone.mac} onClose={() => setPreview(null)}>
          <pre className="max-h-[60vh] overflow-auto rounded-xl border border-line bg-[#0A1224] p-4 font-mono text-xs text-ink-2">{preview.body}</pre>
        </Modal>
      )}
    </>
  )
}

function ProvisioningCard() {
  const { t } = useTranslation()
  const s = useApi<ProvSettings>('/provisioning/settings')
  const [open, setOpen] = useState(false)
  if (!s.data) return null
  const host = s.data.server || window.location.hostname
  const base = `http://${s.data.username}:${s.data.password}@${host}:${window.location.port || '8080'}/prov/`
  return (
    <Card>
      <SectionTitle action={<Button variant="secondary" onClick={() => setOpen(true)}>{t('phones.settings')}</Button>}>{t('phones.provisioning')}</SectionTitle>
      <p className="mb-4 max-w-3xl text-sm text-muted">{t('phones.provText')}</p>
      <div className="grid gap-3 lg:grid-cols-2">
        <CopyField label={t('phones.option66')} value={base} secret />
        <CopyField label={t('phones.option66Snom')} value={`${base}snom{model}.htm`} secret />
        <CopyField label={t('phones.gigasetUrl')} value={`${base}gigaset/`} secret />
      </div>
      <p className="mt-3 text-xs text-subtle">{t('phones.vendorsHint')}</p>
      {open && <ProvSettingsForm value={s.data} onClose={() => setOpen(false)} onSaved={() => { setOpen(false); s.reload() }} />}
    </Card>
  )
}

function ProvSettingsForm({ value, onClose, onSaved }: { value: ProvSettings; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [x, setX] = useState(value)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  async function save() {
    setBusy(true)
    try {
      await api('PUT', '/provisioning/settings', x)
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={t('phones.settings')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <Toggle label={t('phones.enabled')} checked={x.enabled} onChange={(v) => setX({ ...x, enabled: v })} />
      <Input label={t('phones.server')} value={x.server} onChange={(e) => setX({ ...x, server: e.target.value.trim() })} hint={t('phones.serverHint')} className="font-mono" />
      <div className="grid gap-4 sm:grid-cols-2">
        <Input label={t('phones.provUser')} value={x.username} onChange={(e) => setX({ ...x, username: e.target.value.trim() })} className="font-mono" />
        <Input label={t('phones.provPassword')} value={x.password} onChange={(e) => setX({ ...x, password: e.target.value.trim() })} className="font-mono" />
      </div>
      <Input label={t('phones.adminPassword')} value={x.adminPassword} onChange={(e) => setX({ ...x, adminPassword: e.target.value })} hint={t('phones.adminPasswordHint')} className="font-mono" />
      <div className="grid gap-4 sm:grid-cols-2">
        <Input label={t('phones.ntp')} value={x.ntpServer} onChange={(e) => setX({ ...x, ntpServer: e.target.value.trim() })} className="font-mono" />
        <Select label={t('common.language')} value={x.language} onChange={(e) => setX({ ...x, language: e.target.value as 'en' | 'nl' })}>
          <option value="en">English</option><option value="nl">Nederlands</option>
        </Select>
      </div>
    </Modal>
  )
}

function PhoneForm({ value, models, exts, onClose, onSaved }: { value: Phone; models: PhoneModel[]; exts: Extension[]; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [x, setX] = useState(value)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const model = models.find((m) => m.vendor === x.vendor && m.id === x.model)
  const keyCount = model?.keys ?? 0
  const maxLines = model?.lines ?? 0
  const lines = (x.lines ?? []).slice(0, Math.max(0, maxLines - 1))
  const keys: PhoneKey[] = Array.from({ length: keyCount }, (_, i) => x.keys[i] ?? { type: i === 0 ? 'line' : 'none', value: '', label: '' })
  const setKey = (i: number, k: Partial<PhoneKey>) => setX((o) => ({ ...o, keys: keys.map((old, j) => (j === i ? { ...old, ...k } : old)) }))
  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api(x.id ? 'PUT' : 'POST', x.id ? `/phones/${x.id}` : '/phones', { ...x, keys, lines: lines.filter(Boolean) })
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal wide title={x.id ? t('phones.edit') : t('phones.add')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Input label={t('phones.mac')} value={x.mac} onChange={(e) => setX({ ...x, mac: e.target.value })} placeholder="00:15:65:AB:CD:EF" className="font-mono" />
        <Input label={t('phones.label')} value={x.label} onChange={(e) => setX({ ...x, label: e.target.value })} placeholder={t('phones.labelPlaceholder')} />
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <Select label={t('phones.model')} value={`${x.vendor}/${x.model}`} onChange={(e) => { const [v, m] = e.target.value.split('/'); setX({ ...x, vendor: v as Phone['vendor'], model: m }) }}>
          {vendors.map((v) => (
            <optgroup key={v} label={vendorName(v)}>
              {models.filter((m) => m.vendor === v).map((m) => <option key={m.id} value={`${v}/${m.id}`}>{m.name}</option>)}
            </optgroup>
          ))}
        </Select>
        <Select label={t('phones.extension')} value={x.extensionId ?? ''} onChange={(e) => setX({ ...x, extensionId: e.target.value ? Number(e.target.value) : null })}>
          <option value="">–</option>
          {exts.map((e) => <option key={e.id} value={e.id}>{e.number} · {e.name}</option>)}
        </Select>
      </div>
      {untested.has(x.vendor) && <p className="text-xs text-warn">{t('phones.untested')}</p>}
      {maxLines > 1 && (
        <div>
          <h3 className="mb-1 text-sm font-medium text-ink-2">{t('phones.handsets')}</h3>
          <p className="mb-3 text-xs text-subtle">{t('phones.handsetsHint', { max: maxLines })}</p>
          <div className="grid gap-2 sm:grid-cols-2">
            {[...lines, 0].slice(0, maxLines - 1).map((id, i) => (
              <Select key={i} label={t('phones.handset', { n: i + 2 })} value={id || ''} onChange={(e) => {
                const next = [...lines]
                next[i] = Number(e.target.value)
                setX({ ...x, lines: next.filter(Boolean) })
              }}>
                <option value="">–</option>
                {exts.filter((e) => e.id === id || (e.id !== x.extensionId && !lines.includes(e.id))).map((e) => <option key={e.id} value={e.id}>{e.number} · {e.name}</option>)}
              </Select>
            ))}
          </div>
        </div>
      )}
      <div>
        <h3 className="mb-1 text-sm font-medium text-ink-2">{t('phones.keys')}</h3>
        {keyCount === 0 ? (
          <p className="text-sm text-subtle">{t('phones.noKeys')}</p>
        ) : (
          <>
            <p className="mb-3 text-xs text-subtle">{t('phones.keysHint')}</p>
            <KeyEditor keys={keys} setKey={setKey} exts={exts} />
          </>
        )}
      </div>
    </Modal>
  )
}

/** The programmable keys of a phone. Shared by the admin page and the user portal. */
export function KeyEditor({ keys, setKey, exts }: { keys: PhoneKey[]; setKey: (i: number, k: Partial<PhoneKey>) => void; exts: { number: string; name: string }[] }) {
  const { t } = useTranslation()
  return (
    <div className="grid gap-2 rounded-2xl border border-line bg-[#070C18] p-3 sm:grid-cols-2">
      {keys.map((k, i) => (
        <div key={i} className={`grid grid-cols-[28px_96px_1fr_1fr] items-center gap-2 rounded-xl border px-2 py-1.5 ${k.type === 'blf' ? 'border-ok/30 bg-ok/5' : k.type === 'line' ? 'border-accent/30 bg-accent/5' : k.type === 'speed' ? 'border-accent-2/30 bg-accent-2/5' : 'border-line'}`}>
          <span className="text-center font-mono text-xs text-subtle">{i + 1}</span>
          <select aria-label={t('phones.keyType')} value={k.type} onChange={(e) => setKey(i, { type: e.target.value as PhoneKey['type'], value: e.target.value === 'line' || e.target.value === 'none' ? '' : k.value })} className="min-h-9 rounded-lg border border-line bg-[#0A1224] px-2 text-xs">
            {(['none', 'line', 'blf', 'speed'] as const).map((ty) => <option key={ty} value={ty}>{t(`phones.keyTypes.${ty}`)}</option>)}
          </select>
          {k.type === 'blf' ? (
            <select aria-label={t('phones.keyValue')} value={k.value} onChange={(e) => { const ext = exts.find((q) => q.number === e.target.value); setKey(i, { value: e.target.value, label: k.label || ext?.name || '' }) }} className="min-h-9 rounded-lg border border-line bg-[#0A1224] px-2 text-xs">
              <option value="">–</option>
              {exts.map((e) => <option key={e.number} value={e.number}>{e.number} {e.name}</option>)}
            </select>
          ) : (
            <input aria-label={t('phones.keyValue')} disabled={k.type !== 'speed'} value={k.value} onChange={(e) => setKey(i, { value: e.target.value })} className="min-h-9 rounded-lg border border-line bg-[#0A1224] px-2 font-mono text-xs disabled:opacity-40" />
          )}
          <input aria-label={t('phones.keyLabel')} disabled={k.type === 'none'} value={k.label} placeholder={t('phones.keyLabel')} onChange={(e) => setKey(i, { label: e.target.value })} className="min-h-9 rounded-lg border border-line bg-[#0A1224] px-2 text-xs disabled:opacity-40" />
        </div>
      ))}
    </div>
  )
}
