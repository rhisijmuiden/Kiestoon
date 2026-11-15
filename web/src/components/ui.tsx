import { forwardRef, useId, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, CheckCircle2, Loader2 } from 'lucide-react'
import { ApiError } from '../lib/api'

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger'

const variants: Record<Variant, string> = {
  primary:
    'bg-gradient-to-br from-accent to-accent-2 text-[#06121F] font-semibold shadow-[0_8px_30px_-10px_rgb(34_211_238_/_0.6)] hover:brightness-110',
  secondary: 'border border-accent/45 bg-accent/10 text-[#A5F3FC] hover:bg-accent/15',
  ghost: 'border border-line text-ink-2 hover:bg-white/5',
  danger: 'border border-danger/50 bg-danger/10 text-[#FECACA] hover:bg-danger/15',
}

export function Button({
  variant = 'primary',
  busy,
  className = '',
  children,
  ...rest
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; busy?: boolean }) {
  return (
    <button
      {...rest}
      disabled={rest.disabled || busy}
      className={`inline-flex min-h-11 items-center justify-center gap-2 rounded-[10px] px-4 text-sm font-medium transition disabled:cursor-not-allowed disabled:opacity-55 ${variants[variant]} ${className}`}
    >
      {busy && <Loader2 className="size-4 animate-spin" aria-hidden />}
      {children}
    </button>
  )
}

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement> & { label: string; hint?: ReactNode }>(
  function Input({ label, hint, className = '', ...rest }, ref) {
    const id = useId()
    return (
      <div className="flex flex-col gap-1.5">
        <label htmlFor={id} className="text-sm font-medium text-ink-2">
          {label}
        </label>
        <input
          ref={ref}
          id={id}
          aria-describedby={hint ? `${id}-hint` : undefined}
          {...rest}
          className={`min-h-11 rounded-[10px] border border-line bg-[#0A1224] px-3.5 text-[15px] text-ink placeholder:text-subtle outline-none transition focus:border-accent/60 focus:ring-2 focus:ring-accent/20 ${className}`}
        />
        {hint && (
          <p id={`${id}-hint`} className="text-xs text-subtle">
            {hint}
          </p>
        )}
      </div>
    )
  },
)

export function Card({ className = '', children }: { className?: string; children: ReactNode }) {
  return <section className={`card p-5 ${className}`}>{children}</section>
}

export function Alert({ tone = 'danger', children }: { tone?: 'danger' | 'ok' | 'warn'; children: ReactNode }) {
  const styles = {
    danger: 'border-danger/40 bg-danger/10 text-[#FECACA]',
    ok: 'border-ok/40 bg-ok/10 text-[#A7F3D0]',
    warn: 'border-warn/40 bg-warn/10 text-[#FDE68A]',
  }[tone]
  const Icon = tone === 'ok' ? CheckCircle2 : AlertTriangle
  return (
    <div role={tone === 'danger' ? 'alert' : 'status'} className={`flex items-start gap-2.5 rounded-xl border px-3.5 py-3 text-sm ${styles}`}>
      <Icon className="mt-0.5 size-4 flex-none" aria-hidden />
      <div>{children}</div>
    </div>
  )
}

/** Shows an API error in the user's language. */
export function ErrorMessage({ error }: { error: unknown }) {
  const { t } = useTranslation()
  if (!error) return null
  const code = error instanceof ApiError ? error.code : 'internal'
  const field = error instanceof ApiError && error.field ? t(`fields.${error.field}`, { defaultValue: error.field }) : ''
  const detail = error instanceof ApiError ? error.detail : ''
  return (
    <Alert>
      {t(`errors.${code}`, { defaultValue: t('errors.internal'), field })}
      {detail && <span className="mt-1 block font-mono text-xs opacity-80">{detail}</span>}
    </Alert>
  )
}

export function PageHeader({ title, subtitle, children }: { title: string; subtitle?: string; children?: ReactNode }) {
  return (
    <div className="flex flex-wrap items-center justify-between gap-4">
      <div>
        <h1 className="font-display text-[28px] font-semibold tracking-tight">{title}</h1>
        {subtitle && <p className="mt-1 text-sm text-muted">{subtitle}</p>}
      </div>
      {children && <div className="flex flex-wrap items-center gap-2.5">{children}</div>}
    </div>
  )
}
