import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { History, KeySquare, Pencil, Plus, Send, Trash2, Webhook as WebhookIcon } from 'lucide-react'
import { Badge, CopyField, Empty, Modal, Select, Table, Tabs, Toggle, formatDate, td, useApi } from '../components/kit'
import { IconButton } from '../components/pbx'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api, roleRank } from '../lib/api'
import { useSession } from '../lib/session'
import type { ApiToken, Webhook, WebhookDelivery } from '../lib/types'

/** REST API tokens and webhooks. */
export function Integrations() {
  const { t } = useTranslation()
  const [tab, setTab] = useState<'tokens' | 'webhooks'>('tokens')
  return (
    <>
      <PageHeader title={t('integrations.title')} subtitle={t('integrations.subtitle')} />
      <Tabs value={tab} onChange={setTab} tabs={[{ key: 'tokens', label: t('integrations.tokens') }, { key: 'webhooks', label: t('integrations.webhooks') }]} />
      {tab === 'tokens' ? <Tokens /> : <Webhooks />}
    </>
  )
}

function Tokens() {
  const { t, i18n } = useTranslation()
  const list = useApi<ApiToken[]>('/tokens')
  const [adding, setAdding] = useState(false)
  const [created, setCreated] = useState('')
  async function remove(x: ApiToken) {
    if (!window.confirm(t('integrations.confirmRevoke', { name: x.name }))) return
    await api('DELETE', `/tokens/${x.id}`)
    list.reload()
  }
  const expired = (x: ApiToken) => x.expiresAt && new Date(x.expiresAt) < new Date()
  return (
    <>
      <Card>
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <p className="max-w-2xl text-sm text-muted">{t('integrations.tokensIntro')}</p>
          <Button onClick={() => setAdding(true)}><Plus className="size-4" aria-hidden />{t('integrations.addToken')}</Button>
        </div>
        {created && (
          <div className="mb-4 flex flex-col gap-2">
            <Alert tone="warn">{t('integrations.tokenOnce')}</Alert>
            <CopyField label={t('integrations.token')} value={created} />
          </div>
        )}
        <ErrorMessage error={list.error} />
        <Table head={[t('common.name'), t('integrations.token'), t('integrations.rights'), t('integrations.owner'), t('integrations.lastUsed'), t('integrations.expires'), '']}
          empty={list.data?.length === 0 && <Empty icon={<KeySquare className="size-7" />} title={t('integrations.noTokens')} text={t('integrations.noTokensText')} />}>
          {list.data?.map((x) => (
            <tr key={x.id} className={expired(x) ? 'opacity-50' : ''}>
              <td className={`${td} text-ink`}>{x.name}</td>
              <td className={`${td} font-mono text-xs`}>{x.prefix}…</td>
              <td className={td}>
                <div className="flex flex-wrap gap-1">
                  <Badge tone="accent">{t(`roles.${x.role}`)}</Badge>
                  {x.readOnly && <Badge>{t('integrations.readOnly')}</Badge>}
                </div>
              </td>
              <td className={td}>{x.username}</td>
              <td className={td}>{x.lastUsedAt ? <>{formatDate(x.lastUsedAt, i18n.language)} <span className="font-mono text-xs text-subtle">{x.lastIp}</span></> : t('integrations.never')}</td>
              <td className={td}>{x.expiresAt ? (expired(x) ? <Badge tone="danger">{t('integrations.expired')}</Badge> : formatDate(x.expiresAt, i18n.language)) : t('integrations.noExpiry')}</td>
              <td className={`${td} text-right`}><IconButton label={t('integrations.revoke')} danger onClick={() => remove(x)}><Trash2 className="size-4" aria-hidden /></IconButton></td>
            </tr>
          ))}
        </Table>
        <p className="mt-4 text-xs text-subtle">{t('integrations.docs')} <code className="font-mono text-ink-2">curl -H "Authorization: Bearer kt_…" {window.location.origin}/api/pbx/extensions</code></p>
      </Card>
      {adding && <TokenForm onClose={() => setAdding(false)} onCreated={(tok) => { setAdding(false); setCreated(tok); list.reload() }} />}
    </>
  )
}

function TokenForm({ onClose, onCreated }: { onClose: () => void; onCreated: (token: string) => void }) {
  const { t } = useTranslation()
  const { state } = useSession()
  const myRank = roleRank(state.kind === 'ready' ? state.me.user.role : undefined)
  const [x, setX] = useState({ name: '', role: 'admin', readOnly: false, expiresDays: 365 })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  async function save() {
    setBusy(true)
    setError(null)
    try {
      const r = await api<{ token: string }>('POST', '/tokens', x)
      onCreated(r.token)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={t('integrations.addToken')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={save}>{t('integrations.create')}</Button></>}>
      <ErrorMessage error={error} />
      <Input name="name" label={t('common.name')} value={x.name} onChange={(e) => setX({ ...x, name: e.target.value })} placeholder="CRM" autoFocus />
      <div className="grid gap-4 sm:grid-cols-2">
        <Select name="role" label={t('integrations.rights')} value={x.role} onChange={(e) => setX({ ...x, role: e.target.value })} hint={t('integrations.rightsHint')}>
          {(['admin', 'reception', 'user'] as const).filter((r) => roleRank(r) <= myRank).map((r) => <option key={r} value={r}>{t(`roles.${r}`)}</option>)}
        </Select>
        <Select name="expiresDays" label={t('integrations.expires')} value={x.expiresDays} onChange={(e) => setX({ ...x, expiresDays: Number(e.target.value) })}>
          {[30, 90, 365, 730].map((d) => <option key={d} value={d}>{t('integrations.days', { count: d })}</option>)}
          <option value={0}>{t('integrations.noExpiry')}</option>
        </Select>
      </div>
      <Toggle name="readOnly" label={t('integrations.readOnly')} hint={t('integrations.readOnlyHint')} checked={x.readOnly} onChange={(v) => setX({ ...x, readOnly: v })} />
      <p className="text-xs text-subtle">{t('integrations.blockedRoutes')}</p>
    </Modal>
  )
}

const blankHook: Webhook = { id: 0, name: '', url: 'https://', events: ['call.ended'], secret: '', enabled: true, createdAt: '' }

function Webhooks() {
  const { t } = useTranslation()
  const list = useApi<{ webhooks: Webhook[]; events: string[] }>('/webhooks')
  const [edit, setEdit] = useState<Webhook | null>(null)
  const [log, setLog] = useState<Webhook | null>(null)
  const [msg, setMsg] = useState<{ tone: 'ok' | 'danger'; text: string } | null>(null)
  async function remove(h: Webhook) {
    if (!window.confirm(t('common.confirmDelete', { name: h.name }))) return
    await api('DELETE', `/webhooks/${h.id}`)
    list.reload()
  }
  async function test(h: Webhook) {
    setMsg(null)
    try {
      const r = await api<{ status: number; error?: string }>('POST', `/webhooks/${h.id}/test`)
      setMsg(r.error ? { tone: 'danger', text: t('integrations.testFailed', { error: r.error }) } : { tone: 'ok', text: t('integrations.testOk', { status: r.status }) })
    } catch (e) {
      setMsg({ tone: 'danger', text: String((e as Error).message) })
    }
  }
  return (
    <>
      <Card>
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <p className="max-w-2xl text-sm text-muted">{t('integrations.webhooksIntro')}</p>
          <Button onClick={() => setEdit(blankHook)}><Plus className="size-4" aria-hidden />{t('integrations.addWebhook')}</Button>
        </div>
        {msg && <div className="mb-4"><Alert tone={msg.tone}>{msg.text}</Alert></div>}
        <ErrorMessage error={list.error} />
        <Table head={[t('common.name'), 'URL', t('integrations.events'), '']}
          empty={list.data?.webhooks.length === 0 && <Empty icon={<WebhookIcon className="size-7" />} title={t('integrations.noWebhooks')} text={t('integrations.noWebhooksText')} />}>
          {list.data?.webhooks.map((h) => (
            <tr key={h.id} className={h.enabled ? '' : 'opacity-50'}>
              <td className={`${td} text-ink`}>{h.name}</td>
              <td className={`${td} max-w-xs truncate font-mono text-xs`}>{h.url}</td>
              <td className={td}><div className="flex flex-wrap gap-1">{h.events.map((e) => <Badge key={e} tone="accent">{e}</Badge>)}</div></td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <IconButton label={t('integrations.test')} onClick={() => test(h)}><Send className="size-4" aria-hidden /></IconButton>
                <IconButton label={t('integrations.deliveries')} onClick={() => setLog(h)}><History className="size-4" aria-hidden /></IconButton>
                <IconButton label={t('common.edit')} onClick={() => setEdit(h)}><Pencil className="size-4" aria-hidden /></IconButton>
                <IconButton label={t('common.delete')} danger onClick={() => remove(h)}><Trash2 className="size-4" aria-hidden /></IconButton>
              </td>
            </tr>
          ))}
        </Table>
        <p className="mt-4 text-xs text-subtle">{t('integrations.signature')}</p>
      </Card>
      {edit && list.data && <WebhookForm value={edit} events={list.data.events} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); list.reload() }} />}
      {log && <Deliveries hook={log} onClose={() => setLog(null)} />}
    </>
  )
}

function WebhookForm({ value, events, onClose, onSaved }: { value: Webhook; events: string[]; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [h, setH] = useState(value)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api(h.id ? 'PUT' : 'POST', h.id ? `/webhooks/${h.id}` : '/webhooks', h)
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const toggle = (e: string) => setH({ ...h, events: h.events.includes(e) ? h.events.filter((x) => x !== e) : [...h.events, e] })
  return (
    <Modal wide title={h.id ? t('integrations.editWebhook', { name: value.name }) : t('integrations.addWebhook')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <div className="grid gap-4 sm:grid-cols-[1fr_2fr]">
        <Input name="name" label={t('common.name')} value={h.name} onChange={(e) => setH({ ...h, name: e.target.value })} placeholder="CRM" autoFocus />
        <Input name="url" label="URL" value={h.url} onChange={(e) => setH({ ...h, url: e.target.value.trim() })} className="font-mono text-sm" placeholder="https://crm.example.com/kiestoon" />
      </div>
      <div className="flex flex-col gap-2">
        <span className="text-sm font-medium text-ink-2">{t('integrations.events')}</span>
        <div className="grid gap-2 sm:grid-cols-2">
          {events.map((e) => (
            <label key={e} className="flex items-start gap-2.5 rounded-xl border border-line bg-surface-2/60 px-3 py-2.5">
              <input type="checkbox" checked={h.events.includes(e)} onChange={() => toggle(e)} className="mt-1 size-4 accent-[var(--color-accent)]" />
              <span>
                <span className="block font-mono text-sm text-ink">{e}</span>
                <span className="block text-xs text-subtle">{t(`integrations.eventInfo.${e.replace('.', '_')}`)}</span>
              </span>
            </label>
          ))}
        </div>
      </div>
      {h.secret && <CopyField label={t('integrations.secret')} value={h.secret} secret />}
      <Toggle name="enabled" label={t('common.enabled')} checked={h.enabled} onChange={(v) => setH({ ...h, enabled: v })} />
    </Modal>
  )
}

function Deliveries({ hook, onClose }: { hook: Webhook; onClose: () => void }) {
  const { t, i18n } = useTranslation()
  const list = useApi<WebhookDelivery[]>(`/webhooks/${hook.id}/deliveries`, 5000)
  return (
    <Modal wide title={t('integrations.deliveries')} subtitle={hook.name} onClose={onClose}>
      <Table head={[t('integrations.time'), t('integrations.event'), t('integrations.attempt'), t('integrations.result'), 'ms']}
        empty={list.data?.length === 0 && <p className="py-6 text-center text-sm text-muted">{t('integrations.noDeliveries')}</p>}>
        {list.data?.map((d) => (
          <tr key={d.id}>
            <td className={td}>{formatDate(d.at, i18n.language)}</td>
            <td className={`${td} font-mono text-xs`}>{d.event}</td>
            <td className={`${td} font-mono`}>{d.attempt}</td>
            <td className={td}>
              {d.error ? <Badge tone="danger">{d.status || t('integrations.noAnswer')}</Badge> : <Badge tone="ok">{d.status}</Badge>}
              {d.error && <span className="block text-xs text-subtle">{d.error}</span>}
            </td>
            <td className={`${td} font-mono`}>{d.durationMs}</td>
          </tr>
        ))}
      </Table>
    </Modal>
  )
}
