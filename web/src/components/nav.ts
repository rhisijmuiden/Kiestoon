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
      { key: 'extensions', path: '/extensions', icon: User },
      { key: 'ringGroups', path: '/ring-groups', icon: UsersRound, phase: 2 },
      { key: 'queues', path: '/queues', icon: ListOrdered, phase: 2 },
      { key: 'ivr', path: '/ivr', icon: GitBranch, phase: 2 },
      { key: 'timeConditions', path: '/time-conditions', icon: Clock, phase: 2 },
      { key: 'trunks', path: '/trunks', icon: Waypoints },
      { key: 'phones', path: '/phones', icon: Smartphone },
      { key: 'history', path: '/history', icon: Activity },
    ],
  },
  {
    key: 'system',
    items: [
      { key: 'firewall', path: '/firewall', icon: Shield, adminOnly: true },
      { key: 'backups', path: '/backups', icon: Archive, adminOnly: true },
      { key: 'mail', path: '/mail', icon: Mail, adminOnly: true },
      { key: 'sso', path: '/sso', icon: KeyRound, phase: 2, adminOnly: true },
      { key: 'audit', path: '/audit', icon: ScrollText, adminOnly: true },
      { key: 'settings', path: '/settings', icon: Settings, adminOnly: true },
    ],
  },
]

export const allItems = nav.flatMap((g) => g.items)

