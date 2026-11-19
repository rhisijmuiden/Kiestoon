import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CalendarDays, Clock, Pencil, Plus, Trash2, X } from 'lucide-react'
import { Badge, Empty, Modal, Table, Tabs, Toggle, formatDate, td, useApi } from '../components/kit'
import { DestinationLabel, DestinationPicker, IconButton, Segmented, resetDestinations, useSaver } from '../components/pbx'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api, isAdmin } from '../lib/api'
import { useSession } from '../lib/session'
import type { CustomHoliday, HolidayCalendar, HolidaysView, TimeCondition, TimeRange } from '../lib/types'

const DAYS = [1, 2, 3, 4, 5, 6, 7]

const blank: TimeCondition = {
  id: 0, name: '', schedule: [{ days: [1, 2, 3, 4, 5], from: '08:30', to: '17:30' }], useHolidays: true, mode: 'auto',
  open: { type: 'extension', target: '' }, closed: { type: 'voicemail', target: '' }, holiday: { type: '', target: '' },
}

export function TimeConditions() {
  const { t } = useTranslation()
  const [tab, setTab] = useState<'schedules' | 'holidays'>('schedules')
  return (
    <>
      <PageHeader title={t('timeConditions.title')} subtitle={t('timeConditions.subtitle')} />
      <Tabs value={tab} onChange={setTab} tabs={[{ key: 'schedules', label: t('timeConditions.schedules') }, { key: 'holidays', label: t('timeConditions.holidays') }]} />
      {tab === 'schedules' ? <Schedules /> : <Holidays />}
    </>
  )
}

/** The day/night switch: automatic, or forced open or closed. Used by admins and reception. */
export function ModeSwitch({ tc, onChanged }: { tc: TimeCondition; onChanged: () => void }) {
  const { t } = useTranslation()
  const [mode, setMode] = useState(tc.mode)
  useEffect(() => setMode(tc.mode), [tc.mode])
  async function change(m: TimeCondition['mode']) {
    setMode(m)
    try {
      await api('PUT', `/pbx/timeconditions/${tc.id}/mode`, { mode: m })
    } finally {
      onChanged()
    }
  }
  return (
    <Segmented value={mode} onChange={change} options={[
      { key: 'auto', label: t('timeConditions.modes.auto') },
      { key: 'open', label: t('timeConditions.modes.open'), tone: 'ok' },
      { key: 'closed', label: t('timeConditions.modes.closed'), tone: 'danger' },
    ]} />
  )
}

export function dayList(days: number[], t: (k: string) => string) {
  return days.map((d) => t(`timeConditions.daysShort.${d}`)).join(' ')
}

function Schedules() {
  const { t } = useTranslation()
  const list = useApi<TimeCondition[]>('/pbx/timeconditions')
  const [edit, setEdit] = useState<TimeCondition | null>(null)
  async function remove(tc: TimeCondition) {
    if (!window.confirm(t('common.confirmDelete', { name: tc.name }))) return
    await api('DELETE', `/pbx/timeconditions/${tc.id}`)
    resetDestinations()
    list.reload()
  }
  return (
    <>
      <div className="flex justify-end">
        <Button onClick={() => setEdit(blank)}><Plus className="size-4" aria-hidden /> {t('timeConditions.add')}</Button>
      </div>
      <ErrorMessage error={list.error} />
      <Card>
        <Table head={[t('common.name'), t('timeConditions.openHours'), t('timeConditions.whenOpen'), t('timeConditions.whenClosed'), t('timeConditions.mode'), t('timeConditions.code'), '']}
          empty={list.data?.length === 0 && <Empty icon={<Clock className="size-7" />} title={t('timeConditions.emptyTitle')} text={t('timeConditions.emptyText')} />}>
          {list.data?.map((tc) => (
            <tr key={tc.id}>
              <td className={`${td} text-ink`}>{tc.name}{tc.useHolidays && <span className="ml-2"><Badge>{t('timeConditions.holidaysShort')}</Badge></span>}</td>
              <td className={td}>
                {tc.schedule.map((s, i) => <div key={i} className="whitespace-nowrap text-xs"><span className="text-subtle">{dayList(s.days, t)}</span> <span className="font-mono text-ink-2">{s.from}–{s.to}</span></div>)}
              </td>
              <td className={td}><DestinationLabel value={tc.open} /></td>
              <td className={td}><DestinationLabel value={tc.closed} /></td>
              <td className={td}><ModeSwitch tc={tc} onChanged={list.reload} /></td>
              <td className={`${td} font-mono text-xs`}>*28{tc.id}</td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <IconButton label={t('common.edit')} onClick={() => setEdit(tc)}><Pencil className="size-4" aria-hidden /></IconButton>
                <IconButton label={t('common.delete')} danger onClick={() => remove(tc)}><Trash2 className="size-4" aria-hidden /></IconButton>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
      {edit && <TimeConditionForm value={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); resetDestinations(); list.reload() }} />}
    </>
  )
}

function TimeConditionForm({ value, onClose, onSaved }: { value: TimeCondition; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [tc, setTc] = useState(value)
  const { busy, error, save } = useSaver<TimeCondition>('/pbx/timeconditions', onSaved)
  const set = <K extends keyof TimeCondition>(k: K, v: TimeCondition[K]) => setTc((x) => ({ ...x, [k]: v }))
  const setRange = (i: number, patch: Partial<TimeRange>) => set('schedule', tc.schedule.map((s, j) => (j === i ? { ...s, ...patch } : s)))
  const exclude = { type: 'timecondition' as const, target: String(tc.id) }
  return (
    <Modal wide title={tc.id ? t('timeConditions.edit', { name: value.name }) : t('timeConditions.add')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={() => save(tc)}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <Input name="name" label={t('common.name')} value={tc.name} onChange={(e) => set('name', e.target.value)} autoFocus placeholder={t('timeConditions.namePlaceholder')} />
      <div className="flex flex-col gap-2">
        <span className="text-sm font-medium text-ink-2">{t('timeConditions.openHours')}</span>
        {tc.schedule.map((s, i) => (
          <div key={i} className="flex flex-wrap items-center gap-3 rounded-xl border border-line p-3">
            <div className="flex gap-1" role="group" aria-label={t('timeConditions.days')}>
              {DAYS.map((d) => (
                <button key={d} type="button" aria-pressed={s.days.includes(d)}
                  onClick={() => setRange(i, { days: s.days.includes(d) ? s.days.filter((x) => x !== d) : [...s.days, d].sort() })}
                  className={`size-10 rounded-lg border text-xs font-medium transition ${s.days.includes(d) ? 'border-accent/60 bg-accent/15 text-ink' : 'border-line text-muted hover:text-ink'}`}>
                  {t(`timeConditions.daysShort.${d}`)}
                </button>
              ))}
            </div>
            <input type="time" aria-label={t('timeConditions.from')} value={s.from} onChange={(e) => setRange(i, { from: e.target.value })} className="min-h-10 rounded-lg border border-line bg-field px-2 font-mono text-sm text-ink" />
            <span className="text-subtle">–</span>
            <input type="time" aria-label={t('timeConditions.to')} value={s.to === '24:00' ? '23:59' : s.to} onChange={(e) => setRange(i, { to: e.target.value })} className="min-h-10 rounded-lg border border-line bg-field px-2 font-mono text-sm text-ink" />
            <IconButton label={t('common.remove')} danger onClick={() => set('schedule', tc.schedule.filter((_, j) => j !== i))}><X className="size-4" aria-hidden /></IconButton>
          </div>
        ))}
        <div><Button variant="ghost" onClick={() => set('schedule', [...tc.schedule, { days: [1, 2, 3, 4, 5], from: '12:00', to: '13:00' }])}><Plus className="size-4" aria-hidden /> {t('timeConditions.addRange')}</Button></div>
        <p className="text-xs text-subtle">{t('timeConditions.rangesHint')}</p>
      </div>
      <DestinationPicker label={t('timeConditions.whenOpen')} value={tc.open} onChange={(d) => set('open', d)} exclude={exclude} />
      <DestinationPicker label={t('timeConditions.whenClosed')} value={tc.closed} onChange={(d) => set('closed', d)} exclude={exclude} />
      <Toggle name="useHolidays" label={t('timeConditions.useHolidays')} hint={t('timeConditions.useHolidaysHint')} checked={tc.useHolidays} onChange={(v) => set('useHolidays', v)} />
      {tc.useHolidays && <DestinationPicker allowNone label={t('timeConditions.whenHoliday')} value={tc.holiday} onChange={(d) => set('holiday', d)} exclude={exclude} />}
      <Segmented label={t('timeConditions.mode')} value={tc.mode} onChange={(m) => set('mode', m)} options={[
        { key: 'auto', label: t('timeConditions.modes.auto') },
        { key: 'open', label: t('timeConditions.modes.open'), tone: 'ok' },
        { key: 'closed', label: t('timeConditions.modes.closed'), tone: 'danger' },
      ]} />
    </Modal>
  )
}

function Holidays() {
  const { t, i18n } = useTranslation()
  const { state } = useSession()
  const admin = isAdmin(state.kind === 'ready' ? state.me.user : undefined)
  const view = useApi<HolidaysView>('/pbx/holidays')
  const [cal, setCal] = useState<HolidayCalendar | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  const [nh, setNh] = useState<CustomHoliday>({ date: '', name: '', annual: false })
  useEffect(() => { if (view.data) setCal(view.data.calendar) }, [view.data])
  if (!cal || !view.data) return <ErrorMessage error={view.error} />
  const dateOf = (key: string) => view.data!.days.find((d) => d.key === key)?.date
  async function save(c: HolidayCalendar) {
    setBusy(true)
    setError(null)
    setSaved(false)
    try {
      await api('PUT', '/pbx/holidays', c)
      setSaved(true)
      view.reload()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const togglePreset = (k: string) => setCal({ ...cal, preset: cal.preset.includes(k) ? cal.preset.filter((x) => x !== k) : [...cal.preset, k] })
  const day = (d: string) => new Date(`${d}T12:00:00`).toLocaleDateString(i18n.language, { weekday: 'short', day: 'numeric', month: 'long' })
  return (
    <>
      <ErrorMessage error={error} />
      {saved && <p className="text-sm text-ok" role="status">{t('common.saved')}</p>}
      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <h2 className="mb-1 font-display text-lg font-semibold">{t('timeConditions.dutchHolidays', { year: view.data.year })}</h2>
          <p className="mb-4 text-sm text-muted">{t('timeConditions.dutchHolidaysHint')}</p>
          <div className="flex flex-col gap-1.5">
            {view.data.presets.map((k) => (
              <label key={k} className="flex min-h-11 cursor-pointer items-center gap-3 rounded-lg border border-line px-3 text-sm hover:bg-tint/[0.03]">
                <input type="checkbox" disabled={!admin} checked={cal.preset.includes(k)} onChange={() => togglePreset(k)} className="size-4 accent-[var(--color-accent)]" />
                <span className="flex-1 text-ink">{t(`holidays.${k}`)}</span>
                <span className="font-mono text-xs text-subtle">{dateOf(k) ? day(dateOf(k)!) : ''}</span>
              </label>
            ))}
          </div>
        </Card>
        <Card>
          <h2 className="mb-1 font-display text-lg font-semibold">{t('timeConditions.customHolidays')}</h2>
          <p className="mb-4 text-sm text-muted">{t('timeConditions.customHolidaysHint')}</p>
          <div className="flex flex-col gap-1.5">
            {cal.custom.length === 0 && <p className="py-4 text-center text-sm text-subtle">{t('timeConditions.noCustom')}</p>}
            {cal.custom.map((c, i) => (
              <div key={i} className="flex min-h-11 items-center gap-3 rounded-lg border border-line px-3 text-sm">
                <CalendarDays className="size-4 text-accent" aria-hidden />
                <span className="flex-1 text-ink">{c.name}</span>
                <span className="font-mono text-xs text-subtle">{c.annual ? c.date.slice(5) : formatDate(c.date, i18n.language).split(' ')[0]}</span>
                {c.annual && <Badge>{t('timeConditions.annual')}</Badge>}
                {admin && <IconButton label={t('common.remove')} danger onClick={() => setCal({ ...cal, custom: cal.custom.filter((_, j) => j !== i) })}><X className="size-4" aria-hidden /></IconButton>}
              </div>
            ))}
          </div>
          {admin && (
            <div className="mt-4 grid gap-3 rounded-xl border border-line p-3 sm:grid-cols-[1fr_160px]">
              <Input name="name" label={t('common.name')} value={nh.name} onChange={(e) => setNh({ ...nh, name: e.target.value })} placeholder={t('timeConditions.customPlaceholder')} />
              <Input name="date" label={t('timeConditions.date')} type="date" value={nh.date} onChange={(e) => setNh({ ...nh, date: e.target.value })} />
              <label className="flex items-center gap-2 text-sm text-ink-2">
                <input type="checkbox" checked={nh.annual} onChange={(e) => setNh({ ...nh, annual: e.target.checked })} className="size-4 accent-[var(--color-accent)]" />
                {t('timeConditions.annualHint')}
              </label>
              <Button variant="ghost" disabled={!nh.name || !nh.date} onClick={() => { setCal({ ...cal, custom: [...cal.custom, nh] }); setNh({ date: '', name: '', annual: false }) }}>
                <Plus className="size-4" aria-hidden /> {t('common.add')}
              </Button>
            </div>
          )}
        </Card>
      </div>
      {admin && <div className="flex justify-end"><Button busy={busy} onClick={() => save(cal)}>{t('common.save')}</Button></div>}
    </>
  )
}
