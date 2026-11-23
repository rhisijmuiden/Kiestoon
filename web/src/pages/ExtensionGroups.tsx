import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Lock, Pencil, Plus, RefreshCw, Trash2, UsersRound } from 'lucide-react'
import { Badge, Empty, Modal, Table, td, useApi } from '../components/kit'
import { Alert, Button, Card, ErrorMessage, Input } from '../components/ui'
import { IconButton, NumberOptions, Segmented } from '../components/pbx'
import { api } from '../lib/api'
import { formatNumber } from '../lib/numbers'
import type { Extension, ExtensionGroup, GroupSettings, KeyTemplate, OutboundRoute } from '../lib/types'

const blank: ExtensionGroup = { id: 0, name: '', callerId: '', settings: {}, keyTemplateIds: [], members: [] }

/** The settings a group can enforce, in the order the form shows them. */
export const groupSettingKeys = ['voicemail', 'voicemailEmail', 'voicemailDelete', 'ringSeconds', 'maxContacts', 'outboundClass', 'allowPremium', 'recordMode', 'pickupGroup', 'webrtc'] as const
type SettingKey = (typeof groupSettingKeys)[number]

/** Default value when a setting is first enforced. */
const defaults: Required<GroupSettings> = {
  voicemail: true, voicemailEmail: true, voicemailDelete: false, ringSeconds: 25, maxContacts: 3, outboundClass: 'national',
  allowPremium: false, recordMode: 'ondemand', pickupGroup: '1', webrtc: true,
}

/** Label of an extension setting, the same text the extension form uses. */
export function settingLabel(t: (k: string) => string, k: SettingKey): string {
  const map: Record<SettingKey, string> = {
    voicemail: 'extensions.voicemail', voicemailEmail: 'extensions.vmToEmail', voicemailDelete: 'extensions.vmDelete',
    ringSeconds: 'extensions.ringSeconds', maxContacts: 'extensions.maxContacts', outboundClass: 'extensions.outboundClass',
    allowPremium: 'extensions.allowPremium', recordMode: 'extensions.recordMode', pickupGroup: 'extensions.pickupGroup', webrtc: 'extensions.webrtc',
  }
  return t(map[k])
}

/** Short text of an enforced value, for the list and the lock hints. */
export function settingValue(t: (k: string) => string, k: SettingKey, v: unknown): string {
  if (typeof v === 'boolean') return v ? t('extGroups.on') : t('extGroups.off')
  if (k === 'outboundClass') return t(`extensions.classes.${v}`)
  if (k === 'recordMode') return t(`extensions.recordModes.${v === 'off' ? 'never' : v}`)
  if (k === 'ringSeconds') return `${v} s`
  if (k === 'pickupGroup') return v ? String(v) : '–'
  return String(v)
}

export function ExtensionGroupList() {
  const { t } = useTranslation()
  const list = useApi<ExtensionGroup[]>('/pbx/extension-groups')
  const exts = useApi<Extension[]>('/pbx/extensions')
  const templates = useApi<KeyTemplate[]>('/key-templates')
  const routes = useApi<OutboundRoute[]>('/pbx/outbound')
  const [edit, setEdit] = useState<ExtensionGroup | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [notice, setNotice] = useState<string>('')
  const extName = (id: number) => exts.data?.find((e) => e.id === id)?.number ?? `#${id}`

  async function remove(g: ExtensionGroup) {
    if (!window.confirm(t('extGroups.confirmDelete', { name: g.name }))) return
    setError(null)
    try {
      await api('DELETE', `/pbx/extension-groups/${g.id}`)
      list.reload()
    } catch (e) {
      setError(e)
    }
  }
  async function sync(g: ExtensionGroup) {
    setError(null)
    try {
      const r = await api<{ phonesUpdated: number; phonesNotified: number }>('POST', `/pbx/extension-groups/${g.id}/sync-phones`, {})
      setNotice(t('extGroups.synced', { count: r.phonesUpdated, notified: r.phonesNotified }))
    } catch (e) {
      setError(e)
    }
  }
  return (
    <Card>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <p className="max-w-2xl text-sm text-muted">{t('extGroups.text')}</p>
        <Button onClick={() => setEdit(blank)}><Plus className="size-4" aria-hidden /> {t('extGroups.add')}</Button>
      </div>
      <Precedence />
      <ErrorMessage error={list.error || error} />
      {notice && <Alert tone="ok">{notice}</Alert>}
      <Table head={[t('extGroups.name'), t('extGroups.members'), t('extGroups.callerId'), t('extGroups.enforced'), t('extGroups.routes'), '']}
        empty={list.data?.length === 0 && <Empty icon={<UsersRound className="size-7" />} title={t('extGroups.emptyTitle')} text={t('extGroups.emptyText')} />}>
        {list.data?.map((g) => {
          const enforced = groupSettingKeys.filter((k) => g.settings[k] !== undefined)
          const usedBy = (routes.data ?? []).filter((r) => r.groupIds?.includes(g.id))
          return (
            <tr key={g.id}>
              <td className={`${td} text-ink`}>{g.name}</td>
              <td className={`${td} font-mono text-xs`}>{g.members.map(extName).join(', ') || '–'}</td>
              <td className={`${td} font-mono`}>{g.callerId ? formatNumber(g.callerId) : <span className="text-subtle">{t('extGroups.routeDefault')}</span>}</td>
              <td className={td}>
                <div className="flex max-w-xs flex-wrap gap-1">
                  {enforced.length === 0 && g.keyTemplateIds.length === 0 && <span className="text-subtle">–</span>}
                  {enforced.map((k) => <Badge key={k} tone="accent">{settingLabel(t, k)}: {settingValue(t, k, g.settings[k])}</Badge>)}
                  {g.keyTemplateIds.map((id) => <Badge key={`kt${id}`}>{t('extGroups.templateBadge', { name: templates.data?.find((x) => x.id === id)?.name ?? `#${id}` })}</Badge>)}
                </div>
              </td>
              <td className={td}>{usedBy.length ? usedBy.map((r) => r.name).join(', ') : <span className="text-subtle">–</span>}</td>
              <td className={`${td} whitespace-nowrap text-right`}>
                {g.keyTemplateIds.length > 0 && <IconButton label={t('extGroups.syncPhones')} onClick={() => sync(g)}><RefreshCw className="size-4" /></IconButton>}
                <IconButton label={t('common.edit')} onClick={() => setEdit(g)}><Pencil className="size-4" /></IconButton>
                <IconButton label={t('common.delete')} danger onClick={() => remove(g)}><Trash2 className="size-4" /></IconButton>
              </td>
            </tr>
          )
        })}
      </Table>
      {edit && <GroupForm value={edit} exts={exts.data ?? []} groups={list.data ?? []} templates={templates.data ?? []}
        onClose={() => setEdit(null)} onSaved={(n) => { setEdit(null); setNotice(n); list.reload(); exts.reload() }} />}
    </Card>
  )
}

/** Explains which caller ID wins: extension, group, route, trunk. */
export function Precedence() {
  const { t } = useTranslation()
  const steps = [t('extGroups.prec.extension'), t('extGroups.prec.group'), t('extGroups.prec.route'), t('extGroups.prec.trunk')]
  return (
    <div className="mb-4 rounded-xl border border-line bg-surface-2/60 p-3 text-sm">
      <div className="mb-2 font-medium text-ink-2">{t('extGroups.prec.title')}</div>
      <ol className="flex flex-wrap items-center gap-2">
        {steps.map((s, i) => (
          <li key={s} className="flex items-center gap-2">
            <span className="inline-flex items-center gap-2 rounded-lg border border-line bg-surface px-2.5 py-1">
              <span className="font-mono text-xs text-accent">{i + 1}</span>{s}
            </span>
            {i < steps.length - 1 && <span className="text-subtle" aria-hidden>›</span>}
          </li>
        ))}
      </ol>
    </div>
  )
}

function GroupForm({ value, exts, groups, templates, onClose, onSaved }: {
  value: ExtensionGroup; exts: Extension[]; groups: ExtensionGroup[]; templates: KeyTemplate[]; onClose: () => void; onSaved: (notice: string) => void
}) {
  const { t } = useTranslation()
  const [g, setG] = useState<ExtensionGroup>(value)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const otherGroup = (extId: number) => groups.find((x) => x.id !== g.id && x.members.includes(extId))
  const toggleMember = (id: number) => setG((o) => ({ ...o, members: o.members.includes(id) ? o.members.filter((m) => m !== id) : [...o.members, id] }))
  const setSetting = <K extends SettingKey>(k: K, v: GroupSettings[K] | undefined) =>
    setG((o) => {
      const s = { ...o.settings }
      if (v === undefined) delete s[k]
      else s[k] = v
      return { ...o, settings: s }
    })
  const toggleTemplate = (kt: KeyTemplate) => setG((o) => {
    if (o.keyTemplateIds.includes(kt.id)) return { ...o, keyTemplateIds: o.keyTemplateIds.filter((x) => x !== kt.id) }
    // One template per phone model: picking another one for the same model replaces it.
    const sameModel = templates.filter((x) => x.vendor === kt.vendor && x.model === kt.model).map((x) => x.id)
    return { ...o, keyTemplateIds: [...o.keyTemplateIds.filter((x) => !sameModel.includes(x)), kt.id] }
  })

  async function save() {
    setBusy(true)
    setError(null)
    try {
      const r = await api<ExtensionGroup & { phonesUpdated: number; phonesNotified: number }>(g.id ? 'PUT' : 'POST', g.id ? `/pbx/extension-groups/${g.id}` : '/pbx/extension-groups', g)
      onSaved(r.phonesUpdated > 0 ? t('extGroups.savedPhones', { count: r.phonesUpdated, notified: r.phonesNotified }) : t('extGroups.saved', { name: r.name }))
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }

  // Compact controls without their own label: the row already names the setting.
  const field = 'min-h-10 w-full max-w-xs rounded-[10px] border border-line bg-field px-3 text-[15px] text-ink outline-none focus:border-accent/60 focus:ring-2 focus:ring-accent/20'
  const control = (k: SettingKey): ReactNode => {
    const v = g.settings[k]
    const label = settingLabel(t, k)
    if (typeof defaults[k] === 'boolean') {
      return <Segmented label={undefined} value={v ? 'on' : 'off'} onChange={(x) => setSetting(k, (x === 'on') as never)}
        options={[{ key: 'on', label: t('extGroups.on'), tone: 'ok' }, { key: 'off', label: t('extGroups.off'), tone: 'danger' }]} />
    }
    if (k === 'outboundClass') {
      return (
        <select aria-label={label} className={field} value={String(v)} onChange={(e) => setSetting(k, e.target.value as Extension['outboundClass'])}>
          {(['internal', 'national', 'international'] as const).map((c) => <option key={c} value={c}>{t(`extensions.classes.${c}`)}</option>)}
        </select>
      )
    }
    if (k === 'recordMode') {
      return (
        <select aria-label={label} className={field} value={String(v)} onChange={(e) => setSetting(k, e.target.value as Extension['recordMode'])}>
          {(['off', 'ondemand', 'always'] as const).map((c) => <option key={c} value={c}>{t(`extensions.recordModes.${c === 'off' ? 'never' : c}`)}</option>)}
        </select>
      )
    }
    if (k === 'pickupGroup') {
      return <input aria-label={label} className={`${field} font-mono`} value={String(v ?? '')} onChange={(e) => setSetting(k, e.target.value.replace(/\D/g, '').slice(0, 2))} />
    }
    return <input aria-label={label} className={field} type="number" min={k === 'ringSeconds' ? 5 : 1} max={k === 'ringSeconds' ? 300 : 10} value={Number(v)} onChange={(e) => setSetting(k, Number(e.target.value) as never)} />
  }

  return (
    <Modal wide title={g.id ? t('extGroups.edit', { name: value.name }) : t('extGroups.add')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Input name="name" label={t('extGroups.name')} value={g.name} onChange={(e) => setG({ ...g, name: e.target.value })} autoFocus />
        <Input name="callerId" label={t('extGroups.callerId')} value={g.callerId} onChange={(e) => setG({ ...g, callerId: e.target.value.trim() })}
          hint={t('extGroups.callerIdHint')} className="font-mono" list="group-numbers" />
        <NumberOptions id="group-numbers" />
      </div>

      <fieldset className="flex flex-col gap-2">
        <legend className="mb-1 text-sm font-medium text-ink-2">{t('extGroups.members')}</legend>
        <p className="text-xs text-subtle">{t('extGroups.membersHint')}</p>
        <div className="flex flex-wrap gap-2">
          {exts.map((e) => {
            const on = g.members.includes(e.id)
            const other = otherGroup(e.id)
            return (
              <button key={e.id} type="button" onClick={() => toggleMember(e.id)} aria-pressed={on}
                title={other && !on ? t('extGroups.movesFrom', { name: other.name }) : undefined}
                className={`min-h-9 rounded-lg border px-3 text-sm transition ${on ? 'border-accent/60 bg-accent/15 text-ink' : 'border-line text-muted hover:text-ink'}`}>
                <span className="font-mono">{e.number}</span> {e.name}
                {other && !on && <span className="ml-1.5 text-xs text-subtle">({other.name})</span>}
              </button>
            )
          })}
        </div>
      </fieldset>

      <fieldset className="flex flex-col gap-2">
        <legend className="mb-1 text-sm font-medium text-ink-2">{t('extGroups.settings')}</legend>
        <p className="text-xs text-subtle">{t('extGroups.settingsHint')}</p>
        <div className="grid gap-2 sm:grid-cols-2">
          {groupSettingKeys.map((k) => {
            const on = g.settings[k] !== undefined
            return (
              <div key={k} className={`flex flex-col gap-1.5 rounded-xl border px-3 py-2 ${on ? 'border-accent/40 bg-accent/5' : 'border-line'}`}>
                <label className="flex min-h-9 cursor-pointer items-center gap-2 text-sm">
                  <input type="checkbox" className="size-4 accent-[rgb(34_211_238)]" checked={on} onChange={(e) => setSetting(k, e.target.checked ? defaults[k] as never : undefined)} />
                  {on ? <Lock className="size-3.5 text-accent" aria-hidden /> : null}
                  <span className={on ? 'text-ink' : 'text-muted'}>{settingLabel(t, k)}</span>
                </label>
                {on ? control(k) : <span className="pb-1 text-xs text-subtle">{t('extGroups.perExtension')}</span>}
              </div>
            )
          })}
        </div>
      </fieldset>

      <fieldset className="flex flex-col gap-2">
        <legend className="mb-1 text-sm font-medium text-ink-2">{t('extGroups.templates')}</legend>
        <p className="text-xs text-subtle">{t('extGroups.templatesHint')}</p>
        {templates.length === 0 ? <p className="text-sm text-subtle">{t('extGroups.noTemplates')}</p> : (
          <div className="flex flex-wrap gap-2">
            {templates.map((kt) => {
              const on = g.keyTemplateIds.includes(kt.id)
              return (
                <button key={kt.id} type="button" onClick={() => toggleTemplate(kt)} aria-pressed={on}
                  className={`min-h-9 rounded-lg border px-3 text-sm transition ${on ? 'border-accent/60 bg-accent/15 text-ink' : 'border-line text-muted hover:text-ink'}`}>
                  {kt.name} <span className="text-xs text-subtle">· {kt.vendor} {kt.model}</span>
                </button>
              )
            })}
          </div>
        )}
      </fieldset>
    </Modal>
  )
}
