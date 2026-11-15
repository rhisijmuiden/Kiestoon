import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AudioLines, CloudDownload, Loader2, MessageSquareText, Play, Trash2 } from 'lucide-react'
import { Badge, SectionTitle, Select, Table, Textarea, Toggle, formatBytes, td, useApi } from '../components/kit'
import { IconButton } from '../components/pbx'
import { Alert, Button, Card, ErrorMessage, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import { wavFromBase64 } from '../lib/audio'
import type { SpeechData, SpeechItem, SpeechSettings } from '../lib/types'

/** Local text-to-speech (Piper) and speech-to-text (whisper.cpp). Nothing leaves the server. */
export function Speech() {
  const { t } = useTranslation()
  const [poll, setPoll] = useState(false)
  const d = useApi<SpeechData>('/speech', poll ? 2000 : undefined)
  const [s, setS] = useState<SpeechSettings | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
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
      <div className="grid gap-4 xl:grid-cols-2">
        <Card>
          <SectionTitle><span className="inline-flex items-center gap-2"><AudioLines className="size-5 text-accent" aria-hidden />{t('speech.tts')}</span></SectionTitle>
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted">{t('speech.ttsIntro')}</p>
            <Toggle label={t('speech.ttsEnable')} checked={s.tts.enabled} disabled={!data.ttsInstalled} onChange={(v) => setTTS({ enabled: v })} />
            <div className="grid gap-4 sm:grid-cols-2">
              <Select label={t('speech.voiceEn')} value={s.tts.voiceEn} onChange={(e) => setTTS({ voiceEn: e.target.value })}>
                {voices('en').map((v) => <option key={v.id} value={v.id}>{v.name}</option>)}
              </Select>
              <Select label={t('speech.voiceNl')} value={s.tts.voiceNl} onChange={(e) => setTTS({ voiceNl: e.target.value })}>
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
            <Toggle label={t('speech.sttEnable')} checked={s.stt.enabled} disabled={!data.sttInstalled} onChange={(v) => setSTT({ enabled: v })} />
            <div className="grid gap-4 sm:grid-cols-2">
              <Select label={t('speech.model')} value={s.stt.model} onChange={(e) => setSTT({ model: e.target.value })}>
                {data.models.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}
              </Select>
              <Select label={t('speech.language')} value={s.stt.language} onChange={(e) => setSTT({ language: e.target.value as SpeechSettings['stt']['language'] })}>
                <option value="auto">{t('speech.auto')}</option><option value="nl">Nederlands</option><option value="en">English</option>
              </Select>
            </div>
            <Toggle label={t('speech.voicemail')} hint={t('speech.voicemailHint')} checked={s.stt.voicemail} onChange={(v) => setSTT({ voicemail: v })} />
            <Items items={data.models} onDownload={download} onRemove={remove} />
            <p className="text-xs text-subtle">{t('speech.sttHint')}</p>
          </div>
        </Card>
      </div>
    </>
  )
}

function Items({ items, onDownload, onRemove }: { items: SpeechItem[]; onDownload: (id: string) => void; onRemove: (i: SpeechItem) => void }) {
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
        <select aria-label={t('speech.language')} value={lang} onChange={(e) => setLang(e.target.value)} className="min-h-10 rounded-lg border border-line bg-[#0A1224] px-2 text-sm">
          <option value="nl">Nederlands</option><option value="en">English</option>
        </select>
        <Button variant="secondary" busy={busy} disabled={!text.trim()} onClick={speak}><Play className="size-4" aria-hidden />{t('speech.listen')}</Button>
        {url && <audio controls autoPlay src={url} className="h-9 max-w-full" />}
      </div>
      <ErrorMessage error={error} />
    </div>
  )
}
