import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowRightLeft, Check, Info, Link2, Siren, Smartphone, X } from 'lucide-react'
import { Badge, Empty, Modal, Select, Toggle, useApi } from '../../components/kit'
import { Segmented } from '../../components/pbx'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../../components/ui'
import { Avatar, OnCallTimeline, fmtRange, fromLocalInput, toLocalInput } from '../../components/oncall'
import { requestTitle, targetOf } from '../OnCall'
import { api } from '../../lib/api'
import type { OnCallSegment, OnCallShift, OnCallUser, OnCallView } from '../../lib/types'

type Data = { me: number; mobile: string; teams: OnCallView[]; users: OnCallUser[]; now: string }
type Stretch = { line: 1 | 2; userId: number; start: string; end: string; changed: boolean }

/** Joins the segments of one engineer into the stretches they are on duty. */
function stretches(segs: OnCallSegment[], user: number, now: Date): Stretch[] {
  const out: Stretch[] = []
  for (const s of [...segs].sort((a, b) => a.line - b.line || a.start.localeCompare(b.start))) {
    if (s.userId !== user || new Date(s.end) <= now) continue
    const last = out[out.length - 1]
    if (last && last.line === s.line && new Date(last.end).getTime() === new Date(s.start).getTime()) {
      last.end = s.end
      last.changed ||= s.kind !== 'rotation'
    } else {
      out.push({ line: s.line, userId: user, start: s.start, end: s.end, changed: s.kind !== 'rotation' })
    }
  }
  return out.sort((a, b) => a.start.localeCompare(b.start))
}

export function MyOnCall() {
  const { t, i18n } = useTranslation()
  const data = useApi<Data>('/portal/oncall', 60000)
  const [swap, setSwap] = useState<{ team: OnCallView; s: Stretch } | null>(null)
  const [err, setErr] = useState<unknown>(null)
  const users = useMemo(() => new Map((data.data?.users ?? []).map((u) => [u.id, u])), [data.data])
  const d = data.data
  const now = d ? new Date(d.now) : new Date()
  async function call(method: string, path: string, body?: unknown) {
    setErr(null)
    try {
      await api(method, path, body)
      data.reload()
    } catch (e) {
      setErr(e)
    }
  }
  const from = new Date(now)
  from.setHours(0, 0, 0, 0)
  return (
    <>
      <PageHeader title={t('myOnCall.title')} subtitle={t('myOnCall.subtitle')} />
      <ErrorMessage error={data.error ?? err} />
      {d && d.teams.length === 0 && (
        <Card><Empty icon={<Siren className="size-7" />} title={t('myOnCall.emptyTitle')} text={t('myOnCall.emptyText')} /></Card>
      )}
      {d && <MobileCard mobile={d.mobile} onSaved={data.reload} />}
      {d?.teams.map((v) => {
        const mine = stretches(v.segments, d.me, now)
        const next = mine[0]
        const onNow = next && new Date(next.start) <= now
        const pending = v.shifts.filter((s) => s.status === 'pending')
        const forMe = pending.filter((s) => !s.accepted && targetOf(s) === d.me && !(s.kind === 'swap' && s.requestedBy === s.userId))
        const mineReq = pending.filter((s) => s.requestedBy === d.me && !(s.kind === 'swap' && s.requestedBy === s.userId))
        const toApprove = v.team.swapApproval === 'lead' && v.team.leadUser === d.me ? pending.filter((s) => s.accepted && !(s.kind === 'swap' && s.requestedBy === s.userId)) : []
        const pair = (s: OnCallShift) => v.shifts.find((x) => x.id === s.pairId)
        const ical = v.team.icalToken ? `${window.location.origin}/api/oncall/ical/${v.team.icalToken}.ics` : ''
        return (
          <section key={v.team.id} className="flex flex-col gap-4">
            <div className="flex flex-wrap items-center gap-3">
              <h2 className="font-display text-xl font-semibold">{v.team.name} <span className="font-mono text-base text-subtle">{v.team.number}</span></h2>
              <span className="flex-1" />
              {ical && <CopyButton label={t('myOnCall.calendar')} value={ical} />}
              {v.primary !== d.me && (
                <Button variant="ghost" onClick={() => { if (window.confirm(t('myOnCall.takeoverConfirm', { name: users.get(v.primary)?.name ?? t('onCall.nobody') }))) call('POST', '/portal/oncall/takeover', { teamId: v.team.id, line: 1 }) }}>
                  <Siren className="size-4" aria-hidden /> {t('myOnCall.takeover')}
                </Button>
              )}
            </div>
            <div className="grid gap-4 lg:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)]">
              <div className="flex flex-col gap-4">
                <Card className={`flex flex-wrap items-center gap-4 p-5 ${onNow ? 'border-accent/50' : ''}`}>
                  <Avatar user={users.get(d.me)} size="lg" />
                  <div className="min-w-0 flex-1">
                    <div className="text-[11px] uppercase tracking-[0.1em] text-subtle">{onNow ? t('myOnCall.onDutyNow') : next ? t('myOnCall.nextShift') : t('myOnCall.noShifts')}</div>
                    {next && <div className="font-display text-xl font-semibold sm:text-2xl">{fmtRange(next.start, next.end, i18n.language)}</div>}
                    {next && <div className="text-xs text-muted">{t(`onCall.line${next.line}`)}</div>}
                  </div>
                </Card>
                <Card className="p-5">
                  <h3 className="mb-1 font-display text-base font-semibold">{t('myOnCall.upcoming')}</h3>
                  {mine.length === 0 && <p className="text-sm text-muted">{t('myOnCall.noUpcoming')}</p>}
                  <ul>
                    {mine.map((s, i) => {
                      const a = new Date(s.start)
                      return (
                        <li key={i} className="flex flex-wrap items-center gap-3 border-t border-line py-3 first:border-t-0">
                          <span className="w-12 flex-none rounded-[10px] border border-line bg-tint/[0.03] py-1 text-center">
                            <span className="block text-[10px] uppercase tracking-[0.08em] text-subtle">{new Intl.DateTimeFormat(i18n.language, { month: 'short' }).format(a)}</span>
                            <span className="block font-display text-lg font-semibold">{a.getDate()}</span>
                          </span>
                          <div className="min-w-0 flex-1">
                            <div className="text-sm text-ink">{t(`onCall.line${s.line}`)}</div>
                            <div className="text-xs text-muted">{fmtRange(s.start, s.end, i18n.language)}</div>
                          </div>
                          {s.changed && <Badge tone="accent">{t('myOnCall.changed')}</Badge>}
                          <Button variant="ghost" onClick={() => setSwap({ team: v, s })}><ArrowRightLeft className="size-4" aria-hidden /> {t('myOnCall.swap')}</Button>
                        </li>
                      )
                    })}
                  </ul>
                </Card>
              </div>
              <div className="flex flex-col gap-4">
                {forMe.length + toApprove.length + mineReq.length > 0 && (
                  <Card className="flex flex-col gap-3 p-5">
                    {forMe.map((s) => (
                      <div key={s.id} className="flex flex-col gap-2 border-b border-line pb-3 last:border-b-0 last:pb-0">
                        <div className="flex items-start gap-3">
                          <Avatar user={users.get(s.requestedBy)} />
                          <div className="min-w-[200px] flex-1 text-sm">
                            <div className="text-ink">{t('myOnCall.asks', { name: users.get(s.requestedBy)?.name ?? '?', when: fmtRange(s.start, s.end, i18n.language) })}</div>
                            {pair(s) && <div className="text-xs text-muted">{t('myOnCall.inReturn', { when: fmtRange(pair(s)!.start, pair(s)!.end, i18n.language) })}</div>}
                            {s.note && <div className="text-xs italic text-subtle">“{s.note}”</div>}
                          </div>
                        </div>
                        <div className="flex gap-2 pl-11">
                          <Button onClick={() => call('POST', `/oncall/shifts/${s.id}/respond`, { accept: true })}><Check className="size-4" aria-hidden /> {t('myOnCall.accept')}</Button>
                          <Button variant="ghost" onClick={() => call('POST', `/oncall/shifts/${s.id}/respond`, { accept: false })}><X className="size-4" aria-hidden /> {t('myOnCall.decline')}</Button>
                        </div>
                      </div>
                    ))}
                    {toApprove.map((s) => (
                      <div key={s.id} className="flex flex-wrap items-center gap-3 border-b border-line pb-3 last:border-b-0 last:pb-0">
                        <div className="min-w-[200px] flex-1 text-sm">
                          <div className="text-ink">{requestTitle(t, s, users)}</div>
                          <div className="text-xs text-muted">{fmtRange(s.start, s.end, i18n.language)} · {t('myOnCall.needsYou')}</div>
                        </div>
                        <Button variant="ghost" aria-label={t('onCall.decline')} onClick={() => call('POST', `/oncall/shifts/${s.id}/decide`, { approve: false })}><X className="size-4" aria-hidden /></Button>
                        <Button onClick={() => call('POST', `/oncall/shifts/${s.id}/decide`, { approve: true })}><Check className="size-4" aria-hidden /> {t('onCall.approve')}</Button>
                      </div>
                    ))}
                    {mineReq.map((s) => (
                      <div key={s.id} className="flex flex-wrap items-center gap-3 border-b border-line pb-3 last:border-b-0 last:pb-0">
                        <div className="min-w-[200px] flex-1 text-sm">
                          <div className="text-ink">{requestTitle(t, s, users)}</div>
                          <div className="text-xs text-muted">{fmtRange(s.start, s.end, i18n.language)}</div>
                        </div>
                        <div className="ml-auto flex items-center gap-2">
                          <Badge tone="warn">{s.accepted ? t('myOnCall.waitingApproval') : t('onCall.waitingFor', { name: users.get(targetOf(s))?.name ?? '' })}</Badge>
                          <Button variant="ghost" onClick={() => call('DELETE', `/oncall/shifts/${s.id}`)}>{t('myOnCall.withdraw')}</Button>
                        </div>
                      </div>
                    ))}
                  </Card>
                )}
                <Card className="p-5">
                  <div className="text-[11px] uppercase tracking-[0.1em] text-subtle">{t('onCall.nowOnDuty')}</div>
                  <div className="mt-2 flex items-center gap-2.5"><Avatar user={users.get(v.primary)} /><span className="text-ink">{users.get(v.primary)?.name ?? t('onCall.nobody')}</span></div>
                  {v.backup > 0 && <div className="mt-2 flex items-center gap-2.5 text-sm text-muted"><Avatar user={users.get(v.backup)} />{t('onCall.line2')}: {users.get(v.backup)?.name}</div>}
                </Card>
              </div>
            </div>
            <Card className="p-5">
              <OnCallTimeline segments={v.segments} users={users} from={from} days={14} now={now} showBackup={v.team.backup !== 'none'} />
            </Card>
          </section>
        )
      })}
      {swap && d && (
        <SwapForm view={swap.team} stretch={swap.s} me={d.me} users={users} now={now}
          onClose={() => setSwap(null)} onSaved={() => { setSwap(null); data.reload() }} />
      )}
    </>
  )
}

function CopyButton({ label, value }: { label: string; value: string }) {
  const [done, setDone] = useState(false)
  return (
    <Button variant="ghost" onClick={async () => { await navigator.clipboard?.writeText(value); setDone(true) }} title={value}>
      {done ? <Check className="size-4" aria-hidden /> : <Link2 className="size-4" aria-hidden />} {label}
    </Button>
  )
}

function MobileCard({ mobile, onSaved }: { mobile: string; onSaved: () => void }) {
  const { t } = useTranslation()
  const [v, setV] = useState(mobile)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [ok, setOk] = useState(false)
  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api('PUT', '/portal/oncall/mobile', { mobile: v })
      setOk(true)
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Card className="flex flex-col gap-3 p-5">
      <ErrorMessage error={error} />
      <div className="flex flex-wrap items-end gap-3">
        <div className="min-w-56 flex-1">
          <Input name="mobile" label={t('myOnCall.mobile')} value={v} onChange={(e) => { setOk(false); setV(e.target.value) }} inputMode="tel" className="font-mono" placeholder="0612345678"
            hint={t('myOnCall.mobileHint')} />
        </div>
        <Button busy={busy} className="mb-6" onClick={save}>{ok ? <Check className="size-4" aria-hidden /> : <Smartphone className="size-4" aria-hidden />} {t('common.save')}</Button>
      </div>
      {!mobile && <p className="text-sm text-warn-ink">{t('myOnCall.noMobileWarn')}</p>}
    </Card>
  )
}

function SwapForm({ view, stretch, me, users, now, onClose, onSaved }: {
  view: OnCallView; stretch: Stretch; me: number; users: Map<number, OnCallUser>; now: Date; onClose: () => void; onSaved: () => void
}) {
  const { t, i18n } = useTranslation()
  const team = view.team
  const people = [...new Set([...team.members, team.leadUser, team.backupUser])].filter((id) => id && id !== me)
  const startDefault = new Date(Math.max(new Date(stretch.start).getTime(), now.getTime()))
  const [kind, setKind] = useState<'handover' | 'swap'>('handover')
  const [whole, setWhole] = useState(true)
  const [start, setStart] = useState(toLocalInput(startDefault.toISOString()))
  const [end, setEnd] = useState(toLocalInput(stretch.end))
  const [to, setTo] = useState(people[0] ?? 0)
  const [theirs, setTheirs] = useState(0)
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const a = whole ? startDefault.toISOString() : fromLocalInput(start)
  const b = whole ? stretch.end : fromLocalInput(end)
  const their = stretches(view.segments, to, now).filter((s) => s.line === stretch.line && new Date(s.start) > now)
  const status = (id: number) => {
    const other = view.segments.some((s) => s.userId === id && s.line !== stretch.line && new Date(s.end) > new Date(a) && new Date(s.start) < new Date(b))
    if (other) return { tone: 'warn' as const, text: t('myOnCall.otherLine') }
    return { tone: 'ok' as const, text: t('myOnCall.free') }
  }
  async function send() {
    setBusy(true)
    setError(null)
    try {
      const body: Record<string, unknown> = { teamId: team.id, kind, line: stretch.line, start: a, end: b, to, note }
      if (kind === 'swap') {
        const s = their[theirs]
        if (!s) throw new Error(t('myOnCall.pickTheirs'))
        body.swapStart = s.start
        body.swapEnd = s.end
      }
      await api('POST', '/portal/oncall/requests', body)
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const steps = team.swapApproval === 'none' ? t('myOnCall.stepsNone', { name: users.get(to)?.name ?? '' })
    : team.swapApproval === 'lead' ? t('myOnCall.stepsLead', { name: users.get(to)?.name ?? '', lead: users.get(team.leadUser)?.name ?? '' })
      : t('myOnCall.stepsAdmin', { name: users.get(to)?.name ?? '' })
  return (
    <Modal wide title={t('myOnCall.swapTitle')} subtitle={`${t(`onCall.line${stretch.line}`)} · ${fmtRange(stretch.start, stretch.end, i18n.language)}`} onClose={onClose}
      footer={<><span className="mr-auto self-center text-xs text-muted">{steps}</span><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} disabled={!to} onClick={send}><ArrowRightLeft className="size-4" aria-hidden /> {t('myOnCall.send')}</Button></>}>
      <ErrorMessage error={error} />
      <div className="grid gap-5 md:grid-cols-2">
        <div className="flex flex-col gap-4">
          <Segmented label={t('myOnCall.what')} value={kind} onChange={setKind}
            options={[{ key: 'handover', label: t('myOnCall.handover') }, { key: 'swap', label: t('myOnCall.swapKind') }]} />
          <p className="-mt-2 text-xs text-subtle">{t(`myOnCall.${kind}Hint`)}</p>
          <Toggle label={t('myOnCall.whole')} checked={whole} onChange={setWhole} />
          {!whole && (
            <div className="grid gap-3 sm:grid-cols-2">
              <Input name="start" type="datetime-local" label={t('onCall.from')} value={start} onChange={(e) => setStart(e.target.value)} />
              <Input name="end" type="datetime-local" label={t('onCall.to')} value={end} onChange={(e) => setEnd(e.target.value)} />
            </div>
          )}
          <Input name="note" label={t('myOnCall.message')} value={note} onChange={(e) => setNote(e.target.value)} placeholder={t('myOnCall.messagePlaceholder')} />
        </div>
        <div className="flex flex-col gap-2">
          <span className="text-sm font-medium text-ink-2">{t('myOnCall.withWhom')}</span>
          {people.map((id) => {
            const st = status(id)
            return (
              <button key={id} type="button" onClick={() => { setTo(id); setTheirs(0) }} aria-pressed={to === id}
                className={`flex items-center gap-3 rounded-xl border px-3 py-2.5 text-left transition ${to === id ? 'border-accent/55 bg-accent/[0.07]' : 'border-line hover:border-accent/30'}`}>
                <Avatar user={users.get(id)} />
                <span className="min-w-0 flex-1 text-sm text-ink">{users.get(id)?.name ?? '?'}</span>
                <Badge tone={st.tone}>{st.text}</Badge>
              </button>
            )
          })}
          {kind === 'swap' && (
            their.length > 0 ? (
              <Select name="swapStart" label={t('myOnCall.theirShift')} value={theirs} onChange={(e) => setTheirs(Number(e.target.value))}>
                {their.map((s, i) => <option key={i} value={i}>{fmtRange(s.start, s.end, i18n.language)}</option>)}
              </Select>
            ) : <p className="text-sm text-warn-ink">{t('myOnCall.noTheirs')}</p>
          )}
          <div className="mt-1 flex gap-2.5 rounded-xl border border-accent/35 bg-accent/[0.06] px-3 py-2.5 text-xs text-ink-2">
            <Info className="mt-0.5 size-3.5 flex-none text-accent" aria-hidden />
            <span>{t('myOnCall.afterApproval')}</span>
          </div>
        </div>
      </div>
    </Modal>
  )
}
