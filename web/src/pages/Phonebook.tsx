import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BookUser, Download, Pencil, Phone, Plus, Search, Settings2, Trash2, Upload } from 'lucide-react'
import { Badge, CopyField, Empty, Modal, SectionTitle, Table, Textarea, Toggle, td, useApi } from '../components/kit'
import { IconButton, useSaver } from '../components/pbx'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api, isAdmin } from '../lib/api'
import { useSession } from '../lib/session'
import type { Contact, PhonebookInfo, PhonebookSettings } from '../lib/types'

const blank: Contact = { id: 0, name: '', company: '', work: '', mobile: '', other: '', email: '', notes: '' }

export function Phonebook() {
  const { t } = useTranslation()
  const { state } = useSession()
  const me = state.kind === 'ready' ? state.me : null
  const admin = isAdmin(me?.user)
  const list = useApi<Contact[]>('/phonebook')
  const [q, setQ] = useState('')
  const [edit, setEdit] = useState<Contact | null>(null)
  const [importing, setImporting] = useState(false)
  const [settings, setSettings] = useState(false)
  const [msg, setMsg] = useState('')
  const [error, setError] = useState<unknown>(null)
  const rows = useMemo(() => {
    const s = q.trim().toLowerCase()
    const digits = s.replace(/[^0-9]/g, '')
    return (list.data ?? []).filter((c) => !s || `${c.name} ${c.company} ${c.email}`.toLowerCase().includes(s) ||
      (digits.length > 2 && [c.work, c.mobile, c.other].some((n) => n.replace(/[^0-9]/g, '').includes(digits))))
  }, [list.data, q])

  async function remove(c: Contact) {
    if (!window.confirm(t('common.confirmDelete', { name: c.name }))) return
    await api('DELETE', `/phonebook/${c.id}`)
    list.reload()
  }
  async function call(number: string) {
    if (!me?.extension) return
    setError(null)
    try {
      await api('POST', '/calls', { extension: me.extension.number, number })
      setMsg(t('phonebook.calling', { number }))
    } catch (e) {
      setError(e)
    }
  }
  const num = (n: string) => n && (
    <span className="inline-flex items-center gap-1">
      <span className="font-mono">{n}</span>
      {me?.extension && <IconButton label={t('phonebook.call', { number: n })} onClick={() => call(n)}><Phone className="size-3.5" aria-hidden /></IconButton>}
    </span>
  )

  return (
    <>
      <PageHeader title={t('phonebook.title')} subtitle={t('phonebook.subtitle')}>
        {admin && <Button variant="ghost" onClick={() => setSettings(true)}><Settings2 className="size-4" aria-hidden />{t('phonebook.settings')}</Button>}
        {admin && <Button variant="ghost" onClick={() => setImporting(true)}><Upload className="size-4" aria-hidden />{t('phonebook.import')}</Button>}
        {admin && <a href="/api/phonebook/export" className="inline-flex min-h-11 items-center gap-2 rounded-[10px] border border-line px-4 text-sm font-medium text-ink-2 hover:bg-tint/5"><Download className="size-4" aria-hidden />{t('phonebook.export')}</a>}
        {me && me.user.role !== 'user' && <Button onClick={() => setEdit(blank)}><Plus className="size-4" aria-hidden />{t('phonebook.add')}</Button>}
      </PageHeader>
      {msg && <Alert tone="ok">{msg}</Alert>}
      <ErrorMessage error={error ?? list.error} />
      <Card>
        <div className="mb-4 max-w-md">
          <div className="relative">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-subtle" aria-hidden />
            <input aria-label={t('phonebook.search')} value={q} onChange={(e) => setQ(e.target.value)} placeholder={t('phonebook.search')}
              className="min-h-11 w-full rounded-[10px] border border-line bg-field pl-9 pr-3 text-[15px] text-ink placeholder:text-subtle outline-none focus:border-accent/60" />
          </div>
        </div>
        <Table head={[t('common.name'), t('phonebook.company'), t('phonebook.work'), t('phonebook.mobile'), t('phonebook.other'), '']}
          empty={rows.length === 0 && list.data && <Empty icon={<BookUser className="size-7" />} title={q ? t('phonebook.noMatch') : t('phonebook.emptyTitle')} text={q ? undefined : t('phonebook.emptyText')} />}>
          {rows.map((c) => (
            <tr key={c.id}>
              <td className={`${td} text-ink`}>{c.name}{c.email && <span className="block text-xs text-subtle">{c.email}</span>}</td>
              <td className={td}>{c.company || '–'}</td>
              <td className={td}>{num(c.work) || '–'}</td>
              <td className={td}>{num(c.mobile) || '–'}</td>
              <td className={td}>{num(c.other) || '–'}</td>
              <td className={`${td} whitespace-nowrap text-right`}>
                {me && me.user.role !== 'user' && <>
                  <IconButton label={t('common.edit')} onClick={() => setEdit(c)}><Pencil className="size-4" aria-hidden /></IconButton>
                  <IconButton label={t('common.delete')} danger onClick={() => remove(c)}><Trash2 className="size-4" aria-hidden /></IconButton>
                </>}
              </td>
            </tr>
          ))}
        </Table>
      </Card>
      {edit && <ContactForm value={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); list.reload() }} />}
      {importing && <ImportForm onClose={() => setImporting(false)} onDone={(text) => { setImporting(false); setMsg(text); list.reload() }} />}
      {settings && <SettingsForm onClose={() => setSettings(false)} />}
    </>
  )
}

function ContactForm({ value, onClose, onSaved }: { value: Contact; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [c, setC] = useState(value)
  const { busy, error, save } = useSaver<Contact>('/phonebook', onSaved)
  const set = <K extends keyof Contact>(k: K, v: Contact[K]) => setC((x) => ({ ...x, [k]: v }))
  const tel = (v: string) => v.replace(/[^0-9+ ()-]/g, '')
  return (
    <Modal title={c.id ? t('phonebook.edit', { name: value.name }) : t('phonebook.add')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={() => save(c)}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Input name="name" label={t('common.name')} value={c.name} onChange={(e) => set('name', e.target.value)} autoFocus />
        <Input name="company" label={t('phonebook.company')} value={c.company} onChange={(e) => set('company', e.target.value)} />
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        <Input name="work" label={t('phonebook.work')} value={c.work} onChange={(e) => set('work', tel(e.target.value))} inputMode="tel" className="font-mono" />
        <Input name="mobile" label={t('phonebook.mobile')} value={c.mobile} onChange={(e) => set('mobile', tel(e.target.value))} inputMode="tel" className="font-mono" />
        <Input name="other" label={t('phonebook.other')} value={c.other} onChange={(e) => set('other', tel(e.target.value))} inputMode="tel" className="font-mono" />
      </div>
      <Input name="email" label={t('phonebook.email')} type="email" value={c.email} onChange={(e) => set('email', e.target.value)} />
      <Input name="notes" label={t('phonebook.notes')} value={c.notes} onChange={(e) => set('notes', e.target.value)} />
    </Modal>
  )
}

function ImportForm({ onClose, onDone }: { onClose: () => void; onDone: (msg: string) => void }) {
  const { t } = useTranslation()
  const [csv, setCsv] = useState('')
  const [replace, setReplace] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  async function submit() {
    setBusy(true)
    setError(null)
    try {
      const r = await api<{ added: number; skipped: number[] | null }>('POST', '/phonebook/import', { csv, replace })
      const skipped = r.skipped ?? []
      onDone(skipped.length ? t('phonebook.importedSkipped', { added: r.added, rows: skipped.join(', ') }) : t('phonebook.imported', { added: r.added }))
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal wide title={t('phonebook.import')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} disabled={!csv.trim()} onClick={submit}>{t('phonebook.import')}</Button></>}>
      <ErrorMessage error={error} />
      <p className="text-sm text-muted">{t('phonebook.importHint')}</p>
      <input type="file" accept=".csv,text/csv" aria-label={t('phonebook.chooseFile')}
        onChange={async (e) => { const f = e.target.files?.[0]; if (f) setCsv(await f.text()) }}
        className="text-sm text-ink-2 file:mr-3 file:min-h-10 file:rounded-lg file:border file:border-accent/45 file:bg-accent/10 file:px-3 file:text-accent-ink" />
      <Textarea label="CSV" value={csv} onChange={(e) => setCsv(e.target.value)} rows={8} placeholder={'name;company;work;mobile\nJan Jansen;Bakkerij;020 123 4567;06 12345678'} />
      <Toggle label={t('phonebook.replace')} hint={t('phonebook.replaceHint')} checked={replace} onChange={setReplace} />
    </Modal>
  )
}

function SettingsForm({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const info = useApi<PhonebookInfo>('/phonebook/settings')
  const [s, setS] = useState<PhonebookSettings | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  useEffect(() => { if (info.data && !s) setS(info.data.settings) }, [info.data, s])
  const set = <K extends keyof PhonebookSettings>(k: K, v: PhonebookSettings[K]) => s && setS({ ...s, [k]: v })
  async function save() {
    setBusy(true)
    setError(null)
    try {
      info.setData(await api<PhonebookInfo>('PUT', '/phonebook/settings', s))
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const d = info.data
  return (
    <Modal wide title={t('phonebook.settings')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.close')}</Button><Button busy={busy} disabled={!s} onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error ?? info.error} />
      {s && d && <>
        <Toggle name="includeExtensions" label={t('phonebook.includeExtensions')} hint={t('phonebook.includeExtensionsHint')} checked={s.includeExtensions} onChange={(v) => set('includeExtensions', v)} />
        <Toggle name="callerIdLookup" label={t('phonebook.callerIdLookup')} hint={t('phonebook.callerIdLookupHint')} checked={s.callerIdLookup} onChange={(v) => set('callerIdLookup', v)} />
        <Toggle name="xmlEnabled" label={t('phonebook.xml')} hint={t('phonebook.xmlHint')} checked={s.xmlEnabled} onChange={(v) => set('xmlEnabled', v)} />
        <Toggle name="ldapEnabled" label={t('phonebook.ldap')} hint={t('phonebook.ldapHint')} checked={s.ldapEnabled} onChange={(v) => set('ldapEnabled', v)} />
        {s.ldapEnabled && <>
          <div className="grid gap-4 sm:grid-cols-2">
            <Input name="ldapPort" label={t('phonebook.ldapPort')} type="number" value={s.ldapPort} onChange={(e) => set('ldapPort', Number(e.target.value))} />
            <Input name="ldapsPort" label={t('phonebook.ldapsPort')} type="number" value={s.ldapsPort} onChange={(e) => set('ldapsPort', Number(e.target.value))} hint={t('phonebook.ldapsPortHint')} />
          </div>
          <Toggle name="ldapAuth" label={t('phonebook.ldapAuth')} hint={t('phonebook.ldapAuthHint')} checked={s.ldapAuth} onChange={(v) => set('ldapAuth', v)} />
          <SectionTitle action={d.ldapRunning ? <Badge tone="ok" dot>{t('phonebook.ldapRunning')}</Badge> : <Badge dot>{t('phonebook.ldapStopped')}</Badge>}>{t('phonebook.phoneSettings')}</SectionTitle>
          <div className="grid gap-3 sm:grid-cols-2">
            <CopyField label={t('phonebook.server')} value={`${d.server}:${s.ldapPort}`} />
            <CopyField label={t('phonebook.baseDn')} value={d.baseDn} />
            {s.ldapAuth && <CopyField label={t('phonebook.bindDn')} value={d.bindDn} />}
            <CopyField label={t('phonebook.filter')} value="(|(cn=%)(telephoneNumber=%)(mobile=%))" />
          </div>
          {s.ldapAuth && <p className="text-xs text-subtle">{t('phonebook.bindPasswordHint')}</p>}
        </>}
        {s.xmlEnabled && <p className="text-xs text-subtle">{t('phonebook.xmlAuto')}</p>}
      </>}
    </Modal>
  )
}
