import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Delete, Phone } from 'lucide-react'
import { useSoftphone } from '../lib/softphone'
import { Title } from './Contacts'

const KEYS: [string, string][] = [['1', ''], ['2', 'ABC'], ['3', 'DEF'], ['4', 'GHI'], ['5', 'JKL'], ['6', 'MNO'], ['7', 'PQRS'], ['8', 'TUV'], ['9', 'WXYZ'], ['*', ''], ['0', '+'], ['#', '']]

export function Keypad() {
  const { t } = useTranslation()
  const phone = useSoftphone()
  const [n, setN] = useState('')
  const press = (k: string) => {
    if (phone.call?.state === 'active') phone.dtmf(k)
    else setN((v) => (v + k).slice(0, 32))
  }
  const call = () => {
    if (n && !phone.call) phone.dial(n)
  }
  return (
    <div className="flex flex-col items-center px-6 pb-6">
      <div className="self-stretch"><Title>{t('desktop.nav.keypad')}</Title></div>
      <form className="mb-4 flex w-full max-w-[280px] items-center gap-2" onSubmit={(e) => { e.preventDefault(); call() }}>
        <input value={n} onChange={(e) => setN(e.target.value.replace(/[^0-9+*#]/g, ''))} inputMode="tel" aria-label={t('portal.dialNumber')}
          placeholder={t('portal.dialNumber')} className="h-12 min-w-0 flex-1 bg-transparent text-center font-mono text-2xl text-ink outline-none placeholder:text-base placeholder:text-subtle" autoFocus />
        {n && <button type="button" onClick={() => setN(n.slice(0, -1))} aria-label={t('desktop.backspace')} className="inline-flex size-9 items-center justify-center rounded-lg text-muted hover:text-ink"><Delete className="size-5" aria-hidden /></button>}
      </form>
      <div className="grid w-full max-w-[280px] grid-cols-3 gap-2.5">
        {KEYS.map(([k, sub]) => (
          <button key={k} type="button" onClick={() => press(k)}
            className="flex h-14 flex-col items-center justify-center rounded-2xl border border-line bg-surface transition hover:border-accent/50 active:bg-accent/10">
            <span className="font-display text-xl text-ink">{k}</span>
            <span className="h-3 text-[9px] tracking-[0.15em] text-subtle">{sub}</span>
          </button>
        ))}
      </div>
      <button type="button" onClick={call} disabled={!n || !!phone.call || phone.status !== 'registered'} aria-label={t('portal.call')}
        className="mt-5 inline-flex h-14 w-14 items-center justify-center rounded-full bg-ok text-white shadow-lg transition hover:brightness-110 disabled:opacity-40">
        <Phone className="size-6" aria-hidden />
      </button>
      {phone.status !== 'registered' && <p className="mt-3 text-center text-xs text-muted">{t(`desktop.status.${phone.status}`)}</p>}
    </div>
  )
}
