import { useEffect, type ReactElement } from 'react'
import { Navigate, Route, Routes, useLocation, useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { Loader2, PhoneOff } from 'lucide-react'
import { Layout } from './components/Layout'
import { Empty } from './components/kit'
import { allItems, visible } from './components/nav'
import { Card } from './components/ui'
import { Account } from './pages/Account'
import { Audit } from './pages/Audit'
import { Backups } from './pages/Backups'
import { Conferences } from './pages/Conferences'
import { Dashboard } from './pages/Dashboard'
import { Extensions } from './pages/Extensions'
import { Fax } from './pages/Fax'
import { Integrations } from './pages/Integrations'
import { Internet } from './pages/Internet'
import { Phonebook } from './pages/Phonebook'
import { Wallboard } from './pages/Wallboard'
import { WallboardSettings } from './pages/WallboardSettings'
import { WallScreen } from './pages/WallScreen'
import { Features } from './pages/Features'
import { Firewall } from './pages/Firewall'
import { History } from './pages/History'
import { IVRs } from './pages/IVR'
import { Login } from './pages/Login'
import { Mail } from './pages/Mail'
import { MfaEnroll } from './pages/MfaEnroll'
import { Phones } from './pages/Phones'
import { Prompts } from './pages/Prompts'
import { Speech } from './pages/Speech'
import { AI } from './pages/AI'
import { Standby } from './pages/Standby'
import { Sites } from './pages/Sites'
import { Federation } from './pages/Federation'
import { SunRoutes } from './pages/SunRoutes'
import { Cluster } from './pages/Cluster'
import { Queues } from './pages/Queues'
import { Reception } from './pages/Reception'
import { Recordings } from './pages/Recordings'
import { RingGroups } from './pages/RingGroups'
import { Settings } from './pages/Settings'
import { Setup } from './pages/Setup'
import { SetPassword } from './pages/SetPassword'
import { Sso } from './pages/Sso'
import { TimeConditions } from './pages/TimeConditions'
import { Trunks } from './pages/Trunks'
import { Users } from './pages/Users'
import { Colleagues } from './pages/portal/Colleagues'
import { MyCalls } from './pages/portal/MyCalls'
import { MyDevices } from './pages/portal/MyDevices'
import { MyPhone } from './pages/portal/MyPhone'
import { MyQueues } from './pages/portal/MyQueues'
import { Voicemail } from './pages/portal/Voicemail'
import { roleRank } from './lib/api'
import { useSession } from './lib/session'
import { DesktopApp } from './desktop/DesktopApp'
import { inShell } from './desktop/bridge'
import { DesktopLink } from './components/WindowsSoftphone'

const pages: Record<string, ReactElement> = {
  dashboard: <Dashboard />, myPhone: <MyPhone />, voicemail: <Voicemail />, myQueues: <MyQueues />, colleagues: <Colleagues />,
  myCalls: <MyCalls />, devices: <MyDevices />, switchboard: <Reception />, history: <History />, recordings: <Recordings />,
  extensions: <Extensions />, ringGroups: <RingGroups />, queues: <Queues />, ivr: <IVRs />, timeConditions: <TimeConditions />,
  conferences: <Conferences />, prompts: <Prompts />, trunks: <Trunks />, phones: <Phones />, users: <Users />, sso: <Sso />,
  features: <Features />, firewall: <Firewall />, backups: <Backups />, mail: <Mail />, audit: <Audit />, settings: <Settings />,
  phonebook: <Phonebook />, wallboard: <Wallboard />, fax: <Fax />, internet: <Internet />, integrations: <Integrations />, speech: <Speech />, ai: <AI />,
  sites: <Sites />, federation: <Federation />, sunRoutes: <SunRoutes />, cluster: <Cluster />,
}

function NoExtension() {
  const { t } = useTranslation()
  return <Card><Empty icon={<PhoneOff className="size-7" />} title={t('portal.noExtensionTitle')} text={t('portal.noExtensionText')} /></Card>
}

const AFTER_KEY = 'kiestoon.after'

export function App() {
  const { t } = useTranslation()
  const { state } = useSession()
  const { pathname } = useLocation()
  const navigate = useNavigate()

  // "Sign in with browser" from the Windows softphone: remember where to go back to after an SSO round trip.
  useEffect(() => {
    try {
      if (pathname === '/desktop-link' && state.kind !== 'ready') sessionStorage.setItem(AFTER_KEY, pathname)
      if (state.kind === 'ready' && pathname === '/' && sessionStorage.getItem(AFTER_KEY)) {
        sessionStorage.removeItem(AFTER_KEY)
        navigate('/desktop-link', { replace: true })
      }
    } catch {
      /* storage blocked */
    }
  }, [pathname, state.kind, navigate])

  // The link of the welcome e-mail works whether or not someone is signed in on this browser.
  if (pathname === '/set-password') return <SetPassword />
  // A tv link shows the wallboard without signing in; the key in the link is the secret.
  if (pathname.startsWith('/wall/')) return <WallScreen wallKey={pathname.slice(6)} />
  // The Windows softphone shows the compact /desktop pages, also after an SSO sign-in that ends on /.
  if (pathname === '/desktop' || pathname.startsWith('/desktop/') || inShell()) return <DesktopApp />

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
    case 'standby':
      return <Standby />
    case 'signedOut':
      return <Login mfa={false} />
    case 'mfa':
      return <Login mfa />
    case 'enroll':
      return <MfaEnroll />
  }

  if (pathname === '/desktop-link') return <DesktopLink />

  const role = state.me.user.role
  const hasExt = !!state.me.extension
  const items = allItems.filter((i) => i.path !== '/' && visible(i, role, hasExt) && pages[i.key])
  const home = roleRank(role) >= roleRank('reception') ? <Dashboard /> : hasExt ? <MyPhone /> : <NoExtension />

  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={home} />
        <Route path="account" element={<Account />} />
        {roleRank(role) >= roleRank('admin') && <Route path="wallboard/settings" element={<WallboardSettings />} />}
        {items.map((i) => <Route key={i.key} path={i.path.slice(1)} element={pages[i.key]} />)}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}
