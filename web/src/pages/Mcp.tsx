import { useMemo, useState, type ReactNode } from 'react'
import { useSearchParams } from 'react-router'
import { useTranslation } from 'react-i18next'
import { Bot, Check, Copy, Globe, KeyRound, MessageSquare, Plug, Plus, ScrollText, ShieldCheck, Sparkles, Terminal, Trash2 } from 'lucide-react'
import { Badge, CopyField, Empty, Modal, Select, Stat, Table, Tabs, Textarea, formatDate, td, useApi } from '../components/kit'
import { IconButton } from '../components/pbx'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api, roleRank } from '../lib/api'
import type { McpCall, McpConnection, McpKind, McpOverview, McpScope, McpSettings, McpTool } from '../lib/types'

type Tab = 'overview' | 'connections' | 'tools' | 'activity' | 'connect'

const kindTone: Record<McpKind, 'ok' | 'accent' | 'warn'> = { read: 'ok', act: 'accent', write: 'warn' }
export const scopeKey = (k: string) => k.replace(':', '_')

export function KindBadge({ kind }: { kind: McpKind }) {
  const { t } = useTranslation()
  return <Badge tone={kindTone[kind]}>{t(`mcp.kind.${kind}`)}</Badge>
}

function ScopeChip({ scope, scopes }: { scope: string; scopes?: McpScope[] }) {
  const { t } = useTranslation()
  const kind = scopes?.find((s) => s.key === scope)?.kind ?? (scope.endsWith(':read') ? 'read' : scope.endsWith(':act') ? 'act' : 'write')
  const [title] = t(`mcp.scope.${scopeKey(scope)}`, { returnObjects: true }) as string[]
  return <Badge tone={kindTone[kind]}>{title ?? scope}</Badge>
}

/** The MCP server: AI assistants such as Claude connected to the phone system. mine = a user's own connections. */
export function Mcp({ mine = false }: { mine?: boolean }) {
  const { t } = useTranslation()
  const ov = useApi<McpOverview>('/mcp/overview')
  const [params, setParams] = useSearchParams()
  const admin = !mine && !!ov.data?.settings
  const tabs: Tab[] = admin ? ['overview', 'connections', 'tools', 'activity', 'connect'] : ['connections', 'connect', 'activity']
  const p = params.get('tab') as Tab | null
  const tab: Tab = p && tabs.includes(p) ? p : tabs[0]
  const setTab = (v: string) => setParams((q) => { q.set('tab', v); return q }, { replace: true })
  const [adding, setAdding] = useState(false)
  const [created, setCreated] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [version, setVersion] = useState(0)

  async function saveSettings(next: McpSettings) {
    setBusy(true)
    setError(null)
    try {
      await api('PUT', '/mcp/settings', next)
      await ov.reload()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }

  const o = ov.data
  const set = o?.settings
  const header = (
    <PageHeader title={t(mine ? 'mcp.mineTitle' : 'mcp.title')} subtitle={t(mine ? 'mcp.mineSubtitle' : 'mcp.subtitle')}>
      {admin && set && (
        <div className="flex items-center gap-3">
          <Badge tone={set.enabled ? 'ok' : 'muted'} dot>{set.enabled ? t('mcp.active') : t('mcp.off')}</Badge>
          <Switch label={set.enabled ? t('mcp.switchOff') : t('mcp.switchOn')} checked={set.enabled} disabled={busy} onChange={(v) => saveSettings({ ...set, enabled: v })} />
        </div>
      )}
    </PageHeader>
  )
  if (!o) return <>{header}<ErrorMessage error={ov.error} /></>
  return (
    <>
      {header}
      <ErrorMessage error={error} />
      {!o.enabled && <Alert tone="warn">{admin ? t('mcp.offText') : t('mcp.offUser')}</Alert>}
      {o.enabled && !o.allowUsers && !admin && <Alert tone="warn">{t('mcp.usersOff')}</Alert>}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Tabs value={tab} onChange={setTab} tabs={tabs.map((k) => ({ key: k, label: t(`mcp.tabs.${k}`) }))} />
        {o.canConnect && (
          <Button onClick={() => { setCreated(''); setAdding(true) }}><Plus className="size-4" aria-hidden />{t('mcp.newConnection')}</Button>
        )}
      </div>
      {created && (
        <Card>
          <div className="flex flex-col gap-3">
            <Alert tone="warn">{t('mcp.tokenOnce')}</Alert>
            <CopyField label={t('mcp.token')} value={created} />
            <CopyField label={t('mcp.command')} value={claudeCodeCommand(o.endpoint, created)} />
          </div>
        </Card>
      )}
      {tab === 'overview' && set && <Overview o={o} set={set} busy={busy} onSave={saveSettings} onAll={() => setTab('connections')} key={version} />}
      {tab === 'connections' && <Connections o={o} mine={mine} key={version} />}
      {tab === 'tools' && set && <Tools o={o} set={set} busy={busy} onSave={saveSettings} />}
      {tab === 'activity' && <Activity admin={admin} />}
      {tab === 'connect' && <Connect o={o} />}
      {adding && (
        <ConnectionForm o={o} onClose={() => setAdding(false)}
          onCreated={(tok) => { setAdding(false); setCreated(tok); setVersion((v) => v + 1); ov.reload(); if (tab !== 'connections' && tab !== 'overview') setTab('connections') }} />
      )}
    </>
  )
}

function Switch({ label, checked, disabled, onChange }: { label: string; checked: boolean; disabled?: boolean; onChange: (v: boolean) => void }) {
  return (
    <button type="button" role="switch" aria-checked={checked} aria-label={label} disabled={disabled} onClick={() => onChange(!checked)}
      className={`relative inline-flex h-6 w-11 flex-none items-center rounded-full border transition disabled:opacity-50 ${checked ? 'border-accent/60 bg-accent/30' : 'border-line bg-tint/5'}`}>
      <span className={`inline-block size-4 rounded-full transition ${checked ? 'translate-x-6 bg-accent shadow-[0_0_10px_var(--color-accent)]' : 'translate-x-1 bg-muted'}`} />
    </button>
  )
}

const claudeCodeCommand = (endpoint: string, token: string) => `claude mcp add --transport http kiestoon ${endpoint} --header "Authorization: Bearer ${token}"`

// ---- overview -----------------------------------------------------------------

const policyKeys = ['defaultReadOnly', 'confirmWrites', 'maskNumbers', 'shareMedia', 'anthropicOnly', 'allowUsers'] as const

function Overview({ o, set, busy, onSave, onAll }: { o: McpOverview; set: McpSettings; busy: boolean; onSave: (s: McpSettings) => void; onAll: () => void }) {
  const { t } = useTranslation()
  const st = o.stats
  return (
    <>
      {st && (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <Stat label={t('mcp.stats.connections')} value={st.connections} sub={t('mcp.stats.connectionsSub', { oauth: st.oauth, tokens: st.tokens })} />
          <Stat label={t('mcp.stats.calls')} value={st.calls} />
          <Stat label={t('mcp.stats.changes')} value={st.changes} tone={st.changes ? 'warn' : undefined} />
          <Stat label={t('mcp.stats.denied')} value={st.denied} tone={st.denied ? 'danger' : undefined} sub={t('mcp.stats.deniedSub')} />
        </div>
      )}
      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.8fr)_minmax(0,1fr)]">
        <Card>
          <div className="mb-3 flex items-center justify-between gap-3">
            <h2 className="font-display text-lg font-semibold">{t('mcp.activeConnections')}</h2>
            <Button variant="ghost" onClick={onAll}>{t('mcp.viewAll')}</Button>
          </div>
          <ConnectionTable o={o} limit={6} />
        </Card>
        <div className="flex flex-col gap-4">
          <Card>
            <h2 className="mb-3 font-display text-lg font-semibold">{t('mcp.serverAddress')}</h2>
            <CopyField label="URL" value={o.endpoint} />
            <dl className="mt-4 grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
              <Info label={t('mcp.transport')}>Streamable HTTP</Info>
              <Info label={t('mcp.protocol')}><span className="font-mono">MCP {o.version}</span></Info>
              <Info label={t('mcp.signIn')}>{t('mcp.signInValue')}</Info>
              <Info label={t('mcp.reachable')}>
                {o.internet ? <span className="inline-flex items-center gap-1 text-ok-ink"><Check className="size-4" aria-hidden />{t('mcp.reachableYes')}</span>
                  : <span className="text-warn-ink">{t('mcp.reachableNo')}</span>}
              </Info>
            </dl>
          </Card>
          <Card>
            <h2 className="mb-1 font-display text-lg font-semibold">{t('mcp.policy')}</h2>
            <div className="divide-y divide-line">
              {policyKeys.map((k) => {
                const [label, hint] = t(`mcp.policyItems.${k}`, { returnObjects: true }) as string[]
                return (
                  <div key={k} className="flex items-center gap-4 py-3">
                    <div className="min-w-0 flex-1">
                      <div className="text-sm font-medium text-ink">{label}</div>
                      <div className="mt-0.5 text-xs text-muted">{hint}</div>
                    </div>
                    <Switch label={label} checked={set[k]} disabled={busy} onChange={(v) => onSave({ ...set, [k]: v })} />
                  </div>
                )
              })}
            </div>
          </Card>
        </div>
      </div>
    </>
  )
}

function Info({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt className="text-[11px] uppercase tracking-[0.08em] text-subtle">{label}</dt>
      <dd className="mt-1 text-ink-2">{children}</dd>
    </div>
  )
}

// ---- connections ----------------------------------------------------------------

function clientIcon(c: McpConnection) {
  if (c.kind === 'oauth') return <Sparkles className="size-4" aria-hidden />
  if (/code|cli|terminal/i.test(c.name)) return <Terminal className="size-4" aria-hidden />
  return <KeyRound className="size-4" aria-hidden />
}

function ConnectionTable({ o, limit, mine }: { o: McpOverview; limit?: number; mine?: boolean }) {
  const { t, i18n } = useTranslation()
  const list = useApi<McpConnection[]>(mine ? '/mcp/connections?mine=1' : '/mcp/connections')
  const rows = limit ? list.data?.slice(0, limit) : list.data
  async function revoke(c: McpConnection) {
    if (!window.confirm(t('mcp.confirmRevoke', { name: c.name }))) return
    await api('DELETE', `/mcp/connections/${c.id}`)
    list.reload()
  }
  const expiredAt = (c: McpConnection) => c.expiresAt && new Date(c.expiresAt) < new Date()
  return (
    <>
      <ErrorMessage error={list.error} />
      <Table head={[t('mcp.client'), t('mcp.userRole'), t('mcp.scopes'), t('mcp.lastUsed'), '']}
        empty={list.data?.length === 0 && <Empty icon={<Bot className="size-7" />} title={t('mcp.noConnections')} text={t('mcp.noConnectionsText')} />}>
        {rows?.map((c) => (
          <tr key={c.id} className={expiredAt(c) ? 'opacity-50' : ''}>
            <td className={td}>
              <div className="flex items-center gap-3">
                <span className="flex size-8 flex-none items-center justify-center rounded-lg bg-accent-2/15 text-accent-2">{clientIcon(c)}</span>
                <div className="min-w-0">
                  <div className="font-medium text-ink">{c.name}</div>
                  <div className="font-mono text-[11px] text-subtle">{c.kind === 'oauth' ? 'OAuth' : `${c.prefix}…`}</div>
                </div>
              </div>
            </td>
            <td className={td}>
              <div className="text-ink">{c.username}</div>
              <div className="text-xs text-subtle">{t(`roles.${c.role}`)}</div>
            </td>
            <td className={`${td} max-w-[320px]`}>
              <div className="flex flex-wrap gap-1">{c.scopes.map((s) => <ScopeChip key={s} scope={s} scopes={o.scopes} />)}</div>
            </td>
            <td className={td}>
              <div className="flex items-center gap-2">
                <span className={`size-2 rounded-full ${c.lastUsedAt ? 'bg-ok' : 'bg-subtle'}`} aria-hidden />
                {c.lastUsedAt ? formatDate(c.lastUsedAt, i18n.language) : t('mcp.never')}
              </div>
              <div className="text-xs text-subtle">{t('mcp.callsToday', { count: c.callsToday })}{c.lastIp && <span className="ml-1 font-mono">· {c.lastIp}</span>}</div>
            </td>
            <td className={`${td} text-right`}>
              <IconButton label={t('mcp.revoke')} danger onClick={() => revoke(c)}><Trash2 className="size-4" aria-hidden /></IconButton>
            </td>
          </tr>
        ))}
      </Table>
    </>
  )
}

function Connections({ o, mine }: { o: McpOverview; mine: boolean }) {
  return <Card><ConnectionTable o={o} mine={mine} /></Card>
}

export function ScopeList({ scopes, value, onChange }: { scopes: McpScope[]; value: string[]; onChange: (v: string[]) => void }) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-col gap-2">
      {scopes.map((s) => {
        const [title, desc] = t(`mcp.scope.${scopeKey(s.key)}`, { returnObjects: true }) as string[]
        const on = value.includes(s.key)
        return (
          <label key={s.key} className={`flex cursor-pointer gap-3 rounded-xl border px-4 py-3 transition ${on ? 'border-accent/45 bg-accent/5' : 'border-line bg-surface-2/60'} ${s.available ? '' : 'cursor-not-allowed opacity-55'}`}>
            <input type="checkbox" className="mt-1 size-4 flex-none accent-[var(--color-accent)]" checked={on} disabled={!s.available}
              onChange={(e) => onChange(e.target.checked ? [...value, s.key] : value.filter((x) => x !== s.key))} />
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-medium text-ink">{title}</span>
                <KindBadge kind={s.kind} />
                <code className="font-mono text-[11px] text-subtle">{s.key}</code>
                {!s.available && <span className="text-xs text-warn-ink">{t('mcp.mediaOff')}</span>}
              </div>
              <p className="mt-0.5 text-xs text-muted">{desc}</p>
              {on && s.tools.length > 0 && (
                <div className="mt-2 flex flex-wrap gap-1">
                  {s.tools.map((tool) => <code key={tool} className="rounded-md border border-line bg-inset px-1.5 py-0.5 font-mono text-[11px] text-ink-2">{tool}</code>)}
                </div>
              )}
            </div>
          </label>
        )
      })}
    </div>
  )
}

function ConnectionForm({ o, onClose, onCreated }: { o: McpOverview; onClose: () => void; onCreated: (token: string) => void }) {
  const { t } = useTranslation()
  const defaults = o.scopes.filter((s) => s.available && (!o.defaultRead || s.kind === 'read')).map((s) => s.key)
  const [x, setX] = useState({ name: '', role: o.maxRole as string, expiresDays: 90, ips: '' })
  const [scopes, setScopes] = useState<string[]>(defaults)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  // The calls and media scopes are there for every role; the others need an administrator.
  const visibleScopes = useMemo(() => o.scopes.filter((s) => x.role === 'admin' || ['calls:read', 'calls:act', 'media:read'].includes(s.key)), [o.scopes, x.role])
  async function save() {
    setBusy(true)
    setError(null)
    try {
      const r = await api<{ token: string }>('POST', '/mcp/connections', {
        name: x.name, role: x.role, expiresDays: x.expiresDays,
        scopes: scopes.filter((s) => visibleScopes.some((v) => v.key === s)),
        allowedIps: x.ips.split(/[\s,]+/).map((s) => s.trim()).filter(Boolean),
      })
      onCreated(r.token)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal wide title={t('mcp.newConnection')} subtitle={t('mcp.form.oauthNote')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} disabled={!scopes.length} onClick={save}><KeyRound className="size-4" aria-hidden />{t('mcp.form.create')}</Button></>}>
      <ErrorMessage error={error} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Input name="name" label={t('mcp.form.name')} value={x.name} placeholder={t('mcp.form.namePlaceholder')} onChange={(e) => setX({ ...x, name: e.target.value })} autoFocus />
        <Select name="expiresDays" label={t('mcp.form.expires')} value={x.expiresDays} onChange={(e) => setX({ ...x, expiresDays: Number(e.target.value) })}>
          {[30, 90, 180, 365].map((d) => <option key={d} value={d}>{t('mcp.form.days', { count: d })}</option>)}
        </Select>
        {o.maxRole !== 'user' && (
          <Select name="role" label={t('mcp.form.role')} hint={t('mcp.form.roleHint')} value={x.role} onChange={(e) => setX({ ...x, role: e.target.value })}>
            {(['admin', 'reception', 'user'] as const).filter((r) => roleRank(r) <= roleRank(o.maxRole)).map((r) => <option key={r} value={r}>{t(`roles.${r}`)}</option>)}
          </Select>
        )}
        <Textarea name="allowedIps" label={t('mcp.form.allowedIps')} hint={t('mcp.form.allowedIpsHint')} rows={2} value={x.ips} onChange={(e) => setX({ ...x, ips: e.target.value })} />
      </div>
      <div>
        <div className="mb-2 text-xs uppercase tracking-[0.08em] text-subtle">{t('mcp.form.scopes')}</div>
        <ScopeList scopes={visibleScopes} value={scopes} onChange={setScopes} />
      </div>
      <div className="flex gap-2 rounded-xl border border-accent/35 bg-accent/5 px-4 py-3 text-xs text-ink-2">
        <ShieldCheck className="size-4 flex-none text-accent" aria-hidden />{t('mcp.form.never')}
      </div>
    </Modal>
  )
}

// ---- tools -------------------------------------------------------------------------

function Tools({ o, set, busy, onSave }: { o: McpOverview; set: McpSettings; busy: boolean; onSave: (s: McpSettings) => void }) {
  const { t } = useTranslation()
  const list = useApi<McpTool[]>('/mcp/tools')
  function toggle(name: string, on: boolean) {
    const off = on ? set.disabledTools.filter((n) => n !== name) : [...set.disabledTools, name]
    onSave({ ...set, disabledTools: off })
  }
  return (
    <div className="flex flex-col gap-4">
      <p className="max-w-3xl text-sm text-muted">{t('mcp.tools.intro')}</p>
      <ErrorMessage error={list.error} />
      {o.scopes.map((s) => {
        const [title, desc] = t(`mcp.scope.${scopeKey(s.key)}`, { returnObjects: true }) as string[]
        const tools = list.data?.filter((x) => x.scope === s.key) ?? []
        return (
          <Card key={s.key}>
            <div className="mb-3 flex flex-wrap items-center gap-2">
              <h2 className="font-display text-base font-semibold">{title}</h2>
              <KindBadge kind={s.kind} />
              <code className="font-mono text-xs text-subtle">{s.key}</code>
              <span className="w-full text-xs text-muted">{desc}</span>
            </div>
            <div className="divide-y divide-line">
              {tools.map((tool) => (
                <div key={tool.name} className="flex items-center gap-4 py-2.5">
                  <code className="w-56 flex-none font-mono text-[13px] text-ink">{tool.name}</code>
                  <span className="min-w-0 flex-1 text-xs text-muted">{tool.description}</span>
                  {tool.kind !== s.kind && <KindBadge kind={tool.kind} />}
                  <Badge>{t(`roles.${tool.minRole}`)}</Badge>
                  <Switch label={tool.name} checked={!set.disabledTools.includes(tool.name)} disabled={busy} onChange={(v) => toggle(tool.name, v)} />
                </div>
              ))}
            </div>
          </Card>
        )
      })}
    </div>
  )
}

// ---- activity ------------------------------------------------------------------------

type Filter = 'all' | 'read' | 'change' | 'denied'

function Activity({ admin }: { admin: boolean }) {
  const { t, i18n } = useTranslation()
  const [filter, setFilter] = useState<Filter>('all')
  const [q, setQ] = useState('')
  const qs = new URLSearchParams()
  if (filter !== 'all') qs.set('filter', filter)
  if (q.trim()) qs.set('q', q.trim())
  const list = useApi<McpCall[]>(`/mcp/activity?${qs}`, 15000)
  const tone = { ok: 'ok', proposal: 'warn', denied: 'danger', error: 'danger' } as const
  return (
    <>
      <div className="flex flex-wrap items-center gap-3">
        <div className="w-full sm:w-72">
          <Input label={t('mcp.activity.search')} placeholder={t('mcp.activity.search')} value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        <Tabs value={filter} onChange={setFilter} tabs={(['all', 'read', 'change', 'denied'] as const).map((k) => ({ key: k, label: t(`mcp.activity.${k}`) }))} />
        <span className="flex-1" />
        {admin && <a className="inline-flex min-h-11 items-center gap-2 rounded-[10px] border border-line px-4 text-sm text-ink-2 hover:bg-tint/5" href={`/api/mcp/activity/export?${qs}`}>{t('mcp.activity.export')}</a>}
      </div>
      <Card>
        <ErrorMessage error={list.error} />
        <Table head={[t('mcp.activity.time'), t('mcp.client'), t('mcp.activity.user'), t('mcp.activity.tool'), t('mcp.activity.input'), t('mcp.activity.result'), t('mcp.activity.status')]}
          empty={list.data?.length === 0 && <Empty icon={<ScrollText className="size-7" />} title={t('mcp.activity.empty')} />}>
          {list.data?.map((c) => (
            <tr key={c.id}>
              <td className={`${td} whitespace-nowrap font-mono text-xs`}>{formatDate(c.at, i18n.language)}</td>
              <td className={`${td} text-ink`}>{c.connName || '—'}</td>
              <td className={td}>{c.username || '—'}</td>
              <td className={td}><Badge tone={kindTone[c.kind]}><span className="font-mono">{c.tool}</span></Badge></td>
              <td className={`${td} max-w-[280px] truncate font-mono text-xs text-muted`} title={c.input}>{c.input || '—'}</td>
              <td className={`${td} max-w-[260px] truncate`} title={c.result}>{c.result}</td>
              <td className={td}><Badge tone={tone[c.status]} dot>{t(`mcp.activity.st.${c.status}`)}</Badge></td>
            </tr>
          ))}
        </Table>
      </Card>
      <div className="flex gap-2 rounded-xl border border-accent/35 bg-accent/5 px-4 py-3 text-xs text-ink-2">
        <ScrollText className="size-4 flex-none text-accent" aria-hidden />{t('mcp.activity.note')}
      </div>
    </>
  )
}

// ---- connect -------------------------------------------------------------------------

function CodeBlock({ children }: { children: string }) {
  return <pre className="overflow-x-auto rounded-[10px] border border-line bg-inset px-3 py-2.5 font-mono text-xs leading-relaxed text-ink">{children}</pre>
}

function CopyButton({ value, label }: { value: string; label: string }) {
  const { t } = useTranslation()
  const [done, setDone] = useState(false)
  return (
    <Button variant="secondary" onClick={() => navigator.clipboard?.writeText(value).then(() => { setDone(true); window.setTimeout(() => setDone(false), 1500) })}>
      {done ? <Check className="size-4" aria-hidden /> : <Copy className="size-4" aria-hidden />}{done ? t('mcp.connect.copied') : label}
    </Button>
  )
}

function ConnectCard({ icon, title, sub, children }: { icon: ReactNode; title: string; sub: string; children: ReactNode }) {
  return (
    <Card className="flex flex-col gap-3">
      <div className="flex items-center gap-3">
        <span className="flex size-9 flex-none items-center justify-center rounded-lg bg-accent-2/15 text-accent-2">{icon}</span>
        <div>
          <div className="font-display text-base font-semibold">{title}</div>
          <div className="text-xs text-muted">{sub}</div>
        </div>
      </div>
      {children}
    </Card>
  )
}

function Connect({ o }: { o: McpOverview }) {
  const { t } = useTranslation()
  const cmd = `claude mcp add --transport http \\\n  kiestoon ${o.endpoint} \\\n  --header "Authorization: Bearer kt_mcp_…"`
  const json = JSON.stringify({ mcpServers: { kiestoon: { type: 'http', url: o.endpoint, headers: { Authorization: 'Bearer kt_mcp_…' } } } }, null, 2)
  const steps = [t('mcp.connect.step1'), t('mcp.connect.step2'), t('mcp.connect.step3'), t('mcp.connect.step4')]
  const examples = t('mcp.connect.examples', { returnObjects: true }) as string[]
  return (
    <>
      <div className="grid gap-4 xl:grid-cols-3">
        <ConnectCard icon={<Sparkles className="size-4" aria-hidden />} title={t('mcp.connect.claudeAi')} sub={t('mcp.connect.claudeAiSub')}>
          <ol className="flex flex-col gap-3">
            {steps.map((s, i) => (
              <li key={i} className="flex gap-3 text-sm text-ink-2">
                <span className="flex size-6 flex-none items-center justify-center rounded-full bg-accent/15 text-xs font-semibold text-accent-ink">{i + 1}</span>
                <div className="flex min-w-0 flex-1 flex-col gap-2">{s}{i === 1 && <CopyField label="URL" value={o.endpoint} />}</div>
              </li>
            ))}
          </ol>
          {!o.internet && <Alert tone="warn"><span className="inline-flex gap-2"><Globe className="size-4 flex-none" aria-hidden />{t('mcp.connect.internetWarn')}</span></Alert>}
        </ConnectCard>
        <ConnectCard icon={<Terminal className="size-4" aria-hidden />} title={t('mcp.connect.code')} sub={t('mcp.connect.codeSub')}>
          <p className="text-sm text-muted">{t('mcp.connect.codeText')}</p>
          <CodeBlock>{cmd}</CodeBlock>
          <p className="text-xs text-subtle">{t('mcp.connect.codeLan')}</p>
          <div><CopyButton value={cmd.replace(/\\\n\s*/g, '')} label={t('mcp.connect.copyCommand')} /></div>
        </ConnectCard>
        <ConnectCard icon={<Plug className="size-4" aria-hidden />} title={t('mcp.connect.other')} sub={t('mcp.connect.otherSub')}>
          <CodeBlock>{json}</CodeBlock>
          <div><CopyButton value={json} label={t('mcp.connect.copyJson')} /></div>
        </ConnectCard>
      </div>
      <Card>
        <h2 className="mb-3 font-display text-base font-semibold">{t('mcp.connect.try')}</h2>
        <div className="flex flex-wrap gap-2">
          {examples.map((x) => (
            <span key={x} className="inline-flex items-center gap-2 rounded-full border border-line bg-tint/[0.03] px-3 py-1.5 text-xs text-ink-2">
              <MessageSquare className="size-3.5 text-accent" aria-hidden />{x}
            </span>
          ))}
        </div>
      </Card>
    </>
  )
}
