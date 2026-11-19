import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Trash2, Voicemail as VoicemailIcon } from 'lucide-react'
import { Badge, Empty, SectionTitle, Table, Toggle, formatDate, formatDuration, td, useApi } from '../../components/kit'
import { AudioInput, IconButton } from '../../components/pbx'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../../components/ui'
import { api } from '../../lib/api'
import type { Wav } from '../../lib/audio'
import type { PortalMe, VoicemailMessage } from '../../lib/types'
import { SummaryView } from '../ai/Summary'

const KINDS = ['unavail', 'busy', 'greet', 'temp'] as const

export function Voicemail() {
  const { t, i18n } = useTranslation()
  const me = useApi<PortalMe>('/portal/me')
  const msgs = useApi<VoicemailMessage[]>('/portal/voicemail', 30000)
  async function played(m: VoicemailMessage) {
    if (m.folder !== 'INBOX') return
    await api('POST', `/portal/voicemail/INBOX/${m.id}/read`).catch(() => undefined)
    msgs.reload()
  }
  async function remove(m: VoicemailMessage) {
    if (!window.confirm(t('voicemail.confirmDelete'))) return
    await api('DELETE', `/portal/voicemail/${m.folder}/${m.id}`)
    msgs.reload()
  }
  if (me.data && !me.data.voicemail) {
    return <><PageHeader title={t('voicemail.title')} /><Card><Empty icon={<VoicemailIcon className="size-7" />} title={t('voicemail.disabled')} text={t('voicemail.disabledText')} /></Card></>
  }
  return (
    <>
      <PageHeader title={t('voicemail.title')} subtitle={t('voicemail.subtitle')} />
      <ErrorMessage error={msgs.error || me.error} />
      <Card>
        <Table head={['', t('voicemail.from'), t('history.time'), t('history.duration'), t('voicemail.listen'), '']}
          empty={msgs.data?.length === 0 && <Empty icon={<VoicemailIcon className="size-7" />} title={t('voicemail.emptyTitle')} text={t('voicemail.emptyText')} />}>
          {msgs.data?.map((m) => (
            <tr key={`${m.folder}-${m.id}`}>
              <td className={td}>{m.folder === 'INBOX' ? <Badge tone="accent" dot>{t('voicemail.new')}</Badge> : <Badge>{t('voicemail.heard')}</Badge>}</td>
              <td className={`${td} text-ink`}>
                {m.callerId.replace(/"/g, '') || t('reception.unknownCaller')}
                {m.summary && <div className="mt-2 max-w-md font-normal"><SummaryView summary={m.summary} compact /></div>}
                {m.transcript && <p className="mt-1 max-w-md text-xs font-normal italic text-muted" title={t('voicemail.transcript')}>“{m.transcript}”</p>}
              </td>
              <td className={`${td} whitespace-nowrap font-mono text-xs`}>{formatDate(m.date, i18n.language)}</td>
              <td className={`${td} font-mono`}>{formatDuration(m.duration)}</td>
              <td className={td}><audio controls preload="none" src={`/api/portal/voicemail/${m.folder}/${m.id}/audio`} onEnded={() => played(m)} className="h-9 w-56 max-w-full" /></td>
              <td className={`${td} text-right`}><IconButton label={t('common.delete')} danger onClick={() => remove(m)}><Trash2 className="size-4" aria-hidden /></IconButton></td>
            </tr>
          ))}
        </Table>
      </Card>
      <div className="grid gap-6 lg:grid-cols-2">
        <Greetings />
        {me.data && <VoicemailSettings me={me.data} onSaved={me.reload} />}
      </div>
    </>
  )
}

function Greetings() {
  const { t } = useTranslation()
  const g = useApi<Record<string, boolean>>('/portal/greetings')
  const [kind, setKind] = useState<(typeof KINDS)[number] | null>(null)
  const [wav, setWav] = useState<Wav | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [v, setV] = useState(0)
  async function save() {
    if (!kind || !wav) return
    setBusy(true)
    setError(null)
    try {
      await api('PUT', `/portal/greetings/${kind}`, { audio: wav.base64 })
      setKind(null)
      setWav(null)
      setV(v + 1)
      g.reload()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  async function remove(k: string) {
    await api('DELETE', `/portal/greetings/${k}`)
    g.reload()
  }
  return (
    <Card className="flex flex-col gap-3">
      <SectionTitle>{t('voicemail.greetings')}</SectionTitle>
      <p className="-mt-2 text-sm text-muted">{t('voicemail.greetingsHint')}</p>
      <ErrorMessage error={error} />
      {KINDS.map((k) => (
        <div key={k} className="flex flex-col gap-3 rounded-xl border border-line p-3">
          <div className="flex flex-wrap items-center gap-3">
            <div className="flex-1">
              <div className="text-sm font-medium text-ink">{t(`voicemail.kinds.${k}`)}</div>
              <div className="text-xs text-subtle">{t(`voicemail.kindHints.${k}`)}</div>
            </div>
            {g.data?.[k] ? <audio controls preload="none" src={`/api/portal/greetings/${k}?v=${v}`} className="h-9 w-48 max-w-full" /> : <Badge>{t('voicemail.standard')}</Badge>}
            <Button variant="ghost" onClick={() => { setKind(kind === k ? null : k); setWav(null) }}>{g.data?.[k] ? t('voicemail.replace') : t('voicemail.record')}</Button>
            {g.data?.[k] && <IconButton label={t('common.delete')} danger onClick={() => remove(k)}><Trash2 className="size-4" aria-hidden /></IconButton>}
          </div>
          {kind === k && (
            <>
              <AudioInput value={wav} onChange={setWav} />
              <div className="flex justify-end"><Button busy={busy} disabled={!wav} onClick={save}>{t('common.save')}</Button></div>
            </>
          )}
        </div>
      ))}
    </Card>
  )
}

function VoicemailSettings({ me, onSaved }: { me: PortalMe; onSaved: () => void }) {
  const { t } = useTranslation()
  const [pin, setPin] = useState('')
  const [email, setEmail] = useState(me.voicemailEmail)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  async function save() {
    setBusy(true)
    setError(null)
    setSaved(false)
    try {
      await api('PUT', '/portal/voicemail', { pin, email })
      setPin('')
      setSaved(true)
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Card className="flex flex-col gap-4 self-start">
      <SectionTitle>{t('voicemail.settings')}</SectionTitle>
      <ErrorMessage error={error} />
      {saved && <Alert tone="ok">{t('common.saved')}</Alert>}
      <Input name="pin" label={t('voicemail.newPin')} value={pin} onChange={(e) => setPin(e.target.value.replace(/\D/g, ''))} inputMode="numeric" className="font-mono" hint={t('voicemail.newPinHint')} />
      <Toggle label={t('extensions.vmToEmail')} hint={me.email || t('extensions.vmNeedsEmail')} checked={email && !!me.email} disabled={!me.email} onChange={setEmail} />
      <p className="text-xs text-subtle">{t('voicemail.dialIn')}</p>
      <div className="flex justify-end"><Button busy={busy} onClick={save}>{t('common.save')}</Button></div>
    </Card>
  )
}
