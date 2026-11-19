import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2, Mic, Square, Upload } from 'lucide-react'
import { Select } from './kit'
import { Button, Input } from './ui'
import { api } from '../lib/api'
import { startRecording, toTelephoneWav, type Wav } from '../lib/audio'
import type { Destination, DestType, Destinations, NumberPlan } from '../lib/types'

/** A small square icon button for table rows. */
export function IconButton({ label, onClick, danger, children, disabled }: {
  label: string; onClick: () => void; danger?: boolean; children: ReactNode; disabled?: boolean
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={label}
      aria-label={label}
      disabled={disabled}
      className={`inline-flex size-10 items-center justify-center rounded-lg text-muted transition disabled:opacity-40 ${danger ? 'hover:bg-danger/10 hover:text-danger' : 'hover:bg-tint/5 hover:text-ink'}`}
    >
      {children}
    </button>
  )
}

/** A row of mutually exclusive choices. */
export function Segmented<T extends string>({ label, options, value, onChange }: {
  label?: string; options: { key: T; label: string; tone?: 'ok' | 'danger' | 'accent' }[]; value: T; onChange: (v: T) => void
}) {
  const tone = { ok: 'bg-ok/15 text-ok-ink shadow-[inset_0_0_0_1px_rgb(52_211_153_/_0.4)]', danger: 'bg-danger/15 text-danger-ink shadow-[inset_0_0_0_1px_rgb(248_113_113_/_0.45)]', accent: 'bg-accent/15 text-ink shadow-[inset_0_0_0_1px_rgb(34_211_238_/_0.35)]' }
  return (
    <div className="flex flex-col gap-1.5">
      {label && <span className="text-sm font-medium text-ink-2">{label}</span>}
      <div role="radiogroup" aria-label={label} className="inline-flex w-max max-w-full flex-nowrap gap-1 overflow-x-auto rounded-xl border border-line bg-surface-2/70 p-1">
        {options.map((o) => (
          <button
            key={o.key}
            type="button"
            role="radio"
            aria-checked={value === o.key}
            onClick={() => onChange(o.key)}
            className={`min-h-9 whitespace-nowrap rounded-lg px-3 text-sm transition ${value === o.key ? tone[o.tone ?? 'accent'] : 'text-muted hover:text-ink'}`}
          >
            {o.label}
          </button>
        ))}
      </div>
    </div>
  )
}

/** Saves an object with POST (new) or PUT (existing) and reports busy and error state. */
export function useSaver<T extends { id: number }>(base: string, onSaved: (v: T) => void) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  async function save(v: T) {
    setBusy(true)
    setError(null)
    try {
      onSaved(await api<T>(v.id ? 'PUT' : 'POST', v.id ? `${base}/${v.id}` : base, v))
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return { busy, error, save, setError }
}

let destCache: Promise<Destinations> | null = null
/** Loads every possible call destination once per page load (reset after a save). */
export function useDestinations() {
  const [d, setD] = useState<Destinations | null>(null)
  useEffect(() => {
    destCache ??= api<Destinations>('GET', '/pbx/destinations').catch((e) => { destCache = null; throw e })
    destCache.then(setD).catch(() => undefined)
  }, [])
  return d
}
export const resetDestinations = () => { destCache = null }

/** Picks where a call goes: a type and, depending on the type, an object or a phone number. */
export function DestinationPicker({ label, value, onChange, allowNone, exclude }: {
  label: string; value: Destination; onChange: (d: Destination) => void; allowNone?: boolean; exclude?: { type: DestType; target: string }
}) {
  const { t } = useTranslation()
  const d = useDestinations()
  const types = d?.types ?? ['extension', 'voicemail', 'external', 'hangup']
  const options = (d?.options ?? []).filter((o) => o.type === value.type && !(exclude && o.type === exclude.type && o.target === exclude.target))
  const needsTarget = value.type !== '' && value.type !== 'hangup'
  return (
    <div className="grid gap-2 sm:grid-cols-[minmax(0,200px)_1fr]">
      <Select
        label={label}
        value={value.type}
        onChange={(e) => {
          const type = e.target.value as DestType | ''
          const first = (d?.options ?? []).find((o) => o.type === type)
          onChange({ type, target: type === 'external' || type === 'hangup' || type === '' ? '' : first?.target ?? '' })
        }}
      >
        {allowNone && <option value="">{t('dest.none')}</option>}
        {types.map((ty) => <option key={ty} value={ty}>{t(`dest.types.${ty}`)}</option>)}
      </Select>
      {needsTarget && (value.type === 'external' ? (
        <Input name="target" label={t('dest.number')} value={value.target} onChange={(e) => onChange({ ...value, target: e.target.value.replace(/[^0-9+]/g, '') })} inputMode="tel" className="font-mono" placeholder="0612345678" />
      ) : (
        <Select name="target" label={t('dest.target')} value={value.target} onChange={(e) => onChange({ ...value, target: e.target.value })}>
          {!options.some((o) => o.target === value.target) && <option value={value.target}>{value.target ? value.target : t('dest.choose')}</option>}
          {options.map((o) => <option key={o.target} value={o.target}>{o.label}</option>)}
        </Select>
      ))}
    </div>
  )
}

/** Describes a destination in a table cell. */
export function DestinationLabel({ value }: { value: Destination }) {
  const { t } = useTranslation()
  const d = useDestinations()
  if (!value.type) return <span className="text-subtle">–</span>
  const label = d?.options.find((o) => o.type === value.type && o.target === value.target)?.label ?? value.target
  return (
    <span className="inline-flex items-center gap-1.5 whitespace-nowrap">
      <span className="text-xs text-subtle">{t(`dest.types.${value.type}`)}</span>
      {value.type !== 'hangup' && <span className="font-mono text-ink">{label}</span>}
    </span>
  )
}

/** Uploads an audio file or records one with the microphone and converts it for the phone system. */
export function AudioInput({ value, onChange }: { value: Wav | null; onChange: (w: Wav | null) => void }) {
  const { t } = useTranslation()
  const file = useRef<HTMLInputElement>(null)
  const rec = useRef<{ stop: () => Promise<Blob> } | null>(null)
  const [recording, setRecording] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  async function convert(b: Blob) {
    setBusy(true)
    setError('')
    try {
      const w = await toTelephoneWav(b)
      if (w.seconds < 0.3) throw new Error('short')
      onChange(w)
    } catch {
      setError(t('audio.convertFailed'))
    } finally {
      setBusy(false)
    }
  }
  async function toggleRecord() {
    if (recording) {
      setRecording(false)
      const r = rec.current
      rec.current = null
      if (r) convert(await r.stop())
      return
    }
    try {
      rec.current = await startRecording()
      setRecording(true)
      setError('')
    } catch {
      setError(t('audio.micDenied'))
    }
  }
  return (
    <div className="flex flex-col gap-3 rounded-xl border border-line bg-surface-2/60 p-4">
      <div className="flex flex-wrap gap-2">
        <input ref={file} type="file" accept="audio/*" className="hidden" onChange={(e) => { const f = e.target.files?.[0]; if (f) convert(f); e.target.value = '' }} />
        <Button variant="secondary" onClick={() => file.current?.click()} disabled={recording || busy}>
          <Upload className="size-4" aria-hidden /> {t('audio.upload')}
        </Button>
        <Button variant={recording ? 'danger' : 'ghost'} onClick={toggleRecord} disabled={busy}>
          {recording ? <Square className="size-4" aria-hidden /> : <Mic className="size-4" aria-hidden />}
          {recording ? t('audio.stop') : t('audio.record')}
        </Button>
        {recording && <span className="inline-flex items-center gap-2 text-sm text-danger"><span className="size-2 animate-pulse rounded-full bg-danger" aria-hidden />{t('audio.recording')}</span>}
        {busy && <span className="inline-flex items-center gap-2 text-sm text-muted"><Loader2 className="size-4 animate-spin" aria-hidden />{t('audio.converting')}</span>}
      </div>
      {value && (
        <div className="flex flex-wrap items-center gap-3">
          <audio controls src={value.url} className="h-10 max-w-full" />
          <span className="font-mono text-xs text-subtle">{value.seconds.toFixed(1)} s · 8 kHz</span>
          <button type="button" onClick={() => onChange(null)} className="text-xs text-muted underline hover:text-ink">{t('audio.discard')}</button>
        </div>
      )}
      {error && <p className="text-sm text-danger">{error}</p>}
      <p className="text-xs text-subtle">{t('audio.hint')}</p>
    </div>
  )
}

/** Colours for a live extension state. */
export function stateTone(state: string): 'ok' | 'warn' | 'danger' | 'muted' | 'accent' {
  switch (state) {
    case 'idle': return 'ok'
    case 'ringing': case 'ringinuse': return 'accent'
    case 'inuse': case 'busy': case 'onhold': return 'danger'
    case 'unavailable': return 'muted'
  }
  return 'muted'
}

/** Comma or newline separated text to a list. */
export const splitList = (s: string) => s.split(/[\s,;]+/).map((x) => x.trim()).filter(Boolean)

/** Suggests the first free number in a range of the number plan. */
export async function nextNumber(purpose: 'ringGroups' | 'queues' | 'conferences', used: string[]): Promise<string> {
  try {
    const p = await api<{ current: NumberPlan }>('GET', '/pbx/numberplan')
    const r = p.current.ranges.find((x) => x.purpose === purpose)
    if (!r) return ''
    const taken = new Set(used)
    for (let n = Number(r.from) + 1; n <= Number(r.to); n++) {
      const s = String(n)
      if (!taken.has(s)) return s
    }
  } catch {
    /* leave empty */
  }
  return ''
}
