import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { BellOff, CalendarClock, PhoneForwarded, Smartphone, X } from 'lucide-react'
import { Select, Toggle, useApi } from '../../components/kit'
import { IconButton } from '../../components/pbx'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../../components/ui'
import { api } from '../../lib/api'
import type { Features, PortalMe } from '../../lib/types'
import { FeatureCodeTable } from '../Features'

const clean = (s: string) => s.replace(/[^0-9+]/g, '')

/** Call behaviour of the signed-in user's own extension. */
export function MyPhone() {
  const { t } = useTranslation()
  const me = useApi<PortalMe>('/portal/me')
  const [f, setF] = useState<Features | null>(null)
  const [mobile, setMobile] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  useEffect(() => {
    if (!me.data) return
    setF(me.data.features)
    try {
      setMobile(localStorage.getItem('kiestoon.mobile') ?? '')
    } catch {
      /* ignore */
    }
  }, [me.data])
  if (!me.data || !f) return <><PageHeader title={t('portal.myPhone')} /><ErrorMessage error={me.error} /></>

  async function save(next: Features) {
    setBusy(true)
    setError(null)
    setSaved(false)
    try {
      setF(await api<Features>('PUT', '/portal/features', next))
      setSaved(true)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const set = <K extends keyof Features>(k: K, v: Features[K]) => setF({ ...f, [k]: v })
  const fm = f.followMe
  const toMobile = !!mobile && f.forwardAlways === mobile
  function forwardToMobile(on: boolean) {
    try {
      localStorage.setItem('kiestoon.mobile', mobile)
    } catch {
      /* ignore */
    }
    save({ ...f!, forwardAlways: on ? mobile : '' })
  }

  return (
    <>
      <PageHeader title={t('portal.myPhone')} subtitle={`${me.data.number} · ${me.data.name}`}>
        <Button busy={busy} onClick={() => save(f)}>{t('common.save')}</Button>
      </PageHeader>
      <ErrorMessage error={error} />
      {saved && <Alert tone="ok">{t('common.saved')}</Alert>}

      <div className="grid items-start gap-4 md:grid-cols-2">
        <QuickTile icon={<BellOff className="size-5" aria-hidden />} title={t('portal.dnd')} text={t('portal.dndHint')} active={f.dnd} tone="danger"
          onClick={() => save({ ...f, dnd: !f.dnd })} label={f.dnd ? t('portal.on') : t('portal.off')} />
        <div className={`card flex flex-col gap-3 p-5 ${toMobile ? 'shadow-[inset_0_0_0_1px_rgb(34_211_238_/_0.45)]' : ''}`}>
          <div className="flex items-start gap-3">
            <span className={`flex size-10 flex-none items-center justify-center rounded-xl ${toMobile ? 'bg-accent/20 text-accent' : 'bg-tint/5 text-muted'}`}><Smartphone className="size-5" aria-hidden /></span>
            <div>
              <h2 className="font-medium text-ink">{t('portal.toMobile')}</h2>
              <p className="text-xs text-subtle">{t('portal.toMobileHint')}</p>
            </div>
          </div>
          <div className="flex items-end gap-2">
            <div className="flex-1"><Input label={t('portal.mobileNumber')} value={mobile} onChange={(e) => setMobile(clean(e.target.value))} inputMode="tel" className="font-mono" placeholder="0612345678" /></div>
            <Button variant={toMobile ? 'danger' : 'secondary'} disabled={!mobile || busy} onClick={() => forwardToMobile(!toMobile)}>{toMobile ? t('portal.stop') : t('portal.forwardNow')}</Button>
          </div>
        </div>
      </div>

      <div className="grid items-start gap-6 lg:grid-cols-2">
        <Card className="flex flex-col gap-4">
          <h2 className="flex items-center gap-2 font-display text-lg font-semibold"><PhoneForwarded className="size-5 text-accent" aria-hidden />{t('portal.forwarding')}</h2>
          <Input label={t('portal.forwardAlways')} value={f.forwardAlways} onChange={(e) => set('forwardAlways', clean(e.target.value))} inputMode="tel" className="font-mono" hint={t('portal.forwardAlwaysHint')} />
          <Input label={t('portal.forwardBusy')} value={f.forwardBusy} onChange={(e) => set('forwardBusy', clean(e.target.value))} inputMode="tel" className="font-mono" hint={t('portal.forwardBusyHint')} />
          <Input label={t('portal.forwardNoAnswer')} value={f.forwardNoAnswer} onChange={(e) => set('forwardNoAnswer', clean(e.target.value))} inputMode="tel" className="font-mono" hint={t('portal.forwardNoAnswerHint')} />
        </Card>
        <Card className="flex flex-col gap-4">
          <h2 className="font-display text-lg font-semibold">{t('portal.followMe')}</h2>
          <Toggle label={t('portal.followMeOn')} hint={t('portal.followMeHint')} checked={fm.enabled} onChange={(v) => set('followMe', { ...fm, enabled: v })} />
          {fm.enabled && (
            <>
              {fm.numbers.map((n, i) => (
                <div key={i} className="flex items-end gap-2">
                  <div className="flex-1"><Input label={t('portal.followMeNumber', { n: i + 1 })} value={n} inputMode="tel" className="font-mono"
                    onChange={(e) => set('followMe', { ...fm, numbers: fm.numbers.map((x, j) => (j === i ? clean(e.target.value) : x)) })} /></div>
                  <IconButton label={t('common.remove')} danger onClick={() => set('followMe', { ...fm, numbers: fm.numbers.filter((_, j) => j !== i) })}><X className="size-4" aria-hidden /></IconButton>
                </div>
              ))}
              {fm.numbers.length < 3 && <div><Button variant="ghost" onClick={() => set('followMe', { ...fm, numbers: [...fm.numbers, ''] })}>{t('portal.addNumber')}</Button></div>}
              <Input label={t('portal.followMeDelay')} type="number" min={0} max={60} value={fm.delay} onChange={(e) => set('followMe', { ...fm, delay: Number(e.target.value) })} hint={t('portal.followMeDelayHint')} />
              <Toggle label={t('portal.followMeConfirm')} hint={t('portal.followMeConfirmHint')} checked={fm.confirm} onChange={(v) => set('followMe', { ...fm, confirm: v })} />
            </>
          )}
        </Card>
        <Card className="flex flex-col gap-4">
          <h2 className="flex items-center gap-2 font-display text-lg font-semibold"><CalendarClock className="size-5 text-accent" aria-hidden />{t('portal.schedule')}</h2>
          <p className="text-sm text-muted">{t('portal.scheduleHint')}</p>
          <Select label={t('portal.scheduleSelect')} value={f.scheduleId} onChange={(e) => set('scheduleId', Number(e.target.value))}>
            <option value={0}>{t('portal.noSchedule')}</option>
            {me.data.schedules.map((s) => <option key={s.id} value={s.id}>{s.name}</option>)}
          </Select>
          {f.scheduleId > 0 && <Input label={t('portal.closedForward')} value={f.closedForward} onChange={(e) => set('closedForward', clean(e.target.value))} inputMode="tel" className="font-mono" hint={t('portal.closedForwardHint')} />}
        </Card>
        <Card>
          <h2 className="mb-3 font-display text-lg font-semibold">{t('features.codes')}</h2>
          <FeatureCodeTable codes={me.data.codes} />
        </Card>
      </div>
    </>
  )
}

function QuickTile({ icon, title, text, active, onClick, label, tone }: { icon: ReactNode; title: string; text: string; active: boolean; onClick: () => void; label: string; tone: 'danger' | 'accent' }) {
  const on = tone === 'danger' ? 'border-danger/50 bg-danger/10' : 'border-accent/50 bg-accent/10'
  return (
    <button type="button" onClick={onClick} aria-pressed={active} className={`card flex items-start gap-3 p-5 text-left transition ${active ? on : 'hover:border-accent/30'}`}>
      <span className={`flex size-10 flex-none items-center justify-center rounded-xl ${active ? (tone === 'danger' ? 'bg-danger/20 text-danger' : 'bg-accent/20 text-accent') : 'bg-tint/5 text-muted'}`}>{icon}</span>
      <span className="flex-1">
        <span className="block font-medium text-ink">{title}</span>
        <span className="block text-xs text-subtle">{text}</span>
      </span>
      <span className={`rounded-full px-2.5 py-0.5 text-xs font-medium ${active ? 'bg-danger/20 text-danger-ink' : 'bg-tint/5 text-muted'}`}>{label}</span>
    </button>
  )
}
