import { useEffect, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckCircle2, KeyRound, Loader2 } from 'lucide-react'
import { AuthShell } from '../components/AuthShell'
import { Alert, Button, ErrorMessage, Input } from '../components/ui'
import { ApiError, api } from '../lib/api'

type Info = { username: string; displayName: string; language: 'en' | 'nl' }

/** Opened from the welcome e-mail: the token is in the fragment, so it never reaches a server log. */
export function SetPassword() {
  const { t, i18n } = useTranslation()
  const [token] = useState(() => window.location.hash.slice(1))
  const [info, setInfo] = useState<Info | null>(null)
  const [invalid, setInvalid] = useState(false)
  const [password, setPassword] = useState('')
  const [repeat, setRepeat] = useState('')
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState(false)

  useEffect(() => {
    api<Info>('POST', '/auth/password-link', { token })
      .then((i) => { setInfo(i); if (i.language !== i18n.language) void i18n.changeLanguage(i.language) })
      .catch(() => setInvalid(true))
  }, [token, i18n])

  async function submit(e: FormEvent) {
    e.preventDefault()
    setError(null)
    if (password !== repeat) {
      setError(new ApiError(400, 'password_mismatch'))
      return
    }
    setBusy(true)
    try {
      await api('POST', '/auth/password-link/set', { token, password })
      setDone(true)
      window.history.replaceState(null, '', '/set-password')
    } catch (err) {
      setError(err)
    } finally {
      setBusy(false)
    }
  }

  const toLogin = () => { window.location.href = '/' }

  return (
    <AuthShell>
      <div className="flex flex-col gap-5">
        <div className="flex size-12 items-center justify-center rounded-xl border border-accent/40 bg-accent/10">
          {done ? <CheckCircle2 className="size-6 text-ok" aria-hidden /> : <KeyRound className="size-6 text-accent" aria-hidden />}
        </div>
        {!info && !invalid && (
          <div className="flex items-center gap-3 text-muted" role="status"><Loader2 className="size-5 animate-spin text-accent" aria-hidden />{t('app.loading')}</div>
        )}
        {invalid && (
          <>
            <h1 className="font-display text-2xl font-semibold tracking-tight">{t('setPassword.invalidTitle')}</h1>
            <Alert tone="warn">{t('setPassword.invalidText')}</Alert>
            <Button onClick={toLogin}>{t('setPassword.toLogin')}</Button>
          </>
        )}
        {info && done && (
          <>
            <h1 className="font-display text-2xl font-semibold tracking-tight">{t('setPassword.doneTitle')}</h1>
            <p className="text-sm text-muted">{t('setPassword.doneText', { username: info.username })}</p>
            <Button onClick={toLogin}>{t('setPassword.toLogin')}</Button>
          </>
        )}
        {info && !done && (
          <form onSubmit={submit} className="flex flex-col gap-5">
            <div>
              <h1 className="font-display text-2xl font-semibold tracking-tight">{t('setPassword.title', { name: info.displayName })}</h1>
              <p className="mt-1.5 text-sm text-muted">{t('setPassword.subtitle')}</p>
            </div>
            <ErrorMessage error={error} />
            <Input label={t('login.username')} value={info.username} readOnly autoComplete="username" className="font-mono" />
            <Input label={t('setPassword.password')} type="password" value={password} onChange={(e) => setPassword(e.target.value)} required minLength={12} autoFocus autoComplete="new-password" hint={t('setPassword.hint')} />
            <Input label={t('setPassword.repeat')} type="password" value={repeat} onChange={(e) => setRepeat(e.target.value)} required minLength={12} autoComplete="new-password" />
            <Button type="submit" busy={busy}>{t('setPassword.submit')}</Button>
          </form>
        )}
      </div>
    </AuthShell>
  )
}
