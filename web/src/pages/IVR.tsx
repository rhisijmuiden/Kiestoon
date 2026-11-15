import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowLeft, Clock, GitBranch, Megaphone, Pencil, PhoneIncoming, Plus, Trash2, X } from 'lucide-react'
import { Badge, Empty, Select, Table, Toggle, td, useApi } from '../components/kit'
import { DestinationLabel, DestinationPicker, IconButton, resetDestinations, useSaver } from '../components/pbx'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { Destination, IVR, Prompt } from '../lib/types'

const KEYS = ['1', '2', '3', '4', '5', '6', '7', '8', '9', '*', '0', '#']

type Layout = { labels?: Record<string, string> }

const blank: IVR = {
  id: 0, name: '', promptId: 0, timeout: 5, retries: 2, directDial: false, options: [], timeoutDestination: { type: 'hangup', target: '' }, layout: {},
}

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
    return <IVRBuilder value={edit} prompts={prompts.data ?? []} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); resetDestinations(); list.reload() }} />
  }
  const promptName = (id: number) => prompts.data?.find((p) => p.id === id)?.name
  return (
    <>
      <PageHeader title={t('ivr.title')} subtitle={t('ivr.subtitle')}>
        <Button onClick={() => setEdit(blank)}><Plus className="size-4" aria-hidden /> {t('ivr.add')}</Button>
      </PageHeader>
      <ErrorMessage error={list.error} />
      <Card>
        <Table head={[t('common.name'), t('ivr.prompt'), t('ivr.options'), t('ivr.noInput'), '']}
          empty={list.data?.length === 0 && <Empty icon={<GitBranch className="size-7" />} title={t('ivr.emptyTitle')} text={t('ivr.emptyText')} action={<Button onClick={() => setEdit(blank)}>{t('ivr.add')}</Button>} />}>
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

/** The visual builder: the caller enters at the top, each key is a branch to a destination. */
function IVRBuilder({ value, prompts, onClose, onSaved }: { value: IVR; prompts: Prompt[]; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [v, setV] = useState<IVR>(value)
  const [layout, setLayout] = useState<Layout>(() => (value.layout && typeof value.layout === 'object' ? (value.layout as Layout) : {}))
  const [selected, setSelected] = useState<string | null>(value.options[0]?.digit ?? null)
  const { busy, error, save } = useSaver<IVR>('/pbx/ivrs', onSaved)
  const set = <K extends keyof IVR>(k: K, val: IVR[K]) => setV((x) => ({ ...x, [k]: val }))
  const option = (d: string) => v.options.find((o) => o.digit === d)
  const setOption = (d: string, dest: Destination | null) => {
    const rest = v.options.filter((o) => o.digit !== d)
    set('options', dest ? [...rest, { digit: d, destination: dest }].sort((a, b) => KEYS.indexOf(a.digit) - KEYS.indexOf(b.digit)) : rest)
  }
  const label = (d: string) => layout.labels?.[d] ?? ''
  const setLabel = (d: string, s: string) => setLayout((l) => ({ ...l, labels: { ...l.labels, [d]: s } }))
  function pressKey(d: string) {
    if (!option(d)) setOption(d, { type: 'extension', target: '' })
    setSelected(d)
  }
  const sel = selected ? option(selected) : undefined
  const promptName = prompts.find((p) => p.id === v.promptId)?.name

  return (
    <>
      <PageHeader title={v.id ? t('ivr.edit', { name: value.name }) : t('ivr.add')} subtitle={t('ivr.builderSubtitle')}>
        <Button variant="ghost" onClick={onClose}><ArrowLeft className="size-4" aria-hidden /> {t('common.back')}</Button>
        <Button busy={busy} onClick={() => save({ ...v, layout })}>{t('common.save')}</Button>
      </PageHeader>
      <ErrorMessage error={error} />
      <div className="grid gap-6 xl:grid-cols-[340px_1fr]">
        <Card className="flex flex-col gap-4 self-start">
          <Input label={t('common.name')} value={v.name} onChange={(e) => set('name', e.target.value)} autoFocus />
          <Select label={t('ivr.prompt')} value={v.promptId} onChange={(e) => set('promptId', Number(e.target.value))} hint={t('ivr.promptHint')}>
            <option value={0}>{t('ivr.noPrompt')}</option>
            {prompts.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </Select>
          <div className="grid grid-cols-2 gap-3">
            <Input label={t('ivr.timeout')} type="number" min={1} max={60} value={v.timeout} onChange={(e) => set('timeout', Number(e.target.value))} hint={t('common.seconds')} />
            <Input label={t('ivr.retries')} type="number" min={0} max={5} value={v.retries} onChange={(e) => set('retries', Number(e.target.value))} hint={t('ivr.retriesHint')} />
          </div>
          <Toggle label={t('ivr.directDial')} hint={t('ivr.directDialHint')} checked={v.directDial} onChange={(x) => set('directDial', x)} />
          <div>
            <span className="text-sm font-medium text-ink-2">{t('ivr.keypad')}</span>
            <p className="mb-3 mt-0.5 text-xs text-subtle">{t('ivr.keypadHint')}</p>
            <div className="grid grid-cols-3 gap-2" role="group" aria-label={t('ivr.keypad')}>
              {KEYS.map((d) => {
                const used = !!option(d)
                return (
                  <button key={d} type="button" onClick={() => pressKey(d)} aria-pressed={selected === d}
                    className={`flex h-14 flex-col items-center justify-center rounded-xl border font-mono text-xl transition ${
                      selected === d ? 'border-accent bg-accent/20 text-ink shadow-[0_0_24px_-6px_var(--color-accent)]'
                        : used ? 'border-accent/45 bg-accent/10 text-ink' : 'border-line text-muted hover:border-accent/40 hover:text-ink'
                    }`}>
                    {d}
                    {used && <span className="max-w-full truncate px-1 font-sans text-[10px] text-subtle">{label(d) || t(`dest.types.${option(d)!.destination.type}`)}</span>}
                  </button>
                )
              })}
            </div>
          </div>
        </Card>

        <div className="flex min-w-0 flex-col gap-6">
          {/* Flow canvas */}
          <section className="card relative overflow-x-auto p-6" aria-label={t('ivr.flow')}>
            <div className="absolute inset-0 bg-[radial-gradient(var(--color-grid-dot)_1px,transparent_1px)] [background-size:22px_22px]" aria-hidden />
            <div className="relative flex min-w-max flex-col items-center">
              <FlowNode icon={<PhoneIncoming className="size-4" aria-hidden />} title={t('ivr.caller')} />
              <Stem />
              <FlowNode icon={<Megaphone className="size-4" aria-hidden />} title={v.name || t('ivr.untitled')} sub={promptName ?? t('ivr.noPrompt')} accent />
              <Stem />
              <div className="relative flex items-start gap-4">
                {v.options.length + 1 > 1 && <div className="absolute left-[88px] right-[88px] top-0 h-px bg-accent/40" aria-hidden />}
                {v.options.map((o) => (
                  <Branch key={o.digit} active={selected === o.digit} onClick={() => setSelected(o.digit)}
                    head={<span className="inline-flex size-8 items-center justify-center rounded-lg border border-accent/60 bg-accent/15 font-mono text-base text-ink">{o.digit}</span>}
                    title={label(o.digit)} dest={o.destination} />
                ))}
                <Branch active={selected === 'timeout'} onClick={() => setSelected('timeout')}
                  head={<span className="inline-flex size-8 items-center justify-center rounded-lg border border-warn/50 bg-warn/10 text-warn"><Clock className="size-4" aria-hidden /></span>}
                  title={t('ivr.noInput')} dest={v.timeoutDestination} />
              </div>
            </div>
          </section>

          {/* Inspector */}
          <Card>
            {selected === 'timeout' ? (
              <div className="flex flex-col gap-4">
                <h2 className="font-display text-lg font-semibold">{t('ivr.noInput')}</h2>
                <p className="text-sm text-muted">{t('ivr.noInputHint', { retries: v.retries })}</p>
                <DestinationPicker label={t('ivr.goTo')} value={v.timeoutDestination} onChange={(d) => set('timeoutDestination', d)} exclude={{ type: 'ivr', target: String(v.id) }} />
              </div>
            ) : sel ? (
              <div className="flex flex-col gap-4">
                <div className="flex items-center justify-between gap-3">
                  <h2 className="font-display text-lg font-semibold">{t('ivr.key', { key: sel.digit })}</h2>
                  <Button variant="danger" onClick={() => { setOption(sel.digit, null); setSelected(null) }}><X className="size-4" aria-hidden /> {t('ivr.removeKey')}</Button>
                </div>
                <Input label={t('ivr.label')} value={label(sel.digit)} onChange={(e) => setLabel(sel.digit, e.target.value)} placeholder={t('ivr.labelPlaceholder')} hint={t('ivr.labelHint')} maxLength={40} />
                <DestinationPicker label={t('ivr.goTo')} value={sel.destination} onChange={(d) => setOption(sel.digit, d)} exclude={{ type: 'ivr', target: String(v.id) }} />
              </div>
            ) : (
              <p className="py-6 text-center text-sm text-muted">{t('ivr.selectKey')}</p>
            )}
          </Card>
        </div>
      </div>
    </>
  )
}

function Stem() {
  return <div className="h-7 w-px bg-gradient-to-b from-accent/60 to-accent/30" aria-hidden />
}

function FlowNode({ icon, title, sub, accent }: { icon: ReactNode; title: string; sub?: string; accent?: boolean }) {
  return (
    <div className={`flex min-w-[220px] items-center gap-3 rounded-xl border px-4 py-3 ${accent ? 'border-accent/50 bg-accent/10 shadow-[0_0_40px_-16px_var(--color-accent)]' : 'border-line bg-surface-2'}`}>
      <span className="flex size-8 items-center justify-center rounded-lg bg-tint/5 text-accent">{icon}</span>
      <div className="min-w-0">
        <div className="truncate text-sm font-medium text-ink">{title}</div>
        {sub && <div className="truncate text-xs text-subtle">{sub}</div>}
      </div>
    </div>
  )
}

function Branch({ head, title, dest, active, onClick }: { head: ReactNode; title: string; dest: Destination; active: boolean; onClick: () => void }) {
  return (
    <div className="flex w-[176px] flex-col items-center">
      <div className="h-6 w-px bg-accent/40" aria-hidden />
      <button type="button" onClick={onClick} aria-pressed={active}
        className={`flex w-full flex-col items-start gap-2 rounded-xl border p-3 text-left transition ${active ? 'border-accent bg-accent/12 shadow-[0_0_30px_-10px_var(--color-accent)]' : 'border-line bg-surface-2 hover:border-accent/40'}`}>
        <div className="flex w-full items-center gap-2">
          {head}
          <span className="min-w-0 flex-1 truncate text-sm text-ink">{title}</span>
        </div>
        <div className="w-full truncate text-xs"><DestinationLabel value={dest} /></div>
        {!dest.type || (dest.type !== 'hangup' && !dest.target) ? <Badge tone="warn">!</Badge> : null}
      </button>
    </div>
  )
}
