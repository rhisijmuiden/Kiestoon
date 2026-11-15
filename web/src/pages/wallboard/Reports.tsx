import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Eye, FileBarChart, Pencil, Plus, Send, Trash2 } from 'lucide-react'
import { Badge, Empty, Modal, SectionTitle, Select, Table, Textarea, Toggle, td, useApi } from '../../components/kit'
import { Alert, Button, Card, ErrorMessage, Input } from '../../components/ui'
import { IconButton } from '../../components/pbx'
import { api } from '../../lib/api'
import type { Report, ReportSection, WallSettings } from '../../lib/types'

const sections: ReportSection[] = ['queues', 'agents', 'hours', 'trend']

/** Scheduled reports by e-mail, on the wallboard settings page. */
export function ReportsCard({ data }: { data: WallSettings }) {
  const { t, i18n } = useTranslation()
  const list = useApi<Report[]>('/wallboard/reports')
  const [edit, setEdit] = useState<Report | null>(null)
  const [preview, setPreview] = useState<{ subject: string; html: string } | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [sent, setSent] = useState('')
  const blank: Report = {
    id: 0, name: '', period: 'weekly', time: '08:00', weekday: 1, workdays: true, queues: [], sections: ['queues', 'agents', 'trend'],
    recipients: [], language: i18n.language.startsWith('nl') ? 'nl' : 'en', enabled: true, lastSent: '',
  }
  async function act(fn: () => Promise<unknown>) {
    setError(null)
    setSent('')
    try { await fn() } catch (e) { setError(e) }
  }
  const when = (r: Report) => r.period === 'daily' ? t(r.workdays ? 'reports.whenWorkdays' : 'reports.whenDaily', { time: r.time })
    : r.period === 'weekly' ? t('reports.whenWeekly', { day: t(`reports.day.${r.weekday}`), time: r.time }) : t('reports.whenMonthly', { time: r.time })
  const queueNames = (ids: number[]) => ids.length === 0 ? t('wallboard.allQueues') : ids.map((id) => data.queues.find((q) => q.id === id)?.name ?? `#${id}`).join(', ')

  return (
    <Card>
      <SectionTitle action={<Button onClick={() => setEdit({ ...blank })}><Plus className="size-4" aria-hidden />{t('reports.new')}</Button>}>
        <span className="inline-flex items-center gap-2"><FileBarChart className="size-4 text-accent" aria-hidden />{t('reports.title')}</span>
      </SectionTitle>
      <p className="-mt-2 mb-4 max-w-[75ch] text-sm text-muted">{t('reports.intro')}</p>
      <ErrorMessage error={error || list.error} />
      {sent && <Alert tone="ok">{t('reports.sentTo', { to: sent })}</Alert>}
      <Table head={[t('common.name'), t('reports.when'), t('wallboard.queues'), t('reports.recipients'), t('reports.language'), '']}
        empty={list.data?.length === 0 ? <Empty icon={<FileBarChart className="size-7" />} title={t('reports.none')} text={t('reports.noneText')} /> : undefined}>
        {(list.data ?? []).map((r) => (
          <tr key={r.id}>
            <td className={td}><span className="text-ink">{r.name}</span> {!r.enabled && <Badge>{t('reports.paused')}</Badge>}</td>
            <td className={td}>{when(r)}</td>
            <td className={td}>{queueNames(r.queues)}</td>
            <td className={td}><span className="line-clamp-2 break-all">{r.recipients.join(', ')}</span></td>
            <td className={td}><Badge>{r.language.toUpperCase()}</Badge></td>
            <td className={`${td} whitespace-nowrap text-right`}>
              <IconButton label={t('reports.preview')} onClick={() => act(async () => setPreview(await api('GET', `/wallboard/reports/${r.id}/preview`)))}><Eye className="size-4" aria-hidden /></IconButton>
              <IconButton label={t('reports.sendMe')} onClick={() => act(async () => { const res = await api<{ to: string }>('POST', `/wallboard/reports/${r.id}/send`, {}); setSent(res.to) })}><Send className="size-4" aria-hidden /></IconButton>
              <IconButton label={t('common.edit')} onClick={() => setEdit(r)}><Pencil className="size-4" aria-hidden /></IconButton>
              <IconButton label={t('common.delete')} onClick={() => { if (window.confirm(t('common.confirmDelete', { name: r.name }))) act(async () => { await api('DELETE', `/wallboard/reports/${r.id}`, {}); list.reload() }) }}><Trash2 className="size-4" aria-hidden /></IconButton>
            </td>
          </tr>
        ))}
      </Table>
      {edit && <ReportEditor report={edit} data={data} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); list.reload() }} />}
      {preview && (
        <Modal title={preview.subject} subtitle={t('reports.previewHint')} onClose={() => setPreview(null)} wide>
          <iframe title={preview.subject} sandbox="" srcDoc={preview.html} className="h-[70vh] w-full rounded-xl border border-line bg-white" />
        </Modal>
      )}
    </Card>
  )
}

function ReportEditor({ report, data, onClose, onSaved }: { report: Report; data: WallSettings; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [r, setR] = useState(report)
  const [rcpt, setRcpt] = useState(report.recipients.join(', '))
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const set = (p: Partial<Report>) => setR({ ...r, ...p })
  const toggleIn = <T,>(list: T[], v: T) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v])
  const chip = (on: boolean) => `min-h-10 rounded-lg border px-3 text-sm transition ${on ? 'border-accent/50 bg-accent/15 text-ink' : 'border-line text-muted hover:text-ink'}`
  async function save() {
    setBusy(true)
    setError(null)
    try {
      const body = { ...r, recipients: rcpt.split(/[\s,;]+/).filter(Boolean) }
      await api(r.id ? 'PUT' : 'POST', r.id ? `/wallboard/reports/${r.id}` : '/wallboard/reports', body)
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={r.id ? r.name : t('reports.new')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <Input label={t('common.name')} value={r.name} placeholder={t('reports.namePlaceholder')} onChange={(e) => set({ name: e.target.value })} />
      <div className="grid gap-4 sm:grid-cols-3">
        <Select label={t('reports.period')} value={r.period} onChange={(e) => set({ period: e.target.value as Report['period'] })}>
          {(['daily', 'weekly', 'monthly'] as const).map((p) => <option key={p} value={p}>{t(`reports.periods.${p}`)}</option>)}
        </Select>
        {r.period === 'weekly' && (
          <Select label={t('reports.weekday')} value={r.weekday} onChange={(e) => set({ weekday: Number(e.target.value) })}>
            {[1, 2, 3, 4, 5, 6, 7].map((d) => <option key={d} value={d}>{t(`reports.day.${d}`)}</option>)}
          </Select>
        )}
        <Input label={t('reports.time')} type="time" value={r.time} onChange={(e) => set({ time: e.target.value })} />
      </div>
      <p className="-mt-2 text-sm text-muted">{t(`reports.covers.${r.period}`)}</p>
      {r.period === 'daily' && <Toggle label={t('reports.workdays')} checked={r.workdays} onChange={(v) => set({ workdays: v })} />}
      <fieldset className="flex flex-col gap-2">
        <legend className="mb-2 text-sm font-medium text-ink-2">{t('wallboard.queues')}</legend>
        <div className="flex flex-wrap gap-2">
          <button type="button" aria-pressed={r.queues.length === 0} className={chip(r.queues.length === 0)} onClick={() => set({ queues: [] })}>{t('wallboard.allQueues')}</button>
          {data.queues.map((q) => <button key={q.id} type="button" aria-pressed={r.queues.includes(q.id)} className={chip(r.queues.includes(q.id))} onClick={() => set({ queues: toggleIn(r.queues, q.id) })}>{q.name}</button>)}
        </div>
      </fieldset>
      <fieldset className="flex flex-col gap-2">
        <legend className="mb-2 text-sm font-medium text-ink-2">{t('reports.content')}</legend>
        <div className="flex flex-wrap gap-2">
          {sections.map((s) => <button key={s} type="button" aria-pressed={r.sections.includes(s)} className={chip(r.sections.includes(s))}
            onClick={() => { const next = toggleIn(r.sections, s); set({ sections: next.length ? next : r.sections }) }}>{t(`reports.section.${s}`)}</button>)}
        </div>
      </fieldset>
      <Textarea label={t('reports.recipients')} hint={t('wallboard.recipientsHint')} rows={2} value={rcpt} onChange={(e) => setRcpt(e.target.value)} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Select label={t('reports.language')} value={r.language} onChange={(e) => set({ language: e.target.value as 'en' | 'nl' })}>
          <option value="en">English</option>
          <option value="nl">Nederlands</option>
        </Select>
      </div>
      <Toggle label={t('reports.enabled')} checked={r.enabled} onChange={(v) => set({ enabled: v })} />
    </Modal>
  )
}
