import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Download, Link2, Monitor } from 'lucide-react'
import { CopyField, SectionTitle, formatBytes, useApi } from './kit'
import { Button, Card, ErrorMessage } from './ui'
import { api } from '../lib/api'

export type DesktopInfo = {
  version: string
  server?: string
  fingerprint?: string
  installer: { name: string; size: number; sha256: string; url: string } | null
}
export type PairCode = { code: string; server: string; fingerprint: string; expiresAt: string; link: string }

/** Download and link the Windows softphone (in the user portal, under My devices). */
export function WindowsSoftphoneCard() {
  const { t } = useTranslation()
  const info = useApi<DesktopInfo>('/desktop/info')
  const [pair, setPair] = useState<PairCode | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [left, setLeft] = useState(0)

  useEffect(() => {
    if (!pair) return
    const tick = () => setLeft(Math.max(0, Math.round((new Date(pair.expiresAt).getTime() - Date.now()) / 1000)))
    tick()
    const id = window.setInterval(tick, 1000)
    return () => window.clearInterval(id)
  }, [pair])

  async function link() {
    setBusy(true)
    setError(null)
    try {
      const p = await api<PairCode>('POST', '/desktop/pair')
      setPair(p)
      // Opens the softphone when it is installed; otherwise the code below can be typed in.
      window.location.href = p.link
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const inst = info.data?.installer
  return (
    <Card className="flex flex-col gap-3">
      <SectionTitle><span className="inline-flex items-center gap-2"><Monitor className="size-5 text-accent" aria-hidden />{t('desktop.portal.title')}</span></SectionTitle>
      <p className="text-sm text-muted">{t('desktop.portal.text')}</p>
      <ErrorMessage error={error} />
      <div className="flex flex-wrap gap-2.5">
        {inst ? (
          <a href={inst.url} download className="inline-flex min-h-11 items-center justify-center gap-2 rounded-[10px] border border-line px-4 text-sm font-medium text-ink-2 hover:bg-tint/5">
            <Download className="size-4" aria-hidden />{t('desktop.portal.download')} <span className="text-xs text-subtle">({formatBytes(inst.size)})</span>
          </a>
        ) : info.data && <span className="self-center text-xs text-subtle">{t('desktop.portal.noInstaller')}</span>}
        <Button onClick={link} busy={busy}><Link2 className="size-4" aria-hidden />{t('desktop.portal.link')}</Button>
      </div>
      {pair && (
        <div className="flex flex-col gap-2 rounded-xl border border-accent/30 bg-accent/[0.05] p-3.5">
          <p className="text-sm text-ink-2">{t('desktop.portal.codeText')}</p>
          <div className="font-mono text-3xl tracking-[0.2em] text-accent">{pair.code}</div>
          <p className="text-xs text-subtle">{left > 0 ? t('desktop.portal.validFor', { count: Math.ceil(left / 60) }) : t('desktop.portal.expired')}</p>
          <CopyField label={t('desktop.portal.server')} value={pair.server} />
          {pair.fingerprint && <p className="break-all font-mono text-[11px] text-subtle">{t('desktop.portal.fingerprint')}: {pair.fingerprint.match(/.{2}/g)?.join(':')}</p>}
        </div>
      )}
      {inst && <p className="break-all font-mono text-[11px] text-subtle">SHA-256: {inst.sha256}</p>}
    </Card>
  )
}

/**
 * /desktop-link: the softphone's "Sign in with browser" opens this page in the normal browser. Once
 * the user is signed in there (password, passkey, MFA or SSO), it makes a pairing code and hands it
 * to the softphone with the kiestoon:// link.
 */
export function DesktopLink() {
  const { t } = useTranslation()
  const [error, setError] = useState<unknown>(null)
  const [pair, setPair] = useState<PairCode | null>(null)
  useEffect(() => {
    api<PairCode>('POST', '/desktop/pair').then((p) => {
      setPair(p)
      window.location.href = p.link
    }).catch(setError)
  }, [])
  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <Card className="flex w-full max-w-md flex-col gap-3 text-center">
        <Monitor className="mx-auto size-8 text-accent" aria-hidden />
        <h1 className="font-display text-xl font-semibold">{t('desktop.portal.title')}</h1>
        <ErrorMessage error={error} />
        {pair && (
          <>
            <p className="text-sm text-muted">{t('desktop.portal.linkOpened')}</p>
            <div className="font-mono text-3xl tracking-[0.2em] text-accent">{pair.code}</div>
            <a href={pair.link} className="text-sm text-accent underline">{t('desktop.portal.openAgain')}</a>
          </>
        )}
      </Card>
    </div>
  )
}
