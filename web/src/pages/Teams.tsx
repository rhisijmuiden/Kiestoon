import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, Copy, Download, Pencil, Plus, Trash2, UsersRound } from 'lucide-react'
import { Badge, Empty, Modal, Select, Tabs, Toggle, td, Table, useApi } from '../components/kit'
import { IconButton } from '../components/pbx'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api, ApiError } from '../lib/api'
import type { Extension, Trunk } from '../lib/types'

export type TeamsUser = { name: string; upn: string; number: string; extension: string; ring: 'teams' | 'both' }
export type TeamsSettings = {
  enabled: boolean
  fqdn: string
  port: number
  trunkId: number
  callerId: 'own' | 'trunk'
  maxSessions: number
  ringBoth: boolean
  users: TeamsUser[]
}
type TeamsHub = { host: string; status: string; rttMs: number }
type TeamsStatus = {
  active: boolean
  certificate: { public: boolean; subject: string; issuer: string; notAfter: string; dnsNames: string[] | null; rsa: boolean; covers: boolean } | null
  internetOn: boolean
  useForSip: boolean
  domain: string
  firewallOn: boolean
  hubs: TeamsHub[]
  activeCalls: number
  restartNeeded: boolean
  connected: boolean
  nets: string[]
}
type TeamsInfo = { settings: TeamsSettings; status: TeamsStatus }
type Tab = 'overview' | 'users' | 'setup'

/** Field names the server reports ("fqdn", "users.2.upn") as a readable label. */
function useFieldLabel() {
  const { t } = useTranslation()
  return (field?: string) => {
    if (!field) return ''
    const m = /^users\.(\d+)\.(\w+)$/.exec(field)
    if (m) return t('teams.userField', { row: Number(m[1]) + 1, field: t(`teams.f.${m[2]}`) })
    return t(`teams.f.${field}`, { defaultValue: field })
  }
}

function TeamsError({ error }: { error: unknown }) {
  const { t } = useTranslation()
  const label = useFieldLabel()
  if (error instanceof ApiError && error.code === 'invalid' && error.field) {
    return <Alert>{t('teams.invalid', { field: label(error.field) })}</Alert>
  }
  return <ErrorMessage error={error} />
}

export function Teams() {
  const { t } = useTranslation()
  const info = useApi<TeamsInfo>('/teams', 15000)
  const trunks = useApi<Trunk[]>('/pbx/trunks')
  const exts = useApi<Extension[]>('/pbx/extensions')
  const [tab, setTab] = useState<Tab>('overview')
  const [s, setS] = useState<TeamsSettings | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  useEffect(() => { if (info.data && !s) setS(info.data.settings) }, [info.data, s])

  async function save(next: TeamsSettings): Promise<unknown> {
    setBusy(true)
    setError(null)
    setSaved(false)
    try {
      const r = await api<TeamsInfo>('PUT', '/teams', next)
      info.setData(r)
      setS(r.settings)
      setSaved(true)
      return null
    } catch (e) {
      setError(e)
      return e
    } finally {
      setBusy(false)
    }
  }

  const st = info.data?.status
  return (
    <>
      <PageHeader title={t('teams.title')} subtitle={t('teams.subtitle')}>
        {s && <Badge tone={st?.active ? 'ok' : s.enabled ? 'warn' : 'muted'} dot>{st?.active ? t('teams.stateActive') : s.enabled ? t('teams.stateWaiting') : t('teams.stateOff')}</Badge>}
      </PageHeader>
      <Tabs<Tab> value={tab} onChange={setTab} tabs={[
        { key: 'overview', label: t('teams.tabOverview') },
        { key: 'users', label: t('teams.tabUsers') },
        { key: 'setup', label: t('teams.tabSetup') },
      ]} />
      <ErrorMessage error={info.error} />
      {saved && <Alert tone="ok">{t('common.saved')}</Alert>}
      {st?.restartNeeded && <Alert tone="warn">{t('teams.restartNeeded')}</Alert>}
      {s && st && tab === 'overview' && <Overview s={s} setS={setS} st={st} trunks={trunks.data ?? []} busy={busy} error={error} onSave={() => save(s)} />}
      {s && tab === 'users' && <Users s={s} exts={exts.data ?? []} error={error} onSave={save} />}
      {s && tab === 'setup' && <Setup s={s} />}
    </>
  )
}

function Overview({ s, setS, st, trunks, busy, error, onSave }: {
  s: TeamsSettings; setS: (v: TeamsSettings) => void; st: TeamsStatus; trunks: Trunk[]; busy: boolean; error: unknown; onSave: () => Promise<unknown>
}) {
  const { t, i18n } = useTranslation()
  const set = <K extends keyof TeamsSettings>(k: K, v: TeamsSettings[K]) => setS({ ...s, [k]: v })
  const cert = st.certificate
  const certOk = !!cert && cert.rsa && cert.covers
  const hubsUp = st.hubs.filter((h) => h.status === 'reachable').length
  const assigned = s.users.length
  const checks: { ok: boolean; title: string; sub: string }[] = [
    { ok: st.internetOn && st.useForSip, title: t('teams.cInternet'), sub: st.internetOn && st.useForSip ? t('teams.cInternetOk', { domain: st.domain }) : t('teams.cInternetTodo') },
    { ok: certOk, title: t('teams.cCert'), sub: !cert ? t('teams.cCertNone') : !cert.rsa ? t('teams.cCertEcdsa') : !cert.covers ? t('teams.cCertName', { fqdn: s.fqdn || '–' }) : t('teams.cCertOk', { date: new Date(cert.notAfter).toLocaleDateString(i18n.language) }) },
    { ok: true, title: t('teams.cFirewall'), sub: st.firewallOn ? t('teams.cFirewallOn', { port: s.port }) : t('teams.cFirewallOff') },
    { ok: hubsUp > 0, title: t('teams.cHubs'), sub: st.active ? t('teams.cHubsSub', { n: hubsUp }) : t('teams.cHubsOff') },
    { ok: assigned > 0, title: t('teams.cUsers'), sub: t('teams.cUsersSub', { n: assigned }) },
  ]
  return (
    <>
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <Kpi label={t('teams.kSbc')} value={<span className="font-mono text-[15px]">{s.fqdn || '–'}</span>} sub={`SIP TLS ${s.port} · SRTP`} />
        <Kpi label={t('teams.kCert')} value={<span className={certOk ? 'text-ok-ink' : 'text-warn-ink'}>{certOk ? t('teams.certValid') : t('teams.certMissing')}</span>}
          sub={cert ? `${cert.issuer} · ${new Date(cert.notAfter).toLocaleDateString(i18n.language)}` : t('teams.certNeedInternet')} />
        <Kpi label={t('teams.kCalls')} value={<span className="font-mono">{st.activeCalls} <span className="text-sm text-muted">/ {s.maxSessions}</span></span>} sub={t('teams.callsSub')} />
        <Kpi label={t('teams.kUsers')} value={<span className="font-mono">{assigned}</span>} sub={t('teams.usersSub')} />
      </div>
      <div className="grid items-start gap-5 xl:grid-cols-[3fr_2fr]">
        <Card className="flex flex-col gap-4">
          <div>
            <h2 className="font-display text-lg font-semibold">{t('teams.connTitle')}</h2>
            <p className="mt-0.5 text-sm text-muted">{t('teams.connSub')}</p>
          </div>
          <TeamsError error={error} />
          <Toggle label={t('teams.enabled')} hint={t('teams.enabledHint')} checked={s.enabled} onChange={(v) => setS({ ...s, enabled: v, fqdn: v && !s.fqdn ? st.domain : s.fqdn })} />
          <div className="grid gap-4 sm:grid-cols-2">
            <Input label={t('teams.fqdn')} name="fqdn" value={s.fqdn} onChange={(e) => set('fqdn', e.target.value.trim())} placeholder={st.domain || 'sbc.example.com'} className="font-mono" hint={t('teams.fqdnHint')} />
            <Input label={t('teams.port')} name="port" type="number" value={s.port} onChange={(e) => set('port', Number(e.target.value))} className="font-mono" hint={t('teams.portHint')} />
            <Select label={t('teams.trunk')} name="trunkId" value={s.trunkId} onChange={(e) => set('trunkId', Number(e.target.value))} hint={t('teams.trunkHint')}>
              <option value={0}>{t('teams.trunkRoutes')}</option>
              {trunks.map((tr) => <option key={tr.id} value={tr.id}>{tr.name}</option>)}
            </Select>
            <Select label={t('teams.callerId')} name="callerId" value={s.callerId} onChange={(e) => set('callerId', e.target.value as TeamsSettings['callerId'])} hint={t('teams.callerIdHint')}>
              <option value="own">{t('teams.cidOwn')}</option>
              <option value="trunk">{t('teams.cidTrunk')}</option>
            </Select>
            <Input label={t('teams.maxSessions')} name="maxSessions" type="number" value={s.maxSessions} onChange={(e) => set('maxSessions', Number(e.target.value))} className="font-mono" hint={t('teams.maxSessionsHint')} />
          </div>
          <div className="flex flex-col gap-2">
            <span className="text-sm font-medium text-ink-2">{t('teams.codecs')}</span>
            <div className="flex flex-wrap gap-2">
              {['G.722', 'G.711 A-law', 'G.711 µ-law'].map((c, i) => <Badge key={c} tone="accent">{i + 1} · {c}</Badge>)}
              <Badge>SRTP · {t('teams.required')}</Badge>
            </div>
          </div>
          <Toggle label={t('teams.ringBoth')} hint={t('teams.ringBothHint')} checked={s.ringBoth} onChange={(v) => set('ringBoth', v)} />
          <p className="text-xs text-subtle">{t('teams.bypassNote')}</p>
          <div className="flex justify-end">
            <Button busy={busy} onClick={() => { void onSave() }}>{t('common.save')}</Button>
          </div>
        </Card>
        <div className="flex flex-col gap-5">
          <Card className="flex flex-col gap-3">
            <h2 className="font-display text-lg font-semibold">{t('teams.hubsTitle')}</h2>
            {st.hubs.map((h) => {
              const tone = h.status === 'reachable' ? 'ok' : h.status === 'off' || h.status === 'unknown' ? 'muted' : 'danger'
              return (
                <div key={h.host} className="flex items-center gap-3 rounded-xl border border-line bg-inset px-3 py-2.5">
                  <span className={`size-2 flex-none rounded-full ${tone === 'ok' ? 'bg-ok shadow-[0_0_10px_var(--color-ok)]' : tone === 'danger' ? 'bg-danger' : 'bg-subtle'}`} aria-hidden />
                  <span className="min-w-0 flex-1 truncate font-mono text-[12.5px] text-ink">{h.host}</span>
                  <span className="text-xs text-muted">{h.status === 'reachable' ? `${Math.round(h.rttMs)} ms` : t(`teams.hub.${h.status}`, { defaultValue: h.status })}</span>
                </div>
              )
            })}
            <p className="text-xs text-subtle">{t('teams.hubsHint')}</p>
          </Card>
          <Card className="flex flex-col gap-1">
            <h2 className="mb-2 font-display text-lg font-semibold">{t('teams.checkTitle')}</h2>
            {checks.map((c) => (
              <div key={c.title} className="flex items-start gap-3 py-2">
                <span className={`flex size-6 flex-none items-center justify-center rounded-lg text-[13px] font-bold ${c.ok ? 'bg-ok text-on-accent' : 'border border-warn text-warn-ink'}`} aria-hidden>{c.ok ? '✓' : '!'}</span>
                <span className="flex min-w-0 flex-col"><span className="text-sm font-medium text-ink">{c.title}</span><span className="text-xs text-muted">{c.sub}</span></span>
              </div>
            ))}
          </Card>
        </div>
      </div>
    </>
  )
}

function Kpi({ label, value, sub }: { label: string; value: React.ReactNode; sub: string }) {
  return (
    <section className="card flex flex-col gap-1.5 p-4">
      <span className="text-[11px] uppercase tracking-[0.08em] text-subtle">{label}</span>
      <span className="text-xl font-semibold text-ink">{value}</span>
      <span className="text-xs text-muted">{sub}</span>
    </section>
  )
}

function Users({ s, exts, error, onSave }: { s: TeamsSettings; exts: Extension[]; error: unknown; onSave: (v: TeamsSettings) => Promise<unknown> }) {
  const { t } = useTranslation()
  const [edit, setEdit] = useState<{ index: number; user: TeamsUser } | null>(null)
  const byNumber = useMemo(() => new Map(exts.map((e) => [e.number, e])), [exts])
  async function remove(i: number) {
    if (!window.confirm(t('common.confirmDelete', { name: s.users[i].name || s.users[i].upn }))) return
    await onSave({ ...s, users: s.users.filter((_, j) => j !== i) })
  }
  return (
    <Card className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="max-w-2xl">
          <h2 className="font-display text-lg font-semibold">{t('teams.usersTitle')}</h2>
          <p className="mt-0.5 text-sm text-muted">{t('teams.usersIntro')}</p>
        </div>
        <Button onClick={() => setEdit({ index: -1, user: { name: '', upn: '', number: '', extension: '', ring: s.ringBoth ? 'both' : 'teams' } })}>
          <Plus className="size-4" aria-hidden />{t('teams.addUser')}
        </Button>
      </div>
      {!edit && <TeamsError error={error} />}
      <Table head={[t('teams.colName'), t('teams.colUpn'), t('teams.colNumber'), t('teams.colExt'), t('teams.colRing'), '']}
        empty={s.users.length === 0 && <Empty icon={<UsersRound className="size-7" />} title={t('teams.noUsers')} text={t('teams.noUsersText')} />}>
        {s.users.map((u, i) => (
          <tr key={u.upn}>
            <td className={`${td} text-ink`}>{u.name || byNumber.get(u.extension)?.name || '–'}</td>
            <td className={`${td} font-mono text-xs`}>{u.upn}</td>
            <td className={`${td} font-mono text-xs`}>{u.number}</td>
            <td className={td}>{u.extension || '–'}</td>
            <td className={td}><Badge tone={u.ring === 'both' ? 'accent' : 'muted'}>{u.ring === 'both' ? t('teams.ringBothShort') : t('teams.ringTeams')}</Badge></td>
            <td className={`${td} whitespace-nowrap text-right`}>
              <IconButton label={t('common.edit')} onClick={() => setEdit({ index: i, user: u })}><Pencil className="size-4" aria-hidden /></IconButton>
              <IconButton label={t('common.delete')} danger onClick={() => remove(i)}><Trash2 className="size-4" aria-hidden /></IconButton>
            </td>
          </tr>
        ))}
      </Table>
      <div className="grid gap-3 md:grid-cols-2">
        <div className="rounded-xl border border-line bg-inset px-4 py-3"><div className="text-sm font-semibold">{t('teams.ringModes')}</div><p className="mt-1 text-xs text-muted">{t('teams.ringModesHint')}</p></div>
        <div className="rounded-xl border border-line bg-inset px-4 py-3"><div className="text-sm font-semibold">{t('teams.features')}</div><p className="mt-1 text-xs text-muted">{t('teams.featuresHint')}</p></div>
      </div>
      {edit && <UserForm value={edit.user} isNew={edit.index < 0} exts={exts} taken={s.users.filter((_, j) => j !== edit.index).map((u) => u.extension).filter(Boolean)}
        onClose={() => setEdit(null)}
        onSave={async (u) => {
          const users = edit.index < 0 ? [...s.users, u] : s.users.map((x, j) => (j === edit.index ? u : x))
          return onSave({ ...s, users })
        }} done={() => setEdit(null)} />}
    </Card>
  )
}

function UserForm({ value, isNew, exts, taken, onClose, onSave, done }: {
  value: TeamsUser; isNew: boolean; exts: Extension[]; taken: string[]; onClose: () => void; onSave: (u: TeamsUser) => Promise<unknown>; done: () => void
}) {
  const { t } = useTranslation()
  const [u, setU] = useState(value)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const set = <K extends keyof TeamsUser>(k: K, v: TeamsUser[K]) => setU((x) => ({ ...x, [k]: v }))
  async function submit() {
    setBusy(true)
    setError(null)
    try {
      const err = await onSave(u)
      if (err) setError(err)
      else done()
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={isNew ? t('teams.addUser') : t('teams.editUser', { name: value.name || value.upn })} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={submit}>{t('common.save')}</Button></>}>
      <TeamsError error={error} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Input label={t('teams.colName')} value={u.name} onChange={(e) => set('name', e.target.value)} autoFocus />
        <Input label={t('teams.colUpn')} type="email" value={u.upn} onChange={(e) => set('upn', e.target.value.trim())} placeholder="name@example.com" className="font-mono" hint={t('teams.upnHint')} />
        <Input label={t('teams.colNumber')} value={u.number} inputMode="tel" onChange={(e) => set('number', e.target.value.replace(/[^0-9+ ()-]/g, ''))} placeholder="+31 20 123 4567" className="font-mono" hint={t('teams.numberHint')} />
        <Select label={t('teams.colExt')} value={u.extension} onChange={(e) => setU({ ...u, extension: e.target.value, ring: e.target.value ? u.ring : 'teams' })} hint={t('teams.extHint')}>
          <option value="">{t('teams.noExt')}</option>
          {exts.filter((e) => !taken.includes(e.number)).map((e) => <option key={e.id} value={e.number}>{e.number} · {e.name}</option>)}
        </Select>
      </div>
      <Select label={t('teams.colRing')} value={u.extension ? u.ring : 'teams'} disabled={!u.extension} onChange={(e) => set('ring', e.target.value as TeamsUser['ring'])}>
        <option value="both">{t('teams.ringBothShort')}</option>
        <option value="teams">{t('teams.ringTeams')}</option>
      </Select>
    </Modal>
  )
}

function Setup({ s }: { s: TeamsSettings }) {
  const { t, i18n } = useTranslation()
  const lang = i18n.language.startsWith('nl') ? 'nl' : 'en'
  const [script, setScript] = useState('')
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState<unknown>(null)
  useEffect(() => {
    let live = true
    fetch(`/api/teams/script?lang=${lang}`, { credentials: 'same-origin' })
      .then((r) => (r.ok ? r.text() : Promise.reject(new ApiError(r.status, 'internal'))))
      .then((txt) => live && setScript(txt.replace(/^﻿/, '')))
      .catch((e) => live && setError(e))
    return () => { live = false }
  }, [lang, s])
  const steps = [1, 2, 3, 4].map((n) => ({ n, title: t(`teams.p${n}`), sub: t(`teams.p${n}s`) }))
  return (
    <div className="grid items-start gap-5 xl:grid-cols-[2fr_3fr]">
      <Card className="flex flex-col gap-1">
        <h2 className="mb-2 font-display text-lg font-semibold">{t('teams.preTitle')}</h2>
        {steps.map((p) => (
          <div key={p.n} className="flex items-start gap-3 border-b border-line py-2.5 last:border-0">
            <span className="flex size-6 flex-none items-center justify-center rounded-lg border border-line bg-inset text-xs font-semibold text-accent-ink" aria-hidden>{p.n}</span>
            <span className="flex flex-col"><span className="text-sm font-medium text-ink">{p.title}</span><span className="text-xs text-muted">{p.sub}</span></span>
          </div>
        ))}
      </Card>
      <Card className="flex min-w-0 flex-col gap-4">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="max-w-xl">
            <h2 className="font-display text-lg font-semibold">{t('teams.scriptTitle')}</h2>
            <p className="mt-0.5 text-sm text-muted">{t('teams.scriptIntro')}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <a href={`/api/teams/script?lang=${lang}`} download="kiestoon-teams.ps1" className="inline-flex min-h-11 items-center gap-2 rounded-[10px] border border-line px-4 text-sm font-medium text-ink-2 hover:bg-tint/5">
              <Download className="size-4" aria-hidden />{t('teams.download')}
            </a>
            <Button disabled={!script} onClick={() => navigator.clipboard?.writeText(script).then(() => { setCopied(true); window.setTimeout(() => setCopied(false), 1500) })}>
              {copied ? <Check className="size-4" aria-hidden /> : <Copy className="size-4" aria-hidden />}{copied ? t('common.copied') : t('teams.copy')}
            </Button>
          </div>
        </div>
        <ErrorMessage error={error} />
        <pre className="max-h-[520px] overflow-auto rounded-xl border border-line bg-inset p-4 font-mono text-xs leading-relaxed text-ink-2">{script}</pre>
        <Alert tone="warn">{t('teams.scriptNote')}</Alert>
      </Card>
    </div>
  )
}
