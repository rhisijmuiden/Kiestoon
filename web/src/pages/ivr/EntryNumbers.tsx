import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Hash, Plus, X } from 'lucide-react'
import { Select } from '../../components/kit'
import { useNumbers } from '../../components/pbx'
import { Button, ErrorMessage, Input } from '../../components/ui'
import { api } from '../../lib/api'
import { formatNumber } from '../../lib/numbers'
import type { InboundRoute, IVR } from '../../lib/types'

const OTHER = '__other'

/**
 * The phone numbers that ring this menu. Adding one creates (or moves) the inbound route of that
 * number to this menu right away, the same route you see under Trunks & routes › Inbound.
 */
export function EntryNumbers({ ivr, inbound, onChanged }: { ivr: IVR; inbound: InboundRoute[]; onChanged: () => void }) {
  const { t } = useTranslation()
  const [reload, setReload] = useState(0)
  const pool = useNumbers(reload)
  const [pick, setPick] = useState('')
  const [other, setOther] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)

  if (!ivr.id) return <p className="text-xs text-subtle">{t('ivr.numbers.saveFirst')}</p>

  const mine = inbound.filter((r) => r.destinationType === 'ivr' && r.destination === String(ivr.id))
  const free = (pool ?? []).filter((n) => !n.routeId)
  const elsewhere = (pool ?? []).filter((n) => n.routeId && !mine.some((r) => r.id === n.routeId))
  const did = pick === OTHER ? other.trim() : pick

  async function run(fn: () => Promise<unknown>) {
    setBusy(true)
    setError(null)
    try {
      await fn()
      setPick('')
      setOther('')
      setReload((x) => x + 1)
      onChanged()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  function add() {
    if (!did) return
    const existing = inbound.find((r) => r.did === did)
    if (existing) {
      if (!window.confirm(t('ivr.numbers.moveConfirm', { number: did, route: existing.name }))) return
      run(() => api('PUT', `/pbx/inbound/${existing.id}`, { ...existing, destinationType: 'ivr', destination: String(ivr.id) }))
      return
    }
    run(() => api('POST', '/pbx/inbound', { id: 0, name: `${ivr.name || t('ivr.untitled')} ${did}`.slice(0, 60), did, destinationType: 'ivr', destination: String(ivr.id), cidPrefix: '', enabled: true }))
  }
  function unlink(r: InboundRoute) {
    if (!window.confirm(t('ivr.numbers.unlinkConfirm', { number: r.did }))) return
    run(() => api('DELETE', `/pbx/inbound/${r.id}`))
  }

  return (
    <div className="flex flex-col gap-3">
      {mine.length ? (
        <ul className="flex flex-col gap-1.5">
          {mine.map((r) => {
            const info = pool?.find((n) => n.number === r.did)
            return (
              <li key={r.id} className="flex items-center gap-2.5 rounded-xl border border-accent/35 bg-accent/[0.06] px-3 py-2">
                <Hash className="size-4 flex-none text-accent" aria-hidden />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-mono text-sm text-ink">{formatNumber(r.did)}</span>
                  <span className="block truncate text-[11px] text-subtle">{info?.trunkName ?? r.name}{!r.enabled && ` · ${t('ivr.numbers.disabled')}`}</span>
                </span>
                <button type="button" onClick={() => unlink(r)} disabled={busy} aria-label={t('ivr.numbers.unlink', { number: r.did })} title={t('ivr.numbers.unlink', { number: r.did })}
                  className="inline-flex size-8 items-center justify-center rounded-lg text-muted hover:bg-danger/10 hover:text-danger"><X className="size-4" aria-hidden /></button>
              </li>
            )
          })}
        </ul>
      ) : <p className="text-xs text-muted">{t('ivr.numbers.none')}</p>}

      <div className="flex items-end gap-2">
        <div className="min-w-0 flex-1">
          <Select name="did" label={t('ivr.numbers.add')} value={pick} onChange={(e) => setPick(e.target.value)}>
            <option value="">{pool === null ? t('app.loading') : free.length ? t('ivr.numbers.choose') : t('ivr.numbers.noFree')}</option>
            {free.length > 0 && (
              <optgroup label={t('ivr.numbers.free')}>
                {free.map((n) => <option key={n.number} value={n.number}>{formatNumber(n.number)} · {n.trunkName}</option>)}
              </optgroup>
            )}
            {elsewhere.length > 0 && (
              <optgroup label={t('ivr.numbers.inUse')}>
                {elsewhere.map((n) => <option key={n.number} value={n.number}>{formatNumber(n.number)} → {n.routeName}</option>)}
              </optgroup>
            )}
            <option value={OTHER}>{t('ivr.numbers.other')}</option>
          </Select>
        </div>
        {pick !== OTHER && <Button busy={busy} disabled={!did} onClick={add}><Plus className="size-4" aria-hidden />{t('common.add')}</Button>}
      </div>
      {pick === OTHER && (
        <div className="flex items-end gap-2">
          <div className="min-w-0 flex-1">
            <Input name="did" label={t('ivr.numbers.otherLabel')} value={other} onChange={(e) => setOther(e.target.value.replace(/[^0-9+]/g, ''))} inputMode="tel" className="font-mono" placeholder="0201234567" />
          </div>
          <Button busy={busy} disabled={!did} onClick={add}><Plus className="size-4" aria-hidden />{t('common.add')}</Button>
        </div>
      )}
      <ErrorMessage error={error} />
      <p className="text-[11px] text-subtle">{t('ivr.numbers.hint')}</p>
    </div>
  )
}
