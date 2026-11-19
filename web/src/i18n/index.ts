import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import en from './en.json'
import nl from './nl.json'

export const languages = [
  { code: 'en', label: 'English' },
  { code: 'nl', label: 'Nederlands' },
] as const

function initialLanguage(): string {
  try {
    const saved = localStorage.getItem('kiestoon.lang')
    if (saved === 'en' || saved === 'nl') return saved
  } catch {
    /* storage blocked: fall through */
  }
  return 'en'
}

i18n.use(initReactI18next).init({
  resources: { en: { translation: en }, nl: { translation: nl } },
  lng: initialLanguage(),
  fallbackLng: 'en',
  interpolation: { escapeValue: false },
})

i18n.on('languageChanged', (lng) => {
  document.documentElement.lang = lng
  try {
    localStorage.setItem('kiestoon.lang', lng)
  } catch {
    /* ignore */
  }
})
document.documentElement.lang = i18n.language

export default i18n
