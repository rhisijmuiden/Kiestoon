import { useTranslation } from 'react-i18next'
import { PhoneIncoming, PhoneMissed, PhoneOutgoing, Repeat } from 'lucide-react'
import { formatDuration, useApi } from '../components/kit'
import { ErrorMessage } from '../components/ui'
import type { Cdr } from '../lib/types'
import { CallButton, Title } from './Contacts'
import { useDesktop } from './data'

function when(iso: string, lang: string) {
  const d = new Date(iso)
  const today = new Date()
  const same = d.toDateString() === today.toDateString()
  return same ? d.toLocaleTimeString(lang, { hour: '2-digit', minute: '2-digit' }) : d.toLocaleDateString(lang, { day: 'numeric', month: 'short' }) + ' ' + d.toLocaleTimeString(lang, { hour: '2-digit', minute: '2-digit' })
}

export function History() {
  const { t, i18n } = useTranslation()
  const { me, targets } = useDesktop()
  const list = useApi<{ records: Cdr[]; total: number }>('/portal/history?limit=100&offset=0', 15000)
  const mine = me.data?.number
  const nameOf = (n: string) => targets.data?.find((e) => e.number === n)?.name ?? ''
  return (
    <div className="pb-3">
      <Title>{t('desktop.nav.history')}</Title>
      <ErrorMessage error={list.error} />
      {list.data?.records.length === 0 && <p className="px-4 py-8 text-center text-sm text-muted">{t('history.empty')}</p>}
      <ul className="px-2">
        {list.data?.records.map((r) => {
          const out = r.src === mine
          const other = out ? r.dst : r.src
          const missed = !out && r.disposition !== 'ANSWERED'
          const clid = r.clid.replace(/<.*>/, '').replace(/"/g, '').trim()
          const name = nameOf(other) || (!out && clid !== other ? clid : '')
          const Icon = r.direction === 'internal' && !missed ? Repeat : missed ? PhoneMissed : out ? PhoneOutgoing : PhoneIncoming
          return (
            <li key={r.id} className="flex items-center gap-2.5 rounded-[10px] px-1.5 py-1.5 hover:bg-surface">
              <span className={`inline-flex size-9 flex-none items-center justify-center rounded-full ${missed ? 'bg-danger/10 text-danger' : out ? 'bg-accent-2/10 text-accent-2' : 'bg-accent/10 text-accent'}`}>
                <Icon className="size-4" aria-label={missed ? t('desktop.missed') : out ? t('desktop.outgoing') : t('desktop.incoming')} />
              </span>
              <span className="min-w-0 flex-1">
                <span className={`block truncate text-[13.5px] font-medium ${missed ? 'text-danger' : 'text-ink'}`}>{name || other || t('reception.unknownCaller')}</span>
                <span className="block truncate text-[11.5px] text-muted">{name ? `${other} · ` : ''}{when(r.start, i18n.language)}{r.billsec > 0 ? ` · ${formatDuration(r.billsec)}` : ''}</span>
              </span>
              {other && <CallButton number={other} label={name || other} />}
            </li>
          )
        })}
      </ul>
    </div>
  )
}
