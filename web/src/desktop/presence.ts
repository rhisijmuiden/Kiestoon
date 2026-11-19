import type { TransferTarget } from '../lib/types'

export type Presence = 'available' | 'busy' | 'ringing' | 'dnd' | 'offline'

/** The presence a colleague shows, from the BLF state of their extension. */
export function presenceOf(t: Pick<TransferTarget, 'state' | 'dnd'>): Presence {
  switch (t.state) {
    case 'inuse': case 'busy': case 'onhold': return 'busy'
    case 'ringing': case 'ringinuse': return 'ringing'
    case 'unavailable': case 'unknown': case undefined: case '': return 'offline'
  }
  return t.dnd ? 'dnd' : 'available'
}

export const presenceDot: Record<Presence, string> = {
  available: 'bg-ok', busy: 'bg-danger', ringing: 'bg-warn animate-pulse', dnd: 'bg-danger', offline: 'bg-subtle',
}
export const presenceText: Record<Presence, string> = {
  available: 'text-ok', busy: 'text-danger', ringing: 'text-warn', dnd: 'text-danger', offline: 'text-subtle',
}

export function initials(name: string, fallback = '?') {
  const p = name.replace(/"/g, '').trim().split(/\s+/).filter(Boolean)
  if (!p.length) return fallback
  return (p.length === 1 ? p[0].slice(0, 2) : p[0][0] + p[p.length - 1][0]).toUpperCase()
}

// Avatar colours: a gradient picked from the name, so a person keeps their colour.
const gradients = [
  'from-accent to-accent-2', 'from-[#F472B6] to-accent-2', 'from-[#FBBF24] to-[#F87171]', 'from-ok to-accent', 'from-accent-2 to-[#F472B6]',
]
export function avatarClass(name: string) {
  let h = 0
  for (const c of name) h = (h * 31 + c.charCodeAt(0)) >>> 0
  return gradients[h % gradients.length]
}

const FAV_KEY = 'kiestoon.desktop.favorites'
export function loadFavorites(): string[] {
  try {
    return JSON.parse(localStorage.getItem(FAV_KEY) ?? '[]') as string[]
  } catch {
    return []
  }
}
export function saveFavorites(f: string[]) {
  try {
    localStorage.setItem(FAV_KEY, JSON.stringify(f))
  } catch {
    /* ignore */
  }
}
