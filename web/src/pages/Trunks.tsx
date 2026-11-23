import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowDown, ArrowUp, BookOpen, ExternalLink, Hash, Pencil, Plus, RefreshCw, Route, Trash2, Waypoints, X } from 'lucide-react'
import { Badge, Empty, Modal, Select, Table, Tabs, Textarea, Toggle, td, useApi } from '../components/kit'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api, ApiError } from '../lib/api'
import type { DestType, DialPattern, Extension, InboundRoute, Live, OutboundRoute, Trunk, TrunkPreset } from '../lib/types'
import { DestinationLabel, DestinationPicker, NumberOptions, Segmented, useNumbers } from '../components/pbx'
import { expandBlock, formatNumber, MAX_BLOCK, parseNumbers } from '../lib/numbers'
import { localDocsUrl } from '../components/help'

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
  callerId: '', srtp: false, t38: false, enabled: true, numbers: [],
}

function TrunkList() {
  const { t } = useTranslation()
  const list = useApi<Trunk[]>('/pbx/trunks')
  const live = useApi<Live>('/live', 5000)
  const [edit, setEdit] = useState<Trunk | null>(null)
  const [refreshing, setRefreshing] = useState<Record<number, boolean>>({})
  const [result, setResult] = useState<Record<number, { status?: string; error?: unknown }>>({})
  const state = (id: number) => live.data?.trunks.find((x) => x.id === id)?.status ?? 'unknown'
  async function refresh(x: Trunk) {
    setRefreshing((o) => ({ ...o, [x.id]: true }))
    setResult((o) => ({ ...o, [x.id]: {} }))
    try {
      const r = await api<{ status: string }>('POST', `/pbx/trunks/${x.id}/register`)
      setResult((o) => ({ ...o, [x.id]: { status: r.status } }))
      live.reload()
    } catch (e) {
      setResult((o) => ({ ...o, [x.id]: { error: e } }))
    } finally {
      setRefreshing((o) => ({ ...o, [x.id]: false }))
    }
  }
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
            <td className={`${td} text-ink`}>
              {x.name}
              {x.numbers?.length > 0 && <span className="mt-0.5 flex items-center gap-1 text-xs text-subtle"><Hash className="size-3" aria-hidden />{t('trunks.numbersCount', { count: x.numbers.length })}</span>}
            </td>
            <td className={`${td} font-mono text-xs`}>{x.host}{x.port ? `:${x.port}` : ''} {!x.port && <span className="text-subtle">SRV</span>} <span className="uppercase text-subtle">{x.transport}</span></td>
            <td className={td}>{t(`trunks.mode.${x.authMode}`)}</td>
            <td className={td}>
              <Badge tone={statusTone(state(x.id))} dot>{t(`trunks.state.${state(x.id)}`, { defaultValue: state(x.id) })}</Badge>
              <RefreshResult value={result[x.id]} busy={refreshing[x.id]} />
            </td>
            <td className={`${td} font-mono`}>{x.callerId || '–'}</td>
            <td className={`${td} whitespace-nowrap text-right`}>
              {x.authMode === 'register' && x.enabled && (
                <IconBtn label={t('trunks.refresh')} onClick={() => !refreshing[x.id] && refresh(x)}>
                  <RefreshCw className={`size-4 ${refreshing[x.id] ? 'animate-spin text-accent' : ''}`} />
                </IconBtn>
              )}
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

/** The outcome of a "refresh registration", shown under the trunk's status. */
function RefreshResult({ value, busy }: { value?: { status?: string; error?: unknown }; busy?: boolean }) {
  const { t } = useTranslation()
  if (busy) return <p className="mt-1 text-xs text-subtle">{t('trunks.refreshBusy')}</p>
  if (!value) return null
  if (value.error) {
    const code = value.error instanceof ApiError ? value.error.code : 'internal'
    const detail = value.error instanceof ApiError ? value.error.detail : ''
    return <p className="mt-1 max-w-xs text-xs text-danger-ink">{t(`errors.${code}`, { defaultValue: t('errors.internal') })}{detail ? ` (${detail})` : ''}</p>
  }
  if (!value.status) return null
  const tone = value.status === 'registered' ? 'text-ok' : value.status === 'rejected' ? 'text-danger-ink' : 'text-subtle'
  return <p className={`mt-1 max-w-xs text-xs ${tone}`}>{t(`trunks.refreshResult.${value.status}`, { defaultValue: t('trunks.refreshResult.other', { status: value.status }) })}</p>
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
      <Select name="provider" label={t('trunks.provider')} value={x.provider} onChange={(e) => applyPreset(e.target.value)}>
        {presets.data?.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
      </Select>
      {preset && <Alert tone="warn">{t(`trunks.notes.${preset.note}`)}</Alert>}
      {preset?.guide && <ProviderGuide guide={preset.guide} docsUrl={preset.docsUrl} open={!x.id} />}
      <div className="grid gap-4 sm:grid-cols-2">
        <Input name="name" label={t('trunks.name')} value={x.name} onChange={(e) => set('name', e.target.value)} />
        <Select name="authMode" label={t('trunks.authMode')} value={x.authMode} onChange={(e) => set('authMode', e.target.value as Trunk['authMode'])}>
          <option value="register">{t('trunks.mode.register')}</option>
          <option value="ip">{t('trunks.mode.ip')}</option>
        </Select>
      </div>
      <div className="grid gap-4 sm:grid-cols-[1fr_110px_130px]">
        <Input name="host" label={t('trunks.host')} value={x.host} onChange={(e) => set('host', e.target.value.trim())} className="font-mono" placeholder="sip.provider.nl" />
        <Input name="port" label={t('trunks.port')} type="number" min={0} value={x.port} hint={x.port === 0 ? t('trunks.portSrv') : undefined} onChange={(e) => set('port', Number(e.target.value))} />
        <Select name="transport" label={t('trunks.transport')} value={x.transport} onChange={(e) => { const v = e.target.value as Trunk['transport']; setX((o) => ({ ...o, transport: v, srtp: v === 'tls' ? o.srtp : false, port: v === 'tls' && o.port === 5060 ? 5061 : v !== 'tls' && o.port === 5061 ? 5060 : o.port })) }}>
          <option value="udp">UDP</option><option value="tcp">TCP</option><option value="tls">TLS</option>
        </Select>
      </div>
      {x.authMode === 'register' && (
        <div className="grid gap-4 sm:grid-cols-3">
          <Input name="username" label={t('trunks.username')} value={x.username} onChange={(e) => set('username', e.target.value.trim())} className="font-mono" />
          <Input name="authUsername" label={t('trunks.authUsername')} value={x.authUsername} onChange={(e) => set('authUsername', e.target.value.trim())} hint={t('trunks.authUsernameHint')} className="font-mono" />
          <Input name="secret" label={t('trunks.secret')} type="password" value={x.secret ?? ''} onChange={(e) => set('secret', e.target.value)} placeholder={x.secretSet ? '••••••••' : ''} hint={x.secretSet ? t('common.keepSecret') : undefined} autoComplete="new-password" />
        </div>
      )}
      <Textarea name="matchIps" label={t('trunks.matchIps')} value={x.matchIps.split(',').filter(Boolean).join('\n')} onChange={(e) => set('matchIps', e.target.value.split(/\s+/).filter(Boolean).join(','))} hint={t('trunks.matchIpsHint')} />
      <div className="grid gap-4 sm:grid-cols-3">
        <Input name="callerId" label={t('trunks.callerId')} value={x.callerId} onChange={(e) => set('callerId', e.target.value.trim())} hint={t('trunks.callerIdHint')} className="font-mono" list="trunk-form-numbers" />
        <datalist id="trunk-form-numbers">{x.numbers.map((n) => <option key={n} value={n}>{formatNumber(n)}</option>)}</datalist>
        <Select name="didSource" label={t('trunks.didSource')} value={x.didSource} onChange={(e) => set('didSource', e.target.value as Trunk['didSource'])}>
          <option value="to">{t('trunks.did.to')}</option>
          <option value="ruri">{t('trunks.did.ruri')}</option>
        </Select>
        <Input name="maxChannels" label={t('trunks.maxChannels')} type="number" min={0} value={x.maxChannels} onChange={(e) => set('maxChannels', Number(e.target.value))} hint={t('trunks.maxChannelsHint')} />
      </div>
      <NumbersEditor value={x.numbers ?? []} onChange={(v) => set('numbers', v)} />
      <details className="rounded-xl border border-line p-4">
        <summary className="cursor-pointer text-sm text-ink-2">{t('common.advanced')}</summary>
        <div className="mt-4 grid gap-4 sm:grid-cols-3">
          <Input name="fromUser" label={t('trunks.fromUser')} value={x.fromUser} onChange={(e) => set('fromUser', e.target.value.trim())} className="font-mono" />
          <Input name="fromDomain" label={t('trunks.fromDomain')} value={x.fromDomain} onChange={(e) => set('fromDomain', e.target.value.trim())} className="font-mono" />
          <Input name="codecs" label={t('trunks.codecs')} value={x.codecs} onChange={(e) => set('codecs', e.target.value.replace(/\s/g, ''))} className="font-mono" hint="alaw,ulaw,g722" />
        </div>
        <div className="mt-4"><Toggle name="srtp" label={t('trunks.srtp')} checked={x.srtp} disabled={x.transport !== 'tls'} onChange={(v) => set('srtp', v)} /></div>
        <div className="mt-3"><Toggle name="t38" label={t('trunks.t38')} hint={t('trunks.t38Hint')} checked={x.t38} onChange={(v) => set('t38', v)} /></div>
      </details>
      <Toggle name="enabled" label={t('common.enabled')} checked={x.enabled} onChange={(v) => set('enabled', v)} />
    </Modal>
  )
}

/**
 * The phone numbers of a trunk: add one or more numbers at once (pasted, one per line) or a whole
 * number block (first number plus last number or count).
 */
function NumbersEditor({ value, onChange }: { value: string[]; onChange: (v: string[]) => void }) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<'block' | 'list'>('block')
  const [first, setFirst] = useState('')
  const [last, setLast] = useState('')
  const [paste, setPaste] = useState('')
  const [showAll, setShowAll] = useState(false)
  const block = first && last ? expandBlock(first, last) : null
  const blockBad = !!first && !!last && !block
  const pasted = parseNumbers(paste)
  function add(list: string[]) {
    onChange([...new Set([...value, ...list])])
  }
  const shown = showAll ? value : value.slice(0, 40)
  return (
    <fieldset className="flex flex-col gap-3 rounded-xl border border-line p-4">
      <legend className="px-1 text-sm font-medium text-ink-2">{t('trunks.numbers')}</legend>
      <p className="text-xs text-subtle">{t('trunks.numbersHint')}</p>
      {value.length > 0 && (
        <div className="flex flex-col gap-2">
          <div className="flex flex-wrap gap-1.5">
            {shown.map((n) => (
              <span key={n} className="inline-flex items-center gap-1 rounded-lg border border-line bg-surface-2/60 py-1 pl-2.5 pr-1 font-mono text-xs text-ink-2">
                {formatNumber(n)}
                <button type="button" aria-label={t('common.delete')} title={t('common.delete')} onClick={() => onChange(value.filter((v) => v !== n))}
                  className="inline-flex size-5 items-center justify-center rounded text-subtle hover:bg-danger/10 hover:text-danger"><X className="size-3" /></button>
              </span>
            ))}
            {value.length > shown.length && (
              <button type="button" onClick={() => setShowAll(true)} className="rounded-lg px-2 text-xs text-accent hover:underline">{t('trunks.numbersMore', { count: value.length - shown.length })}</button>
            )}
          </div>
          <div className="flex items-center justify-between text-xs text-subtle">
            <span>{t('trunks.numbersCount', { count: value.length })}</span>
            <button type="button" onClick={() => onChange([])} className="text-danger-ink hover:underline">{t('trunks.numbersClear')}</button>
          </div>
        </div>
      )}
      <Segmented<'block' | 'list'> value={mode} onChange={setMode} options={[
        { key: 'block', label: t('trunks.numbersBlock') },
        { key: 'list', label: t('trunks.numbersList') },
      ]} />
      {mode === 'block' ? (
        <div className="grid items-start gap-3 sm:grid-cols-[1fr_1fr_auto]">
          <Input label={t('trunks.blockFirst')} value={first} onChange={(e) => setFirst(e.target.value)} className="font-mono" placeholder="020 123 4500" />
          <Input label={t('trunks.blockLast')} value={last} onChange={(e) => setLast(e.target.value)} className="font-mono" placeholder="020 123 4599 / 100"
            hint={blockBad ? <span className="text-danger-ink">{t('trunks.blockInvalid', { max: MAX_BLOCK })}</span> : block ? t('trunks.blockPreview', { first: formatNumber(block[0]), last: formatNumber(block[block.length - 1]), count: block.length }) : t('trunks.blockHint')} />
          <div className="sm:pt-7">
            <Button variant="ghost" disabled={!block} onClick={() => { if (block) { add(block); setFirst(''); setLast('') } }}><Plus className="size-4" aria-hidden /> {t('trunks.numbersAdd', { count: block?.length ?? 0 })}</Button>
          </div>
        </div>
      ) : (
        <div className="flex flex-col gap-2">
          <Textarea name="numbers" label={t('trunks.numbersPaste')} value={paste} onChange={(e) => setPaste(e.target.value)} hint={t('trunks.numbersPasteHint')} rows={4} />
          <div><Button variant="ghost" disabled={pasted.length === 0} onClick={() => { add(pasted); setPaste('') }}><Plus className="size-4" aria-hidden /> {t('trunks.numbersAdd', { count: pasted.length })}</Button></div>
        </div>
      )}
    </fieldset>
  )
}

/** Step-by-step setup of a provider: what to do in the provider portal and in Kiestoon. */
function ProviderGuide({ guide, docsUrl, open }: { guide: string; docsUrl?: string; open: boolean }) {
  const { t, i18n } = useTranslation()
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
          <a href={localDocsUrl(docsUrl, i18n.language)} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1.5 text-sm text-accent hover:underline">
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
  const [edit, setEdit] = useState<{ route: InboundRoute; dids?: string[] } | null>(null)
  const [reloadKey, setReloadKey] = useState(0)
  const numbers = useNumbers(reloadKey)
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const blank = (did = ''): InboundRoute => ({ id: 0, name: '', did, destinationType: 'extension', destination: exts.data?.[0]?.number ?? '', cidPrefix: '', enabled: true })
  const reload = () => { list.reload(); setReloadKey((k) => k + 1); setPicked(new Set()) }
  const free = numbers?.filter((n) => !n.routeId) ?? []
  const toggle = (n: string) => setPicked((o) => { const s = new Set(o); if (s.has(n)) s.delete(n); else s.add(n); return s })
  async function remove(x: InboundRoute) {
    if (!window.confirm(t('routes.confirmDelete', { name: x.name }))) return
    await api('DELETE', `/pbx/inbound/${x.id}`)
    reload()
  }
  return (
    <>
    {free.length > 0 && (
      <Card className="mb-4">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 className="text-sm font-semibold text-ink">{t('routes.freeNumbers', { count: free.length })}</h3>
            <p className="text-xs text-subtle">{t('routes.freeNumbersText')}</p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button variant="ghost" onClick={() => setPicked(picked.size === free.length ? new Set() : new Set(free.map((n) => n.number)))}>
              {picked.size === free.length ? t('routes.selectNone') : t('routes.selectAll')}
            </Button>
            <Button disabled={picked.size === 0} onClick={() => setEdit({ route: blank(), dids: free.map((n) => n.number).filter((n) => picked.has(n)) })}>
              <Route className="size-4" aria-hidden /> {t('routes.routeSelected', { count: picked.size })}
            </Button>
          </div>
        </div>
        <div className="flex max-h-56 flex-wrap gap-1.5 overflow-y-auto">
          {free.map((n) => (
            <label key={n.number} className={`inline-flex cursor-pointer items-center gap-2 rounded-lg border px-2.5 py-1.5 font-mono text-xs transition ${picked.has(n.number) ? 'border-accent/60 bg-accent/10 text-ink' : 'border-line bg-surface-2/60 text-ink-2 hover:border-accent/40'}`}>
              <input type="checkbox" className="accent-[var(--color-accent)]" checked={picked.has(n.number)} onChange={() => toggle(n.number)} />
              {formatNumber(n.number)} <span className="font-sans text-subtle">{n.trunkName}</span>
            </label>
          ))}
        </div>
      </Card>
    )}
    <Card>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <p className="max-w-2xl text-sm text-muted">{t('routes.inboundText')}</p>
        <Button onClick={() => setEdit({ route: blank() })}>
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
              <IconBtn label={t('common.edit')} onClick={() => setEdit({ route: x })}><Pencil className="size-4" /></IconBtn>
              <IconBtn label={t('common.delete')} danger onClick={() => remove(x)}><Trash2 className="size-4" /></IconBtn>
            </td>
          </tr>
        ))}
      </Table>
      {edit && <InboundForm value={edit.route} dids={edit.dids} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); reload() }} />}
    </Card>
    </>
  )
}

/** Edits one inbound route, or creates one route per number for a set of trunk numbers (dids). */
function InboundForm({ value, dids, onClose, onSaved }: { value: InboundRoute; dids?: string[]; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [x, setX] = useState(value)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [done, setDone] = useState(0)
  const multi = !!dids && dids.length > 0
  const set = <K extends keyof InboundRoute>(k: K, v: InboundRoute[K]) => setX((o) => ({ ...o, [k]: v }))
  async function save() {
    setBusy(true)
    setError(null)
    try {
      if (multi) {
        // One route per number, so each number can later get its own destination.
        for (let i = done; i < dids.length; i++) {
          const name = x.name.trim() ? (dids.length > 1 ? `${x.name.trim()} ${formatNumber(dids[i])}` : x.name.trim()) : formatNumber(dids[i])
          await api('POST', '/pbx/inbound', { ...x, did: dids[i], name })
          setDone(i + 1)
        }
      } else {
        await api(x.id ? 'PUT' : 'POST', x.id ? `/pbx/inbound/${x.id}` : '/pbx/inbound', x)
      }
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
      {multi && done > 0 && done < dids.length && <Alert tone="warn">{t('routes.partlyDone', { done, count: dids.length })}</Alert>}
      <Input name="name" label={t('routes.name')} value={x.name} onChange={(e) => set('name', e.target.value)} hint={multi ? t('routes.multiNameHint') : undefined} />
      {multi ? (
        <div className="flex flex-col gap-1.5">
          <span className="text-sm font-medium text-ink-2">{t('routes.multiNumbers', { count: dids.length })}</span>
          <div className="flex max-h-32 flex-wrap gap-1.5 overflow-y-auto">
            {dids.map((d, i) => <span key={d} className={`rounded-lg border border-line px-2 py-1 font-mono text-xs ${i < done ? 'text-ok' : 'text-ink-2'}`}>{formatNumber(d)}</span>)}
          </div>
        </div>
      ) : (
        <>
          <Input name="did" label={t('routes.did')} value={x.did} onChange={(e) => set('did', e.target.value)} hint={t('routes.didHint')} className="font-mono" placeholder="020 123 4567" list="inbound-numbers" />
          <NumberOptions id="inbound-numbers" />
        </>
      )}
      <DestinationPicker label={t('routes.destination')} value={{ type: x.destinationType, target: x.destination }}
        onChange={(d) => setX((o) => ({ ...o, destinationType: (d.type || 'hangup') as DestType, destination: d.target }))} />
      <Input name="cidPrefix" label={t('routes.cidPrefix')} value={x.cidPrefix} onChange={(e) => set('cidPrefix', e.target.value)} hint={t('routes.cidPrefixHint')} />
      <Toggle name="enabled" label={t('common.enabled')} checked={x.enabled} onChange={(v) => set('enabled', v)} />
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
        <Input name="name" label={t('routes.name')} value={x.name} onChange={(e) => setX({ ...x, name: e.target.value })} />
        <Input name="callerId" label={t('routes.callerId')} value={x.callerId} onChange={(e) => setX({ ...x, callerId: e.target.value.trim() })} hint={t('routes.callerIdHint')} className="font-mono" list="outbound-numbers" />
        <NumberOptions id="outbound-numbers" />
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
          <Select name="trunkIds" label={t('routes.addTrunk')} value="" onChange={(e) => e.target.value && setX({ ...x, trunkIds: [...x.trunkIds, Number(e.target.value)] })}>
            <option value="">–</option>
            {unused.map((tr) => <option key={tr.id} value={tr.id}>{tr.name}</option>)}
          </Select>
        )}
      </fieldset>
      <Toggle name="enabled" label={t('common.enabled')} checked={x.enabled} onChange={(v) => setX({ ...x, enabled: v })} />
    </Modal>
  )
}
