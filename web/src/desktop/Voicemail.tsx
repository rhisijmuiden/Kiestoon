import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Pause, Play, Trash2 } from 'lucide-react'
import { formatDuration } from '../components/kit'
import { ErrorMessage } from '../components/ui'
import { api } from '../lib/api'
import { audioPref } from '../lib/softphone'
import type { VoicemailMessage } from '../lib/types'
import { CallButton, Title } from './Contacts'
import { useDesktop } from './data'

function number(callerId: string) {
  return callerId.match(/<([^>]+)>/)?.[1] ?? callerId.replace(/"/g, '').trim()
}
function name(callerId: string) {
  return callerId.replace(/<.*>/, '').replace(/"/g, '').trim()
}

export function Voicemail() {
  const { t, i18n } = useTranslation()
  const { voicemail } = useDesktop()
  const [playing, setPlaying] = useState<string | null>(null)
  const [error, setError] = useState<unknown>(null)
  const audio = useRef<HTMLAudioElement>(null)
  async function play(m: VoicemailMessage) {
    const key = `${m.folder}-${m.id}`
    const a = audio.current
    if (!a) return
    if (playing === key) {
      a.pause()
      setPlaying(null)
      return
    }
    const sink = audioPref('speaker')
    if (sink) await (a as unknown as { setSinkId?: (id: string) => Promise<void> }).setSinkId?.(sink).catch(() => undefined)
    a.src = `/api/portal/voicemail/${m.folder}/${m.id}/audio`
    a.onended = async () => {
      setPlaying(null)
      if (m.folder === 'INBOX') {
        await api('POST', `/portal/voicemail/INBOX/${m.id}/read`).catch(() => undefined)
        voicemail.reload()
      }
    }
    a.play().then(() => setPlaying(key)).catch(setError)
  }
  async function remove(m: VoicemailMessage) {
    if (!window.confirm(t('voicemail.confirmDelete'))) return
    await api('DELETE', `/portal/voicemail/${m.folder}/${m.id}`).catch(setError)
    voicemail.reload()
  }
  return (
    <div className="pb-3">
      <Title>{t('desktop.nav.voicemail')}</Title>
      <ErrorMessage error={error || voicemail.error} />
      {voicemail.data?.length === 0 && <p className="px-4 py-8 text-center text-sm text-muted">{t('voicemail.emptyTitle')}</p>}
      <audio ref={audio} className="hidden" />
      <ul className="px-2">
        {voicemail.data?.map((m) => {
          const key = `${m.folder}-${m.id}`
          const fresh = m.folder === 'INBOX'
          const nm = name(m.callerId)
          const nr = number(m.callerId)
          return (
            <li key={key} className="flex items-start gap-2.5 rounded-[10px] px-1.5 py-2 hover:bg-surface">
              <button type="button" onClick={() => play(m)} aria-label={playing === key ? t('desktop.stop') : t('voicemail.listen')}
                className={`inline-flex size-9 flex-none items-center justify-center rounded-full ${fresh ? 'bg-accent text-on-accent' : 'bg-surface text-accent border border-line'}`}>
                {playing === key ? <Pause className="size-4" aria-hidden /> : <Play className="size-4" aria-hidden />}
              </button>
              <span className="min-w-0 flex-1">
                <span className={`block truncate text-[13.5px] ${fresh ? 'font-semibold' : 'font-medium'} text-ink`}>{nm || nr || t('reception.unknownCaller')}</span>
                <span className="block truncate text-[11.5px] text-muted">{nm && nr ? `${nr} · ` : ''}{new Date(m.date).toLocaleString(i18n.language, { dateStyle: 'short', timeStyle: 'short' })} · {formatDuration(m.duration)}</span>
                {m.summary && <span className="mt-1 block text-[11.5px] text-ink/90">{m.summary.summary}</span>}
                {m.transcript && !m.summary && <span className="mt-1 block text-[11.5px] italic text-muted">“{m.transcript}”</span>}
              </span>
              {nr && <CallButton number={nr} label={nm || nr} />}
              <button type="button" onClick={() => remove(m)} aria-label={t('common.delete')} title={t('common.delete')}
                className="inline-flex size-[30px] items-center justify-center rounded-[9px] text-subtle hover:bg-danger/10 hover:text-danger"><Trash2 className="size-4" aria-hidden /></button>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
