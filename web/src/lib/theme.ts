import { useSyncExternalStore } from 'react'

/** The user's choice. "system" follows the operating system's light/dark setting. */
export type ThemePref = 'system' | 'dark' | 'light'
export type Theme = 'dark' | 'light'

export const themePrefs: ThemePref[] = ['system', 'light', 'dark']

const KEY = 'kiestoon.theme'
const media = typeof window !== 'undefined' && window.matchMedia ? window.matchMedia('(prefers-color-scheme: light)') : null
const listeners = new Set<() => void>()

export function isThemePref(v: unknown): v is ThemePref {
  return v === 'system' || v === 'dark' || v === 'light'
}

function readPref(): ThemePref {
  try {
    const saved = localStorage.getItem(KEY)
    if (isThemePref(saved)) return saved
  } catch {
    /* storage blocked */
  }
  return 'system'
}

let pref: ThemePref = readPref()

export function resolveTheme(p: ThemePref): Theme {
  if (p === 'system') return media?.matches ? 'light' : 'dark'
  return p
}

function apply() {
  const theme = resolveTheme(pref)
  document.documentElement.dataset.theme = theme
  document.querySelector('meta[name="color-scheme"]')?.setAttribute('content', theme)
  listeners.forEach((l) => l())
}

/** Changes the theme on this device. The caller saves it on the account when signed in. */
export function setThemePref(p: ThemePref) {
  pref = p
  try {
    localStorage.setItem(KEY, p)
  } catch {
    /* ignore */
  }
  apply()
}

export function getThemePref(): ThemePref {
  return pref
}

media?.addEventListener('change', () => {
  if (pref === 'system') apply()
})
apply()

function subscribe(l: () => void) {
  listeners.add(l)
  return () => listeners.delete(l)
}

/** The current choice and the theme it resolves to; re-renders when either changes. */
export function useTheme(): { pref: ThemePref; theme: Theme } {
  const p = useSyncExternalStore(subscribe, () => pref)
  const theme = useSyncExternalStore(subscribe, () => resolveTheme(pref))
  return { pref: p, theme }
}
