import { Suspense, lazy, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { GitBranch, Loader2, Pencil, Plus, Trash2 } from 'lucide-react'
import { Empty, Table, td, useApi } from '../components/kit'
import { DestinationLabel, IconButton, resetDestinations } from '../components/pbx'
import { Button, Card, ErrorMessage, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { IVR, Prompt } from '../lib/types'
import { blankIVR } from './ivr/flow'

// The canvas (React Flow) loads only when a menu is opened.
const FlowEditor = lazy(() => import('./ivr/FlowEditor').then((m) => ({ default: m.FlowEditor })))

export function IVRs() {
  const { t } = useTranslation()
  const list = useApi<IVR[]>('/pbx/ivrs')
  const prompts = useApi<Prompt[]>('/pbx/prompts')
  const [edit, setEdit] = useState<IVR | null>(null)
  async function remove(v: IVR) {
    if (!window.confirm(t('common.confirmDelete', { name: v.name }))) return
    await api('DELETE', `/pbx/ivrs/${v.id}`)
    resetDestinations()
    list.reload()
  }
  if (edit) {
    return (
      <Suspense fallback={<div className="flex justify-center py-20"><Loader2 className="size-6 animate-spin text-accent" aria-label={t('common.loading')} /></div>}>
        <FlowEditor value={edit} onClose={() => { setEdit(null); list.reload() }} onSaved={(v) => { setEdit(v); resetDestinations(); list.reload() }} />
      </Suspense>
    )
  }
  const promptName = (id: number) => prompts.data?.find((p) => p.id === id)?.name
  return (
    <>
      <PageHeader title={t('ivr.title')} subtitle={t('ivr.subtitle')}>
        <Button onClick={() => setEdit(blankIVR())}><Plus className="size-4" aria-hidden /> {t('ivr.add')}</Button>
      </PageHeader>
      <ErrorMessage error={list.error} />
      <Card>
        <Table head={[t('common.name'), t('ivr.prompt'), t('ivr.options'), t('ivr.noInput'), '']}
          empty={list.data?.length === 0 && <Empty icon={<GitBranch className="size-7" />} title={t('ivr.emptyTitle')} text={t('ivr.emptyText')} action={<Button onClick={() => setEdit(blankIVR())}>{t('ivr.add')}</Button>} />}>
          {list.data?.map((v) => (
            <tr key={v.id}>
              <td className={`${td} text-ink`}>{v.name}</td>
              <td className={td}>{promptName(v.promptId) ?? <span className="text-subtle">{t('ivr.noPrompt')}</span>}</td>
              <td className={td}>
                <div className="flex flex-wrap gap-1">{v.options.map((o) => <span key={o.digit} className="inline-flex size-7 items-center justify-center rounded-md border border-accent/40 bg-accent/10 font-mono text-xs text-ink">{o.digit}</span>)}</div>
              </td>
              <td className={td}><DestinationLabel value={v.timeoutDestination} /></td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <IconButton label={t('common.edit')} onClick={() => setEdit(v)}><Pencil className="size-4" aria-hidden /></IconButton>
                <IconButton label={t('common.delete')} danger onClick={() => remove(v)}><Trash2 className="size-4" aria-hidden /></IconButton>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
    </>
  )
}
