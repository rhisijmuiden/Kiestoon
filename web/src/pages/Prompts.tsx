import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { AudioLines, Pencil, Plus, Trash2 } from 'lucide-react'
import { Empty, Modal, Table, formatDate, td, useApi } from '../components/kit'
import { AudioInput, IconButton } from '../components/pbx'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { Wav } from '../lib/audio'
import type { Prompt } from '../lib/types'

export function Prompts() {
  const { t, i18n } = useTranslation()
  const list = useApi<Prompt[]>('/pbx/prompts')
  const [edit, setEdit] = useState<Prompt | null>(null)
  const [error, setError] = useState<unknown>(null)
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
      <Card>
        <Table head={[t('common.name'), t('prompts.listen'), t('prompts.length'), t('prompts.added'), '']}
          empty={list.data?.length === 0 && <Empty icon={<AudioLines className="size-7" />} title={t('prompts.emptyTitle')} text={t('prompts.emptyText')} />}>
          {list.data?.map((p) => (
            <tr key={p.id}>
              <td className={`${td} text-ink`}>{p.name}</td>
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
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api(value.id ? 'PUT' : 'POST', value.id ? `/pbx/prompts/${value.id}` : '/pbx/prompts', { name, audio: wav?.base64 ?? '' })
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
      <Input label={t('common.name')} value={name} onChange={(e) => setName(e.target.value)} autoFocus placeholder={t('prompts.namePlaceholder')} />
      {value.id > 0 && <p className="text-sm text-muted">{t('prompts.replaceHint')}</p>}
      <AudioInput value={wav} onChange={setWav} />
    </Modal>
  )
}
