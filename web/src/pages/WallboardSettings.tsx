import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { ArrowLeft, BellRing, KeyRound, MonitorPlay, Pencil, Plus, Trash2, Tv } from 'lucide-react'
import { Badge, CopyField, Empty, Modal, SectionTitle, Select, Table, Textarea, Toggle, td, useApi } from '../components/kit'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { IconButton } from '../components/pbx'
import { api } from '../lib/api'
import type { AlertSettings, SLAOptions, WallLayout, WallScreen, WallSettings } from '../lib/types'

const allLayouts: WallLayout[] = ['overview', 'queue', 'agents']
const blank: WallScreen = { id: 0, name: '', key: '', queues: [], layouts: ['overview'], rotate: 30, theme: 'dark', hideNames: false, createdAt: '', lastSeen: '' }

export const wallLink = (key: string) => `${window.location.origin}/wall/${key}`

/** Wallboard settings: tv links, SLA thresholds per queue and who gets alerts. */
export function WallboardSettings() {
  const { t } = useTranslation()
  const d = useApi<WallSettings>('/wallboard/settings')
  const [edit, setEdit] = useState<WallScreen | null>(null)
  const [error, setError] = useState<unknown>(null)
  if (!d.data) return <><PageHeader title={t('wallboard.settingsTitle')} /><ErrorMessage error={d.error} /></>
  const data = d.data

  async function act(fn: () => Promise<unknown>) {
    setError(null)
    try { await fn(); await d.reload() } catch (e) { setError(e) }
  }
  const newKey = (s: WallScreen) => {
    if (window.confirm(t('wallboard.newKeyConfirm', { name: s.name }))) act(() => api('POST', `/wallboard/screens/${s.id}/key`, {}))
  }
  const revoke = (s: WallScreen) => {
    if (window.confirm(t('wallboard.revokeConfirm', { name: s.name }))) act(() => api('DELETE', `/wallboard/screens/${s.id}`, {}))
  }
  const queueName = (id: number) => data.queues.find((q) => q.id === id)?.name ?? `#${id}`

  return (
    <>
      <PageHeader title={t('wallboard.settingsTitle')} subtitle={t('wallboard.settingsSubtitle')}>
        <Link to="/wallboard" className="inline-flex min-h-11 items-center justify-center gap-2 rounded-[10px] border border-line px-4 text-sm font-medium text-ink-2 transition hover:bg-tint/5">
          <ArrowLeft className="size-4" aria-hidden />{t('wallboard.title')}
        </Link>
      </PageHeader>
      <ErrorMessage error={error} />
      <Card>
        <SectionTitle action={<Button onClick={() => setEdit({ ...blank })}><Plus className="size-4" aria-hidden />{t('wallboard.newScreen')}</Button>}>
          <span className="inline-flex items-center gap-2"><Tv className="size-4 text-accent" aria-hidden />{t('wallboard.screens')}</span>
        </SectionTitle>
        <p className="-mt-2 mb-4 max-w-[75ch] text-sm text-muted">{t('wallboard.screensIntro')}</p>
        {data.screens.length === 0 && <Empty icon={<MonitorPlay className="size-7" />} title={t('wallboard.noScreens')} text={t('wallboard.noScreensText')} />}
        <div className="grid gap-4 xl:grid-cols-2">
          {data.screens.map((s) => (
            <div key={s.id} className="flex flex-col gap-3 rounded-xl border border-line p-4">
              <div className="flex items-start justify-between gap-3">
                <div>
                  <div className="font-medium text-ink">{s.name}</div>
                  <div className="text-xs text-subtle">{s.lastSeen ? t('wallboard.lastSeen', { time: new Date(s.lastSeen).toLocaleString() }) : t('wallboard.neverSeen')}</div>
                </div>
                <div className="flex">
                  <IconButton label={t('common.edit')} onClick={() => setEdit(s)}><Pencil className="size-4" aria-hidden /></IconButton>
                  <IconButton label={t('wallboard.newKey')} onClick={() => newKey(s)}><KeyRound className="size-4" aria-hidden /></IconButton>
                  <IconButton label={t('wallboard.revoke')} onClick={() => revoke(s)}><Trash2 className="size-4" aria-hidden /></IconButton>
                </div>
              </div>
              <CopyField label={t('wallboard.link')} value={wallLink(s.key)} />
              <div className="flex flex-wrap gap-1.5">
                {s.queues.length === 0 ? <Badge>{t('wallboard.allQueues')}</Badge> : s.queues.map((id) => <Badge key={id}>{queueName(id)}</Badge>)}
                {s.layouts.map((l) => <Badge key={l} tone="accent">{t(`wallboard.layout.${l}`)}</Badge>)}
                <Badge>{t(s.theme === 'light' ? 'wallboard.themeLight' : 'wallboard.themeDark')}</Badge>
                {s.layouts.length > 1 || s.layouts.includes('queue') ? <Badge>{s.rotate ? t('wallboard.rotateEvery', { s: s.rotate }) : t('wallboard.noRotate')}</Badge> : null}
                {s.hideNames && <Badge tone="warn">{t('wallboard.namesHidden')}</Badge>}
              </div>
            </div>
          ))}
        </div>
      </Card>
      <AlertsCard data={data} onSaved={d.reload} />
      <ThresholdsCard data={data} onSaved={d.reload} />
      {edit && <ScreenEditor screen={edit} data={data} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); d.reload() }} />}
    </>
  )
}

function ScreenEditor({ screen, data, onClose, onSaved }: { screen: WallScreen; data: WallSettings; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [s, setS] = useState(screen)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const set = (p: Partial<WallScreen>) => setS({ ...s, ...p })
  const toggleIn = <T,>(list: T[], v: T) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v])
  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api(s.id ? 'PUT' : 'POST', s.id ? `/wallboard/screens/${s.id}` : '/wallboard/screens', s)
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const chip = (on: boolean) => `min-h-10 rounded-lg border px-3 text-sm transition ${on ? 'border-accent/50 bg-accent/15 text-ink' : 'border-line text-muted hover:text-ink'}`
  return (
    <Modal title={s.id ? s.name : t('wallboard.newScreen')} subtitle={t('wallboard.screenHint')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <Input label={t('common.name')} value={s.name} onChange={(e) => set({ name: e.target.value })} placeholder={t('wallboard.namePlaceholder')} />
      <fieldset className="flex flex-col gap-2">
        <legend className="mb-2 text-sm font-medium text-ink-2">{t('wallboard.queues')}</legend>
        <div className="flex flex-wrap gap-2">
          <button type="button" aria-pressed={s.queues.length === 0} className={chip(s.queues.length === 0)} onClick={() => set({ queues: [] })}>{t('wallboard.allQueues')}</button>
          {data.queues.map((q) => (
            <button key={q.id} type="button" aria-pressed={s.queues.includes(q.id)} className={chip(s.queues.includes(q.id))} onClick={() => set({ queues: toggleIn(s.queues, q.id) })}>
              {q.name} <span className="font-mono text-xs text-subtle">{q.number}</span>
            </button>
          ))}
        </div>
      </fieldset>
      <fieldset className="flex flex-col gap-2">
        <legend className="mb-2 text-sm font-medium text-ink-2">{t('wallboard.layouts')}</legend>
        <div className="flex flex-wrap gap-2">
          {allLayouts.map((l) => (
            <button key={l} type="button" aria-pressed={s.layouts.includes(l)} className={chip(s.layouts.includes(l))}
              onClick={() => { const next = toggleIn(s.layouts, l); set({ layouts: next.length ? next : s.layouts }) }}>{t(`wallboard.layout.${l}`)}</button>
          ))}
        </div>
      </fieldset>
      <div className="grid gap-4 sm:grid-cols-2">
        <Select label={t('wallboard.rotate')} value={s.rotate} onChange={(e) => set({ rotate: Number(e.target.value) })}>
          <option value={0}>{t('wallboard.noRotate')}</option>
          {[15, 30, 60, 120].map((n) => <option key={n} value={n}>{t('wallboard.rotateEvery', { s: n })}</option>)}
        </Select>
        <Select label={t('wallboard.theme')} value={s.theme} onChange={(e) => set({ theme: e.target.value as 'dark' | 'light' })}>
          <option value="dark">{t('wallboard.themeDark')}</option>
          <option value="light">{t('wallboard.themeLight')}</option>
        </Select>
      </div>
      <Toggle label={t('wallboard.hideNames')} hint={t('wallboard.hideNamesHint')} checked={s.hideNames} onChange={(v) => set({ hideNames: v })} />
    </Modal>
  )
}

function AlertsCard({ data, onSaved }: { data: WallSettings; onSaved: () => void }) {
  const { t } = useTranslation()
  const [a, setA] = useState<AlertSettings>(data.alerts)
  const [rcpt, setRcpt] = useState(data.alerts.recipients.join(', '))
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  const set = (p: Partial<AlertSettings>) => { setSaved(false); setA({ ...a, ...p }) }
  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api('PUT', '/wallboard/alerts', { ...a, recipients: rcpt.split(/[\s,;]+/).filter(Boolean) })
      setSaved(true)
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Card>
      <SectionTitle><span className="inline-flex items-center gap-2"><BellRing className="size-4 text-accent" aria-hidden />{t('wallboard.alerts')}</span></SectionTitle>
      <p className="-mt-2 mb-4 max-w-[75ch] text-sm text-muted">{t('wallboard.alertsIntro')}</p>
      <ErrorMessage error={error} />
      {saved && <Alert tone="ok">{t('common.saved')}</Alert>}
      <div className="grid gap-4 lg:grid-cols-2">
        <div className="flex flex-col gap-3">
          <Toggle label={t('wallboard.alertScreens')} hint={t('wallboard.alertScreensHint')} checked={a.screens} onChange={(v) => set({ screens: v })} />
          <Toggle label={t('wallboard.alertEmail')} checked={a.email} onChange={(v) => set({ email: v })} />
          {a.email && <Textarea label={t('wallboard.recipients')} hint={t('wallboard.recipientsHint')} rows={2} value={rcpt} onChange={(e) => { setSaved(false); setRcpt(e.target.value) }} />}
          <Toggle label={t('wallboard.alertPush')} hint={t('wallboard.alertPushHint')} checked={false} onChange={() => undefined} disabled />
        </div>
        <div className="flex flex-col gap-3">
          <Input label={t('wallboard.quiet')} hint={t('wallboard.quietHint')} type="number" min={1} max={1440} value={a.quiet} onChange={(e) => set({ quiet: Number(e.target.value) })} />
          <Select label={t('wallboard.onlyDuring')} value={a.timeCondition} onChange={(e) => set({ timeCondition: Number(e.target.value) })}>
            <option value={0}>{t('wallboard.always')}</option>
            {data.timeConditions.map((tc) => <option key={tc.id} value={tc.id}>{t('wallboard.openHoursOf', { name: tc.name })}</option>)}
          </Select>
          <Toggle label={t('wallboard.recovered')} checked={a.recovered} onChange={(v) => set({ recovered: v })} />
        </div>
      </div>
      <div className="mt-4 flex justify-end"><Button busy={busy} onClick={save}>{t('common.save')}</Button></div>
    </Card>
  )
}

type Row = { id: number; sla: SLAOptions }

function NumberCell({ value, onChange, label, suffix, max }: { value: number; onChange: (v: number) => void; label: string; suffix?: string; max: number }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <input type="number" min={0} max={max} aria-label={label} value={value || ''} placeholder="–" onChange={(e) => onChange(Math.max(0, Number(e.target.value) || 0))}
        className="h-10 w-20 rounded-lg border border-line bg-inset px-2.5 font-mono text-sm text-ink outline-none focus:border-accent/60" />
      {suffix && <span className="text-xs text-subtle">{suffix}</span>}
    </span>
  )
}

function ThresholdsCard({ data, onSaved }: { data: WallSettings; onSaved: () => void }) {
  const { t } = useTranslation()
  const [rows, setRows] = useState<Row[]>(() => data.queues.map((q) => ({ id: q.id, sla: q.sla })))
  useEffect(() => { setRows(data.queues.map((q) => ({ id: q.id, sla: q.sla }))) }, [data.queues])
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  const set = (id: number, p: Partial<SLAOptions>) => { setSaved(false); setRows(rows.map((r) => (r.id === id ? { ...r, sla: { ...r.sla, ...p } } : r))) }
  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api('PUT', '/wallboard/sla', rows.map((r) => ({ queueId: r.id, sla: r.sla })))
      setSaved(true)
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const monitored = (s: SLAOptions) => s.norm > 0 || s.maxWaiting > 0 || s.maxWait > 0 || s.maxAbandoned > 0
  return (
    <Card>
      <SectionTitle>{t('wallboard.thresholds')}</SectionTitle>
      <p className="-mt-2 mb-4 max-w-[75ch] text-sm text-muted">{t('wallboard.thresholdsIntro')}</p>
      <ErrorMessage error={error} />
      {saved && <Alert tone="ok">{t('common.saved')}</Alert>}
      <Table head={[t('wallboard.queue'), t('wallboard.target'), t('wallboard.norm'), t('wallboard.maxWaiting'), t('wallboard.maxWait'), t('wallboard.maxAbandoned'), t('wallboard.now')]}
        empty={data.queues.length === 0 ? <Empty title={t('wallboard.emptyTitle')} text={t('wallboard.emptyText')} /> : undefined}>
        {rows.map((r) => {
          const q = data.queues.find((x) => x.id === r.id)!
          return (
            <tr key={r.id}>
              <td className={td}><span className="text-ink">{q.name}</span> <span className="font-mono text-xs text-subtle">{q.number}</span></td>
              <td className={td}><NumberCell label={t('wallboard.target')} value={r.sla.target} max={600} suffix="s" onChange={(v) => set(r.id, { target: v })} /></td>
              <td className={td}><NumberCell label={t('wallboard.norm')} value={r.sla.norm} max={100} suffix="%" onChange={(v) => set(r.id, { norm: v })} /></td>
              <td className={td}><NumberCell label={t('wallboard.maxWaiting')} value={r.sla.maxWaiting} max={500} onChange={(v) => set(r.id, { maxWaiting: v })} /></td>
              <td className={td}><NumberCell label={t('wallboard.maxWait')} value={r.sla.maxWait} max={7200} suffix="s" onChange={(v) => set(r.id, { maxWait: v })} /></td>
              <td className={td}><NumberCell label={t('wallboard.maxAbandoned')} value={r.sla.maxAbandoned} max={100} suffix="%" onChange={(v) => set(r.id, { maxAbandoned: v })} /></td>
              <td className={td}>
                {!monitored(q.sla) ? <Badge>{t('wallboard.notMonitored')}</Badge>
                  : q.alerts.length ? <Badge tone="danger" dot>{t('wallboard.over')}</Badge> : <Badge tone="ok" dot>{t('wallboard.withinNorm')}</Badge>}
              </td>
            </tr>
          )
        })}
      </Table>
      <div className="mt-4 flex justify-end"><Button busy={busy} disabled={!rows.length} onClick={save}>{t('common.save')}</Button></div>
    </Card>
  )
}
