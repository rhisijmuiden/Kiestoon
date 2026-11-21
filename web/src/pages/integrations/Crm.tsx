import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { ArrowDown, ArrowUp, LogIn, RefreshCw, Search, Settings2, ShieldCheck, Unplug } from 'lucide-react'
import { Badge, CopyField, Modal, Select, Table, Toggle, formatDate, td, useApi } from '../../components/kit'
import { Alert, Button, Card, ErrorMessage, Input } from '../../components/ui'
import { api } from '../../lib/api'
import { crmColors, crmKinds, crmNames, type CrmConfig, type CrmKind, type CrmLogEntry, type CrmOverview, type CrmOwner, type CrmOwnerRow, type CrmResult, type CrmSettings, type CrmStatus } from '../../lib/crm'

function Logo({ kind, big }: { kind: CrmKind; big?: boolean }) {
  return (
    <span className={`grid ${big ? 'size-11 rounded-xl text-base' : 'size-5 rounded-[5px] text-[10px]'} flex-none place-items-center font-display font-bold text-white`} style={{ background: crmColors[kind] }} aria-hidden>
      {crmNames[kind][0]}
    </span>
  )
}

/** Integrations → CRM: connect HubSpot, Salesforce, Pipedrive or Exact Online. */
export function Crm() {
  const { t } = useTranslation()
  const data = useApi<CrmOverview>('/crm')
  const [edit, setEdit] = useState<CrmKind | null>(null)
  const [params, setParams] = useSearchParams()
  const exact = params.get('exact')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<unknown>(null)

  useEffect(() => {
    if (exact === 'ok') data.reload()
  }, [exact]) // eslint-disable-line react-hooks/exhaustive-deps

  async function save(st: CrmSettings) {
    setSaving(true)
    setError(null)
    try {
      data.setData(await api<CrmOverview>('PUT', '/crm', st))
    } catch (e) {
      setError(e)
      throw e
    } finally {
      setSaving(false)
    }
  }

  if (!data.data) return <ErrorMessage error={data.error} />
  const { settings, status } = data.data
  const byKind = Object.fromEntries(status.map((s) => [s.kind, s])) as Record<CrmKind, CrmStatus>
  const move = (i: number, d: number) => {
    const o = [...settings.order]
    const j = i + d
    if (j < 0 || j >= o.length) return
    ;[o[i], o[j]] = [o[j], o[i]]
    save({ ...settings, order: o }).catch(() => undefined)
  }
  const active = settings.order.filter((k) => byKind[k]?.connected)

  return (
    <>
      {exact && (
        <Alert tone={exact === 'ok' ? 'ok' : 'danger'}>
          <span className="flex flex-wrap items-center justify-between gap-2">
            {t(`crm.exactResult.${exact}`, { defaultValue: t('crm.exactResult.failed') })}
            <button type="button" className="text-xs underline" onClick={() => setParams((p) => { p.delete('exact'); return p })}>{t('common.close')}</button>
          </span>
        </Alert>
      )}
      <ErrorMessage error={error} />
      <div className="grid gap-4 lg:grid-cols-2">
        {crmKinds.map((k) => <CrmCard key={k} st={byKind[k]} enabled={settings.connectors[k].enabled} onEdit={() => setEdit(k)} />)}
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card className="flex flex-col gap-4">
          <h2 className="font-display text-lg font-semibold">{t('crm.general')}</h2>
          <div className="flex flex-col gap-1.5">
            <span className="text-sm font-medium text-ink-2">{t('crm.order')}</span>
            <ol className="flex flex-col gap-1.5">
              {settings.order.map((k, i) => (
                <li key={k} className={`flex items-center gap-3 rounded-xl border border-line bg-surface-2/60 px-3 py-2 ${byKind[k]?.connected ? '' : 'opacity-55'}`}>
                  <span className="w-4 font-mono text-xs text-subtle">{i + 1}</span>
                  <Logo kind={k} />
                  <span className="flex-1 text-sm text-ink">{crmNames[k]}</span>
                  {!byKind[k]?.connected && <span className="text-xs text-subtle">{t('crm.notConnected')}</span>}
                  <button type="button" disabled={i === 0 || saving} onClick={() => move(i, -1)} aria-label={t('crm.moveUp', { name: crmNames[k] })} className="inline-flex size-8 items-center justify-center rounded-lg text-muted hover:bg-tint/5 disabled:opacity-30"><ArrowUp className="size-4" aria-hidden /></button>
                  <button type="button" disabled={i === settings.order.length - 1 || saving} onClick={() => move(i, 1)} aria-label={t('crm.moveDown', { name: crmNames[k] })} className="inline-flex size-8 items-center justify-center rounded-lg text-muted hover:bg-tint/5 disabled:opacity-30"><ArrowDown className="size-4" aria-hidden /></button>
                </li>
              ))}
            </ol>
            <p className="text-xs text-subtle">{t('crm.orderHint')}</p>
          </div>
          <Toggle label={t('crm.deskName')} hint={t('crm.deskNameHint')} checked={settings.deskName} disabled={saving} onChange={(v) => save({ ...settings, deskName: v }).catch(() => undefined)} />
          <Toggle label={t('crm.popupAll')} hint={t('crm.popupAllHint')} checked={settings.popupAll} disabled={saving} onChange={(v) => save({ ...settings, popupAll: v }).catch(() => undefined)} />
        </Card>
        <div className="flex flex-col gap-4">
          <div className="flex gap-3 rounded-2xl border border-dashed border-accent/40 bg-accent/[0.06] p-4 text-[13px] text-ink-2">
            <ShieldCheck className="mt-0.5 size-4 flex-none text-accent" aria-hidden />
            <p>{t('crm.privacy')}</p>
          </div>
          {active.length > 0 && <TestLookup />}
        </div>
      </div>
      {active.length > 0 && <SyncLog />}
      {edit && <CrmForm kind={edit} overview={data.data} onClose={() => setEdit(null)} onChange={(o) => data.setData(o)} />}
    </>
  )
}

function CrmCard({ st, enabled, onEdit }: { st: CrmStatus; enabled: boolean; onEdit: () => void }) {
  const { t } = useTranslation()
  const badge = st.loginExpired ? <Badge tone="danger">{t('crm.signInAgain')}</Badge>
    : st.connected ? <Badge tone="ok" dot>{t('crm.connected')}</Badge>
    : st.configured && !enabled ? <Badge>{t('crm.paused')}</Badge>
    : <Badge>{t('crm.notConnected')}</Badge>
  const hit = st.lookups ? Math.round((st.hits / st.lookups) * 100) : 0
  return (
    <Card className="flex flex-col gap-3">
      <div className="flex items-center gap-3">
        <Logo kind={st.kind} big />
        <div className="min-w-0 flex-1">
          <h2 className="font-display text-base font-semibold text-ink">{st.name}</h2>
          <p className="text-xs text-subtle">{st.account || t(`crm.how.${st.kind}`)}</p>
        </div>
        {badge}
      </div>
      {st.configured && (
        <div className="grid grid-cols-3 gap-2">
          {[[st.lookups, t('crm.stat.lookups')], [st.logged, t('crm.stat.logged')], [`${hit}%`, t('crm.stat.hits')]].map(([v, l]) => (
            <div key={String(l)} className="min-w-0 rounded-xl border border-line bg-inset px-3 py-2">
              <b className="block font-display text-lg font-semibold tabular-nums">{v}</b>
              <span className="text-[11.5px] text-subtle">{l}</span>
            </div>
          ))}
        </div>
      )}
      {st.loginExpired && <p className="text-[13px] font-medium text-danger-ink">{t('crm.expiredText', { name: st.name })}</p>}
      {st.failed > 0 && !st.loginExpired && <p className="text-[13px] text-warn-ink">{t('crm.failedToday', { count: st.failed })}</p>}
      <div>
        <Button variant={st.configured ? 'ghost' : 'primary'} onClick={onEdit}>
          {st.configured ? <><Settings2 className="size-4" aria-hidden />{t('crm.settings')}</> : t('crm.connect')}
        </Button>
      </div>
    </Card>
  )
}

const showFields = ['deals', 'tickets', 'last', 'owner', 'lifecycle'] as const

function CrmForm({ kind, overview, onClose, onChange }: { kind: CrmKind; overview: CrmOverview; onClose: () => void; onChange: (o: CrmOverview) => void }) {
  const { t } = useTranslation()
  const st = overview.status.find((s) => s.kind === kind)!
  const [cfg, setCfg] = useState<CrmConfig>(overview.settings.connectors[kind])
  const [secret, setSecret] = useState('')
  const [busy, setBusy] = useState('')
  const [error, setError] = useState<unknown>(null)
  const [msg, setMsg] = useState<{ tone: 'ok' | 'danger'; text: string } | null>(null)
  const set = <K extends keyof CrmConfig>(k: K, v: CrmConfig[K]) => setCfg({ ...cfg, [k]: v })
  const tokenKind = kind === 'hubspot' || kind === 'pipedrive'

  async function run<T>(what: string, f: () => Promise<T>) {
    setBusy(what)
    setError(null)
    setMsg(null)
    try {
      return await f()
    } catch (e) {
      setError(e)
      return undefined
    } finally {
      setBusy('')
    }
  }
  const saveSettings = (c: CrmConfig) => api<CrmOverview>('PUT', '/crm', { ...overview.settings, connectors: { ...overview.settings.connectors, [kind]: c } })

  async function save() {
    const o = await run('save', async () => {
      let o = await saveSettings(cfg)
      if (secret.trim()) o = await api<CrmOverview>('PUT', `/crm/${kind}/secret`, tokenKind ? { token: secret } : { clientSecret: secret })
      return o
    })
    if (o) {
      onChange(o)
      setSecret('')
      const s = o.status.find((x) => x.kind === kind)!
      if (kind === 'exact' && s.hasSecret && !s.configured) setMsg({ tone: 'ok', text: t('crm.exactNext') })
      else onClose()
    }
  }
  async function exactLogin() {
    const r = await run('login', async () => {
      const o = await saveSettings({ ...cfg, enabled: true })
      if (secret.trim()) await api('PUT', `/crm/exact/secret`, { clientSecret: secret })
      onChange(o)
      return api<{ url: string }>('POST', '/crm/exact/login')
    })
    if (r) window.location.href = r.url
  }
  async function test() {
    const r = await run('test', () => api<{ ok: boolean; account?: string; error?: string }>('POST', `/crm/${kind}/test`))
    if (r) setMsg(r.ok ? { tone: 'ok', text: t('crm.testOk', { account: r.account }) } : { tone: 'danger', text: t('crm.testFailed', { error: t(`crm.err.${r.error}`, { defaultValue: r.error }) }) })
  }
  async function disconnect() {
    if (!window.confirm(t('crm.confirmDisconnect', { name: crmNames[kind] }))) return
    const o = await run('disconnect', () => api<CrmOverview>('DELETE', `/crm/${kind}/secret`))
    if (o) { onChange(o); onClose() }
  }

  return (
    <Modal wide title={crmNames[kind]} subtitle={t('crm.formSubtitle')} onClose={onClose}
      footer={<>
        {st.configured && <Button variant="danger" busy={busy === 'disconnect'} onClick={disconnect} className="mr-auto"><Unplug className="size-4" aria-hidden />{t('crm.disconnect')}</Button>}
        <Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button>
        <Button busy={busy === 'save'} onClick={save}>{t('common.save')}</Button>
      </>}>
      <ErrorMessage error={error} />
      {msg && <Alert tone={msg.tone}>{msg.text}</Alert>}
      <section className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="font-display text-base font-semibold">{t('crm.connection')}</h3>
          {st.loginExpired ? <Badge tone="danger">{t('crm.signInAgain')}</Badge> : st.connected ? <Badge tone="ok" dot>{t('crm.connected')}</Badge> : null}
        </div>
        <Toggle label={t('crm.enable', { name: crmNames[kind] })} hint={t('crm.enableHint')} checked={cfg.enabled} onChange={(v) => set('enabled', v)} />
        <p className="text-xs text-subtle">{t(`crm.help.${kind}`)}</p>
        <div className="grid gap-3 sm:grid-cols-2">
          {kind === 'pipedrive' && <Input name="pipedrive.domain" label={t('crm.pdDomain')} hint={t('crm.pdDomainHint')} value={cfg.domain ?? ''} onChange={(e) => set('domain', e.target.value)} placeholder="acme" />}
          {kind === 'salesforce' && <>
            <Input name="salesforce.domain" label={t('crm.sfDomain')} hint={t('crm.sfDomainHint')} value={cfg.domain ?? ''} onChange={(e) => set('domain', e.target.value)} placeholder="https://acme.my.salesforce.com" />
            <Input name="salesforce.clientId" label={t('crm.sfClientId')} value={cfg.clientId ?? ''} onChange={(e) => set('clientId', e.target.value)} className="font-mono text-sm" />
          </>}
          {kind === 'exact' && <>
            <Select name="exact.region" label={t('crm.exRegion')} value={cfg.region ?? 'nl'} onChange={(e) => set('region', e.target.value)}>
              {['nl', 'be', 'de', 'uk', 'fr', 'es', 'us'].map((r) => <option key={r} value={r}>{t(`crm.regions.${r}`)}</option>)}
            </Select>
            <Input name="exact.clientId" label={t('crm.exClientId')} value={cfg.clientId ?? ''} onChange={(e) => set('clientId', e.target.value)} className="font-mono text-sm" />
          </>}
          <Input name={tokenKind ? 'token' : 'clientSecret'} type="password" autoComplete="off" label={tokenKind ? t('crm.token') : t('crm.clientSecret')}
            hint={st.hasSecret ? t('crm.secretStored') : t('crm.secretHint')} value={secret} onChange={(e) => setSecret(e.target.value)}
            placeholder={st.hasSecret ? '••••••••••••' : ''} className="font-mono text-sm" />
        </div>
        {kind === 'exact' && <CopyField label={t('crm.exRedirect')} value={overview.exactRedirect} />}
        <div className="flex flex-wrap gap-2">
          {kind === 'exact' && <Button variant="secondary" busy={busy === 'login'} disabled={!cfg.clientId || (!st.hasSecret && !secret)} onClick={exactLogin}><LogIn className="size-4" aria-hidden />{st.configured ? t('crm.exLoginAgain') : t('crm.exLogin')}</Button>}
          {st.configured && <Button variant="ghost" busy={busy === 'test'} onClick={test}><RefreshCw className="size-4" aria-hidden />{t('crm.test')}</Button>}
        </div>
      </section>

      <section className="flex flex-col gap-3">
        <h3 className="font-display text-base font-semibold">{t('crm.recognise')}</h3>
        <div className="grid gap-3 sm:grid-cols-2">
          <Toggle label={t('crm.lookupContacts')} hint={t('crm.lookupContactsHint')} checked={cfg.lookupContacts} onChange={(v) => set('lookupContacts', v)} />
          <Toggle label={t('crm.lookupCompanies')} hint={kind === 'pipedrive' ? t('crm.pdNoCompanies') : t('crm.lookupCompaniesHint')} checked={cfg.lookupCompanies} disabled={kind === 'pipedrive'} onChange={(v) => set('lookupCompanies', v)} />
        </div>
        <div className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-ink-2">{t('crm.showInPopup')}</span>
          <div className="flex flex-wrap gap-1.5">
            {showFields.map((f) => {
              const on = cfg.show.includes(f)
              return (
                <button key={f} type="button" aria-pressed={on} onClick={() => set('show', on ? cfg.show.filter((x) => x !== f) : [...cfg.show, f])}
                  className={`rounded-full border px-3 py-1.5 text-[12.5px] font-medium ${on ? 'border-accent/55 bg-accent/15 text-accent-ink' : 'border-line bg-field text-ink-2'}`}>
                  {t(`crm.show.${f}`)}
                </button>
              )
            })}
          </div>
        </div>
      </section>

      <section className="flex flex-col gap-3">
        <h3 className="font-display text-base font-semibold">{t('crm.logging')}</h3>
        <Toggle label={t('crm.logCalls')} hint={t(`crm.logCallsHint.${kind}`)} checked={cfg.logCalls} onChange={(v) => set('logCalls', v)} />
        <Select name={`${kind}.logWhich`} label={t('crm.logWhich')} value={cfg.logWhich} disabled={!cfg.logCalls} onChange={(e) => set('logWhich', e.target.value as CrmConfig['logWhich'])}>
          {(['all', 'inbound', 'answered'] as const).map((w) => <option key={w} value={w}>{t(`crm.which.${w}`)}</option>)}
        </Select>
        <div className="grid gap-3 sm:grid-cols-2">
          <Toggle label={t('crm.missedTask')} hint={t('crm.missedTaskHint')} checked={cfg.missedTask} onChange={(v) => set('missedTask', v)} />
          <Toggle label={t('crm.recordingLink')} hint={t('crm.recordingLinkHint')} checked={cfg.recordingLink} onChange={(v) => set('recordingLink', v)} />
          <Toggle label={t('crm.aiSummary')} hint={overview.aiCalls ? t('crm.aiSummaryHint') : t('crm.aiSummaryOff')} checked={cfg.aiSummary} onChange={(v) => set('aiSummary', v)} />
          <Toggle label={t('crm.createUnknown')} hint={t('crm.createUnknownHint')} checked={cfg.createUnknown} onChange={(v) => set('createUnknown', v)} />
        </div>
      </section>

      {st.configured && !st.loginExpired && <Owners kind={kind} cfg={cfg} onChange={(owners) => set('owners', owners)} />}
    </Modal>
  )
}

function Owners({ kind, cfg, onChange }: { kind: CrmKind; cfg: CrmConfig; onChange: (o: Record<string, string>) => void }) {
  const { t } = useTranslation()
  const list = useApi<{ owners: CrmOwner[]; users: CrmOwnerRow[]; error?: string }>(`/crm/${kind}/owners`)
  const owners = list.data?.owners ?? []
  const pick = (u: CrmOwnerRow, id: string) => {
    const o = { ...(cfg.owners ?? {}) }
    if (id) o[String(u.id)] = id
    else delete o[String(u.id)]
    onChange(o)
  }
  return (
    <section className="flex flex-col gap-2">
      <h3 className="font-display text-base font-semibold">{t('crm.owners', { name: crmNames[kind] })}</h3>
      <p className="text-xs text-subtle">{t('crm.ownersHint')}</p>
      {list.data?.error && <Alert tone="danger">{t('crm.ownersFailed')}</Alert>}
      <Table head={[t('crm.user'), t('crm.owner', { name: crmNames[kind] }), t('crm.match')]}
        empty={list.data?.users.length === 0 && <p className="py-4 text-center text-sm text-muted">{t('crm.noUsers')}</p>}>
        {list.data?.users.map((u) => {
          const manual = cfg.owners?.[String(u.id)] ?? ''
          const auto = u.match === 'email' ? u.ownerId : ''
          const value = manual || auto
          const match = manual ? 'manual' : auto ? 'email' : 'none'
          return (
            <tr key={u.id}>
              <td className={`${td} text-ink`}>{u.name}{u.ext && <span className="ml-1.5 font-mono text-xs text-subtle">{u.ext}</span>}</td>
              <td className={td}>
                <select aria-label={t('crm.owner', { name: crmNames[kind] })} value={value} onChange={(e) => pick(u, e.target.value === auto ? '' : e.target.value)}
                  className="min-h-9 w-full max-w-[16rem] rounded-lg border border-line bg-field px-2 text-sm text-ink">
                  <option value="">—</option>
                  {owners.map((o) => <option key={o.id} value={o.id}>{o.name || o.email}</option>)}
                </select>
              </td>
              <td className={td}>{match === 'none' ? <Badge tone="warn">{t('crm.matchNone')}</Badge> : <Badge>{t(`crm.matchKind.${match}`)}</Badge>}</td>
            </tr>
          )
        })}
      </Table>
    </section>
  )
}

function TestLookup() {
  const { t } = useTranslation()
  const [number, setNumber] = useState('')
  const [busy, setBusy] = useState(false)
  const [res, setRes] = useState<{ result: CrmResult; error?: string } | null>(null)
  const [error, setError] = useState<unknown>(null)
  async function go() {
    setBusy(true)
    setError(null)
    try {
      setRes(await api('POST', '/crm/lookup', { number }))
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const c = res?.result?.contacts?.[0]
  return (
    <Card className="flex flex-col gap-3">
      <h2 className="font-display text-lg font-semibold">{t('crm.testLookup')}</h2>
      <form className="flex items-end gap-2" onSubmit={(e) => { e.preventDefault(); go() }}>
        <div className="flex-1"><Input name="number" label={t('crm.testNumber')} value={number} onChange={(e) => setNumber(e.target.value)} placeholder="06 1234 5678" inputMode="tel" /></div>
        <Button type="submit" busy={busy} disabled={!number.trim()}><Search className="size-4" aria-hidden />{t('crm.lookup')}</Button>
      </form>
      <ErrorMessage error={error} />
      {res && (c ? (
        <div className="flex items-center gap-3 rounded-xl border border-ok/35 bg-ok/[0.06] px-3 py-2.5 text-sm">
          <Logo kind={c.crm} />
          <span className="min-w-0 flex-1"><b className="text-ink">{c.name}</b>{c.company && c.company !== c.name && <span className="text-muted"> · {c.company}</span>}</span>
          {c.url && <a href={c.url} target="_blank" rel="noopener noreferrer" className="text-xs text-accent-ink underline">{t('crm.open')}</a>}
        </div>
      ) : (
        <p className="text-sm text-muted">{res.error ? t('crm.testFailed', { error: t(`crm.err.${res.error}`, { defaultValue: res.error }) }) : t('crm.notFound')}</p>
      ))}
    </Card>
  )
}

function SyncLog() {
  const { t, i18n } = useTranslation()
  const log = useApi<{ entries: CrmLogEntry[]; pending: number; failed: number }>('/crm/log', 15000)
  async function retry(id: number) {
    await api('POST', `/crm/calls/${id}/retry`).catch(() => undefined)
    log.reload()
  }
  return (
    <Card>
      <div className="mb-3 flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="font-display text-lg font-semibold">{t('crm.syncLog')}</h2>
          <p className="text-xs text-subtle">{t('crm.syncHint')}</p>
        </div>
        <div className="flex gap-2">
          {!!log.data?.pending && <Badge tone="accent">{t('crm.pending', { count: log.data.pending })}</Badge>}
          {!!log.data?.failed && <Badge tone="danger">{t('crm.failed', { count: log.data.failed })}</Badge>}
        </div>
      </div>
      <Table head={[t('crm.time'), 'CRM', t('crm.action'), t('crm.result'), '']}
        empty={log.data?.entries.length === 0 && <p className="py-6 text-center text-sm text-muted">{t('crm.noLog')}</p>}>
        {log.data?.entries.slice(0, 50).map((e) => (
          <tr key={e.id}>
            <td className={`${td} whitespace-nowrap font-mono text-xs`}>{formatDate(e.at, i18n.language)}</td>
            <td className={td}><span className="inline-flex items-center gap-1.5 text-xs"><Logo kind={e.crm} />{crmNames[e.crm]}</span></td>
            <td className={td}><span className="text-ink">{t(`crm.actions.${e.action}`, { defaultValue: e.action })}</span> <span className="text-xs text-subtle">{e.detail}</span></td>
            <td className={td}>{e.ok ? <span className="text-xs font-medium text-ok-ink">OK · {e.durationMs} ms</span> : <span className="text-xs font-medium text-danger-ink">{t(`crm.err.${e.error}`, { defaultValue: e.error })}</span>}</td>
            <td className={`${td} text-right`}>{!e.ok && e.callId > 0 && ['failed', 'retry', 'lookup'].includes(e.callState) && <Button variant="ghost" onClick={() => retry(e.callId)}>{t('crm.retryNow')}</Button>}</td>
          </tr>
        ))}
      </Table>
    </Card>
  )
}
