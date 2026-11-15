import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Pencil, Plus, Trash2, UsersRound, X } from 'lucide-react'
import { Badge, Empty, Modal, Select, Table, Toggle, td, useApi } from '../components/kit'
import { DestinationLabel, DestinationPicker, IconButton, nextNumber, resetDestinations, useSaver } from '../components/pbx'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { Extension, RingGroup } from '../lib/types'

const blank: RingGroup = {
  id: 0, number: '', name: '', strategy: 'simultaneous', ringSeconds: 20, members: [], cidPrefix: '', skipBusy: false,
  failover: { type: 'hangup', target: '' }, enabled: true,
}

export function RingGroups() {
  const { t } = useTranslation()
  const list = useApi<RingGroup[]>('/pbx/ringgroups')
  const [edit, setEdit] = useState<RingGroup | null>(null)
  async function remove(g: RingGroup) {
    if (!window.confirm(t('common.confirmDelete', { name: g.name }))) return
    await api('DELETE', `/pbx/ringgroups/${g.id}`)
    resetDestinations()
    list.reload()
  }
  return (
    <>
      <PageHeader title={t('ringGroups.title')} subtitle={t('ringGroups.subtitle')}>
        <Button onClick={async () => setEdit({ ...blank, number: await nextNumber('ringGroups', (list.data ?? []).map((g) => g.number)) })}><Plus className="size-4" aria-hidden /> {t('ringGroups.add')}</Button>
      </PageHeader>
      <ErrorMessage error={list.error} />
      <Card>
        <Table head={[t('common.number'), t('common.name'), t('ringGroups.strategy'), t('ringGroups.members'), t('ringGroups.failover'), '']}
          empty={list.data?.length === 0 && <Empty icon={<UsersRound className="size-7" />} title={t('ringGroups.emptyTitle')} text={t('ringGroups.emptyText')} />}>
          {list.data?.map((g) => (
            <tr key={g.id} className={g.enabled ? '' : 'opacity-50'}>
              <td className={`${td} font-mono text-base text-ink`}>{g.number}</td>
              <td className={`${td} text-ink`}>{g.name}</td>
              <td className={td}><Badge tone="accent">{t(`ringGroups.strategies.${g.strategy}`)}</Badge></td>
              <td className={td}><span className="font-mono text-xs">{g.members.join(', ') || '–'}</span></td>
              <td className={td}><DestinationLabel value={g.failover} /></td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <IconButton label={t('common.edit')} onClick={() => setEdit(g)}><Pencil className="size-4" aria-hidden /></IconButton>
                <IconButton label={t('common.delete')} danger onClick={() => remove(g)}><Trash2 className="size-4" aria-hidden /></IconButton>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
      {edit && <RingGroupForm value={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); resetDestinations(); list.reload() }} />}
    </>
  )
}

function RingGroupForm({ value, onClose, onSaved }: { value: RingGroup; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [g, setG] = useState(value)
  const exts = useApi<Extension[]>('/pbx/extensions')
  const [external, setExternal] = useState('')
  const { busy, error, save } = useSaver<RingGroup>('/pbx/ringgroups', onSaved)
  const set = <K extends keyof RingGroup>(k: K, v: RingGroup[K]) => setG((x) => ({ ...x, [k]: v }))
  const toggle = (n: string) => set('members', g.members.includes(n) ? g.members.filter((m) => m !== n) : [...g.members, n])
  const move = (i: number, d: number) => {
    const m = [...g.members]
    const j = i + d
    if (j < 0 || j >= m.length) return
    ;[m[i], m[j]] = [m[j], m[i]]
    set('members', m)
  }
  const names = new Map((exts.data ?? []).map((e) => [e.number, e.name]))
  return (
    <Modal wide title={g.id ? t('ringGroups.edit', { name: value.name }) : t('ringGroups.add')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={() => save(g)}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <div className="grid gap-4 sm:grid-cols-[160px_1fr]">
        <Input label={t('common.number')} value={g.number} onChange={(e) => set('number', e.target.value.replace(/\D/g, ''))} inputMode="numeric" className="font-mono" />
        <Input label={t('common.name')} value={g.name} onChange={(e) => set('name', e.target.value)} autoFocus />
      </div>
      <div className="grid gap-4 sm:grid-cols-3">
        <Select label={t('ringGroups.strategy')} value={g.strategy} onChange={(e) => set('strategy', e.target.value as RingGroup['strategy'])}
          hint={t(`ringGroups.strategyHints.${g.strategy}`)}>
          {(['simultaneous', 'sequential', 'rotating', 'paging'] as const).map((s) => <option key={s} value={s}>{t(`ringGroups.strategies.${s}`)}</option>)}
        </Select>
        <Input label={t('ringGroups.ringSeconds')} type="number" min={5} max={300} value={g.ringSeconds} onChange={(e) => set('ringSeconds', Number(e.target.value))}
          hint={g.strategy === 'sequential' || g.strategy === 'rotating' ? t('ringGroups.perMember') : undefined} />
        <Input label={t('ringGroups.cidPrefix')} value={g.cidPrefix} onChange={(e) => set('cidPrefix', e.target.value)} placeholder={t('ringGroups.cidPrefixPlaceholder')} hint={t('ringGroups.cidPrefixHint')} />
      </div>
      <div className="flex flex-col gap-2">
        <span className="text-sm font-medium text-ink-2">{t('ringGroups.members')}</span>
        <div className="flex flex-wrap gap-2">
          {(exts.data ?? []).map((e) => (
            <button key={e.id} type="button" onClick={() => toggle(e.number)} aria-pressed={g.members.includes(e.number)}
              className={`min-h-9 rounded-lg border px-3 text-sm transition ${g.members.includes(e.number) ? 'border-accent/60 bg-accent/15 text-ink' : 'border-line text-muted hover:text-ink'}`}>
              <span className="font-mono">{e.number}</span> {e.name}
            </button>
          ))}
        </div>
        <div className="flex items-end gap-2">
          <div className="flex-1">
            <Input label={t('ringGroups.external')} value={external} onChange={(e) => setExternal(e.target.value.replace(/[^0-9+]/g, ''))} inputMode="tel" className="font-mono" placeholder="0612345678" hint={t('ringGroups.externalHint')} />
          </div>
          <Button variant="ghost" className="mb-6" disabled={external.length < 3} onClick={() => { if (!g.members.includes(external)) set('members', [...g.members, external]); setExternal('') }}>
            <Plus className="size-4" aria-hidden /> {t('common.add')}
          </Button>
        </div>
        {g.members.length > 0 && (
          <ol className="flex flex-col gap-1.5 rounded-xl border border-line p-2">
            {g.members.map((m, i) => (
              <li key={m} className="flex items-center gap-3 rounded-lg bg-tint/[0.03] px-3 py-1.5 text-sm">
                <span className="w-5 font-mono text-xs text-subtle">{i + 1}</span>
                <span className="font-mono text-ink">{m}</span>
                <span className="flex-1 truncate text-muted">{names.get(m) ?? t('ringGroups.externalNumber')}</span>
                <IconButton label={t('common.moveUp')} onClick={() => move(i, -1)} disabled={i === 0}>↑</IconButton>
                <IconButton label={t('common.moveDown')} onClick={() => move(i, 1)} disabled={i === g.members.length - 1}>↓</IconButton>
                <IconButton label={t('common.remove')} danger onClick={() => toggle(m)}><X className="size-4" aria-hidden /></IconButton>
              </li>
            ))}
          </ol>
        )}
      </div>
      <Toggle label={t('ringGroups.skipBusy')} hint={t('ringGroups.skipBusyHint')} checked={g.skipBusy} onChange={(v) => set('skipBusy', v)} />
      <DestinationPicker label={t('ringGroups.failover')} value={g.failover} onChange={(d) => set('failover', d)} exclude={{ type: 'ringgroup', target: String(g.id) }} />
      <Toggle label={t('common.enabled')} checked={g.enabled} onChange={(v) => set('enabled', v)} />
    </Modal>
  )
}
