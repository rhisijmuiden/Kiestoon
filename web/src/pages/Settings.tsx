import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowDown, ArrowUp, Hash, PhoneCall, Power, RotateCw, Siren, Wifi } from 'lucide-react'
import { Badge, Modal, SectionTitle, Select, Table, Textarea, Toggle, td, useApi } from '../components/kit'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { NumberPlan, PbxSettings, PbxStatus, Trunk } from '../lib/types'

type Preview = { changes: { from: string; to: string }[]; conflicts: string[]; plan: NumberPlan }

export function Settings() {
  const { t } = useTranslation()
  const st = useApi<PbxSettings>('/pbx/settings')
  const status = useApi<PbxStatus>('/pbx/status', 10000)
  const trunks = useApi<Trunk[]>('/pbx/trunks')
  const [s, setS] = useState<PbxSettings | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  const [plan, setPlan] = useState(false)
  const [restart, setRestart] = useState(false)

  useEffect(() => { if (st.data && !s) setS(st.data) }, [st.data, s])
  if (!s) return <><PageHeader title={t('settings.title')} /><ErrorMessage error={st.error} /></>
  const set = <K extends keyof PbxSettings>(k: K, v: PbxSettings[K]) => { setSaved(false); setS({ ...s, [k]: v }) }

  async function save() {
    setBusy(true)
    setError(null)
    try {
      setS(await api<PbxSettings>('PUT', '/pbx/settings', s))
      setSaved(true)
      window.setTimeout(status.reload, 2500)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const allTrunks = trunks.data ?? []
  const order = s.emergencyTrunkIds
  const unused = allTrunks.filter((tr) => !order.includes(tr.id))
  const move = (i: number, d: number) => {
    const o = [...order]
    const j = i + d
    if (j < 0 || j >= o.length) return
    ;[o[i], o[j]] = [o[j], o[i]]
    set('emergencyTrunkIds', o)
  }
  const nameOf = (id: number) => allTrunks.find((x) => x.id === id)?.name ?? `#${id}`

  return (
    <>
      <PageHeader title={t('settings.title')} subtitle={t('settings.subtitle')}>
        {status.data && (status.data.connected
          ? <Badge tone="ok" dot>Asterisk {status.data.version}</Badge>
          : <Badge tone="danger" dot>{t('settings.asteriskDown')}</Badge>)}
        <Button variant="ghost" onClick={() => setRestart(true)}><Power className="size-4" />{t('settings.restart')}</Button>
      </PageHeader>
      {status.data?.restartNeeded && (
        <Alert tone="warn">
          {t('settings.restartNeeded')}{' '}
          <button type="button" className="underline" onClick={() => setRestart(true)}>{t('settings.restart')}</button>
        </Alert>
      )}
      {status.data?.lastError && <Alert>{t('settings.applyError', { error: status.data.lastError })}</Alert>}

      <div className="grid gap-4 xl:grid-cols-2">
        <Card>
          <SectionTitle action={<Button variant="secondary" onClick={() => setPlan(true)}><Hash className="size-4" />{t('settings.changePlan')}</Button>}>
            {t('settings.numberPlan')}
          </SectionTitle>
          <PlanTable digits={s.digits} />
        </Card>

        <Card>
          <SectionTitle><span className="inline-flex items-center gap-2"><Siren className="size-5 text-danger" />{t('settings.emergency')}</span></SectionTitle>
          <div className="flex flex-col gap-3">
            <Alert tone="ok">{t('settings.emergencyAlways')}</Alert>
            <Input label={t('settings.emergencyCid')} value={s.emergencyCallerId} onChange={(e) => set('emergencyCallerId', e.target.value)} placeholder="+31201234567" hint={t('settings.emergencyCidHint')} />
            <div>
              <div className="mb-1.5 text-sm font-medium text-ink-2">{t('settings.emergencyTrunks')}</div>
              <p className="mb-2 text-xs text-subtle">{t('settings.emergencyTrunksHint')}</p>
              <ol className="flex flex-col gap-1.5">
                {order.map((id, i) => (
                  <li key={id} className="flex items-center gap-2 rounded-xl border border-line bg-surface-2/60 px-3 py-1.5">
                    <span className="w-5 font-mono text-xs text-subtle">{i + 1}</span>
                    <span className="flex-1 text-sm text-ink">{nameOf(id)}</span>
                    <button type="button" aria-label={t('settings.up')} onClick={() => move(i, -1)} className="inline-flex size-9 items-center justify-center rounded-lg text-muted hover:text-ink"><ArrowUp className="size-4" /></button>
                    <button type="button" aria-label={t('settings.down')} onClick={() => move(i, 1)} className="inline-flex size-9 items-center justify-center rounded-lg text-muted hover:text-ink"><ArrowDown className="size-4" /></button>
                    <button type="button" onClick={() => set('emergencyTrunkIds', order.filter((x) => x !== id))} className="rounded-lg px-2 py-1 text-xs text-muted hover:text-danger">{t('common.delete')}</button>
                  </li>
                ))}
              </ol>
              {unused.length > 0 && (
                <div className="mt-2 flex flex-wrap gap-1.5">
                  {unused.map((tr) => (
                    <button key={tr.id} type="button" onClick={() => set('emergencyTrunkIds', [...order, tr.id])}
                      className="min-h-9 rounded-full border border-dashed border-line px-3 text-xs text-muted hover:border-accent/50 hover:text-ink">+ {tr.name}</button>
                  ))}
                </div>
              )}
              {order.length === 0 && <p className="mt-2 text-xs text-muted">{t('settings.emergencyAll')}</p>}
            </div>
          </div>
        </Card>

        <Card>
          <SectionTitle><span className="inline-flex items-center gap-2"><Wifi className="size-5 text-accent" />{t('settings.network')}</span></SectionTitle>
          <div className="flex flex-col gap-3">
            <Input label={t('settings.externalAddress')} value={s.externalAddress} onChange={(e) => set('externalAddress', e.target.value)} placeholder="203.0.113.10" hint={t('settings.externalAddressHint')} />
            <Textarea label={t('settings.localNets')} value={s.localNets.join('\n')} onChange={(e) => set('localNets', e.target.value.split(/[\s,]+/).filter(Boolean))} hint={t('settings.localNetsHint')} />
            <div className="grid gap-3 sm:grid-cols-2">
              <Input label={t('settings.sipPort')} type="number" value={s.sipPort} onChange={(e) => set('sipPort', Number(e.target.value))} />
              <Input label={t('settings.tlsPort')} type="number" value={s.tlsPort} onChange={(e) => set('tlsPort', Number(e.target.value))} />
              <Input label={t('settings.rtpStart')} type="number" value={s.rtpStart} onChange={(e) => set('rtpStart', Number(e.target.value))} />
              <Input label={t('settings.rtpEnd')} type="number" value={s.rtpEnd} onChange={(e) => set('rtpEnd', Number(e.target.value))} />
            </div>
            <p className="text-xs text-subtle">{t('settings.portsHint')}</p>
          </div>
        </Card>

        <Card>
          <SectionTitle><span className="inline-flex items-center gap-2"><PhoneCall className="size-5 text-accent" />{t('settings.calls')}</span></SectionTitle>
          <div className="flex flex-col gap-3">
            <div className="grid gap-3 sm:grid-cols-2">
              <Select label={t('settings.language')} value={s.language} onChange={(e) => set('language', e.target.value as PbxSettings['language'])} hint={t('settings.languageHint')}>
                <option value="en">English</option>
                <option value="nl">Nederlands</option>
              </Select>
              <Input label={t('settings.timezone')} value={s.timezone} onChange={(e) => set('timezone', e.target.value)} placeholder="Europe/Amsterdam" />
              <Input label={t('settings.countryCode')} value={s.countryCode} onChange={(e) => set('countryCode', e.target.value.replace(/\D/g, ''))} hint={t('settings.countryCodeHint')} />
              <Input label={t('settings.codecs')} value={s.codecs} onChange={(e) => set('codecs', e.target.value)} hint={t('settings.codecsHint')} />
            </div>
            <Toggle label={t('settings.nationalCid')} hint={t('settings.nationalCidHint')} checked={s.nationalCallerId} onChange={(v) => set('nationalCallerId', v)} />
          </div>
        </Card>
      </div>

      <ErrorMessage error={error} />
      <div className="flex flex-wrap items-center justify-end gap-3">
        {saved && <span className="text-sm text-ok">{t('common.saved')}</span>}
        <Button busy={busy} onClick={save}>{t('common.save')}</Button>
      </div>

      {plan && <PlanWizard current={s.digits} onClose={() => setPlan(false)} onApplied={(d) => { setPlan(false); setS({ ...s, digits: d }); status.reload() }} />}
      {restart && <RestartModal onClose={() => { setRestart(false); window.setTimeout(status.reload, 3000) }} />}
    </>
  )
}

function PlanTable({ digits, plan }: { digits: number; plan?: NumberPlan }) {
  const { t } = useTranslation()
  const np = useApi<{ current: NumberPlan; options: Record<string, NumberPlan> }>(plan ? null : '/pbx/numberplan')
  const p = plan ?? np.data?.options[String(digits)]
  return (
    <>
      <p className="mb-3 text-sm text-ink-2">{t('settings.digitsNow', { digits })}</p>
      <Table head={[t('settings.purpose'), t('settings.range')]}>
        {p?.ranges.map((r) => (
          <tr key={r.purpose}>
            <td className={td}>{t(`settings.purposes.${r.purpose}`)}</td>
            <td className={`${td} font-mono text-ink`}>{r.from} – {r.to}</td>
          </tr>
        ))}
        <tr>
          <td className={td}>{t('settings.purposes.emergency')}</td>
          <td className={`${td} font-mono text-danger`}>112, 911</td>
        </tr>
        <tr>
          <td className={td}>{t('settings.purposes.features')}</td>
          <td className={`${td} font-mono text-ink`}>*97 · *98 · *43</td>
        </tr>
      </Table>
    </>
  )
}

function PlanWizard({ current, onClose, onApplied }: { current: number; onClose: () => void; onApplied: (d: number) => void }) {
  const { t } = useTranslation()
  const [digits, setDigits] = useState(current === 3 ? 4 : 3)
  const [preview, setPreview] = useState<Preview | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)

  useEffect(() => {
    let live = true
    setPreview(null)
    api<Preview>('POST', '/pbx/numberplan/preview', { digits }).then((p) => live && setPreview(p)).catch((e) => live && setError(e))
    return () => { live = false }
  }, [digits])

  async function apply() {
    setBusy(true)
    setError(null)
    try {
      await api('POST', '/pbx/numberplan/apply', { digits })
      onApplied(digits)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const blocked = digits === current || (preview?.conflicts.length ?? 0) > 0
  return (
    <Modal wide title={t('settings.changePlan')} subtitle={t('settings.planWizardHint')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} disabled={blocked || !preview} onClick={apply}>{t('settings.applyPlan', { digits })}</Button></>}>
      <div className="grid grid-cols-3 gap-2">
        {[3, 4, 5].map((d) => (
          <button key={d} type="button" onClick={() => setDigits(d)}
            className={`rounded-xl border p-4 text-left transition ${digits === d ? 'border-accent/60 bg-accent/10 shadow-[0_0_30px_-12px_rgb(34_211_238_/_0.7)]' : 'border-line bg-surface-2/60 hover:border-accent/30'}`}>
            <div className="font-mono text-2xl text-ink">{d}</div>
            <div className="text-xs text-muted">{t('settings.digitsLabel')}{d === current ? ` · ${t('settings.currentPlan')}` : ''}</div>
            <div className="mt-1 font-mono text-xs text-subtle">{'2' + '0'.repeat(d - 1)} – {'5' + '9'.repeat(d - 1)}</div>
          </button>
        ))}
      </div>
      {preview && (
        <>
          {preview.conflicts.length > 0 && <Alert>{t('settings.planConflicts', { numbers: preview.conflicts.join(', ') })}</Alert>}
          {preview.changes.length > 0 ? (
            <div>
              <div className="mb-2 text-sm font-medium text-ink-2">{t('settings.planChanges', { count: preview.changes.length })}</div>
              <div className="max-h-64 overflow-y-auto rounded-xl border border-line p-3">
                <div className="grid grid-cols-2 gap-x-6 gap-y-1 font-mono text-sm sm:grid-cols-3">
                  {preview.changes.map((c) => <div key={c.from}><span className="text-muted">{c.from}</span> → <span className="text-accent">{c.to}</span></div>)}
                </div>
              </div>
              <p className="mt-2 text-xs text-subtle">{t('settings.planPhones')}</p>
            </div>
          ) : digits !== current && <p className="text-sm text-muted">{t('settings.planNoChanges')}</p>}
          <PlanTable digits={digits} plan={preview.plan} />
        </>
      )}
      <ErrorMessage error={error} />
    </Modal>
  )
}

function RestartModal({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation()
  const [busy, setBusy] = useState('')
  const [error, setError] = useState<unknown>(null)
  const [done, setDone] = useState(false)
  async function go(now: boolean) {
    setBusy(now ? 'now' : 'idle')
    setError(null)
    try {
      await api('POST', '/pbx/restart', { now })
      setDone(true)
    } catch (e) {
      setError(e)
    } finally {
      setBusy('')
    }
  }
  return (
    <Modal title={t('settings.restart')} onClose={onClose}>
      {done ? <Alert tone="ok">{t('settings.restartSent')}</Alert> : <p className="text-sm text-ink-2">{t('settings.restartText')}</p>}
      <ErrorMessage error={error} />
      {!done && (
        <div className="flex flex-wrap justify-end gap-2.5">
          <Button variant="secondary" busy={busy === 'idle'} onClick={() => go(false)}><RotateCw className="size-4" />{t('settings.restartIdle')}</Button>
          <Button variant="danger" busy={busy === 'now'} onClick={() => go(true)}><Power className="size-4" />{t('settings.restartNow')}</Button>
        </div>
      )}
    </Modal>
  )
}
