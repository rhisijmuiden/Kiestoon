import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LayoutTemplate, Pencil, Plus, Send, Trash2 } from 'lucide-react'
import { Empty, Modal, Select, Table, Toggle, td, useApi } from '../components/kit'
import { Button, Card, ErrorMessage, Input } from '../components/ui'
import { KeyLayout, fillKeys } from '../components/KeyLayout'
import { useDestinations } from '../components/pbx'
import type { LinkNotice } from '../components/PhoneLink'
import { formatMac } from '../components/PhoneLink'
import { vendorName } from './PhoneExtras'
import { api } from '../lib/api'
import type { Extension, KeyTemplate, ModuleType, Phone, PhoneModel } from '../lib/types'

/** Key layouts per phone model, applied to many phones at once. */
export function KeyTemplatesPanel({ models, moduleTypes, exts, phones, onApplied }: {
  models: PhoneModel[]; moduleTypes: ModuleType[]; exts: Extension[]; phones: Phone[]; onApplied: (n: LinkNotice) => void
}) {
  const { t } = useTranslation()
  const list = useApi<KeyTemplate[]>('/key-templates')
  const [edit, setEdit] = useState<KeyTemplate | null>(null)
  const [apply, setApply] = useState<KeyTemplate | null>(null)
  const withKeys = models.filter((m) => m.keys > 0)
  const name = (tp: { vendor: string; model: string }) => models.find((m) => m.vendor === tp.vendor && m.id === tp.model)?.name ?? tp.model
  async function remove(tp: KeyTemplate) {
    if (!window.confirm(t('keys.confirmDeleteTemplate', { name: tp.name }))) return
    await api('DELETE', `/key-templates/${tp.id}`)
    list.reload()
  }
  return (
    <Card>
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <p className="max-w-3xl text-sm text-muted">{t('keys.templatesText')}</p>
        <Button onClick={() => withKeys[0] && setEdit({ id: 0, name: '', vendor: withKeys[0].vendor, model: withKeys[0].id, keys: [], modules: [] })}>
          <Plus className="size-4" aria-hidden /> {t('keys.newTemplate')}
        </Button>
      </div>
      <ErrorMessage error={list.error} />
      <Table head={[t('keys.templateName'), t('phones.model'), t('phones.keys'), t('keys.phonesOfModel'), '']}
        empty={list.data?.length === 0 && <Empty icon={<LayoutTemplate className="size-7" />} title={t('keys.noTemplates')} text={t('keys.noTemplatesText')} />}>
        {list.data?.map((tp) => {
          const count = phones.filter((p) => p.vendor === tp.vendor && p.model === tp.model).length
          const used = tp.keys.filter((k) => k.type !== 'none').length + tp.modules.reduce((n, m) => n + m.keys.filter((k) => k.type !== 'none').length, 0)
          return (
            <tr key={tp.id}>
              <td className={`${td} text-ink`}>{tp.name}</td>
              <td className={td}>{name(tp)}{tp.modules.length > 0 && <span className="ml-1.5 text-xs text-subtle">+ {tp.modules.length}× {tp.modules[0].type}</span>}</td>
              <td className={td}>{used}</td>
              <td className={td}>{count}</td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <button type="button" disabled={count === 0} onClick={() => setApply(tp)} aria-label={t('keys.apply')} title={t('keys.apply')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-accent disabled:opacity-40"><Send className="size-4" /></button>
                <button type="button" onClick={() => setEdit(tp)} aria-label={t('common.edit')} title={t('common.edit')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-ink"><Pencil className="size-4" /></button>
                <button type="button" onClick={() => remove(tp)} aria-label={t('common.delete')} title={t('common.delete')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-danger/10 hover:text-danger"><Trash2 className="size-4" /></button>
              </td>
            </tr>
          )
        })}
      </Table>
      {edit && <TemplateForm value={edit} models={withKeys} moduleTypes={moduleTypes} exts={exts} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); list.reload() }} />}
      {apply && <ApplyForm tpl={apply} phones={phones.filter((p) => p.vendor === apply.vendor && p.model === apply.model)} exts={exts}
        onClose={() => setApply(null)} onDone={(n) => { setApply(null); onApplied(n) }} />}
    </Card>
  )
}

function TemplateForm({ value, models, moduleTypes, exts, onClose, onSaved }: {
  value: KeyTemplate; models: PhoneModel[]; moduleTypes: ModuleType[]; exts: Extension[]; onClose: () => void; onSaved: () => void
}) {
  const dests = useDestinations()
  const { t } = useTranslation()
  const [x, setX] = useState(value)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const model = models.find((m) => m.vendor === x.vendor && m.id === x.model)
  const keys = fillKeys(x.keys, model?.keys ?? 0)
  const fits = moduleTypes.filter((m) => m.vendor === x.vendor && m.models.includes(x.model))
  const vendors = [...new Set(models.map((m) => m.vendor))]
  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api(x.id ? 'PUT' : 'POST', x.id ? `/key-templates/${x.id}` : '/key-templates', { ...x, keys })
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal wide title={x.id ? t('keys.editTemplate') : t('keys.newTemplate')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Input name="name" label={t('keys.templateName')} value={x.name} onChange={(e) => setX({ ...x, name: e.target.value })} placeholder={t('keys.templatePlaceholder')} />
        <Select name="model" label={t('phones.model')} value={`${x.vendor}/${x.model}`} disabled={!!x.id} hint={x.id ? t('keys.modelFixed') : undefined}
          onChange={(e) => { const [v, m] = e.target.value.split('/'); setX({ ...x, vendor: v as KeyTemplate['vendor'], model: m, keys: [], modules: [] }) }}>
          {vendors.map((v) => (
            <optgroup key={v} label={vendorName(v)}>
              {models.filter((m) => m.vendor === v).map((m) => <option key={m.id} value={`${v}/${m.id}`}>{m.name}</option>)}
            </optgroup>
          ))}
        </Select>
      </div>
      <KeyLayout key={`${x.vendor}/${x.model}`} vendor={x.vendor} perPage={model?.perPage} keys={keys} modules={x.modules} moduleTypes={fits} lines={model?.lines ?? 1} exts={exts} dests={dests}
        onKeys={(k) => setX((o) => ({ ...o, keys: k }))} onModules={(m) => setX((o) => ({ ...o, modules: m }))} />
    </Modal>
  )
}

function ApplyForm({ tpl, phones, exts, onClose, onDone }: { tpl: KeyTemplate; phones: Phone[]; exts: Extension[]; onClose: () => void; onDone: (n: LinkNotice) => void }) {
  const { t } = useTranslation()
  const [ids, setIds] = useState<number[]>([])
  const [resync, setResync] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const extLabel = (id: number | null) => { const e = exts.find((x) => x.id === id); return e ? `${e.number} · ${e.name}` : '–' }
  async function run() {
    setBusy(true)
    setError(null)
    try {
      const r = await api<{ applied: number; notified: number }>('POST', `/key-templates/${tpl.id}/apply`, { phoneIds: ids, resync })
      onDone({ kind: 'applied', applied: r.applied, notified: r.notified, resync })
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const all = ids.length === phones.length
  return (
    <Modal title={t('keys.applyTitle', { name: tpl.name })} subtitle={t('keys.applySubtitle')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} disabled={ids.length === 0} onClick={run}><Send className="size-4" aria-hidden /> {t('keys.applyN', { count: ids.length })}</Button></>}>
      <ErrorMessage error={error} />
      <label className="flex items-center gap-2 text-sm text-ink-2">
        <input type="checkbox" checked={all} onChange={() => setIds(all ? [] : phones.map((p) => p.id))} className="size-4 accent-[var(--color-accent)]" /> {t('keys.selectAll')}
      </label>
      <ul className="grid max-h-72 gap-1.5 overflow-y-auto">
        {phones.map((p) => (
          <li key={p.id}>
            <label className="flex items-center gap-3 rounded-lg border border-line bg-inset px-3 py-2 text-sm">
              <input type="checkbox" checked={ids.includes(p.id)} onChange={() => setIds(ids.includes(p.id) ? ids.filter((i) => i !== p.id) : [...ids, p.id])} className="size-4 accent-[var(--color-accent)]" />
              <span className="flex-1 text-ink">{p.label || <span className="font-mono">{formatMac(p.mac)}</span>}</span>
              <span className="text-xs text-subtle">{extLabel(p.extensionId)}</span>
            </label>
          </li>
        ))}
      </ul>
      <Toggle label={t('phoneLink.resyncToggle')} hint={t('keys.applyHint')} checked={resync} onChange={setResync} />
    </Modal>
  )
}
