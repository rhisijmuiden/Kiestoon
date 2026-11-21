import { useEffect, useState } from 'react'
import { Navigate, Route, Routes } from 'react-router'
import { useTranslation } from 'react-i18next'
import { Loader2, LogOut, PhoneOff } from 'lucide-react'
import { MfaEnroll } from '../pages/MfaEnroll'
import { FirstRun } from '../pages/Setup'
import { Login } from '../pages/Login'
import { Button, ErrorMessage } from '../components/ui'
import { Mark } from '../components/brand'
import { api, ApiError } from '../lib/api'
import { useSession } from '../lib/session'
import { inShell, shell } from './bridge'
import { DesktopLayout } from './Layout'
import { Contacts } from './Contacts'
import { History } from './History'
import { Keypad } from './Keypad'
import { Queues } from './Queues'
import { Voicemail } from './Voicemail'
import { Settings } from './Settings'
import { Info } from './Info'

/** Swaps the session the softphone signed in with for a desktop session (30 days, user portal only). */
async function toDesktopSession() {
  const res = await fetch('/api/desktop/session', {
    method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json', 'X-Kiestoon-Desktop': '1' }, body: '{}',
  })
  if (!res.ok) {
    const d = (await res.json().catch(() => ({}))) as { error?: string }
    throw new ApiError(res.status, d.error ?? 'internal')
  }
}

function Centered({ children }: { children: React.ReactNode }) {
  return <div className="flex min-h-screen flex-col items-center justify-center gap-4 p-6 text-center">{children}</div>
}

function Busy() {
  const { t } = useTranslation()
  return <Centered><Loader2 className="size-6 animate-spin text-accent" aria-hidden /><span className="text-sm text-muted">{t('app.loading')}</span></Centered>
}

/** The pages of the Windows softphone, under /desktop. */
export function DesktopApp() {
  const { t } = useTranslation()
  const { state, refresh, signOut } = useSession()
  const [error, setError] = useState<unknown>(null)
  const [claiming, setClaiming] = useState(() => new URLSearchParams(window.location.search).has('pair'))

  // A pairing code from the portal (kiestoon://pair link or typed in the softphone).
  useEffect(() => {
    const code = new URLSearchParams(window.location.search).get('pair')
    if (!code) return
    window.history.replaceState(null, '', '/desktop')
    api('POST', '/desktop/claim', { code })
      .then(() => refresh())
      .catch(setError)
      .finally(() => setClaiming(false))
  }, [refresh])

  // Signed in with a password, passkey or SSO inside the softphone: make it a desktop session.
  const method = state.kind === 'ready' ? state.me.signInMethod : ''
  const hasExt = state.kind === 'ready' && !!state.me.extension
  useEffect(() => {
    if (state.kind !== 'ready' || !hasExt || method === 'desktop') return
    toDesktopSession().then(refresh).catch(setError)
  }, [state.kind, hasExt, method, refresh])

  if (claiming) return <Busy />

  switch (state.kind) {
    case 'loading':
      return <Busy />
    case 'setup':
    case 'standby':
      return <Centered><Mark size={48} /><p className="max-w-xs text-sm text-muted">{t('desktop.notReady')}</p></Centered>
    case 'signedOut':
    case 'mfa':
      return (
        <>
          {error ? <div className="fixed inset-x-4 top-4 z-50"><ErrorMessage error={error} /></div> : null}
          <Login mfa={state.kind === 'mfa'} />
          {inShell() && (
            <div className="fixed bottom-4 left-4 z-40">
              <Button variant="ghost" className="min-h-9 text-xs" onClick={() => shell.unpair()}>{t('desktop.otherServer')}</Button>
            </div>
          )}
        </>
      )
    case 'firstRun':
      return <FirstRun />
    case 'enroll':
      return <MfaEnroll />
  }

  if (!hasExt) {
    return (
      <Centered>
        <PhoneOff className="size-8 text-subtle" aria-hidden />
        <h1 className="font-display text-lg font-semibold">{t('portal.noExtensionTitle')}</h1>
        <p className="max-w-xs text-sm text-muted">{t('portal.noExtensionText')}</p>
        <Button variant="ghost" onClick={signOut}><LogOut className="size-4" aria-hidden /> {t('common.signOut')}</Button>
      </Centered>
    )
  }
  if (method !== 'desktop') return error ? <Centered><ErrorMessage error={error} /></Centered> : <Busy />

  return (
    <Routes>
      <Route path="desktop" element={<DesktopLayout />}>
        <Route index element={<Contacts />} />
        <Route path="history" element={<History />} />
        <Route path="keypad" element={<Keypad />} />
        <Route path="queues" element={<Queues />} />
        <Route path="voicemail" element={<Voicemail />} />
        <Route path="settings" element={<Settings />} />
        <Route path="info" element={<Info />} />
        <Route path="*" element={<Navigate to="/desktop" replace />} />
      </Route>
      <Route path="*" element={<Navigate to="/desktop" replace />} />
    </Routes>
  )
}
