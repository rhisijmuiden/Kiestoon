import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Eye, Link2, Pencil, Plus, Save, Smartphone, Trash2 } from 'lucide-react'
import { Badge, CopyField, Empty, Modal, Select, SectionTitle, Table, Tabs, Toggle, formatDate, td, useApi } from '../components/kit'
import { AssignForm, LinkNoticeAlert, ResyncButton, WaitingPhones, formatMac, type LinkNotice } from '../components/PhoneLink'
import { KeyLayout, fillKeys } from '../components/KeyLayout'
import { KeyTemplatesPanel } from './KeyTemplates'
import { FirmwarePanel, ZtpBadge, ZtpPanel, vendorName } from './PhoneExtras'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { DiscoveredPhone, Extension, KeyTemplate, ModuleType, Phone, PhoneModel, ProvSettings, ResyncState } from '../lib/types'

const vendors = ['yealink', 'snom', 'grandstream', 'fanvil', 'gigaset', 'poly', 'htek', 'akuvox', 'cisco'] as const
const untested = new Set<string>(['grandstream', 'fanvil', 'gigaset', 'poly', 'htek', 'akuvox', 'cisco'])

export function Phones() {
  const { t, i18n } = useTranslation()
  const list = useApi<Phone[]>('/phones')
  const models = useApi<PhoneModel[]>('/provisioning/models')
  const moduleTypes = useApi<ModuleType[]>('/provisioning/modules')
  const exts = useApi<Extension[]>('/pbx/extensions')
  const discovered = useApi<DiscoveredPhone[]>('/phones/discovered', 15000)
  const [assign, setAssign] = useState<Phone | null>(null)
  const [notice, setNotice] = useState<LinkNotice | null>(null)
  const changed = (n?: LinkNotice) => { if (n) setNotice(n); list.reload(); discovered.reload() }
  const [edit, setEdit] = useState<Phone | null>(null)
  const [preview, setPreview] = useState<{ phone: Phone; body: string } | null>(null)
  const [tab, setTab] = useState<'phones' | 'templates' | 'firmware' | 'ztp'>('phones')
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
      <Tabs value={tab} onChange={setTab} tabs={[{ key: 'phones', label: t('phones.tabPhones') }, { key: 'templates', label: t('keys.tabTemplates') }, { key: 'firmware', label: t('phones.tabFirmware') }, { key: 'ztp', label: t('phones.tabZtp') }]} />
      {tab === 'firmware' && models.data && <FirmwarePanel models={models.data} />}
      {tab === 'ztp' && <ZtpPanel />}
      {tab === 'templates' && models.data && moduleTypes.data && <KeyTemplatesPanel models={models.data} moduleTypes={moduleTypes.data} exts={exts.data ?? []} phones={list.data ?? []} onApplied={(n) => { setNotice(n); list.reload() }} />}
      {tab === 'phones' && <>
      {notice && <LinkNoticeAlert notice={notice} onClose={() => setNotice(null)} />}
      {models.data && exts.data && (
        <WaitingPhones discovered={discovered.data ?? []} unassigned={(list.data ?? []).filter((p) => p.extensionId === null && !p.lines?.length)}
          models={models.data} exts={exts.data} onChanged={changed} />
      )}
      <ProvisioningCard />
      <Card>
        <ErrorMessage error={list.error} />
        <Table head={[t('phones.label'), t('phones.mac'), t('phones.model'), t('phones.extension'), t('phones.lastSeen'), t('phones.ztp'), '']}
          empty={list.data?.length === 0 && <Empty icon={<Smartphone className="size-7" />} title={t('phones.emptyTitle')} text={t('phones.emptyText')} />}>
          {list.data?.map((p) => (
            <tr key={p.id}>
              <td className={`${td} text-ink`}>{p.label || '–'}</td>
              <td className={`${td} font-mono text-xs`}>{formatMac(p.mac)}</td>
              <td className={td}>
                {models.data?.find((m) => m.vendor === p.vendor && m.id === p.model)?.name ?? p.model}
                {p.lines?.length > 0 && <span className="ml-1.5 text-xs text-subtle">{p.vendor === 'gigaset' ? t('phones.handsetCount', { count: p.lines.length + 1 }) : t('keys.lineCount', { count: p.lines.length + 1 })}</span>}
                {(p.modules?.length ?? 0) > 0 && <span className="ml-1.5 text-xs text-subtle">+ {p.modules!.length}× {p.modules![0].type}</span>}
              </td>
              <td className={td}>{extLabel(p.extensionId)}</td>
              <td className={td}>{p.lastSeenAt ? <><Badge tone="ok" dot>{formatDate(p.lastSeenAt, i18n.language)}</Badge> <span className="font-mono text-xs text-subtle">{p.lastIp}</span></> : <Badge>{t('phones.never')}</Badge>}</td>
              <td className={td}>{p.vendor === 'yealink' && rps ? <ZtpBadge phoneId={p.id} state={p.ztpState} error={p.ztpError} onDone={list.reload} /> : <span className="text-xs text-subtle">–</span>}</td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <button type="button" onClick={() => setAssign(p)} aria-label={t('phoneLink.assignTitle')} title={t('phoneLink.assignTitle')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-accent"><Link2 className="size-4" /></button>
                <ResyncButton phone={p} onDone={setNotice} />
                <button type="button" onClick={() => showPreview(p)} aria-label={t('phones.preview')} title={t('phones.preview')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-accent"><Eye className="size-4" /></button>
                <button type="button" onClick={() => setEdit(p)} aria-label={t('common.edit')} title={t('common.edit')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-ink"><Pencil className="size-4" /></button>
                <button type="button" onClick={() => remove(p)} aria-label={t('common.delete')} title={t('common.delete')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-danger/10 hover:text-danger"><Trash2 className="size-4" /></button>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
      </>}
      {assign && <AssignForm phone={assign} exts={exts.data ?? []} onClose={() => setAssign(null)} onDone={(n) => { setAssign(null); changed(n) }} />}
      {edit && models.data && <PhoneForm value={edit} models={models.data} moduleTypes={moduleTypes.data ?? []} exts={exts.data ?? []} onClose={() => setEdit(null)}
        onSaved={(resync) => { const wasNew = !edit.id; setEdit(null); changed(wasNew || !resync ? undefined : { kind: 'resync', resync }) }} />}
      {preview && (
        <Modal wide title={t('phones.preview')} subtitle={preview.phone.mac} onClose={() => setPreview(null)}>
          <pre className="max-h-[60vh] overflow-auto rounded-xl border border-line bg-field p-4 font-mono text-xs text-ink-2">{preview.body}</pre>
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

// Intercom options each brand's provisioning file can set (auto-answer itself works on all of them).
// Gigaset DECT handsets have no intercom auto-answer, so the section is hidden there.
const icSupport: Record<string, ('tone' | 'mute' | 'barge')[]> = {
  yealink: ['tone', 'mute', 'barge'], snom: ['tone', 'mute', 'barge'], fanvil: ['tone', 'mute', 'barge'],
  poly: ['mute'], grandstream: [], htek: [], cisco: [], akuvox: [],
}

function PhoneForm({ value, models, moduleTypes, exts, onClose, onSaved }: { value: Phone; models: PhoneModel[]; moduleTypes: ModuleType[]; exts: Extension[]; onClose: () => void; onSaved: (resync?: ResyncState) => void }) {
  const { t } = useTranslation()
  const [x, setX] = useState<Phone>({ ...value, modules: value.modules ?? [], options: value.options ?? { intercom: { allow: true, mute: false, tone: true, barge: false } } })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const templates = useApi<KeyTemplate[]>('/key-templates')
  const [tplName, setTplName] = useState<string | null>(null)
  const [tplMsg, setTplMsg] = useState('')
  const model = models.find((m) => m.vendor === x.vendor && m.id === x.model)
  const keyCount = model?.keys ?? 0
  const maxLines = model?.lines ?? 0
  const lines = (x.lines ?? []).slice(0, Math.max(0, maxLines - 1))
  const keys = fillKeys(x.keys, keyCount)
  const fits = moduleTypes.filter((m) => m.vendor === x.vendor && m.models.includes(x.model))
  const modules = (x.modules ?? []).filter((m) => fits.some((f) => f.id === m.type))
  const ic = x.options!.intercom
  const setIc = (p: Partial<typeof ic>) => setX({ ...x, options: { intercom: { ...ic, ...p } } })
  const dect = x.vendor === 'gigaset'
  const accountExts = [x.extensionId, ...lines].filter(Boolean) as number[]
  const forModel = (templates.data ?? []).filter((tp) => tp.vendor === x.vendor && tp.model === x.model)
  async function save() {
    setBusy(true)
    setError(null)
    try {
      const r = await api<{ resync?: ResyncState }>(x.id ? 'PUT' : 'POST', x.id ? `/phones/${x.id}` : '/phones', { ...x, keys, modules, lines: lines.filter(Boolean) })
      onSaved(r.resync)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  function loadTemplate(id: string) {
    const tp = forModel.find((q) => String(q.id) === id)
    if (!tp) return
    setX({ ...x, keys: tp.keys, modules: tp.modules })
    setTplMsg(t('keys.templateLoaded', { name: tp.name }))
  }
  async function saveTemplate() {
    if (!tplName?.trim()) return
    try {
      await api('POST', '/key-templates', { name: tplName.trim(), vendor: x.vendor, model: x.model, keys, modules })
      setTplMsg(t('keys.templateSaved', { name: tplName.trim() }))
      setTplName(null)
      templates.reload()
    } catch (e) {
      setError(e)
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
          <h3 className="mb-1 text-sm font-medium text-ink-2">{dect ? t('phones.handsets') : t('keys.accounts')}</h3>
          <p className="mb-3 text-xs text-subtle">{dect ? t('phones.handsetsHint', { max: maxLines }) : t('keys.accountsHint', { max: maxLines })}</p>
          <div className="grid gap-2 sm:grid-cols-2">
            {[...lines, 0].slice(0, maxLines - 1).map((id, i) => (
              <Select key={i} label={dect ? t('phones.handset', { n: i + 2 }) : t('keys.lineN', { n: i + 2 })} value={id || ''} onChange={(e) => {
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
        <div className="mb-2 flex flex-wrap items-end justify-between gap-2">
          <div>
            <h3 className="text-sm font-medium text-ink-2">{t('phones.keys')}</h3>
            <p className="text-xs text-subtle">{t('phones.keysHint')}</p>
          </div>
          {(keyCount > 0 || fits.length > 0) && (
            <div className="flex flex-wrap items-center gap-2">
              {forModel.length > 0 && (
                <select aria-label={t('keys.loadTemplate')} value="" onChange={(e) => loadTemplate(e.target.value)} className="min-h-10 rounded-lg border border-line bg-field px-2.5 text-sm">
                  <option value="">{t('keys.loadTemplate')}</option>
                  {forModel.map((tp) => <option key={tp.id} value={tp.id}>{tp.name}</option>)}
                </select>
              )}
              {tplName === null ? (
                <Button variant="ghost" className="min-h-10 px-3" onClick={() => setTplName('')}><Save className="size-4" aria-hidden /> {t('keys.saveTemplate')}</Button>
              ) : (
                <span className="flex items-center gap-2">
                  <input autoFocus aria-label={t('keys.templateName')} placeholder={t('keys.templateName')} value={tplName} onChange={(e) => setTplName(e.target.value)} onKeyDown={(e) => { if (e.key === 'Enter') { e.preventDefault(); void saveTemplate() } }} className="min-h-10 w-44 rounded-lg border border-line bg-field px-2.5 text-sm" />
                  <Button className="min-h-10 px-3" onClick={saveTemplate} disabled={!tplName.trim()}>{t('common.save')}</Button>
                </span>
              )}
            </div>
          )}
        </div>
        {tplMsg && <p className="mb-2 text-xs text-ok-ink">{tplMsg}</p>}
        {keyCount === 0 && fits.length === 0 ? (
          <p className="text-sm text-subtle">{t('phones.noKeys')}</p>
        ) : (
          <KeyLayout key={`${x.vendor}/${x.model}`} vendor={x.vendor} perPage={model?.perPage} keys={keys} modules={modules} moduleTypes={fits} lines={Math.max(1, accountExts.length)} exts={exts}
            onKeys={(k) => setX((o) => ({ ...o, keys: k }))} onModules={(m) => setX((o) => ({ ...o, modules: m }))} />
        )}
      </div>
      {icSupport[x.vendor] && (
        <div className="flex flex-col gap-3 border-t border-line pt-4">
          <div>
            <h3 className="text-sm font-medium text-ink-2">{t('keys.intercom')}</h3>
            <p className="text-xs text-subtle">{t('keys.intercomHint')}</p>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <Toggle label={t('keys.icAllow')} checked={ic.allow} onChange={(v) => setIc({ allow: v })} />
            {icSupport[x.vendor].includes('tone') && <Toggle label={t('keys.icTone')} checked={ic.tone} disabled={!ic.allow} onChange={(v) => setIc({ tone: v })} />}
            {icSupport[x.vendor].includes('mute') && <Toggle label={t('keys.icMute')} checked={ic.mute} disabled={!ic.allow} onChange={(v) => setIc({ mute: v })} />}
            {icSupport[x.vendor].includes('barge') && <Toggle label={t('keys.icBarge')} hint={t('keys.icBargeHint')} checked={ic.barge} disabled={!ic.allow} onChange={(v) => setIc({ barge: v })} />}
          </div>
          {icSupport[x.vendor].length === 0 && <p className="text-xs text-subtle">{t('keys.icOnlyAllow')}</p>}
          {x.vendor === 'akuvox' && <p className="text-xs text-subtle">{t('keys.icAkuvox')}</p>}
        </div>
      )}
    </Modal>
  )
}
