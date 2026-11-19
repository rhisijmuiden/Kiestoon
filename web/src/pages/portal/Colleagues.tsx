import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Phone } from 'lucide-react'
import { useApi } from '../../components/kit'
import { ActiveCalls, TransferNotice, TransferTargets } from '../../components/Transfer'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../../components/ui'
import type { PortalMe } from '../../lib/types'
import { useSoftphone } from '../../lib/softphone'
import { TransferProvider } from '../../lib/transfer'

/**
 * Colleagues, ring groups and queues with their live status. A click calls them, with the
 * softphone or the desk phone; dragging an active call onto them transfers it.
 */
export function Colleagues() {
  const { t } = useTranslation()
  const me = useApi<PortalMe>('/portal/me')
  const phone = useSoftphone()
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
  return (
    <TransferProvider>
      <PageHeader title={t('portal.colleagues')} subtitle={t('portal.colleaguesHint')} />
      <ErrorMessage error={error} />
      {note && <Alert tone="ok">{note}</Alert>}
      <TransferNotice />
      <ActiveCalls hint />
      <Card>
        <form className="flex flex-wrap items-end gap-3" onSubmit={(e) => { e.preventDefault(); if (number) call(number) }}>
          <div className="min-w-48 flex-1"><Input label={t('portal.dialNumber')} value={number} onChange={(e) => setNumber(e.target.value.replace(/[^0-9+*#]/g, ''))} inputMode="tel" className="font-mono" placeholder="0201234567" /></div>
          <Button type="submit" disabled={!number}><Phone className="size-4" aria-hidden /> {t('portal.call')}</Button>
        </form>
        <p className="mt-2 text-xs text-subtle">{phone.status === 'registered' ? t('portal.viaSoftphone') : t('portal.viaDeskphone')}</p>
      </Card>
      <TransferTargets path="/portal/targets" self={me.data?.number} onCall={call} />
    </TransferProvider>
  )
}
