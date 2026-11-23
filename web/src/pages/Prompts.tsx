import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AudioLines, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { Badge, Empty, Modal, Table, formatDate, td, useApi } from '../components/kit'
import { AudioInput, IconButton } from '../components/pbx'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { SpeakFromText } from '../components/SpeakFromText'
import { LockedProfile, VoiceBox, useTTS } from '../components/Voices'
import { api } from '../lib/api'
import { wavFromBase64, type Wav } from '../lib/audio'
import type { Prompt, PromptSource, VoiceRef } from '../lib/types'

export function Prompts() {
  const { t, i18n } = useTranslation()
  const list = useApi<Prompt[]>('/pbx/prompts')
  const [edit, setEdit] = useState<Prompt | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [sel, setSel] = useState<number[]>([])
  const spoken = (list.data ?? []).filter((p) => p.source)
  const toggle = (id: number) => setSel((v) => (v.includes(id) ? v.filter((x) => x !== id) : [...v, id]))
  async function remove(p: Prompt) {
    if (!window.confirm(t('common.confirmDelete', { name: p.name }))) return
    setError(null)
    try {
      await api('DELETE', `/pbx/prompts/${p.id}`)
      list.reload()
    } catch (e) {
      setError(e)
    }
  }
  return (
    <>
      <PageHeader title={t('prompts.title')} subtitle={t('prompts.subtitle')}>
        <Button onClick={() => setEdit({ id: 0, name: '', durationMs: 0, createdAt: '' })}><Plus className="size-4" aria-hidden /> {t('prompts.add')}</Button>
      </PageHeader>
      <ErrorMessage error={list.error || error} />
      {spoken.length > 0 && <Regenerate prompts={spoken.filter((p) => sel.includes(p.id))} onDone={() => { setSel([]); list.reload() }} />}
      <Card>
        <Table head={[
          spoken.length > 0 ? <input type="checkbox" aria-label={t('prompts.selectAll')} className="size-4 accent-[var(--color-accent)]"
            checked={spoken.length > 0 && spoken.every((p) => sel.includes(p.id))}
            onChange={(e) => setSel(e.target.checked ? spoken.map((p) => p.id) : [])} /> : '',
          t('common.name'), t('prompts.source'), t('prompts.listen'), t('prompts.length'), t('prompts.added'), '']}
          empty={list.data?.length === 0 && <Empty icon={<AudioLines className="size-7" />} title={t('prompts.emptyTitle')} text={t('prompts.emptyText')} />}>
          {list.data?.map((p) => (
            <tr key={p.id}>
              <td className={`${td} w-8`}>{p.source && <input type="checkbox" aria-label={t('prompts.select', { name: p.name })} className="size-4 accent-[var(--color-accent)]" checked={sel.includes(p.id)} onChange={() => toggle(p.id)} />}</td>
              <td className={`${td} text-ink`}>{p.name}{p.source && <span className="block max-w-[38ch] truncate text-xs text-subtle" title={p.source.text}>{p.source.text}</span>}</td>
              <td className={td}><SourceBadge p={p} /></td>
              <td className={td}><audio controls preload="none" src={`/api/pbx/prompts/${p.id}/audio?v=${p.durationMs}`} className="h-9 w-56 max-w-full" /></td>
              <td className={`${td} font-mono`}>{(p.durationMs / 1000).toFixed(1)} s</td>
              <td className={`${td} text-xs`}>{formatDate(p.createdAt, i18n.language)}</td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <IconButton label={t('common.edit')} onClick={() => setEdit(p)}><Pencil className="size-4" aria-hidden /></IconButton>
                <IconButton label={t('common.delete')} danger onClick={() => remove(p)}><Trash2 className="size-4" aria-hidden /></IconButton>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
      {edit && <PromptForm value={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); list.reload() }} />}
    </>
  )
}

function PromptForm({ value, onClose, onSaved }: { value: Prompt; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [name, setName] = useState(value.name)
  const [wav, setWav] = useState<Wav | null>(null)
  const [source, setSource] = useState<PromptSource | undefined>(undefined)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api(value.id ? 'PUT' : 'POST', value.id ? `/pbx/prompts/${value.id}` : '/pbx/prompts', { name, audio: wav?.base64 ?? '', source: wav ? source : undefined })
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={value.id ? t('prompts.edit', { name: value.name }) : t('prompts.add')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} disabled={!name || (!value.id && !wav)} onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <Input name="name" label={t('common.name')} value={name} onChange={(e) => setName(e.target.value)} autoFocus placeholder={t('prompts.namePlaceholder')} />
      {value.id > 0 && <p className="text-sm text-muted">{t('prompts.replaceHint')}</p>}
      <div className="flex flex-col gap-3 rounded-xl border border-accent/25 bg-accent/5 p-4">
        <SpeakFromText initial={value.source} regenerate={!!value.source} onAudio={(r) => { setWav(wavFromBase64(r.audio)); setSource(r.source) }} />
      </div>
      {wav && source && <audio controls src={wav.url} className="h-9 max-w-full" />}
      <AudioInput value={wav} onChange={(w) => { setWav(w); setSource(undefined) }} />
    </Modal>
  )
}

function SourceBadge({ p }: { p: Prompt }) {
  const { t } = useTranslation()
  if (!p.source) return <Badge>{t('prompts.recorded')}</Badge>
  const v = p.source.voiceName || p.source.voice
  return p.source.engine === 'elevenlabs' ? <Badge tone="accent">ElevenLabs · {v}</Badge> : <Badge>Piper · {v}</Badge>
}

/** Speaks the selected prompts again with another voice, from the text they were made from. */
function Regenerate({ prompts, onDone }: { prompts: Prompt[]; onDone: () => void }) {
  const { t, i18n } = useTranslation()
  const tts = useTTS()
  const lang: 'nl' | 'en' = tts.el?.pbxLanguage ?? tts.speech?.pbxLanguage ?? (i18n.language.startsWith('nl') ? 'nl' : 'en')
  const [engine, setEngine] = useState<'piper' | 'elevenlabs' | ''>('')
  const [elVoice, setElVoice] = useState<VoiceRef | null>(null)
  const [piperVoice, setPiperVoice] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [result, setResult] = useState<{ done: number; failed: number } | null>(null)
  if (!tts.loaded || (!tts.piperReady && !tts.elevenReady)) return null
  const locked = tts.elevenReady && !!tts.el?.settings.locked
  const eng = locked ? 'elevenlabs' : engine || (tts.elevenReady ? 'elevenlabs' : 'piper')
  const ev = locked ? { id: 'profile', name: '' } : elVoice ?? (lang === 'nl' ? tts.el!.settings.voiceNl : tts.el!.settings.voiceEn)
  const pv = piperVoice || (tts.piperVoices.find((v) => v.language === lang)?.id ?? '')
  const chars = prompts.reduce((n, p) => n + [...(p.source?.text ?? '')].length, 0)
  async function go() {
    setBusy(true)
    setError(null)
    setResult(null)
    try {
      const r = await api<{ done: number[]; failed: { id: number; error: string }[] }>('POST', '/pbx/prompts/regenerate', eng === 'elevenlabs'
        ? { ids: prompts.map((p) => p.id), engine: eng, voice: locked ? '' : ev.id, voiceName: ev.name }
        : { ids: prompts.map((p) => p.id), engine: eng, voice: pv, voiceName: tts.piperVoices.find((v) => v.id === pv)?.name ?? pv })
      setResult({ done: r.done.length, failed: r.failed.length })
      onDone()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="flex flex-col gap-3 rounded-2xl border border-accent-2/35 bg-accent-2/8 p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="font-medium text-ink">{prompts.length ? t('prompts.selected', { count: prompts.length }) : t('prompts.regenTitle')}</div>
          <div className="text-xs text-muted">{prompts.length ? (eng === 'elevenlabs' ? t('prompts.regenChars', { n: chars }) : t('prompts.regenFree')) : t('prompts.regenHint')}</div>
        </div>
        {prompts.length > 0 && (
          <div className="flex flex-wrap items-center gap-2">
            {locked ? <div className="min-w-72"><LockedProfile compact el={tts.el!} language={lang} /></div> : <>
            {tts.piperReady && tts.elevenReady && (
              <select aria-label={t('tts.engine')} value={eng} onChange={(e) => setEngine(e.target.value as 'piper' | 'elevenlabs')} className="min-h-10 rounded-lg border border-line bg-field px-2 text-sm">
                <option value="elevenlabs">{t('tts.engines.elevenlabs')}</option><option value="piper">{t('tts.engines.piper')}</option>
              </select>
            )}
            {eng === 'elevenlabs' ? <div className="min-w-64"><VoiceBox compact voice={ev} language={lang} onChange={setElVoice} /></div> : (
              <select aria-label={t('tts.voice')} value={pv} onChange={(e) => setPiperVoice(e.target.value)} className="min-h-10 rounded-lg border border-line bg-field px-2 text-sm">
                {tts.piperVoices.map((v) => <option key={v.id} value={v.id}>{v.name}</option>)}
              </select>
            )}
            </>}
            <Button busy={busy} disabled={eng === 'elevenlabs' ? !ev.id : !pv} onClick={go}><RefreshCw className="size-4" aria-hidden />{t('prompts.regenerate')}</Button>
          </div>
        )}
      </div>
      {result && <Alert tone={result.failed ? 'warn' : 'ok'}>{t('prompts.regenResult', { done: result.done, failed: result.failed })}</Alert>}
      <ErrorMessage error={error} />
    </div>
  )
}
