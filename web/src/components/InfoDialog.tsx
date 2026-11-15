import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowUpCircle, CheckCircle2, ExternalLink, FlaskConical, RefreshCw, ShieldCheck, X } from 'lucide-react'
import { api, type CheckResult, type SystemInfo, type UpdateStatus } from '../lib/api'
import { Alert, Button, ErrorMessage } from './ui'
import { Mark } from './brand'

export function useSystemInfo() {
  const [info, setInfo] = useState<SystemInfo | null>(null)
  const reload = useCallback(() => {
    api<SystemInfo>('GET', '/system/info').then(setInfo).catch(() => undefined)
  }, [])
  useEffect(reload, [reload])
  return { info, reload }
}

function formatUptime(s: number) {
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  const m = Math.floor((s % 3600) / 60)
  return d ? `${d}d ${h}h` : h ? `${h}h ${m}m` : `${m}m`
}

export function InfoDialog({ admin, onClose }: { admin: boolean; onClose: () => void }) {
  const { t, i18n } = useTranslation()
  const ref = useRef<HTMLDialogElement>(null)
  const { info, reload } = useSystemInfo()
  const [check, setCheck] = useState<CheckResult | null>(null)
  const [checking, setChecking] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const [status, setStatus] = useState<UpdateStatus | null>(null)
  const [error, setError] = useState<unknown>(null)
  const startVersion = useRef<string | null>(null)

  useEffect(() => {
    const d = ref.current
    if (d && !d.open) d.showModal()
  }, [])

  // Load the updater status, and keep polling while an update runs.
  useEffect(() => {
    if (!admin) return
    let stop = false
    const tick = async () => {
      try {
        const st = await api<UpdateStatus>('GET', '/system/updates/status')
        if (!stop) setStatus(st)
        // The server restarts during an update; once it reports a new version, reload the page.
        const health = await api<{ version: string }>('GET', '/health')
        if (startVersion.current && health.version !== startVersion.current && st.state === 'success') window.location.reload()
      } catch {
        /* server restarting */
      }
    }
    tick()
    const busy = status?.state === 'queued' || status?.state === 'running'
    const id = busy ? window.setInterval(tick, 3000) : undefined
    return () => {
      stop = true
      if (id) window.clearInterval(id)
    }
  }, [admin, status?.state])

  async function setChannel(channel: 'production' | 'testing') {
    setError(null)
    setCheck(null)
    try {
      await api('PUT', '/system/channel', { channel })
      reload()
    } catch (e) {
      setError(e)
    }
  }

  async function runCheck() {
    setError(null)
    setChecking(true)
    try {
      setCheck(await api<CheckResult>('POST', '/system/updates/check'))
    } catch (e) {
      setError(e)
    } finally {
      setChecking(false)
    }
  }

  async function apply() {
    if (!check?.latest) return
    setError(null)
    try {
      startVersion.current = info?.version ?? null
      setStatus(await api<UpdateStatus>('POST', '/system/updates/apply', { version: check.latest.version }))
      setConfirming(false)
    } catch (e) {
      setError(e)
    }
  }

  const busy = status?.state === 'queued' || status?.state === 'running'
  const rows: [string, string][] = info
    ? [
        [t('info.version'), info.version],
        [t('info.commit'), info.commit.slice(0, 12)],
        [t('info.built'), info.buildDate ? new Date(info.buildDate).toLocaleString(i18n.language) : '–'],
        [t('info.host'), info.hostname],
        [t('info.uptime'), formatUptime(info.uptime)],
      ]
    : []

  return (
    <dialog
      ref={ref}
      onClose={onClose}
      aria-labelledby="info-title"
      className="m-auto w-[min(560px,calc(100vw-32px))] max-h-[calc(100vh-32px)] overflow-y-auto rounded-2xl border border-line bg-surface p-0 text-ink shadow-2xl backdrop:bg-black/70 backdrop:backdrop-blur-sm"
    >
      <div className="flex items-start justify-between gap-4 border-b border-line p-5">
        <div className="flex items-center gap-3">
          <Mark size={40} />
          <div>
            <h2 id="info-title" className="font-display text-xl font-semibold">{t('info.title')}</h2>
            <p className="font-mono text-sm text-accent">{info?.version ?? '…'}</p>
          </div>
        </div>
        <button type="button" onClick={() => ref.current?.close()} aria-label={t('common.close')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-white/5 hover:text-ink">
          <X className="size-5" aria-hidden />
        </button>
      </div>

      <div className="flex flex-col gap-5 p-5">
        <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-sm">
          {rows.map(([k, v]) => (
            <div key={k} className="contents">
              <dt className="text-muted">{k}</dt>
              <dd className="m-0 truncate font-mono text-ink-2">{v}</dd>
            </div>
          ))}
        </dl>

        {admin ? (
          <>
            <fieldset className="flex flex-col gap-2">
              <legend className="mb-2 text-sm font-medium text-ink-2">{t('info.channel')}</legend>
              <div className="grid gap-2 sm:grid-cols-2">
                {(['production', 'testing'] as const).map((c) => {
                  const active = info?.channel === c
                  const Icon = c === 'production' ? ShieldCheck : FlaskConical
                  return (
                    <label
                      key={c}
                      className={`flex cursor-pointer gap-3 rounded-xl border p-3 transition ${
                        active ? 'border-accent/50 bg-accent/10' : 'border-line hover:bg-white/5'
                      } ${busy ? 'pointer-events-none opacity-60' : ''}`}
                    >
                      <input type="radio" name="channel" className="sr-only" checked={active} onChange={() => setChannel(c)} disabled={busy} />
                      <Icon className={`mt-0.5 size-5 flex-none ${c === 'production' ? 'text-ok' : 'text-warn'}`} aria-hidden />
                      <span>
                        <span className="block text-sm font-medium">{t(`info.${c}`)}</span>
                        <span className="block text-xs text-subtle">{t(`info.${c}Hint`)}</span>
                      </span>
                    </label>
                  )
                })}
              </div>
            </fieldset>

            <ErrorMessage error={error} />

            {status && status.state !== 'idle' && (
              <Alert tone={status.state === 'failed' ? 'danger' : status.state === 'success' ? 'ok' : 'warn'}>
                {t(`info.state.${status.state}`, { version: status.version, message: status.message })}
              </Alert>
            )}

            {check && !busy && (
              <div className="rounded-xl border border-line bg-surface-2/70 p-4 text-sm">
                {!check.latest && <p className="text-muted">{t('info.noReleases')}</p>}
                {check.latest && !check.updateAvailable && !check.downgrade && (
                  <p className="flex items-center gap-2 text-[#A7F3D0]"><CheckCircle2 className="size-4" aria-hidden />{t('info.upToDate')}</p>
                )}
                {check.latest && (check.updateAvailable || check.downgrade) && (
                  <div className="flex flex-col gap-3">
                    <p className="flex items-start gap-2">
                      <ArrowUpCircle className="mt-0.5 size-4 flex-none text-accent" aria-hidden />
                      {check.updateAvailable ? t('info.available', { version: check.latest.version }) : t('info.downgrade', { version: check.latest.version })}
                    </p>
                    {check.latest.notes && (
                      <details className="text-ink-2">
                        <summary className="cursor-pointer text-accent">{t('info.releaseNotes')}</summary>
                        <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap font-sans text-xs text-muted">{check.latest.notes}</pre>
                      </details>
                    )}
                    {check.latest.url && (
                      <a href={check.latest.url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-xs text-accent hover:underline">
                        GitHub <ExternalLink className="size-3" aria-hidden />
                      </a>
                    )}
                    {confirming ? (
                      <div className="flex flex-col gap-3 rounded-lg border border-warn/40 bg-warn/10 p-3">
                        <p className="text-[#FDE68A]">{t('info.confirm', { version: check.latest.version })}</p>
                        <div className="flex flex-wrap gap-2">
                          <Button onClick={apply}>{t('info.updateNow')}</Button>
                          <Button variant="ghost" onClick={() => setConfirming(false)}>{t('common.cancel')}</Button>
                        </div>
                      </div>
                    ) : (
                      <Button className="self-start" onClick={() => setConfirming(true)}>
                        {check.updateAvailable ? t('info.updateNow') : t('info.installVersion', { version: check.latest.version })}
                      </Button>
                    )}
                  </div>
                )}
              </div>
            )}

            <Button variant="secondary" onClick={runCheck} busy={checking} disabled={busy}>
              {!checking && <RefreshCw className="size-4" aria-hidden />}
              {checking ? t('info.checking') : t('info.check')}
            </Button>
          </>
        ) : (
          <p className="text-sm text-muted">{t('info.adminOnly')}</p>
        )}
      </div>
    </dialog>
  )
}
