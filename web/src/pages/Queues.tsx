import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { BookUser, ListOrdered, Pencil, Phone, Plus, Trash2 } from 'lucide-react'
import { Badge, Empty, Modal, Select, Table, Toggle, formatDuration, td, useApi } from '../components/kit'
import { CallerLanguageSelect } from '../components/CallerLanguage'
import { DestinationLabel, DestinationPicker, IconButton, nextNumber, resetDestinations, useSaver } from '../components/pbx'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { Contact, Extension, ExternalMember, FederationView, Prompt, Queue, QueueMember, QueueView, RemoteMember } from '../lib/types'

export const queueStrategies = ['rrmemory', 'ringall', 'leastrecent', 'fewestcalls', 'random', 'linear', 'wrandom'] as const

const blank: Queue = {
  id: 0, number: '', name: '', strategy: 'rrmemory', music: 'default', announcePosition: true, announceFrequency: 60, agentTimeout: 15,
  retry: 5, wrapup: 10, maxWait: 300, maxCallers: 0, joinEmpty: true, record: false, cidPrefix: '', greetingPrompt: 0,
  overflow: { type: 'hangup', target: '' }, enabled: true, members: [],
  options: { supervision: false, callback: { enabled: false, offerEvery: 60, retries: 2, retryDelay: 120, expire: 240, callerId: '' } },
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
        <Button onClick={async () => setEdit(blankQueue(await nextNumber('queues', (list.data ?? []).map((q) => q.number))))}>
          <Plus className="size-4" aria-hidden /> {t('queues.add')}
        </Button>
      </PageHeader>
      <ErrorMessage error={list.error} />
      <Card>
        <Table head={[t('common.number'), t('common.name'), t('queues.strategy'), t('queues.agents'), t('queues.waiting'), t('queues.overflow'), '']}
          empty={list.data?.length === 0 && <Empty icon={<ListOrdered className="size-7" />} title={t('queues.emptyTitle')} text={t('queues.emptyText')} />}>
          {list.data?.map((q) => {
            const ext = (q.options?.external ?? []).filter((m) => m.active).length
            const signedIn = q.members.filter((m) => !m.dynamic || m.loggedIn).length + ext
            return (
              <tr key={q.id} className={q.enabled ? '' : 'opacity-50'}>
                <td className={`${td} font-mono text-base text-ink`}>{q.number}</td>
                <td className={`${td} text-ink`}>{q.name}</td>
                <td className={td}><Badge tone="accent">{t(`queues.strategies.${q.strategy}`)}</Badge></td>
                <td className={td}><span className="font-mono">{signedIn}</span><span className="text-subtle"> / {q.members.length + ext}</span></td>
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
  const [q, setQ] = useState<Queue>(() => editableQueue(value))
  const { busy, error, save } = useSaver<Queue>('/pbx/queues', onSaved)
  return (
    <Modal wide title={q.id ? t('queues.edit', { name: value.name }) : t('queues.add')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={() => save(q)}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <QueueFields q={q} setQ={setQ} />
    </Modal>
  )
}

/** A queue without its live state, ready to edit and save. */
export function editableQueue(v: Queue | QueueView): Queue {
  const { live: _live, alerts: _alerts, ...rest } = v as QueueView
  return { ...rest, options: rest.options ?? blank.options }
}

/** A new, empty queue with sensible defaults. */
export const blankQueue = (number = ''): Queue => ({ ...blank, number, members: [], options: { ...blank.options, callback: { ...blank.options.callback } } })

const plain = (_title: string, children: ReactNode, _id: string) => children

/**
 * Every setting of a call queue. Used in the queue dialog and, in a narrow column (compact),
 * in the side panel of the IVR editor; group wraps each block of settings (for example in a
 * collapsible section).
 */
export function QueueFields({ q, setQ, compact, group = plain, prompts: promptList }: {
  q: Queue; setQ: (fn: (q: Queue) => Queue) => void; compact?: boolean; group?: (title: string, children: ReactNode, id: string) => ReactNode; prompts?: Prompt[]
}) {
  const { t } = useTranslation()
  const exts = useApi<Extension[]>('/pbx/extensions')
  const ownPrompts = useApi<Prompt[]>(promptList ? null : '/pbx/prompts')
  const prompts = promptList ?? ownPrompts.data ?? []
  const set = <K extends keyof Queue>(k: K, v: Queue[K]) => setQ((x) => ({ ...x, [k]: v }))
  const opts = q.options ?? blank.options
  const cb = opts.callback
  const setCb = (patch: Partial<Queue['options']['callback']>) => set('options', { ...opts, callback: { ...cb, ...patch } })
  const member = (id: number) => q.members.find((m) => m.extensionId === id)
  const setMember = (id: number, patch: Partial<QueueMember> | null) => {
    const others = q.members.filter((m) => m.extensionId !== id)
    if (patch === null) return set('members', others)
    const cur = member(id) ?? { extensionId: id, number: '', name: '', penalty: 0, dynamic: true, loggedIn: false, paused: false }
    set('members', [...others, { ...cur, ...patch }])
  }
  const grid3 = compact ? 'grid grid-cols-2 gap-3 [&>*]:min-w-0' : 'grid gap-4 sm:grid-cols-3'
  const grid2 = compact ? 'flex flex-col gap-3' : 'grid gap-4 sm:grid-cols-2'
  const chosen = q.members.length + (opts.external ?? []).length
  return (
    <>
      {group(t('queues.groups.general'), (
        <div key="general" data-group="general" className={compact ? 'flex flex-col gap-3' : 'flex flex-col gap-4'}>
          <div className={compact ? 'grid grid-cols-[96px_minmax(0,1fr)] gap-3 [&>*]:min-w-0' : 'grid gap-4 sm:grid-cols-[160px_1fr_1fr]'}>
            <Input name="number" label={t('common.number')} value={q.number} onChange={(e) => set('number', e.target.value.replace(/\D/g, ''))} inputMode="numeric" className="font-mono" />
            <Input name="name" label={t('common.name')} value={q.name} onChange={(e) => set('name', e.target.value)} autoFocus={!compact} maxLength={60} />
            {!compact && <StrategySelect q={q} set={set} />}
          </div>
          {compact && <StrategySelect q={q} set={set} />}
          {compact && <Toggle name="enabled" label={t('common.enabled')} checked={q.enabled} onChange={(v) => set('enabled', v)} />}
        </div>
      ), 'general')}

      {group(compact ? `${t('queues.agents')} · ${chosen}` : t('queues.agents'), (
        <div key="agents" data-group="agents" className="flex flex-col gap-2">
          {!compact && <span className="text-sm font-medium text-ink-2">{t('queues.agents')}</span>}
          <p className="text-xs text-subtle">{t('queues.agentsHint')}</p>
          <div className="grid gap-1.5">
            {exts.data?.length === 0 && <p className="text-sm text-muted">{t('queues.noExtensions')}</p>}
            {(exts.data ?? []).map((e) => {
              const m = member(e.id)
              return (
                <div key={e.id} className={`flex flex-wrap items-center gap-x-3 gap-y-1.5 rounded-lg border px-3 py-1.5 ${m ? 'border-accent/40 bg-accent/[0.06]' : 'border-line'}`}>
                  <label className="flex min-h-9 min-w-0 flex-1 cursor-pointer items-center gap-3 text-sm">
                    <input type="checkbox" checked={!!m} onChange={(x) => setMember(e.id, x.target.checked ? {} : null)} className="size-4 flex-none accent-[var(--color-accent)]" />
                    <span className="font-mono text-ink">{e.number}</span> <span className="truncate text-muted">{e.name}</span>
                  </label>
                  {m && (
                    <span className={`flex items-center gap-3 ${compact ? 'w-full pl-7' : ''}`}>
                      <select aria-label={t('queues.memberType')} value={m.dynamic ? 'dynamic' : 'static'} onChange={(x) => setMember(e.id, { dynamic: x.target.value === 'dynamic' })}
                        className="min-h-9 rounded-lg border border-line bg-field px-2 text-sm text-ink">
                        <option value="dynamic">{t('queues.dynamic')}</option>
                        <option value="static">{t('queues.static')}</option>
                      </select>
                      <label className="flex items-center gap-2 text-xs text-muted">
                        {t('queues.penalty')}
                        <input type="number" min={0} max={10} value={m.penalty} onChange={(x) => setMember(e.id, { penalty: Number(x.target.value) })}
                          className="min-h-9 w-16 rounded-lg border border-line bg-field px-2 font-mono text-sm text-ink" />
                      </label>
                    </span>
                  )}
                </div>
              )
            })}
          </div>
          <ExternalAgents value={opts.external ?? []} onChange={(v) => set('options', { ...opts, external: v })} compact={compact} />
          <RemoteAgents value={q.remoteMembers ?? []} onChange={(v) => set('remoteMembers', v)} />
        </div>
      ), 'agents')}

      {group(t('queues.groups.timing'), (
        <div key="timing" data-group="timing" className={grid3}>
          <Input name="agentTimeout" label={t('queues.agentTimeout')} type="number" min={5} max={120} value={q.agentTimeout} onChange={(e) => set('agentTimeout', Number(e.target.value))} hint={t('common.seconds')} />
          <Input name="retry" label={t('queues.retry')} type="number" min={0} max={60} value={q.retry} onChange={(e) => set('retry', Number(e.target.value))} hint={t('common.seconds')} />
          <Input name="wrapup" label={t('queues.wrapup')} type="number" min={0} max={600} value={q.wrapup} onChange={(e) => set('wrapup', Number(e.target.value))} hint={t('queues.wrapupHint')} />
          <Input name="maxWait" label={t('queues.maxWait')} type="number" min={0} max={7200} value={q.maxWait} onChange={(e) => set('maxWait', Number(e.target.value))} hint={t('queues.zeroUnlimited')} />
          <Input name="maxCallers" label={t('queues.maxCallers')} type="number" min={0} max={500} value={q.maxCallers} onChange={(e) => set('maxCallers', Number(e.target.value))} hint={t('queues.zeroUnlimited')} />
          <Input name="cidPrefix" label={t('ringGroups.cidPrefix')} value={q.cidPrefix} onChange={(e) => set('cidPrefix', e.target.value)} hint={t('ringGroups.cidPrefixHint')} />
        </div>
      ), 'timing')}

      {group(t('queues.groups.caller'), (
        <div key="caller" data-group="caller" className={compact ? 'flex flex-col gap-3' : 'flex flex-col gap-4'}>
          <div className={grid2}>
            <Select name="greetingPrompt" label={t('queues.greeting')} value={q.greetingPrompt} onChange={(e) => set('greetingPrompt', Number(e.target.value))} hint={t('queues.greetingHint')}>
              <option value={0}>{t('queues.noGreeting')}</option>
              {prompts.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            </Select>
            <Input name="music" label={t('queues.music')} value={q.music} onChange={(e) => set('music', e.target.value)} hint={t('queues.musicHint')} className="font-mono" />
          </div>
          <CallerLanguageSelect value={opts.language} onChange={(v) => set('options', { ...opts, language: v || undefined })} compact={compact} />
          <Toggle name="announcePosition" label={t('queues.announcePosition')} hint={t('queues.announcePositionHint')} checked={q.announcePosition} onChange={(v) => set('announcePosition', v)} />
          {q.announcePosition && <Input name="announceFrequency" label={t('queues.announceFrequency')} type="number" min={15} max={600} value={q.announceFrequency} onChange={(e) => set('announceFrequency', Number(e.target.value))} hint={t('common.seconds')} />}
          <Toggle name="joinEmpty" label={t('queues.joinEmpty')} hint={t('queues.joinEmptyHint')} checked={q.joinEmpty} onChange={(v) => set('joinEmpty', v)} />
          <Toggle name="record" label={t('queues.record')} hint={t('queues.recordHint')} checked={q.record} onChange={(v) => set('record', v)} />
        </div>
      ), 'caller')}

      {group(t('queues.groups.more'), (
        <div key="more" data-group="more" className={compact ? 'flex flex-col gap-3' : 'flex flex-col gap-4'}>
          <div className="flex flex-col gap-3 rounded-xl border border-line p-4">
            <Toggle name="callback" label={t('queues.callback')} hint={t('queues.callbackHint')} checked={cb.enabled} onChange={(v) => setCb({ enabled: v })} />
            {cb.enabled && (
              <div className={grid3}>
                <Input name="offerEvery" label={t('queues.callbackOfferEvery')} type="number" min={15} max={600} value={cb.offerEvery} onChange={(e) => setCb({ offerEvery: Number(e.target.value) })} hint={t('common.seconds')} />
                <Input name="retries" label={t('queues.callbackRetries')} type="number" min={0} max={5} value={cb.retries} onChange={(e) => setCb({ retries: Number(e.target.value) })} hint={t('queues.callbackRetriesHint')} />
                <Input name="retryDelay" label={t('queues.callbackRetryDelay')} type="number" min={30} max={3600} value={cb.retryDelay} onChange={(e) => setCb({ retryDelay: Number(e.target.value) })} hint={t('common.seconds')} />
                <Input name="expire" label={t('queues.callbackExpire')} type="number" min={10} max={1440} value={cb.expire} onChange={(e) => setCb({ expire: Number(e.target.value) })} hint={t('queues.callbackExpireHint')} />
                <Input name="callerId" label={t('queues.callbackCallerId')} value={cb.callerId} onChange={(e) => setCb({ callerId: e.target.value.replace(/[^0-9+]/g, '') })} hint={t('queues.callbackCallerIdHint')} className="font-mono" />
              </div>
            )}
          </div>
          <Toggle name="supervision" label={t('queues.supervision')} hint={t('queues.supervisionHint')} checked={opts.supervision} onChange={(v) => set('options', { ...opts, supervision: v })} />
          <DestinationPicker label={t('queues.overflow')} value={q.overflow} onChange={(d) => set('overflow', d)} exclude={q.id ? { type: 'queue', target: String(q.id) } : undefined} />
          {!compact && <Toggle name="enabled" label={t('common.enabled')} checked={q.enabled} onChange={(v) => set('enabled', v)} />}
        </div>
      ), 'more')}
    </>
  )
}

function StrategySelect({ q, set }: { q: Queue; set: <K extends keyof Queue>(k: K, v: Queue[K]) => void }) {
  const { t } = useTranslation()
  return (
    <Select name="strategy" label={t('queues.strategy')} value={q.strategy} onChange={(e) => set('strategy', e.target.value)} hint={t(`queues.strategyHints.${q.strategy}`)}>
      {queueStrategies.map((s) => <option key={s} value={s}>{t(`queues.strategies.${s}`)}</option>)}
    </Select>
  )
}

/** Agents on linked Kiestoon systems (federation): their phones ring for this queue too. */
function RemoteAgents({ value, onChange }: { value: RemoteMember[]; onChange: (v: RemoteMember[]) => void }) {
  const { t } = useTranslation()
  const fed = useApi<FederationView>('/federation')
  const peers = (fed.data?.peers ?? []).filter((p) => p.state === 'active' && (p.directory.entries ?? []).some((e) => e.kind === 'extension'))
  if (!peers.length && !value.length) return null
  const find = (peerId: number, number: string) => value.find((m) => m.peerId === peerId && m.number === number)
  const toggle = (peerId: number, number: string, name: string, site: string, on: boolean) =>
    onChange(on ? [...value, { peerId, number, name, site, penalty: 0 }] : value.filter((m) => !(m.peerId === peerId && m.number === number)))
  const setPenalty = (m: RemoteMember, penalty: number) => onChange(value.map((x) => (x === m ? { ...x, penalty } : x)))
  return (
    <div className="flex flex-col gap-2">
      <span className="text-sm font-medium text-ink-2">{t('queues.remoteAgents')}</span>
      <p className="text-xs text-subtle">{t('queues.remoteAgentsHint')}</p>
      {peers.map((p) => (
        <div key={p.id} className="flex flex-col gap-1.5">
          <span className="text-xs uppercase tracking-[0.08em] text-subtle">{p.name}</span>
          {(p.directory.entries ?? []).filter((e) => e.kind === 'extension').map((e) => {
            const m = find(p.id, e.number)
            return (
              <div key={e.number} className={`flex flex-wrap items-center gap-3 rounded-lg border px-3 py-1.5 ${m ? 'border-accent/40 bg-accent/[0.06]' : 'border-line'}`}>
                <label className="flex min-h-9 flex-1 cursor-pointer items-center gap-3 text-sm">
                  <input type="checkbox" checked={!!m} onChange={(x) => toggle(p.id, e.number, e.name, p.name, x.target.checked)} className="size-4 accent-[var(--color-accent)]" />
                  <span className="font-mono text-ink">{p.prefix}{e.number}</span> <span className="text-muted">{e.name}</span>
                </label>
                {m && (
                  <label className="flex items-center gap-2 text-xs text-muted">
                    {t('queues.penalty')}
                    <input type="number" min={0} max={10} value={m.penalty} onChange={(x) => setPenalty(m, Number(x.target.value))}
                      className="min-h-9 w-16 rounded-lg border border-line bg-field px-2 font-mono text-sm text-ink" />
                  </label>
                )}
              </div>
            )
          })}
        </div>
      ))}
    </div>
  )
}

/**
 * External numbers that take calls of this queue like an agent, for example a mobile or another
 * office. Typed in, or picked from the phone book. The queue rings them over the trunk.
 */
function ExternalAgents({ value, onChange, compact }: { value: ExternalMember[]; onChange: (v: ExternalMember[]) => void; compact?: boolean }) {
  const { t } = useTranslation()
  const [book, setBook] = useState(false)
  const contacts = useApi<Contact[]>(book ? '/phonebook' : null)
  const set = (i: number, patch: Partial<ExternalMember>) => onChange(value.map((m, j) => (j === i ? { ...m, ...patch } : m)))
  const add = (m: Partial<ExternalMember> = {}) => onChange([...value, { name: '', number: '', penalty: 0, active: true, ...m }])
  const numbers = (c: Contact) => [c.mobile, c.work, c.other].filter((n) => n && n.replace(/[^0-9+]/g, '').length >= 3)
  const full = value.length >= 20
  return (
    <div className="mt-2 flex flex-col gap-2">
      <span className="text-sm font-medium text-ink-2">{t('queues.external.title')}</span>
      <p className="text-xs text-subtle">{t('queues.external.hint')}</p>
      {value.map((m, i) => (
        <div key={i} className={`flex flex-wrap items-center gap-2 rounded-lg border px-3 py-2 ${m.active ? 'border-accent/40 bg-accent/[0.06]' : 'border-line opacity-70'}`}>
          <input type="checkbox" checked={m.active} onChange={(x) => set(i, { active: x.target.checked })} aria-label={t('queues.external.active')} title={t('queues.external.active')}
            className="size-4 flex-none accent-[var(--color-accent)]" />
          <Phone className="size-4 flex-none text-accent" aria-hidden />
          <input value={m.name} onChange={(x) => set(i, { name: x.target.value })} placeholder={t('queues.external.name')} aria-label={t('queues.external.name')} maxLength={40}
            className={`min-h-9 min-w-0 rounded-lg border border-line bg-field px-2 text-sm text-ink ${compact ? 'flex-1' : 'w-44'}`} />
          <input value={m.number} onChange={(x) => set(i, { number: x.target.value.replace(/[^0-9+ ]/g, '') })} placeholder="0612345678" aria-label={t('queues.external.number')} inputMode="tel"
            className={`min-h-9 min-w-0 rounded-lg border border-line bg-field px-2 font-mono text-sm text-ink ${compact ? 'w-full' : 'flex-1'}`} />
          <label className="flex items-center gap-2 text-xs text-muted">
            {t('queues.penalty')}
            <input type="number" min={0} max={10} value={m.penalty} onChange={(x) => set(i, { penalty: Number(x.target.value) })}
              className="min-h-9 w-16 rounded-lg border border-line bg-field px-2 font-mono text-sm text-ink" />
          </label>
          <IconButton label={t('common.delete')} danger onClick={() => onChange(value.filter((_, j) => j !== i))}><Trash2 className="size-4" aria-hidden /></IconButton>
        </div>
      ))}
      <div className="flex flex-wrap gap-2">
        <Button variant="secondary" disabled={full} onClick={() => add()}><Plus className="size-4" aria-hidden />{t('queues.external.add')}</Button>
        <Button variant="ghost" disabled={full} onClick={() => setBook(!book)} aria-expanded={book}><BookUser className="size-4" aria-hidden />{t('queues.external.fromBook')}</Button>
      </div>
      {book && (
        <div className="flex max-h-56 flex-col gap-1 overflow-y-auto rounded-lg border border-line p-2">
          {contacts.loading && <span className="text-xs text-muted">{t('app.loading')}</span>}
          {contacts.data?.length === 0 && <span className="text-xs text-muted">{t('queues.external.bookEmpty')}</span>}
          {(contacts.data ?? []).flatMap((c) => numbers(c).map((n) => {
            const num = n.replace(/[^0-9+]/g, '')
            const taken = value.some((m) => m.number.replace(/\s/g, '') === num)
            return (
              <button key={`${c.id}-${num}`} type="button" disabled={taken || full} onClick={() => { add({ name: c.name.slice(0, 40), number: num }); setBook(false) }}
                className="flex items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-tint/5 disabled:opacity-40">
                <span className="min-w-0 flex-1 truncate text-ink">{c.name}{c.company && <span className="text-subtle"> · {c.company}</span>}</span>
                <span className="font-mono text-xs text-muted">{num}</span>
              </button>
            )
          }))}
        </div>
      )}
    </div>
  )
}
