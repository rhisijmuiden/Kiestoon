import {
  Sparkles,
  Activity, Archive, Import, FileText, Gauge, Building2, Network, ServerCog, Sun, AudioLines, AudioWaveform, BookUser, Globe, Monitor, MonitorPlay, Plug, Printer, Clock, Disc3, GitBranch, Headset, History, KeyRound, LayoutGrid, ListOrdered, Mail, MonitorSmartphone,
  MessagesSquare, Phone, Presentation, ScrollText, Settings, Shield, ShieldAlert, Siren, Smartphone, TabletSmartphone, User, UserCog, UsersRound, Voicemail, Waypoints,
  type LucideIcon,
} from 'lucide-react'
import { roleRank, type Role } from '../lib/api'

/** min is the lowest role that sees the item (default admin). portal items need a linked extension. */
export type NavItem = { key: string; path: string; icon: LucideIcon; phase?: number; min?: Role; portal?: boolean }
export type NavGroup = { key?: string; items: NavItem[] }

export const nav: NavGroup[] = [
  { items: [{ key: 'dashboard', path: '/', icon: LayoutGrid, min: 'reception' }] },
  {
    key: 'workspace',
    items: [
      { key: 'myPhone', path: '/me', icon: Phone, min: 'user', portal: true },
      { key: 'voicemail', path: '/me/voicemail', icon: Voicemail, min: 'user', portal: true },
      { key: 'myQueues', path: '/me/queues', icon: Headset, min: 'user', portal: true },
      { key: 'colleagues', path: '/me/colleagues', icon: UsersRound, min: 'user', portal: true },
      { key: 'myCalls', path: '/me/calls', icon: History, min: 'user', portal: true },
      { key: 'devices', path: '/me/devices', icon: MonitorSmartphone, min: 'user', portal: true },
      { key: 'myOnCall', path: '/me/oncall', icon: Siren, min: 'user' },
      { key: 'phonebook', path: '/phonebook', icon: BookUser, min: 'user' },
    ],
  },
  {
    key: 'reception',
    items: [
      { key: 'switchboard', path: '/reception', icon: Activity, min: 'reception' },
      { key: 'wallboard', path: '/wallboard', icon: MonitorPlay, min: 'reception' },
      { key: 'fax', path: '/fax', icon: Printer, min: 'reception' },
      { key: 'history', path: '/history', icon: ScrollText, min: 'reception' },
      { key: 'recordings', path: '/recordings', icon: Disc3, min: 'reception' },
    ],
  },
  {
    key: 'telephony',
    items: [
      { key: 'extensions', path: '/extensions', icon: User },
      { key: 'ringGroups', path: '/ring-groups', icon: UsersRound },
      { key: 'queues', path: '/queues', icon: ListOrdered },
      { key: 'ivr', path: '/ivr', icon: GitBranch },
      { key: 'timeConditions', path: '/time-conditions', icon: Clock },
      { key: 'conferences', path: '/conferences', icon: Presentation },
      { key: 'onCall', path: '/on-call', icon: Siren },
      { key: 'prompts', path: '/prompts', icon: AudioLines },
      { key: 'trunks', path: '/trunks', icon: Waypoints },
      { key: 'teams', path: '/teams', icon: MessagesSquare },
      { key: 'quality', path: '/quality', icon: Gauge },
      { key: 'phones', path: '/phones', icon: Smartphone },
    ],
  },
  {
    key: 'multisite',
    items: [
      { key: 'sites', path: '/sites', icon: Building2 },
      { key: 'federation', path: '/federation', icon: Network },
      { key: 'sunRoutes', path: '/follow-the-sun', icon: Sun },
      { key: 'cluster', path: '/cluster', icon: ServerCog },
    ],
  },
  {
    key: 'system',
    items: [
      { key: 'users', path: '/users', icon: UserCog },
      { key: 'sso', path: '/sso', icon: KeyRound },
      { key: 'features', path: '/features', icon: ShieldAlert },
      { key: 'firewall', path: '/firewall', icon: Shield },
      { key: 'internet', path: '/internet', icon: Globe },
      { key: 'integrations', path: '/integrations', icon: Plug },
      { key: 'speech', path: '/speech', icon: AudioWaveform },
      { key: 'ai', path: '/ai', icon: Sparkles },
      { key: 'mobileApp', path: '/mobile-app', icon: TabletSmartphone },
      { key: 'windowsSoftphone', path: '/softphone', icon: Monitor },
      { key: 'backups', path: '/backups', icon: Archive },
      { key: 'import', path: '/import', icon: Import },
      { key: 'mail', path: '/mail', icon: Mail },
      { key: 'audit', path: '/audit', icon: ScrollText },
      { key: 'logs', path: '/logs', icon: FileText },
      { key: 'settings', path: '/settings', icon: Settings },
    ],
  },
]

export const allItems = nav.flatMap((g) => g.items)

export function visible(item: NavItem, role: Role | undefined, hasExtension: boolean) {
  return roleRank(role) >= roleRank(item.min ?? 'admin') && (!item.portal || hasExtension)
}
