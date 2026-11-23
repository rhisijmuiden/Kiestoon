import { useTranslation } from 'react-i18next'
import { Select, useApi } from './kit'
import type { PbxSettings } from '../lib/types'

export type Lang = 'en' | 'nl'
export const langName: Record<Lang, string> = { en: 'English', nl: 'Nederlands' }

/** The caller language of the phone system (Settings > Caller language), or null while loading. */
export function useCallerLanguage(): Lang | null {
  const st = useApi<PbxSettings>('/pbx/settings')
  return st.data ? st.data.language : null
}

/**
 * The language callers hear in one queue or IVR: empty follows the caller language of the
 * phone system, en or nl sets it apart.
 */
export function CallerLanguageSelect({ value, onChange, compact }: { value: string | undefined; onChange: (v: '' | Lang) => void; compact?: boolean }) {
  const { t } = useTranslation()
  const sys = useCallerLanguage()
  return (
    <Select name="language" label={t('callerLanguage.override')} value={value ?? ''} onChange={(e) => onChange(e.target.value as '' | Lang)}
      hint={compact ? undefined : t('callerLanguage.overrideHint')}>
      <option value="">{sys ? t('callerLanguage.follow', { language: langName[sys] }) : t('callerLanguage.followPlain')}</option>
      <option value="en">{langName.en}</option>
      <option value="nl">{langName.nl}</option>
    </Select>
  )
}
