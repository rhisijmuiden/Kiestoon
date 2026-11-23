import { useEffect, useRef, useState, type RefObject } from 'react'
import { useTranslation } from 'react-i18next'
import { AudioLines, Check, Cloud, CloudDownload, KeyRound, Loader2, Lock, MessageSquareText, Play, ShieldCheck, Trash2 } from 'lucide-react'
import { Badge, SectionTitle, Select, Table, Textarea, Toggle, formatBytes, td, useApi } from '../components/kit'
import { IconButton } from '../components/pbx'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import { wavFromBase64 } from '../lib/audio'
import type { ElevenData, ElevenSettings, ElevenUsage, SpeechData, SpeechItem, SpeechSettings } from '../lib/types'
import { VoiceBox } from '../components/Voices'

/** Local text-to-speech (Piper) and speech-to-text (whisper.cpp). Nothing leaves the server. */
export function Speech() {
  const { t } = useTranslation()
  const [poll, setPoll] = useState(false)
  const d = useApi<SpeechData>('/speech', poll ? 2000 : undefined)
  const [s, setS] = useState<SpeechSettings | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  const elSave = useRef<(() => Promise<void>) | null>(null)
  useEffect(() => { if (d.data && !s) setS(d.data.settings) }, [d.data, s])
  useEffect(() => {
    const items = [...(d.data?.voices ?? []), ...(d.data?.models ?? [])]
    setPoll(items.some((i) => i.progress && !i.progress.error && !i.downloaded))
  }, [d.data])
  if (!s || !d.data) return <><PageHeader title={t('speech.title')} /><ErrorMessage error={d.error} /></>
  const data = d.data

  async function save() {
    setBusy(true)
    setError(null)
    setSaved(false)
    try {
      await elSave.current?.()
      const r = await api<SpeechData>('PUT', '/speech', s)
      d.setData(r)
      setS(r.settings)
      setSaved(true)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  async function download(id: string) {
    setError(null)
    try {
      await api('POST', '/speech/download', { id })
      setPoll(true)
      d.reload()
    } catch (e) {
      setError(e)
    }
  }
  async function remove(i: SpeechItem) {
    if (!window.confirm(t('common.confirmDelete', { name: i.name }))) return
    await api('DELETE', `/speech/files/${i.id}`)
    d.reload()
  }
  const voices = (lang: string) => data.voices.filter((v) => v.language?.startsWith(lang))
  const setTTS = (p: Partial<SpeechSettings['tts']>) => { setSaved(false); setS({ ...s, tts: { ...s.tts, ...p } }) }
  const setSTT = (p: Partial<SpeechSettings['stt']>) => { setSaved(false); setS({ ...s, stt: { ...s.stt, ...p } }) }

  return (
    <>
      <PageHeader title={t('speech.title')} subtitle={t('speech.subtitle')}>
        <Button busy={busy} onClick={save}>{t('common.save')}</Button>
      </PageHeader>
      {(!data.ttsInstalled || !data.sttInstalled) && <Alert tone="warn">{t('speech.notInstalled')}</Alert>}
      {saved && <Alert tone="ok">{t('speech.saved')}</Alert>}
      <ErrorMessage error={error} />
      <ElevenLabs saveRef={elSave} onChange={() => setSaved(false)} />
      <div className="grid gap-4 xl:grid-cols-2">
        <Card>
          <SectionTitle><span className="inline-flex items-center gap-2"><AudioLines className="size-5 text-accent" aria-hidden />{t('speech.tts')}</span></SectionTitle>
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted">{t('speech.ttsIntro')}</p>
            <Toggle name="enabled" label={t('speech.ttsEnable')} checked={s.tts.enabled} disabled={!data.ttsInstalled} onChange={(v) => setTTS({ enabled: v })} />
            <div className="grid gap-4 sm:grid-cols-2">
              <Select name="voiceEn" label={t('speech.voiceEn')} value={s.tts.voiceEn} onChange={(e) => setTTS({ voiceEn: e.target.value })}>
                {voices('en').map((v) => <option key={v.id} value={v.id}>{v.name}</option>)}
              </Select>
              <Select name="voiceNl" label={t('speech.voiceNl')} value={s.tts.voiceNl} onChange={(e) => setTTS({ voiceNl: e.target.value })}>
                {voices('nl').map((v) => <option key={v.id} value={v.id}>{v.name}</option>)}
              </Select>
            </div>
            <Items items={data.voices} onDownload={download} onRemove={remove} />
            {s.tts.enabled && data.settings.tts.enabled && <TryVoice />}
          </div>
        </Card>
        <Card>
          <SectionTitle><span className="inline-flex items-center gap-2"><MessageSquareText className="size-5 text-accent" aria-hidden />{t('speech.stt')}</span></SectionTitle>
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted">{t('speech.sttIntro')}</p>
            <Toggle name="enabled" label={t('speech.sttEnable')} checked={s.stt.enabled} disabled={!data.sttInstalled} onChange={(v) => setSTT({ enabled: v })} />
            <div className="grid gap-4 sm:grid-cols-2">
              <Select name="model" label={t('speech.model')} value={s.stt.model} onChange={(e) => setSTT({ model: e.target.value })}>
                {data.models.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}
              </Select>
              <Select name="language" label={t('speech.language')} value={s.stt.language} onChange={(e) => setSTT({ language: e.target.value as SpeechSettings['stt']['language'] })}>
                <option value="auto">{t('speech.auto')}</option><option value="nl">Nederlands</option><option value="en">English</option>
              </Select>
            </div>
            <Toggle name="voicemail" label={t('speech.voicemail')} hint={t('speech.voicemailHint')} checked={s.stt.voicemail} onChange={(v) => setSTT({ voicemail: v })} />
            <Items items={data.models} onDownload={download} onRemove={remove} />
            <p className="text-xs text-subtle">{t('speech.sttHint')}</p>
          </div>
        </Card>
      </div>
    </>
  )
}

export function Items({ items, onDownload, onRemove }: { items: SpeechItem[]; onDownload: (id: string) => void; onRemove: (i: SpeechItem) => void }) {
  const { t } = useTranslation()
  return (
    <Table head={[t('common.name'), t('speech.size'), t('speech.licence'), '']}>
      {items.map((i) => {
        const p = i.progress
        const running = !!p && !p.error && !i.downloaded
        return (
          <tr key={i.id}>
            <td className={`${td} text-ink`}>
              {i.name}
              {i.speed && <span className="ml-1.5 text-xs text-subtle">{t(`speech.speeds.${i.speed}`)}</span>}
            </td>
            <td className={`${td} whitespace-nowrap font-mono text-xs`}>{formatBytes(i.size)}</td>
            <td className={`${td} text-xs`}>{i.licence}</td>
            <td className={`${td} whitespace-nowrap text-right`}>
              {i.downloaded ? (
                <span className="inline-flex items-center gap-1"><Badge tone="ok" dot>{t('speech.ready')}</Badge>
                  <IconButton label={t('common.delete')} danger onClick={() => onRemove(i)}><Trash2 className="size-4" aria-hidden /></IconButton></span>
              ) : running ? (
                <span className="inline-flex items-center gap-2 text-xs text-muted" role="status">
                  <Loader2 className="size-4 animate-spin text-accent" aria-hidden />{p.total ? `${Math.round((p.done / p.total) * 100)}%` : t('speech.downloading')}
                </span>
              ) : (
                <span className="inline-flex items-center gap-2">
                  {p?.error && <span className="text-xs text-danger" title={p.error}>{t('speech.failed')}</span>}
                  <Button variant="secondary" onClick={() => onDownload(i.id)}><CloudDownload className="size-4" aria-hidden />{t('speech.download')}</Button>
                </span>
              )}
            </td>
          </tr>
        )
      })}
    </Table>
  )
}

function TryVoice() {
  const { t, i18n } = useTranslation()
  const [text, setText] = useState(() => t('speech.sample'))
  const [lang, setLang] = useState(i18n.language.startsWith('nl') ? 'nl' : 'en')
  const [url, setUrl] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  async function speak() {
    setBusy(true)
    setError(null)
    try {
      const r = await api<{ audio: string }>('POST', '/speech/tts', { text, language: lang })
      setUrl(wavFromBase64(r.audio).url)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="flex flex-col gap-3 rounded-xl border border-line bg-surface-2/60 p-4">
      <Textarea label={t('speech.try')} value={text} onChange={(e) => setText(e.target.value)} maxLength={2000} />
      <div className="flex flex-wrap items-center gap-3">
        <select aria-label={t('speech.language')} value={lang} onChange={(e) => setLang(e.target.value)} className="min-h-10 rounded-lg border border-line bg-field px-2 text-sm">
          <option value="nl">Nederlands</option><option value="en">English</option>
        </select>
        <Button variant="secondary" busy={busy} disabled={!text.trim()} onClick={speak}><Play className="size-4" aria-hidden />{t('speech.listen')}</Button>
        {url && <audio controls autoPlay src={url} className="h-9 max-w-full" />}
      </div>
      <ErrorMessage error={error} />
    </div>
  )
}

/** ElevenLabs: switch, API key (sealed on the server), usage, model and the default voices. */
function ElevenLabs({ saveRef, onChange }: { saveRef: RefObject<(() => Promise<void>) | null>; onChange: () => void }) {
  const { t, i18n } = useTranslation()
  const d = useApi<ElevenData>('/speech/elevenlabs')
  const [s, setS] = useState<ElevenSettings | null>(null)
  const [key, setKey] = useState('')
  const [editKey, setEditKey] = useState(false)
  const [busy, setBusy] = useState<'' | 'key' | 'test'>('')
  const [error, setError] = useState<unknown>(null)
  const [usage, setUsage] = useState<ElevenUsage | null>(null)
  const [checked, setChecked] = useState('')
  useEffect(() => { if (d.data && !s) setS(d.data.settings) }, [d.data, s])
  useEffect(() => {
    saveRef.current = s ? async () => { const r = await api<ElevenData>('PUT', '/speech/elevenlabs', s); d.setData(r); setS(r.settings) } : null
  })
  async function test() {
    setBusy('test')
    setError(null)
    try {
      setUsage(await api<ElevenUsage>('GET', '/speech/elevenlabs/usage'))
      setChecked(new Date().toLocaleTimeString(i18n.language, { hour: '2-digit', minute: '2-digit' }))
    } catch (e) {
      setError(e)
      setUsage(null)
    } finally {
      setBusy('')
    }
  }
  useEffect(() => { if (d.data?.keySet) void test() }, [d.data?.keySet]) // eslint-disable-line react-hooks/exhaustive-deps
  if (!s || !d.data) return <ErrorMessage error={d.error} />
  const data = d.data
  const set = (p: Partial<ElevenSettings>) => { onChange(); setS({ ...s, ...p }) }
  async function saveKey(k: string) {
    setBusy('key')
    setError(null)
    try {
      const r = await api<ElevenData>('PUT', '/speech/elevenlabs/key', { key: k })
      d.setData({ ...r, settings: s! })
      setKey('')
      setEditKey(false)
      if (!r.keySet) { setUsage(null); setChecked('') }
    } catch (e) {
      setError(e)
    } finally {
      setBusy('')
    }
  }
  const pct = usage && usage.characterLimit > 0 ? Math.min(100, Math.round((usage.characterCount / usage.characterLimit) * 100)) : 0
  const nf = (n: number) => n.toLocaleString(i18n.language.startsWith('nl') ? 'nl-NL' : 'en-GB')
  const reset = usage?.resetUnix ? new Date(usage.resetUnix * 1000).toLocaleDateString(i18n.language, { day: 'numeric', month: 'long' }) : ''

  const locked = s.locked && s.enabled
  const lockable = s.enabled && data.keySet && !!(s.voiceNl.id || s.voiceEn.id)

  return (
    <div className="mb-4 flex flex-col gap-4">
      <Card>
        <SectionTitle>
          <span className="inline-flex flex-wrap items-center gap-2"><Cloud className="size-5 text-accent" aria-hidden />ElevenLabs
            <Badge tone="accent">{t('eleven.cloud')}</Badge>{usage && <Badge tone="ok" dot>{t('eleven.connected')}</Badge>}</span>
        </SectionTitle>
        <div className="grid gap-x-6 gap-y-4 lg:grid-cols-2">
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted">{t('eleven.intro')}</p>
            <Toggle label={t('eleven.enable')} hint={t('eleven.enableHint')} checked={s.enabled} onChange={(v) => set({ enabled: v, defaultEngine: v ? s.defaultEngine : 'piper', locked: v && s.locked })} />
            <div className="flex gap-3 rounded-2xl border border-dashed border-accent/40 bg-accent/6 p-4 text-sm text-ink-2">
              <ShieldCheck className="mt-0.5 size-5 flex-none text-accent" aria-hidden /><p>{t('eleven.privacy')}</p>
            </div>
          </div>
          <div className="flex flex-col gap-4">
            <div className="flex flex-col gap-1.5">
              {data.keySet && !editKey ? (
                <>
                  <span className="text-sm font-medium text-ink-2">{t('eleven.apiKey')}</span>
                  <div className="flex flex-wrap gap-2">
                    <span className="inline-flex min-h-11 min-w-0 flex-1 items-center gap-2 rounded-[10px] border border-line bg-field px-3.5 font-mono text-sm text-ink-2">
                      <KeyRound className="size-4 text-accent" aria-hidden />sk_••••••••••••{data.keyHint.replace('…', '')}
                    </span>
                    <Button variant="secondary" onClick={() => setEditKey(true)}>{t('eleven.replace')}</Button>
                    <Button variant="ghost" busy={busy === 'key'} onClick={() => saveKey('')}>{t('common.remove')}</Button>
                  </div>
                </>
              ) : (
                <>
                  <Input label={t('eleven.apiKey')} type="password" autoComplete="off" value={key} onChange={(e) => setKey(e.target.value)} placeholder="sk_…" />
                  <div className="flex flex-wrap gap-2">
                    <Button variant="secondary" busy={busy === 'key'} disabled={!key.trim()} onClick={() => saveKey(key)}><Check className="size-4" aria-hidden />{t('eleven.saveKey')}</Button>
                    {data.keySet && <Button variant="ghost" onClick={() => { setEditKey(false); setKey('') }}>{t('common.cancel')}</Button>}
                  </div>
                </>
              )}
              <p className="text-xs text-subtle">{t('eleven.apiKeyHint')}</p>
            </div>
            {data.keySet && (
              <div className="flex flex-wrap items-center gap-3">
                <Button variant="secondary" busy={busy === 'test'} onClick={test}><Check className="size-4" aria-hidden />{t('eleven.test')}</Button>
                {usage && checked && <span className="text-xs text-subtle">{t('eleven.testResult', { tier: usage.tier.charAt(0).toUpperCase() + usage.tier.slice(1), time: checked })}</span>}
              </div>
            )}
            <ErrorMessage error={error} />
            {usage && usage.characterLimit > 0 && (
              <div className="flex flex-col gap-2">
                <div className="flex flex-wrap justify-between gap-2 text-sm"><span className="font-medium text-ink">{t('eleven.usage')}</span>
                  <span className="font-mono text-muted tabular-nums">{nf(usage.characterCount)} / {nf(usage.characterLimit)}</span></div>
                <div className="h-2 overflow-hidden rounded-full bg-tint/8" role="meter" aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct} aria-label={t('eleven.usage')}>
                  <div className={`h-full rounded-full ${pct >= 80 ? 'bg-warn' : 'bg-gradient-to-r from-accent to-accent-2'}`} style={{ width: `${pct}%` }} />
                </div>
                <span className="text-xs text-subtle">{reset ? t('eleven.usageHint', { date: reset }) : t('eleven.usageHintNoDate')}</span>
              </div>
            )}
          </div>
        </div>
      </Card>
      <Card>
        <SectionTitle>
          <span className="inline-flex flex-wrap items-center gap-2">{t('eleven.profile')}{locked && <Badge tone="accent"><Lock className="mr-1 inline size-3" aria-hidden />{t('eleven.lockedBadge')}</Badge>}</span>
        </SectionTitle>
        <div className="flex flex-col gap-4">
          <p className="text-sm text-muted">{t('eleven.profileIntro')}</p>
          <Toggle name="locked" label={t('eleven.lock')} hint={lockable || s.locked ? t('eleven.lockHint') : t('eleven.lockNeeds')} checked={s.locked}
            disabled={!lockable && !s.locked} onChange={(v) => set({ locked: v, defaultEngine: v ? 'elevenlabs' : s.defaultEngine })} />
          <div className="grid gap-x-6 gap-y-4 lg:grid-cols-2">
            <div className="flex flex-col gap-4">
              {(['nl', 'en'] as const).map((l) => (
                <div key={l} className="flex flex-col gap-1.5">
                  <span className="text-sm font-medium text-ink-2">{t(l === 'nl' ? 'eleven.voiceNl' : 'eleven.voiceEn')}{l === data.pbxLanguage && <span className="ml-2 text-[11px] font-semibold uppercase tracking-wide text-accent">{t('voices.pbxLanguage')}</span>}</span>
                  {data.keySet ? <VoiceBox voice={l === 'nl' ? s.voiceNl : s.voiceEn} language={l} onChange={(v) => set(l === 'nl' ? { voiceNl: v } : { voiceEn: v })} />
                    : <p className="rounded-xl border border-line bg-inset px-3 py-2.5 text-sm text-muted">{t('eleven.keyFirst')}</p>}
                </div>
              ))}
              <Select label={t('eleven.model')} hint={t('eleven.modelHint')} value={s.model} onChange={(e) => set({ model: e.target.value })}>
                {data.models.map((m) => <option key={m.id} value={m.id}>{t(`eleven.models.${m.id}`, { defaultValue: m.name })}</option>)}
              </Select>
              {locked ? (
                <p className="rounded-xl border border-line bg-inset px-3 py-2.5 text-xs text-muted">{t('eleven.lockedEngine')}</p>
              ) : (
                <div role="radiogroup" aria-label={t('eleven.defaultEngine')} className="flex flex-col gap-1.5">
                  <span className="text-sm font-medium text-ink-2">{t('eleven.defaultEngine')}</span>
                  <div className="grid grid-cols-2 gap-2">
                    {(['piper', 'elevenlabs'] as const).map((e) => {
                      const on = s.defaultEngine === e
                      const avail = e === 'piper' || (s.enabled && data.keySet)
                      return (
                        <button key={e} type="button" role="radio" aria-checked={on} disabled={!avail} onClick={() => set({ defaultEngine: e })}
                          className={`rounded-xl border px-3 py-2.5 text-left transition disabled:cursor-not-allowed disabled:opacity-45 ${on ? 'border-accent bg-accent/8 shadow-[inset_0_0_0_1px_var(--color-accent)]' : 'border-line bg-field hover:border-accent/40'}`}>
                          <span className="block text-sm font-semibold text-ink">{t(`tts.engines.${e}`)}</span>
                          <span className="block text-xs text-subtle">{t(`eleven.engineHints.${e}`)}</span>
                        </button>
                      )
                    })}
                  </div>
                </div>
              )}
            </div>
            <div className="flex flex-col gap-4">
              <span className="text-sm font-semibold text-ink">{t('eleven.voiceSettings')}</span>
              <Range label={t('eleven.stability')} hint={t('eleven.stabilityHint')} min={0} max={100} value={s.stability} unit="%" onChange={(v) => set({ stability: v })} />
              <Range label={t('eleven.similarity')} hint={t('eleven.similarityHint')} min={0} max={100} value={s.similarity} unit="%" onChange={(v) => set({ similarity: v })} />
              <Range label={t('eleven.style')} hint={t('eleven.styleHint')} min={0} max={100} value={s.style} unit="%" onChange={(v) => set({ style: v })} />
              <Range label={t('eleven.speed')} hint={t('eleven.speedHint')} min={70} max={120} value={s.speed} unit="%" onChange={(v) => set({ speed: v })} />
              <Toggle label={t('eleven.speakerBoost')} hint={t('eleven.speakerBoostHint')} checked={s.speakerBoost} onChange={(v) => set({ speakerBoost: v })} />
              <Toggle label={t('eleven.fallback')} hint={t('eleven.fallbackHint')} checked={s.fallback} onChange={(v) => set({ fallback: v })} />
            </div>
          </div>
        </div>
      </Card>
    </div>
  )
}

function Range({ label, hint, min, max, value, unit, onChange }: { label: string; hint: string; min: number; max: number; value: number; unit: string; onChange: (v: number) => void }) {
  const id = label.replace(/\W/g, '')
  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex justify-between text-sm"><label htmlFor={id} className="font-medium text-ink-2">{label}</label><span className="font-mono text-muted tabular-nums">{value}{unit}</span></div>
      <input id={id} type="range" min={min} max={max} value={value} onChange={(e) => onChange(Number(e.target.value))} className="w-full accent-[var(--color-accent)]" />
      <p className="text-xs text-subtle">{hint}</p>
    </div>
  )
}
