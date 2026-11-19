import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Pencil, Plus, Sun, Trash2, X } from 'lucide-react'
import { Badge, Empty, Modal, Select, Table, Toggle, td, useApi } from '../components/kit'
import { DestinationLabel, DestinationPicker, IconButton, resetDestinations, useSaver } from '../components/pbx'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { SunRoute, SunRule } from '../lib/types'
import { dayList } from './TimeConditions'

const DAYS = [1, 2, 3, 4, 5, 6, 7]

const zones: string[] = (() => {
  try {
    return (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf?.('timeZone') ?? []
  } catch {
    return []
  }
})()
const fallbackZones = ['Europe/Amsterdam', 'Europe/London', 'America/New_York', 'America/Chicago', 'America/Los_Angeles', 'Asia/Singapore', 'Asia/Tokyo', 'Australia/Sydney', 'UTC']

const newRule = (tz: string): SunRule => ({ label: '', timezone: tz, days: [1, 2, 3, 4, 5], from: '09:00', to: '17:00', destination: { type: 'queue', target: '' } })
const localZone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'Europe/Amsterdam'
const blank: SunRoute = { id: 0, name: '', rules: [newRule(localZone)], fallback: { type: 'voicemail', target: '' }, enabled: true }

/** Follow-the-sun: send calls to the site where it is office hours now, each in its own time zone. */
export function SunRoutes() {
  const { t } = useTranslation()
  const list = useApi<SunRoute[]>('/pbx/sun-routes')
  const [edit, setEdit] = useState<SunRoute | null>(null)
  async function remove(r: SunRoute) {
    if (!window.confirm(t('common.confirmDelete', { name: r.name }))) return
    await api('DELETE', `/pbx/sun-routes/${r.id}`)
    resetDestinations()
    list.reload()
  }
  return (
    <>
      <PageHeader title={t('sun.title')} subtitle={t('sun.subtitle')}>
        <Button onClick={() => setEdit(structuredClone(blank))}><Plus className="size-4" aria-hidden /> {t('sun.add')}</Button>
      </PageHeader>
      <ErrorMessage error={list.error} />
      <Card>
        <Table head={[t('common.name'), t('sun.rules'), t('sun.fallback'), '']}
          empty={list.data?.length === 0 && <Empty icon={<Sun className="size-7" />} title={t('sun.emptyTitle')} text={t('sun.emptyText')} />}>
          {list.data?.map((r) => (
            <tr key={r.id} className={r.enabled ? '' : 'opacity-50'}>
              <td className={`${td} text-ink`}>{r.name}</td>
              <td className={td}>
                <div className="flex flex-col gap-1">
                  {r.rules.map((x, i) => (
                    <div key={i} className="flex flex-wrap items-center gap-2 text-xs">
                      <Badge tone="accent">{x.label || x.timezone}</Badge>
                      <span className="font-mono">{dayList(x.days, t)} {x.from}–{x.to}</span>
                      <DestinationLabel value={x.destination} />
                    </div>
                  ))}
                </div>
              </td>
              <td className={td}><DestinationLabel value={r.fallback} /></td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <IconButton label={t('common.edit')} onClick={() => setEdit(r)}><Pencil className="size-4" aria-hidden /></IconButton>
                <IconButton label={t('common.delete')} danger onClick={() => remove(r)}><Trash2 className="size-4" aria-hidden /></IconButton>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
      {edit && <SunForm value={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); resetDestinations(); list.reload() }} />}
    </>
  )
}

function SunForm({ value, onClose, onSaved }: { value: SunRoute; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [r, setR] = useState(value)
  const { busy, error, save } = useSaver<SunRoute>('/pbx/sun-routes', onSaved)
  const setRule = (i: number, patch: Partial<SunRule>) => setR({ ...r, rules: r.rules.map((x, j) => (j === i ? { ...x, ...patch } : x)) })
  const exclude = { type: 'followsun' as const, target: String(r.id) }
  const tzs = zones.length ? zones : fallbackZones
  return (
    <Modal wide title={r.id ? t('sun.edit', { name: value.name }) : t('sun.add')} subtitle={t('sun.formHint')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={() => save(r)}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <Input name="name" label={t('common.name')} value={r.name} onChange={(e) => setR({ ...r, name: e.target.value })} placeholder={t('sun.namePlaceholder')} autoFocus />
      {r.rules.map((x, i) => (
        <div key={i} className="flex flex-col gap-3 rounded-xl border border-line p-4">
          <div className="flex items-start gap-3">
            <div className="grid flex-1 gap-3 sm:grid-cols-2">
              <Input label={t('sun.label')} value={x.label} onChange={(e) => setRule(i, { label: e.target.value })} placeholder={t('sun.labelPlaceholder')} />
              <Select label={t('sun.timezone')} value={x.timezone} onChange={(e) => setRule(i, { timezone: e.target.value })}>
                {!tzs.includes(x.timezone) && <option value={x.timezone}>{x.timezone}</option>}
                {tzs.map((z) => <option key={z} value={z}>{z}</option>)}
              </Select>
            </div>
            <IconButton label={t('common.remove')} danger onClick={() => setR({ ...r, rules: r.rules.filter((_, j) => j !== i) })}><X className="size-4" aria-hidden /></IconButton>
          </div>
          <div className="flex flex-wrap items-center gap-3">
            <div className="flex gap-1" role="group" aria-label={t('timeConditions.days')}>
              {DAYS.map((d) => (
                <button key={d} type="button" aria-pressed={x.days.includes(d)}
                  onClick={() => setRule(i, { days: x.days.includes(d) ? x.days.filter((y) => y !== d) : [...x.days, d].sort() })}
                  className={`size-10 rounded-lg border text-xs font-medium transition ${x.days.includes(d) ? 'border-accent/60 bg-accent/15 text-ink' : 'border-line text-muted hover:text-ink'}`}>
                  {t(`timeConditions.daysShort.${d}`)}
                </button>
              ))}
            </div>
            <input type="time" aria-label={t('timeConditions.from')} value={x.from} onChange={(e) => setRule(i, { from: e.target.value })} className="min-h-10 rounded-lg border border-line bg-field px-2 font-mono text-sm text-ink" />
            <span className="text-subtle">–</span>
            <input type="time" aria-label={t('timeConditions.to')} value={x.to === '24:00' ? '23:59' : x.to} onChange={(e) => setRule(i, { to: e.target.value })} className="min-h-10 rounded-lg border border-line bg-field px-2 font-mono text-sm text-ink" />
          </div>
          <DestinationPicker label={t('sun.goesTo')} value={x.destination} onChange={(d) => setRule(i, { destination: d })} exclude={exclude} />
        </div>
      ))}
      <div><Button variant="ghost" onClick={() => setR({ ...r, rules: [...r.rules, newRule(r.rules[r.rules.length - 1]?.timezone ?? localZone)] })}><Plus className="size-4" aria-hidden /> {t('sun.addRule')}</Button></div>
      <p className="text-xs text-subtle">{t('sun.orderHint')}</p>
      <DestinationPicker label={t('sun.fallback')} value={r.fallback} onChange={(d) => setR({ ...r, fallback: d })} exclude={exclude} />
      <Toggle name="enabled" label={t('common.enabled')} checked={r.enabled} onChange={(v) => setR({ ...r, enabled: v })} />
    </Modal>
  )
}
