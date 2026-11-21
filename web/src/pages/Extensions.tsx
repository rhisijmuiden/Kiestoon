import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { KeyRound, Mail, Pencil, Plus, Smartphone, Trash2, User } from 'lucide-react'
import { Badge, CopyField, Empty, Modal, Select, Table, Toggle, td, useApi } from '../components/kit'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { WelcomeNotice } from '../components/WelcomeNotice'
import { ExtensionPhones } from '../components/PhoneLink'
import { api } from '../lib/api'
import { blankFeatures, type Extension, type Live, type WelcomeResult } from '../lib/types'

const blank: Extension = {
  id: 0, number: '', name: '', email: '', transport: 'udp', srtp: false, maxContacts: 3, outboundCallerId: '',
  ringSeconds: 25, voicemail: true, voicemailEmail: true, voicemailDelete: false, enabled: true,
  features: blankFeatures, recordMode: 'ondemand', pickupGroup: '1', outboundClass: 'national', allowPremium: false, webrtc: true,
}

export function Extensions() {
  const { t } = useTranslation()
  const list = useApi<Extension[]>('/pbx/extensions')
  const live = useApi<Live>('/live', 5000)
  const [edit, setEdit] = useState<Extension | null>(null)
  const [details, setDetails] = useState<(Extension & { welcome?: WelcomeResult }) | null>(null)
  const [notice, setNotice] = useState<WelcomeResult | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [phonesOf, setPhonesOf] = useState<Extension | null>(null)

  async function openNew() {
    const next = await api<{ number: string }>('GET', '/pbx/extensions/next').catch(() => ({ number: '' }))
    setEdit({ ...blank, number: next.number })
  }
  async function openEdit(id: number) {
    setEdit(await api<Extension>('GET', `/pbx/extensions/${id}`))
  }
  async function openDetails(id: number) {
    setDetails(await api<Extension>('GET', `/pbx/extensions/${id}`))
  }
  async function remove(e: Extension) {
    if (!window.confirm(t('extensions.confirmDelete', { number: e.number, name: e.name }))) return
    await api('DELETE', `/pbx/extensions/${e.id}`)
    list.reload()
  }
  async function resendWelcome(e: Extension) {
    if (!window.confirm(t('welcome.confirm', { email: e.email }))) return
    setError(null)
    setNotice(null)
    try {
      setNotice(await api<WelcomeResult>('POST', `/pbx/extensions/${e.id}/welcome`, {}))
    } catch (err) {
      setError(err)
    }
  }
  const online = new Set((live.data?.contacts ?? []).filter((c) => c.status !== 'Unavailable').map((c) => c.endpoint))

  return (
    <>
      <PageHeader title={t('extensions.title')} subtitle={t('extensions.subtitle')}>
        <Button onClick={openNew}>
          <Plus className="size-4" aria-hidden /> {t('extensions.add')}
        </Button>
      </PageHeader>
      <ErrorMessage error={list.error || error} />
      {notice && <WelcomeNotice result={notice} />}
      <Card>
        <Table
          head={[t('extensions.number'), t('extensions.name'), t('extensions.status'), t('extensions.voicemail'), t('extensions.security'), '']}
          empty={list.data?.length === 0 && <Empty icon={<User className="size-7" />} title={t('extensions.emptyTitle')} text={t('extensions.emptyText')} />}
        >
          {list.data?.map((e) => (
            <tr key={e.id} className={e.enabled ? '' : 'opacity-50'}>
              <td className={`${td} font-mono text-base text-ink`}>{e.number}</td>
              <td className={td}>
                <div className="text-ink">{e.name}</div>
                <div className="text-xs text-subtle">{e.email}</div>
              </td>
              <td className={td}>
                {online.has(e.number) ? <Badge tone="ok" dot>{t('extensions.online')}</Badge> : <Badge dot>{t('extensions.offline')}</Badge>}
              </td>
              <td className={td}>{e.voicemail ? <Badge tone="accent">{e.voicemailEmail ? t('extensions.vmEmail') : t('extensions.vmOn')}</Badge> : '–'}</td>
              <td className={td}>
                <span className="font-mono text-xs uppercase">{e.transport}</span>
                {e.srtp && <Badge tone="ok">SRTP</Badge>}
              </td>
              <td className={`${td} text-right whitespace-nowrap`}>
                <button type="button" onClick={() => setPhonesOf(e)} title={t('phoneLink.extTitle')} aria-label={t('phoneLink.extTitle')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-accent">
                  <Smartphone className="size-4" aria-hidden />
                </button>
                <button type="button" onClick={() => openDetails(e.id)} title={t('extensions.sipDetails')} aria-label={t('extensions.sipDetails')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-accent">
                  <KeyRound className="size-4" aria-hidden />
                </button>
                {e.email && (
                  <button type="button" onClick={() => resendWelcome(e)} title={t('welcome.resend')} aria-label={t('welcome.resend')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-accent">
                    <Mail className="size-4" aria-hidden />
                  </button>
                )}
                <button type="button" onClick={() => openEdit(e.id)} title={t('common.edit')} aria-label={t('common.edit')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-ink">
                  <Pencil className="size-4" aria-hidden />
                </button>
                <button type="button" onClick={() => remove(e)} title={t('common.delete')} aria-label={t('common.delete')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-danger/10 hover:text-danger">
                  <Trash2 className="size-4" aria-hidden />
                </button>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
      {edit && <ExtensionForm value={edit} onClose={() => setEdit(null)} onSaved={(e) => { setEdit(null); list.reload(); if (!edit.id) setDetails(e) }} />}
      {phonesOf && <ExtensionPhones ext={phonesOf} onClose={() => setPhonesOf(null)} />}
      {details && <SipDetails ext={details} onClose={() => setDetails(null)} onChanged={setDetails} />}
    </>
  )
}

function ExtensionForm({ value, onClose, onSaved }: { value: Extension; onClose: () => void; onSaved: (e: Extension & { welcome?: WelcomeResult }) => void }) {
  const { t, i18n } = useTranslation()
  const [e, setE] = useState<Extension>(value)
  const [welcome, setWelcome] = useState(true)
  const [lang, setLang] = useState<'en' | 'nl'>(i18n.language === 'nl' ? 'nl' : 'en')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const set = <K extends keyof Extension>(k: K, v: Extension[K]) => setE((x) => ({ ...x, [k]: v }))

  async function save() {
    setBusy(true)
    setError(null)
    try {
      const path = e.id ? `/pbx/extensions/${e.id}` : welcome && e.email ? `/pbx/extensions?welcome=1&lang=${lang}` : '/pbx/extensions'
      const saved = await api<Extension & { welcome?: WelcomeResult }>(e.id ? 'PUT' : 'POST', path, e)
      onSaved(saved)
    } catch (err) {
      setError(err)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      title={e.id ? t('extensions.edit', { number: value.number }) : t('extensions.add')}
      onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={save}>{t('common.save')}</Button></>}
    >
      <ErrorMessage error={error} />
      <div className="grid gap-4 sm:grid-cols-[140px_1fr]">
        <Input name="number" label={t('extensions.number')} value={e.number} onChange={(x) => set('number', x.target.value.replace(/\D/g, ''))} inputMode="numeric" className="font-mono" />
        <Input name="name" label={t('extensions.name')} value={e.name} onChange={(x) => set('name', x.target.value)} autoFocus />
      </div>
      <Input name="email" label={t('extensions.email')} type="email" value={e.email} onChange={(x) => set('email', x.target.value)} hint={t('extensions.emailHint')} />
      {!e.id && (
        <div className="grid gap-3 rounded-xl border border-accent/30 bg-accent/5 p-4">
          <Toggle label={t('welcome.toggle')} hint={e.email ? t('welcome.toggleHint') : t('welcome.needsEmail')} checked={welcome && !!e.email} disabled={!e.email} onChange={setWelcome} />
          {welcome && !!e.email && (
            <Select label={t('welcome.language')} value={lang} onChange={(x) => setLang(x.target.value as 'en' | 'nl')}>
              <option value="en">English</option>
              <option value="nl">Nederlands</option>
            </Select>
          )}
        </div>
      )}
      <div className="grid gap-4 sm:grid-cols-2">
        <Select name="transport" label={t('extensions.transport')} value={e.transport} onChange={(x) => { const v = x.target.value as Extension['transport']; setE((o) => ({ ...o, transport: v, srtp: v === 'tls' ? o.srtp : false })) }}>
          <option value="udp">UDP</option>
          <option value="tcp">TCP</option>
          <option value="tls">TLS ({t('extensions.encrypted')})</option>
        </Select>
        <Input name="ringSeconds" label={t('extensions.ringSeconds')} type="number" min={5} max={300} value={e.ringSeconds} onChange={(x) => set('ringSeconds', Number(x.target.value))} />
      </div>
      <Toggle name="srtp" label={t('extensions.srtp')} hint={t('extensions.srtpHint')} checked={e.srtp} disabled={e.transport !== 'tls'} onChange={(v) => set('srtp', v)} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Input name="outboundCallerId" label={t('extensions.outboundCid')} value={e.outboundCallerId} onChange={(x) => set('outboundCallerId', x.target.value)} hint={t('extensions.outboundCidHint')} className="font-mono" />
        <Input name="maxContacts" label={t('extensions.maxContacts')} type="number" min={1} max={10} value={e.maxContacts} onChange={(x) => set('maxContacts', Number(x.target.value))} hint={t('extensions.maxContactsHint')} />
      </div>
      <Toggle name="voicemail" label={t('extensions.voicemail')} checked={e.voicemail} onChange={(v) => set('voicemail', v)} />
      {e.voicemail && (
        <div className="grid gap-3 rounded-xl border border-line p-4">
          <Input name="voicemailPin" label={t('extensions.pin')} value={e.voicemailPin ?? ''} onChange={(x) => set('voicemailPin', x.target.value.replace(/\D/g, ''))} inputMode="numeric" className="font-mono" hint={t('extensions.pinHint')} />
          <Toggle name="voicemailEmail" label={t('extensions.vmToEmail')} hint={e.email ? e.email : t('extensions.vmNeedsEmail')} checked={e.voicemailEmail && !!e.email} disabled={!e.email} onChange={(v) => set('voicemailEmail', v)} />
          <Toggle name="voicemailDelete" label={t('extensions.vmDelete')} hint={t('extensions.vmDeleteHint')} checked={e.voicemailDelete} disabled={!e.voicemailEmail} onChange={(v) => set('voicemailDelete', v)} />
        </div>
      )}
      <div className="grid gap-4 sm:grid-cols-3">
        <Select name="outboundClass" label={t('extensions.outboundClass')} value={e.outboundClass} onChange={(x) => set('outboundClass', x.target.value as Extension['outboundClass'])} hint={t('extensions.outboundClassHint')}>
          {(['internal', 'national', 'international'] as const).map((c) => <option key={c} value={c}>{t(`extensions.classes.${c}`)}</option>)}
        </Select>
        <Select name="recordMode" label={t('extensions.recordMode')} value={e.recordMode} onChange={(x) => set('recordMode', x.target.value as Extension['recordMode'])} hint={t('extensions.recordModeHint')}>
          {(['never', 'ondemand', 'always'] as const).map((c) => <option key={c} value={c}>{t(`extensions.recordModes.${c}`)}</option>)}
        </Select>
        <Input name="pickupGroup" label={t('extensions.pickupGroup')} value={e.pickupGroup} onChange={(x) => set('pickupGroup', x.target.value.replace(/[^0-9,]/g, ''))} className="font-mono" hint={t('extensions.pickupGroupHint')} />
      </div>
      <Toggle name="allowPremium" label={t('extensions.allowPremium')} hint={t('extensions.allowPremiumHint')} checked={e.allowPremium} onChange={(v) => set('allowPremium', v)} />
      <Toggle name="webrtc" label={t('extensions.webrtc')} hint={t('extensions.webrtcHint')} checked={e.webrtc} onChange={(v) => set('webrtc', v)} />
      <Toggle name="enabled" label={t('common.enabled')} checked={e.enabled} onChange={(v) => set('enabled', v)} />
    </Modal>
  )
}

function SipDetails({ ext, onClose, onChanged }: { ext: Extension & { welcome?: WelcomeResult }; onClose: () => void; onChanged: (e: Extension) => void }) {
  const { t } = useTranslation()
  const host = window.location.hostname
  const [busy, setBusy] = useState(false)
  async function reset() {
    if (!window.confirm(t('extensions.resetConfirm'))) return
    setBusy(true)
    try {
      onChanged(await api<Extension>('POST', `/pbx/extensions/${ext.id}/secret`))
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={t('extensions.sipDetails')} subtitle={`${ext.number} · ${ext.name}`} onClose={onClose}
      footer={<Button variant="danger" busy={busy} onClick={reset}>{t('extensions.resetSecret')}</Button>}>
      {ext.welcome && <WelcomeNotice result={ext.welcome} />}
      <p className="text-sm text-muted">{t('extensions.sipDetailsText')}</p>
      <div className="grid gap-3 sm:grid-cols-2">
        <CopyField label={t('extensions.server')} value={host} />
        <CopyField label={t('extensions.port')} value={ext.transport === 'tls' ? '5061 (TLS)' : `5060 (${ext.transport.toUpperCase()})`} />
        <CopyField label={t('extensions.username')} value={ext.number} />
        <CopyField label={t('extensions.password')} value={ext.sipSecret ?? ''} secret />
        {ext.voicemail && <CopyField label={t('extensions.pin')} value={ext.voicemailPin ?? ''} secret />}
        <CopyField label={t('extensions.vmNumber')} value="*97" />
      </div>
    </Modal>
  )
}
