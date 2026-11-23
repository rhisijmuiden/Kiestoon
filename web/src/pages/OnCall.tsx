import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { AlertTriangle, ArrowDown, ArrowUp, CalendarPlus, Check, ChevronLeft, ChevronRight, Copy, Link2, Plus, RefreshCw, Siren, Smartphone, Trash2, X } from 'lucide-react'
import { Badge, Empty, Modal, Select, Table, Tabs, Toggle, td, useApi } from '../components/kit'
import { DestinationPicker, IconButton, Segmented, nextNumber, resetDestinations, useSaver } from '../components/pbx'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { Avatar, OnCallTimeline, fmtRange, fmtWhen, fromLocalInput, toLocalInput } from '../components/oncall'
import { api } from '../lib/api'
import type { OnCallShift, OnCallStep, OnCallTeam, OnCallUser, OnCallView, Prompt } from '../lib/types'

const today = () => new Date().toISOString().slice(0, 10)

export const blankTeam = (): OnCallTeam => ({
  id: 0, number: '', name: '', enabled: true, mode: 'weekly', handoverDay: 0, handoverTime: '08:00', startDate: today(),
  members: [], backup: 'next', backupUser: 0, leadUser: 0,
  steps: [{ kind: 'primary', target: '', seconds: 25 }, { kind: 'primary', target: '', seconds: 20 }, { kind: 'backup', target: '', seconds: 25 }, { kind: 'lead', target: '', seconds: 30 }],
  repeat: 0, confirm: true, ringExtension: true, music: true, greetingPrompt: 0, cidPrefix: '', final: { type: 'hangup', target: '' },
  swapApproval: 'lead', reminders: true,
})

type Tab = 'overview' | 'schedule' | 'escalation' | 'changes'

export function OnCall() {
  const { t } = useTranslation()
  const teams = useApi<OnCallTeam[]>('/oncall/teams')
  const [sel, setSel] = useState(0)
  const [tab, setTab] = useState<Tab>('overview')
  const [adding, setAdding] = useState(false)
  const team = teams.data?.find((x) => x.id === sel) ?? teams.data?.[0]
  useEffect(() => { if (team && team.id !== sel) setSel(team.id) }, [team, sel])

  return (
    <>
      <PageHeader title={t('onCall.title')} subtitle={t('onCall.subtitle')}>
        {teams.data && teams.data.length > 1 && (
          <div className="min-w-56">
            <Select label={t('onCall.team')} value={team?.id ?? 0} onChange={(e) => setSel(Number(e.target.value))}>
              {teams.data.map((x) => <option key={x.id} value={x.id}>{x.number} {x.name}</option>)}
            </Select>
          </div>
        )}
        <Button variant="ghost" onClick={() => setAdding(true)}><Plus className="size-4" aria-hidden /> {t('onCall.add')}</Button>
      </PageHeader>
      <ErrorMessage error={teams.error} />
      {teams.data?.length === 0 && (
        <Card><Empty icon={<Siren className="size-7" />} title={t('onCall.emptyTitle')} text={t('onCall.emptyText')}
          action={<Button onClick={() => setAdding(true)}><Plus className="size-4" aria-hidden /> {t('onCall.add')}</Button>} /></Card>
      )}
      {team && (
        <>
          <Tabs value={tab} onChange={setTab} tabs={(['overview', 'schedule', 'escalation', 'changes'] as const).map((k) => ({ key: k, label: t(`onCall.tabs.${k}`) }))} />
          {tab === 'overview' && <Overview key={team.id} team={team} />}
          {(tab === 'schedule' || tab === 'escalation') && (
            <TeamEditor key={`${team.id}-${tab}`} value={team} part={tab}
              onSaved={() => { resetDestinations(); teams.reload() }}
              onDeleted={() => { resetDestinations(); setSel(0); setTab('overview'); teams.reload() }} />
          )}
          {tab === 'changes' && <Changes key={team.id} team={team} />}
        </>
      )}
      {adding && <NewTeam existing={teams.data ?? []} onClose={() => setAdding(false)} onSaved={(x) => { setAdding(false); resetDestinations(); teams.reload(); setSel(x.id); setTab('schedule') }} />}
    </>
  )
}

function NewTeam({ existing, onClose, onSaved }: { existing: OnCallTeam[]; onClose: () => void; onSaved: (t: OnCallTeam) => void }) {
  const { t } = useTranslation()
  const [v, setV] = useState<OnCallTeam>({ ...blankTeam(), mode: 'manual' })
  const { busy, error, save } = useSaver<OnCallTeam>('/oncall/teams', onSaved)
  useEffect(() => {
    api<{ number: string }[]>('GET', '/pbx/ringgroups').catch(() => [])
      .then((rg) => nextNumber('ringGroups', existing.map((x) => x.number).concat(rg.map((g) => g.number))))
      .then((n) => setV((x) => ({ ...x, number: x.number || n }))).catch(() => undefined)
  }, [existing])
  return (
    <Modal title={t('onCall.add')} subtitle={t('onCall.addHint')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={() => save(v)}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <div className="grid gap-4 sm:grid-cols-[140px_1fr]">
        <Input name="number" label={t('common.number')} value={v.number} onChange={(e) => setV({ ...v, number: e.target.value.replace(/\D/g, '') })} className="font-mono" inputMode="numeric" />
        <Input name="name" label={t('common.name')} value={v.name} onChange={(e) => setV({ ...v, name: e.target.value })} placeholder={t('onCall.namePlaceholder')} autoFocus />
      </div>
    </Modal>
  )
}

type Timeline = { view: OnCallView; users: OnCallUser[]; now: string }

function useTimeline(team: OnCallTeam, from: Date, days: number) {
  const ymd = `${from.getFullYear()}-${String(from.getMonth() + 1).padStart(2, '0')}-${String(from.getDate()).padStart(2, '0')}`
  return useApi<Timeline>(`/oncall/teams/${team.id}/timeline?from=${ymd}&days=${days}`, 60000)
}

function startOfToday() {
  const d = new Date()
  d.setHours(0, 0, 0, 0)
  return d
}

function Overview({ team }: { team: OnCallTeam }) {
  const { t, i18n } = useTranslation()
  const [from, setFrom] = useState(startOfToday)
  const [override, setOverride] = useState(false)
  const days = 14
  const tl = useTimeline(team, from, days)
  const users = useMemo(() => new Map((tl.data?.users ?? []).map((u) => [u.id, u])), [tl.data])
  const v = tl.data?.view
  const p = v ? users.get(v.primary) : undefined
  const b = v ? users.get(v.backup) : undefined
  const pending = (v?.shifts ?? []).filter((s) => s.status === 'pending' && !(s.kind === 'swap' && s.pairId && s.requestedBy === s.userId))
  const shift = (d: number) => setFrom((f) => { const n = new Date(f); n.setDate(n.getDate() + d); return n })
  async function decide(s: OnCallShift, approve: boolean) {
    await api('POST', `/oncall/shifts/${s.id}/decide`, { approve })
    tl.reload()
  }
  return (
    <>
      <ErrorMessage error={tl.error} />
      {!team.enabled && <Card className="p-4"><span className="text-sm text-warn-ink">{t('onCall.disabled')}</span></Card>}
      <Card className="flex flex-wrap items-center gap-6 p-5">
        <div className="flex min-w-60 flex-1 items-center gap-4">
          <Avatar user={p} size="lg" />
          <div className="min-w-0">
            <div className="text-[11px] uppercase tracking-[0.1em] text-subtle">{t('onCall.nowOnDuty')}</div>
            <div className="font-display text-2xl font-semibold">{p?.name ?? t('onCall.nobody')}</div>
            {p && <MobileLine u={p} />}
          </div>
        </div>
        <div className="min-w-44">
          <div className="text-[11px] uppercase tracking-[0.1em] text-subtle">{t('onCall.backupNow')}</div>
          <div className="mt-1 flex items-center gap-2"><Avatar user={b} /><span className="text-ink">{b?.name ?? '–'}</span></div>
        </div>
        <div className="min-w-40">
          <div className="text-[11px] uppercase tracking-[0.1em] text-subtle">{t('onCall.until')}</div>
          <div className="font-display text-xl font-semibold">{v?.until ? fmtWhen(v.until, i18n.language) : '–'}</div>
        </div>
        <Button onClick={() => setOverride(true)}><CalendarPlus className="size-4" aria-hidden /> {t('onCall.override')}</Button>
      </Card>
      <Card className="p-5">
        <div className="mb-3 flex flex-wrap items-center gap-3">
          <h2 className="font-display text-lg font-semibold">{t('onCall.nextDays', { n: days })}</h2>
          <span className="text-xs text-muted">{t(`onCall.modeHint.${team.mode}`, { day: t(`onCall.days.${team.handoverDay}`), time: team.handoverTime })}</span>
          <span className="flex-1" />
          <IconButton label={t('onCall.earlier')} onClick={() => shift(-7)}><ChevronLeft className="size-4" aria-hidden /></IconButton>
          <Button variant="ghost" onClick={() => setFrom(startOfToday())}>{t('onCall.today')}</Button>
          <IconButton label={t('onCall.later')} onClick={() => shift(7)}><ChevronRight className="size-4" aria-hidden /></IconButton>
        </div>
        {v && <OnCallTimeline segments={v.segments} users={users} from={from} days={days} now={new Date(tl.data!.now)} showBackup={team.backup !== 'none'} />}
      </Card>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card className="p-5">
          <h2 className="mb-2 font-display text-lg font-semibold">{t('onCall.requests')} {pending.length > 0 && <Badge tone="warn">{pending.length}</Badge>}</h2>
          {pending.length === 0 && <p className="text-sm text-muted">{t('onCall.noRequests')}</p>}
          <ul className="flex flex-col">
            {pending.map((s) => (
              <li key={s.id} className="flex flex-wrap items-center gap-3 border-t border-line py-3 first:border-t-0">
                <Avatar user={users.get(s.userId)} />
                <div className="min-w-[200px] flex-1">
                  <div className="text-sm text-ink">{requestTitle(t, s, users)}</div>
                  <div className="text-xs text-muted">{fmtRange(s.start, s.end, i18n.language)} · {t(`onCall.line${s.line}`)}</div>
                  {s.note && <div className="text-xs italic text-subtle">“{s.note}”</div>}
                </div>
                <div className="ml-auto flex items-center gap-2">
                  {s.accepted ? <Badge tone="accent">{t('onCall.accepted')}</Badge> : <Badge tone="warn">{t('onCall.waitingFor', { name: users.get(targetOf(s))?.name ?? '' })}</Badge>}
                  <IconButton label={t('onCall.decline')} danger onClick={() => decide(s, false)}><X className="size-4" aria-hidden /></IconButton>
                  <Button onClick={() => decide(s, true)}><Check className="size-4" aria-hidden /> {t('onCall.approve')}</Button>
                </div>
              </li>
            ))}
          </ul>
        </Card>
        <Card className="p-5">
          <h2 className="mb-2 font-display text-lg font-semibold">{t('onCall.attention')}</h2>
          {v && v.warnings.length === 0 && <p className="text-sm text-muted">{t('onCall.allGood')}</p>}
          <div className="flex flex-col gap-2">
            {v?.warnings.map((w, i) => (
              <div key={i} className="flex gap-2.5 rounded-xl border border-warn/40 bg-warn/[0.07] px-3 py-2.5 text-sm text-ink-2">
                {w.kind === 'gap' ? <AlertTriangle className="mt-0.5 size-4 flex-none text-warn" aria-hidden /> : <Smartphone className="mt-0.5 size-4 flex-none text-warn" aria-hidden />}
                <span>{w.kind === 'gap'
                  ? t('onCall.warnGap', { line: t(`onCall.line${w.line}`), when: fmtRange(w.start!, w.end!, i18n.language) })
                  : t('onCall.warnMobile', { name: users.get(w.userId!)?.name ?? '' })}</span>
              </div>
            ))}
          </div>
        </Card>
      </div>
      {override && v && <OverrideForm team={team} users={tl.data!.users} onClose={() => setOverride(false)} onSaved={() => { setOverride(false); tl.reload() }} />}
    </>
  )
}

/** The colleague who is asked: the one who takes over (for a swap, the other side of the first half). */
export const targetOf = (s: OnCallShift) => (s.kind === 'swap' && s.requestedBy === s.userId ? s.fromUser : s.userId)

export function requestTitle(t: TFunction, s: OnCallShift, users: Map<number, OnCallUser>) {
  const name = (id: number) => users.get(id)?.name ?? '?'
  if (s.kind === 'swap') return t('onCall.reqSwap', { from: name(s.fromUser), to: name(s.userId) })
  if (s.kind === 'takeover') return t('onCall.reqTakeover', { to: name(s.userId), from: name(s.fromUser) })
  if (s.kind === 'handover') return t('onCall.reqHandover', { to: name(s.userId), from: name(s.fromUser) })
  return t('onCall.reqShift', { to: name(s.userId) })
}

function MobileLine({ u }: { u: OnCallUser }) {
  const { t } = useTranslation()
  return (
    <div className="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-muted">
      <Smartphone className="size-3.5" aria-hidden />
      {u.mobile ? <span className="font-mono">{u.mobile}</span> : <Badge tone="warn">{t('onCall.noMobile')}</Badge>}
      {u.extension && <Badge tone="accent">{t('onCall.plusExt', { ext: u.extension })}</Badge>}
    </div>
  )
}

function OverrideForm({ team, users, onClose, onSaved }: { team: OnCallTeam; users: OnCallUser[]; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const now = new Date()
  now.setMinutes(0, 0, 0)
  const later = new Date(now.getTime() + 12 * 3600000)
  const people = users.filter((u) => team.members.includes(u.id) || u.id === team.leadUser || u.id === team.backupUser)
  const [v, setV] = useState({ line: 1, userId: people[0]?.id ?? 0, start: toLocalInput(now.toISOString()), end: toLocalInput(later.toISOString()), kind: team.mode === 'manual' ? 'shift' : 'override', note: '' })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api('POST', `/oncall/teams/${team.id}/shifts`, { ...v, start: fromLocalInput(v.start), end: fromLocalInput(v.end) })
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={t('onCall.override')} subtitle={t('onCall.overrideHint')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <Segmented label={t('onCall.lineLabel')} value={String(v.line)} onChange={(x) => setV({ ...v, line: Number(x) })}
        options={[{ key: '1', label: t('onCall.line1') }, { key: '2', label: t('onCall.line2') }]} />
      <Select name="userId" label={t('onCall.engineer')} value={v.userId} onChange={(e) => setV({ ...v, userId: Number(e.target.value) })}>
        {people.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
      </Select>
      <div className="grid gap-4 sm:grid-cols-2">
        <Input name="start" type="datetime-local" label={t('onCall.from')} value={v.start} onChange={(e) => setV({ ...v, start: e.target.value })} />
        <Input name="end" type="datetime-local" label={t('onCall.to')} value={v.end} onChange={(e) => setV({ ...v, end: e.target.value })} />
      </div>
      <Input name="note" label={t('onCall.note')} value={v.note} onChange={(e) => setV({ ...v, note: e.target.value })} />
    </Modal>
  )
}

function TeamEditor({ value, part, onSaved, onDeleted }: { value: OnCallTeam; part: 'schedule' | 'escalation'; onSaved: () => void; onDeleted: () => void }) {
  const { t } = useTranslation()
  const [v, setV] = useState(value)
  const [ok, setOk] = useState(false)
  const users = useApi<OnCallUser[]>('/oncall/engineers')
  const { busy, error, save } = useSaver<OnCallTeam>('/oncall/teams', (x) => { setV(x); setOk(true); onSaved() })
  const set = <K extends keyof OnCallTeam>(k: K, x: OnCallTeam[K]) => { setOk(false); setV((o) => ({ ...o, [k]: x })) }
  async function remove() {
    if (!window.confirm(t('common.confirmDelete', { name: v.name }))) return
    await api('DELETE', `/oncall/teams/${v.id}`)
    onDeleted()
  }
  return (
    <Card className="flex flex-col gap-5 p-5">
      <ErrorMessage error={error} />
      {part === 'schedule'
        ? <ScheduleForm v={v} set={set} users={users.data ?? []} reloadUsers={users.reload} />
        : <EscalationForm v={v} set={set} users={users.data ?? []} />}
      <div className="flex flex-wrap items-center gap-2.5 border-t border-line pt-4">
        {ok && <Badge tone="ok"><Check className="size-3" aria-hidden /> {t('common.saved')}</Badge>}
        <span className="flex-1" />
        {part === 'schedule' && <Button variant="ghost" onClick={remove}><Trash2 className="size-4" aria-hidden /> {t('common.delete')}</Button>}
        <Button busy={busy} onClick={() => save(v)}>{t('common.save')}</Button>
      </div>
    </Card>
  )
}

type SetFn = <K extends keyof OnCallTeam>(k: K, x: OnCallTeam[K]) => void

function ScheduleForm({ v, set, users, reloadUsers }: { v: OnCallTeam; set: SetFn; users: OnCallUser[]; reloadUsers: () => void }) {
  const { t } = useTranslation()
  const byId = new Map(users.map((u) => [u.id, u]))
  const [pick, setPick] = useState(0)
  const [mobile, setMobile] = useState<{ id: number; value: string } | null>(null)
  const [copied, setCopied] = useState(false)
  const [token, setToken] = useState(v.icalToken ?? '')
  const move = (i: number, d: number) => {
    const m = [...v.members]
    const j = i + d
    if (j < 0 || j >= m.length) return
    ;[m[i], m[j]] = [m[j], m[i]]
    set('members', m)
  }
  const ical = token ? `${window.location.origin}/api/oncall/ical/${token}.ics` : ''
  async function saveMobile() {
    if (!mobile) return
    await api('PUT', `/oncall/engineers/${mobile.id}/mobile`, { mobile: mobile.value })
    setMobile(null)
    reloadUsers()
  }
  async function newToken() {
    const r = await api<{ icalToken: string }>('POST', `/oncall/teams/${v.id}/ical`)
    setToken(r.icalToken)
  }
  return (
    <>
      <div className="grid gap-4 sm:grid-cols-[140px_1fr]">
        <Input name="number" label={t('common.number')} value={v.number} onChange={(e) => set('number', e.target.value.replace(/\D/g, ''))} className="font-mono" inputMode="numeric" hint={t('onCall.numberHint')} />
        <Input name="name" label={t('common.name')} value={v.name} onChange={(e) => set('name', e.target.value)} />
      </div>
      <Segmented label={t('onCall.modeLabel')} value={v.mode} onChange={(x) => set('mode', x)}
        options={(['weekly', 'daily', 'manual'] as const).map((k) => ({ key: k, label: t(`onCall.modes.${k}`) }))} />
      <p className="-mt-3 text-xs text-subtle">{t(`onCall.modeExplain.${v.mode}`)}</p>
      {v.mode !== 'manual' && (
        <div className="grid gap-4 sm:grid-cols-3">
          {v.mode === 'weekly' && (
            <Select name="handoverDay" label={t('onCall.handoverDay')} value={v.handoverDay} onChange={(e) => set('handoverDay', Number(e.target.value))}>
              {[0, 1, 2, 3, 4, 5, 6].map((d) => <option key={d} value={d}>{t(`onCall.days.${d}`)}</option>)}
            </Select>
          )}
          <Input name="handoverTime" type="time" label={t('onCall.handoverTime')} value={v.handoverTime} onChange={(e) => set('handoverTime', e.target.value)} />
          <Input name="startDate" type="date" label={t('onCall.startDate')} value={v.startDate} onChange={(e) => set('startDate', e.target.value)} hint={t('onCall.startDateHint')} />
        </div>
      )}
      <div className="flex flex-col gap-2">
        <span className="text-sm font-medium text-ink-2">{t('onCall.members')}</span>
        <p className="text-xs text-subtle">{t('onCall.membersHint')}</p>
        {v.members.length > 0 && (
          <ol className="flex flex-col gap-1.5 rounded-xl border border-line p-2">
            {v.members.map((id, i) => {
              const u = byId.get(id)
              return (
                <li key={id} className="flex flex-wrap items-center gap-3 rounded-lg bg-tint/[0.03] px-3 py-1.5 text-sm">
                  <span className="w-5 font-mono text-xs text-subtle">{i + 1}</span>
                  <Avatar user={u} />
                  <span className="min-w-32 flex-1 text-ink">{u?.name ?? `#${id}`}</span>
                  {mobile?.id === id ? (
                    <span className="flex items-center gap-1.5">
                      <input value={mobile.value} onChange={(e) => setMobile({ id, value: e.target.value })} inputMode="tel" aria-label={t('onCall.mobile')}
                        className="min-h-9 w-40 rounded-lg border border-line bg-field px-2 font-mono text-sm text-ink outline-none focus:border-accent/60" placeholder="0612345678" />
                      <IconButton label={t('common.save')} onClick={saveMobile}><Check className="size-4" aria-hidden /></IconButton>
                      <IconButton label={t('common.cancel')} onClick={() => setMobile(null)}><X className="size-4" aria-hidden /></IconButton>
                    </span>
                  ) : (
                    <button type="button" onClick={() => setMobile({ id, value: u?.mobile ?? '' })} className="inline-flex min-h-9 items-center gap-1.5 rounded-lg px-2 text-xs text-muted hover:bg-tint/5 hover:text-ink">
                      <Smartphone className="size-3.5" aria-hidden />
                      {u?.mobile ? <span className="font-mono">{u.mobile}</span> : <span className="text-warn-ink">{t('onCall.noMobile')}</span>}
                    </button>
                  )}
                  <IconButton label={t('common.moveUp')} onClick={() => move(i, -1)} disabled={i === 0}><ArrowUp className="size-4" aria-hidden /></IconButton>
                  <IconButton label={t('common.moveDown')} onClick={() => move(i, 1)} disabled={i === v.members.length - 1}><ArrowDown className="size-4" aria-hidden /></IconButton>
                  <IconButton label={t('common.remove')} danger onClick={() => set('members', v.members.filter((x) => x !== id))}><X className="size-4" aria-hidden /></IconButton>
                </li>
              )
            })}
          </ol>
        )}
        <div className="flex items-end gap-2">
          <div className="flex-1">
            <Select name="members" label={t('onCall.addMember')} value={pick} onChange={(e) => setPick(Number(e.target.value))}>
              <option value={0}>{t('dest.choose')}</option>
              {users.filter((u) => !v.members.includes(u.id)).map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
            </Select>
          </div>
          <Button variant="ghost" disabled={!pick} onClick={() => { set('members', [...v.members, pick]); setPick(0) }}><Plus className="size-4" aria-hidden /> {t('common.add')}</Button>
        </div>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <Select name="backup" label={t('onCall.backupLabel')} value={v.backup} onChange={(e) => set('backup', e.target.value as OnCallTeam['backup'])} hint={t(`onCall.backupHint.${v.backup}`)}>
          {(['next', 'fixed', 'none'] as const).map((k) => <option key={k} value={k}>{t(`onCall.backupModes.${k}`)}</option>)}
        </Select>
        {v.backup === 'fixed' && (
          <Select name="backupUser" label={t('onCall.backupUser')} value={v.backupUser} onChange={(e) => set('backupUser', Number(e.target.value))}>
            <option value={0}>{t('dest.choose')}</option>
            {users.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
          </Select>
        )}
        <Select name="leadUser" label={t('onCall.lead')} value={v.leadUser} onChange={(e) => set('leadUser', Number(e.target.value))} hint={t('onCall.leadHint')}>
          <option value={0}>{t('onCall.noLead')}</option>
          {users.map((u) => <option key={u.id} value={u.id}>{u.name}</option>)}
        </Select>
        <Select name="swapApproval" label={t('onCall.swapApproval')} value={v.swapApproval} onChange={(e) => set('swapApproval', e.target.value as OnCallTeam['swapApproval'])} hint={t(`onCall.swapHint.${v.swapApproval}`)}>
          {(['lead', 'admin', 'none'] as const).map((k) => <option key={k} value={k} disabled={k === 'lead' && !v.leadUser}>{t(`onCall.swapModes.${k}`)}</option>)}
        </Select>
      </div>
      <Toggle name="reminders" label={t('onCall.reminders')} hint={t('onCall.remindersHint')} checked={v.reminders} onChange={(x) => set('reminders', x)} />
      <Toggle name="enabled" label={t('common.enabled')} hint={t('onCall.enabledHint')} checked={v.enabled} onChange={(x) => set('enabled', x)} />
      {v.id > 0 && ical && (
        <div className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-ink-2">{t('onCall.ical')}</span>
          <div className="flex flex-wrap items-center gap-2">
            <code className="min-h-10 flex-1 truncate rounded-lg border border-line bg-inset px-3 py-2.5 font-mono text-xs text-ink">{ical}</code>
            <Button variant="ghost" onClick={async () => { await navigator.clipboard?.writeText(ical); setCopied(true) }}>{copied ? <Check className="size-4" aria-hidden /> : <Copy className="size-4" aria-hidden />} {t('common.copy')}</Button>
            <Button variant="ghost" onClick={newToken}><RefreshCw className="size-4" aria-hidden /> {t('onCall.icalReset')}</Button>
          </div>
          <p className="text-xs text-subtle"><Link2 className="mr-1 inline size-3" aria-hidden />{t('onCall.icalHint')}</p>
        </div>
      )}
    </>
  )
}

function EscalationForm({ v, set, users }: { v: OnCallTeam; set: SetFn; users: OnCallUser[] }) {
  const { t } = useTranslation()
  const prompts = useApi<Prompt[]>('/pbx/prompts')
  const steps = v.steps
  const setStep = (i: number, s: Partial<OnCallStep>) => set('steps', steps.map((x, j) => (j === i ? { ...x, ...s } : x)))
  const move = (i: number, d: number) => {
    const m = [...steps]
    const j = i + d
    if (j < 0 || j >= m.length) return
    ;[m[i], m[j]] = [m[j], m[i]]
    set('steps', m)
  }
  return (
    <>
      <div>
        <h2 className="font-display text-lg font-semibold">{t('onCall.stepsTitle')}</h2>
        <p className="text-sm text-muted">{t('onCall.stepsHint')}</p>
      </div>
      <ol className="flex flex-col gap-2">
        {steps.map((s, i) => (
          <li key={i} className="flex flex-wrap items-end gap-3 rounded-xl border border-line bg-surface-2/60 p-3">
            <span className="mb-2.5 inline-flex size-7 flex-none items-center justify-center rounded-full bg-accent/15 text-xs font-semibold text-accent-ink shadow-[inset_0_0_0_1px_rgb(34_211_238_/_0.4)]">{i + 1}</span>
            <div className="min-w-48 flex-1">
              <Select name={`steps.${i}`} label={t('onCall.stepWho')} value={s.kind} onChange={(e) => setStep(i, { kind: e.target.value as OnCallStep['kind'], target: '' })}>
                {(['primary', 'backup', 'lead', 'all', 'user', 'number'] as const).map((k) => <option key={k} value={k}>{t(`onCall.stepKinds.${k}`)}</option>)}
              </Select>
            </div>
            {s.kind === 'user' && (
              <div className="min-w-44">
                <Select label={t('onCall.engineer')} value={s.target} onChange={(e) => setStep(i, { target: e.target.value })}>
                  <option value="">{t('dest.choose')}</option>
                  {users.map((u) => <option key={u.id} value={String(u.id)}>{u.name}</option>)}
                </Select>
              </div>
            )}
            {s.kind === 'number' && (
              <div className="min-w-44">
                <Input label={t('dest.number')} value={s.target} onChange={(e) => setStep(i, { target: e.target.value.replace(/[^0-9+]/g, '') })} className="font-mono" inputMode="tel" placeholder="0612345678" />
              </div>
            )}
            <div className="w-28">
              <Input label={t('onCall.seconds')} type="number" min={5} max={300} value={s.seconds} onChange={(e) => setStep(i, { seconds: Number(e.target.value) })} />
            </div>
            <div className="mb-0.5 flex">
              <IconButton label={t('common.moveUp')} onClick={() => move(i, -1)} disabled={i === 0}><ArrowUp className="size-4" aria-hidden /></IconButton>
              <IconButton label={t('common.moveDown')} onClick={() => move(i, 1)} disabled={i === steps.length - 1}><ArrowDown className="size-4" aria-hidden /></IconButton>
              <IconButton label={t('common.remove')} danger disabled={steps.length === 1} onClick={() => set('steps', steps.filter((_, j) => j !== i))}><X className="size-4" aria-hidden /></IconButton>
            </div>
          </li>
        ))}
      </ol>
      <div>
        <Button variant="ghost" disabled={steps.length >= 10} onClick={() => set('steps', [...steps, { kind: 'all', target: '', seconds: 30 }])}><Plus className="size-4" aria-hidden /> {t('onCall.addStep')}</Button>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <Select name="repeat" label={t('onCall.repeat')} value={v.repeat} onChange={(e) => set('repeat', Number(e.target.value))}>
          {[0, 1, 2, 3].map((n) => <option key={n} value={n}>{t('onCall.repeatN', { count: n })}</option>)}
        </Select>
        <Select name="greetingPrompt" label={t('onCall.greeting')} value={v.greetingPrompt} onChange={(e) => set('greetingPrompt', Number(e.target.value))} hint={t('onCall.greetingHint')}>
          <option value={0}>{t('onCall.noGreeting')}</option>
          {(prompts.data ?? []).map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </Select>
      </div>
      <DestinationPicker label={t('onCall.final')} value={v.final} onChange={(d) => set('final', d)} exclude={{ type: 'oncall', target: String(v.id) }} />
      <div className="grid gap-3 sm:grid-cols-2">
        <Toggle name="confirm" label={t('onCall.confirm')} hint={t('onCall.confirmHint')} checked={v.confirm} onChange={(x) => set('confirm', x)} />
        <Toggle name="ringExtension" label={t('onCall.ringExtension')} hint={t('onCall.ringExtensionHint')} checked={v.ringExtension} onChange={(x) => set('ringExtension', x)} />
        <Toggle name="music" label={t('onCall.music')} hint={t('onCall.musicHint')} checked={v.music} onChange={(x) => set('music', x)} />
        <Input name="cidPrefix" label={t('ringGroups.cidPrefix')} value={v.cidPrefix} onChange={(e) => set('cidPrefix', e.target.value)} placeholder={t('onCall.cidPlaceholder')} hint={t('onCall.cidHint')} />
      </div>
    </>
  )
}

function Changes({ team }: { team: OnCallTeam }) {
  const { t, i18n } = useTranslation()
  const tl = useTimeline(team, startOfToday(), 62)
  const users = useMemo(() => new Map((tl.data?.users ?? []).map((u) => [u.id, u])), [tl.data])
  const shifts = tl.data?.view.shifts ?? []
  async function remove(s: OnCallShift) {
    if (!window.confirm(t('onCall.confirmRemove'))) return
    await api('DELETE', `/oncall/shifts/${s.id}`)
    tl.reload()
  }
  return (
    <Card className="p-5">
      <ErrorMessage error={tl.error} />
      <p className="mb-3 text-sm text-muted">{t('onCall.changesHint')}</p>
      <Table head={[t('onCall.when'), t('onCall.lineLabel'), t('onCall.engineer'), t('onCall.kind'), t('onCall.status'), '']}
        empty={shifts.length === 0 && <Empty icon={<Siren className="size-7" />} title={t('onCall.noChanges')} />}>
        {shifts.map((s) => (
          <tr key={s.id}>
            <td className={`${td} whitespace-nowrap`}>{fmtRange(s.start, s.end, i18n.language)}</td>
            <td className={td}>{t(`onCall.line${s.line}`)}</td>
            <td className={td}><span className="inline-flex items-center gap-2"><Avatar user={users.get(s.userId)} />{users.get(s.userId)?.name ?? '?'}</span>
              {s.fromUser > 0 && s.fromUser !== s.userId && <span className="ml-2 text-xs text-subtle">{t('onCall.insteadOf', { name: users.get(s.fromUser)?.name ?? '?' })}</span>}</td>
            <td className={td}><Badge tone="accent">{t(`onCall.kinds.${s.kind}`)}</Badge></td>
            <td className={td}><Badge tone={s.status === 'approved' ? 'ok' : 'warn'}>{t(`onCall.statuses.${s.status}`)}</Badge></td>
            <td className={`${td} text-right`}><IconButton label={t('common.delete')} danger onClick={() => remove(s)}><Trash2 className="size-4" aria-hidden /></IconButton></td>
          </tr>
        ))}
      </Table>
    </Card>
  )
}
