import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ListOrdered, Phone, PhoneForwarded, Search, Star, Users } from 'lucide-react'
import { useApi } from '../components/kit'
import { ErrorMessage } from '../components/ui'
import { useSoftphone } from '../lib/softphone'
import { useTransfer } from '../lib/transfer'
import type { Contact, TransferTarget } from '../lib/types'
import { Avatar } from './Avatar'
import { useDesktop } from './data'
import { loadFavorites, presenceOf, presenceText, saveFavorites, type Presence } from './presence'

type Tab = 'colleagues' | 'phonebook' | 'favorites'

export function Title({ children, sub }: { children: React.ReactNode; sub?: string }) {
  return (
    <div className="px-3.5 pb-2.5 pt-3.5">
      <h1 className="font-display text-[17px] font-semibold">{children}</h1>
      {sub && <p className="mt-0.5 text-xs text-muted">{sub}</p>}
    </div>
  )
}

export function SearchBox({ value, onChange, placeholder }: { value: string; onChange: (v: string) => void; placeholder: string }) {
  return (
    <label className="mx-3.5 mb-2.5 flex h-9 items-center gap-2 rounded-[10px] border border-line bg-field px-2.5 focus-within:border-accent/60">
      <Search className="size-4 text-subtle" aria-hidden />
      <input value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} aria-label={placeholder}
        className="min-w-0 flex-1 bg-transparent text-[13px] text-ink outline-none placeholder:text-subtle" />
    </label>
  )
}

export function CallButton({ number, label }: { number: string; label?: string }) {
  const { t } = useTranslation()
  const phone = useSoftphone()
  return (
    <button type="button" onClick={(e) => { e.stopPropagation(); phone.dial(number.replace(/[^0-9+*#]/g, '')) }} disabled={!number || !!phone.call || phone.status !== 'registered'}
      title={t('portal.callNumber', { number: label ?? number })} aria-label={t('portal.callNumber', { number: label ?? number })}
      className="inline-flex size-[30px] flex-none items-center justify-center rounded-[9px] border border-line bg-surface text-accent transition hover:border-accent/60 disabled:opacity-40">
      <Phone className="size-[15px]" aria-hidden />
    </button>
  )
}

function Label({ children }: { children: React.ReactNode }) {
  return <div className="mx-2 mb-1 mt-2.5 text-[10.5px] uppercase tracking-[0.08em] text-subtle">{children}</div>
}

/** One colleague, group or queue. A drop target while a call is dragged or picked. */
function TargetRow({ e, fav, onFav }: { e: TransferTarget; fav: boolean; onFav: () => void }) {
  const { t } = useTranslation()
  const x = useTransfer()
  const pr: Presence | undefined = e.kind === 'extension' ? presenceOf(e) : undefined
  const over = x.dragging && x.hover === e.number
  const armed = !!x.active && !x.dragging
  const status = e.kind === 'extension' ? t(`desktop.presence.${pr}`)
    : e.kind === 'queue' && e.waiting ? t('transfer.waiting', { count: e.waiting }) : t('transfer.members', { count: e.members ?? 0 })
  return (
    <div data-xfer-target={e.number} data-xfer-name={e.name} onClick={armed ? () => x.drop(e.number, e.name) : undefined}
      className={`group flex items-center gap-2.5 rounded-[10px] border px-1.5 py-1.5 transition ${over ? 'scale-[1.02] border-accent bg-accent/15'
        : armed ? 'cursor-pointer border-dashed border-accent/50 hover:bg-accent/10' : 'border-transparent hover:bg-surface'}`}>
      {e.kind === 'extension' ? <Avatar name={e.name} presence={pr} />
        : <span className="inline-flex size-9 flex-none items-center justify-center rounded-[10px] bg-accent-2/15 text-accent-2">{e.kind === 'queue' ? <ListOrdered className="size-4" aria-hidden /> : <Users className="size-4" aria-hidden />}</span>}
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[13.5px] font-medium text-ink">{e.name || e.number}</span>
        <span className="block truncate text-[11.5px] text-muted">{e.number} · <span className={pr ? presenceText[pr] : ''}>{status}</span></span>
      </span>
      {over ? <PhoneForwarded className="size-4 text-accent" aria-hidden /> : (
        <>
          {e.kind === 'extension' && (
            <button type="button" onClick={(ev) => { ev.stopPropagation(); onFav() }} aria-pressed={fav} title={t('desktop.favorite')} aria-label={t('desktop.favorite')}
              className={`inline-flex size-7 items-center justify-center rounded-lg ${fav ? 'text-warn' : 'text-subtle opacity-0 group-hover:opacity-100 focus:opacity-100'}`}>
              <Star className="size-4" fill={fav ? 'currentColor' : 'none'} aria-hidden />
            </button>
          )}
          <CallButton number={e.number} label={e.name || e.number} />
        </>
      )}
    </div>
  )
}

export function Contacts() {
  const { t } = useTranslation()
  const { targets, me } = useDesktop()
  const [tab, setTab] = useState<Tab>('colleagues')
  const [q, setQ] = useState('')
  const [favs, setFavs] = useState(loadFavorites)
  const book = useApi<Contact[]>(tab === 'colleagues' ? null : '/phonebook')
  const toggleFav = (n: string) => {
    const next = favs.includes(n) ? favs.filter((f) => f !== n) : [...favs, n]
    setFavs(next)
    saveFavorites(next)
  }
  const match = (...s: string[]) => !q || s.some((v) => v.toLowerCase().includes(q.toLowerCase()))
  const all = useMemo(() => (targets.data ?? []).filter((e) => !(e.kind === 'extension' && e.number === me.data?.number)), [targets.data, me.data?.number])
  const people = all.filter((e) => e.kind === 'extension' && match(e.name, e.number))
  const online = people.filter((e) => presenceOf(e) !== 'offline')
  const offline = people.filter((e) => presenceOf(e) === 'offline')
  const groups = all.filter((e) => e.kind !== 'extension' && match(e.name, e.number))
  const contacts = (book.data ?? []).filter((c) => match(c.name, c.company, c.work, c.mobile, c.other))

  const tabs: Tab[] = ['colleagues', 'phonebook', 'favorites']
  return (
    <div className="pb-3">
      <Title>{t('desktop.nav.contacts')}</Title>
      <SearchBox value={q} onChange={setQ} placeholder={t('desktop.searchContacts')} />
      <div role="tablist" className="mx-3.5 mb-1.5 flex gap-1.5">
        {tabs.map((k) => (
          <button key={k} role="tab" aria-selected={tab === k} type="button" onClick={() => setTab(k)}
            className={`rounded-lg border px-2.5 py-1 text-xs transition ${tab === k ? 'border-line bg-surface text-ink' : 'border-transparent text-muted hover:text-ink'}`}>
            {t(`desktop.tabs.${k}`)}
          </button>
        ))}
      </div>
      <ErrorMessage error={targets.error || book.error} />
      <div className="px-2">
        {tab === 'colleagues' && (
          <>
            {online.length > 0 && <Label>{t('desktop.online', { count: online.length })}</Label>}
            {online.map((e) => <TargetRow key={e.number} e={e} fav={favs.includes(e.number)} onFav={() => toggleFav(e.number)} />)}
            {groups.length > 0 && <Label>{t('desktop.groups')}</Label>}
            {groups.map((e) => <TargetRow key={e.kind + e.number} e={e} fav={false} onFav={() => undefined} />)}
            {offline.length > 0 && <Label>{t('desktop.offline', { count: offline.length })}</Label>}
            {offline.map((e) => <TargetRow key={e.number} e={e} fav={favs.includes(e.number)} onFav={() => toggleFav(e.number)} />)}
            {people.length + groups.length === 0 && !targets.loading && <p className="px-2 py-8 text-center text-sm text-muted">{t('desktop.nothingFound')}</p>}
          </>
        )}
        {tab === 'favorites' && (
          <>
            {people.filter((e) => favs.includes(e.number)).map((e) => <TargetRow key={e.number} e={e} fav onFav={() => toggleFav(e.number)} />)}
            {contacts.filter((c) => favs.includes(`pb:${c.id}`)).map((c) => <ContactRow key={c.id} c={c} fav onFav={() => toggleFav(`pb:${c.id}`)} />)}
            {favs.length === 0 && <p className="px-4 py-8 text-center text-sm text-muted">{t('desktop.noFavorites')}</p>}
          </>
        )}
        {tab === 'phonebook' && (
          <>
            {contacts.map((c) => <ContactRow key={c.id} c={c} fav={favs.includes(`pb:${c.id}`)} onFav={() => toggleFav(`pb:${c.id}`)} />)}
            {contacts.length === 0 && !book.loading && <p className="px-2 py-8 text-center text-sm text-muted">{t('desktop.nothingFound')}</p>}
          </>
        )}
      </div>
    </div>
  )
}

/** A phone book entry with a call button per number. External numbers are drop targets too. */
function ContactRow({ c, fav, onFav }: { c: Contact; fav: boolean; onFav: () => void }) {
  const { t } = useTranslation()
  const x = useTransfer()
  const numbers = [c.work, c.mobile, c.other].filter(Boolean)
  const first = numbers[0] ?? ''
  const over = x.dragging && x.hover === first
  return (
    <div data-xfer-target={first || undefined} data-xfer-name={c.name}
      className={`group flex items-center gap-2.5 rounded-[10px] border px-1.5 py-1.5 ${over ? 'border-accent bg-accent/15' : 'border-transparent hover:bg-surface'}`}>
      <Avatar name={c.name || c.company} plain />
      <span className="min-w-0 flex-1">
        <span className="block truncate text-[13.5px] font-medium text-ink">{c.name || c.company}</span>
        <span className="block truncate text-[11.5px] text-muted">{[c.name && c.company, ...numbers].filter(Boolean).join(' · ')}</span>
      </span>
      <button type="button" onClick={onFav} aria-pressed={fav} title={t('desktop.favorite')} aria-label={t('desktop.favorite')}
        className={`inline-flex size-7 items-center justify-center rounded-lg ${fav ? 'text-warn' : 'text-subtle opacity-0 group-hover:opacity-100 focus:opacity-100'}`}>
        <Star className="size-4" fill={fav ? 'currentColor' : 'none'} aria-hidden />
      </button>
      {numbers.slice(0, 2).map((n) => <CallButton key={n} number={n} label={`${c.name || c.company} (${n})`} />)}
    </div>
  )
}
