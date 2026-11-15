import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Link2, Link2Off, RefreshCw, Smartphone, X } from 'lucide-react'
import { Badge, Modal, Select, Toggle, formatDate } from './kit'
import { Alert, Button, ErrorMessage, Input } from './ui'
import { vendorName } from '../pages/PhoneExtras'
import { api } from '../lib/api'
import type { DiscoveredPhone, Extension, Phone, PhoneModel, ResyncResult } from '../lib/types'

/** Linking provisioned phones to extensions, shared by the Phones and Extensions pages. */

export const formatMac = (mac: string) => mac.match(/.{2}/g)?.join(':').toUpperCase() ?? mac

export type LinkNotice = { kind: 'adopted' } | ({ kind: 'resync' } & ResyncResult)

/** Tells the admin what happens next on the phone after a change. */
export function LinkNoticeAlert({ notice, onClose }: { notice: LinkNotice; onClose: () => void }) {
  const { t } = useTranslation()
  let tone: 'ok' | 'warn' = 'ok'
  let text = t('phoneLink.adopted')
  if (notice.kind === 'resync') {
    tone = notice.resync === 'sent' || notice.resync === 'skipped' ? 'ok' : 'warn'
    text = t(`phoneLink.resync.${notice.resync}`, { message: notice.message || '' })
  }
  return (
    <div className="relative">
      <Alert tone={tone}>{text}</Alert>
      <button type="button" onClick={onClose} aria-label={t('common.close')} className="absolute right-2 top-2 inline-flex size-7 items-center justify-center rounded-md text-muted hover:text-ink"><X className="size-3.5" /></button>
    </div>
  )
}

const extOption = (e: Extension) => `${e.number} · ${e.name}`

/** Adopt a discovered phone: confirm the model and pick the extension. */
export function AdoptForm({ phone, models, exts, presetExt, onClose, onDone }: {
  phone: DiscoveredPhone; models: PhoneModel[]; exts: Extension[]; presetExt?: number | null
  onClose: () => void; onDone: (n: LinkNotice) => void
}) {
  const { t } = useTranslation()
  const guess = models.find((m) => m.vendor === phone.vendor && m.id === phone.model)
  const [model, setModel] = useState(guess ? `${guess.vendor}/${guess.id}` : '')
  const [ext, setExt] = useState<number | null>(presetExt ?? null)
  const [label, setLabel] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const vendors = [...new Set(models.map((m) => m.vendor))]
  // Grandstream and Htek share file names: without a vendor guess every model stays possible.
  const shown = phone.vendor ? vendors.filter((v) => v === phone.vendor) : vendors
  async function save() {
    const [vendor, id] = model.split('/')
    setBusy(true)
    setError(null)
    try {
      await api('POST', `/phones/discovered/${phone.mac}/adopt`, { vendor, model: id, extensionId: ext, label })
      onDone({ kind: 'adopted' })
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={t('phoneLink.adoptTitle')} subtitle={formatMac(phone.mac)} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} disabled={!model} onClick={save}><Link2 className="size-4" aria-hidden /> {t('phoneLink.link')}</Button></>}>
      <ErrorMessage error={error} />
      <div className="rounded-xl border border-line bg-inset px-3.5 py-3 text-xs text-muted">
        <div className={phone.userAgent ? 'font-mono text-ink-2' : 'text-ink-2'}>{phone.userAgent || t('phoneLink.noUserAgent')}</div>
        <div className="mt-1">{t('phoneLink.seenFrom', { ip: phone.ip, count: phone.hits })}</div>
      </div>
      <Select label={t('phones.model')} value={model} onChange={(e) => setModel(e.target.value)} hint={guess ? t('phoneLink.modelGuessed') : t('phoneLink.modelUnknown')}>
        <option value="">–</option>
        {shown.map((v) => (
          <optgroup key={v} label={vendorName(v)}>
            {models.filter((m) => m.vendor === v).map((m) => <option key={m.id} value={`${v}/${m.id}`}>{m.name}</option>)}
          </optgroup>
        ))}
      </Select>
      <div className="grid gap-4 sm:grid-cols-2">
        <Select label={t('phones.extension')} value={ext ?? ''} onChange={(e) => setExt(e.target.value ? Number(e.target.value) : null)}>
          <option value="">{t('phoneLink.none')}</option>
          {exts.map((e) => <option key={e.id} value={e.id}>{extOption(e)}</option>)}
        </Select>
        <Input label={t('phones.label')} value={label} onChange={(e) => setLabel(e.target.value)} placeholder={t('phones.labelPlaceholder')} />
      </div>
      <p className="text-xs text-subtle">{t('phoneLink.adoptHint')}</p>
    </Modal>
  )
}

/** Link a known phone to another extension, or unlink it. */
export function AssignForm({ phone, exts, onClose, onDone }: {
  phone: Phone; exts: Extension[]; onClose: () => void; onDone: (n: LinkNotice) => void
}) {
  const { t } = useTranslation()
  const [ext, setExt] = useState<number | null>(phone.extensionId)
  const [resync, setResync] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  async function save() {
    setBusy(true)
    setError(null)
    try {
      const r = await api<ResyncResult>('PUT', `/phones/${phone.id}/assign`, { extensionId: ext, resync })
      onDone({ kind: 'resync', ...r })
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={t('phoneLink.assignTitle')} subtitle={`${phone.label || formatMac(phone.mac)} · ${vendorName(phone.vendor)} ${phone.model}`} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} disabled={ext === phone.extensionId} onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <Select label={t('phones.extension')} value={ext ?? ''} onChange={(e) => setExt(e.target.value ? Number(e.target.value) : null)}>
        <option value="">{t('phoneLink.none')}</option>
        {exts.map((e) => <option key={e.id} value={e.id}>{extOption(e)}</option>)}
      </Select>
      <Toggle label={t('phoneLink.resyncToggle')} hint={t('phoneLink.resyncHint')} checked={resync} onChange={setResync} />
    </Modal>
  )
}

/** Ask a phone to fetch its configuration again. */
export function ResyncButton({ phone, onDone }: { phone: Phone; onDone: (n: LinkNotice) => void }) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState(false)
  async function run() {
    setBusy(true)
    try {
      onDone({ kind: 'resync', ...(await api<ResyncResult>('POST', `/phones/${phone.id}/resync`, {})) })
    } catch {
      onDone({ kind: 'resync', resync: 'failed' })
    } finally {
      setBusy(false)
    }
  }
  return (
    <button type="button" onClick={run} disabled={busy} aria-label={t('phoneLink.resyncNow')} title={t('phoneLink.resyncNow')} className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-tint/5 hover:text-accent disabled:opacity-50">
      <RefreshCw className={`size-4 ${busy ? 'animate-spin' : ''}`} />
    </button>
  )
}

/** Phones that wait for an extension: just discovered, or known without an extension. */
export function WaitingPhones({ discovered, unassigned, models, exts, onChanged }: {
  discovered: DiscoveredPhone[]; unassigned: Phone[]; models: PhoneModel[]; exts: Extension[]; onChanged: (n?: LinkNotice) => void
}) {
  const { t, i18n } = useTranslation()
  const [adopt, setAdopt] = useState<{ phone: DiscoveredPhone; ext: number | null } | null>(null)
  const [pick, setPick] = useState<Record<string, number | null>>({})
  const [busy, setBusy] = useState('')
  const [error, setError] = useState<unknown>(null)
  const modelName = (vendor: string, id: string) => models.find((m) => m.vendor === vendor && m.id === id)?.name
  if (discovered.length + unassigned.length === 0) return null

  async function linkKnown(p: Phone) {
    const ext = pick[`p${p.id}`]
    if (!ext) return
    setBusy(`p${p.id}`)
    setError(null)
    try {
      const r = await api<ResyncResult>('PUT', `/phones/${p.id}/assign`, { extensionId: ext, resync: true })
      onChanged({ kind: 'resync', ...r })
    } catch (e) {
      setError(e)
    } finally {
      setBusy('')
    }
  }
  async function linkDiscovered(d: DiscoveredPhone) {
    const ext = pick[d.mac] ?? null
    // Without a confident model guess the admin confirms the model first.
    if (!modelName(d.vendor, d.model)) {
      setAdopt({ phone: d, ext })
      return
    }
    setBusy(d.mac)
    setError(null)
    try {
      await api('POST', `/phones/discovered/${d.mac}/adopt`, { vendor: d.vendor, model: d.model, extensionId: ext, label: '' })
      onChanged({ kind: 'adopted' })
    } catch (e) {
      setError(e)
    } finally {
      setBusy('')
    }
  }
  async function dismiss(d: DiscoveredPhone) {
    if (!window.confirm(t('phoneLink.confirmDismiss', { mac: formatMac(d.mac) }))) return
    await api('DELETE', `/phones/discovered/${d.mac}`)
    onChanged()
  }
  const row = (key: string, title: ReactNode, sub: ReactNode, badge: ReactNode, onLink: () => void, extra?: ReactNode) => (
    <li key={key} className="flex flex-col gap-3 rounded-xl border border-line bg-inset p-3 sm:flex-row sm:items-center">
      <div className="flex min-w-0 flex-1 items-center gap-3">
        <span className="inline-flex size-10 flex-none items-center justify-center rounded-xl bg-accent/10 text-accent"><Smartphone className="size-5" aria-hidden /></span>
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2 text-sm text-ink">{title} {badge}</div>
          <div className="truncate text-xs text-subtle">{sub}</div>
        </div>
      </div>
      <div className="flex items-center gap-2">
        <select aria-label={t('phones.extension')} value={pick[key] ?? ''} onChange={(e) => setPick({ ...pick, [key]: e.target.value ? Number(e.target.value) : null })}
          className="min-h-10 w-full rounded-lg border border-line bg-field px-2.5 text-sm sm:w-56">
          <option value="">{t('phoneLink.chooseExtension')}</option>
          {exts.map((e) => <option key={e.id} value={e.id}>{extOption(e)}</option>)}
        </select>
        <Button busy={busy === key} onClick={onLink} disabled={!pick[key]} className="min-h-10 whitespace-nowrap px-3">
          <Link2 className="size-4" aria-hidden /> {t('phoneLink.link')}
        </Button>
        {extra}
      </div>
    </li>
  )
  return (
    <section className="card p-5" aria-labelledby="waiting-phones">
      <div className="mb-1 flex items-center gap-2">
        <h2 id="waiting-phones" className="text-base font-semibold text-ink">{t('phoneLink.waitingTitle')}</h2>
        <Badge tone="accent">{discovered.length + unassigned.length}</Badge>
      </div>
      <p className="mb-4 max-w-3xl text-sm text-muted">{t('phoneLink.waitingText')}</p>
      <ErrorMessage error={error} />
      <ul className="grid gap-2">
        {discovered.map((d) => row(d.mac,
          <span className="font-mono">{formatMac(d.mac)}</span>,
          <>{modelName(d.vendor, d.model) ?? (d.vendor ? `${vendorName(d.vendor)} · ${t('phoneLink.modelUnknownShort')}` : t('phoneLink.unknownPhone'))} · {d.ip} · {formatDate(d.lastSeen, i18n.language)}</>,
          <Badge tone="accent" dot>{t('phoneLink.new')}</Badge>,
          () => linkDiscovered(d),
          <button type="button" onClick={() => dismiss(d)} aria-label={t('phoneLink.dismiss')} title={t('phoneLink.dismiss')} className="inline-flex size-10 flex-none items-center justify-center rounded-lg text-muted hover:bg-danger/10 hover:text-danger"><X className="size-4" /></button>,
        ))}
        {unassigned.map((p) => row(`p${p.id}`,
          <>{p.label || <span className="font-mono">{formatMac(p.mac)}</span>}</>,
          <>{modelName(p.vendor, p.model) ?? p.model}{p.lastSeenAt ? ` · ${p.lastIp} · ${formatDate(p.lastSeenAt, i18n.language)}` : ` · ${t('phones.never')}`}</>,
          <Badge tone="warn">{t('phoneLink.notLinked')}</Badge>,
          () => linkKnown(p),
        ))}
      </ul>
      {adopt && <AdoptForm phone={adopt.phone} models={models} exts={exts} presetExt={adopt.ext} onClose={() => setAdopt(null)} onDone={(n) => { setAdopt(null); onChanged(n) }} />}
    </section>
  )
}

/** The phones of one extension, opened from the Extensions page. */
export function ExtensionPhones({ ext, onClose }: { ext: Extension; onClose: () => void }) {
  const { t, i18n } = useTranslation()
  const [phones, setPhones] = useState<Phone[] | null>(null)
  const [discovered, setDiscovered] = useState<DiscoveredPhone[]>([])
  const [models, setModels] = useState<PhoneModel[]>([])
  const [exts, setExts] = useState<Extension[]>([])
  const [pick, setPick] = useState('')
  const [adopt, setAdopt] = useState<DiscoveredPhone | null>(null)
  const [notice, setNotice] = useState<LinkNotice | null>(null)
  const [busy, setBusy] = useState('')
  const [error, setError] = useState<unknown>(null)
  async function load() {
    try {
      const [p, d, m, e] = await Promise.all([
        api<Phone[]>('GET', '/phones'), api<DiscoveredPhone[]>('GET', '/phones/discovered'),
        api<PhoneModel[]>('GET', '/provisioning/models'), api<Extension[]>('GET', '/pbx/extensions'),
      ])
      setPhones(p); setDiscovered(d); setModels(m); setExts(e)
    } catch (err) {
      setError(err)
    }
  }
  useEffect(() => { void load() }, [])
  const mine = (phones ?? []).filter((p) => p.extensionId === ext.id || p.lines?.includes(ext.id))
  const free = (phones ?? []).filter((p) => p.extensionId === null && !p.lines?.length)
  const modelName = (vendor: string, id: string) => models.find((m) => m.vendor === vendor && m.id === id)?.name

  async function assign(p: Phone, to: number | null) {
    setBusy(`p${p.id}`)
    setError(null)
    try {
      const r = await api<ResyncResult>('PUT', `/phones/${p.id}/assign`, { extensionId: to, resync: true })
      setNotice({ kind: 'resync', ...r })
      await load()
    } catch (e) {
      setError(e)
    } finally {
      setBusy('')
    }
  }
  async function linkPicked() {
    if (pick.startsWith('p')) {
      const p = free.find((x) => `p${x.id}` === pick)
      if (p) await assign(p, ext.id)
    } else {
      const d = discovered.find((x) => x.mac === pick)
      if (d) setAdopt(d)
    }
    setPick('')
  }
  return (
    <Modal wide title={t('phoneLink.extTitle')} subtitle={`${ext.number} · ${ext.name}`} onClose={onClose}>
      <ErrorMessage error={error} />
      {notice && <LinkNoticeAlert notice={notice} onClose={() => setNotice(null)} />}
      {phones && mine.length === 0 && <p className="text-sm text-muted">{t('phoneLink.extNone')}</p>}
      <ul className="grid gap-2">
        {mine.map((p) => (
          <li key={p.id} className="flex items-center gap-3 rounded-xl border border-line bg-inset p-3">
            <span className="inline-flex size-10 flex-none items-center justify-center rounded-xl bg-accent/10 text-accent"><Smartphone className="size-5" aria-hidden /></span>
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2 text-sm text-ink">
                {p.label || <span className="font-mono">{formatMac(p.mac)}</span>}
                {p.extensionId !== ext.id && <Badge>{t('phoneLink.extraLine')}</Badge>}
              </div>
              <div className="truncate text-xs text-subtle">{modelName(p.vendor, p.model) ?? p.model} · {p.lastSeenAt ? `${p.lastIp} · ${formatDate(p.lastSeenAt, i18n.language)}` : t('phones.never')}</div>
            </div>
            <ResyncButton phone={p} onDone={setNotice} />
            {p.extensionId === ext.id && (
              <Button variant="secondary" busy={busy === `p${p.id}`} onClick={() => { if (window.confirm(t('phoneLink.confirmUnlink', { number: ext.number }))) void assign(p, null) }} className="min-h-10 px-3">
                <Link2Off className="size-4" aria-hidden /> {t('phoneLink.unlink')}
              </Button>
            )}
          </li>
        ))}
      </ul>
      <div className="flex flex-col gap-2 border-t border-line pt-4 sm:flex-row sm:items-end">
        <div className="flex-1">
          <Select label={t('phoneLink.linkPhone')} value={pick} onChange={(e) => setPick(e.target.value)}
            hint={free.length + discovered.length === 0 ? t('phoneLink.extNoFree') : undefined}>
            <option value="">{t('phoneLink.choosePhone')}</option>
            {discovered.length > 0 && (
              <optgroup label={t('phoneLink.groupNew')}>
                {discovered.map((d) => <option key={d.mac} value={d.mac}>{formatMac(d.mac)} · {modelName(d.vendor, d.model) ?? (d.vendor ? vendorName(d.vendor) : t('phoneLink.unknownPhone'))}</option>)}
              </optgroup>
            )}
            {free.length > 0 && (
              <optgroup label={t('phoneLink.groupFree')}>
                {free.map((p) => <option key={p.id} value={`p${p.id}`}>{p.label ? `${p.label} · ` : ''}{formatMac(p.mac)} · {modelName(p.vendor, p.model) ?? p.model}</option>)}
              </optgroup>
            )}
          </Select>
        </div>
        <Button disabled={!pick} busy={busy !== '' && pick !== ''} onClick={linkPicked} className="sm:mb-[1px]"><Link2 className="size-4" aria-hidden /> {t('phoneLink.link')}</Button>
      </div>
      {adopt && <AdoptForm phone={adopt} models={models} exts={exts} presetExt={ext.id} onClose={() => setAdopt(null)} onDone={(n) => { setAdopt(null); setNotice(n); void load() }} />}
    </Modal>
  )
}
