import { useEffect, useRef, useState } from 'react'
import { NavLink, Outlet, useNavigate } from 'react-router'
import { useTranslation } from 'react-i18next'
import { BookUser, ChevronDown, Clock3, Grid3x3, Info as InfoIcon, ListOrdered, Settings as SettingsIcon, Voicemail as VoicemailIcon } from 'lucide-react'
import { PoweredBy } from '../components/brand'
import { api } from '../lib/api'
import { useSession } from '../lib/session'
import { SoftphoneProvider, useSoftphone } from '../lib/softphone'
import { TransferProvider } from '../lib/transfer'
import { Avatar } from './Avatar'
import { CallPanel } from './CallPanel'
import { DesktopData, myQueues, useDesktop } from './data'
import { ShellSync } from './ShellSync'
import { MobileSync, mobileAccount, pushActive } from './MobileSync'
import { inMobile, shell } from './bridge'

export function DesktopLayout() {
  const [version, setVersion] = useState('')
  useEffect(() => {
    shell.info().then((i) => setVersion(i?.version ?? '')).catch(() => undefined)
  }, [])
  return (
    <DesktopData>
      <Phone version={version} />
    </DesktopData>
  )
}

function Phone({ version }: { version: string }) {
  const { me } = useDesktop()
  const onMobile = inMobile()
  // With push the phone's own call screen rings; without it the page rings while it is open.
  const [push, setPush] = useState(false)
  useEffect(() => {
    if (onMobile) pushActive().then(setPush).catch(() => undefined)
  }, [onMobile])
  if (onMobile) {
    return (
      <SoftphoneProvider webrtc={!!me.data?.webrtc} auto account={mobileAccount} ring={!push} agent={`Kiestoon Mobile${version ? '/' + version : ''}`}>
        <TransferProvider>
          <MobileSync />
          <Frame />
        </TransferProvider>
      </SoftphoneProvider>
    )
  }
  return (
    <SoftphoneProvider webrtc={!!me.data?.webrtc} auto agent={`Kiestoon Softphone${version ? '/' + version : ''}`}>
      <TransferProvider>
        <ShellSync />
        <Frame />
      </TransferProvider>
    </SoftphoneProvider>
  )
}

function RailLink({ to, label, icon, badge, end }: { to: string; label: string; icon: React.ReactNode; badge?: number; end?: boolean }) {
  return (
    <NavLink to={to} end={end} title={label} aria-label={label}
      className={({ isActive }) => `relative inline-flex size-10 items-center justify-center rounded-[10px] transition ${isActive
        ? 'bg-accent/15 text-accent before:absolute before:-left-2 before:inset-y-2.5 before:w-[3px] before:rounded before:bg-accent'
        : 'text-muted hover:bg-tint/5 hover:text-ink'}`}>
      {icon}
      {!!badge && <span className="absolute right-1 top-1 rounded-full bg-danger px-1 text-[9px] font-bold leading-[14px] text-white">{badge}</span>}
    </NavLink>
  )
}

function Frame() {
  const { t } = useTranslation()
  const { voicemail, queues, me } = useDesktop()
  const phone = useSoftphone()
  const unread = (voicemail.data ?? []).filter((m) => m.folder === 'INBOX').length
  const q = myQueues(queues.data, me.data?.id)
  const icon = 'size-5'
  const links = (
    <>
      <RailLink to="/desktop" end label={t('desktop.nav.contacts')} icon={<BookUser className={icon} aria-hidden />} />
      <RailLink to="/desktop/history" label={t('desktop.nav.history')} icon={<Clock3 className={icon} aria-hidden />} />
      <RailLink to="/desktop/keypad" label={t('desktop.nav.keypad')} icon={<Grid3x3 className={icon} aria-hidden />} />
      <RailLink to="/desktop/queues" label={t('desktop.nav.queues')} icon={<ListOrdered className={icon} aria-hidden />} />
      {me.data?.voicemail && <RailLink to="/desktop/voicemail" label={t('desktop.nav.voicemail')} badge={unread} icon={<VoicemailIcon className={icon} aria-hidden />} />}
    </>
  )
  const status = q.mine.length > 0
    ? <span><b className="font-semibold text-muted">{t('desktop.queuesSignedIn', { count: q.signedIn.length, total: q.mine.length })}</b></span>
    : <span>{phone.status === 'registered' ? t('desktop.status.registered') : t(`desktop.status.${phone.status}`)}</span>

  // The phone app: the menu as a tab bar at the bottom, clear of the notch and the home indicator.
  if (inMobile()) {
    return (
      <div className="flex h-dvh flex-col overflow-hidden pt-[env(safe-area-inset-top)]">
        <Header />
        <main className="min-h-0 flex-1 overflow-y-auto">
          {phone.call && <CallPanel />}
          <Outlet />
        </main>
        <div className="flex-none border-t border-line px-3.5 py-1 text-[11px] text-subtle">{status}</div>
        <nav aria-label={t('nav.menu')}
          className="flex flex-none items-center justify-around border-t border-line bg-surface-2 px-2 pt-1.5 pb-[max(0.375rem,env(safe-area-inset-bottom))]">
          {links}
          <RailLink to="/desktop/settings" label={t('desktop.nav.settings')} icon={<SettingsIcon className={icon} aria-hidden />} />
          <RailLink to="/desktop/info" label={t('desktop.nav.info')} icon={<InfoIcon className={icon} aria-hidden />} />
        </nav>
      </div>
    )
  }

  return (
    <div className="flex h-screen overflow-hidden">
      <nav aria-label={t('nav.menu')} className="flex w-14 flex-none flex-col items-center gap-1.5 border-r border-line bg-surface-2 py-2.5">
        {links}
        <div className="flex-1" />
        <RailLink to="/desktop/settings" label={t('desktop.nav.settings')} icon={<SettingsIcon className={icon} aria-hidden />} />
        <RailLink to="/desktop/info" label={t('desktop.nav.info')} icon={<InfoIcon className={icon} aria-hidden />} />
      </nav>
      <div className="flex min-w-0 flex-1 flex-col">
        <Header />
        <main className="min-h-0 flex-1 overflow-y-auto">
          {phone.call && <CallPanel />}
          <Outlet />
        </main>
        <footer className="flex flex-none items-center gap-2 border-t border-line px-3.5 py-1.5 text-[11px] text-subtle">
          {status}
          <PoweredBy className="ml-auto flex scale-[0.8] origin-right !p-1" />
        </footer>
      </div>
    </div>
  )
}

/** The user, the line status and the presence switch (available / do not disturb). */
function Header() {
  const { t } = useTranslation()
  const { state } = useSession()
  const { me } = useDesktop()
  const phone = useSoftphone()
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()
  const name = state.kind === 'ready' ? state.me.user.displayName || state.me.user.username : ''
  const dnd = !!me.data?.features.dnd
  useEffect(() => {
    if (!open) return
    const close = (e: MouseEvent) => { if (!ref.current?.contains(e.target as Node)) setOpen(false) }
    window.addEventListener('mousedown', close)
    return () => window.removeEventListener('mousedown', close)
  }, [open])
  async function setDnd(v: boolean) {
    setOpen(false)
    if (!me.data) return
    await api('PUT', '/portal/features', { ...me.data.features, dnd: v }).catch(() => undefined)
    me.reload()
  }
  const line = phone.status === 'registered' ? (dnd ? 'dnd' : 'available') : 'offline'
  const dot = { available: 'bg-ok', dnd: 'bg-danger', offline: 'bg-subtle' }[line]
  return (
    <header className="flex flex-none items-center gap-2.5 border-b border-line px-3.5 py-2.5">
      <button type="button" onClick={() => navigate('/desktop/settings')} className="flex min-w-0 items-center gap-2.5 text-left" title={t('desktop.nav.settings')}>
        <Avatar name={name} presence={line === 'available' ? 'available' : line === 'dnd' ? 'dnd' : 'offline'} />
        <span className="min-w-0">
          <span className="block truncate text-[13.5px] font-semibold text-ink">{name}{me.data ? ` · ${me.data.number}` : ''}</span>
          <span className="block truncate text-[11.5px] text-muted">{t(`desktop.status.${phone.status}`)}</span>
        </span>
      </button>
      <div ref={ref} className="relative ml-auto">
        <button type="button" onClick={() => setOpen(!open)} aria-haspopup="menu" aria-expanded={open}
          className="inline-flex items-center gap-1.5 whitespace-nowrap rounded-full border border-line px-2.5 py-1 text-[11px] text-ink-2 hover:bg-tint/5">
          <span className={`size-[7px] rounded-full ${dot}`} aria-hidden />{t(`desktop.presence.${line}`)}<ChevronDown className="size-3" aria-hidden />
        </button>
        {open && (
          <div role="menu" className="absolute right-0 top-full z-40 mt-1 w-44 overflow-hidden rounded-xl border border-line bg-surface py-1 shadow-2xl">
            {(['available', 'dnd'] as const).map((k) => (
              <button key={k} role="menuitemradio" aria-checked={(k === 'dnd') === dnd} type="button" onClick={() => setDnd(k === 'dnd')}
                className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm text-ink hover:bg-tint/5">
                <span className={`size-2 rounded-full ${k === 'dnd' ? 'bg-danger' : 'bg-ok'}`} aria-hidden />{t(`desktop.presence.${k}`)}
              </button>
            ))}
          </div>
        )}
      </div>
    </header>
  )
}
