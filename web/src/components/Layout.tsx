import { useEffect, useState } from 'react'
import { NavLink, Outlet, useLocation } from 'react-router'
import { useTranslation } from 'react-i18next'
import { LogOut, Menu, X } from 'lucide-react'
import { Brand, LanguageSwitch, PoweredBy, ThemeSwitch } from './brand'
import { InfoDialog, useSystemInfo } from './InfoDialog'
import { nav, visible } from './nav'
import { SoftphoneDock } from './SoftphoneDock'
import { useApi } from './kit'
import { isAdmin } from '../lib/api'
import { useSession } from '../lib/session'
import { SoftphoneProvider } from '../lib/softphone'
import type { PortalMe } from '../lib/types'

export function Layout() {
  const { t } = useTranslation()
  const { state, signOut } = useSession()
  const me = state.kind === 'ready' ? state.me : null
  const admin = isAdmin(me?.user)
  const hasExtension = !!me?.extension
  const portal = useApi<PortalMe>(hasExtension ? '/portal/me' : null)
  const sub = admin ? t('app.admin') : t('app.portal')
  const [open, setOpen] = useState(false)
  const [infoOpen, setInfoOpen] = useState(false)
  const { info, reload } = useSystemInfo()
  const location = useLocation()

  useEffect(() => setOpen(false), [location.pathname])

  const initials = (me?.user.displayName || me?.user.username || '?')
    .split(/\s+/)
    .map((p) => p[0])
    .join('')
    .slice(0, 2)
    .toUpperCase()

  return (
    <SoftphoneProvider webrtc={!!portal.data?.webrtc}>
    <div className="min-h-screen lg:flex">
      {/* Phone / tablet top bar */}
      <div className="sticky top-0 z-30 flex items-center justify-between border-b border-line bg-surface-2/90 px-4 py-3 backdrop-blur lg:hidden">
        <Brand sub={sub} />
        <button
          type="button"
          onClick={() => setOpen(true)}
          aria-label={t('nav.openMenu')}
          className="inline-flex size-11 items-center justify-center rounded-xl border border-line text-ink-2"
        >
          <Menu className="size-5" aria-hidden />
        </button>
      </div>

      {open && <div className="fixed inset-0 z-40 bg-black/60 lg:hidden" onClick={() => setOpen(false)} aria-hidden />}

      <aside
        className={`fixed inset-y-0 left-0 z-50 flex w-[272px] flex-col gap-0.5 overflow-y-auto border-r border-line bg-surface-2/95 px-3.5 py-5 backdrop-blur transition-transform lg:sticky lg:top-0 lg:h-screen lg:translate-x-0 ${
          open ? 'translate-x-0' : '-translate-x-full'
        }`}
        aria-label={t('nav.menu')}
      >
        <div className="flex items-center justify-between px-3 pb-4">
          <Brand sub={sub} />
          <button type="button" onClick={() => setOpen(false)} aria-label={t('nav.closeMenu')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted lg:hidden">
            <X className="size-5" aria-hidden />
          </button>
        </div>
        <nav className="flex flex-col gap-0.5">
          {nav.map((group, gi) => {
            const items = group.items.filter((i) => visible(i, me?.user.role, hasExtension))
            if (!items.length) return null
            return (
              <div key={gi} className="flex flex-col gap-0.5">
                {group.key && <div className="px-3 pb-1.5 pt-4 text-[11px] uppercase tracking-[0.12em] text-subtle">{t(`nav.${group.key}`)}</div>}
                {items.map((item) => (
                  <NavLink
                    key={item.key}
                    to={item.path}
                    end={item.path === '/' || item.path === '/me'}
                    className={({ isActive }) =>
                      `group flex min-h-9 items-center gap-3 rounded-[10px] px-3 text-sm transition ${
                        isActive ? 'bg-accent/12 text-ink shadow-[inset_0_0_0_1px_rgb(34_211_238_/_0.35)]' : 'text-nav hover:bg-tint/5 hover:text-ink'
                      }`
                    }
                  >
                    {({ isActive }) => (
                      <>
                        <item.icon className={`size-[18px] flex-none ${isActive ? 'text-accent' : ''}`} strokeWidth={1.8} aria-hidden />
                        <span className="flex-1 truncate">{t(`nav.${item.key}`)}</span>
                        {item.phase && <span className="font-mono text-[10px] text-subtle/80 opacity-0 transition group-hover:opacity-100">F{item.phase}</span>}
                      </>
                    )}
                  </NavLink>
                ))}
              </div>
            )
          })}
        </nav>

        <div className="mt-auto pt-5">
          <div className="card rounded-[14px] p-3.5">
            <div className="flex items-center justify-between">
              <span className="text-xs text-muted">{t('info.version')}</span>
              <span className="font-mono text-xs text-ink">{info?.version ?? '…'}</span>
            </div>
            <div className="mt-2 flex items-center gap-2">
              <span className={`size-2 rounded-full ${info?.channel === 'testing' ? 'bg-warn shadow-[0_0_10px_var(--color-warn)]' : 'bg-ok glow-ok'}`} aria-hidden />
              <span className="text-xs text-ink-2">{info?.channel === 'testing' ? t('info.testingChannel') : t('info.productionChannel')}</span>
            </div>
            <button
              type="button"
              onClick={() => setInfoOpen(true)}
              className="mt-3 min-h-10 w-full rounded-[10px] border border-accent/45 bg-accent/10 text-[13px] font-medium text-accent-ink transition hover:bg-accent/15"
            >
              {t('info.button')}
            </button>
          </div>
        </div>
      </aside>

      <div className="min-w-0 flex-1">
        <div className="hidden items-center justify-end gap-2.5 px-8 pt-6 lg:flex">
          <ThemeSwitch persist />
          <LanguageSwitch persist />
          <NavLink
            to="/account"
            aria-label={t('nav.account')}
            title={me?.user.displayName}
            className="inline-flex size-11 items-center justify-center rounded-xl border border-accent-2/45 bg-gradient-to-br from-accent-2/35 to-accent/25 text-[13px] font-semibold text-ink"
          >
            {initials}
          </NavLink>
          <button type="button" onClick={signOut} aria-label={t('common.signOut')} title={t('common.signOut')} className="inline-flex size-11 items-center justify-center rounded-xl border border-line bg-surface/90 text-ink-2 hover:text-ink">
            <LogOut className="size-[18px]" aria-hidden />
          </button>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2 px-4 pt-4 lg:hidden">
          <ThemeSwitch persist />
          <LanguageSwitch persist />
          <NavLink to="/account" className="inline-flex min-h-11 items-center rounded-xl border border-line px-3 text-sm text-ink-2">
            {t('nav.account')}
          </NavLink>
          <button type="button" onClick={signOut} aria-label={t('common.signOut')} className="inline-flex size-11 items-center justify-center rounded-xl border border-line text-ink-2">
            <LogOut className="size-[18px]" aria-hidden />
          </button>
        </div>
        <main className="mx-auto flex max-w-[1400px] flex-col gap-6 px-4 pb-28 pt-4 sm:px-8 lg:pt-2">
          <Outlet context={{ info, openInfo: () => setInfoOpen(true) }} />
          <PoweredBy className="flex self-end sm:hidden" />
        </main>
      </div>

      <PoweredBy className="fixed bottom-4 right-6 z-20 hidden sm:flex" />
      {infoOpen && <InfoDialog admin={admin} onClose={() => { setInfoOpen(false); reload() }} />}
      <SoftphoneDock />
    </div>
    </SoftphoneProvider>
  )
}
