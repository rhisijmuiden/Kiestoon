import { useEffect, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Fingerprint, KeyRound } from 'lucide-react'
import { AuthShell } from '../components/AuthShell'
import { Alert, Button, ErrorMessage, Input } from '../components/ui'
import { api } from '../lib/api'
import { useSession } from '../lib/session'
import { getPasskey, passkeysSupported } from '../lib/passkey'

export function Login({ mfa }: { mfa: boolean }) {
  const { t } = useTranslation()
  const { refresh, signOut } = useSession()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState(false)
  const [providers, setProviders] = useState<{ id: number; name: string; kind: string }[]>([])
  const [ssoError] = useState(() => new URLSearchParams(window.location.search).get('sso_error') ?? '')
  useEffect(() => {
    if (ssoError) window.history.replaceState(null, '', window.location.pathname)
    api<{ id: number; name: string; kind: string }[]>('GET', '/auth/sso/providers').then(setProviders).catch(() => undefined)
  }, [ssoError])

  async function passkey() {
    setBusy(true)
    setError(null)
    try {
      const begin = await api<{ id: string; options: { publicKey: Record<string, unknown> } }>('POST', '/auth/passkey/begin')
      const credential = await getPasskey(begin.options)
      await api('POST', '/auth/passkey/finish', { id: begin.id, credential })
      await refresh()
    } catch (err) {
      // Closing the browser dialog is not an error worth showing.
      if (!(err instanceof DOMException && err.name === 'NotAllowedError') && !(err instanceof Error && err.message === 'cancelled')) setError(err)
    } finally {
      setBusy(false)
    }
  }

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
          {ssoError && <Alert>{t(`errors.${ssoError}`, { defaultValue: t('errors.sso_failed') })}</Alert>}
          <ErrorMessage error={error} />
          {providers.length > 0 && (
            <>
              <div className="flex flex-col gap-2.5">
                {providers.map((p) => (
                  <a key={p.id} href={`/api/auth/sso/${p.id}/start`}
                    className="inline-flex min-h-11 items-center justify-center gap-2.5 rounded-[10px] border border-accent/45 bg-accent/10 px-4 text-sm font-medium text-[#A5F3FC] transition hover:bg-accent/15">
                    {p.kind === 'entra' && <MicrosoftMark />}
                    {t('login.ssoWith', { name: p.name })}
                  </a>
                ))}
              </div>
              <div className="flex items-center gap-3 text-xs text-subtle"><span className="h-px flex-1 bg-line" />{t('login.or')}<span className="h-px flex-1 bg-line" /></div>
            </>
          )}
          <Input label={t('login.username')} value={username} onChange={(e) => setUsername(e.target.value)} required autoFocus autoComplete="username" />
          <Input label={t('login.password')} type="password" value={password} onChange={(e) => setPassword(e.target.value)} required autoComplete="current-password" />
          <Button type="submit" busy={busy}>{t('login.submit')}</Button>
          {passkeysSupported() && (
            <Button type="button" variant="ghost" disabled={busy} onClick={passkey}><Fingerprint className="size-4" aria-hidden />{t('login.passkey')}</Button>
          )}
        </form>
      )}
    </AuthShell>
  )
}

function MicrosoftMark() {
  return (
    <svg viewBox="0 0 21 21" className="size-4" aria-hidden>
      <rect x="1" y="1" width="9" height="9" fill="#F25022" />
      <rect x="11" y="1" width="9" height="9" fill="#7FBA00" />
      <rect x="1" y="11" width="9" height="9" fill="#00A4EF" />
      <rect x="11" y="11" width="9" height="9" fill="#FFB900" />
    </svg>
  )
}
