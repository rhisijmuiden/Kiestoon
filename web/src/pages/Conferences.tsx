import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Pencil, Plus, Presentation, Trash2 } from 'lucide-react'
import { Badge, Empty, Modal, Table, Toggle, td, useApi } from '../components/kit'
import { IconButton, nextNumber, resetDestinations, useSaver } from '../components/pbx'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { Conference } from '../lib/types'

const blank: Conference = { id: 0, number: '', name: '', pin: '', adminPin: '', maxMembers: 20, record: false, waitAdmin: false, enabled: true }

export function Conferences() {
  const { t } = useTranslation()
  const list = useApi<Conference[]>('/pbx/conferences')
  const [edit, setEdit] = useState<Conference | null>(null)
  async function remove(c: Conference) {
    if (!window.confirm(t('common.confirmDelete', { name: c.name }))) return
    await api('DELETE', `/pbx/conferences/${c.id}`)
    resetDestinations()
    list.reload()
  }
  return (
    <>
      <PageHeader title={t('conferences.title')} subtitle={t('conferences.subtitle')}>
        <Button onClick={async () => setEdit({ ...blank, number: await nextNumber('conferences', (list.data ?? []).map((c) => c.number)) })}>
          <Plus className="size-4" aria-hidden /> {t('conferences.add')}
        </Button>
      </PageHeader>
      <ErrorMessage error={list.error} />
      <Card>
        <Table head={[t('common.number'), t('common.name'), t('conferences.pin'), t('conferences.maxMembers'), '', '']}
          empty={list.data?.length === 0 && <Empty icon={<Presentation className="size-7" />} title={t('conferences.emptyTitle')} text={t('conferences.emptyText')} />}>
          {list.data?.map((c) => (
            <tr key={c.id} className={c.enabled ? '' : 'opacity-50'}>
              <td className={`${td} font-mono text-base text-ink`}>{c.number}</td>
              <td className={`${td} text-ink`}>{c.name}</td>
              <td className={td}>{c.pin ? <Badge tone="ok">{t('conferences.protected')}</Badge> : <Badge>{t('conferences.open')}</Badge>}</td>
              <td className={`${td} font-mono`}>{c.maxMembers}</td>
              <td className={td}>
                <div className="flex flex-wrap gap-1">
                  {c.record && <Badge tone="danger" dot>{t('conferences.recorded')}</Badge>}
                  {c.waitAdmin && <Badge tone="accent">{t('conferences.waitsForHost')}</Badge>}
                </div>
              </td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <IconButton label={t('common.edit')} onClick={() => setEdit(c)}><Pencil className="size-4" aria-hidden /></IconButton>
                <IconButton label={t('common.delete')} danger onClick={() => remove(c)}><Trash2 className="size-4" aria-hidden /></IconButton>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
      {edit && <ConferenceForm value={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); resetDestinations(); list.reload() }} />}
    </>
  )
}

function ConferenceForm({ value, onClose, onSaved }: { value: Conference; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [c, setC] = useState(value)
  const { busy, error, save } = useSaver<Conference>('/pbx/conferences', onSaved)
  const set = <K extends keyof Conference>(k: K, v: Conference[K]) => setC((x) => ({ ...x, [k]: v }))
  return (
    <Modal title={c.id ? t('conferences.edit', { name: value.name }) : t('conferences.add')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={() => save(c)}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <div className="grid gap-4 sm:grid-cols-[160px_1fr]">
        <Input label={t('common.number')} value={c.number} onChange={(e) => set('number', e.target.value.replace(/\D/g, ''))} inputMode="numeric" className="font-mono" />
        <Input label={t('common.name')} value={c.name} onChange={(e) => set('name', e.target.value)} autoFocus />
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        <Input label={t('conferences.pin')} value={c.pin} onChange={(e) => set('pin', e.target.value.replace(/\D/g, ''))} inputMode="numeric" className="font-mono" hint={t('conferences.pinHint')} />
        <Input label={t('conferences.adminPin')} value={c.adminPin} onChange={(e) => set('adminPin', e.target.value.replace(/\D/g, ''))} inputMode="numeric" className="font-mono" hint={t('conferences.adminPinHint')} />
        <Input label={t('conferences.maxMembers')} type="number" min={2} max={100} value={c.maxMembers} onChange={(e) => set('maxMembers', Number(e.target.value))} />
      </div>
      <Toggle label={t('conferences.waitAdmin')} hint={t('conferences.waitAdminHint')} checked={c.waitAdmin} onChange={(v) => set('waitAdmin', v)} />
      <Toggle label={t('conferences.record')} hint={t('conferences.recordHint')} checked={c.record} onChange={(v) => set('record', v)} />
      <Toggle label={t('common.enabled')} checked={c.enabled} onChange={(v) => set('enabled', v)} />
    </Modal>
  )
}
