import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Download, Inbox, Pencil, Plus, Printer, Trash2 } from 'lucide-react'
import { Badge, Empty, Modal, Table, Tabs, Toggle, formatBytes, formatDate, td, useApi } from '../components/kit'
import { IconButton, resetDestinations, useSaver } from '../components/pbx'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api, isAdmin } from '../lib/api'
import { useSession } from '../lib/session'
import type { Fax as FaxRecord, FaxBox } from '../lib/types'

const blank: FaxBox = { id: 0, name: '', emails: '', keepDays: 90, ecm: true, enabled: true }

export function Fax() {
  const { t } = useTranslation()
  const { state } = useSession()
  const admin = state.kind === 'ready' && isAdmin(state.me.user)
  const [tab, setTab] = useState<'received' | 'boxes'>('received')
  return (
    <>
      <PageHeader title={t('fax.title')} subtitle={t('fax.subtitle')} />
      {admin && <Tabs value={tab} onChange={setTab} tabs={[{ key: 'received', label: t('fax.received') }, { key: 'boxes', label: t('fax.boxes') }]} />}
      {tab === 'received' || !admin ? <Received admin={admin} /> : <Boxes />}
    </>
  )
}

function Received({ admin }: { admin: boolean }) {
  const { t, i18n } = useTranslation()
  const list = useApi<FaxRecord[]>('/faxes', 15000)
  async function remove(f: FaxRecord) {
    if (!window.confirm(t('fax.confirmDelete'))) return
    await api('DELETE', `/faxes/${f.id}`)
    list.reload()
  }
  return (
    <Card>
      <ErrorMessage error={list.error} />
      <Table head={[t('fax.receivedAt'), t('fax.from'), t('fax.to'), t('fax.box'), t('fax.pages'), t('fax.status'), '']}
        empty={list.data?.length === 0 && <Empty icon={<Inbox className="size-7" />} title={t('fax.emptyTitle')} text={t('fax.emptyText')} />}>
        {list.data?.map((f) => (
          <tr key={f.id}>
            <td className={td}>{formatDate(f.receivedAt, i18n.language)}</td>
            <td className={`${td} text-ink`}>
              <span className="font-mono">{f.caller || t('fax.unknown')}</span>
              {f.remoteStation && <span className="block text-xs text-subtle">{f.remoteStation}</span>}
            </td>
            <td className={`${td} font-mono`}>{f.did || '–'}</td>
            <td className={td}>{f.boxName || '–'}</td>
            <td className={`${td} font-mono`}>{f.pages}</td>
            <td className={td}>
              {f.status === 'success'
                ? <Badge tone="ok" dot>{f.mailed ? t('fax.mailed') : t('fax.ok')}</Badge>
                : <Badge tone="danger" dot>{t('fax.failed')}</Badge>}
              {f.error && <span className="block text-xs text-subtle">{f.error}</span>}
            </td>
            <td className={`${td} whitespace-nowrap text-right`}>
              {f.size > 0 && (
                <a href={`/api/faxes/${f.id}/pdf`} title={`${t('fax.download')} (${formatBytes(f.size)})`} aria-label={t('fax.download')}
                  className="inline-flex size-10 items-center justify-center rounded-lg text-muted hover:bg-white/5 hover:text-accent"><Download className="size-4" aria-hidden /></a>
              )}
              {admin && <IconButton label={t('common.delete')} danger onClick={() => remove(f)}><Trash2 className="size-4" aria-hidden /></IconButton>}
            </td>
          </tr>
        ))}
      </Table>
    </Card>
  )
}

function Boxes() {
  const { t } = useTranslation()
  const list = useApi<FaxBox[]>('/pbx/faxboxes')
  const [edit, setEdit] = useState<FaxBox | null>(null)
  async function remove(b: FaxBox) {
    if (!window.confirm(t('common.confirmDelete', { name: b.name }))) return
    await api('DELETE', `/pbx/faxboxes/${b.id}`)
    resetDestinations()
    list.reload()
  }
  return (
    <>
      <Card>
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <p className="max-w-2xl text-sm text-muted">{t('fax.boxesIntro')}</p>
          <Button onClick={() => setEdit(blank)}><Plus className="size-4" aria-hidden /> {t('fax.addBox')}</Button>
        </div>
        <ErrorMessage error={list.error} />
        <Table head={[t('common.name'), t('fax.emails'), t('fax.keepDays'), '', '']}
          empty={list.data?.length === 0 && <Empty icon={<Printer className="size-7" />} title={t('fax.noBoxesTitle')} text={t('fax.noBoxesText')} />}>
          {list.data?.map((b) => (
            <tr key={b.id} className={b.enabled ? '' : 'opacity-50'}>
              <td className={`${td} text-ink`}>{b.name}</td>
              <td className={`${td} text-xs`}>{b.emails}</td>
              <td className={`${td} font-mono`}>{b.keepDays ? t('fax.days', { count: b.keepDays }) : t('fax.forever')}</td>
              <td className={td}>{b.ecm && <Badge tone="accent">ECM</Badge>}</td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <IconButton label={t('common.edit')} onClick={() => setEdit(b)}><Pencil className="size-4" aria-hidden /></IconButton>
                <IconButton label={t('common.delete')} danger onClick={() => remove(b)}><Trash2 className="size-4" aria-hidden /></IconButton>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
      {edit && <BoxForm value={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); resetDestinations(); list.reload() }} />}
    </>
  )
}

function BoxForm({ value, onClose, onSaved }: { value: FaxBox; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [b, setB] = useState(value)
  const { busy, error, save } = useSaver<FaxBox>('/pbx/faxboxes', onSaved)
  const set = <K extends keyof FaxBox>(k: K, v: FaxBox[K]) => setB((x) => ({ ...x, [k]: v }))
  return (
    <Modal title={b.id ? t('fax.editBox', { name: value.name }) : t('fax.addBox')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={() => save(b)}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <Input label={t('common.name')} value={b.name} onChange={(e) => set('name', e.target.value)} placeholder={t('fax.namePlaceholder')} autoFocus />
      <Input label={t('fax.emails')} value={b.emails} onChange={(e) => set('emails', e.target.value)} placeholder="fax@example.com, office@example.com" hint={t('fax.emailsHint')} />
      <Input label={t('fax.keepDays')} type="number" min={0} max={3650} value={b.keepDays} onChange={(e) => set('keepDays', Number(e.target.value))} hint={t('fax.keepDaysHint')} />
      <Toggle label={t('fax.ecm')} hint={t('fax.ecmHint')} checked={b.ecm} onChange={(v) => set('ecm', v)} />
      <Toggle label={t('common.enabled')} checked={b.enabled} onChange={(v) => set('enabled', v)} />
      <p className="text-xs text-subtle">{t('fax.routeHint')}</p>
    </Modal>
  )
}
