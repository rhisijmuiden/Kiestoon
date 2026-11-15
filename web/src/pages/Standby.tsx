import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { ServerCog } from 'lucide-react'
import { AuthShell } from '../components/AuthShell'
import { Badge, Toggle, formatBytes, formatDate, useApi } from '../components/kit'
import { Alert, Button, ErrorMessage, Input } from '../components/ui'
import { ApiError, api } from '../lib/api'
import type { StandbyView } from '../lib/types'

/** The portal of the standby node: replication status and promotion by hand. */
export function Standby() {
  const { t, i18n } = useTranslation()
  const view = useApi<StandbyView>('/cluster/standby', 5000)
  const [f, setF] = useState({ username: '', password: '', code: '', force: false })
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState(false)
  const [promoted, setPromoted] = useState(false)
  const v = view.data
  const r = v?.replication
  const alive = !!r?.primaryReachable

  async function promote(e: FormEvent) {
    e.preventDefault()
    if (!window.confirm(t('standby.confirm'))) return
    setBusy(true)
    setError(null)
    try {
      await api('POST', '/cluster/standby/promote', f)
      setPromoted(true)
      window.setTimeout(() => window.location.reload(), 15000)
    } catch (err) {
      setError(err)
    } finally {
      setBusy(false)
    }
  }
  const primaryAlive = error instanceof ApiError && error.code === 'primary_alive'

  return (
    <AuthShell wide>
      <div className="flex flex-col gap-5">
        <div className="flex items-start gap-3">
          <span className="grid size-10 flex-none place-items-center rounded-xl bg-accent/15 text-accent"><ServerCog className="size-5" aria-hidden /></span>
          <div>
            <h1 className="font-display text-2xl font-semibold tracking-tight">{t('standby.title')}</h1>
            <p className="mt-1 text-sm text-muted">{t('standby.subtitle', { name: v?.node.peer?.name ?? '…' })}</p>
          </div>
        </div>
        <ErrorMessage error={view.error} />
        {v && r && (
          <dl className="grid gap-x-6 gap-y-2 rounded-xl border border-line p-4 text-sm sm:grid-cols-[180px_1fr]">
            <dt className="text-muted">{t('standby.primary')}</dt>
            <dd>{alive ? <Badge tone="ok" dot>{t('sites.connected')}</Badge> : <Badge tone="danger" dot>{t('sites.offline')}</Badge>} {v.node.peer?.name} · {v.node.peer?.address}</dd>
            <dt className="text-muted">{t('sites.lastSeen')}</dt><dd>{r.lastContact && !r.lastContact.startsWith('0001') ? formatDate(r.lastContact, i18n.language) : '–'}</dd>
            <dt className="text-muted">{t('standby.database')}</dt>
            <dd>{r.replica.seq ? t('cluster.snapshot', { seq: r.replica.seq, size: formatBytes(r.replica.size) }) : t('standby.noCopy')}{r.lastDbSync && !r.lastDbSync.startsWith('0001') && ` · ${formatDate(r.lastDbSync, i18n.language)}`}</dd>
            <dt className="text-muted">{t('cluster.files')}</dt><dd>{t('cluster.filesSynced', { count: r.files })}</dd>
            <dt className="text-muted">{t('cluster.thisNode')}</dt><dd>{v.hostname} · {v.node.address} · {v.version}</dd>
            {r.lastError && <><dt className="text-muted">{t('standby.lastError')}</dt><dd className="font-mono text-xs text-danger">{r.lastError}</dd></>}
          </dl>
        )}
        {promoted ? <Alert tone="ok">{t('standby.promoted')}</Alert> : (
          <form onSubmit={promote} className="flex flex-col gap-4">
            <h2 className="font-display text-lg font-semibold">{t('standby.promoteTitle')}</h2>
            <p className="text-sm text-muted">{t('standby.promoteText')}</p>
            <ErrorMessage error={primaryAlive ? null : error} />
            {primaryAlive && <Alert tone="warn">{t('standby.primaryAlive')}</Alert>}
            <div className="grid gap-4 sm:grid-cols-3">
              <Input label={t('setup.username')} value={f.username} onChange={(e) => setF({ ...f, username: e.target.value })} required autoComplete="username" />
              <Input label={t('setup.password')} type="password" value={f.password} onChange={(e) => setF({ ...f, password: e.target.value })} required autoComplete="current-password" />
              <Input label={t('standby.code')} value={f.code} onChange={(e) => setF({ ...f, code: e.target.value })} inputMode="numeric" autoComplete="one-time-code" className="font-mono" />
            </div>
            {(alive || primaryAlive) && <Toggle label={t('standby.force')} hint={t('standby.forceHint')} checked={f.force} onChange={(x) => setF({ ...f, force: x })} />}
            <div><Button type="submit" busy={busy}>{t('standby.promote')}</Button></div>
          </form>
        )}
      </div>
    </AuthShell>
  )
}
