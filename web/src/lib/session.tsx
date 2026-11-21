import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'
import i18n from '../i18n'
import { api, ApiError, type Me } from './api'
import { getThemePref, isThemePref, setThemePref } from './theme'

export type SessionState =
  | { kind: 'loading' }
  | { kind: 'setup' }
  | { kind: 'standby' }
  | { kind: 'signedOut' }
  | { kind: 'mfa' }
  | { kind: 'firstRun'; me: Me }
  | { kind: 'enroll'; me: Me }
  | { kind: 'ready'; me: Me }

type Ctx = { state: SessionState; refresh: () => Promise<void>; signOut: () => Promise<void> }
const SessionContext = createContext<Ctx | null>(null)

export function SessionProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<SessionState>({ kind: 'loading' })

  const refresh = useCallback(async () => {
    try {
      const setup = await api<{ setupRequired: boolean; standby?: boolean }>('GET', '/setup/status')
      if (setup.standby) return setState({ kind: 'standby' })
      if (setup.setupRequired) return setState({ kind: 'setup' })
      const me = await api<Me>('GET', '/me')
      if (me.user.language && me.user.language !== i18n.language) i18n.changeLanguage(me.user.language)
      if (isThemePref(me.user.theme) && me.user.theme !== getThemePref()) setThemePref(me.user.theme)
      setState(me.firstRunRequired ? { kind: 'firstRun', me } : me.mfaEnrollmentRequired ? { kind: 'enroll', me } : { kind: 'ready', me })
    } catch (e) {
      if (e instanceof ApiError && e.code === 'mfa_required') return setState({ kind: 'mfa' })
      setState({ kind: 'signedOut' })
    }
  }, [])

  const signOut = useCallback(async () => {
    await api('POST', '/auth/logout').catch(() => undefined)
    setState({ kind: 'signedOut' })
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  useEffect(() => {
    const onMfa = () => setState((s) => (s.kind === 'ready' || s.kind === 'enroll' ? { kind: 'mfa' } : s))
    window.addEventListener('kiestoon:mfa-required', onMfa)
    return () => window.removeEventListener('kiestoon:mfa-required', onMfa)
  }, [])

  return <SessionContext.Provider value={{ state, refresh, signOut }}>{children}</SessionContext.Provider>
}

export function useSession() {
  const ctx = useContext(SessionContext)
  if (!ctx) throw new Error('useSession outside SessionProvider')
  return ctx
}
