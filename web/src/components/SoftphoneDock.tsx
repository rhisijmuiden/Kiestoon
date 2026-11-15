import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { Delete, Grid3x3, Headset, Mic, MicOff, Pause, Phone, PhoneForwarded, PhoneIncoming, PhoneOff, Play, X } from 'lucide-react'
import { formatDuration } from './kit'
import { useSoftphone } from '../lib/softphone'

const PAD = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '*', '0', '#']

/** The floating softphone: a headset button that opens a dialer, and the call controls. */
export function SoftphoneDock() {
  const { t } = useTranslation()
  const p = useSoftphone()
  const [number, setNumber] = useState('')
  const [showPad, setShowPad] = useState(false)
  const [now, setNow] = useState(Date.now())
  useEffect(() => {
    if (p.call?.state !== 'active') return
    const id = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(id)
  }, [p.call?.state])
  if (!p.available) return null
  const dot = p.status === 'registered' ? 'bg-ok glow-ok' : p.status === 'connecting' ? 'bg-warn animate-pulse' : p.status === 'off' ? 'bg-subtle' : 'bg-danger'
  const c = p.call
  const press = (d: string) => (c?.state === 'active' ? p.dtmf(d) : setNumber((n) => (n + d).slice(0, 24)))

  return (
    <>
      <button
        type="button"
        onClick={() => p.setOpen(!p.open)}
        aria-label={t('softphone.title')}
        aria-expanded={p.open}
        className={`fixed bottom-16 right-6 z-40 inline-flex size-14 items-center justify-center rounded-2xl border shadow-2xl transition ${
          c?.state === 'ringing' ? 'animate-bounce border-ok/60 bg-ok/25 text-ok' : c ? 'border-danger/50 bg-danger/20 text-danger-ink' : 'border-accent/45 bg-surface text-accent hover:bg-accent/10'
        }`}
      >
        {c?.state === 'ringing' ? <PhoneIncoming className="size-6" aria-hidden /> : <Headset className="size-6" aria-hidden />}
        <span className={`absolute right-2 top-2 size-2.5 rounded-full ${dot}`} aria-hidden />
      </button>

      {p.open && (
        <section aria-label={t('softphone.title')} className="fixed bottom-32 right-6 z-40 w-[min(320px,calc(100vw-32px))] rounded-2xl border border-line bg-surface/95 p-4 shadow-2xl backdrop-blur">
          <div className="mb-3 flex items-center justify-between">
            <div className="flex items-center gap-2">
              <span className={`size-2 rounded-full ${dot}`} aria-hidden />
              <span className="text-sm font-medium text-ink">{t('softphone.title')}</span>
              <span className="text-xs text-subtle">{t(`softphone.status.${p.status}`)}</span>
            </div>
            <button type="button" onClick={() => p.setOpen(false)} aria-label={t('common.close')} className="inline-flex size-9 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-ink">
              <X className="size-4" aria-hidden />
            </button>
          </div>

          {p.status === 'off' || p.status === 'failed' ? (
            <div className="flex flex-col gap-3 text-sm text-muted">
              <p>{p.status === 'failed' ? t('softphone.failedText') : t('softphone.offText')}</p>
              <button type="button" onClick={p.enable} className="min-h-11 rounded-[10px] bg-gradient-to-br from-accent to-accent-2 font-semibold text-on-accent">{t('softphone.enable')}</button>
            </div>
          ) : p.status === 'insecure' ? (
            <p className="text-sm text-warn">{t('softphone.insecureText')}</p>
          ) : c ? (
            <div className="flex flex-col items-center gap-4 py-2">
              <div className="text-center">
                <div className="text-xs uppercase tracking-[0.1em] text-subtle">{t(`softphone.call.${c.state}`)}</div>
                <div className="mt-1 font-mono text-2xl text-ink">{c.remote}</div>
                {c.remoteName && <div className="text-sm text-muted">{c.remoteName}</div>}
                {c.state === 'active' && <div className="mt-1 font-mono text-sm text-accent">{formatDuration(Math.floor((now - c.startedAt) / 1000))}</div>}
              </div>
              {c.state === 'active' && (
                <div className="flex gap-2">
                  <CtrlButton label={c.muted ? t('softphone.unmute') : t('softphone.mute')} active={c.muted} onClick={p.toggleMute}>{c.muted ? <MicOff className="size-5" aria-hidden /> : <Mic className="size-5" aria-hidden />}</CtrlButton>
                  <CtrlButton label={c.held ? t('softphone.resume') : t('softphone.hold')} active={c.held} onClick={p.toggleHold}>{c.held ? <Play className="size-5" aria-hidden /> : <Pause className="size-5" aria-hidden />}</CtrlButton>
                  <CtrlButton label={t('softphone.keypad')} active={showPad} onClick={() => setShowPad(!showPad)}><Grid3x3 className="size-5" aria-hidden /></CtrlButton>
                  <Link to="/me/colleagues" aria-label={t('transfer.transfer')} title={t('transfer.transfer')}
                    className="inline-flex size-12 items-center justify-center rounded-xl border border-line text-ink-2 transition hover:bg-tint/5">
                    <PhoneForwarded className="size-5" aria-hidden />
                  </Link>
                </div>
              )}
              {c.state === 'active' && showPad && <Keypad onPress={press} />}
              <div className="flex gap-4">
                {c.state === 'ringing' && c.direction === 'incoming' && (
                  <button type="button" onClick={p.answer} aria-label={t('softphone.answer')} className="inline-flex size-14 items-center justify-center rounded-full bg-ok text-on-accent shadow-[0_0_30px_-6px_var(--color-ok)]">
                    <Phone className="size-6" aria-hidden />
                  </button>
                )}
                <button type="button" onClick={p.hangup} aria-label={t('softphone.hangup')} className="inline-flex size-14 items-center justify-center rounded-full bg-danger text-white shadow-[0_0_30px_-6px_var(--color-danger)]">
                  <PhoneOff className="size-6" aria-hidden />
                </button>
              </div>
            </div>
          ) : (
            <form className="flex flex-col gap-3" onSubmit={(e) => { e.preventDefault(); p.dial(number); setNumber('') }}>
              <div className="flex items-center gap-1 rounded-[10px] border border-line bg-field pl-3">
                <input value={number} onChange={(e) => setNumber(e.target.value.replace(/[^0-9+*#]/g, ''))} inputMode="tel" aria-label={t('softphone.number')}
                  placeholder={t('softphone.number')} className="min-h-12 flex-1 bg-transparent font-mono text-xl text-ink outline-none placeholder:font-sans placeholder:text-sm placeholder:text-subtle" />
                {number && <button type="button" onClick={() => setNumber(number.slice(0, -1))} aria-label={t('softphone.backspace')} className="inline-flex size-10 items-center justify-center text-muted hover:text-ink"><Delete className="size-4" aria-hidden /></button>}
              </div>
              <Keypad onPress={press} />
              <button type="submit" disabled={!number || p.status !== 'registered'} className="inline-flex min-h-12 items-center justify-center gap-2 rounded-[10px] bg-ok font-semibold text-on-accent disabled:opacity-40">
                <Phone className="size-5" aria-hidden /> {t('softphone.call.start')}
              </button>
            </form>
          )}
        </section>
      )}
    </>
  )
}

function Keypad({ onPress }: { onPress: (d: string) => void }) {
  return (
    <div className="grid w-full grid-cols-3 gap-2">
      {PAD.map((d) => (
        <button key={d} type="button" onClick={() => onPress(d)} className="min-h-12 rounded-xl border border-line font-mono text-lg text-ink transition hover:border-accent/50 hover:bg-accent/10">
          {d}
        </button>
      ))}
    </div>
  )
}

function CtrlButton({ label, active, onClick, children }: { label: string; active?: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button type="button" onClick={onClick} aria-label={label} title={label} aria-pressed={active}
      className={`inline-flex size-12 items-center justify-center rounded-xl border transition ${active ? 'border-warn/50 bg-warn/15 text-warn' : 'border-line text-ink-2 hover:bg-tint/5'}`}>
      {children}
    </button>
  )
}
