import { Navigate, Route, Routes } from 'react-router'
import { useTranslation } from 'react-i18next'
import { Loader2 } from 'lucide-react'
import { Layout } from './components/Layout'
import { allItems } from './components/nav'
import { Account } from './pages/Account'
import { Audit } from './pages/Audit'
import { Backups } from './pages/Backups'
import { Extensions } from './pages/Extensions'
import { Firewall } from './pages/Firewall'
import { History } from './pages/History'
import { Mail } from './pages/Mail'
import { Phones } from './pages/Phones'
import { Settings } from './pages/Settings'
import { Trunks } from './pages/Trunks'
import { ComingSoon } from './pages/ComingSoon'
import { Dashboard } from './pages/Dashboard'
import { Login } from './pages/Login'
import { MfaEnroll } from './pages/MfaEnroll'
import { Setup } from './pages/Setup'
import { useSession } from './lib/session'

export function App() {
  const { t } = useTranslation()
  const { state } = useSession()

  switch (state.kind) {
    case 'loading':
      return (
        <div className="flex min-h-screen items-center justify-center gap-3 text-muted" role="status">
          <Loader2 className="size-5 animate-spin text-accent" aria-hidden />
          {t('app.loading')}
        </div>
      )
    case 'setup':
      return <Setup />
    case 'signedOut':
      return <Login mfa={false} />
    case 'mfa':
      return <Login mfa />
    case 'enroll':
      return <MfaEnroll />
  }

  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Dashboard />} />
        <Route path="account" element={<Account />} />
        <Route path="audit" element={<Audit />} />
        <Route path="backups" element={<Backups />} />
        <Route path="extensions" element={<Extensions />} />
        <Route path="firewall" element={<Firewall />} />
        <Route path="history" element={<History />} />
        <Route path="mail" element={<Mail />} />
        <Route path="phones" element={<Phones />} />
        <Route path="settings" element={<Settings />} />
        <Route path="trunks" element={<Trunks />} />
        {allItems
          .filter((i) => i.phase)
          .map((i) => (
            <Route key={i.key} path={i.path.slice(1)} element={<ComingSoon item={i} />} />
          ))}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}
