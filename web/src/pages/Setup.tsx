import { useEffect, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, Server, ShieldCheck } from 'lucide-react'
import { AuthShell } from '../components/AuthShell'
import { Button, ErrorMessage, Input, Alert } from '../components/ui'
import { languages } from '../i18n'
import { api } from '../lib/api'
import { useSession } from '../lib/session'

const steps = ['welcome', 'admin', 'security'] as const

export function Setup() {
  const { t, i18n } = useTranslation()
  const { refresh } = useSession()
  const [step, setStep] = useState(0)
  const [tokenRequired, setTokenRequired] = useState(false)
  const [form, setForm] = useState({ token: '', organization: '', username: 'admin', displayName: '', password: '', repeat: '' })
  const [error, setError] = useState<unknown>(null)
  const [mismatch, setMismatch] = useState(false)
  const [busy, setBusy] = useState(false)
  const [standby, setStandby] = useState(false)

  useEffect(() => {
    api<{ tokenRequired: boolean }>('GET', '/setup/status').then((s) => setTokenRequired(s.tokenRequired)).catch(() => undefined)
  }, [])

  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, [k]: e.target.value })

  function next(e: FormEvent) {
    e.preventDefault()
    if (step === 1) {
      if (form.password !== form.repeat) return setMismatch(true)
      setMismatch(false)
    }
    setStep(step + 1)
  }

  async function create(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await api('POST', '/setup', {
        token: form.token,
        organization: form.organization,
        username: form.username,
        displayName: form.displayName,
        password: form.password,
        language: i18n.language,
      })
      await refresh()
    } catch (err) {
      setError(err)
      setStep(1)
    } finally {
      setBusy(false)
    }
  }

  if (standby) return <JoinStandby tokenRequired={tokenRequired} endpoint="/setup/cluster-join" onBack={() => setStandby(false)} />

  return (
    <AuthShell wide>
      <ol className="mb-7 flex items-center gap-2" aria-label="Progress">
        {steps.map((s, i) => (
          <li key={s} className="flex flex-1 items-center gap-2" aria-current={i === step ? 'step' : undefined}>
            <span
              className={`inline-flex size-7 flex-none items-center justify-center rounded-full border font-mono text-xs ${
                i < step ? 'border-accent bg-accent text-on-accent' : i === step ? 'border-accent text-accent' : 'border-line text-subtle'
              }`}
            >
              {i < step ? <Check className="size-3.5" aria-hidden /> : i + 1}
            </span>
            <span className={`hidden text-xs sm:inline ${i === step ? 'text-ink' : 'text-subtle'}`}>{t(`setup.steps.${s}`)}</span>
            {i < steps.length - 1 && <span className={`h-px flex-1 ${i < step ? 'bg-accent/60' : 'bg-line'}`} />}
          </li>
        ))}
      </ol>

      {step === 0 && (
        <form onSubmit={next} className="flex flex-col gap-6">
          <div>
            <h1 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">{t('setup.title')}</h1>
            <p className="mt-2 text-muted">{t('setup.subtitle')}</p>
          </div>
          <fieldset>
            <legend className="mb-2 text-sm font-medium text-ink-2">{t('common.language')}</legend>
            <div className="grid grid-cols-2 gap-2">
              {languages.map((l) => (
                <label key={l.code} className={`flex min-h-12 cursor-pointer items-center gap-3 rounded-xl border px-4 ${i18n.language === l.code ? 'border-accent/50 bg-accent/10' : 'border-line hover:bg-tint/5'}`}>
                  <input type="radio" name="lang" checked={i18n.language === l.code} onChange={() => i18n.changeLanguage(l.code)} className="accent-accent" />
                  <span>{l.label}</span>
                </label>
              ))}
            </div>
            <p className="mt-2 text-xs text-subtle">{t('setup.languageHint')}</p>
          </fieldset>
          {tokenRequired && (
            <Input name="token" label={t('setup.tokenLabel')} hint={t('setup.tokenHint')} value={form.token} onChange={set('token')} required autoComplete="off" spellCheck={false} className="font-mono uppercase tracking-widest" placeholder="XXXX-XXXX-XXXX" />
          )}
          <Input name="organization" label={t('setup.organization')} hint={t('setup.organizationHint')} value={form.organization} onChange={set('organization')} required autoComplete="organization" />
          <div className="flex flex-wrap items-center justify-between gap-3">
            <button type="button" onClick={() => setStandby(true)} className="inline-flex items-center gap-2 text-sm text-muted hover:text-ink">
              <Server className="size-4" aria-hidden /> {t('setup.joinStandby')}
            </button>
            <Button type="submit">{t('common.continue')}</Button>
          </div>
        </form>
      )}

      {step === 1 && (
        <form onSubmit={next} className="flex flex-col gap-5">
          <h1 className="font-display text-2xl font-semibold tracking-tight">{t('setup.steps.admin')}</h1>
          <ErrorMessage error={error} />
          <div className="grid gap-5 sm:grid-cols-2">
            <Input name="displayName" label={t('setup.displayName')} value={form.displayName} onChange={set('displayName')} required autoComplete="name" />
            <Input name="username" label={t('setup.username')} value={form.username} onChange={set('username')} required autoComplete="username" />
          </div>
          <Input name="password" label={t('setup.password')} hint={t('setup.passwordHint')} type="password" value={form.password} onChange={set('password')} required minLength={12} autoComplete="new-password" />
          <Input name="repeat" label={t('setup.passwordRepeat')} type="password" value={form.repeat} onChange={set('repeat')} required minLength={12} autoComplete="new-password" />
          {mismatch && <Alert>{t('setup.passwordMismatch')}</Alert>}
          <div className="flex justify-between gap-2">
            <Button type="button" variant="ghost" onClick={() => setStep(0)}>{t('common.back')}</Button>
            <Button type="submit">{t('common.continue')}</Button>
          </div>
        </form>
      )}

      {step === 2 && (
        <form onSubmit={create} className="flex flex-col gap-5">
          <div className="flex items-start gap-4 rounded-xl border border-accent-2/30 bg-accent-2/10 p-4">
            <ShieldCheck className="mt-0.5 size-6 flex-none text-accent-2" aria-hidden />
            <div>
              <h1 className="font-display text-lg font-semibold">{t('setup.securityTitle')}</h1>
              <p className="mt-1 text-sm text-ink-2">{t('setup.securityText')}</p>
            </div>
          </div>
          <ErrorMessage error={error} />
          <div className="flex justify-between gap-2">
            <Button type="button" variant="ghost" onClick={() => setStep(1)}>{t('common.back')}</Button>
            <Button type="submit" busy={busy}>{t('setup.create')}</Button>
          </div>
        </form>
      )}
    </AuthShell>
  )
}

/**
 * The first sign-in with the generated administrator: choose the language, the organization name
 * and an own password. Two-factor authentication follows, then the rest is done in the portal.
 */
export function FirstRun() {
  const { t, i18n } = useTranslation()
  const { refresh, signOut } = useSession()
  const [step, setStep] = useState(0)
  const [form, setForm] = useState({ organization: '', displayName: '', password: '', repeat: '' })
  const [error, setError] = useState<unknown>(null)
  const [mismatch, setMismatch] = useState(false)
  const [busy, setBusy] = useState(false)
  const [standby, setStandby] = useState(false)

  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => setForm({ ...form, [k]: e.target.value })

  function next(e: FormEvent) {
    e.preventDefault()
    if (step === 1) {
      if (form.password !== form.repeat) return setMismatch(true)
      setMismatch(false)
    }
    setStep(step + 1)
  }

  async function finish(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await api('POST', '/setup/first-run', { organization: form.organization, displayName: form.displayName, password: form.password, language: i18n.language })
      await refresh()
    } catch (err) {
      setError(err)
      setStep(1)
    } finally {
      setBusy(false)
    }
  }

  if (standby) return <JoinStandby tokenRequired={false} endpoint="/setup/first-run/cluster-join" onBack={() => setStandby(false)} />

  return (
    <AuthShell wide>
      <ol className="mb-7 flex items-center gap-2" aria-label="Progress">
        {steps.map((s, i) => (
          <li key={s} className="flex flex-1 items-center gap-2" aria-current={i === step ? 'step' : undefined}>
            <span
              className={`inline-flex size-7 flex-none items-center justify-center rounded-full border font-mono text-xs ${
                i < step ? 'border-accent bg-accent text-on-accent' : i === step ? 'border-accent text-accent' : 'border-line text-subtle'
              }`}
            >
              {i < step ? <Check className="size-3.5" aria-hidden /> : i + 1}
            </span>
            <span className={`hidden text-xs sm:inline ${i === step ? 'text-ink' : 'text-subtle'}`}>{t(`setup.steps.${s}`)}</span>
            {i < steps.length - 1 && <span className={`h-px flex-1 ${i < step ? 'bg-accent/60' : 'bg-line'}`} />}
          </li>
        ))}
      </ol>

      {step === 0 && (
        <form onSubmit={next} className="flex flex-col gap-6">
          <div>
            <h1 className="font-display text-2xl font-semibold tracking-tight sm:text-3xl">{t('setup.title')}</h1>
            <p className="mt-2 text-muted">{t('setup.firstRunSubtitle')}</p>
          </div>
          <fieldset>
            <legend className="mb-2 text-sm font-medium text-ink-2">{t('common.language')}</legend>
            <div className="grid grid-cols-2 gap-2">
              {languages.map((l) => (
                <label key={l.code} className={`flex min-h-12 cursor-pointer items-center gap-3 rounded-xl border px-4 ${i18n.language === l.code ? 'border-accent/50 bg-accent/10' : 'border-line hover:bg-tint/5'}`}>
                  <input type="radio" name="lang" checked={i18n.language === l.code} onChange={() => i18n.changeLanguage(l.code)} className="accent-accent" />
                  <span>{l.label}</span>
                </label>
              ))}
            </div>
            <p className="mt-2 text-xs text-subtle">{t('setup.languageHint')}</p>
          </fieldset>
          <Input name="organization" label={t('setup.organization')} hint={t('setup.organizationHint')} value={form.organization} onChange={set('organization')} required autoComplete="organization" />
          <div className="flex flex-wrap items-center justify-between gap-3">
            <button type="button" onClick={() => setStandby(true)} className="inline-flex items-center gap-2 text-sm text-muted hover:text-ink">
              <Server className="size-4" aria-hidden /> {t('setup.joinStandby')}
            </button>
            <div className="flex gap-2">
              <Button type="button" variant="ghost" onClick={signOut}>{t('common.signOut')}</Button>
              <Button type="submit">{t('common.continue')}</Button>
            </div>
          </div>
        </form>
      )}

      {step === 1 && (
        <form onSubmit={next} className="flex flex-col gap-5">
          <div>
            <h1 className="font-display text-2xl font-semibold tracking-tight">{t('setup.steps.admin')}</h1>
            <p className="mt-2 text-sm text-muted">{t('setup.newPasswordText')}</p>
          </div>
          <ErrorMessage error={error} />
          <Input name="displayName" label={t('setup.displayName')} value={form.displayName} onChange={set('displayName')} required autoComplete="name" />
          <Input name="password" label={t('setup.newPassword')} hint={t('setup.passwordHint')} type="password" value={form.password} onChange={set('password')} required minLength={12} autoComplete="new-password" />
          <Input name="repeat" label={t('setup.passwordRepeat')} type="password" value={form.repeat} onChange={set('repeat')} required minLength={12} autoComplete="new-password" />
          {mismatch && <Alert>{t('setup.passwordMismatch')}</Alert>}
          <div className="flex justify-between gap-2">
            <Button type="button" variant="ghost" onClick={() => setStep(0)}>{t('common.back')}</Button>
            <Button type="submit">{t('common.continue')}</Button>
          </div>
        </form>
      )}

      {step === 2 && (
        <form onSubmit={finish} className="flex flex-col gap-5">
          <div className="flex items-start gap-4 rounded-xl border border-accent-2/30 bg-accent-2/10 p-4">
            <ShieldCheck className="mt-0.5 size-6 flex-none text-accent-2" aria-hidden />
            <div>
              <h1 className="font-display text-lg font-semibold">{t('setup.securityTitle')}</h1>
              <p className="mt-1 text-sm text-ink-2">{t('setup.securityText')}</p>
            </div>
          </div>
          <ErrorMessage error={error} />
          <div className="flex justify-between gap-2">
            <Button type="button" variant="ghost" onClick={() => setStep(1)}>{t('common.back')}</Button>
            <Button type="submit" busy={busy}>{t('setup.finish')}</Button>
          </div>
        </form>
      )}
    </AuthShell>
  )
}

/** Makes this fresh installation the standby node of an existing Kiestoon (high availability). */
function JoinStandby({ tokenRequired, endpoint, onBack }: { tokenRequired: boolean; endpoint: string; onBack: () => void }) {
  const { t } = useTranslation()
  const { refresh } = useSession()
  const [f, setF] = useState({ token: '', code: '', address: '' })
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState('')
  async function join(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const r = await api<{ primary: string }>('POST', endpoint, f)
      setDone(r.primary)
      // The portal restarts as standby.
      window.setTimeout(() => { refresh() }, 6000)
    } catch (err) {
      setError(err)
    } finally {
      setBusy(false)
    }
  }
  return (
    <AuthShell wide>
      <form onSubmit={join} className="flex flex-col gap-5">
        <div>
          <h1 className="font-display text-2xl font-semibold tracking-tight">{t('setup.standbyTitle')}</h1>
          <p className="mt-2 text-sm text-muted">{t('setup.standbyText')}</p>
        </div>
        <ErrorMessage error={error} />
        {done ? <Alert tone="ok">{t('setup.standbyDone', { name: done })}</Alert> : (
          <>
            {tokenRequired && (
              <Input name="token" label={t('setup.tokenLabel')} hint={t('setup.tokenHint')} value={f.token} onChange={(e) => setF({ ...f, token: e.target.value })} required autoComplete="off" spellCheck={false} className="font-mono uppercase tracking-widest" placeholder="XXXX-XXXX-XXXX" />
            )}
            <Input name="code" label={t('setup.standbyCode')} hint={t('setup.standbyCodeHint')} value={f.code} onChange={(e) => setF({ ...f, code: e.target.value })} required className="font-mono" placeholder="KT1-H-…" />
            <Input name="address" label={t('cluster.address')} hint={t('setup.standbyAddressHint')} value={f.address} onChange={(e) => setF({ ...f, address: e.target.value })} className="font-mono" />
            <div className="flex justify-between gap-2">
              <Button type="button" variant="ghost" onClick={onBack}>{t('common.back')}</Button>
              <Button type="submit" busy={busy}>{t('setup.standbyJoin')}</Button>
            </div>
          </>
        )}
      </form>
    </AuthShell>
  )
}
