import { useLocation } from 'react-router'
import { useTranslation } from 'react-i18next'
import { CircleHelp } from 'lucide-react'

/** The manual on GitHub Pages. Dutch pages live under nl/. */
export const DOCS_URL = 'https://rhisijmuiden.github.io/Kiestoon/'

type Topic = { page: string; en?: string; nl?: string }

/** Portal path -> manual page (and section, whose id differs per language). */
const topics: Record<string, Topic> = {
  '/': { page: '' },
  '/me': { page: 'portal', en: 'my-phone', nl: 'mijn-toestel' },
  '/me/voicemail': { page: 'voicemail' },
  '/me/queues': { page: 'portal', en: 'queues-groups', nl: 'wachtrijen-groepen' },
  '/me/colleagues': { page: 'portal', en: 'colleagues-and-drag-and-drop-transfer', nl: 'collegas-en-doorverbinden-met-slepen' },
  '/me/calls': { page: 'portal', en: 'my-calls', nl: 'mijn-gesprekken' },
  '/me/devices': { page: 'portal', en: 'softphone-devices', nl: 'softphone-toestellen' },
  '/phonebook': { page: 'phonebook' },
  '/reception': { page: 'switchboard' },
  '/wallboard': { page: 'wallboard' },
  '/wallboard/settings': { page: 'wallboard', en: 'tv-links', nl: 'tv-links' },
  '/fax': { page: 'fax' },
  '/history': { page: 'history' },
  // A call journey (?call=...) opened from the call history or My calls.
  '/history#call': { page: 'call-journey' },
  '/me/calls#call': { page: 'call-journey' },
  '/recordings': { page: 'history', en: 'recordings', nl: 'opnames' },
  '/extensions': { page: 'extensions' },
  '/ring-groups': { page: 'ring-groups' },
  '/queues': { page: 'queues' },
  '/ivr': { page: 'ivr' },
  '/time-conditions': { page: 'time-conditions' },
  '/conferences': { page: 'conferences' },
  '/prompts': { page: 'prompts' },
  '/trunks': { page: 'trunks' },
  '/quality': { page: 'call-quality' },
  '/quality/settings': { page: 'call-quality', en: 'alerts', nl: 'meldingen' },
  '/phones': { page: 'phones' },
  '/sites': { page: 'edge' },
  '/federation': { page: 'multi-site' },
  '/follow-the-sun': { page: 'multi-site', en: 'follow-the-sun', nl: 'follow-the-sun' },
  '/cluster': { page: 'high-availability' },
  '/users': { page: 'users' },
  '/sso': { page: 'users', en: 'single-sign-on', nl: 'single-sign-on' },
  '/features': { page: 'firewall', en: 'fraud-protection', nl: 'fraudebescherming' },
  '/firewall': { page: 'firewall' },
  '/internet': { page: 'internet' },
  '/integrations': { page: 'api' },
  '/speech': { page: 'prompts', en: 'speech', nl: 'spraak' },
  '/ai': { page: 'ai' },
  '/mobile-app': { page: 'mobile-app' },
  '/softphone': { page: 'softphone' },
  '/backups': { page: 'backups' },
  '/mail': { page: 'mail' },
  '/audit': { page: 'users', en: 'audit-log', nl: 'auditlog' },
  '/settings': { page: 'settings' },
  '/account': { page: 'users', en: 'passkeys', nl: 'passkeys' },
}

/** The manual page for a portal path, in the given language (en or nl). */
export function helpUrl(pathname: string, language: string): string {
  const nl = language.startsWith('nl')
  const path = pathname.replace(/\/+$/, '') || '/'
  const topic = topics[path] ?? topics['/']
  const section = nl ? topic.nl : topic.en
  return `${DOCS_URL}${nl ? 'nl/' : ''}${topic.page ? `${topic.page}/` : ''}${section ? `#${section}` : ''}`
}

/** A link into the manual, moved to the Dutch pages for Dutch users. Other links stay as they are. */
export function localDocsUrl(url: string, language: string): string {
  return language.startsWith('nl') && url.startsWith(DOCS_URL) && !url.startsWith(`${DOCS_URL}nl/`) ? `${DOCS_URL}nl/${url.slice(DOCS_URL.length)}` : url
}

/** Opens the manual page that belongs to the current portal page, in the user's language. */
export function HelpButton({ className = '' }: { className?: string }) {
  const { t, i18n } = useTranslation()
  const { pathname, search } = useLocation()
  const path = new URLSearchParams(search).has('call') ? `${pathname.replace(/\/+$/, '')}#call` : pathname
  return (
    <a
      href={helpUrl(path, i18n.language)}
      target="_blank"
      rel="noopener noreferrer"
      title={t('nav.helpTitle')}
      aria-label={t('nav.helpTitle')}
      className={`inline-flex min-h-11 items-center gap-2 rounded-xl border border-line bg-surface/90 px-3 text-sm text-ink-2 transition hover:border-accent/45 hover:text-ink ${className}`}
    >
      <CircleHelp className="size-[18px]" aria-hidden />
      <span>{t('nav.help')}</span>
    </a>
  )
}
