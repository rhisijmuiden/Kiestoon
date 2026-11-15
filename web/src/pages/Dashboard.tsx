import { useTranslation } from 'react-i18next'
import { useOutletContext } from 'react-router'
import { Archive, ArrowUpRight, Mail, Phone, ShieldCheck, Smartphone, Waypoints } from 'lucide-react'
import { Card, PageHeader } from '../components/ui'
import type { SystemInfo } from '../lib/api'
import { useSession } from '../lib/session'

const ROADMAP = 'https://github.com/rhisijmuiden/kiestoon/blob/main/ROADMAP.md'

export function Dashboard() {
  const { t } = useTranslation()
  const { state } = useSession()
  const { info, openInfo } = useOutletContext<{ info: SystemInfo | null; openInfo: () => void }>()
  const me = state.kind === 'ready' ? state.me : null

  const next = [
    { icon: Phone, key: 'extensions' },
    { icon: Waypoints, key: 'trunks' },
    { icon: Smartphone, key: 'phones' },
    { icon: Mail, key: 'mail' },
    { icon: Archive, key: 'backups' },
  ]

  return (
    <>
      <PageHeader title={t('dashboard.title')} subtitle={me?.organization ? `${me.organization} · ${t('dashboard.subtitle')}` : t('dashboard.subtitle')}>
        <span className="inline-flex min-h-11 items-center gap-2 rounded-xl border border-ok/35 bg-ok/10 px-3.5 text-[13px] text-[#A7F3D0]">
          <span className="size-2 rounded-full bg-ok glow-ok" aria-hidden />
          {t('dashboard.status')}
        </span>
      </PageHeader>

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        <Card>
          <div className="text-[13px] text-muted">{t('dashboard.version')}</div>
          <button type="button" onClick={openInfo} className="mt-2.5 font-mono text-3xl font-medium text-ink hover:text-accent">
            {info?.version ?? '…'}
          </button>
          <div className="mt-1.5 text-xs text-subtle">{info?.commit?.slice(0, 7)}</div>
        </Card>
        <Card>
          <div className="text-[13px] text-muted">{t('dashboard.channel')}</div>
          <div className={`mt-2.5 font-mono text-3xl font-medium ${info?.channel === 'testing' ? 'text-warn' : 'text-ok'}`}>
            {info ? t(`info.${info.channel}`) : '…'}
          </div>
          <div className="mt-3 h-1.5 rounded-full bg-white/10">
            <div className="h-1.5 rounded-full bg-gradient-to-r from-accent to-accent-2" style={{ width: info?.channel === 'testing' ? '100%' : '50%' }} />
          </div>
        </Card>
        <Card>
          <div className="text-[13px] text-muted">{t('dashboard.mfa')}</div>
          <div className="mt-2.5 flex items-center gap-2 font-mono text-3xl font-medium text-ok">
            <ShieldCheck className="size-7" aria-hidden />
            {t('dashboard.mfaOn')}
          </div>
          <div className="mt-1.5 text-xs text-subtle">TOTP</div>
        </Card>
      </div>

      <Card className="relative overflow-hidden">
        <div aria-hidden className="pointer-events-none absolute -right-24 -top-24 size-72 rounded-full bg-accent/10 blur-3xl" />
        <h2 className="font-display text-xl font-semibold">{t('dashboard.welcome', { name: me?.user.displayName ?? '' })}</h2>
        <p className="mt-2 max-w-2xl text-sm text-ink-2">{t('dashboard.phase0')}</p>
        <h3 className="mt-6 text-[11px] uppercase tracking-[0.12em] text-subtle">{t('dashboard.next')}</h3>
        <ul className="mt-3 flex flex-wrap gap-2">
          {next.map((n) => (
            <li key={n.key} className="inline-flex items-center gap-2 rounded-full border border-line bg-surface-2/70 px-3 py-1.5 text-sm text-ink-2">
              <n.icon className="size-4 text-accent" aria-hidden />
              {t(`nav.${n.key}`)}
            </li>
          ))}
        </ul>
        <a href={ROADMAP} target="_blank" rel="noreferrer" className="mt-5 inline-flex items-center gap-1 text-sm text-accent hover:underline">
          {t('dashboard.roadmap')} <ArrowUpRight className="size-4" aria-hidden />
        </a>
      </Card>
    </>
  )
}
