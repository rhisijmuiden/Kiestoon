import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ListOrdered, Pencil, Plus, Trash2 } from 'lucide-react'
import { Badge, Empty, Modal, Select, Table, Toggle, formatDuration, td, useApi } from '../components/kit'
import { DestinationLabel, DestinationPicker, IconButton, nextNumber, resetDestinations, useSaver } from '../components/pbx'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { Extension, Prompt, Queue, QueueMember, QueueView } from '../lib/types'

export const queueStrategies = ['rrmemory', 'ringall', 'leastrecent', 'fewestcalls', 'random', 'linear', 'wrandom'] as const

const blank: Queue = {
  id: 0, number: '', name: '', strategy: 'rrmemory', music: 'default', announcePosition: true, announceFrequency: 60, agentTimeout: 15,
  retry: 5, wrapup: 10, maxWait: 300, maxCallers: 0, joinEmpty: true, record: false, cidPrefix: '', greetingPrompt: 0,
  overflow: { type: 'hangup', target: '' }, enabled: true, members: [],
}

export function Queues() {
  const { t } = useTranslation()
  const list = useApi<QueueView[]>('/reception/queues', 5000)
  const [edit, setEdit] = useState<Queue | null>(null)
  async function remove(q: Queue) {
    if (!window.confirm(t('common.confirmDelete', { name: q.name }))) return
    await api('DELETE', `/pbx/queues/${q.id}`)
    resetDestinations()
    list.reload()
  }
  return (
    <>
      <PageHeader title={t('queues.title')} subtitle={t('queues.subtitle')}>
        <Button onClick={async () => setEdit({ ...blank, number: await nextNumber('queues', (list.data ?? []).map((q) => q.number)) })}>
          <Plus className="size-4" aria-hidden /> {t('queues.add')}
        </Button>
      </PageHeader>
      <ErrorMessage error={list.error} />
      <Card>
        <Table head={[t('common.number'), t('common.name'), t('queues.strategy'), t('queues.agents'), t('queues.waiting'), t('queues.overflow'), '']}
          empty={list.data?.length === 0 && <Empty icon={<ListOrdered className="size-7" />} title={t('queues.emptyTitle')} text={t('queues.emptyText')} />}>
          {list.data?.map((q) => {
            const signedIn = q.members.filter((m) => !m.dynamic || m.loggedIn).length
            return (
              <tr key={q.id} className={q.enabled ? '' : 'opacity-50'}>
                <td className={`${td} font-mono text-base text-ink`}>{q.number}</td>
                <td className={`${td} text-ink`}>{q.name}</td>
                <td className={td}><Badge tone="accent">{t(`queues.strategies.${q.strategy}`)}</Badge></td>
                <td className={td}><span className="font-mono">{signedIn}</span><span className="text-subtle"> / {q.members.length}</span></td>
                <td className={td}>
                  {q.live ? (
                    <span className={`font-mono ${q.live.calls ? 'text-warn' : ''}`}>{q.live.calls}{q.live.calls > 0 && <span className="ml-1.5 text-xs text-subtle">{formatDuration(q.live.longestWait)}</span>}</span>
                  ) : '–'}
                </td>
                <td className={td}><DestinationLabel value={q.overflow} /></td>
                <td className={`${td} whitespace-nowrap text-right`}>
                  <IconButton label={t('common.edit')} onClick={() => setEdit(q)}><Pencil className="size-4" aria-hidden /></IconButton>
                  <IconButton label={t('common.delete')} danger onClick={() => remove(q)}><Trash2 className="size-4" aria-hidden /></IconButton>
                </td>
              </tr>
            )
          })}
        </Table>
      </Card>
      {edit && <QueueForm value={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); resetDestinations(); list.reload() }} />}
    </>
  )
}

function QueueForm({ value, onClose, onSaved }: { value: Queue; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [q, setQ] = useState<Queue>(() => {
    const { live: _live, ...rest } = value as QueueView
    return rest
  })
  const exts = useApi<Extension[]>('/pbx/extensions')
  const prompts = useApi<Prompt[]>('/pbx/prompts')
  const { busy, error, save } = useSaver<Queue>('/pbx/queues', onSaved)
  const set = <K extends keyof Queue>(k: K, v: Queue[K]) => setQ((x) => ({ ...x, [k]: v }))
  const member = (id: number) => q.members.find((m) => m.extensionId === id)
  const setMember = (id: number, patch: Partial<QueueMember> | null) => {
    const others = q.members.filter((m) => m.extensionId !== id)
    if (patch === null) return set('members', others)
    const cur = member(id) ?? { extensionId: id, number: '', name: '', penalty: 0, dynamic: true, loggedIn: false, paused: false }
    set('members', [...others, { ...cur, ...patch }])
  }
  return (
    <Modal wide title={q.id ? t('queues.edit', { name: value.name }) : t('queues.add')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={() => save(q)}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <div className="grid gap-4 sm:grid-cols-[160px_1fr_1fr]">
        <Input label={t('common.number')} value={q.number} onChange={(e) => set('number', e.target.value.replace(/\D/g, ''))} inputMode="numeric" className="font-mono" />
        <Input label={t('common.name')} value={q.name} onChange={(e) => set('name', e.target.value)} autoFocus />
        <Select label={t('queues.strategy')} value={q.strategy} onChange={(e) => set('strategy', e.target.value)} hint={t(`queues.strategyHints.${q.strategy}`)}>
          {queueStrategies.map((s) => <option key={s} value={s}>{t(`queues.strategies.${s}`)}</option>)}
        </Select>
      </div>

      <div className="flex flex-col gap-2">
        <span className="text-sm font-medium text-ink-2">{t('queues.agents')}</span>
        <p className="text-xs text-subtle">{t('queues.agentsHint')}</p>
        <div className="grid gap-1.5">
          {(exts.data ?? []).map((e) => {
            const m = member(e.id)
            return (
              <div key={e.id} className={`flex flex-wrap items-center gap-3 rounded-lg border px-3 py-1.5 ${m ? 'border-accent/40 bg-accent/[0.06]' : 'border-line'}`}>
                <label className="flex min-h-9 flex-1 cursor-pointer items-center gap-3 text-sm">
                  <input type="checkbox" checked={!!m} onChange={(x) => setMember(e.id, x.target.checked ? {} : null)} className="size-4 accent-[var(--color-accent)]" />
                  <span className="font-mono text-ink">{e.number}</span> <span className="text-muted">{e.name}</span>
                </label>
                {m && (
                  <>
                    <select aria-label={t('queues.memberType')} value={m.dynamic ? 'dynamic' : 'static'} onChange={(x) => setMember(e.id, { dynamic: x.target.value === 'dynamic' })}
                      className="min-h-9 rounded-lg border border-line bg-[#0A1224] px-2 text-sm text-ink">
                      <option value="dynamic">{t('queues.dynamic')}</option>
                      <option value="static">{t('queues.static')}</option>
                    </select>
                    <label className="flex items-center gap-2 text-xs text-muted">
                      {t('queues.penalty')}
                      <input type="number" min={0} max={10} value={m.penalty} onChange={(x) => setMember(e.id, { penalty: Number(x.target.value) })}
                        className="min-h-9 w-16 rounded-lg border border-line bg-[#0A1224] px-2 font-mono text-sm text-ink" />
                    </label>
                  </>
                )}
              </div>
            )
          })}
        </div>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        <Input label={t('queues.agentTimeout')} type="number" min={5} max={120} value={q.agentTimeout} onChange={(e) => set('agentTimeout', Number(e.target.value))} hint={t('common.seconds')} />
        <Input label={t('queues.retry')} type="number" min={0} max={60} value={q.retry} onChange={(e) => set('retry', Number(e.target.value))} hint={t('common.seconds')} />
        <Input label={t('queues.wrapup')} type="number" min={0} max={600} value={q.wrapup} onChange={(e) => set('wrapup', Number(e.target.value))} hint={t('queues.wrapupHint')} />
        <Input label={t('queues.maxWait')} type="number" min={0} max={7200} value={q.maxWait} onChange={(e) => set('maxWait', Number(e.target.value))} hint={t('queues.zeroUnlimited')} />
        <Input label={t('queues.maxCallers')} type="number" min={0} max={500} value={q.maxCallers} onChange={(e) => set('maxCallers', Number(e.target.value))} hint={t('queues.zeroUnlimited')} />
        <Input label={t('ringGroups.cidPrefix')} value={q.cidPrefix} onChange={(e) => set('cidPrefix', e.target.value)} hint={t('ringGroups.cidPrefixHint')} />
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <Select label={t('queues.greeting')} value={q.greetingPrompt} onChange={(e) => set('greetingPrompt', Number(e.target.value))} hint={t('queues.greetingHint')}>
          <option value={0}>{t('queues.noGreeting')}</option>
          {(prompts.data ?? []).map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
        </Select>
        <Input label={t('queues.music')} value={q.music} onChange={(e) => set('music', e.target.value)} hint={t('queues.musicHint')} className="font-mono" />
      </div>
      <Toggle label={t('queues.announcePosition')} hint={t('queues.announcePositionHint')} checked={q.announcePosition} onChange={(v) => set('announcePosition', v)} />
      {q.announcePosition && <Input label={t('queues.announceFrequency')} type="number" min={15} max={600} value={q.announceFrequency} onChange={(e) => set('announceFrequency', Number(e.target.value))} hint={t('common.seconds')} />}
      <Toggle label={t('queues.joinEmpty')} hint={t('queues.joinEmptyHint')} checked={q.joinEmpty} onChange={(v) => set('joinEmpty', v)} />
      <Toggle label={t('queues.record')} hint={t('queues.recordHint')} checked={q.record} onChange={(v) => set('record', v)} />
      <DestinationPicker label={t('queues.overflow')} value={q.overflow} onChange={(d) => set('overflow', d)} exclude={{ type: 'queue', target: String(q.id) }} />
      <Toggle label={t('common.enabled')} checked={q.enabled} onChange={(v) => set('enabled', v)} />
    </Modal>
  )
}
