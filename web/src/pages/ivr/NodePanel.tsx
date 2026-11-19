import { useRef, useState, type DragEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { Ban, ChevronDown, Copy, Library, Loader2, Mic, Plus, Sparkles, Square, Trash2, Upload, X } from 'lucide-react'
import { Select, Textarea, Toggle, useApi } from '../../components/kit'
import { DestinationPicker } from '../../components/pbx'
import { Button, ErrorMessage, Input } from '../../components/ui'
import { WavePlayer, promptSrc } from '../../components/WavePlayer'
import { ApiError, api } from '../../lib/api'
import { startRecording, toTelephoneWav } from '../../lib/audio'
import type { IVR, Prompt, SpeechData, SpeechIntent, TimeCondition } from '../../lib/types'
import { KEYS, hasAudio, toneOf, tones, type StepData, type StepNode } from './flow'
import { NodeTitle, iconOf } from './nodes'

type Mode = 'generate' | 'record' | 'upload' | 'library' | 'none'
const MAX_UPLOAD = 10 << 20

function Section({ title, children, aside }: { title: string; children: ReactNode; aside?: ReactNode }) {
  const [open, setOpen] = useState(true)
  return (
    <section className="rounded-2xl border border-line bg-surface p-4">
      <button type="button" onClick={() => setOpen(!open)} aria-expanded={open} className="flex w-full items-center justify-between gap-2 text-left">
        <h3 className="font-display text-sm font-semibold text-ink">{title}</h3>
        <span className="flex items-center gap-2 text-[11px] text-subtle">{aside}<ChevronDown className={`size-4 transition ${open ? 'rotate-180' : ''}`} aria-hidden /></span>
      </button>
      {open && <div className="mt-3 flex flex-col gap-3">{children}</div>}
    </section>
  )
}

/** Chooses the voice prompt of a step: generate it from text, record, upload, pick one, or none. */
function AudioSource({ promptId, onPrompt, promptName, prompts, onPromptsChanged, tone }: {
  promptId: number; onPrompt: (id: number) => void; promptName: string; prompts: Prompt[]; onPromptsChanged: () => void; tone: string
}) {
  const { t, i18n } = useTranslation()
  const [mode, setMode] = useState<Mode>(promptId ? 'library' : 'generate')
  const speech = useApi<SpeechData>('/speech')
  const nl = i18n.language.startsWith('nl')
  const [text, setText] = useState('')
  const [voice, setVoice] = useState('')
  const [busy, setBusy] = useState<'' | 'tts' | 'convert' | 'save'>('')
  const [recording, setRecording] = useState(false)
  const [drag, setDrag] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const rec = useRef<{ stop: () => Promise<Blob> } | null>(null)
  const file = useRef<HTMLInputElement>(null)

  const voices = (speech.data?.voices ?? []).filter((v) => v.downloaded)
  const defVoice = speech.data ? (nl ? speech.data.settings.tts.voiceNl : speech.data.settings.tts.voiceEn) : ''
  const chosenVoice = voice || (voices.some((v) => v.id === defVoice) ? defVoice : voices[0]?.id ?? '')
  const ttsReady = !!speech.data?.settings.tts.enabled && speech.data.ttsInstalled && voices.length > 0

  async function store(base64: string) {
    setBusy('save')
    const p = await api<Prompt>('POST', '/pbx/prompts', { name: promptName.slice(0, 60), audio: base64 })
    onPromptsChanged()
    onPrompt(p.id)
  }
  async function run(kind: 'tts' | 'convert', fn: () => Promise<void>) {
    setBusy(kind)
    setError(null)
    try {
      await fn()
    } catch (e) {
      setError(e)
    } finally {
      setBusy('')
    }
  }
  const generate = () => run('tts', async () => {
    const r = await api<{ audio: string }>('POST', '/speech/tts', { text, voice: chosenVoice, language: nl ? 'nl' : 'en' })
    await store(r.audio)
  })
  const convert = (b: Blob) => run('convert', async () => {
    if (b.size > MAX_UPLOAD) throw new Error(t('ivr.panel.tooLarge'))
    let w
    try {
      w = await toTelephoneWav(b)
    } catch {
      throw new Error(t('audio.convertFailed'))
    }
    if (w.seconds < 0.3) throw new Error(t('audio.convertFailed'))
    await store(w.base64)
  })
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
      setError(null)
    } catch {
      setError(new Error(t('audio.micDenied')))
    }
  }
  function drop(e: DragEvent) {
    e.preventDefault()
    setDrag(false)
    const f = e.dataTransfer.files?.[0]
    if (f) convert(f)
  }
  function pick(m: Mode) {
    setMode(m)
    setError(null)
    if (m === 'none') onPrompt(0)
  }

  const modes: { key: Mode; icon: typeof Mic }[] = [
    { key: 'generate', icon: Sparkles }, { key: 'record', icon: Mic }, { key: 'upload', icon: Upload }, { key: 'library', icon: Library }, { key: 'none', icon: Ban },
  ]
  const current = prompts.find((p) => p.id === promptId)

  return (
    <>
      <div role="radiogroup" aria-label={t('ivr.panel.source')} className="grid grid-cols-5 gap-1.5">
        {modes.map(({ key, icon: I }) => (
          <button key={key} type="button" role="radio" aria-checked={mode === key} onClick={() => pick(key)}
            className={`flex flex-col items-center gap-1 rounded-xl border px-0.5 py-2 text-[11px] transition ${mode === key ? 'border-accent bg-accent/12 font-semibold text-ink' : 'border-line bg-field text-muted hover:text-ink'}`}>
            <I className="size-4" aria-hidden />{t(`ivr.panel.modes.${key}`)}
          </button>
        ))}
      </div>

      {mode === 'generate' && (ttsReady ? (
        <>
          <Textarea label={t('ivr.panel.text')} value={text} onChange={(e) => setText(e.target.value)} rows={3} maxLength={2000} placeholder={t('ivr.panel.textPlaceholder')} />
          <Select label={t('ivr.panel.voice')} value={chosenVoice} onChange={(e) => setVoice(e.target.value)}>
            {voices.map((v) => <option key={v.id} value={v.id}>{v.name}</option>)}
          </Select>
          <Button busy={busy === 'tts' || busy === 'save'} disabled={!text.trim() || !!busy} onClick={generate}>
            <Sparkles className="size-4" aria-hidden />{promptId ? t('ivr.panel.regenerate') : t('ivr.panel.generate')}
          </Button>
          <p className="text-[11px] text-subtle">{t('ivr.panel.generateHint')}</p>
        </>
      ) : speech.data ? (
        <p className="rounded-xl border border-line bg-inset p-3 text-xs text-muted">
          {t('ivr.panel.ttsOff')} <Link to="/speech" className="text-accent underline">{t('nav.speech')}</Link>
        </p>
      ) : null)}

      {mode === 'record' && (
        <div className="flex flex-wrap items-center gap-3">
          <Button variant={recording ? 'danger' : 'secondary'} onClick={toggleRecord} disabled={!!busy}>
            {recording ? <Square className="size-4" aria-hidden /> : <Mic className="size-4" aria-hidden />}
            {recording ? t('audio.stop') : t('audio.record')}
          </Button>
          {recording && <span className="inline-flex items-center gap-2 text-sm text-danger"><span className="size-2 animate-pulse rounded-full bg-danger" aria-hidden />{t('audio.recording')}</span>}
        </div>
      )}

      {mode === 'upload' && (
        <div onDragOver={(e) => { e.preventDefault(); setDrag(true) }} onDragLeave={() => setDrag(false)} onDrop={drop}
          className={`flex flex-col items-center gap-1 rounded-xl border-[1.5px] border-dashed px-3 py-5 text-center text-sm transition ${drag ? 'border-accent bg-accent/8 text-ink' : 'border-line text-muted'}`}>
          <Upload className="mb-1 size-5" aria-hidden />
          <span>{t('ivr.panel.dropHere')} <button type="button" onClick={() => file.current?.click()} className="text-accent underline">{t('ivr.panel.browse')}</button></span>
          <span className="text-[11px] text-subtle">{t('ivr.panel.dropHint')}</span>
          <input ref={file} type="file" accept="audio/*" className="hidden" onChange={(e) => { const f = e.target.files?.[0]; if (f) convert(f); e.target.value = '' }} />
        </div>
      )}

      {mode === 'library' && (
        <Select label={t('ivr.panel.library')} value={promptId} onChange={(e) => onPrompt(Number(e.target.value))}>
          <option value={0}>{t('ivr.noPrompt')}</option>
          {prompts.map((p) => <option key={p.id} value={p.id}>{p.name} ({(p.durationMs / 1000).toFixed(1)} s)</option>)}
        </Select>
      )}

      {(busy === 'convert' || busy === 'save') && <span className="inline-flex items-center gap-2 text-sm text-muted"><Loader2 className="size-4 animate-spin" aria-hidden />{t('audio.converting')}</span>}
      {error instanceof ApiError ? <ErrorMessage error={error} /> : error ? <p className="text-sm text-danger">{(error as Error).message}</p> : null}

      {promptId > 0 && (
        <div className="flex flex-col gap-1.5">
          <span className="text-xs font-medium text-ink-2">{t('ivr.panel.preview')}</span>
          <WavePlayer src={promptSrc(promptId, current?.durationMs)} size="lg" tone={tone} label={current?.name} />
          {current && <span className="truncate text-[11px] text-subtle">{t('ivr.panel.savedAs', { name: current.name })}</span>}
        </div>
      )}
    </>
  )
}

export function NodePanel({ node, ivr, setIvr, prompts, onPromptsChanged, timeConditions, usedDigits, onChange, onRemoveIntent, onDelete, onDuplicate, onClose }: {
  node: StepNode
  ivr: IVR
  setIvr: (fn: (v: IVR) => IVR) => void
  prompts: Prompt[]
  onPromptsChanged: () => void
  timeConditions: TimeCondition[]
  usedDigits: string[]
  onChange: (d: Partial<StepData>) => void
  onRemoveIntent: (i: number) => void
  onDelete: () => void
  onDuplicate: () => void
  onClose: () => void
}) {
  const { t } = useTranslation()
  const d = node.data
  const tone = tones[toneOf(d)]
  const Icon = iconOf(d)
  const stepName = d.type === 'key' ? `${t('ivr.node.press')} ${d.digit}${d.label ? ` ${t('ivr.node.for')} ${d.label}` : ''}`
    : d.type === 'entry' ? t('ivr.panel.welcome') : d.label || t(`ivr.kind.${d.type === 'destination' ? d.destination?.type || 'hangup' : d.type}`)
  const promptName = `${ivr.name || t('ivr.untitled')} · ${stepName}`

  return (
    <aside aria-label={t('ivr.panel.title')} className="flex h-full min-h-0 flex-col">
      <div className="flex items-center gap-3 border-b border-line px-4 py-3.5">
        <span className={`inline-flex size-9 flex-none items-center justify-center rounded-xl border ${tone.border} ${tone.bg} ${tone.text}`}><Icon className="size-4" aria-hidden /></span>
        <div className="min-w-0 flex-1">
          <div className="truncate font-display text-base font-semibold text-ink"><NodeTitle d={d} /></div>
          <div className="truncate text-xs text-subtle">{t(`ivr.panel.kinds.${d.type}`)}</div>
        </div>
        <button type="button" onClick={onClose} aria-label={t('common.close')} className="inline-flex size-9 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-ink"><X className="size-4" aria-hidden /></button>
      </div>

      <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-4">
        {d.type === 'entry' && (
          <Section title={t('ivr.panel.menu')}>
            <Input name="name" label={t('common.name')} value={ivr.name} onChange={(e) => setIvr((v) => ({ ...v, name: e.target.value }))} maxLength={60} />
            <div className="grid grid-cols-2 gap-3 [&>*]:min-w-0">
              <Input name="timeout" label={t('ivr.timeout')} type="number" min={1} max={60} value={ivr.timeout} onChange={(e) => setIvr((v) => ({ ...v, timeout: Number(e.target.value) }))} hint={t('common.seconds')} />
              <Input name="retries" label={t('ivr.retries')} type="number" min={0} max={5} value={ivr.retries} onChange={(e) => setIvr((v) => ({ ...v, retries: Number(e.target.value) }))} hint={t('ivr.retriesHint')} />
            </div>
            <Toggle label={t('ivr.directDial')} hint={t('ivr.directDialHint')} checked={ivr.directDial} onChange={(x) => setIvr((v) => ({ ...v, directDial: x }))} />
          </Section>
        )}

        {hasAudio(d.type) && (
          <Section title={t('ivr.panel.audio')} aside={t(`ivr.panel.audioWhen.${d.type}`)}>
            <AudioSource key={node.id} promptId={d.promptId ?? 0} onPrompt={(id) => onChange({ promptId: id })} promptName={promptName}
              prompts={prompts} onPromptsChanged={onPromptsChanged} tone={tone.text} />
          </Section>
        )}

        {d.type !== 'entry' && (
          <Section title={t('ivr.panel.manage')}>
            {d.type === 'key' && (
              <div className="grid grid-cols-[96px_minmax(0,1fr)] gap-3 [&>*]:min-w-0">
                <Select name="digit" label={t('ivr.panel.keypress')} value={d.digit} onChange={(e) => onChange({ digit: e.target.value })}>
                  {KEYS.map((k) => <option key={k} value={k} disabled={k !== d.digit && usedDigits.includes(k)}>{k}</option>)}
                </Select>
                <Input name="label" label={t('ivr.label')} value={d.label ?? ''} onChange={(e) => onChange({ label: e.target.value })} placeholder={t('ivr.labelPlaceholder')} maxLength={40} />
              </div>
            )}
            {d.type === 'speech' && <SpeechIntents d={d} onChange={onChange} onRemoveIntent={onRemoveIntent} />}
            {(d.type === 'audio' || d.type === 'destination' || d.type === 'repeat' || d.type === 'time' || d.type === 'timeout' || d.type === 'speech') && (
              <Input name="label" label={t('ivr.label')} value={d.label ?? ''} onChange={(e) => onChange({ label: e.target.value })} hint={t('ivr.labelHint')} maxLength={40} />
            )}
            {d.type === 'destination' && (
              <DestinationPicker label={t('ivr.goTo')} value={d.destination ?? { type: 'hangup', target: '' }} onChange={(dest) => onChange({ destination: dest })}
                exclude={ivr.id ? { type: 'ivr', target: String(ivr.id) } : undefined} />
            )}
            {d.type === 'time' && (
              <>
                <Select name="destination" label={t('ivr.panel.timeCondition')} value={d.destination?.target ?? ''} onChange={(e) => onChange({ destination: { type: 'timecondition', target: e.target.value } })}>
                  <option value="">{t('dest.choose')}</option>
                  {timeConditions.map((c) => <option key={c.id} value={String(c.id)}>{c.name}</option>)}
                </Select>
                <p className="text-xs text-subtle">{t('ivr.panel.timeHint')}</p>
              </>
            )}
            {d.type === 'timeout' && <p className="text-xs text-subtle">{t('ivr.noInputHint', { retries: ivr.retries })}</p>}
            {d.type === 'repeat' && <p className="text-xs text-subtle">{t('ivr.panel.repeatHint')}</p>}
            <p className="text-xs text-subtle">{t(d.type === 'speech' ? 'ivr.panel.speechLinesHint' : d.type === 'key' || d.type === 'timeout' || d.type === 'audio' || d.type === 'time' ? 'ivr.panel.linesHint' : 'ivr.panel.endHint')}</p>
          </Section>
        )}

        {d.type !== 'entry' && (
          <div className="flex gap-2">
            {d.type !== 'timeout' && <Button variant="ghost" onClick={onDuplicate}><Copy className="size-4" aria-hidden />{t('ivr.panel.duplicate')}</Button>}
            <Button variant="danger" onClick={onDelete}><Trash2 className="size-4" aria-hidden />{t('common.delete')}</Button>
          </div>
        )}
      </div>
    </aside>
  )
}

/** The intents of a speech step: a name and the keywords that choose it. */
function SpeechIntents({ d, onChange, onRemoveIntent }: { d: StepData; onChange: (d: Partial<StepData>) => void; onRemoveIntent: (i: number) => void }) {
  const { t } = useTranslation()
  const intents = d.intents ?? []
  const set = (i: number, p: Partial<SpeechIntent>) => onChange({ intents: intents.map((x, j) => (j === i ? { ...x, ...p } : x)) })
  return (
    <div className="flex flex-col gap-3">
      <p className="text-xs text-muted">{t('ivr.panel.speechIntro')}</p>
      {intents.map((it, i) => (
        <div key={i} className="flex flex-col gap-2 rounded-xl border border-line bg-surface-2/60 p-3">
          <div className="flex items-end gap-2">
            <div className="min-w-0 flex-1">
              <Input label={t('ivr.panel.intent', { n: i + 1 })} value={it.label} maxLength={40} placeholder={t('ivr.panel.intentPlaceholder')}
                onChange={(e) => set(i, { label: e.target.value })} />
            </div>
            <button type="button" onClick={() => onRemoveIntent(i + 1)} aria-label={t('common.delete')}
              className="inline-flex size-10 flex-none items-center justify-center rounded-lg text-muted hover:bg-danger/10 hover:text-danger"><Trash2 className="size-4" aria-hidden /></button>
          </div>
          <Input label={t('ivr.panel.keywords')} value={it.keywords.join(',')} placeholder={t('ivr.panel.keywordsPlaceholder')}
            onChange={(e) => set(i, { keywords: e.target.value.split(',').slice(0, 30) })} />
        </div>
      ))}
      {intents.length < 8 && (
        <Button variant="secondary" onClick={() => onChange({ intents: [...intents, { label: '', keywords: [] }] })}><Plus className="size-4" aria-hidden />{t('ivr.panel.addIntent')}</Button>
      )}
      <p className="text-xs text-subtle">{t('ivr.panel.keywordsHint')}</p>
      <Toggle name="ai" label={t('ivr.panel.speechAI')} hint={t('ivr.panel.speechAIHint')} checked={!!d.ai} onChange={(v) => onChange({ ai: v })} />
    </div>
  )
}
