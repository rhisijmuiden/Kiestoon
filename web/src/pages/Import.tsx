import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { ArrowLeft, CheckCircle2, FileArchive, FileUp, GitBranch, Import as ImportIcon, Info, ListOrdered, Phone, Clock, Upload, User, UsersRound, Waypoints, AudioLines } from 'lucide-react'
import { Badge, SectionTitle, Select, Table, td, useApi } from '../components/kit'
import { Alert, Button, Card, ErrorMessage, PageHeader } from '../components/ui'
import { ApiError, api } from '../lib/api'
import { toTelephoneWav } from '../lib/audio'

type Note = { code: string; args?: Record<string, string> }
type Ext = { key: string; sourceNumber: string; name: string; email?: string; number: string; voicemail: boolean; keepPassword: boolean; existingId?: number }
type Group = { key: string; sourceNumber: string; name: string; number: string; strategy: string; members: string[]; failover: string; existingId?: number }
type Queue = { key: string; sourceNumber: string; name: string; number: string; strategy: string; maxWait: number; agents: string[]; overflow: string; audio: string[]; existingId?: number }
type Trunk = { key: string; name: string; provider: string; host: string; username: string; register: boolean; hasSecret: boolean; numbers: string[]; existingId?: number }
type TimeRange = { days: number[]; from: string; to: string }
type Time = { key: string; name: string; schedule: TimeRange[]; open: string; closed: string; existingId?: number }
type NumberRow = { key: string; number: string; target: string; exists: boolean }
type Step = { id: number; type: string; key?: string; title?: string; audio?: string; greeting?: string; target?: string; noAction?: string; children?: Step[] }
type Menu = { key: string; name: string; sourceNumber: string; numbers: string[]; steps: Step; existingId?: number }
type Audio = { file: string; name: string; bytes: number; usedBy: string[]; playable: boolean }
type Plan = {
  source: { system: string; product: string; host?: string; createdAt?: number; details?: Record<string, string> }
  digits: number
  extensions: Ext[]; ringGroups: Group[]; queues: Queue[]; trunks: Trunk[]; timeConditions: Time[]; numbers: NumberRow[]; menus: Menu[]; audio: Audio[]; notes: Note[]
}
type Created = { kind: string; key: string; id: number; name: string; number?: string; reused?: boolean }
type Result = { created: Created[]; warnings: Note[] }
type Section = 'extensions' | 'ringGroups' | 'queues' | 'trunks' | 'timeConditions' | 'menus' | 'numbers'

const SYSTEMS = ['auto', 'asterisk', 'freepbx', 'freeswitch', '3cx', 'unifi-talk']

/** Uploads one file as the request body, with progress. */
function uploadFile(url: string, file: File, onProgress: (sent: number) => void): Promise<{ id: string }> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', url)
    xhr.withCredentials = true
    xhr.setRequestHeader('Content-Type', 'application/octet-stream')
    xhr.setRequestHeader('X-Kiestoon-Upload', '1')
    xhr.upload.onprogress = (e) => onProgress(e.loaded)
    xhr.onload = () => {
      let data: { id?: string; error?: string; field?: string; message?: string } = {}
      try {
        data = JSON.parse(xhr.responseText)
      } catch {
        return reject(new ApiError(xhr.status, xhr.status === 413 ? 'proxy_too_large' : 'internal'))
      }
      if (xhr.status >= 200 && xhr.status < 300 && data.id) return resolve({ id: data.id })
      reject(new ApiError(xhr.status, data.error ?? 'internal', data.field, data.message))
    }
    xhr.onerror = () => reject(new ApiError(0, 'upload_interrupted'))
    xhr.send(file)
  })
}

function NoteList({ notes, tone = 'warn' }: { notes: Note[]; tone?: 'warn' | 'ok' }) {
  const { t } = useTranslation()
  if (!notes.length) return null
  return (
    <Alert tone={tone}>
      <ul className="flex list-disc flex-col gap-1 pl-4">
        {notes.map((n, i) => <li key={i}>{t(`import.notes.${n.code}`, { ...n.args, defaultValue: n.code })}</li>)}
      </ul>
    </Alert>
  )
}

function Check({ checked, onChange, label, disabled }: { checked: boolean; onChange: (v: boolean) => void; label: string; disabled?: boolean }) {
  return <input type="checkbox" aria-label={label} checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} className="size-4 accent-[var(--color-accent)]" />
}

function SectionCard<T extends { key: string; existingId?: number }>({ id, icon, rows, head, row, picked, setPicked }: {
id: Section; icon: ReactNode; rows: T[]; head: string[]; row: (r: T) => ReactNode[]
picked: Record<string, boolean>; setPicked: (f: (p: Record<string, boolean>) => Record<string, boolean>) => void
}) {
const { t } = useTranslation()
const setAll = (keys: string[], v: boolean) => setPicked((p) => ({ ...p, ...Object.fromEntries(keys.map((k) => [k, v])) }))
  if (!rows.length) return null
  const keys = rows.map((r) => r.key)
  const n = keys.filter((k) => picked[k]).length
  return (
    <Card>
      <SectionTitle action={<div className="flex items-center gap-3 text-sm text-muted">
        <span>{t('import.selected', { n, total: rows.length })}</span>
        <button type="button" className="text-accent hover:underline" onClick={() => setAll(keys, n !== rows.length)}>{n === rows.length ? t('import.none') : t('import.all')}</button>
      </div>}>
        <span className="inline-flex items-center gap-2">{icon}{t(`import.sections.${id}`)}</span>
      </SectionTitle>
      <Table head={['', ...head]}>
        {rows.map((r) => (
          <tr key={r.key} className={picked[r.key] ? '' : 'opacity-55'}>
            <td className={`${td} w-8`}><Check label={r.key} checked={!!picked[r.key]} onChange={(v) => setPicked((p) => ({ ...p, [r.key]: v }))} /></td>
            {row(r).map((c, i) => <td key={i} className={td}>{c}</td>)}
          </tr>
        ))}
      </Table>
    </Card>
  )
}


export function Import() {
  const { t, i18n } = useTranslation()
  const systems = useApi<{ systems: string[] }>('/import/systems')
  const [system, setSystem] = useState('auto')
  const [files, setFiles] = useState<File[]>([])
  const [progress, setProgress] = useState<string>('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [session, setSession] = useState('')
  const [plan, setPlan] = useState<Plan | null>(null)
  const [picked, setPicked] = useState<Record<string, boolean>>({})
  const [numberOf, setNumberOf] = useState<Record<string, string>>({})
  const [result, setResult] = useState<Result | null>(null)
  const [failed, setFailed] = useState<{ key: string; field: string; code: string } | null>(null)

  const offered = SYSTEMS.filter((s) => s === 'auto' || systems.data?.systems.includes(s))

  async function read() {
    setBusy(true)
    setError(null)
    setPlan(null)
    setResult(null)
    try {
      let id = ''
      const total = files.reduce((n, f) => n + f.size, 0)
      let done = 0
      for (const f of files) {
        const url = `/api/import/upload?name=${encodeURIComponent(f.name)}${id ? `&session=${id}` : ''}`
        const res = await uploadFile(url, f, (sent) => setProgress(t('import.uploading', { pct: Math.min(100, Math.floor(((done + sent) / Math.max(1, total)) * 100)) })))
        done += f.size
        id = res.id
      }
      setProgress(t('import.reading'))
      setSession(id)
      const r = await api<{ plan: Plan }>('POST', `/import/${id}/read`, { system })
      const p = r.plan
      const sel: Record<string, boolean> = {}
      const all = [...p.extensions, ...p.ringGroups, ...p.queues, ...p.trunks, ...p.timeConditions, ...p.menus]
      for (const x of all) sel[x.key] = true
      for (const n of p.numbers) sel[n.key] = !n.exists
      setPicked(sel)
      setNumberOf({})
      setPlan(p)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
      setProgress('')
    }
  }

  // Labels for the plan keys (u:…, g:…, x:…).
  const labels = useMemo(() => {
    const m: Record<string, string> = {}
    if (!plan) return m
    for (const e of plan.extensions) {
      m[e.key] = `${numberOf[e.key] || e.number} ${e.name}`
      m['v:' + e.key.slice(2)] = t('import.voicemailOf', { name: e.name })
    }
    for (const g of plan.ringGroups) m[g.key] = `${numberOf[g.key] || g.number} ${g.name}`
    for (const q of plan.queues) m[q.key] = `${numberOf[q.key] || q.number} ${q.name}`
    for (const v of plan.menus) m[v.key] = v.name
    for (const c of plan.timeConditions) m[c.key] = c.name
    return m
  }, [plan, numberOf, t])
  const label = (key?: string) => {
    if (!key) return t('import.hangup')
    if (key.startsWith('x:')) return key.slice(2)
    return labels[key] ?? key
  }

  function setAll(keys: string[], v: boolean) {
    setPicked((p) => ({ ...p, ...Object.fromEntries(keys.map((k) => [k, v])) }))
  }

  async function apply() {
    if (!plan) return
    setBusy(true)
    setError(null)
    setFailed(null)
    try {
      // The recordings of what is ticked become prompts, converted in the browser like any upload.
      const wanted = plan.audio.filter((a) => a.playable && a.usedBy.some((k) => k === 'greeting' || picked[k]))
      const prompts: Record<string, number> = {}
      let i = 0
      for (const a of wanted) {
        i++
        setProgress(t('import.converting', { n: i, total: wanted.length }))
        try {
          const res = await fetch(`/api/import/${session}/audio?file=${encodeURIComponent(a.file)}`, { credentials: 'same-origin' })
          if (!res.ok) continue
          const wav = await toTelephoneWav(await res.blob())
          const p = await api<{ id: number }>('POST', '/pbx/prompts', { name: a.name, audio: wav.base64 })
          prompts[a.file] = p.id
        } catch {
          /* a recording the browser cannot decode is skipped; Apply warns about it */
        }
      }
      setProgress(t('import.importing'))
      const keys = (rows: { key: string }[]) => rows.filter((r) => picked[r.key]).map((r) => r.key)
      const choice = {
        extensions: keys(plan.extensions), ringGroups: keys(plan.ringGroups), queues: keys(plan.queues), trunks: keys(plan.trunks),
        timeConditions: keys(plan.timeConditions), menus: keys(plan.menus), numbers: keys(plan.numbers), numberOf, prompts,
      }
      const res = await fetch(`/api/import/${session}/apply`, {
        method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(choice),
      })
      const data = await res.json().catch(() => ({}))
      if (!res.ok) {
        if (data.error === 'import_item') setFailed({ key: data.key, field: data.field, code: data.code })
        if (data.result) setResult(data.result)
        throw new ApiError(res.status, data.error ?? 'internal', data.field, data.message)
      }
      setResult(data as Result)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
      setProgress('')
    }
  }

  function restart() {
    if (session) api('DELETE', `/import/${session}`).catch(() => {})
    setSession('')
    setPlan(null)
    setResult(null)
    setFiles([])
    setError(null)
    setFailed(null)
  }

  const numberInput = (key: string, planned: string, existing?: number) => existing
    ? <span className="font-mono">{planned}</span>
    : <input aria-label={t('import.newNumber')} value={numberOf[key] ?? planned} onChange={(e) => setNumberOf((m) => ({ ...m, [key]: e.target.value.replace(/\D/g, '') }))}
        className="w-24 rounded-lg border border-line bg-field px-2 py-1 font-mono text-[13px] text-ink focus:outline-none focus:ring-2 focus:ring-accent/30" inputMode="numeric" />
  const existing = (id?: number) => id ? <Badge tone="accent">{t('import.exists')}</Badge> : null

  const days = (d: number[]) => d.map((x) => t(`import.days.${x}`)).join(' ')

  function StepTree({ s, depth = 0 }: { s: Step; depth?: number }) {
    return (
      <li className="flex flex-col gap-1">
        <div className="flex flex-wrap items-center gap-2 text-[13px]">
          {s.key && <span className="inline-flex size-6 items-center justify-center rounded-md border border-accent/40 bg-accent/10 font-mono text-accent-ink">{s.key}</span>}
          <span className="text-ink-2">{depth === 0 ? t('import.welcome') : s.title || s.key}</span>
          {s.audio && <Badge tone="muted"><AudioLines className="size-3" aria-hidden />{s.audio.split('/').pop()}</Badge>}
          {s.target && <span className="text-muted">→ {label(s.target)}</span>}
          {s.noAction && <span className="text-subtle">({t('import.noChoice')}: {label(s.noAction)})</span>}
        </div>
        {!!s.children?.length && <ul className="ml-4 flex flex-col gap-1 border-l border-line pl-3">{s.children.map((c) => <StepTree key={c.id + (c.key ?? '')} s={c} depth={depth + 1} />)}</ul>}
      </li>
    )
  }

  if (result) {
    const counts: Record<string, number> = {}
    for (const c of result.created) counts[c.kind] = (counts[c.kind] ?? 0) + 1
    return (
      <div className="flex flex-col gap-6">
        <PageHeader title={t('import.title')} subtitle={t('import.doneSubtitle')} />
        <ErrorMessage error={error} />
        {failed && <Alert>{t('import.failedItem', { item: label(failed.key) || failed.key, field: failed.field, code: t(`errors.${failed.code}`, { defaultValue: failed.code }) })}</Alert>}
        <Card>
          <SectionTitle><span className="inline-flex items-center gap-2"><CheckCircle2 className="size-5 text-ok" aria-hidden />{t('import.doneTitle')}</span></SectionTitle>
          <div className="mb-4 flex flex-wrap gap-2">
            {Object.entries(counts).map(([k, n]) => <Badge key={k} tone="ok">{t(`import.kinds.${k}`, { count: n })}</Badge>)}
          </div>
          <Table head={[t('import.kind'), t('import.name'), t('import.number'), '']}>
            {result.created.map((c) => (
              <tr key={c.kind + c.key}>
                <td className={td}>{t(`import.kind_${c.kind}`)}</td>
                <td className={td}>{c.name}</td>
                <td className={`${td} font-mono`}>{c.number}</td>
                <td className={td}>{c.reused && <Badge tone="muted">{t('import.reused')}</Badge>}</td>
              </tr>
            ))}
          </Table>
        </Card>
        <NoteList notes={result.warnings} />
        <div className="flex flex-wrap gap-2.5">
          <Link to="/ivr"><Button variant="secondary"><GitBranch className="size-4" aria-hidden />{t('import.openIvr')}</Button></Link>
          <Link to="/extensions"><Button variant="ghost"><User className="size-4" aria-hidden />{t('import.openExtensions')}</Button></Link>
          <Button variant="ghost" onClick={restart}>{t('import.another')}</Button>
        </div>
      </div>
    )
  }

  if (plan) {
    const src = plan.source
    const total = Object.values(picked).filter(Boolean).length
    const audioUsed = plan.audio.filter((a) => a.playable && a.usedBy.some((k) => k === 'greeting' || picked[k])).length
    return (
      <div className="flex flex-col gap-6">
        <PageHeader title={t('import.title')} subtitle={t('import.previewSubtitle')}>
          <Button variant="ghost" onClick={restart}><ArrowLeft className="size-4" aria-hidden />{t('import.back')}</Button>
          <Button busy={busy} disabled={!total} onClick={apply}><ImportIcon className="size-4" aria-hidden />{t('import.importN', { n: total })}</Button>
        </PageHeader>
        <Card>
          <div className="flex flex-wrap items-center gap-x-8 gap-y-2 text-sm">
            <span className="inline-flex items-center gap-2 font-medium text-ink"><Info className="size-4 text-accent" aria-hidden />{t(`import.systems.${src.system}`, { defaultValue: src.system })}</span>
            {src.product && <span className="text-muted">{src.product}</span>}
            {src.host && <span className="text-muted">{src.host}</span>}
            {!!src.createdAt && <span className="text-muted">{t('import.madeOn', { date: new Date(src.createdAt).toLocaleString(i18n.language) })}</span>}
            {Object.entries(src.details ?? {}).map(([k, v]) => <span key={k} className="text-subtle">{k}: {v}</span>)}
          </div>
        </Card>
        {progress && <Alert tone="ok">{progress}</Alert>}
        <ErrorMessage error={error} />
        {failed && <Alert>{t('import.failedItem', { item: label(failed.key) || failed.key, field: failed.field, code: t(`errors.${failed.code}`, { defaultValue: failed.code }) })}</Alert>}
        <NoteList notes={plan.notes} />
        <SectionCard picked={picked} setPicked={setPicked} id="extensions" icon={<User className="size-5 text-accent" aria-hidden />} rows={plan.extensions}
          head={[t('import.oldNumber'), t('import.name'), t('import.newNumber'), '']}
          row={(e) => [<span className="font-mono text-muted">{e.sourceNumber}</span>, <span>{e.name}{e.email && <span className="block text-xs text-subtle">{e.email}</span>}</span>,
            numberInput(e.key, e.number, e.existingId),
            <span className="flex flex-wrap gap-1.5">{existing(e.existingId)}{e.voicemail && <Badge tone="muted">{t('import.voicemail')}</Badge>}{e.keepPassword ? <Badge tone="ok">{t('import.keepsPassword')}</Badge> : <Badge tone="warn">{t('import.newPassword')}</Badge>}</span>]} />
        <SectionCard picked={picked} setPicked={setPicked} id="ringGroups" icon={<UsersRound className="size-5 text-accent" aria-hidden />} rows={plan.ringGroups}
          head={[t('import.oldNumber'), t('import.name'), t('import.newNumber'), t('import.members'), t('import.failover')]}
          row={(g) => [<span className="font-mono text-muted">{g.sourceNumber}</span>, <span>{g.name} {existing(g.existingId)}</span>, numberInput(g.key, g.number, g.existingId),
            <span className="text-muted">{g.members.map(label).join(', ')}</span>, <span className="text-muted">{label(g.failover)}</span>]} />
        <SectionCard picked={picked} setPicked={setPicked} id="queues" icon={<ListOrdered className="size-5 text-accent" aria-hidden />} rows={plan.queues}
          head={[t('import.oldNumber'), t('import.name'), t('import.newNumber'), t('import.agents'), t('import.overflow')]}
          row={(q) => [<span className="font-mono text-muted">{q.sourceNumber}</span>, <span>{q.name} {existing(q.existingId)}</span>, numberInput(q.key, q.number, q.existingId),
            <span className="text-muted">{q.agents.map(label).join(', ')}</span>, <span className="text-muted">{label(q.overflow)}</span>]} />
        <SectionCard picked={picked} setPicked={setPicked} id="timeConditions" icon={<Clock className="size-5 text-accent" aria-hidden />} rows={plan.timeConditions}
          head={[t('import.name'), t('import.hours'), t('import.open'), t('import.closed')]}
          row={(c) => [<span>{c.name} {existing(c.existingId)}</span>,
            <span className="text-muted">{c.schedule.length ? c.schedule.map((s) => `${days(s.days)} ${s.from}–${s.to}`).join('; ') : t('import.noHours')}</span>,
            <span className="text-muted">{label(c.open)}</span>, <span className="text-muted">{label(c.closed)}</span>]} />
        {plan.menus.length > 0 && (
          <Card>
            <SectionTitle action={<button type="button" className="text-sm text-accent hover:underline" onClick={() => setAll(plan.menus.map((m) => m.key), !plan.menus.every((m) => picked[m.key]))}>
              {plan.menus.every((m) => picked[m.key]) ? t('import.none') : t('import.all')}</button>}>
              <span className="inline-flex items-center gap-2"><GitBranch className="size-5 text-accent" aria-hidden />{t('import.sections.menus')}</span>
            </SectionTitle>
            <div className="flex flex-col gap-4">
              {plan.menus.map((m) => (
                <div key={m.key} className={`rounded-xl border border-line p-4 ${picked[m.key] ? '' : 'opacity-55'}`}>
                  <label className="mb-3 flex flex-wrap items-center gap-2.5">
                    <Check label={m.name} checked={!!picked[m.key]} onChange={(v) => setPicked((p) => ({ ...p, [m.key]: v }))} />
                    <span className="font-medium text-ink">{m.name}</span>
                    {m.sourceNumber && <span className="font-mono text-xs text-muted">{m.sourceNumber}</span>}
                    {m.numbers.map((n) => <Badge key={n} tone="muted"><Phone className="size-3" aria-hidden />{n}</Badge>)}
                    {existing(m.existingId)}
                  </label>
                  <ul className="flex flex-col gap-1"><StepTree s={m.steps} /></ul>
                </div>
              ))}
            </div>
          </Card>
        )}
        <SectionCard picked={picked} setPicked={setPicked} id="trunks" icon={<Waypoints className="size-5 text-accent" aria-hidden />} rows={plan.trunks}
          head={[t('import.name'), t('import.host'), t('import.username'), t('import.provider'), t('import.numbers')]}
          row={(tr) => [<span>{tr.name} {existing(tr.existingId)}</span>, <span className="font-mono text-muted">{tr.host}</span>,
            <span className="font-mono text-muted">{tr.username}{!tr.hasSecret && tr.register && <Badge tone="warn">{t('import.noPassword')}</Badge>}</span>,
            <span className="text-muted">{tr.provider}</span>, <span className="font-mono text-xs text-muted">{tr.numbers.join(', ')}</span>]} />
        <SectionCard picked={picked} setPicked={setPicked} id="numbers" icon={<Phone className="size-5 text-accent" aria-hidden />} rows={plan.numbers}
          head={[t('import.number'), t('import.goesTo'), '']}
          row={(n) => [<span className="font-mono">{n.number}</span>, <span className="text-muted">{label(n.target)}</span>,
            n.exists ? <Badge tone="accent">{t('import.alreadyRouted')}</Badge> : null]} />
        {plan.audio.length > 0 && (
          <Card>
            <SectionTitle><span className="inline-flex items-center gap-2"><AudioLines className="size-5 text-accent" aria-hidden />{t('import.sections.audio')}</span></SectionTitle>
            <p className="mb-3 text-sm text-muted">{t('import.audioIntro', { n: audioUsed })}</p>
            <ul className="grid gap-1.5 text-[13px] sm:grid-cols-2">
              {plan.audio.map((a) => (
                <li key={a.file} className="flex items-center justify-between gap-3 rounded-lg border border-line px-3 py-2">
                  <span className="min-w-0 truncate">{a.name}</span>
                  {a.playable ? <span className="text-xs text-subtle">{Math.round(a.bytes / 1024)} KB</span> : <Badge tone="warn">{t('import.unsupportedAudio')}</Badge>}
                </li>
              ))}
            </ul>
          </Card>
        )}
        <div className="flex justify-end">
          <Button busy={busy} disabled={!total} onClick={apply}><ImportIcon className="size-4" aria-hidden />{t('import.importN', { n: total })}</Button>
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t('import.title')} subtitle={t('import.subtitle')} />
      <Card>
        <SectionTitle><span className="inline-flex items-center gap-2"><FileUp className="size-5 text-accent" aria-hidden />{t('import.uploadTitle')}</span></SectionTitle>
        <div className="flex flex-col gap-4">
          <p className="max-w-3xl text-sm text-muted">{t('import.intro')}</p>
          <div className="grid gap-4 md:grid-cols-[260px_1fr]">
            <Select label={t('import.system')} value={system} onChange={(e) => setSystem(e.target.value)}>
              {offered.map((s) => <option key={s} value={s}>{t(`import.systems.${s}`, { defaultValue: s })}</option>)}
            </Select>
            <div className="flex flex-col gap-1.5">
              <span className="text-sm font-medium text-ink-2">{t('import.files')}</span>
              <label
                className="flex min-h-24 cursor-pointer flex-col items-center justify-center gap-1.5 rounded-[12px] border border-dashed border-line bg-field px-4 py-5 text-center text-sm text-muted hover:border-accent/50 focus-within:ring-2 focus-within:ring-accent/30"
                onDragOver={(e) => e.preventDefault()}
                onDrop={(e) => { e.preventDefault(); setFiles(Array.from(e.dataTransfer.files)) }}>
                {files.length ? <FileArchive className="size-6 text-accent" aria-hidden /> : <Upload className="size-6 text-accent" aria-hidden />}
                <span className="text-ink-2">{files.length ? files.map((f) => f.name).join(', ') : t('import.choose')}</span>
                <span className="text-xs text-subtle">{t('import.chooseHint')}</span>
                <input type="file" multiple className="sr-only" onChange={(e) => setFiles(Array.from(e.target.files ?? []))} />
              </label>
            </div>
          </div>
          <div className="grid gap-2 text-[13px] text-muted md:grid-cols-2">
            {['asterisk', 'freepbx', 'freeswitch', '3cx', 'unifi-talk'].filter((s) => offered.includes(s)).map((s) => (
              <div key={s} className="rounded-lg border border-line px-3 py-2"><span className="font-medium text-ink-2">{t(`import.systems.${s}`)}</span> · {t(`import.howto.${s}`)}</div>
            ))}
          </div>
          {progress && <Alert tone="ok">{progress}</Alert>}
          <ErrorMessage error={error} />
          <div className="flex justify-end">
            <Button busy={busy} disabled={!files.length} onClick={read}><FileUp className="size-4" aria-hidden />{t('import.read')}</Button>
          </div>
        </div>
      </Card>
      <Card>
        <div className="flex gap-3 text-sm text-muted"><Info className="size-5 flex-none text-accent" aria-hidden /><p>{t('import.safety')}</p></div>
      </Card>
    </div>
  )
}

