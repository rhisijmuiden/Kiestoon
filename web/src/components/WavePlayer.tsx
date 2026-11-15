import { useEffect, useRef, useState, type KeyboardEvent, type MouseEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Loader2, Pause, Play } from 'lucide-react'

// Plays a voice prompt with its waveform. The audio is fetched once per prompt (the CSP only allows
// blob: media), decoded in the browser for the waveform, and only one player plays at a time.

type Clip = { url: string; peaks: number[]; seconds: number }

const BARS = 64
const cache = new Map<string, Promise<Clip>>()

async function loadClip(src: string): Promise<Clip> {
  const res = await fetch(src, { credentials: 'same-origin' })
  if (!res.ok) throw new Error(String(res.status))
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  let peaks: number[] = []
  let seconds = 0
  try {
    const AC = window.AudioContext || (window as unknown as { webkitAudioContext: typeof AudioContext }).webkitAudioContext
    const ctx = new AC()
    try {
      const buf = await ctx.decodeAudioData(await blob.arrayBuffer())
      seconds = buf.duration
      const data = buf.getChannelData(0)
      const step = Math.max(1, Math.floor(data.length / BARS))
      for (let i = 0; i < BARS; i++) {
        let sum = 0
        const end = Math.min(data.length, (i + 1) * step)
        for (let j = i * step; j < end; j++) sum += data[j] * data[j]
        peaks.push(Math.sqrt(sum / Math.max(1, end - i * step)))
      }
      const max = Math.max(0.0001, ...peaks)
      peaks = peaks.map((p) => p / max)
    } finally {
      ctx.close()
    }
  } catch {
    peaks = []
  }
  return { url, peaks, seconds }
}

export function promptSrc(id: number, version?: number) {
  return `/api/pbx/prompts/${id}/audio?v=${version ?? 0}`
}

function useClip(src: string | null) {
  const [clip, setClip] = useState<Clip | null>(null)
  const [error, setError] = useState(false)
  useEffect(() => {
    setClip(null)
    setError(false)
    if (!src) return
    let alive = true
    let p = cache.get(src)
    if (!p) {
      p = loadClip(src)
      cache.set(src, p)
      p.catch(() => cache.delete(src))
    }
    p.then((c) => alive && setClip(c)).catch(() => alive && setError(true))
    return () => { alive = false }
  }, [src])
  return { clip, error }
}

let current: HTMLAudioElement | null = null

const fmt = (s: number) => {
  const v = Math.max(0, Math.floor(s))
  return `${String(Math.floor(v / 60)).padStart(2, '0')}:${String(v % 60).padStart(2, '0')}`
}

const SPEEDS = [0.75, 1, 1.25, 1.5, 2]

/**
 * A compact player: play button, time, waveform (click to seek) and length. `size="lg"` adds a speed
 * button. `tone` is a text colour class that colours the button and the played part.
 */
export function WavePlayer({ src, size = 'sm', tone = 'text-accent', label }: { src: string | null; size?: 'sm' | 'lg'; tone?: string; label?: string }) {
  const { t } = useTranslation()
  const { clip, error } = useClip(src)
  const audio = useRef<HTMLAudioElement | null>(null)
  const [playing, setPlaying] = useState(false)
  const [pos, setPos] = useState(0)
  const [speed, setSpeed] = useState(1)

  useEffect(() => {
    if (!clip) return
    const a = new Audio(clip.url)
    a.preload = 'auto'
    a.playbackRate = speed
    const tick = () => setPos(a.currentTime)
    const stop = () => { setPlaying(false); setPos(0) }
    const pause = () => setPlaying(false)
    a.addEventListener('timeupdate', tick)
    a.addEventListener('ended', stop)
    a.addEventListener('pause', pause)
    audio.current = a
    return () => {
      a.pause()
      a.removeEventListener('timeupdate', tick)
      a.removeEventListener('ended', stop)
      a.removeEventListener('pause', pause)
      if (current === a) current = null
      audio.current = null
      setPlaying(false)
      setPos(0)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [clip])

  useEffect(() => { if (audio.current) audio.current.playbackRate = speed }, [speed])

  function toggle(e: MouseEvent) {
    e.stopPropagation()
    const a = audio.current
    if (!a) return
    if (!a.paused) {
      a.pause()
      return
    }
    if (current && current !== a) current.pause()
    current = a
    a.play().then(() => setPlaying(true)).catch(() => setPlaying(false))
  }
  const seconds = clip?.seconds ?? 0
  function seekTo(frac: number) {
    const a = audio.current
    if (!a || !seconds) return
    a.currentTime = Math.max(0, Math.min(seconds, frac * seconds))
    setPos(a.currentTime)
  }
  function seek(e: MouseEvent<HTMLDivElement>) {
    e.stopPropagation()
    const r = e.currentTarget.getBoundingClientRect()
    seekTo((e.clientX - r.left) / r.width)
  }
  function seekKey(e: KeyboardEvent<HTMLDivElement>) {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
    e.preventDefault()
    e.stopPropagation()
    seekTo((pos + (e.key === 'ArrowRight' ? 1 : -1)) / Math.max(seconds, 0.001))
  }

  const lg = size === 'lg'
  const bars = clip?.peaks.length ? clip.peaks : Array.from({ length: BARS }, () => 0.08)
  const shown = lg ? bars : bars.filter((_, i) => i % 2 === 0)
  const played = seconds ? pos / seconds : 0
  const btn = lg ? 'size-9' : 'size-7'

  return (
    <div className={`flex min-w-0 items-center gap-2 ${lg ? 'rounded-xl border border-line bg-inset px-2.5 py-2' : ''}`}>
      <button type="button" onClick={toggle} disabled={!clip} aria-label={`${playing ? t('audio.pause') : t('audio.play')}${label ? `: ${label}` : ''}`}
        className={`nodrag inline-flex ${btn} flex-none items-center justify-center rounded-full bg-current transition disabled:opacity-40 ${tone}`}>
        {!clip && !error && src ? <Loader2 className="size-3.5 animate-spin text-on-accent" aria-hidden />
          : playing ? <Pause className="size-3.5 fill-on-accent text-on-accent" aria-hidden />
            : <Play className="ml-0.5 size-3.5 fill-on-accent text-on-accent" aria-hidden />}
      </button>
      <span className="flex-none font-mono text-[10px] tabular-nums text-muted">{fmt(pos)}</span>
      <div role="slider" tabIndex={clip ? 0 : -1} aria-label={t('audio.position')} aria-valuemin={0} aria-valuemax={Math.round(seconds)} aria-valuenow={Math.round(pos)}
        onClick={seek} onKeyDown={seekKey}
        className={`nodrag flex min-w-0 flex-1 cursor-pointer items-center gap-[2px] ${lg ? 'h-8' : 'h-6'}`}>
        {shown.map((p, i) => (
          <span key={i} aria-hidden
            className={`block min-w-px max-w-[3px] flex-1 rounded-full ${i / shown.length < played ? 'bg-current' : 'bg-subtle/45'} ${tone}`}
            style={{ height: `${Math.max(8, Math.round(p * 100))}%` }} />
        ))}
      </div>
      <span className="flex-none font-mono text-[10px] tabular-nums text-muted">{fmt(seconds)}</span>
      {lg && (
        <button type="button" onClick={() => setSpeed(SPEEDS[(SPEEDS.indexOf(speed) + 1) % SPEEDS.length])}
          aria-label={t('audio.speed', { speed })}
          className="flex-none rounded-md border border-line px-1.5 py-0.5 font-mono text-[11px] text-ink-2 hover:bg-tint/5">
          {speed}×
        </button>
      )}
      {error && <span className="text-[10px] text-danger">!</span>}
    </div>
  )
}
