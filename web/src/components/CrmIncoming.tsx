import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ExternalLink } from 'lucide-react'
import { Card } from './ui'
import { SectionTitle } from './kit'
import { crmColors, crmNames, useCrmStatus, type CrmEvent } from '../lib/crm'

/** Switchboard: every call from outside that rings now, with who it is according to the CRM. */
export function CrmIncoming() {
  const { t } = useTranslation()
  const status = useCrmStatus()
  const [calls, setCalls] = useState<CrmEvent[]>([])
  useEffect(() => {
    if (!status?.active) return
    const es = new EventSource('/api/portal/crm/events?scope=all')
    es.onmessage = (m) => {
      let ev: CrmEvent
      try { ev = JSON.parse(m.data) } catch { return }
      setCalls((l) => {
        const rest = l.filter((c) => c.callId !== ev.callId)
        if (ev.type === 'ring') return [{ ...(l.find((c) => c.callId === ev.callId) ?? {}), ...ev }, ...rest].slice(0, 8)
        if (ev.type === 'answered' || ev.type === 'ended') return rest
        return l
      })
    }
    return () => es.close()
  }, [status?.active])
  if (!status?.active || calls.length === 0) return null
  return (
    <Card>
      <SectionTitle>{t('crm.incoming')}</SectionTitle>
      <ul className="flex flex-col gap-2">
        {calls.map((c) => {
          const ct = c.result?.contacts?.[0]
          return (
            <li key={c.callId} className="flex flex-wrap items-center gap-3 rounded-xl border border-accent/35 bg-accent/[0.05] px-3 py-2.5">
              <span className="relative inline-flex size-2"><span className="absolute inline-flex size-full animate-ping rounded-full bg-accent opacity-60 motion-reduce:animate-none" /><span className="relative inline-flex size-2 rounded-full bg-accent" /></span>
              <span className="min-w-0 flex-1">
                <b className="text-ink">{ct?.name || c.name || t('crm.pop.unknown')}</b>
                <span className="text-sm text-muted">{[ct?.company && ct.company !== ct.name ? ct.company : '', c.number].filter(Boolean).map((x) => ` · ${x}`).join('')}</span>
              </span>
              {ct && <span className="inline-flex items-center gap-1.5 text-xs text-subtle"><span className="inline-block size-2.5 rounded-[3px]" style={{ background: crmColors[ct.crm] }} aria-hidden />{crmNames[ct.crm]}</span>}
              {ct?.url && <a href={ct.url} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-1 text-xs text-accent-ink hover:underline">{t('crm.open')}<ExternalLink className="size-3" aria-hidden /></a>}
            </li>
          )
        })}
      </ul>
    </Card>
  )
}
