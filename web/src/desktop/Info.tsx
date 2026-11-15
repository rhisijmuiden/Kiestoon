import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowUpCircle, CheckCircle2, RefreshCw } from 'lucide-react'
import { Mark, PoweredBy } from '../components/brand'
import { useSystemInfo } from '../components/InfoDialog'
import { Alert, Button, ErrorMessage } from '../components/ui'
import { useSession } from '../lib/session'
import { inShell, listen, shell, type AppInfo, type Channel, type UpdateCheck } from './bridge'
import { Title } from './Contacts'

/** Version of the app and the server, the update channel, checking for and installing updates. */
export function Info() {
  const { t, i18n } = useTranslation()
  const { state } = useSession()
  const { info: server } = useSystemInfo()
  const [app, setApp] = useState<AppInfo | null>(null)
  const [check, setCheck] = useState<UpdateCheck | null>(null)
  const [checking, setChecking] = useState(false)
  const [installing, setInstalling] = useState<number | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [headset, setHeadset] = useState('')

  useEffect(() => {
    shell.info().then(setApp).catch(setError)
    shell.headsets().then((h) => setHeadset(h?.[0]?.name ?? '')).catch(() => undefined)
    return listen<{ percent: number }>('update-progress', (p) => setInstalling(p.percent))
  }, [])

  async function runCheck() {
    setChecking(true)
    setError(null)
    try {
      setCheck(await shell.checkUpdate())
    } catch (e) {
      setError(new Error(String(e)))
    } finally {
      setChecking(false)
    }
  }
  async function setChannel(c: Channel) {
    if (!app || c === app.channel) return
    await shell.setChannel(c)
    setApp({ ...app, channel: c })
    setCheck(null)
  }
  async function install() {
    setError(null)
    setInstalling(0)
    try {
      await shell.installUpdate()
    } catch (e) {
      setInstalling(null)
      setError(new Error(String(e)))
    }
  }
  const user = state.kind === 'ready' ? state.me : null
  const row = (k: string, v: React.ReactNode) => (
    <div className="flex justify-between gap-3 border-b border-line py-2 text-[12.5px]"><span className="text-muted">{k}</span><span className="truncate text-right text-ink">{v}</span></div>
  )
  return (
    <div className="pb-4">
      <Title>{t('desktop.nav.info')}</Title>
      <div className="flex flex-col gap-3 px-3.5">
        <div className="flex items-center gap-3">
          <Mark size={46} />
          <div><div className="text-[15px] font-semibold text-ink">Kiestoon Softphone</div><div className="text-xs text-muted">{app?.os ?? t('desktop.info.browser')}</div></div>
        </div>
        <div>
          {row(t('desktop.info.version'), <span className="font-mono">{app?.version ?? '–'}</span>)}
          {row(t('desktop.info.server'), <span>{app?.server ? new URL(app.server).host : location.host}{server ? <span className="font-mono text-subtle"> · {server.version}</span> : null}</span>)}
          {row(t('desktop.info.account'), user ? `${user.extension?.number ?? ''} · ${user.user.displayName || user.user.username}` : '–')}
          {inShell() && row(t('desktop.settings.headset'), headset || t('desktop.settings.noHeadsetShort'))}
        </div>

        {inShell() && app && (
          <>
            <div>
              <div className="mb-1.5 text-[10.5px] uppercase tracking-[0.08em] text-subtle">{t('desktop.info.channel')}</div>
              <div role="radiogroup" aria-label={t('desktop.info.channel')} className="flex overflow-hidden rounded-[10px] border border-line text-xs">
                {(['production', 'testing'] as const).map((c) => (
                  <button key={c} type="button" role="radio" aria-checked={app.channel === c} onClick={() => setChannel(c)}
                    className={`flex-1 py-2 ${app.channel === c ? 'bg-accent/15 font-semibold text-accent' : 'text-ink-2 hover:bg-tint/5'}`}>
                    {t(`desktop.info.${c}`)}
                  </button>
                ))}
              </div>
              {app.channel === 'testing' && <p className="mt-1.5 text-xs text-warn">{t('desktop.info.testingHint')}</p>}
            </div>

            <ErrorMessage error={error} />
            {check && !check.available && <Alert tone="ok">{t('desktop.info.upToDate', { version: check.current })}</Alert>}
            {check?.available && check.latest && (
              <div className="rounded-xl border border-ok/50 bg-ok/[0.08] p-3 text-xs">
                <b className="flex items-center gap-1.5 text-[13px] text-ok"><ArrowUpCircle className="size-4" aria-hidden />{t('desktop.info.available', { version: check.latest.version })}</b>
                {check.latest.publishedAt && <div className="mt-0.5 text-subtle">{new Date(check.latest.publishedAt).toLocaleDateString(i18n.language)}</div>}
                {check.latest.notes && <pre className="mt-2 max-h-32 overflow-y-auto whitespace-pre-wrap font-sans text-muted">{check.latest.notes}</pre>}
              </div>
            )}
            {installing !== null && (
              <div className="flex flex-col gap-1.5" role="status">
                <div className="h-1.5 overflow-hidden rounded-full bg-line"><div className="h-full bg-gradient-to-r from-accent to-accent-2 transition-all" style={{ width: `${installing}%` }} /></div>
                <span className="text-xs text-muted">{installing < 100 ? t('desktop.info.downloading', { percent: installing }) : t('desktop.info.installing')}</span>
              </div>
            )}
            <div className="grid grid-cols-2 gap-2">
              <Button variant="ghost" className="min-h-9 text-[12.5px]" busy={checking} onClick={runCheck}><RefreshCw className="size-4" aria-hidden />{t('desktop.info.check')}</Button>
              <Button className="min-h-9 text-[12.5px]" disabled={!check?.available || installing !== null} onClick={install}>
                {check && !check.available ? <CheckCircle2 className="size-4" aria-hidden /> : <ArrowUpCircle className="size-4" aria-hidden />}{t('desktop.info.install')}
              </Button>
            </div>
          </>
        )}
        <PoweredBy className="mt-2 flex self-end" />
      </div>
    </div>
  )
}
