import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { Sparkles } from 'lucide-react'
import { Select, Textarea } from './kit'
import { Button, ErrorMessage } from './ui'
import { VoiceBox, useTTS } from './Voices'
import { api } from '../lib/api'
import type { ElevenUsage, PromptSource, TTSResult, VoiceRef } from '../lib/types'

type Engine = 'piper' | 'elevenlabs'

const nf = (n: number, lang: string) => n.toLocaleString(lang.startsWith('nl') ? 'nl-NL' : 'en-GB')

/**
 * Typed text to a prompt: text, language, engine (local Piper or ElevenLabs) and voice. onAudio gets
 * the 8 kHz WAV and its source, which the caller stores with the prompt.
 */
export function SpeakFromText({ initial, onAudio, action, regenerate, compact }: {
  initial?: PromptSource
  onAudio: (r: TTSResult) => Promise<void> | void
  action?: string
  regenerate?: boolean
  compact?: boolean
}) {
  const { t, i18n } = useTranslation()
  const tts = useTTS()
  const [text, setText] = useState(initial?.text ?? '')
  const [lang, setLang] = useState<'nl' | 'en' | ''>((initial?.language as 'nl' | 'en') ?? '')
  const [engine, setEngine] = useState<Engine | ''>(initial?.engine ?? '')
  const [piperVoice, setPiperVoice] = useState(initial?.engine === 'piper' ? initial.voice : '')
  const [elVoice, setElVoice] = useState<VoiceRef | null>(initial?.engine === 'elevenlabs' ? { id: initial.voice, name: initial.voiceName ?? '' } : null)
  const [usage, setUsage] = useState<ElevenUsage | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [last, setLast] = useState<TTSResult | null>(null)

  const language = lang || tts.el?.pbxLanguage || (i18n.language.startsWith('nl') ? 'nl' : 'en')
  const eng: Engine = engine || (tts.elevenReady && tts.el?.settings.defaultEngine === 'elevenlabs' ? 'elevenlabs' : tts.piperReady ? 'piper' : 'elevenlabs')
  useEffect(() => {
    if (!tts.elevenReady) return
    api<ElevenUsage>('GET', '/speech/elevenlabs/usage').then(setUsage).catch(() => setUsage(null))
  }, [tts.elevenReady])

  if (!tts.loaded) return null
  if (!tts.piperReady && !tts.elevenReady) {
    return (
      <p className="rounded-xl border border-line bg-inset p-3 text-xs text-muted">
        {t('tts.off')} <Link to="/speech" className="text-accent underline">{t('nav.speech')}</Link>
      </p>
    )
  }
  const voices = tts.piperVoices.filter((v) => v.language === language)
  const defPiper = language === 'nl' ? tts.speech?.settings.tts.voiceNl : tts.speech?.settings.tts.voiceEn
  const pv = piperVoice && voices.some((v) => v.id === piperVoice) ? piperVoice : voices.some((v) => v.id === defPiper) ? defPiper! : voices[0]?.id ?? ''
  const defEl = tts.el ? (language === 'nl' ? tts.el.settings.voiceNl : tts.el.settings.voiceEn) : { id: '', name: '' }
  const ev = elVoice ?? defEl
  const n = [...text.trim()].length
  const max = tts.el?.maxText ?? 2000
  const ready = eng === 'piper' ? !!pv : !!ev.id

  async function generate() {
    setBusy(true)
    setError(null)
    try {
      const body = eng === 'piper' ? { text, language, engine: eng, voice: pv } : { text, language, engine: eng, voice: ev.id, voiceName: ev.name }
      const r = await api<TTSResult>('POST', '/speech/tts', body)
      setLast(r)
      await onAudio(r)
      if (eng === 'elevenlabs' && !r.cached && usage) setUsage({ ...usage, characterCount: usage.characterCount + r.characters })
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className={`flex flex-col ${compact ? 'gap-3' : 'gap-3.5'}`}>
      <div className="flex flex-col gap-1">
        <Textarea label={t('tts.text')} value={text} onChange={(e) => setText(e.target.value)} rows={compact ? 3 : 4} maxLength={max} placeholder={t('tts.placeholder')} />
        <div className="flex flex-wrap justify-between gap-2 text-[11px] tabular-nums text-subtle">
          <span>{t('tts.characters', { n: nf(n, i18n.language) })}</span>
          {eng === 'elevenlabs' && <span>{usage && usage.characterLimit > 0
            ? t('tts.creditsLeft', { n: nf(n, i18n.language), left: nf(Math.max(0, usage.characterLimit - usage.characterCount), i18n.language) })
            : t('tts.credits', { n: nf(n, i18n.language) })}</span>}
        </div>
      </div>
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-end gap-2">
        <span className="text-sm font-medium text-ink-2">{t('tts.engine')}</span>
        <select aria-label={t('tts.language')} value={language} onChange={(e) => setLang(e.target.value as 'nl' | 'en')} className="min-h-8 rounded-lg border border-line bg-field px-2 text-xs text-ink-2">
          <option value="nl">Nederlands</option><option value="en">English</option>
        </select>
      </div>
      <div role="radiogroup" aria-label={t('tts.engine')} className="-mt-1.5 grid grid-cols-2 gap-2">
        {(['piper', 'elevenlabs'] as const).map((e) => {
          const avail = e === 'piper' ? tts.piperReady : tts.elevenReady
          const on = eng === e
          return (
            <button key={e} type="button" role="radio" aria-checked={on} disabled={!avail} onClick={() => setEngine(e)}
              title={avail ? undefined : t('tts.engineOff')}
              className={`rounded-xl border px-3 py-2 text-left transition disabled:cursor-not-allowed disabled:opacity-45 ${on ? 'border-accent bg-accent/8 shadow-[inset_0_0_0_1px_var(--color-accent)]' : 'border-line bg-field hover:border-accent/40'}`}>
              <span className="block text-[13px] font-semibold text-ink">{t(`tts.engines.${e}`)}</span>
              <span className="block text-[11px] text-subtle">{avail ? t(`tts.engineHints.${e}`) : t('tts.engineOff')}</span>
            </button>
          )
        })}
      </div>
      {eng === 'piper' ? (
        <Select label={t('tts.voice')} value={pv} onChange={(e) => setPiperVoice(e.target.value)}>
          {voices.length === 0 && <option value="">{t('tts.noPiperVoice')}</option>}
          {voices.map((v) => <option key={v.id} value={v.id}>{v.name}</option>)}
        </Select>
      ) : (
        <div className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-ink-2">{t('tts.voice')}</span>
          <VoiceBox voice={ev} language={language} onChange={setElVoice} compact={compact} />
        </div>
      )}
      <Button busy={busy} disabled={!text.trim() || n > max || !ready} onClick={generate}>
        <Sparkles className="size-4" aria-hidden />{action ?? (regenerate ? t('tts.regenerate') : t('tts.generate'))}
      </Button>
      {last && !busy && !error && (
        <p className={`rounded-lg border px-3 py-2 text-[11.5px] ${last.fellBack ? 'border-warn/40 bg-warn/10 text-warn-ink' : 'border-ok/35 bg-ok/8 text-ok-ink'}`}>
          {last.fellBack ? t('tts.fellBack') : last.cached ? t('tts.cached', { voice: last.source.voiceName || last.source.voice })
            : t('tts.done', { engine: t(`tts.engines.${last.source.engine}`), voice: last.source.voiceName || last.source.voice })}
        </p>
      )}
      <ErrorMessage error={error} />
      <p className="text-[11px] text-subtle">{eng === 'elevenlabs' ? t('tts.hintEleven') : t('tts.hintPiper')}</p>
    </div>
  )
}
