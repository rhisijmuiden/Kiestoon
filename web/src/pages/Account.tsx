import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
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
    </>
  )
}
