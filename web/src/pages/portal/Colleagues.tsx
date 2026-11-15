import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Phone } from 'lucide-react'
import { useApi } from '../../components/kit'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../../components/ui'
import type { BlfEntry, PortalMe } from '../../lib/types'
import { useSoftphone } from '../../lib/softphone'
import { BlfGrid } from '../Reception'

/** Colleagues with their live status. A click calls them, with the softphone or the desk phone. */
export function Colleagues() {
  const { t } = useTranslation()
  const me = useApi<PortalMe>('/portal/me')
  const blf = useApi<BlfEntry[]>('/portal/colleagues', 3000)
  const phone = useSoftphone()
  const [q, setQ] = useState('')
  const [number, setNumber] = useState('')
  const [note, setNote] = useState('')
  const [error, setError] = useState<unknown>(null)
  async function call(n: string) {
    setError(null)
    setNote('')
    try {
      const via = await phone.callAny(n)
      if (via === 'deskphone') setNote(t('portal.deskphoneRinging', { number: n }))
    } catch (e) {
      setError(e)
    }
  }
  const list = (blf.data ?? []).filter((e) => !q || e.number.includes(q) || e.name.toLowerCase().includes(q.toLowerCase()))
  return (
    <>
      <PageHeader title={t('portal.colleagues')} subtitle={t('portal.colleaguesHint')} />
      <ErrorMessage error={error || blf.error} />
      {note && <Alert tone="ok">{note}</Alert>}
      <Card>
        <form className="flex flex-wrap items-end gap-3" onSubmit={(e) => { e.preventDefault(); if (number) call(number) }}>
          <div className="min-w-48 flex-1"><Input label={t('portal.dialNumber')} value={number} onChange={(e) => setNumber(e.target.value.replace(/[^0-9+*#]/g, ''))} inputMode="tel" className="font-mono" placeholder="0201234567" /></div>
          <Button type="submit" disabled={!number}><Phone className="size-4" aria-hidden /> {t('portal.call')}</Button>
        </form>
        <p className="mt-2 text-xs text-subtle">{phone.status === 'registered' ? t('portal.viaSoftphone') : t('portal.viaDeskphone')}</p>
      </Card>
      <Card>
        <div className="mb-4 max-w-xs"><Input label={t('common.search')} value={q} onChange={(e) => setQ(e.target.value)} /></div>
        <BlfGrid entries={list} onCall={call} self={me.data?.number} />
      </Card>
    </>
  )
}
