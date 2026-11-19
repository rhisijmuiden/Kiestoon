import { useTranslation } from 'react-i18next'
import { Monitor, Moon, Sun } from 'lucide-react'
import rhisLogo from '../assets/rhis-logo-wit.svg'
import { languages } from '../i18n'
import { api } from '../lib/api'
import { setThemePref, themePrefs, useTheme, type ThemePref } from '../lib/theme'

export function Mark({ size = 34 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 34 34" aria-hidden="true">
      <rect x="1" y="1" width="32" height="32" rx="10" className="fill-accent/15 stroke-accent" />
      <path d="M9 17h3l2-6 4 12 2-6h5" fill="none" className="stroke-accent" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

export function Brand({ sub }: { sub?: string }) {
  return (
    <div className="flex items-center gap-3">
      <Mark />
      <div>
        <div className="font-display text-[19px] font-bold tracking-tight">Kiestoon</div>
        {sub && <div className="text-[11px] uppercase tracking-[0.14em] text-subtle">{sub}</div>}
      </div>
    </div>
  )
}

/** "Powered by RHIS" with the RHIS logo. The logo keeps its own colours (RHIS house style). */
export function PoweredBy({ className = '' }: { className?: string }) {
  const { t } = useTranslation()
  return (
    <div className={`items-center gap-2.5 rhis-badge rounded-xl px-3 py-2 backdrop-blur ${className}`}>
      <span className="text-xs">{t('app.poweredBy')}</span>
      <img src={rhisLogo} alt="RHIS" className="block h-[18px] w-auto" />
    </div>
  )
}

/** EN / NL toggle. When signed in, the choice is saved on the user's account. */
export function LanguageSwitch({ persist = false }: { persist?: boolean }) {
  const { t, i18n } = useTranslation()
  return (
    <div role="group" aria-label={t('common.language')} className="inline-flex min-h-11 items-center rounded-xl border border-line bg-surface/90 p-1">
      {languages.map((l) => {
        const active = i18n.language === l.code
        return (
          <button
            key={l.code}
            type="button"
            aria-pressed={active}
            title={l.label}
            onClick={() => {
              i18n.changeLanguage(l.code)
              if (persist) api('PUT', '/account/language', { language: l.code }).catch(() => undefined)
            }}
            className={`min-h-9 min-w-11 rounded-lg px-2.5 text-[13px] font-medium uppercase transition ${
              active ? 'bg-accent/15 text-ink shadow-[inset_0_0_0_1px_rgb(34_211_238_/_0.35)]' : 'text-muted hover:text-ink'
            }`}
          >
            {l.code}
          </button>
        )
      })}
    </div>
  )
}

const themeIcons: Record<ThemePref, typeof Sun> = { system: Monitor, light: Sun, dark: Moon }

/** System / light / dark toggle. When signed in, the choice is saved on the user's account. */
export function ThemeSwitch({ persist = false, labels = false }: { persist?: boolean; labels?: boolean }) {
  const { t } = useTranslation()
  const { pref } = useTheme()
  return (
    <div role="group" aria-label={t('theme.label')} className="inline-flex min-h-11 items-center rounded-xl border border-line bg-surface/90 p-1">
      {themePrefs.map((p) => {
        const Icon = themeIcons[p]
        const active = pref === p
        return (
          <button
            key={p}
            type="button"
            aria-pressed={active}
            aria-label={labels ? undefined : t(`theme.${p}`)}
            title={t(`theme.${p}`)}
            onClick={() => {
              setThemePref(p)
              if (persist) api('PUT', '/account/theme', { theme: p }).catch(() => undefined)
            }}
            className={`inline-flex min-h-9 min-w-10 items-center justify-center gap-2 rounded-lg px-2 text-[13px] font-medium transition ${labels ? 'px-3' : ''} ${
              active ? 'bg-accent/15 text-ink shadow-[inset_0_0_0_1px_rgb(34_211_238_/_0.35)]' : 'text-muted hover:text-ink'
            }`}
          >
            <Icon className="size-[17px]" strokeWidth={1.8} aria-hidden />
            {labels && <span>{t(`theme.${p}`)}</span>}
          </button>
        )
      })}
    </div>
  )
}
