import { useEffect, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import QRCode from 'qrcode'
import { Copy, Download } from 'lucide-react'
import { AuthShell } from '../components/AuthShell'
import { Button, ErrorMessage, Input } from '../components/ui'
import { api } from '../lib/api'
import { useSession } from '../lib/session'

export function MfaEnroll() {
  const { t } = useTranslation()
  const { refresh, signOut } = useSession()
  const [secret, setSecret] = useState('')
  const [qr, setQr] = useState('')
  const [code, setCode] = useState('')
  const [codes, setCodes] = useState<string[] | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState(false)
  const [copied, setCopied] = useState(false)

  useEffect(() => {
    api<{ secret: string; otpauthUrl: string }>('POST', '/account/mfa/enroll')
      .then(async (r) => {
        setSecret(r.secret)
        setQr(await QRCode.toDataURL(r.otpauthUrl, { margin: 1, width: 220, color: { dark: '#070B14', light: '#FFFFFF' } }))
      })
      .catch(setError)
  }, [])

  async function confirm(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const r = await api<{ recoveryCodes: string[] }>('POST', '/account/mfa/confirm', { code })
      setCodes(r.recoveryCodes)
    } catch (err) {
      setError(err)
    } finally {
      setBusy(false)
    }
  }

  function download() {
    const blob = new Blob([`Kiestoon recovery codes\n\n${codes!.join('\n')}\n`], { type: 'text/plain' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = 'kiestoon-recovery-codes.txt'
    a.click()
    URL.revokeObjectURL(a.href)
  }

  if (codes) {
    return (
      <AuthShell>
        <div className="flex flex-col gap-5">
          <div>
            <h1 className="font-display text-2xl font-semibold tracking-tight">{t('mfa.codesTitle')}</h1>
            <p className="mt-1.5 text-sm text-muted">{t('mfa.codesText')}</p>
          </div>
          <ul className="grid grid-cols-2 gap-2 rounded-xl border border-line bg-surface-2 p-4 font-mono text-[15px]">
            {codes.map((c) => <li key={c} className="text-center text-ink">{c}</li>)}
          </ul>
          <div className="flex flex-wrap gap-2">
            <Button variant="ghost" onClick={download}><Download className="size-4" aria-hidden />{t('mfa.download')}</Button>
            <Button variant="ghost" onClick={() => { navigator.clipboard?.writeText(codes.join('\n')); setCopied(true) }}>
              <Copy className="size-4" aria-hidden />{copied ? t('common.copied') : t('common.copy')}
            </Button>
          </div>
          <Button onClick={refresh}>{t('mfa.codesSaved')}</Button>
        </div>
      </AuthShell>
    )
  }

  return (
    <AuthShell>
      <form onSubmit={confirm} className="flex flex-col gap-5">
        <div>
          <h1 className="font-display text-2xl font-semibold tracking-tight">{t('mfa.title')}</h1>
          <p className="mt-1.5 text-sm text-muted">{t('mfa.text')}</p>
        </div>
        <div className="flex justify-center">
          <div className="rounded-2xl bg-white p-3 shadow-[0_0_40px_-10px_rgb(34_211_238_/_0.6)]">
            {qr ? <img src={qr} alt="QR code" width={220} height={220} /> : <div className="size-[220px]" />}
          </div>
        </div>
        <div className="text-center">
          <p className="text-xs text-subtle">{t('mfa.manual')}</p>
          <code className="mt-1 block break-all font-mono text-sm tracking-wider text-accent">{secret.replace(/(.{4})/g, '$1 ').trim()}</code>
        </div>
        <ErrorMessage error={error} />
        <Input label={t('login.code')} value={code} onChange={(e) => setCode(e.target.value)} required inputMode="numeric" autoComplete="one-time-code" maxLength={6} pattern="[0-9]{6}" className="text-center font-mono text-2xl tracking-[0.4em]" />
        <Button type="submit" busy={busy}>{t('mfa.confirm')}</Button>
        <button type="button" onClick={signOut} className="text-sm text-muted hover:text-ink">{t('common.signOut')}</button>
      </form>
    </AuthShell>
  )
}
