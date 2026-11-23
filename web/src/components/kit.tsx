import { useCallback, useEffect, useId, useRef, useState, type ReactNode, type SelectHTMLAttributes, type TextareaHTMLAttributes } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, Copy, Eye, EyeOff, X } from 'lucide-react'
import { api } from '../lib/api'
import { FieldErrorScope, FieldErrorText, invalidCls, useFieldError } from './fielderrors'

/** Loads data from the API and offers a reload. */
export function useApi<T>(path: string | null, intervalMs?: number) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [loading, setLoading] = useState(true)
  const load = useCallback(async () => {
    if (!path) return
    try {
      setData(await api<T>('GET', path))
      setError(null)
    } catch (e) {
      setError(e)
    } finally {
      setLoading(false)
    }
  }, [path])
  useEffect(() => {
    load()
    if (!intervalMs) return
    const id = window.setInterval(load, intervalMs)
    return () => window.clearInterval(id)
  }, [load, intervalMs])
  return { data, error, loading, reload: load, setData }
}

export function Modal({ title, subtitle, onClose, children, footer, wide }: {
  title: string; subtitle?: string; onClose: () => void; children: ReactNode; footer?: ReactNode; wide?: boolean
}) {
  const ref = useRef<HTMLDialogElement>(null)
  const { t } = useTranslation()
  const id = useId()
  useEffect(() => {
    const d = ref.current
    if (d && !d.open) d.showModal()
  }, [])
  return (
    <dialog
      ref={ref}
      onClose={onClose}
      aria-labelledby={id}
      className={`m-auto ${wide ? 'w-[min(860px,calc(100vw-32px))]' : 'w-[min(600px,calc(100vw-32px))]'} max-h-[calc(100vh-32px)] overflow-y-auto rounded-2xl border border-line bg-surface p-0 text-ink shadow-2xl backdrop:bg-black/70 backdrop:backdrop-blur-sm`}
    >
      <div className="sticky top-0 z-10 flex items-start justify-between gap-4 border-b border-line bg-surface/95 p-5 backdrop-blur">
        <div>
          <h2 id={id} className="font-display text-xl font-semibold">{title}</h2>
          {subtitle && <p className="mt-0.5 text-sm text-muted">{subtitle}</p>}
        </div>
        <button type="button" onClick={() => ref.current?.close()} aria-label={t('common.close')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-ink">
          <X className="size-5" aria-hidden />
        </button>
      </div>
      <FieldErrorScope>
        <div className="flex flex-col gap-4 p-5">{children}</div>
        {footer && <div className="sticky bottom-0 flex flex-wrap justify-end gap-2.5 border-t border-line bg-surface/95 p-4 backdrop-blur">{footer}</div>}
      </FieldErrorScope>
    </dialog>
  )
}

const fieldBase =
  'min-h-11 w-full rounded-[10px] border bg-field px-3.5 text-[15px] text-ink placeholder:text-subtle outline-none transition'
const fieldOk = 'border-line focus:border-accent/60 focus:ring-2 focus:ring-accent/20'
const fieldState = (invalid: boolean) => `${fieldBase} ${invalid ? invalidCls : fieldOk} disabled:cursor-not-allowed disabled:opacity-60`
const labelCls = (invalid: boolean) => `text-sm font-medium ${invalid ? 'text-danger-ink' : 'text-ink-2'}`

export function Select({ label, hint, children, onChange, ...rest }: SelectHTMLAttributes<HTMLSelectElement> & { label: string; hint?: ReactNode }) {
  const id = useId()
  const ref = useRef<HTMLSelectElement>(null)
  const err = useFieldError(rest.name, label, ref)
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className={labelCls(!!err.message)}>{label}</label>
      <select ref={ref} id={id} aria-invalid={err.message ? true : undefined} aria-describedby={err.message ? `${id}-err` : undefined}
        onChange={(e) => { err.clear(); onChange?.(e) }} {...rest} className={fieldState(!!err.message)}>
        {children}
      </select>
      {err.message && <FieldErrorText id={`${id}-err`}>{err.message}</FieldErrorText>}
      {hint && <p className="text-xs text-subtle">{hint}</p>}
    </div>
  )
}

export function Textarea({ label, hint, onChange, ...rest }: TextareaHTMLAttributes<HTMLTextAreaElement> & { label: string; hint?: ReactNode }) {
  const id = useId()
  const ref = useRef<HTMLTextAreaElement>(null)
  const err = useFieldError(rest.name, label, ref)
  return (
    <div className="flex flex-col gap-1.5">
      <label htmlFor={id} className={labelCls(!!err.message)}>{label}</label>
      <textarea ref={ref} id={id} aria-invalid={err.message ? true : undefined} aria-describedby={err.message ? `${id}-err` : undefined}
        onChange={(e) => { err.clear(); onChange?.(e) }} {...rest} className={`${fieldState(!!err.message)} min-h-24 py-2.5 font-mono text-sm`} />
      {err.message && <FieldErrorText id={`${id}-err`}>{err.message}</FieldErrorText>}
      {hint && <p className="text-xs text-subtle">{hint}</p>}
    </div>
  )
}

export function Toggle({ label, hint, checked, onChange, disabled, name }: {
  label: string; hint?: ReactNode; checked: boolean; onChange: (v: boolean) => void; disabled?: boolean; name?: string
}) {
  const id = useId()
  const ref = useRef<HTMLButtonElement>(null)
  const err = useFieldError(name, label, ref)
  return (
    <div className={`flex flex-col gap-1.5 rounded-xl border px-4 py-3 ${err.message ? 'border-danger/70 bg-danger/5' : 'border-line bg-surface-2/60'}`}>
      <div className="flex items-start justify-between gap-4">
        <div>
          <label htmlFor={id} className={`text-sm font-medium ${err.message ? 'text-danger-ink' : 'text-ink'}`}>{label}</label>
          {hint && <p className="mt-0.5 text-xs text-subtle">{hint}</p>}
        </div>
        <button
          ref={ref}
          id={id}
          type="button"
          role="switch"
          aria-checked={checked}
          aria-invalid={err.message ? true : undefined}
          aria-describedby={err.message ? `${id}-err` : undefined}
          disabled={disabled}
          onClick={() => { err.clear(); onChange(!checked) }}
          className={`relative mt-0.5 inline-flex h-6 w-11 flex-none items-center rounded-full border transition disabled:opacity-50 ${
            checked ? 'border-accent/60 bg-accent/30' : 'border-line bg-tint/5'
          }`}
        >
          <span className={`inline-block size-4 rounded-full transition ${checked ? 'translate-x-6 bg-accent shadow-[0_0_10px_var(--color-accent)]' : 'translate-x-1 bg-muted'}`} />
        </button>
      </div>
      {err.message && <FieldErrorText id={`${id}-err`}>{err.message}</FieldErrorText>}
    </div>
  )
}

export function Tabs<T extends string>({ tabs, value, onChange }: { tabs: { key: T; label: string }[]; value: T; onChange: (v: T) => void }) {
  return (
    <div role="tablist" className="flex flex-wrap gap-1 rounded-xl border border-line bg-surface-2/70 p-1">
      {tabs.map((tb) => (
        <button
          key={tb.key}
          role="tab"
          type="button"
          aria-selected={value === tb.key}
          onClick={() => onChange(tb.key)}
          className={`min-h-10 rounded-lg px-4 text-sm transition ${value === tb.key ? 'bg-accent/15 text-ink shadow-[inset_0_0_0_1px_rgb(34_211_238_/_0.35)]' : 'text-muted hover:text-ink'}`}
        >
          {tb.label}
        </button>
      ))}
    </div>
  )
}

type Tone = 'ok' | 'warn' | 'danger' | 'muted' | 'accent'
const tones: Record<Tone, string> = {
  ok: 'border-ok/35 bg-ok/10 text-ok-ink',
  warn: 'border-warn/35 bg-warn/10 text-warn-ink',
  danger: 'border-danger/40 bg-danger/10 text-danger-ink',
  muted: 'border-line bg-tint/5 text-muted',
  accent: 'border-accent/40 bg-accent/10 text-accent-ink',
}
const dots: Record<Tone, string> = { ok: 'bg-ok', warn: 'bg-warn', danger: 'bg-danger', muted: 'bg-subtle', accent: 'bg-accent' }

export function Badge({ tone = 'muted', dot, children }: { tone?: Tone; dot?: boolean; children: ReactNode }) {
  return (
    <span className={`inline-flex items-center gap-1.5 whitespace-nowrap rounded-full border px-2.5 py-0.5 text-xs font-medium ${tones[tone]}`}>
      {dot && <span className={`size-1.5 rounded-full ${dots[tone]}`} aria-hidden />}
      {children}
    </span>
  )
}

export const th = 'pb-2.5 pr-4 text-left text-[11px] font-medium uppercase tracking-[0.08em] text-subtle whitespace-nowrap'
export const td = 'border-t border-tint/[0.06] py-2.5 pr-4 text-[13.5px] text-ink-2 align-middle'

export function Table({ head, children, empty }: { head: ReactNode[]; children: ReactNode; empty?: ReactNode }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-collapse">
        <thead>
          <tr>{head.map((h, i) => <th key={i} className={th}>{h}</th>)}</tr>
        </thead>
        <tbody>{children}</tbody>
      </table>
      {empty}
    </div>
  )
}

export function Empty({ icon, title, text, action }: { icon?: ReactNode; title: string; text?: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center gap-3 py-12 text-center">
      {icon && <div className="flex size-14 items-center justify-center rounded-2xl border border-accent/40 bg-accent/10 text-accent shadow-[0_0_40px_-12px_rgb(34_211_238_/_0.7)]">{icon}</div>}
      <h3 className="font-display text-lg font-semibold">{title}</h3>
      {text && <p className="max-w-md text-sm text-muted">{text}</p>}
      {action}
    </div>
  )
}

export function CopyField({ label, value, secret }: { label: string; value: string; secret?: boolean }) {
  const { t } = useTranslation()
  const [shown, setShown] = useState(!secret)
  const [copied, setCopied] = useState(false)
  return (
    <div className="flex flex-col gap-1">
      <span className="text-xs text-muted">{label}</span>
      <div className="flex min-h-10 items-center gap-1 rounded-[10px] border border-line bg-field pl-3">
        <code className="flex-1 truncate font-mono text-[13px] text-ink">{shown ? value : '••••••••••••'}</code>
        {secret && (
          <button type="button" onClick={() => setShown(!shown)} aria-label={shown ? t('common.hide') : t('common.show')} className="inline-flex size-9 items-center justify-center text-muted hover:text-ink">
            {shown ? <EyeOff className="size-4" aria-hidden /> : <Eye className="size-4" aria-hidden />}
          </button>
        )}
        <button
          type="button"
          onClick={() => navigator.clipboard?.writeText(value).then(() => { setCopied(true); window.setTimeout(() => setCopied(false), 1500) })}
          aria-label={t('common.copy')}
          className="inline-flex size-9 items-center justify-center text-muted hover:text-ink"
        >
          {copied ? <Check className="size-4 text-ok" aria-hidden /> : <Copy className="size-4" aria-hidden />}
        </button>
      </div>
    </div>
  )
}

export function Stat({ label, value, sub, tone }: { label: string; value: ReactNode; sub?: ReactNode; tone?: 'ok' | 'warn' | 'danger' }) {
  const color = tone === 'ok' ? 'text-ok' : tone === 'warn' ? 'text-warn' : tone === 'danger' ? 'text-danger' : 'text-ink'
  return (
    <section className="card p-5">
      <div className="text-[13px] text-muted">{label}</div>
      <div className={`mt-2.5 font-mono text-3xl font-medium ${color}`}>{value}</div>
      {sub && <div className="mt-1.5 text-xs text-subtle">{sub}</div>}
    </section>
  )
}

export function SectionTitle({ children, action }: { children: ReactNode; action?: ReactNode }) {
  return (
    <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
      <h2 className="font-display text-lg font-semibold">{children}</h2>
      {action}
    </div>
  )
}

export function formatDate(s: string, lang: string) {
  if (!s) return '–'
  const d = new Date(s)
  return isNaN(d.getTime()) ? s : d.toLocaleString(lang, { dateStyle: 'short', timeStyle: 'short' })
}

export function formatDuration(sec: number) {
  const h = Math.floor(sec / 3600)
  const m = Math.floor((sec % 3600) / 60)
  const s = sec % 60
  return h ? `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}` : `${m}:${String(s).padStart(2, '0')}`
}

export function formatBytes(n: number) {
  if (n < 1024) return `${n} B`
  const u = ['KB', 'MB', 'GB', 'TB']
  let i = -1
  do {
    n /= 1024
    i++
  } while (n >= 1024 && i < u.length - 1)
  return `${n.toFixed(1)} ${u[i]}`
}
