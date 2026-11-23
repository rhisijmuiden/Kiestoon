import { useState, type DragEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { BellOff, CircleParking, PhoneForwarded, GripVertical, Hash, Megaphone, Phone as PhoneIcon, Plus, SquareParking, Trash2, User, Voicemail } from 'lucide-react'
import { Select } from './kit'
import { Button, Input } from './ui'
import { DestinationPicker } from './pbx'
import type { DestType, Destination, Destinations, KeyType, ModuleType, PhoneKey, PhoneModule } from '../lib/types'

/** A visual editor for the programmable keys of a phone and its expansion modules. */

export const keyTypes: KeyType[] = ['none', 'line', 'blf', 'speed', 'park', 'parkslot', 'intercom', 'dnd', 'voicemail', 'forward']
export const blankKey = (i: number): PhoneKey => ({ type: i === 0 ? 'line' : 'none', value: '', label: '' })

/** Fills a key list up to n keys. */
export function fillKeys(keys: PhoneKey[] | undefined, n: number, first = true): PhoneKey[] {
  return Array.from({ length: n }, (_, i) => keys?.[i] ?? (first ? blankKey(i) : { type: 'none', value: '', label: '' }))
}

const tone: Record<KeyType, string> = {
  none: 'border-line bg-field text-subtle',
  line: 'border-accent/40 bg-accent/10 text-ink',
  blf: 'border-ok/40 bg-ok/10 text-ink',
  speed: 'border-accent-2/40 bg-accent-2/10 text-ink',
  park: 'border-warn/40 bg-warn/10 text-ink',
  parkslot: 'border-warn/40 bg-warn/10 text-ink',
  intercom: 'border-accent-2/40 bg-accent-2/10 text-ink',
  dnd: 'border-danger/40 bg-danger/10 text-ink',
  voicemail: 'border-accent/40 bg-accent/10 text-ink',
  forward: 'border-accent-2/40 bg-accent-2/10 text-ink',
}
const icons: Record<KeyType, typeof PhoneIcon> = {
  none: Hash, line: PhoneIcon, blf: User, speed: Hash, park: SquareParking, parkslot: CircleParking, intercom: Megaphone, dnd: BellOff, voicemail: Voicemail, forward: PhoneForwarded,
}

type Surface = { key: string; title: string; keys: PhoneKey[]; perPage: number; module?: number }
type Ext = { number: string; name: string }

/** Keys per visible page of a phone, roughly as the display shows them. */
function phonePerPage(vendor: string, keys: number, perPage?: number) {
  if (perPage && keys > perPage) return perPage
  if (keys <= 12) return keys
  return vendor === 'snom' ? 12 : 10
}

/** The destination types a forward key can send calls to (pbx.ForwardKeyTypes). */
export const forwardTypes: DestType[] = ['external', 'extension', 'voicemail', 'ringgroup', 'queue', 'ivr', 'timecondition', 'conference', 'oncall', 'followsun', 'fax']

/** Reads a forward key value: a phone number, or "<type>:<target>" for a destination inside Kiestoon. */
export function forwardDest(v: string): Destination {
  const i = v.indexOf(':')
  return i < 0 ? { type: 'external', target: v } : { type: v.slice(0, i) as DestType, target: v.slice(i + 1) }
}
const forwardCodes: Record<string, DestType> = {
  '01': 'extension', '02': 'voicemail', '03': 'ringgroup', '04': 'queue', '05': 'ivr', '06': 'timecondition', '07': 'conference', '08': 'oncall', '09': 'followsun', '10': 'fax',
}
/** The destination type of unconditional forwarding set by a forward key ("**053" is IVR 3), or null for a phone number. */
export const forwardCodeType = (fwd: string): DestType | null => (fwd.startsWith('**') ? forwardCodes[fwd.slice(2, 4)] ?? null : null)
const forwardValue = (d: Destination) => (d.type === 'external' ? d.target : `${d.type}:${d.target}`)

/**
 * dests: give the call destinations (admins) to let forward keys go to any destination; without
 * them a forward key takes a phone number only.
 */
export function KeyLayout({ vendor, perPage, keys, modules, moduleTypes, lines, exts, onKeys, onModules, canEditModules = true, dests }: {
  vendor: string; perPage?: number; keys: PhoneKey[]; modules: PhoneModule[]; moduleTypes: ModuleType[]; lines: number; exts: Ext[]
  onKeys: (k: PhoneKey[]) => void; onModules: (m: PhoneModule[]) => void; canEditModules?: boolean; dests?: Destinations | null
}) {
  const { t } = useTranslation()
  const typeOf = (id: string) => moduleTypes.find((m) => m.id === id)
  const surfaces: Surface[] = []
  if (keys.length > 0) surfaces.push({ key: 'phone', title: t('keys.phone'), keys, perPage: phonePerPage(vendor, keys.length, perPage) })
  modules.forEach((m, i) => {
    const mt = typeOf(m.type)
    surfaces.push({ key: `m${i}`, title: t('keys.module', { n: i + 1, name: mt?.name ?? m.type }), keys: fillKeys(m.keys, mt?.keys ?? m.keys.length, false), perPage: mt?.perPage ?? 20, module: i })
  })
  const [surface, setSurface] = useState(surfaces[0]?.key ?? 'phone')
  const [page, setPage] = useState(0)
  const [sel, setSel] = useState<{ s: string; i: number } | null>(null)
  const [drag, setDrag] = useState<{ s: string; i: number } | null>(null)
  const [addType, setAddType] = useState(moduleTypes[0]?.id ?? '')
  const cur = surfaces.find((s) => s.key === surface) ?? surfaces[0]

  function update(sk: string, i: number, k: PhoneKey | ((old: PhoneKey) => PhoneKey)) {
    const s = surfaces.find((x) => x.key === sk)
    if (!s) return
    const next = s.keys.map((old, j) => (j === i ? (typeof k === 'function' ? k(old) : k) : old))
    if (s.module === undefined) onKeys(next)
    else onModules(modules.map((m, j) => (j === s.module ? { ...m, keys: next } : m)))
  }
  function swap(a: { s: string; i: number }, b: { s: string; i: number }) {
    if (a.s === b.s && a.i === b.i) return
    const sa = surfaces.find((x) => x.key === a.s)!, sb = surfaces.find((x) => x.key === b.s)!
    const ka = sa.keys[a.i], kb = sb.keys[b.i]
    if (a.s === b.s) {
      const next = sa.keys.map((k, j) => (j === a.i ? kb : j === b.i ? ka : k))
      if (sa.module === undefined) onKeys(next)
      else onModules(modules.map((m, j) => (j === sa.module ? { ...m, keys: next } : m)))
      return
    }
    // Across surfaces: build both lists, then hand them over in one go.
    const listA = sa.keys.map((k, j) => (j === a.i ? kb : k)), listB = sb.keys.map((k, j) => (j === b.i ? ka : k))
    let nextKeys = keys
    const nextMods = modules.map((m) => ({ ...m }))
    for (const [s, list] of [[sa, listA], [sb, listB]] as const) {
      if (s.module === undefined) nextKeys = list
      else nextMods[s.module] = { ...nextMods[s.module], keys: list }
    }
    if (sa.module === undefined || sb.module === undefined) onKeys(nextKeys)
    onModules(nextMods)
    setSel(b)
  }
  function addModule() {
    const mt = typeOf(addType)
    if (!mt) return
    onModules([...modules, { type: mt.id, keys: fillKeys([], mt.keys, false) }])
    setSurface(`m${modules.length}`)
    setPage(0)
  }
  function removeModule(i: number) {
    onModules(modules.filter((_, j) => j !== i))
    setSurface('phone')
    setPage(0)
    setSel(null)
  }

  if (!cur) {
    return canEditModules && moduleTypes.length > 0 ? <AddModule types={moduleTypes} value={addType} onType={setAddType} onAdd={addModule} disabled={false} note="" /> : <p className="text-sm text-subtle">{t('phones.noKeys')}</p>
  }
  const pages = Math.max(1, Math.ceil(cur.keys.length / cur.perPage))
  const pg = Math.min(page, pages - 1)
  const visible = cur.keys.slice(pg * cur.perPage, (pg + 1) * cur.perPage).map((k, j) => ({ k, i: pg * cur.perPage + j }))
  const half = Math.ceil(visible.length / 2)
  const isPhone = cur.module === undefined
  const left = isPhone && visible.length > 4 ? visible.slice(0, half) : visible.filter((_, j) => j % 2 === 0)
  const right = isPhone && visible.length > 4 ? visible.slice(half) : visible.filter((_, j) => j % 2 === 1)
  const selected = sel && sel.s === cur.key ? cur.keys[sel.i] : null
  const mt = cur.module !== undefined ? typeOf(modules[cur.module].type) : undefined
  const sameTypeLeft = modules.length === 0 || modules.every((m) => m.type === addType)
  const maxReached = (typeOf(addType)?.max ?? 0) <= modules.length

  function keyButton(k: PhoneKey, i: number) {
    const Icon = icons[k.type]
    const isSel = sel?.s === cur.key && sel.i === i
    const text = k.label || describe(k)
    return (
      <button key={i} type="button" draggable
        onClick={() => setSel({ s: cur.key, i })}
        onDragStart={(e: DragEvent) => { e.dataTransfer.effectAllowed = 'move'; e.dataTransfer.setData('text/plain', `${cur.key}:${i}`); setDrag({ s: cur.key, i }) }}
        onDragEnd={() => setDrag(null)}
        onDragOver={(e: DragEvent) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move' }}
        onDrop={(e: DragEvent) => { e.preventDefault(); if (drag) swap(drag, { s: cur.key, i }); setDrag(null) }}
        aria-label={t('keys.keyAria', { n: i + 1, what: text })}
        aria-pressed={isSel}
        className={`group flex min-h-11 w-full items-center gap-2 rounded-lg border px-2 text-left text-xs transition ${tone[k.type]} ${isSel ? 'ring-2 ring-accent shadow-[0_0_16px_rgb(34_211_238_/_0.25)]' : 'hover:border-accent/50'} ${drag?.s === cur.key && drag.i === i ? 'opacity-40' : ''}`}>
        <span className="w-5 flex-none text-center font-mono text-[10px] text-subtle">{i + 1}</span>
        <Icon className="size-3.5 flex-none opacity-80" aria-hidden />
        <span className="min-w-0 flex-1 truncate">{k.type === 'none' ? <span className="text-subtle">{t('phones.keyTypes.none')}</span> : text}</span>
        <GripVertical className="size-3.5 flex-none opacity-0 group-hover:opacity-50" aria-hidden />
      </button>
    )
  }
  function describe(k: PhoneKey) {
    switch (k.type) {
      case 'line': return t('keys.lineN', { n: k.value || '1' })
      case 'blf': case 'intercom': { const e = exts.find((x) => x.number === k.value); return e ? `${k.value} ${e.name}` : k.value }
      case 'forward': {
        const d = forwardDest(k.value)
        if (d.type === 'external') return k.value || t('phones.keyTypes.forward')
        return dests?.options.find((o) => o.type === d.type && o.target === d.target)?.label ?? t(`dest.types.${d.type}`)
      }
      default: return k.value || t(`phones.keyTypes.${k.type}`)
    }
  }
  function moduleAdder() {
    const note = !sameTypeLeft ? t('keys.oneType') : maxReached ? t('keys.maxModules', { max: typeOf(addType)?.max }) : ''
    return <AddModule types={moduleTypes} value={addType} onType={setAddType} onAdd={addModule} disabled={!sameTypeLeft || maxReached} note={note} />
  }

  return (
    <div className="flex flex-col gap-3">
      {surfaces.length > 1 && (
        <div role="tablist" className="flex flex-wrap gap-1">
          {surfaces.map((s) => (
            <button key={s.key} type="button" role="tab" aria-selected={s.key === cur.key} onClick={() => { setSurface(s.key); setPage(0); setSel(null) }}
              onDragOver={(e) => { if (drag) { e.preventDefault(); setSurface(s.key) } }}
              className={`min-h-9 rounded-lg border px-3 text-xs transition ${s.key === cur.key ? 'border-accent/50 bg-accent/15 text-ink' : 'border-line text-muted hover:text-ink'}`}>{s.title}</button>
          ))}
        </div>
      )}
      <div className="rounded-[22px] border border-line bg-inset p-3 shadow-inner sm:p-4">
        <div className={`grid items-start gap-2 ${isPhone ? 'grid-cols-1 sm:grid-cols-[1fr_minmax(120px,0.9fr)_1fr]' : 'grid-cols-2'}`}>
          <div className="grid gap-1.5">{left.map(({ k, i }) => keyButton(k, i))}</div>
          {isPhone && (
            <div className="hidden h-full min-h-32 flex-col items-center justify-center rounded-xl border border-line bg-gradient-to-b from-accent/10 to-accent-2/10 p-3 text-center sm:flex" aria-hidden>
              <span className="font-mono text-[11px] text-subtle">{t('keys.screen')}</span>
              <span className="mt-1 text-xs text-muted">{t('keys.page', { n: pg + 1, of: pages })}</span>
            </div>
          )}
          <div className="grid gap-1.5">{right.map(({ k, i }) => keyButton(k, i))}</div>
        </div>
        {pages > 1 && (
          <div className="mt-3 flex justify-center gap-1.5">
            {Array.from({ length: pages }, (_, n) => (
              <button key={n} type="button" onClick={() => setPage(n)} onDragOver={(e) => { if (drag) { e.preventDefault(); setPage(n) } }}
                aria-label={t('keys.page', { n: n + 1, of: pages })} aria-current={n === pg}
                className={`h-2.5 rounded-full transition ${n === pg ? 'w-6 bg-accent' : 'w-2.5 bg-tint/20 hover:bg-tint/40'}`} />
            ))}
          </div>
        )}
      </div>
      <p className="text-xs text-subtle">{t('keys.hint')}</p>
      {selected && sel && (
        <KeyForm k={selected} n={sel.i + 1} lines={lines} exts={exts} dests={dests} onChange={(k) => update(cur.key, sel.i, k)} />
      )}
      {canEditModules && (
        <div className="flex flex-wrap items-end justify-between gap-3 border-t border-line pt-3">
          {moduleTypes.length > 0 ? moduleAdder() : <span className="text-xs text-subtle">{t('keys.noModules')}</span>}
          {cur.module !== undefined && (
            <Button variant="ghost" onClick={() => removeModule(cur.module!)}><Trash2 className="size-4" aria-hidden /> {t('keys.removeModule', { name: mt?.name ?? '' })}</Button>
          )}
        </div>
      )}
    </div>
  )
}

function AddModule({ types, value, onType, onAdd, disabled, note }: { types: ModuleType[]; value: string; onType: (v: string) => void; onAdd: () => void; disabled: boolean; note: string }) {
  const { t } = useTranslation()
  return (
    <div className="flex flex-wrap items-end gap-2">
      <div className="w-60"><Select label={t('keys.addModule')} value={value} onChange={(e) => onType(e.target.value)}>
        {types.map((m) => <option key={m.id} value={m.id}>{m.name} ({t('keys.moduleKeys', { count: m.keys })})</option>)}
      </Select></div>
      <Button variant="secondary" onClick={onAdd} disabled={disabled}><Plus className="size-4" aria-hidden /> {t('keys.add')}</Button>
      {note && <span className="pb-3 text-xs text-subtle">{note}</span>}
    </div>
  )
}

/** The settings of one key. */
function KeyForm({ k, n, lines, exts, dests, onChange }: {
  k: PhoneKey; n: number; lines: number; exts: Ext[]; dests?: Destinations | null; onChange: (k: PhoneKey) => void
}) {
  const { t } = useTranslation()
  const set = (p: Partial<PhoneKey>) => onChange({ ...k, ...p })
  const changeType = (type: KeyType) => {
    let value = ''
    if (type === 'parkslot') value = '71'
    if ((type === 'blf' || type === 'intercom') && (k.type === 'blf' || k.type === 'intercom')) value = k.value
    if ((type === 'speed' || type === 'forward') && (k.type === 'speed' || k.type === 'forward')) value = k.value
    onChange({ type, value, label: type === 'none' ? '' : k.label })
  }
  return (
    <div className="grid gap-3 rounded-xl border border-accent/30 bg-accent/5 p-3 sm:grid-cols-3">
      <Select label={t('keys.keyN', { n })} value={k.type} onChange={(e) => changeType(e.target.value as KeyType)}>
        {keyTypes.map((ty) => <option key={ty} value={ty}>{t(`phones.keyTypes.${ty}`)}</option>)}
      </Select>
      {k.type === 'line' && (
        <Select name="value" label={t('keys.account')} value={k.value || '1'} onChange={(e) => set({ value: e.target.value === '1' ? '' : e.target.value })}>
          {Array.from({ length: Math.max(1, lines) }, (_, i) => <option key={i} value={String(i + 1)}>{t('keys.lineN', { n: i + 1 })}</option>)}
        </Select>
      )}
      {(k.type === 'blf' || k.type === 'intercom') && (
        <Select label={k.type === 'blf' ? t('keys.colleague') : t('keys.intercomTo')} value={k.value}
          onChange={(e) => { const ext = exts.find((q) => q.number === e.target.value); set({ value: e.target.value, label: k.label || ext?.name || '' }) }}>
          <option value="">–</option>
          {exts.map((e) => <option key={e.number} value={e.number}>{e.number} · {e.name}</option>)}
        </Select>
      )}
      {k.type === 'parkslot' && (
        <Select name="value" label={t('keys.slot')} value={k.value} onChange={(e) => set({ value: e.target.value, label: k.label || `Park ${e.target.value}` })}>
          {Array.from({ length: 9 }, (_, i) => <option key={i} value={`7${i + 1}`}>{`7${i + 1}`}</option>)}
        </Select>
      )}
      {k.type === 'speed' && <Input name="value" label={t('keys.number')} value={k.value} onChange={(e) => set({ value: e.target.value })} className="font-mono" inputMode="tel" />}
      {k.type === 'forward' && (dests !== undefined ? (
        <div className="sm:col-span-3">
          <DestinationPicker label={t('keys.forwardTo')} value={forwardDest(k.value)} types={forwardTypes}
            onChange={(d) => {
              const label = d.type === 'external' ? '' : dests?.options.find((o) => o.type === d.type && o.target === d.target)?.label ?? ''
              set({ value: forwardValue(d), label: k.label || label.slice(0, 20) })
            }} />
        </div>
      ) : forwardDest(k.value).type === 'external' ? (
        <Input name="value" label={t('keys.forwardTo')} value={k.value} placeholder="0612345678" onChange={(e) => set({ value: e.target.value })} className="font-mono" inputMode="tel" />
      ) : <p className="self-center text-xs text-subtle">{t('keys.forwardInternal', { type: t(`dest.types.${forwardDest(k.value).type}`) })}</p>)}
      {['none', 'park', 'dnd', 'voicemail', 'forward'].includes(k.type) && <p className="self-center text-xs text-subtle sm:col-span-1">{t(`keys.explain.${k.type}`)}</p>}
      {k.type !== 'none' && <Input name="label" label={t('phones.keyLabel')} value={k.label} maxLength={20} onChange={(e) => set({ label: e.target.value })} />}
    </div>
  )
}
