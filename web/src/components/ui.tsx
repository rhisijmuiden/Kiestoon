import { forwardRef, useId, useImperativeHandle, useRef, type ButtonHTMLAttributes, type InputHTMLAttributes, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, CheckCircle2, Loader2 } from 'lucide-react'
import { ApiError } from '../lib/api'
import { FieldErrorText, invalidCls, nextErrorSeq, useFieldError, useFieldScope, useReportFieldError } from './fielderrors'

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger'

const variants: Record<Variant, string> = {
  primary:
    'bg-gradient-to-br from-accent to-accent-2 text-on-accent font-semibold shadow-[0_8px_30px_-10px_rgb(34_211_238_/_0.6)] hover:brightness-110',
  secondary: 'border border-accent/45 bg-accent/10 text-accent-ink hover:bg-accent/15',
  ghost: 'border border-line text-ink-2 hover:bg-tint/5',
  danger: 'border border-danger/50 bg-danger/10 text-danger-ink hover:bg-danger/15',
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
  function Input({ label, hint, className = '', onChange, ...rest }, ref) {
    const id = useId()
    const inner = useRef<HTMLInputElement>(null)
    useImperativeHandle(ref, () => inner.current as HTMLInputElement)
    const err = useFieldError(rest.name, label, inner)
    const describedBy = [err.message && `${id}-err`, hint && `${id}-hint`].filter(Boolean).join(' ') || undefined
    return (
      <div className="flex flex-col gap-1.5">
        <label htmlFor={id} className={`text-sm font-medium ${err.message ? 'text-danger-ink' : 'text-ink-2'}`}>
          {label}
        </label>
        <input
          ref={inner}
          id={id}
          aria-describedby={describedBy}
          aria-invalid={err.message ? true : undefined}
          onChange={(e) => {
            err.clear()
            onChange?.(e)
          }}
          {...rest}
          className={`min-h-11 rounded-[10px] border bg-field px-3.5 text-[15px] text-ink placeholder:text-subtle outline-none transition ${err.message ? invalidCls : 'border-line focus:border-accent/60 focus:ring-2 focus:ring-accent/20'} ${className}`}
        />
        {err.message && <FieldErrorText id={`${id}-err`}>{err.message}</FieldErrorText>}
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
    danger: 'border-danger/40 bg-danger/10 text-danger-ink',
    ok: 'border-ok/40 bg-ok/10 text-ok-ink',
    warn: 'border-warn/40 bg-warn/10 text-warn-ink',
  }[tone]
  const Icon = tone === 'ok' ? CheckCircle2 : AlertTriangle
  return (
    <div role={tone === 'danger' ? 'alert' : 'status'} className={`flex items-start gap-2.5 rounded-xl border px-3.5 py-3 text-sm ${styles}`}>
      <Icon className="mt-0.5 size-4 flex-none" aria-hidden />
      <div>{children}</div>
    </div>
  )
}

const errorSeqs = new WeakMap<object, number>()
const errorSeq = (e: unknown) => {
  if (!e || typeof e !== 'object') return 0
  let n = errorSeqs.get(e)
  if (!n) {
    n = nextErrorSeq()
    errorSeqs.set(e, n)
  }
  return n
}

/**
 * Shows an API error in the user's language. When the error is about one field, the banner names
 * that field and the field itself turns red with the message under it (see fielderrors.tsx).
 */
export function ErrorMessage({ error }: { error: unknown }) {
  const { t } = useTranslation()
  const scope = useFieldScope()
  const apiErr = error instanceof ApiError ? error : null
  const code = apiErr?.code ?? 'internal'
  const fieldKey = apiErr?.field || undefined
  const rawField = fieldKey ? scope?.label(fieldKey) || t(`fields.${fieldKey}`, { defaultValue: '' }) : ''
  const field = rawField.charAt(0).toUpperCase() + rawField.slice(1)
  const message = error ? t(`errors.${code}`, { defaultValue: t('errors.internal'), field }) : ''
  useReportFieldError(error ? fieldKey : undefined, message, errorSeq(error))
  if (!error) return null
  const detail = apiErr?.detail ?? ''
  return (
    <Alert>
      {field ? (
        <>
          {fieldKey && scope?.label(fieldKey) ? (
            <button type="button" onClick={() => scope.focus(fieldKey)} className="font-semibold underline decoration-dotted underline-offset-2 hover:decoration-solid">
              {field}
            </button>
          ) : (
            <span className="font-semibold">{field}</span>
          )}
          {': '}
          {message}
        </>
      ) : (
        message
      )}
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
