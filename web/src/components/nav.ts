import {
  Activity, Archive, Clock, GitBranch, KeyRound, LayoutGrid, ListOrdered, Mail, ScrollText, Settings, Shield,
  Smartphone, User, UsersRound, Waypoints, type LucideIcon,
} from 'lucide-react'

export type NavItem = { key: string; path: string; icon: LucideIcon; phase?: number; adminOnly?: boolean }
export type NavGroup = { key?: string; items: NavItem[] }

/** The full admin menu. Items with a phase open a "coming in phase N" page until they are built. */
export const nav: NavGroup[] = [
  { items: [{ key: 'dashboard', path: '/', icon: LayoutGrid }] },
  {
    key: 'telephony',
    items: [
      { key: 'extensions', path: '/extensions', icon: User, phase: 1 },
      { key: 'ringGroups', path: '/ring-groups', icon: UsersRound, phase: 2 },
      { key: 'queues', path: '/queues', icon: ListOrdered, phase: 2 },
      { key: 'ivr', path: '/ivr', icon: GitBranch, phase: 2 },
      { key: 'timeConditions', path: '/time-conditions', icon: Clock, phase: 2 },
      { key: 'trunks', path: '/trunks', icon: Waypoints, phase: 1 },
      { key: 'phones', path: '/phones', icon: Smartphone, phase: 1 },
      { key: 'history', path: '/history', icon: Activity, phase: 1 },
    ],
  },
  {
    key: 'system',
    items: [
      { key: 'firewall', path: '/firewall', icon: Shield, phase: 1, adminOnly: true },
      { key: 'backups', path: '/backups', icon: Archive, phase: 1, adminOnly: true },
      { key: 'mail', path: '/mail', icon: Mail, phase: 1, adminOnly: true },
      { key: 'sso', path: '/sso', icon: KeyRound, phase: 2, adminOnly: true },
      { key: 'audit', path: '/audit', icon: ScrollText, adminOnly: true },
      { key: 'settings', path: '/settings', icon: Settings, phase: 1, adminOnly: true },
    ],
  },
]

export const allItems = nav.flatMap((g) => g.items)

