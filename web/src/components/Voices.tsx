import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { Check, Loader2, Pause, Play, Search, Sparkles } from 'lucide-react'
import { Modal, Select, Tabs, useApi } from './kit'
import { Button, ErrorMessage } from './ui'
import { api } from '../lib/api'
import { wavFromBase64 } from '../lib/audio'
import type { ElevenData, ElevenVoice, SpeechData, TTSResult, VoiceRef } from '../lib/types'

// ElevenLabs voices: a voice chip with a preview button, and the voice picker (account voices and
// the public voice library, filtered on the PBX language by default). Samples are fetched through
// Kiestoon as blobs, because the CSP allows no other media hosts.

const GRADIENTS = [
  ['#22D3EE', '#818CF8'], ['#34D399', '#22D3EE'], ['#F472B6', '#818CF8'], ['#FBBF24', '#F87171'],
  ['#A78BFA', '#22D3EE'], ['#60A5FA', '#34D399'],
]

function gradientOf(id: string) {
  let h = 0
  for (const c of id) h = (h * 31 + c.charCodeAt(0)) >>> 0
  const [a, b] = GRADIENTS[h % GRADIENTS.length]
  return `linear-gradient(135deg, ${a}, ${b})`
}

export function VoiceAvatar({ voice, size = 'md' }: { voice: { id: string; name: string }; size?: 'sm' | 'md' }) {
  return (
    <span aria-hidden style={{ background: gradientOf(voice.id || voice.name) }}
      className={`inline-flex flex-none items-center justify-center rounded-full font-display font-semibold text-[#06121F] ${size === 'sm' ? 'size-8 text-xs' : 'size-10 text-sm'}`}>
      {(voice.name || '?').slice(0, 1).toUpperCase()}
    </span>
  )
}

// One sample plays at a time.
let playing: { audio: HTMLAudioElement; key: string; stop: () => void } | null = null
const samples = new Map<string, Promise<string>>()

function sampleURL(id: string, language?: string) {
  const key = `${id}/${language ?? ''}`
  let p = samples.get(key)
  if (!p) {
    p = fetch(`/api/speech/elevenlabs/voices/${encodeURIComponent(id)}/preview${language ? `?language=${language}` : ''}`, { credentials: 'same-origin' })
      .then(async (r) => {
        if (!r.ok) throw new Error(String(r.status))
        return URL.createObjectURL(await r.blob())
      })
    samples.set(key, p)
    p.catch(() => samples.delete(key))
  }
  return p
}

/** A round play button for the free sample of a voice. */
export function PreviewButton({ id, language, label }: { id: string; language?: string; label: string }) {
  const [state, setState] = useState<'idle' | 'loading' | 'playing' | 'error'>('idle')
  const key = `${id}/${language ?? ''}`
  useEffect(() => () => { if (playing?.key === key) playing.stop() }, [key])
  async function toggle() {
    if (state === 'playing') {
      playing?.stop()
      return
    }
    playing?.stop()
    setState('loading')
    try {
      const url = await sampleURL(id, language)
      const audio = new Audio(url)
      const stop = () => { audio.pause(); setState('idle'); if (playing?.audio === audio) playing = null }
      audio.addEventListener('ended', stop)
      playing = { audio, key, stop }
      await audio.play()
      setState('playing')
    } catch {
      setState('error')
    }
  }
  const on = state === 'playing'
  return (
    <button type="button" onClick={toggle} aria-label={label} aria-pressed={on} title={label}
      className={`inline-flex size-9 flex-none items-center justify-center rounded-full border transition ${on ? 'border-accent bg-accent text-on-accent' : state === 'error' ? 'border-danger/50 text-danger' : 'border-accent/40 bg-accent/10 text-accent-ink hover:bg-accent/20'}`}>
      {state === 'loading' ? <Loader2 className="size-4 animate-spin" aria-hidden /> : on ? <Pause className="size-3.5 fill-current" aria-hidden /> : <Play className="size-3.5 fill-current" aria-hidden />}
    </button>
  )
}

/** The chosen voice with its sample and a button to choose another. */
export function VoiceBox({ voice, language, onChange, compact }: { voice: VoiceRef; language: 'nl' | 'en'; onChange: (v: VoiceRef) => void; compact?: boolean }) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  return (
    <div className={`flex min-w-0 items-center gap-3 rounded-xl border border-line bg-inset ${compact ? 'px-2.5 py-2' : 'px-3 py-2.5'}`}>
      {voice.id ? (
        <>
          <VoiceAvatar voice={voice} size={compact ? 'sm' : 'md'} />
          <span className="min-w-0 flex-1">
            <span className="block truncate font-medium text-ink">{voice.name || voice.id}</span>
            <span className="block truncate text-xs text-subtle">{compact ? t(`voices.lang.${language}`) : `ElevenLabs · ${t(`voices.lang.${language}`)}`}</span>
          </span>
          <PreviewButton id={voice.id} language={language} label={t('voices.preview', { name: voice.name })} />
        </>
      ) : (
        <span className="min-w-0 flex-1 text-sm text-muted">{t('voices.none')}</span>
      )}
      <Button variant="ghost" onClick={() => setOpen(true)}>{voice.id ? t('voices.change') : t('voices.choose')}</Button>
      {open && <VoicePicker language={language} current={voice} onClose={() => setOpen(false)} onChoose={(v) => { setOpen(false); onChange(v) }} />}
    </div>
  )
}

const LANGS = ['nl', 'en', 'de', 'fr', '']

/** Chooses an ElevenLabs voice. Library voices are added to the account when chosen. */
export function VoicePicker({ language, current, onClose, onChoose }: { language: 'nl' | 'en'; current?: VoiceRef; onClose: () => void; onChoose: (v: VoiceRef) => void }) {
  const { t } = useTranslation()
  const el = useApi<ElevenData>('/speech/elevenlabs')
  const pbxLang = el.data?.pbxLanguage ?? language
  const [source, setSource] = useState<'library' | 'mine'>('library')
  const [lang, setLang] = useState<string>(language)
  const [gender, setGender] = useState('')
  const [search, setSearch] = useState('')
  const [q, setQ] = useState('')
  const [useCase, setUseCase] = useState('')
  const [accent, setAccent] = useState('')
  const [page, setPage] = useState(0)
  const [voices, setVoices] = useState<ElevenVoice[]>([])
  const [hasMore, setHasMore] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [chosen, setChosen] = useState<ElevenVoice | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    const id = window.setTimeout(() => setQ(search.trim()), 350)
    return () => window.clearTimeout(id)
  }, [search])
  useEffect(() => { setPage(0) }, [source, lang, gender, q])
  useEffect(() => {
    let alive = true
    setLoading(true)
    setError(null)
    const params = new URLSearchParams({ source, page: String(page) })
    if (lang) params.set('language', lang)
    if (gender) params.set('gender', gender)
    if (q) params.set('search', q)
    api<{ voices: ElevenVoice[]; hasMore: boolean }>('GET', `/speech/elevenlabs/voices?${params}`)
      .then((r) => { if (alive) { setVoices((v) => (page > 0 ? [...v, ...r.voices] : r.voices)); setHasMore(r.hasMore) } })
      .catch((e) => { if (alive) { setError(e); if (page === 0) setVoices([]) } })
      .finally(() => { if (alive) setLoading(false) })
    return () => { alive = false }
  }, [source, lang, gender, q, page])

  // Use case and accent filter the loaded voices: their values differ per language.
  const useCases = useMemo(() => [...new Set(voices.map((v) => v.useCase).filter(Boolean) as string[])].sort(), [voices])
  const accents = useMemo(() => [...new Set(voices.map((v) => v.accent).filter(Boolean) as string[])].sort(), [voices])
  const shown = voices.filter((v) => (!useCase || v.useCase === useCase) && (!accent || v.accent === accent))
  const selected = chosen ?? voices.find((v) => v.id === current?.id) ?? null

  async function use() {
    if (!selected) return
    setBusy(true)
    setError(null)
    try {
      if (selected.mine || !selected.ownerId) {
        onChoose({ id: selected.id, name: selected.name })
      } else {
        onChoose(await api<VoiceRef>('POST', '/speech/elevenlabs/voices', { ownerId: selected.ownerId, voiceId: selected.id, name: selected.name }))
      }
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal wide title={t('voices.title')} subtitle={t('voices.subtitle')} onClose={onClose}
      footer={<>
        <span className="mr-auto self-center text-xs text-subtle">{t('voices.footHint')}</span>
        <Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button>
        <Button busy={busy} disabled={!selected} onClick={use}><Check className="size-4" aria-hidden />{selected ? t('voices.use', { name: selected.name }) : t('voices.choose')}</Button>
      </>}>
      <Tabs tabs={[{ key: 'library', label: t('voices.library') }, { key: 'mine', label: t('voices.mine') }]} value={source} onChange={setSource} />
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <div className="flex flex-col gap-1.5 sm:col-span-2 lg:col-span-1">
          <label htmlFor="voice-search" className="text-sm font-medium text-ink-2">{t('common.search')}</label>
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-subtle" aria-hidden />
            <input id="voice-search" value={search} onChange={(e) => setSearch(e.target.value)} placeholder={t('voices.searchPlaceholder')}
              className="min-h-11 w-full rounded-[10px] border border-line bg-field pl-9 pr-3 text-[15px] text-ink outline-none placeholder:text-subtle focus:border-accent/60 focus:ring-2 focus:ring-accent/20" />
          </div>
        </div>
        <Select label={t('voices.gender')} value={gender} onChange={(e) => setGender(e.target.value)}>
          <option value="">{t('voices.any')}</option><option value="female">{t('voices.female')}</option><option value="male">{t('voices.male')}</option><option value="neutral">{t('voices.neutral')}</option>
        </Select>
        <Select label={t('voices.useCase')} value={useCase} onChange={(e) => setUseCase(e.target.value)}>
          <option value="">{t('voices.any')}</option>
          {useCases.map((u) => <option key={u} value={u}>{label(t, 'useCases', u)}</option>)}
        </Select>
        <Select label={t('voices.accent')} value={accent} onChange={(e) => setAccent(e.target.value)}>
          <option value="">{t('voices.any')}</option>
          {accents.map((a) => <option key={a} value={a}>{label(t, 'accents', a)}</option>)}
        </Select>
      </div>
      <div className="flex flex-col gap-2">
        <span className="text-sm font-medium text-ink-2">{t('voices.language')}</span>
        <div role="radiogroup" aria-label={t('voices.language')} className="flex flex-wrap gap-1.5">
          {LANGS.map((l) => (
            <button key={l || 'all'} type="button" role="radio" aria-checked={lang === l} onClick={() => setLang(l)}
              className={`inline-flex min-h-9 items-center gap-1.5 rounded-full border px-3 text-[13px] font-medium transition ${lang === l ? 'border-accent/55 bg-accent/15 text-accent-ink' : 'border-line bg-field text-ink-2 hover:text-ink'}`}>
              {l ? t(`voices.lang.${l}`) : t('voices.allLanguages')}
              {l === pbxLang && <span className="text-[10px] font-semibold uppercase tracking-wide text-accent">· {t('voices.pbxLanguage')}</span>}
            </button>
          ))}
        </div>
      </div>
      <ErrorMessage error={error} />
      {loading && page === 0 ? (
        <div className="flex items-center gap-2 py-8 text-sm text-muted" role="status"><Loader2 className="size-4 animate-spin" aria-hidden />{t('voices.loading')}</div>
      ) : shown.length === 0 && !error ? (
        <p className="py-8 text-center text-sm text-muted">{t('voices.empty')}</p>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {shown.map((v) => {
            const sel = selected?.id === v.id
            const tags = [v.gender && t(`voices.${v.gender}`, { defaultValue: v.gender }), v.age && label(t, 'ages', v.age), v.accent && label(t, 'accents', v.accent),
              v.useCase && label(t, 'useCases', v.useCase), v.descriptive && label(t, 'descriptives', v.descriptive)].filter(Boolean) as string[]
            return (
              <div key={v.id} className={`flex min-w-0 flex-col gap-2.5 rounded-2xl border p-3.5 transition ${sel ? 'border-accent bg-accent/5 shadow-[inset_0_0_0_1px_var(--color-accent)]' : 'border-line bg-inset'}`}>
                <div className="flex items-center gap-3">
                  <VoiceAvatar voice={v} />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-medium text-ink">{v.name}</span>
                    <span className="block truncate text-xs text-subtle">{[v.language && t(`voices.lang.${v.language}`, { defaultValue: v.language.toUpperCase() }), v.mine && t('voices.inAccount')].filter(Boolean).join(' · ')}</span>
                  </span>
                  <PreviewButton id={v.id} language={lang || undefined} label={t('voices.preview', { name: v.name })} />
                </div>
                {tags.length > 0 && <div className="flex flex-wrap gap-1">{tags.map((x) => <span key={x} className="rounded-md bg-tint/6 px-1.5 py-0.5 text-[11px] text-muted">{x}</span>)}</div>}
                {v.description && <p className="line-clamp-2 text-xs text-subtle">{v.description}</p>}
                <Button variant={sel ? 'primary' : 'secondary'} onClick={() => setChosen(v)}>{sel ? <><Check className="size-4" aria-hidden />{t('voices.chosen')}</> : t('voices.choose')}</Button>
              </div>
            )
          })}
        </div>
      )}
      {hasMore && source === 'library' && <div className="flex justify-center"><Button variant="secondary" busy={loading && page > 0} onClick={() => setPage(page + 1)}>{t('voices.more')}</Button></div>}
      {selected && <OwnText voice={selected} language={(lang === 'nl' || lang === 'en' ? lang : language)} />}
    </Modal>
  )
}

/** Speaks own text with the selected voice (costs characters). Library voices play their sample instead. */
function OwnText({ voice, language }: { voice: ElevenVoice; language: 'nl' | 'en' }) {
  const { t } = useTranslation()
  const [text, setText] = useState(() => t('voices.ownTextSample'))
  const [url, setUrl] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  if (!voice.mine) return null
  async function listen() {
    setBusy(true)
    setError(null)
    try {
      const r = await api<TTSResult>('POST', '/speech/tts', { text, language, engine: 'elevenlabs', voice: voice.id, voiceName: voice.name })
      setUrl(wavFromBase64(r.audio).url)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="flex flex-col gap-2 rounded-xl border border-line bg-surface-2/60 p-3.5">
      <label htmlFor="voice-own" className="text-sm font-medium text-ink-2">{t('voices.ownText', { name: voice.name })}</label>
      <div className="flex flex-wrap gap-2">
        <input id="voice-own" value={text} maxLength={300} onChange={(e) => setText(e.target.value)}
          className="min-h-10 min-w-0 flex-1 rounded-[10px] border border-line bg-field px-3 text-sm text-ink outline-none focus:border-accent/60" />
        <Button variant="secondary" busy={busy} disabled={!text.trim()} onClick={listen}><Sparkles className="size-4" aria-hidden />{t('voices.listen')}</Button>
      </div>
      {url && <audio controls autoPlay src={url} className="h-9 max-w-full" />}
      <p className="text-[11px] text-subtle">{t('voices.ownTextHint', { n: text.length })}</p>
      <ErrorMessage error={error} />
    </div>
  )
}

function label(t: TFunction, group: string, v: string) {
  return t(`voices.${group}.${v.toLowerCase().replace(/[\s-]+/g, '_')}`, { defaultValue: v.replace(/_/g, ' ') })
}

/** Loads what the text-to-speech controls need: the Piper voices and the ElevenLabs settings. */
export function useTTS() {
  const speech = useApi<SpeechData>('/speech')
  const el = useApi<ElevenData>('/speech/elevenlabs')
  const piperVoices = (speech.data?.voices ?? []).filter((v) => v.downloaded)
  const piperReady = !!speech.data?.settings.tts.enabled && !!speech.data?.ttsInstalled && piperVoices.length > 0
  const elevenReady = !!el.data?.settings.enabled && !!el.data?.keySet
  return { speech: speech.data, el: el.data, piperVoices, piperReady, elevenReady, loaded: !!speech.data && !!el.data }
}
