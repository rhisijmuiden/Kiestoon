import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowLeftRight, Loader2, ShieldCheck, Sparkles } from 'lucide-react'
import { AuthShell } from '../components/AuthShell'
import { Select, useApi } from '../components/kit'
import { Alert, Button, ErrorMessage } from '../components/ui'
import { api } from '../lib/api'
import type { McpScope } from '../lib/types'
import { ScopeList } from './Mcp'

type RequestInfo = { client: string; redirectHost: string; role: string; username: string; scopes: McpScope[]; requested: string[] }

/** The OAuth consent page: an MCP client (Claude) asks to connect; the signed-in person chooses what it may do. */
export function OAuthConsent() {
  const { t } = useTranslation()
  const query = window.location.search.replace(/^\?/, '')
  const info = useApi<RequestInfo>(`/mcp/oauth/request?q=${encodeURIComponent(query)}`)
  const [scopes, setScopes] = useState<string[] | null>(null)
  const [days, setDays] = useState(30)
  const [busy, setBusy] = useState<'allow' | 'deny' | ''>('')
  const [error, setError] = useState<unknown>(null)
  const chosen = scopes ?? info.data?.requested ?? []

  async function decide(allow: boolean) {
    setBusy(allow ? 'allow' : 'deny')
    setError(null)
    try {
      const r = await api<{ redirect: string }>('POST', '/mcp/oauth/decide', { query, allow, scopes: chosen, days })
      window.location.assign(r.redirect)
    } catch (e) {
      setError(e)
      setBusy('')
    }
  }

  const d = info.data
  return (
    <AuthShell wide>
      {!d && !info.error && <div className="flex justify-center py-10 text-muted"><Loader2 className="size-5 animate-spin" aria-hidden /></div>}
      {info.error ? (
        <div className="flex flex-col gap-4">
          <ErrorMessage error={info.error} />
          <p className="text-sm text-muted">{t('mcp.consent.invalid')}</p>
        </div>
      ) : null}
      {d && (
        <div className="flex flex-col gap-5">
          <div className="flex items-center justify-center gap-4">
            <span className="flex size-12 items-center justify-center rounded-2xl border border-accent/40 bg-accent/10 text-accent"><ShieldCheck className="size-6" aria-hidden /></span>
            <ArrowLeftRight className="size-5 text-subtle" aria-hidden />
            <span className="flex size-12 items-center justify-center rounded-2xl bg-accent-2/15 text-accent-2"><Sparkles className="size-6" aria-hidden /></span>
          </div>
          <div className="text-center">
            <h1 className="font-display text-xl font-semibold">{t('mcp.consent.title', { client: d.client })}</h1>
            <p className="mt-1 text-sm text-muted">
              {t('mcp.consent.signedInAs')} <b className="text-ink">{d.username}</b> · {t(`roles.${d.role}`)}
            </p>
            <p className="mt-0.5 text-xs text-subtle">{t('mcp.consent.via')} <span className="font-mono">{d.redirectHost}</span></p>
          </div>
          <div>
            <div className="mb-2 text-xs uppercase tracking-[0.08em] text-subtle">{t('mcp.consent.may', { client: d.client })}</div>
            <ScopeList scopes={d.scopes} value={chosen} onChange={setScopes} />
          </div>
          <Select label={t('mcp.consent.valid')} value={days} onChange={(e) => setDays(Number(e.target.value))}>
            {[1, 7, 30, 90, 365].map((n) => <option key={n} value={n}>{t('mcp.form.days', { count: n })}</option>)}
          </Select>
          <p className="text-xs text-muted">{t('mcp.consent.note')}</p>
          <ErrorMessage error={error} />
          {chosen.length === 0 && <Alert tone="warn">{t('mcp.consent.pick')}</Alert>}
          <div className="grid grid-cols-2 gap-3">
            <Button variant="ghost" busy={busy === 'deny'} disabled={!!busy} onClick={() => decide(false)}>{t('mcp.consent.deny')}</Button>
            <Button busy={busy === 'allow'} disabled={!!busy || chosen.length === 0} onClick={() => decide(true)}>{t('mcp.consent.allow')}</Button>
          </div>
        </div>
      )}
    </AuthShell>
  )
}
