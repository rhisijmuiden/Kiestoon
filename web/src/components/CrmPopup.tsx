import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, ExternalLink, Phone, PhoneOff, UserPlus, X } from 'lucide-react'
import { Button } from './ui'
import { api } from '../lib/api'
import { crmColors, crmNames, useCrmStatus, type CrmContact, type CrmEvent, type CrmKind, type CrmResult } from '../lib/crm'
import { useSession } from '../lib/session'
import { useSoftphone } from '../lib/softphone'

type Phase = 'ring' | 'talk' | 'after' | 'saved'
type Pop = { ev: CrmEvent; phase: Phase; pick: number; result: CrmResult | null; ended?: number }

const last9 = (n: string) => n.replace(/\D/g, '').slice(-9)

/**
 * The call pop-up: shows who is calling (from the CRM) while the phone rings, and after the call a
 * note for the CRM. Events come from the server, so it works for desk phones, the browser phone and
 * the Windows softphone alike.
 */
export function CrmPopup() {
  const { state } = useSession()
  const me = state.kind === 'ready' ? state.me : null
  const myExt = me?.extension?.number ?? ''
  const status = useCrmStatus(!!myExt)
  const [pops, setPops] = useState<Pop[]>([])

  useEffect(() => {
    if (!status?.active || !myExt) return
    const es = new EventSource('/api/portal/crm/events')
    es.onmessage = (m) => {
      let ev: CrmEvent
      try { ev = JSON.parse(m.data) } catch { return }
      setPops((list) => apply(list, ev, myExt))
    }
    return () => es.close()
  }, [status?.active, myExt])

  // A note that was saved closes by itself.
  useEffect(() => {
    if (!pops.some((p) => p.phase === 'saved')) return
    const id = window.setTimeout(() => setPops((l) => l.filter((p) => p.phase !== 'saved')), 2500)
    return () => window.clearTimeout(id)
  }, [pops])

  if (!pops.length) return null
  const update = (id: string, f: (p: Pop) => Pop | null) => setPops((l) => l.flatMap((p) => (p.ev.callId === id ? (f(p) ? [f(p)!] : []) : [p])))
  return (
    <div className="pointer-events-none fixed right-4 top-4 z-40 flex w-[min(400px,calc(100vw-32px))] flex-col gap-3" aria-live="polite">
      {pops.slice(0, 3).map((p) => <PopCard key={p.ev.callId} pop={p} createIn={status?.createIn || ''} update={(f) => update(p.ev.callId, f)} />)}
    </div>
  )
}

function apply(list: Pop[], ev: CrmEvent, myExt: string): Pop[] {
  const cur = list.find((p) => p.ev.callId === ev.callId)
  const rest = list.filter((p) => p.ev.callId !== ev.callId)
  switch (ev.type) {
    case 'ring':
      if (cur) return [...rest, { ...cur, ev: { ...cur.ev, ...ev }, result: ev.result ?? cur.result }]
      return [{ ev, phase: 'ring', pick: 0, result: ev.result ?? null }, ...rest]
    case 'stop':
      return cur?.phase === 'ring' ? rest : list
    case 'answered':
      if (!cur) return list
      return ev.ext === myExt ? [{ ...cur, phase: 'talk' }, ...rest] : rest
    case 'ended':
      if (!cur) return list
      if (cur.phase === 'talk' && ev.loggable) return [{ ...cur, phase: 'after', ended: ev.talkSec }, ...rest]
      return rest
  }
  return list
}

function Logo({ kind, size = 'size-5' }: { kind: CrmKind; size?: string }) {
  return (
    <span className={`inline-grid ${size} flex-none place-items-center rounded-[5px] text-[10px] font-bold text-white`} style={{ background: crmColors[kind] }} aria-hidden>
      {crmNames[kind][0]}
    </span>
  )
}

function shortDate(s: string | undefined, lang: string) {
  if (!s) return ''
  const d = new Date(s)
  if (Number.isNaN(d.getTime())) return ''
  return d.toLocaleDateString(lang === 'nl' ? 'nl-NL' : 'en-GB', { day: 'numeric', month: 'short' })
}

function PopCard({ pop, createIn, update }: { pop: Pop; createIn: CrmKind | ''; update: (f: (p: Pop) => Pop | null) => void }) {
  const { t, i18n } = useTranslation()
  const phone = useSoftphone()
  const { ev, phase, result } = pop
  const contacts = result?.contacts ?? []
  const c: CrmContact | undefined = contacts[pop.pick]
  const kind = (result?.crm || c?.crm || createIn || 'hubspot') as CrmKind
  const number = ev.number ?? result?.number ?? ''
  const browserRinging = phone.call?.direction === 'incoming' && phone.call.state === 'ringing' && last9(phone.call.remote) === last9(number)
  const close = () => update(() => null)
  const initials = (c?.name || '?').split(/\s+/).map((x) => x[0]).join('').slice(0, 2).toUpperCase()

  const head = phase === 'ring' ? t('crm.pop.ringing') : phase === 'talk' ? t('crm.pop.inCall') : t('crm.pop.ended', { time: fmt(pop.ended ?? 0) })
  return (
    <section className="pointer-events-auto overflow-hidden rounded-2xl border border-accent/45 bg-surface shadow-[0_30px_80px_-24px_rgb(0_0_0_/_0.55),0_0_0_4px_rgb(34_211_238_/_0.08)]" aria-label={t('crm.pop.label')}>
      <div className="flex flex-col gap-2.5 border-b border-line px-4 pb-3 pt-3.5">
        <div className="flex items-center justify-between gap-2 text-xs font-semibold">
          <span className={`inline-flex items-center gap-2 ${phase === 'ring' ? 'text-accent-ink' : 'text-muted'}`}>
            {phase === 'ring' && <span className="relative inline-flex size-2"><span className="absolute inline-flex size-full animate-ping rounded-full bg-accent opacity-60 motion-reduce:animate-none" /><span className="relative inline-flex size-2 rounded-full bg-accent" /></span>}
            {head}
          </span>
          <span className="flex items-center gap-2">
            {c && <span className="inline-flex items-center gap-1.5 font-medium text-subtle"><Logo kind={c.crm} size="size-3.5" />{t('crm.pop.from', { crm: crmNames[c.crm] })}</span>}
            <button type="button" onClick={close} aria-label={t('common.close')} className="inline-flex size-7 items-center justify-center rounded-lg text-subtle hover:bg-tint/5 hover:text-ink"><X className="size-4" aria-hidden /></button>
          </span>
        </div>
        <div className="flex items-center gap-3">
          <span className="grid size-11 flex-none place-items-center rounded-full bg-gradient-to-br from-[#0891B2] to-[#6366F1] font-display text-base font-semibold text-white" aria-hidden>{c ? initials : '?'}</span>
          <div className="min-w-0">
            <p className="truncate font-display text-lg font-semibold text-ink">{c?.name || ev.name || t('crm.pop.unknown')}</p>
            <p className="truncate text-[13px] text-muted">{[c?.company && c.company !== c.name ? c.company : '', number].filter(Boolean).join(' · ')}</p>
          </div>
        </div>
        {contacts.length > 1 && (
          <label className="flex items-center gap-2 text-xs text-subtle">
            {t('crm.pop.several', { count: contacts.length })}
            <select value={pop.pick} onChange={(e) => update((p) => ({ ...p, pick: Number(e.target.value) }))} className="min-h-8 flex-1 rounded-lg border border-line bg-field px-2 text-xs text-ink">
              {contacts.map((x, i) => <option key={x.id} value={i}>{x.name}{x.company ? ` · ${x.company}` : ''}</option>)}
            </select>
          </label>
        )}
        {ev.did && phase === 'ring' && <p className="text-xs text-subtle">{t('crm.pop.via', { did: ev.did })}</p>}
      </div>

      {phase !== 'after' && phase !== 'saved' && c && <Details c={c} lang={i18n.language} />}
      {phase !== 'after' && phase !== 'saved' && !c && result && <Unknown number={number} createIn={createIn} onCreated={(nc) => update((p) => ({ ...p, result: { number, crm: nc.crm, contacts: [nc] }, pick: 0 }))} />}
      {phase === 'after' && <AfterCall pop={pop} kind={kind} onSaved={() => update((p) => ({ ...p, phase: 'saved' }))} onSkip={close} />}
      {phase === 'saved' && <p className="flex items-center gap-2 px-4 py-4 text-sm font-medium text-ok-ink"><Check className="size-4" aria-hidden />{t('crm.pop.saved', { crm: crmNames[kind] })}</p>}

      {(phase === 'ring' || phase === 'talk') && (browserRinging || c?.url) && (
        <div className="flex flex-wrap gap-2 bg-surface-2 px-4 py-3">
          {browserRinging && (
            <>
              <button type="button" onClick={() => phone.answer()} className="inline-flex min-h-10 flex-1 items-center justify-center gap-2 rounded-[10px] bg-ok px-3 text-sm font-semibold text-[#05140D]"><Phone className="size-4" aria-hidden />{t('crm.pop.answer')}</button>
              <button type="button" onClick={() => phone.hangup()} className="inline-flex min-h-10 flex-1 items-center justify-center gap-2 rounded-[10px] bg-danger px-3 text-sm font-semibold text-[#1A0606]"><PhoneOff className="size-4" aria-hidden />{t('crm.pop.decline')}</button>
            </>
          )}
          {c?.url && (
            <a href={c.url} target="_blank" rel="noopener noreferrer" className="inline-flex min-h-10 basis-full items-center justify-center gap-2 rounded-[10px] border border-line bg-tint/5 px-3 text-sm font-medium text-ink hover:bg-tint/10">
              {t('crm.pop.open', { crm: crmNames[c.crm] })}<ExternalLink className="size-3.5" aria-hidden />
            </a>
          )}
        </div>
      )}
    </section>
  )
}

function fmt(sec: number) {
  return `${Math.floor(sec / 60)}:${String(sec % 60).padStart(2, '0')}`
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col gap-1.5 border-b border-line px-4 py-2.5">
      <h3 className="text-[10.5px] font-semibold uppercase tracking-[0.09em] text-subtle">{title}</h3>
      {children}
    </div>
  )
}

function Row({ title, value, url }: { title: string; value?: string; url?: string }) {
  const label = url ? <a href={url} target="_blank" rel="noopener noreferrer" className="truncate hover:text-accent-ink hover:underline">{title}</a> : <span className="truncate">{title}</span>
  return <div className="flex justify-between gap-3 text-[13px] text-ink-2">{label}{value && <span className="whitespace-nowrap text-subtle tabular-nums">{value}</span>}</div>
}

function Details({ c, lang }: { c: CrmContact; lang: string }) {
  const { t } = useTranslation()
  const deals = c.deals ?? []
  const tickets = c.tickets ?? []
  return (
    <>
      {deals.length > 0 && <Section title={t('crm.pop.deals')}>{deals.slice(0, 3).map((d, i) => <Row key={i} title={d.title} value={d.value || shortDate(d.when, lang)} url={d.url} />)}</Section>}
      {tickets.length > 0 && <Section title={t('crm.pop.tickets')}>{tickets.slice(0, 3).map((d, i) => <Row key={i} title={d.title} value={shortDate(d.when, lang)} url={d.url} />)}</Section>}
      {(c.last || c.owner || c.lifecycle) && (
        <Section title={t('crm.pop.last')}>
          {c.last && <Row title={t('crm.pop.lastContact')} value={shortDate(c.last.when, lang)} />}
          {(c.owner || c.lifecycle) && <p className="text-xs text-subtle">{[c.owner && t('crm.pop.owner', { name: c.owner }), c.lifecycle].filter(Boolean).join(' · ')}</p>}
        </Section>
      )}
    </>
  )
}

function Unknown({ number, createIn, onCreated }: { number: string; createIn: CrmKind | ''; onCreated: (c: CrmContact) => void }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [company, setCompany] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const nameRef = useRef<HTMLInputElement>(null)
  useEffect(() => { if (open) nameRef.current?.focus() }, [open])
  if (!createIn) return <p className="border-b border-line px-4 py-3 text-xs text-subtle">{t('crm.pop.notFound')}</p>
  async function create() {
    setBusy(true)
    setErr('')
    try {
      const r = await api<{ contact: CrmContact }>('POST', '/portal/crm/contact', { number, name, company })
      onCreated(r.contact)
    } catch {
      setErr(t('crm.pop.createFailed'))
    } finally {
      setBusy(false)
    }
  }
  const field = 'min-h-10 w-full rounded-[10px] border border-line bg-field px-3 text-sm text-ink placeholder:text-subtle outline-none focus:border-accent/60'
  return (
    <div className="flex flex-col gap-2 border-b border-line px-4 py-3">
      <p className="text-xs text-subtle">{t('crm.pop.notFound')}</p>
      {!open ? (
        <Button variant="ghost" onClick={() => setOpen(true)}><UserPlus className="size-4" aria-hidden />{t('crm.pop.create', { crm: crmNames[createIn] })}</Button>
      ) : (
        <form className="flex flex-col gap-2" onSubmit={(e) => { e.preventDefault(); if (name.trim()) create() }}>
          <input ref={nameRef} name="name" className={field} placeholder={t('crm.pop.name')} aria-label={t('crm.pop.name')} value={name} onChange={(e) => setName(e.target.value)} maxLength={120} />
          <input name="company" className={field} placeholder={t('crm.pop.company')} aria-label={t('crm.pop.company')} value={company} onChange={(e) => setCompany(e.target.value)} maxLength={120} />
          {err && <p className="text-xs text-danger-ink" role="alert">{err}</p>}
          <Button type="submit" busy={busy} disabled={!name.trim()}>{t('crm.pop.create', { crm: crmNames[createIn] })}</Button>
        </form>
      )}
    </div>
  )
}

function AfterCall({ pop, kind, onSaved, onSkip }: { pop: Pop; kind: CrmKind; onSaved: () => void; onSkip: () => void }) {
  const { t } = useTranslation()
  const [note, setNote] = useState('')
  const [outcome, setOutcome] = useState('connected')
  const [task, setTask] = useState(false)
  const [days, setDays] = useState(3)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const known = (pop.result?.contacts.length ?? 0) > 0
  async function save() {
    setBusy(true)
    setErr('')
    try {
      await api('POST', '/portal/crm/note', { callId: pop.ev.callId, note, outcome, taskDays: task ? days : 0 })
      onSaved()
    } catch {
      setErr(t('crm.pop.noteFailed'))
    } finally {
      setBusy(false)
    }
  }
  const field = 'w-full rounded-[10px] border border-line bg-field px-3 text-sm text-ink outline-none focus:border-accent/60'
  if (!known) {
    return (
      <div className="flex flex-col gap-2 px-4 py-3">
        <p className="text-xs text-subtle">{t('crm.pop.notLogged')}</p>
        <Button variant="ghost" onClick={onSkip}>{t('common.close')}</Button>
      </div>
    )
  }
  return (
    <form className="flex flex-col gap-3 px-4 py-3" onSubmit={(e) => { e.preventDefault(); save() }}>
      <label className="flex flex-col gap-1.5 text-[13px] font-medium text-ink-2">
        {t('crm.pop.note', { crm: crmNames[kind] })}
        <textarea name="note" rows={4} maxLength={4000} value={note} onChange={(e) => setNote(e.target.value)} className={`${field} resize-y py-2 leading-relaxed`} autoFocus placeholder={t('crm.pop.notePlaceholder')} />
      </label>
      <label className="flex flex-col gap-1.5 text-[13px] font-medium text-ink-2">
        {t('crm.pop.outcome')}
        <select name="outcome" value={outcome} onChange={(e) => setOutcome(e.target.value)} className={`${field} min-h-10`}>
          {['connected', 'voicemail', 'noanswer', 'busy'].map((o) => <option key={o} value={o}>{t(`crm.pop.outcomes.${o}`)}</option>)}
        </select>
      </label>
      <div className="flex flex-wrap items-center gap-2 text-[13px] text-ink-2">
        <label className="inline-flex items-center gap-2"><input type="checkbox" checked={task} onChange={(e) => setTask(e.target.checked)} className="size-4 accent-[var(--color-accent)]" />{t('crm.pop.task')}</label>
        {task && (
          <select aria-label={t('crm.pop.taskWhen')} value={days} onChange={(e) => setDays(Number(e.target.value))} className="min-h-8 rounded-lg border border-line bg-field px-2 text-xs text-ink">
            {[1, 3, 7, 14].map((d) => <option key={d} value={d}>{t('crm.pop.inDays', { count: d })}</option>)}
          </select>
        )}
      </div>
      {err && <p className="text-xs text-danger-ink" role="alert">{err}</p>}
      <div className="flex gap-2">
        <Button type="submit" busy={busy} disabled={!note.trim() && !task} className="flex-1">{t('crm.pop.save', { crm: crmNames[kind] })}</Button>
        <Button type="button" variant="ghost" onClick={onSkip}>{t('crm.pop.skip')}</Button>
      </div>
      <p className="flex items-center gap-1.5 text-xs text-ok-ink"><Check className="size-3.5" aria-hidden />{t('crm.pop.autoLogged')}</p>
    </form>
  )
}
