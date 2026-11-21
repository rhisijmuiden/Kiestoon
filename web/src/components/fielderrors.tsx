import { createContext, useCallback, useContext, useEffect, useId, useMemo, useRef, useState, type ReactNode, type RefObject } from 'react'

/**
 * Field errors link an API error ({ error, field }) to the form field it is about.
 *
 * A scope wraps a form area: the whole app has one, and every Modal adds its own. An ErrorMessage
 * inside a scope reports the field of its error; the Input, Select, Textarea or Toggle with the
 * same `name` in that scope turns red, shows the message under itself and takes the focus. The
 * fields register their label, so the banner can name the field exactly as the form shows it.
 */

type Report = { field: string; message: string; seq: number }

type Scope = {
  reports: Record<string, Report>
  dismissed: Record<string, number>
  report: (id: string, r: Report | null) => void
  dismiss: (field: string) => void
  register: (name: string, label: string, el: RefObject<HTMLElement | null>) => () => void
  label: (field: string) => string | undefined
  focus: (field: string) => void
  claimFocus: (seq: number) => boolean
}

const FieldScopeContext = createContext<Scope | null>(null)

let seqCounter = 0
export const nextErrorSeq = () => ++seqCounter

export function FieldErrorScope({ children }: { children: ReactNode }) {
  const [reports, setReports] = useState<Record<string, Report>>({})
  const [dismissed, setDismissed] = useState<Record<string, number>>({})
  const fields = useRef(new Map<string, { label: string; el: RefObject<HTMLElement | null> }[]>())
  const focused = useRef(0)

  const report = useCallback((id: string, r: Report | null) => {
    setReports((cur) => {
      if (!r) {
        if (!(id in cur)) return cur
        const next = { ...cur }
        delete next[id]
        return next
      }
      if (cur[id]?.seq === r.seq) return cur
      return { ...cur, [id]: r }
    })
  }, [])
  const dismiss = useCallback((field: string) => {
    setDismissed((cur) => (cur[field] === seqCounter ? cur : { ...cur, [field]: seqCounter }))
  }, [])
  const register = useCallback((name: string, label: string, el: RefObject<HTMLElement | null>) => {
    const entry = { label, el }
    const list = fields.current.get(name) ?? []
    list.push(entry)
    fields.current.set(name, list)
    return () => {
      const l = fields.current.get(name)?.filter((x) => x !== entry) ?? []
      if (l.length) fields.current.set(name, l)
      else fields.current.delete(name)
    }
  }, [])
  const label = useCallback((field: string) => fields.current.get(field)?.[0]?.label, [])
  const focus = useCallback((field: string) => {
    const el = fields.current.get(field)?.[0]?.el.current
    if (!el) return
    el.scrollIntoView({ block: 'center', behavior: 'smooth' })
    el.focus({ preventScroll: true })
  }, [])
  const claimFocus = useCallback((seq: number) => {
    if (focused.current >= seq) return false
    focused.current = seq
    return true
  }, [])

  const value = useMemo(
    () => ({ reports, dismissed, report, dismiss, register, label, focus, claimFocus }),
    [reports, dismissed, report, dismiss, register, label, focus, claimFocus],
  )
  return <FieldScopeContext.Provider value={value}>{children}</FieldScopeContext.Provider>
}

export const useFieldScope = () => useContext(FieldScopeContext)

/** Reports the field of an error to the nearest scope while the error is shown. */
export function useReportFieldError(field: string | undefined, message: string, seq: number) {
  const scope = useFieldScope()
  const id = useId()
  const report = scope?.report
  useEffect(() => {
    if (!report) return
    report(id, field ? { field, message, seq } : null)
    return () => report(id, null)
  }, [report, id, field, message, seq])
}

/**
 * Connects one form field to the scope: returns the error message for it (or undefined) and an
 * onChange wrapper that clears the red state as soon as the user edits the field.
 */
export function useFieldError(name: string | undefined, label: string, el: RefObject<HTMLElement | null>) {
  const scope = useFieldScope()
  const register = scope?.register
  useEffect(() => {
    if (!register || !name) return
    return register(name, label, el)
  }, [register, name, label, el])

  let hit: Report | undefined
  if (scope && name) {
    for (const r of Object.values(scope.reports)) {
      if (r.field === name && (scope.dismissed[name] ?? 0) < r.seq && (!hit || r.seq > hit.seq)) hit = r
    }
  }
  const seq = hit?.seq ?? 0
  const claimFocus = scope?.claimFocus
  useEffect(() => {
    if (!seq || !claimFocus || !claimFocus(seq)) return
    const node = el.current
    if (!node) return
    node.scrollIntoView({ block: 'center', behavior: 'smooth' })
    node.focus({ preventScroll: true })
  }, [seq, claimFocus, el])

  const dismiss = scope?.dismiss
  const clear = useCallback(() => {
    if (name && hit && dismiss) dismiss(name)
  }, [name, hit, dismiss])
  return { message: hit?.message, clear }
}

/** Message shown under an invalid field. */
export function FieldErrorText({ id, children }: { id: string; children: ReactNode }) {
  return (
    <p id={id} className="text-xs font-medium text-danger-ink">
      {children}
    </p>
  )
}

/** Extra classes for a field with an error: red border and ring in both themes. */
export const invalidCls = 'border-danger/70 ring-2 ring-danger/20 focus:border-danger focus:ring-danger/25'
