import { useTranslation } from 'react-i18next'
import type { QualityLimits } from '../lib/types'

// Shared pieces of the call quality screens: the MOS pill, number formatting, the MOS chart and
// the timeline strip of a call leg.

export type Rating = 'good' | 'fair' | 'poor'

export function rating(mos: number, l: QualityLimits): Rating {
  return mos >= l.good ? 'good' : mos >= l.poor ? 'fair' : 'poor'
}

const pill: Record<Rating, string> = {
  good: 'border-ok/35 bg-ok/10 text-ok-ink',
  fair: 'border-warn/35 bg-warn/10 text-warn-ink',
  poor: 'border-danger/40 bg-danger/10 text-danger-ink',
}
const dot: Record<Rating, string> = { good: 'bg-ok', fair: 'bg-warn', poor: 'bg-danger' }
export const ratingText: Record<Rating, string> = { good: 'text-ok-ink', fair: 'text-warn-ink', poor: 'text-danger-ink' }
export const ratingFill: Record<Rating, string> = { good: 'bg-ok/65', fair: 'bg-warn/75', poor: 'bg-danger/80' }

/** A number with a fixed number of decimals in the user's language (comma in Dutch). */
export function num(v: number, digits: number, lang: string) {
  return v.toLocaleString(lang, { minimumFractionDigits: digits, maximumFractionDigits: digits })
}

export function MosPill({ mos, limits, label }: { mos: number; limits: QualityLimits; label?: string }) {
  const { t, i18n } = useTranslation()
  if (!mos) return <span className="text-xs text-subtle">{t('quality.noAudio')}</span>
  const r = rating(mos, limits)
  return (
    <span className={`inline-flex items-center gap-1.5 whitespace-nowrap rounded-full border px-2.5 py-0.5 font-mono text-xs font-medium ${pill[r]}`}
      title={t(`quality.rating.${r}`)}>
      <span className={`size-1.5 rounded-full ${dot[r]}`} aria-hidden />
      {label ? `${label} ` : ''}{num(mos, 2, i18n.language)}
      <span className="sr-only"> ({t(`quality.rating.${r}`)})</span>
    </span>
  )
}

/** A value that turns red when it reaches its limit. */
export function Over({ value, limit, unit, digits = 0 }: { value: number; limit: number; unit: string; digits?: number }) {
  const { i18n } = useTranslation()
  return <span className={value >= limit ? 'text-danger-ink' : ''}>{num(value, digits, i18n.language)}{unit}</span>
}

export const seriesColors = ['var(--color-accent)', 'var(--color-accent-2)', '#F472B6', 'var(--color-ok)', 'var(--color-warn)']

/** MOS per time bucket, one line per series, with the poor range shaded. */
export function MosChart({ series, labels, limits }: { series: { name: string; values: number[] }[]; labels: string[]; limits: QualityLimits }) {
  const { t } = useTranslation()
  const W = 640, H = 230, L = 34, R = 630, T = 10, B = 200
  const lo = 2.5, hi = 4.5
  const y = (v: number) => B - ((Math.max(lo, Math.min(hi, v)) - lo) / (hi - lo)) * (B - T)
  const n = Math.max(1, labels.length - 1)
  const x = (i: number) => L + (i * (R - L)) / n
  const every = Math.ceil(labels.length / 8)
  return (
    <div className="overflow-x-auto">
      <svg viewBox={`0 0 ${W} ${H}`} className="w-full min-w-[480px]" role="img" aria-label={t('quality.chartTitle')}>
        <rect x={L} y={y(limits.poor)} width={R - L} height={B - y(limits.poor)} fill="var(--color-danger)" opacity={0.09} />
        {[2.5, 3, 3.5, 4, 4.5].map((v) => (
          <g key={v}>
            <line x1={L} x2={R} y1={y(v)} y2={y(v)} stroke="var(--color-line)" />
            <text x={L - 6} y={y(v) + 3} textAnchor="end" className="fill-subtle font-mono text-[10px]">{v.toFixed(1)}</text>
          </g>
        ))}
        <line x1={L} x2={R} y1={y(limits.poor)} y2={y(limits.poor)} stroke="var(--color-danger)" strokeDasharray="4 4" opacity={0.6} />
        {labels.map((lb, i) => (i % every === 0 || i === labels.length - 1) && (
          <text key={i} x={x(i)} y={B + 16} textAnchor="middle" className="fill-subtle font-mono text-[10px]">{lb}</text>
        ))}
        {series.map((s, si) => {
          let d = ''
          let on = false
          s.values.forEach((v, i) => {
            if (!v) { on = false; return }
            d += `${on ? 'L' : 'M'}${x(i).toFixed(1)} ${y(v).toFixed(1)} `
            on = true
          })
          const last = s.values.reduce((a, v, i) => (v ? i : a), -1)
          const c = seriesColors[si % seriesColors.length]
          return (
            <g key={s.name}>
              <path d={d} fill="none" stroke={c} strokeWidth={2.2} strokeLinejoin="round" strokeLinecap="round" />
              {s.values.map((v, i) => v ? (
                <circle key={i} cx={x(i)} cy={y(v)} r={i === last ? 3.6 : 2} fill={c} stroke="var(--color-surface)" strokeWidth={i === last ? 1.5 : 0}>
                  <title>{`${s.name} · ${labels[i]} · MOS ${v.toFixed(2)}`}</title>
                </circle>
              ) : null)}
            </g>
          )
        })}
      </svg>
    </div>
  )
}

export function Legend({ names }: { names: string[] }) {
  return (
    <span className="flex flex-wrap gap-3.5 text-[12.5px] text-muted">
      {names.map((n, i) => (
        <span key={n} className="inline-flex items-center gap-1.5">
          <i className="inline-block h-[3px] w-3.5 rounded" style={{ background: seriesColors[i % seriesColors.length] }} aria-hidden />{n}
        </span>
      ))}
    </span>
  )
}

/** A small line of the MOS per bucket. */
export function Spark({ values, color }: { values: number[]; color: string }) {
  const W = 120, H = 26
  const y = (v: number) => H - 3 - ((Math.max(3, Math.min(4.5, v)) - 3) / 1.5) * (H - 6)
  const n = Math.max(1, values.length - 1)
  let d = ''
  let on = false
  values.forEach((v, i) => {
    if (!v) { on = false; return }
    d += `${on ? 'L' : 'M'}${((i * W) / n).toFixed(1)} ${y(v).toFixed(1)} `
    on = true
  })
  const last = values.reduce((a, v, i) => (v ? i : a), -1)
  return (
    <svg viewBox={`-3 0 ${W + 6} ${H}`} width={W} height={H} aria-hidden className="overflow-visible">
      <path d={d} fill="none" stroke={color} strokeWidth={1.6} />
      {last >= 0 && <circle cx={(last * W) / n} cy={y(values[last])} r={2.4} fill={color} />}
    </svg>
  )
}

/** The MOS of each report interval of a call leg as coloured blocks. */
export function Strip({ samples, limits, seconds }: { samples: number[]; limits: QualityLimits; seconds: number }) {
  const { t } = useTranslation()
  if (samples.length === 0) return null
  const fmt = (s: number) => `${Math.floor(s / 60)}:${String(Math.round(s % 60)).padStart(2, '0')}`
  return (
    <div className="mt-2.5">
      <div className="flex h-[22px] gap-[2px]" role="img" aria-label={t('quality.stripLabel')}>
        {samples.map((s, i) => <i key={i} className={`min-w-px flex-1 rounded-[2px] ${ratingFill[rating(s / 100, limits)]}`} title={`MOS ${(s / 100).toFixed(2)}`} />)}
      </div>
      <div className="mt-1 flex justify-between font-mono text-[10.5px] text-subtle">
        <span>0:00</span><span>{fmt(seconds / 2)}</span><span>{fmt(seconds)}</span>
      </div>
    </div>
  )
}
