import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { KeyRound } from 'lucide-react'
import { AuthShell } from '../components/AuthShell'
import { Button, ErrorMessage, Input } from '../components/ui'
import { api } from '../lib/api'
import { useSession } from '../lib/session'

export function Login({ mfa }: { mfa: boolean }) {
  const { t } = useTranslation()
  const { refresh, signOut } = useSession()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      if (mfa) await api('POST', '/auth/mfa', { code })
      else await api('POST', '/auth/login', { username, password })
      await refresh()
    } catch (err) {
      setError(err)
    } finally {
      setBusy(false)
    }
  }

  return (
    <AuthShell>
      {mfa ? (
        <form onSubmit={submit} className="flex flex-col gap-5">
          <div className="flex size-12 items-center justify-center rounded-xl border border-accent/40 bg-accent/10">
            <KeyRound className="size-6 text-accent" aria-hidden />
          </div>
          <div>
            <h1 className="font-display text-2xl font-semibold tracking-tight">{t('login.mfaTitle')}</h1>
            <p className="mt-1.5 text-sm text-muted">{t('login.mfaText')}</p>
          </div>
          <ErrorMessage error={error} />
          <Input
            label={t('login.code')}
            value={code}
            onChange={(e) => setCode(e.target.value)}
            required
            autoFocus
            autoComplete="one-time-code"
            inputMode="text"
            spellCheck={false}
            className="text-center font-mono text-2xl tracking-[0.4em]"
            maxLength={11}
          />
          <Button type="submit" busy={busy}>{t('login.verify')}</Button>
          <button type="button" onClick={signOut} className="text-sm text-muted hover:text-ink">{t('common.back')}</button>
        </form>
      ) : (
        <form onSubmit={submit} className="flex flex-col gap-5">
          <div>
            <h1 className="font-display text-2xl font-semibold tracking-tight">{t('login.title')}</h1>
            <p className="mt-1.5 text-sm text-muted">{t('login.subtitle')}</p>
          </div>
          <ErrorMessage error={error} />
          <Input label={t('login.username')} value={username} onChange={(e) => setUsername(e.target.value)} required autoFocus autoComplete="username" />
          <Input label={t('login.password')} type="password" value={password} onChange={(e) => setPassword(e.target.value)} required autoComplete="current-password" />
          <Button type="submit" busy={busy}>{t('login.submit')}</Button>
        </form>
      )}
    </AuthShell>
  )
}
