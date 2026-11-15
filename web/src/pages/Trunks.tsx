import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowDown, ArrowUp, BookOpen, ExternalLink, Pencil, Plus, Trash2, Waypoints, X } from 'lucide-react'
import { Badge, Empty, Modal, Select, Table, Tabs, Textarea, Toggle, td, useApi } from '../components/kit'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { DestType, DialPattern, Extension, InboundRoute, Live, OutboundRoute, Trunk, TrunkPreset } from '../lib/types'
import { DestinationLabel, DestinationPicker } from '../components/pbx'

type Tab = 'trunks' | 'inbound' | 'outbound'

export function Trunks() {
  const { t } = useTranslation()
  const [tab, setTab] = useState<Tab>('trunks')
  return (
    <>
      <PageHeader title={t('trunks.title')} subtitle={t('trunks.subtitle')} />
      <Tabs<Tab> value={tab} onChange={setTab} tabs={[
        { key: 'trunks', label: t('trunks.tabTrunks') },
        { key: 'inbound', label: t('trunks.tabInbound') },
        { key: 'outbound', label: t('trunks.tabOutbound') },
      ]} />
      {tab === 'trunks' && <TrunkList />}
      {tab === 'inbound' && <InboundList />}
      {tab === 'outbound' && <OutboundList />}
    </>
  )
}

function IconBtn({ label, onClick, danger, children }: { label: string; onClick: () => void; danger?: boolean; children: React.ReactNode }) {
  return (
    <button type="button" onClick={onClick} title={label} aria-label={label}
      className={`inline-flex size-10 items-center justify-center rounded-lg text-muted ${danger ? 'hover:bg-danger/10 hover:text-danger' : 'hover:bg-tint/5 hover:text-ink'}`}>
      {children}
    </button>
  )
}

function statusTone(s: string): 'ok' | 'warn' | 'danger' | 'muted' {
  if (s === 'registered' || s === 'reachable') return 'ok'
  if (s === 'rejected' || s === 'unreachable' || s === 'unregistered') return 'danger'
  return 'muted'
}

// ---- trunks -------------------------------------------------------------------

const blankTrunk: Trunk = {
  id: 0, name: '', provider: 'generic', authMode: 'register', host: '', port: 5060, transport: 'udp', username: '', authUsername: '',
  secret: '', secretSet: false, fromDomain: '', fromUser: '', matchIps: '', didSource: 'to', codecs: 'alaw,ulaw,g722', maxChannels: 0,
  callerId: '', srtp: false, t38: false, enabled: true,
}

function TrunkList() {
  const { t } = useTranslation()
  const list = useApi<Trunk[]>('/pbx/trunks')
  const live = useApi<Live>('/live', 5000)
  const [edit, setEdit] = useState<Trunk | null>(null)
  const state = (id: number) => live.data?.trunks.find((x) => x.id === id)?.status ?? 'unknown'
  async function remove(x: Trunk) {
    if (!window.confirm(t('trunks.confirmDelete', { name: x.name }))) return
    await api('DELETE', `/pbx/trunks/${x.id}`)
    list.reload()
  }
  return (
    <Card>
      <div className="mb-4 flex justify-end">
        <Button onClick={() => setEdit({ ...blankTrunk })}><Plus className="size-4" aria-hidden /> {t('trunks.add')}</Button>
      </div>
      <ErrorMessage error={list.error} />
      <Table head={[t('trunks.name'), t('trunks.host'), t('trunks.authMode'), t('trunks.status'), t('trunks.callerId'), '']}
        empty={list.data?.length === 0 && <Empty icon={<Waypoints className="size-7" />} title={t('trunks.emptyTitle')} text={t('trunks.emptyText')} />}>
        {list.data?.map((x) => (
          <tr key={x.id} className={x.enabled ? '' : 'opacity-50'}>
            <td className={`${td} text-ink`}>{x.name}</td>
            <td className={`${td} font-mono text-xs`}>{x.host}{x.port ? `:${x.port}` : ''} {!x.port && <span className="text-subtle">SRV</span>} <span className="uppercase text-subtle">{x.transport}</span></td>
            <td className={td}>{t(`trunks.mode.${x.authMode}`)}</td>
            <td className={td}><Badge tone={statusTone(state(x.id))} dot>{t(`trunks.state.${state(x.id)}`, { defaultValue: state(x.id) })}</Badge></td>
            <td className={`${td} font-mono`}>{x.callerId || '–'}</td>
            <td className={`${td} whitespace-nowrap text-right`}>
              <IconBtn label={t('common.edit')} onClick={() => setEdit({ ...x, secret: '' })}><Pencil className="size-4" /></IconBtn>
              <IconBtn label={t('common.delete')} danger onClick={() => remove(x)}><Trash2 className="size-4" /></IconBtn>
            </td>
          </tr>
        ))}
      </Table>
      {edit && <TrunkForm value={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); list.reload() }} />}
    </Card>
  )
}

function TrunkForm({ value, onClose, onSaved }: { value: Trunk; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const presets = useApi<TrunkPreset[]>('/pbx/trunks/presets')
  const [x, setX] = useState<Trunk>(value)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const set = <K extends keyof Trunk>(k: K, v: Trunk[K]) => setX((o) => ({ ...o, [k]: v }))
  function applyPreset(id: string) {
    const p = presets.data?.find((q) => q.id === id)
    if (!p) return
    setX((o) => ({ ...o, provider: p.id, authMode: p.authMode, host: p.host || o.host, port: p.port, transport: p.transport, didSource: p.didSource, codecs: p.codecs, matchIps: p.matchIps || (p.id === o.provider ? o.matchIps : ''), name: o.name || p.name }))
  }
  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api(x.id ? 'PUT' : 'POST', x.id ? `/pbx/trunks/${x.id}` : '/pbx/trunks', x)
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const preset = presets.data?.find((p) => p.id === x.provider)
  return (
    <Modal wide title={x.id ? t('trunks.edit') : t('trunks.add')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <Select label={t('trunks.provider')} value={x.provider} onChange={(e) => applyPreset(e.target.value)}>
        {presets.data?.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
      </Select>
      {preset && <Alert tone="warn">{t(`trunks.notes.${preset.note}`)}</Alert>}
      {preset?.guide && <ProviderGuide guide={preset.guide} docsUrl={preset.docsUrl} open={!x.id} />}
      <div className="grid gap-4 sm:grid-cols-2">
        <Input label={t('trunks.name')} value={x.name} onChange={(e) => set('name', e.target.value)} />
        <Select label={t('trunks.authMode')} value={x.authMode} onChange={(e) => set('authMode', e.target.value as Trunk['authMode'])}>
          <option value="register">{t('trunks.mode.register')}</option>
          <option value="ip">{t('trunks.mode.ip')}</option>
        </Select>
      </div>
      <div className="grid gap-4 sm:grid-cols-[1fr_110px_130px]">
        <Input label={t('trunks.host')} value={x.host} onChange={(e) => set('host', e.target.value.trim())} className="font-mono" placeholder="sip.provider.nl" />
        <Input label={t('trunks.port')} type="number" min={0} value={x.port} hint={x.port === 0 ? t('trunks.portSrv') : undefined} onChange={(e) => set('port', Number(e.target.value))} />
        <Select label={t('trunks.transport')} value={x.transport} onChange={(e) => { const v = e.target.value as Trunk['transport']; setX((o) => ({ ...o, transport: v, srtp: v === 'tls' ? o.srtp : false, port: v === 'tls' && o.port === 5060 ? 5061 : v !== 'tls' && o.port === 5061 ? 5060 : o.port })) }}>
          <option value="udp">UDP</option><option value="tcp">TCP</option><option value="tls">TLS</option>
        </Select>
      </div>
      {x.authMode === 'register' && (
        <div className="grid gap-4 sm:grid-cols-3">
          <Input label={t('trunks.username')} value={x.username} onChange={(e) => set('username', e.target.value.trim())} className="font-mono" />
          <Input label={t('trunks.authUsername')} value={x.authUsername} onChange={(e) => set('authUsername', e.target.value.trim())} hint={t('trunks.authUsernameHint')} className="font-mono" />
          <Input label={t('trunks.secret')} type="password" value={x.secret ?? ''} onChange={(e) => set('secret', e.target.value)} placeholder={x.secretSet ? '••••••••' : ''} hint={x.secretSet ? t('common.keepSecret') : undefined} autoComplete="new-password" />
        </div>
      )}
      <Textarea label={t('trunks.matchIps')} value={x.matchIps.split(',').filter(Boolean).join('\n')} onChange={(e) => set('matchIps', e.target.value.split(/\s+/).filter(Boolean).join(','))} hint={t('trunks.matchIpsHint')} />
      <div className="grid gap-4 sm:grid-cols-3">
        <Input label={t('trunks.callerId')} value={x.callerId} onChange={(e) => set('callerId', e.target.value.trim())} hint={t('trunks.callerIdHint')} className="font-mono" />
        <Select label={t('trunks.didSource')} value={x.didSource} onChange={(e) => set('didSource', e.target.value as Trunk['didSource'])}>
          <option value="to">{t('trunks.did.to')}</option>
          <option value="ruri">{t('trunks.did.ruri')}</option>
        </Select>
        <Input label={t('trunks.maxChannels')} type="number" min={0} value={x.maxChannels} onChange={(e) => set('maxChannels', Number(e.target.value))} hint={t('trunks.maxChannelsHint')} />
      </div>
      <details className="rounded-xl border border-line p-4">
        <summary className="cursor-pointer text-sm text-ink-2">{t('common.advanced')}</summary>
        <div className="mt-4 grid gap-4 sm:grid-cols-3">
          <Input label={t('trunks.fromUser')} value={x.fromUser} onChange={(e) => set('fromUser', e.target.value.trim())} className="font-mono" />
          <Input label={t('trunks.fromDomain')} value={x.fromDomain} onChange={(e) => set('fromDomain', e.target.value.trim())} className="font-mono" />
          <Input label={t('trunks.codecs')} value={x.codecs} onChange={(e) => set('codecs', e.target.value.replace(/\s/g, ''))} className="font-mono" hint="alaw,ulaw,g722" />
        </div>
        <div className="mt-4"><Toggle label={t('trunks.srtp')} checked={x.srtp} disabled={x.transport !== 'tls'} onChange={(v) => set('srtp', v)} /></div>
        <div className="mt-3"><Toggle label={t('trunks.t38')} hint={t('trunks.t38Hint')} checked={x.t38} onChange={(v) => set('t38', v)} /></div>
      </details>
      <Toggle label={t('common.enabled')} checked={x.enabled} onChange={(v) => set('enabled', v)} />
    </Modal>
  )
}

/** Step-by-step setup of a provider: what to do in the provider portal and in Kiestoon. */
function ProviderGuide({ guide, docsUrl, open }: { guide: string; docsUrl?: string; open: boolean }) {
  const { t } = useTranslation()
  const steps = (key: string) => {
    const v = t(`trunks.guides.${guide}.${key}`, { returnObjects: true }) as unknown
    return Array.isArray(v) ? (v as string[]) : []
  }
  const section = (key: string) => (
    <div>
      <h4 className="mb-2 text-sm font-semibold text-ink">{t(`trunks.guides.${guide}.${key}Title`)}</h4>
      <ol className="flex flex-col gap-2">
        {steps(key).map((s, i) => (
          <li key={i} className="flex gap-3 text-sm text-ink-2">
            <span className="flex size-6 flex-none items-center justify-center rounded-full bg-accent/15 font-mono text-xs text-accent">{i + 1}</span>
            <span className="pt-0.5">{s}</span>
          </li>
        ))}
      </ol>
    </div>
  )
  return (
    <details open={open} className="rounded-xl border border-accent/30 bg-accent/5 p-4">
      <summary className="flex cursor-pointer items-center gap-2 text-sm font-medium text-ink">
        <BookOpen className="size-4 text-accent" aria-hidden /> {t(`trunks.guides.${guide}.title`)}
      </summary>
      <div className="mt-4 flex flex-col gap-5">
        <p className="text-sm text-muted">{t(`trunks.guides.${guide}.intro`)}</p>
        {section('portal')}
        {section('kiestoon')}
        {steps('troubleshoot').length > 0 && section('troubleshoot')}
        <p className="text-xs text-subtle">{t(`trunks.guides.${guide}.terms`)}</p>
        {docsUrl && (
          <a href={docsUrl} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1.5 text-sm text-accent hover:underline">
            {t('trunks.guideDocs')} <ExternalLink className="size-3.5" aria-hidden />
          </a>
        )}
      </div>
    </details>
  )
}

// ---- inbound ------------------------------------------------------------------

function InboundList() {
  const { t } = useTranslation()
  const list = useApi<InboundRoute[]>('/pbx/inbound')
  const exts = useApi<Extension[]>('/pbx/extensions')
  const [edit, setEdit] = useState<InboundRoute | null>(null)
  async function remove(x: InboundRoute) {
    if (!window.confirm(t('routes.confirmDelete', { name: x.name }))) return
    await api('DELETE', `/pbx/inbound/${x.id}`)
    list.reload()
  }
  return (
    <Card>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <p className="max-w-2xl text-sm text-muted">{t('routes.inboundText')}</p>
        <Button onClick={() => setEdit({ id: 0, name: '', did: '', destinationType: 'extension', destination: exts.data?.[0]?.number ?? '', cidPrefix: '', enabled: true })}>
          <Plus className="size-4" aria-hidden /> {t('routes.addInbound')}
        </Button>
      </div>
      <ErrorMessage error={list.error} />
      <Table head={[t('routes.name'), t('routes.did'), t('routes.destination'), '']}
        empty={list.data?.length === 0 && <p className="py-8 text-center text-sm text-muted">{t('routes.emptyInbound')}</p>}>
        {list.data?.map((x) => (
          <tr key={x.id} className={x.enabled ? '' : 'opacity-50'}>
            <td className={`${td} text-ink`}>{x.name}</td>
            <td className={`${td} font-mono`}>{x.did || <Badge tone="accent">{t('routes.anyNumber')}</Badge>}</td>
            <td className={td}>
              <DestinationLabel value={{ type: x.destinationType, target: x.destination }} />
            </td>
            <td className={`${td} whitespace-nowrap text-right`}>
              <IconBtn label={t('common.edit')} onClick={() => setEdit(x)}><Pencil className="size-4" /></IconBtn>
              <IconBtn label={t('common.delete')} danger onClick={() => remove(x)}><Trash2 className="size-4" /></IconBtn>
            </td>
          </tr>
        ))}
      </Table>
      {edit && <InboundForm value={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); list.reload() }} />}
    </Card>
  )
}

function InboundForm({ value, onClose, onSaved }: { value: InboundRoute; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [x, setX] = useState(value)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const set = <K extends keyof InboundRoute>(k: K, v: InboundRoute[K]) => setX((o) => ({ ...o, [k]: v }))
  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api(x.id ? 'PUT' : 'POST', x.id ? `/pbx/inbound/${x.id}` : '/pbx/inbound', x)
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={x.id ? t('routes.editInbound') : t('routes.addInbound')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <Input label={t('routes.name')} value={x.name} onChange={(e) => set('name', e.target.value)} />
      <Input label={t('routes.did')} value={x.did} onChange={(e) => set('did', e.target.value)} hint={t('routes.didHint')} className="font-mono" placeholder="020 123 4567" />
      <DestinationPicker label={t('routes.destination')} value={{ type: x.destinationType, target: x.destination }}
        onChange={(d) => setX((o) => ({ ...o, destinationType: (d.type || 'hangup') as DestType, destination: d.target }))} />
      <Input label={t('routes.cidPrefix')} value={x.cidPrefix} onChange={(e) => set('cidPrefix', e.target.value)} hint={t('routes.cidPrefixHint')} />
      <Toggle label={t('common.enabled')} checked={x.enabled} onChange={(v) => set('enabled', v)} />
    </Modal>
  )
}

// ---- outbound -----------------------------------------------------------------

function OutboundList() {
  const { t } = useTranslation()
  const list = useApi<OutboundRoute[]>('/pbx/outbound')
  const trunks = useApi<Trunk[]>('/pbx/trunks')
  const [edit, setEdit] = useState<OutboundRoute | null>(null)
  const name = (id: number) => trunks.data?.find((x) => x.id === id)?.name ?? `#${id}`
  async function move(i: number, d: number) {
    if (!list.data) return
    const ids = list.data.map((x) => x.id)
    const j = i + d
    if (j < 0 || j >= ids.length) return
    ;[ids[i], ids[j]] = [ids[j], ids[i]]
    await api('PUT', '/pbx/outbound/order', { ids })
    list.reload()
  }
  async function remove(x: OutboundRoute) {
    if (!window.confirm(t('routes.confirmDelete', { name: x.name }))) return
    await api('DELETE', `/pbx/outbound/${x.id}`)
    list.reload()
  }
  return (
    <Card>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <p className="max-w-2xl text-sm text-muted">{t('routes.outboundText')}</p>
        <Button disabled={!trunks.data?.length} onClick={() => setEdit({ id: 0, name: '', position: 0, patterns: [{ prefix: '', match: '0[1-9]XXXXXXXX', prepend: '' }], trunkIds: trunks.data?.[0] ? [trunks.data[0].id] : [], callerId: '', enabled: true })}>
          <Plus className="size-4" aria-hidden /> {t('routes.addOutbound')}
        </Button>
      </div>
      {trunks.data?.length === 0 && <Alert tone="warn">{t('routes.needTrunk')}</Alert>}
      <ErrorMessage error={list.error} />
      <Table head={['#', t('routes.name'), t('routes.patterns'), t('routes.trunks'), '']}
        empty={list.data?.length === 0 && <p className="py-8 text-center text-sm text-muted">{t('routes.emptyOutbound')}</p>}>
        {list.data?.map((x, i) => (
          <tr key={x.id} className={x.enabled ? '' : 'opacity-50'}>
            <td className={`${td} font-mono text-subtle`}>{i + 1}</td>
            <td className={`${td} text-ink`}>{x.name}</td>
            <td className={`${td} font-mono text-xs`}>{x.patterns.map((p) => `${p.prefix ? `${p.prefix}|` : ''}${p.match}`).join(', ')}</td>
            <td className={td}>{x.trunkIds.map(name).join(' → ')}</td>
            <td className={`${td} whitespace-nowrap text-right`}>
              <IconBtn label={t('routes.up')} onClick={() => move(i, -1)}><ArrowUp className="size-4" /></IconBtn>
              <IconBtn label={t('routes.down')} onClick={() => move(i, 1)}><ArrowDown className="size-4" /></IconBtn>
              <IconBtn label={t('common.edit')} onClick={() => setEdit(x)}><Pencil className="size-4" /></IconBtn>
              <IconBtn label={t('common.delete')} danger onClick={() => remove(x)}><Trash2 className="size-4" /></IconBtn>
            </td>
          </tr>
        ))}
      </Table>
      {edit && <OutboundForm value={edit} trunks={trunks.data ?? []} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); list.reload() }} />}
    </Card>
  )
}

function OutboundForm({ value, trunks, onClose, onSaved }: { value: OutboundRoute; trunks: Trunk[]; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [x, setX] = useState(value)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const setPat = (i: number, k: keyof DialPattern, v: string) =>
    setX((o) => ({ ...o, patterns: o.patterns.map((p, j) => (j === i ? { ...p, [k]: v } : p)) }))
  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api(x.id ? 'PUT' : 'POST', x.id ? `/pbx/outbound/${x.id}` : '/pbx/outbound', x)
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const unused = trunks.filter((tr) => !x.trunkIds.includes(tr.id))
  return (
    <Modal wide title={x.id ? t('routes.editOutbound') : t('routes.addOutbound')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Input label={t('routes.name')} value={x.name} onChange={(e) => setX({ ...x, name: e.target.value })} />
        <Input label={t('routes.callerId')} value={x.callerId} onChange={(e) => setX({ ...x, callerId: e.target.value.trim() })} hint={t('routes.callerIdHint')} className="font-mono" />
      </div>
      <fieldset className="flex flex-col gap-2">
        <legend className="mb-1 text-sm font-medium text-ink-2">{t('routes.patterns')}</legend>
        <p className="text-xs text-subtle">{t('routes.patternsHint')}</p>
        {x.patterns.map((p, i) => (
          <div key={i} className="grid grid-cols-[90px_1fr_90px_44px] items-end gap-2">
            <Input label={t('routes.prefix')} value={p.prefix} onChange={(e) => setPat(i, 'prefix', e.target.value)} className="font-mono" />
            <Input label={t('routes.match')} value={p.match} onChange={(e) => setPat(i, 'match', e.target.value.toUpperCase())} className="font-mono" />
            <Input label={t('routes.prepend')} value={p.prepend} onChange={(e) => setPat(i, 'prepend', e.target.value)} className="font-mono" />
            <IconBtn label={t('common.delete')} danger onClick={() => setX({ ...x, patterns: x.patterns.filter((_, j) => j !== i) })}><X className="size-4" /></IconBtn>
          </div>
        ))}
        <div className="flex flex-wrap gap-2">
          <Button variant="ghost" onClick={() => setX({ ...x, patterns: [...x.patterns, { prefix: '', match: '', prepend: '' }] })}><Plus className="size-4" /> {t('routes.addPattern')}</Button>
          <Button variant="ghost" onClick={() => setX({ ...x, patterns: [...x.patterns, { prefix: '', match: '0[1-9]XXXXXXXX', prepend: '' }, { prefix: '', match: '06XXXXXXXX', prepend: '' }, { prefix: '', match: '0800XXXX.', prepend: '' }, { prefix: '', match: '14XXX', prepend: '' }] })}>{t('routes.presetNL')}</Button>
          <Button variant="ghost" onClick={() => setX({ ...x, patterns: [...x.patterns, { prefix: '', match: '00Z.', prepend: '' }] })}>{t('routes.presetIntl')}</Button>
        </div>
      </fieldset>
      <fieldset className="flex flex-col gap-2">
        <legend className="mb-1 text-sm font-medium text-ink-2">{t('routes.trunkOrder')}</legend>
        <ol className="flex flex-col gap-2">
          {x.trunkIds.map((id, i) => (
            <li key={id} className="flex items-center gap-2 rounded-xl border border-line bg-surface-2/60 px-3 py-2 text-sm">
              <span className="font-mono text-subtle">{i + 1}</span>
              <span className="flex-1">{trunks.find((tr) => tr.id === id)?.name}</span>
              <IconBtn label={t('routes.up')} onClick={() => { const ids = [...x.trunkIds]; if (i > 0) { [ids[i - 1], ids[i]] = [ids[i], ids[i - 1]] } setX({ ...x, trunkIds: ids }) }}><ArrowUp className="size-4" /></IconBtn>
              <IconBtn label={t('common.delete')} danger onClick={() => setX({ ...x, trunkIds: x.trunkIds.filter((y) => y !== id) })}><X className="size-4" /></IconBtn>
            </li>
          ))}
        </ol>
        {unused.length > 0 && (
          <Select label={t('routes.addTrunk')} value="" onChange={(e) => e.target.value && setX({ ...x, trunkIds: [...x.trunkIds, Number(e.target.value)] })}>
            <option value="">–</option>
            {unused.map((tr) => <option key={tr.id} value={tr.id}>{tr.name}</option>)}
          </Select>
        )}
      </fieldset>
      <Toggle label={t('common.enabled')} checked={x.enabled} onChange={(v) => setX({ ...x, enabled: v })} />
    </Modal>
  )
}
