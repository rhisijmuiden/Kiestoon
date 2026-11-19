import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import QRCode from 'qrcode'
import { BellRing, QrCode, Smartphone, Trash2 } from 'lucide-react'
import { Badge, SectionTitle, formatDate, useApi } from './kit'
import { IconButton } from './pbx'
import { Alert, Button, Card, ErrorMessage } from './ui'
import { api } from '../lib/api'
import type { MobileDevice } from '../lib/types'
import type { PairCode } from './WindowsSoftphone'

/** Link the mobile app with a QR code and manage the linked phones (user portal, My devices). */
export function MobileAppCard() {
  const { t, i18n } = useTranslation()
  const d = useApi<{ enabled: boolean; devices: MobileDevice[] }>('/portal/mobile')
  const [pair, setPair] = useState<PairCode | null>(null)
  const [qr, setQr] = useState('')
  const [left, setLeft] = useState(0)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [tested, setTested] = useState<number | null>(null)

  useEffect(() => {
    if (!pair) return
    const tick = () => setLeft(Math.max(0, Math.round((new Date(pair.expiresAt).getTime() - Date.now()) / 1000)))
    tick()
    const id = window.setInterval(tick, 1000)
    return () => window.clearInterval(id)
  }, [pair])
  // While the code is shown, look for the phone that links with it.
  useEffect(() => {
    if (!pair || left === 0) return
    const id = window.setInterval(d.reload, 3000)
    return () => window.clearInterval(id)
  }, [pair, left, d.reload])
  const count = d.data?.devices.length ?? 0
  const [startCount, setStartCount] = useState(0)
  useEffect(() => {
    if (pair && count > startCount) setPair(null)
  }, [count, startCount, pair])

  if (!d.data?.enabled) return null

  async function link() {
    setBusy(true)
    setError(null)
    try {
      const p = await api<PairCode>('POST', '/desktop/pair')
      setQr(await QRCode.toDataURL(p.link, { margin: 1, width: 200, color: { dark: '#070B14', light: '#FFFFFF' } }))
      setStartCount(count)
      setPair(p)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  async function test(dev: MobileDevice) {
    setError(null)
    setTested(null)
    try {
      await api('POST', `/portal/mobile/devices/${dev.id}/test`)
      setTested(dev.id)
    } catch (e) {
      setError(e)
      d.reload()
    }
  }
  async function remove(dev: MobileDevice) {
    if (!window.confirm(t('mobile.portal.removeConfirm', { name: dev.name || t(`mobile.platforms.${dev.platform}`) }))) return
    try {
      await api('DELETE', `/portal/mobile/devices/${dev.id}`)
      d.reload()
    } catch (e) {
      setError(e)
    }
  }

  return (
    <Card className="flex flex-col gap-3">
      <SectionTitle><span className="inline-flex items-center gap-2"><Smartphone className="size-5 text-accent" aria-hidden />{t('mobile.portal.title')}</span></SectionTitle>
      <p className="text-sm text-muted">{t('mobile.portal.text')}</p>
      <ErrorMessage error={error} />
      {tested !== null && <Alert tone="ok">{t('mobile.portal.testSent')}</Alert>}
      {d.data.devices.length > 0 && (
        <ul className="flex flex-col divide-y divide-tint/[0.06] rounded-xl border border-line">
          {d.data.devices.map((dev) => (
            <li key={dev.id} className="flex items-center gap-3 px-3.5 py-2.5">
              <Smartphone className="size-4 shrink-0 text-subtle" aria-hidden />
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2 text-sm font-medium text-ink">
                  <span className="truncate">{dev.name || t(`mobile.platforms.${dev.platform}`)}</span>
                  <Badge>{t(`mobile.platforms.${dev.platform}`)}</Badge>
                </div>
                <div className="text-xs text-subtle">{t('mobile.portal.lastSeen', { date: dev.lastSeen ? formatDate(dev.lastSeen, i18n.language) : '–' })}</div>
              </div>
              <IconButton label={t('mobile.portal.test')} onClick={() => test(dev)}><BellRing className="size-4" /></IconButton>
              <IconButton label={t('mobile.remove')} danger onClick={() => remove(dev)}><Trash2 className="size-4" /></IconButton>
            </li>
          ))}
        </ul>
      )}
      <div><Button onClick={link} busy={busy} variant={count ? 'secondary' : 'primary'}><QrCode className="size-4" aria-hidden />{t('mobile.portal.link')}</Button></div>
      {pair && (
        <div className="grid items-center gap-4 rounded-xl border border-accent/30 bg-accent/[0.05] p-3.5 sm:grid-cols-[auto_1fr]">
          {qr && <img src={qr} alt={t('mobile.portal.qrAlt')} className="size-[200px] rounded-xl" />}
          <div className="flex flex-col gap-2">
            <p className="text-sm text-ink-2">{t('mobile.portal.scan')}</p>
            <p className="text-xs text-muted">{t('mobile.portal.orCode')}</p>
            <div className="font-mono text-2xl tracking-[0.2em] text-accent">{pair.code}</div>
            <p className="text-xs text-subtle">{left > 0 ? t('desktop.portal.validFor', { count: Math.ceil(left / 60) }) : t('desktop.portal.expired')}</p>
          </div>
        </div>
      )}
    </Card>
  )
}
