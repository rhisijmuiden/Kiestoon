import { createContext, useCallback, useContext, useEffect, useState, type ReactNode } from 'react'
import i18n from '../i18n'
import { api, ApiError, type Me } from './api'

export type SessionState =
  | { kind: 'loading' }
  | { kind: 'setup' }
  | { kind: 'signedOut' }
  | { kind: 'mfa' }
  | { kind: 'enroll'; me: Me }
  | { kind: 'ready'; me: Me }

type Ctx = { state: SessionState; refresh: () => Promise<void>; signOut: () => Promise<void> }
const SessionContext = createContext<Ctx | null>(null)

export function SessionProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<SessionState>({ kind: 'loading' })

  const refresh = useCallback(async () => {
    try {
      const setup = await api<{ setupRequired: boolean }>('GET', '/setup/status')
      if (setup.setupRequired) return setState({ kind: 'setup' })
      const me = await api<Me>('GET', '/me')
      if (me.user.language && me.user.language !== i18n.language) i18n.changeLanguage(me.user.language)
      setState(me.mfaEnrollmentRequired ? { kind: 'enroll', me } : { kind: 'ready', me })
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

  return <SessionContext.Provider value={{ state, refresh, signOut }}>{children}</SessionContext.Provider>
}

export function useSession() {
  const ctx = useContext(SessionContext)
  if (!ctx) throw new Error('useSession outside SessionProvider')
  return ctx
}
