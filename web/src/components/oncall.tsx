import { useTranslation } from 'react-i18next'
import type { OnCallSegment, OnCallUser } from '../lib/types'

/** Every engineer gets their own colour in the schedule. */
const palette = ['#22D3EE', '#818CF8', '#F472B6', '#34D399', '#FBBF24', '#FB923C', '#A78BFA', '#2DD4BF', '#F87171', '#60A5FA']
export const userColor = (id: number) => palette[Math.abs(id) % palette.length]

export function initials(name: string) {
  const p = name.trim().split(/\s+/).filter(Boolean)
  if (p.length === 0) return '?'
  return ((p[0][0] ?? '') + (p.length > 1 ? p[p.length - 1][0] : (p[0][1] ?? ''))).toUpperCase()
}

export function Avatar({ user, size = 'sm' }: { user?: OnCallUser; size?: 'sm' | 'lg' }) {
  const c = user ? userColor(user.id) : '#64748B'
  const cls = size === 'lg' ? 'size-12 rounded-[13px] text-base' : 'size-8 rounded-[9px] text-xs'
  return (
    <span className={`inline-flex flex-none items-center justify-center font-display font-semibold ${cls}`} style={{ background: `${c}26`, color: c }} aria-hidden>
      {user ? initials(user.name) : '?'}
    </span>
  )
}

export function fmtWhen(iso: string, lang: string) {
  return new Intl.DateTimeFormat(lang, { weekday: 'short', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' }).format(new Date(iso))
}
export const fmtRange = (a: string, b: string, lang: string) => `${fmtWhen(a, lang)} → ${fmtWhen(b, lang)}`

/** Value for a datetime-local input (local time of the browser). */
export function toLocalInput(iso: string) {
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`
}
export const fromLocalInput = (v: string) => (v ? new Date(v).toISOString() : '')

/** The schedule as lanes (1st line, backup) over a number of days, with the current time. */
export function OnCallTimeline({ segments, users, from, days, now, showBackup = true }: {
  segments: OnCallSegment[]; users: Map<number, OnCallUser>; from: Date; days: number; now: Date; showBackup?: boolean
}) {
  const { t, i18n } = useTranslation()
  const start = from.getTime()
  const span = days * 86400000
  const pct = (iso: string | Date) => Math.min(100, Math.max(0, ((new Date(iso).getTime() - start) / span) * 100))
  const nowPct = pct(now)
  const nowIn = now.getTime() >= start && now.getTime() <= start + span
  const dayStarts = Array.from({ length: days }, (_, i) => { const d = new Date(from); d.setDate(d.getDate() + i); return d })
  const fmtDay = new Intl.DateTimeFormat(i18n.language, { weekday: 'short', day: 'numeric' })
  const lanes: { line: 1 | 2; label: string }[] = [{ line: 1, label: t('onCall.line1') }]
  if (showBackup) lanes.push({ line: 2, label: t('onCall.line2') })
  const grid = (
    <>
      {dayStarts.map((d, i) => (
        <span key={i} className={`absolute inset-y-0 border-l border-line ${d.getDay() === 0 || d.getDay() === 6 ? 'bg-tint/[0.025]' : ''}`}
          style={{ left: `${pct(d)}%`, width: `${100 / days}%` }} aria-hidden />
      ))}
    </>
  )
  return (
    <div className="overflow-x-auto">
      <div className="min-w-[720px]">
        <div className="grid grid-cols-[110px_1fr] text-[11px] text-subtle">
          <span />
          <div className="relative h-6">
            {dayStarts.map((d, i) => (
              <span key={i} className="absolute top-0 whitespace-nowrap pl-1.5 font-medium text-ink-2" style={{ left: `${pct(d)}%` }}>{fmtDay.format(d)}</span>
            ))}
            {nowIn && <span className="absolute bottom-0 h-1.5 w-0.5 bg-accent" style={{ left: `${nowPct}%` }} aria-hidden />}
          </div>
        </div>
        {lanes.map((ln) => (
          <div key={ln.line} className="grid min-h-11 grid-cols-[110px_1fr] items-center border-t border-line">
            <span className="pr-2 text-xs text-muted">{ln.label}</span>
            <div className="relative h-11 overflow-hidden">
              {grid}
              {segments.filter((s) => s.line === ln.line).map((s, i) => {
                const l = pct(s.start), w = pct(s.end) - l
                if (w <= 0) return null
                const u = users.get(s.userId)
                const gap = s.kind === 'gap' || !u
                const c = u ? userColor(u.id) : ''
                const changed = s.kind !== 'rotation' && !gap
                const title = `${gap ? t('onCall.gap') : u?.name} · ${fmtRange(s.start, s.end, i18n.language)}${changed ? ` · ${t(`onCall.kinds.${s.kind}`)}` : ''}`
                return (
                  <span key={i} title={title}
                    className={`absolute top-2 flex h-7 items-center overflow-hidden whitespace-nowrap rounded-[7px] border px-1.5 text-[11px] font-medium ${gap ? 'border-dashed border-danger bg-danger/15 text-danger-ink' : 'text-ink'}`}
                    style={{
                      left: `${l}%`, width: `calc(${w}% - 2px)`,
                      ...(gap ? {} : { background: `${c}2e`, borderColor: `${c}80` }),
                      ...(changed ? { backgroundImage: 'repeating-linear-gradient(135deg, transparent 0 6px, rgb(from var(--color-tint) r g b / 0.14) 6px 9px)' } : {}),
                    }}>
                    {gap ? '!' : u?.name.split(' ')[0]}
                  </span>
                )
              })}
              {nowIn && <span className="absolute inset-y-0 w-0.5 bg-accent shadow-[0_0_8px_var(--color-accent)]" style={{ left: `${nowPct}%` }} aria-hidden />}
            </div>
          </div>
        ))}
        <div className="mt-3 flex flex-wrap gap-4 text-xs text-muted">
          <span className="inline-flex items-center gap-1.5"><span className="h-2.5 w-4 rounded-[3px] border border-accent bg-accent/25" />{t('onCall.legendRotation')}</span>
          <span className="inline-flex items-center gap-1.5"><span className="h-2.5 w-4 rounded-[3px] border border-[#F472B680] bg-[#F472B62e]" style={{ backgroundImage: 'repeating-linear-gradient(135deg, transparent 0 3px, rgb(255 255 255 / 0.25) 3px 5px)' }} />{t('onCall.legendChange')}</span>
          <span className="inline-flex items-center gap-1.5"><span className="h-2.5 w-4 rounded-[3px] border border-dashed border-danger bg-danger/15" />{t('onCall.legendGap')}</span>
          <span className="inline-flex items-center gap-1.5"><span className="h-2.5 w-0.5 bg-accent" />{t('onCall.legendNow')}</span>
        </div>
      </div>
    </div>
  )
}
