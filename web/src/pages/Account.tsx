import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Fingerprint, Plus, Trash2 } from 'lucide-react'
import { Empty, Table, formatDate, td, useApi } from '../components/kit'
import { IconButton } from '../components/pbx'
import { createPasskey, passkeysSupported } from '../lib/passkey'
import type { Passkey } from '../lib/types'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { LanguageSwitch } from '../components/brand'
import { api } from '../lib/api'
import { useSession } from '../lib/session'

export function Account() {
  const { t } = useTranslation()
  const { state } = useSession()
  const me = state.kind === 'ready' ? state.me : null
  const [form, setForm] = useState({ current: '', next: '', repeat: '' })
  const [error, setError] = useState<unknown>(null)
  const [mismatch, setMismatch] = useState(false)
  const [done, setDone] = useState(false)
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setDone(false)
    setError(null)
    if (form.next !== form.repeat) return setMismatch(true)
    setMismatch(false)
    setBusy(true)
    try {
      await api('PUT', '/account/password', { current: form.current, new: form.next })
      setDone(true)
      setForm({ current: '', next: '', repeat: '' })
    } catch (err) {
      setError(err)
    } finally {
      setBusy(false)
    }
  }

  return (
    <>
      <PageHeader title={t('account.title')} subtitle={me ? `${me.user.displayName} · ${me.user.username}` : t('account.subtitle')} />
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <h2 className="font-display text-lg font-semibold">{t('account.languageTitle')}</h2>
          <div className="mt-4"><LanguageSwitch persist /></div>
        </Card>
        <Card>
          <h2 className="font-display text-lg font-semibold">{t('account.passwordTitle')}</h2>
          <form onSubmit={submit} className="mt-4 flex flex-col gap-4">
            {done && <Alert tone="ok">{t('account.changed')}</Alert>}
            {mismatch && <Alert>{t('setup.passwordMismatch')}</Alert>}
            <ErrorMessage error={error} />
            <Input label={t('account.current')} type="password" value={form.current} onChange={(e) => setForm({ ...form, current: e.target.value })} required autoComplete="current-password" />
            <Input label={t('account.new')} hint={t('setup.passwordHint')} type="password" value={form.next} onChange={(e) => setForm({ ...form, next: e.target.value })} required minLength={12} autoComplete="new-password" />
            <Input label={t('account.repeat')} type="password" value={form.repeat} onChange={(e) => setForm({ ...form, repeat: e.target.value })} required minLength={12} autoComplete="new-password" />
            <Button type="submit" busy={busy} className="self-start">{t('common.save')}</Button>
          </form>
        </Card>
      </div>
      <Passkeys />
    </>
  )
}

function Passkeys() {
  const { t, i18n } = useTranslation()
  const list = useApi<Passkey[]>('/account/passkeys')
  const [name, setName] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const ipHost = /^[\d.]+$/.test(window.location.hostname) || window.location.hostname.includes(":")
  async function add() {
    setBusy(true)
    setError(null)
    try {
      const opts = await api<{ publicKey: Record<string, unknown> }>('POST', '/account/passkeys/begin')
      const credential = await createPasskey(opts)
      await api('POST', '/account/passkeys/finish', { name: name.trim() || defaultName(), credential })
      setName('')
      list.reload()
    } catch (e) {
      if (!(e instanceof DOMException && e.name === 'NotAllowedError') && !(e instanceof Error && e.message === 'cancelled')) setError(e)
    } finally {
      setBusy(false)
    }
  }
  async function remove(p: Passkey) {
    if (!window.confirm(t('common.confirmDelete', { name: p.name }))) return
    await api('DELETE', `/account/passkeys/${p.id}`)
    list.reload()
  }
  return (
    <Card>
      <h2 className="flex items-center gap-2 font-display text-lg font-semibold"><Fingerprint className="size-5 text-accent" aria-hidden />{t('account.passkeys.title')}</h2>
      <p className="mt-1 max-w-3xl text-sm text-muted">{t('account.passkeys.intro')}</p>
      <div className="mt-4 flex flex-col gap-4">
        <ErrorMessage error={error} />
        {!passkeysSupported() || ipHost ? (
          <Alert tone="warn">{ipHost ? t('account.passkeys.needsName') : t('account.passkeys.unsupported')}</Alert>
        ) : (
          <div className="flex flex-wrap items-end gap-2">
            <div className="min-w-56 flex-1"><Input label={t('account.passkeys.name')} value={name} onChange={(e) => setName(e.target.value)} placeholder={defaultName()} /></div>
            <Button busy={busy} onClick={add}><Plus className="size-4" aria-hidden />{t('account.passkeys.add')}</Button>
          </div>
        )}
        <Table head={[t('common.name'), t('account.passkeys.created'), t('account.passkeys.lastUsed'), '']}
          empty={list.data?.length === 0 && <Empty title={t('account.passkeys.empty')} />}>
          {list.data?.map((p) => (
            <tr key={p.id}>
              <td className={`${td} text-ink`}>{p.name}</td>
              <td className={td}>{formatDate(p.createdAt, i18n.language)}</td>
              <td className={td}>{formatDate(p.lastUsedAt, i18n.language)}</td>
              <td className={`${td} text-right`}><IconButton label={t('common.delete')} danger onClick={() => remove(p)}><Trash2 className="size-4" aria-hidden /></IconButton></td>
            </tr>
          ))}
        </Table>
      </div>
    </Card>
  )
}

function defaultName() {
  const ua = navigator.userAgent
  const os = /iPhone|iPad/.test(ua) ? 'iPhone/iPad' : /Android/.test(ua) ? 'Android' : /Mac/.test(ua) ? 'Mac' : /Windows/.test(ua) ? 'Windows' : 'Linux'
  return `${os} · ${new Date().toLocaleDateString()}`
}
