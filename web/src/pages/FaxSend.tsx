import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Download, FileText, MailCheck, RotateCcw, Send, Trash2 } from 'lucide-react'
import { Badge, Empty, SectionTitle, Select, Table, Toggle, formatDate, td, useApi } from '../components/kit'
import { IconButton } from '../components/pbx'
import { Alert, Button, Card, ErrorMessage, Input } from '../components/ui'
import { api } from '../lib/api'
import type { FaxJob, FaxMail, FaxSendBox } from '../lib/types'

const MAX_PDF = 11 * 1024 * 1024

function readBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const r = new FileReader()
    r.onload = () => resolve(String(r.result).replace(/^data:[^,]*,/, ''))
    r.onerror = () => reject(r.error)
    r.readAsDataURL(file)
  })
}

const statusTone = { queued: 'muted', sending: 'accent', success: 'ok', failed: 'danger' } as const

/** Sends a PDF as a fax and lists the outgoing faxes. */
export function SendFax({ admin }: { admin: boolean }) {
  const { t, i18n } = useTranslation()
  const boxes = useApi<FaxSendBox[]>('/fax/send/boxes')
  const jobs = useApi<FaxJob[]>('/fax/jobs', 5000)
  const [boxId, setBoxId] = useState(0)
  const [number, setNumber] = useState('')
  const [file, setFile] = useState<File | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [sent, setSent] = useState('')
  useEffect(() => { if (!boxId && boxes.data?.length) setBoxId(boxes.data[0].id) }, [boxes.data, boxId])

  async function send() {
    if (!file) return
    setBusy(true)
    setError(null)
    setSent('')
    try {
      const pdf = await readBase64(file)
      await api('POST', '/fax/send', { boxId, number, name: file.name, pdf })
      setSent(number)
      setNumber('')
      setFile(null)
      jobs.reload()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  async function act(j: FaxJob, action: 'retry' | 'delete') {
    setError(null)
    try {
      if (action === 'retry') await api('POST', `/fax/jobs/${j.id}/retry`)
      else {
        if (!window.confirm(t('fax.send.confirmDelete'))) return
        await api('DELETE', `/fax/jobs/${j.id}`)
      }
      jobs.reload()
    } catch (e) {
      setError(e)
    }
  }
  const tooBig = !!file && file.size > MAX_PDF
  return (
    <>
      <Card>
        <SectionTitle><span className="inline-flex items-center gap-2"><Send className="size-5 text-accent" aria-hidden />{t('fax.send.title')}</span></SectionTitle>
        {boxes.data?.length === 0 ? (
          <p className="text-sm text-muted">{admin ? t('fax.send.noBoxesAdmin') : t('fax.send.noBoxes')}</p>
        ) : (
          <div className="flex flex-col gap-4">
            <p className="max-w-3xl text-sm text-muted">{t('fax.send.intro')}</p>
            <div className="grid gap-4 md:grid-cols-3">
              <Select label={t('fax.send.from')} value={boxId} onChange={(e) => setBoxId(Number(e.target.value))}>
                {boxes.data?.map((b) => <option key={b.id} value={b.id}>{b.name}{b.number ? ` (${b.number})` : ''}</option>)}
              </Select>
              <Input name="number" label={t('fax.send.to')} value={number} onChange={(e) => setNumber(e.target.value)} placeholder="+31201234567" className="font-mono" inputMode="tel" />
              <div className="flex flex-col gap-1.5">
                <span className="text-sm font-medium text-ink-2">{t('fax.send.document')}</span>
                <label className="flex min-h-11 cursor-pointer items-center gap-2 rounded-[10px] border border-dashed border-line bg-field px-3.5 text-sm text-muted hover:border-accent/50 focus-within:ring-2 focus-within:ring-accent/30">
                  <FileText className="size-4 text-accent" aria-hidden />
                  <span className="truncate">{file ? file.name : t('fax.send.choosePdf')}</span>
                  <input type="file" accept="application/pdf,.pdf" className="sr-only" onChange={(e) => setFile(e.target.files?.[0] ?? null)} />
                </label>
              </div>
            </div>
            {tooBig && <Alert tone="warn">{t('fax.send.tooBig')}</Alert>}
            {sent && <Alert tone="ok">{t('fax.send.queued', { number: sent })}</Alert>}
            <ErrorMessage error={error} />
            <div className="flex items-center justify-between gap-3">
              <p className="text-xs text-subtle">{t('fax.send.hint')}</p>
              <Button busy={busy} disabled={!file || !number || !boxId || tooBig} onClick={send}><Send className="size-4" aria-hidden />{t('fax.send.submit')}</Button>
            </div>
          </div>
        )}
      </Card>
      <Card>
        <SectionTitle>{t('fax.send.outbox')}</SectionTitle>
        <ErrorMessage error={jobs.error} />
        <Table head={[t('fax.send.created'), t('fax.send.to'), t('fax.box'), t('fax.send.document'), t('fax.pages'), t('fax.status'), '']}
          empty={jobs.data?.length === 0 && <Empty icon={<Send className="size-7" />} title={t('fax.send.emptyTitle')} text={t('fax.send.emptyText')} />}>
          {jobs.data?.map((j) => (
            <tr key={j.id}>
              <td className={`${td} whitespace-nowrap text-xs`}>{formatDate(j.createdAt, i18n.language)}</td>
              <td className={`${td} font-mono text-ink`}>{j.number}</td>
              <td className={td}>{j.boxName || '–'}</td>
              <td className={td}>
                <span className="block max-w-[220px] truncate">{j.document}</span>
                <span className="text-xs text-subtle">{j.source === 'email' ? t('fax.send.viaMail', { sender: j.sender }) : j.sender}</span>
              </td>
              <td className={`${td} font-mono`}>{j.pages || '–'}</td>
              <td className={td}>
                <Badge tone={statusTone[j.status]} dot>{t(`fax.send.status.${j.status}`)}</Badge>
                {j.attempts > 0 && j.status !== 'success' && <span className="ml-1.5 text-xs text-subtle">{t('fax.send.attempt', { n: j.attempts })}</span>}
                {j.error && <span className="block text-xs text-subtle">{j.error}</span>}
              </td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <a href={`/api/fax/jobs/${j.id}/pdf`} title={t('fax.download')} aria-label={t('fax.download')}
                  className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-accent"><Download className="size-4" aria-hidden /></a>
                {j.status === 'failed' && <IconButton label={t('fax.send.retry')} onClick={() => act(j, 'retry')}><RotateCcw className="size-4" aria-hidden /></IconButton>}
                {admin && j.status !== 'sending' && <IconButton label={t('common.delete')} danger onClick={() => act(j, 'delete')}><Trash2 className="size-4" aria-hidden /></IconButton>}
              </td>
            </tr>
          ))}
        </Table>
      </Card>
    </>
  )
}

/** E-mail to fax: Kiestoon reads a mailbox and faxes the PDF attachments of allowed senders. */
export function FaxMailSettings() {
  const { t, i18n } = useTranslation()
  const d = useApi<FaxMail>('/fax/mail')
  const [m, setM] = useState<FaxMail | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  useEffect(() => { if (d.data && !m) setM(d.data) }, [d.data, m])
  if (!m) return <ErrorMessage error={d.error} />
  const set = <K extends keyof FaxMail>(k: K, v: FaxMail[K]) => { setSaved(false); setM({ ...m, [k]: v }) }
  async function save() {
    setBusy(true)
    setError(null)
    try {
      setM(await api<FaxMail>('PUT', '/fax/mail', m))
      setSaved(true)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <div className="grid gap-4 xl:grid-cols-[3fr_2fr]">
      <Card>
        <SectionTitle><span className="inline-flex items-center gap-2"><MailCheck className="size-5 text-accent" aria-hidden />{t('fax.mail.title')}</span></SectionTitle>
        <div className="flex flex-col gap-4">
          <p className="text-sm text-muted">{t('fax.mail.intro')}</p>
          <Toggle name="enabled" label={t('fax.mail.enable')} checked={m.enabled} onChange={(v) => set('enabled', v)} />
          <div className="grid gap-4 sm:grid-cols-[2fr_1fr_1fr]">
            <Input name="host" label={t('fax.mail.host')} value={m.host} onChange={(e) => set('host', e.target.value.trim())} placeholder="imap.example.com" className="font-mono" />
            <Input name="port" label={t('fax.mail.port')} type="number" value={m.port || ''} onChange={(e) => set('port', Number(e.target.value))} placeholder={m.security === 'tls' ? '993' : '143'} />
            <Select name="security" label={t('fax.mail.security')} value={m.security} onChange={(e) => set('security', e.target.value as FaxMail['security'])}>
              <option value="tls">TLS</option><option value="starttls">STARTTLS</option>
            </Select>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <Input name="username" label={t('fax.mail.username')} value={m.username} onChange={(e) => set('username', e.target.value.trim())} autoComplete="off" placeholder="fax@example.com" />
            <Input name="password" label={t('fax.mail.password')} type="password" value={m.password ?? ''} onChange={(e) => set('password', e.target.value)} autoComplete="new-password" placeholder={m.passwordSet ? t('common.keepSecret') : ''} />
          </div>
          <Input name="folder" label={t('fax.mail.folder')} value={m.folder} onChange={(e) => set('folder', e.target.value)} placeholder="INBOX" className="font-mono" />
          <Toggle name="delete" label={t('fax.mail.delete')} hint={t('fax.mail.deleteHint')} checked={m.delete} onChange={(v) => set('delete', v)} />
          {saved && <Alert tone="ok">{t('fax.mail.saved')}</Alert>}
          <ErrorMessage error={error} />
          <div className="flex justify-end"><Button busy={busy} onClick={save}>{t('common.save')}</Button></div>
        </div>
      </Card>
      <Card>
        <SectionTitle>{t('fax.mail.howTitle')}</SectionTitle>
        <ol className="flex list-decimal flex-col gap-2 pl-5 text-sm text-ink-2">
          <li>{t('fax.mail.how1')}</li>
          <li>{t('fax.mail.how2')}</li>
          <li>{t('fax.mail.how3')}</li>
          <li>{t('fax.mail.how4')}</li>
        </ol>
        <p className="mt-3 text-xs text-subtle">{t('fax.mail.security_note')}</p>
        {m.lastCheck && (
          <p className="mt-4 text-xs text-muted">
            {t('fax.mail.lastCheck', { time: formatDate(m.lastCheck, i18n.language) })}
            {m.lastError && <span className="block text-danger">{m.lastError}</span>}
          </p>
        )}
      </Card>
    </div>
  )
}
