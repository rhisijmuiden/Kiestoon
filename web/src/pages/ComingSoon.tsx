import { useTranslation } from 'react-i18next'
import { ArrowUpRight } from 'lucide-react'
import { Card, PageHeader } from '../components/ui'
import type { NavItem } from '../components/nav'

export function ComingSoon({ item }: { item: NavItem }) {
  const { t } = useTranslation()
  return (
    <>
      <PageHeader title={t(`nav.${item.key}`)} />
      <Card className="flex flex-col items-center gap-4 py-16 text-center">
        <div className="flex size-16 items-center justify-center rounded-2xl border border-accent/40 bg-accent/10 shadow-[0_0_40px_-12px_rgb(34_211_238_/_0.7)]">
          <item.icon className="size-8 text-accent" strokeWidth={1.6} aria-hidden />
        </div>
        <h2 className="font-display text-xl font-semibold">{t('soon.title', { phase: item.phase })}</h2>
        <p className="max-w-md text-sm text-muted">{t('soon.text', { phase: item.phase })}</p>
        <a href="https://github.com/rhisijmuiden/kiestoon/blob/main/ROADMAP.md" target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-sm text-accent hover:underline">
          {t('soon.roadmap')} <ArrowUpRight className="size-4" aria-hidden />
        </a>
      </Card>
    </>
  )
}
