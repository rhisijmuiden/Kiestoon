import { useTranslation } from 'react-i18next'
import { CopyField } from './kit'
import { Alert } from './ui'
import type { WelcomeResult } from '../lib/types'

/** Shows whether the welcome e-mail went out; when it did not, the admin can copy the link. */
export function WelcomeNotice({ result }: { result: WelcomeResult }) {
  const { t } = useTranslation()
  if (result.sent) {
    return <Alert tone="ok">{t(result.created ? 'welcome.sentCreated' : 'welcome.sent', { email: result.email, username: result.username })}</Alert>
  }
  return (
    <div className="flex flex-col gap-3">
      <Alert tone="warn">{t(`welcome.${result.error ?? 'mail_failed'}`, { defaultValue: t('welcome.mail_failed'), email: result.email, username: result.username })}</Alert>
      {result.link && <CopyField label={t('welcome.link')} value={result.link} secret />}
    </div>
  )
}
