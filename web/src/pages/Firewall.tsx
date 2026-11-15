import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Globe2, KeyRound, Plus, RefreshCw, ShieldAlert, ShieldCheck, Trash2, Unlock, X } from 'lucide-react'
import { Badge, SectionTitle, Table, Textarea, Toggle, formatDate, td, useApi } from '../components/kit'
import { Segmented } from '../components/pbx'
import { countryCodes, countryName, europe } from '../lib/countries'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api, roles as allRoles, type Role } from '../lib/api'
import type { Ban, FirewallConfig, FirewallStatus, GeoStatus, TrustedNetworks, TrustedNetworksData } from '../lib/types'

type FwData = { config: FirewallConfig; status: FirewallStatus; helperAvailable: boolean; trunkIps: string[]; clientIp: string; ports: { Portal: number[]; SIP: number; SIPTLS: number; RTPStart: number; RTPEnd: number } }

const lines = (a: string[]) => a.join('\n')
const parse = (s: string) => s.split(/[\s,]+/).map((x) => x.trim()).filter(Boolean)

export function Firewall() {
  const { t } = useTranslation()
  const fw = useApi<FwData>('/firewall')
  const bans = useApi<Ban[]>('/firewall/bans', 15000)
  const [c, setC] = useState<FirewallConfig | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [pending, setPending] = useState(0)

  useEffect(() => { if (fw.data && !c) setC(fw.data.config) }, [fw.data, c])
  // Safe mode: if this page can still reach the server after the change, confirm it automatically.
  useEffect(() => {
    if (pending <= 0) return
    const id = window.setTimeout(async () => {
      try {
        await api('POST', '/firewall/confirm')
        setPending(0)
        fw.reload()
      } catch {
        setPending((p) => p - 1)
      }
    }, 3000)
    return () => window.clearTimeout(id)
  }, [pending, fw])

  if (!c || !fw.data) return <><PageHeader title={t('firewall.title')} /><ErrorMessage error={fw.error} /></>
  const set = <K extends keyof FirewallConfig>(k: K, v: FirewallConfig[K]) => setC({ ...c, [k]: v })

  async function save() {
    setBusy(true)
    setError(null)
    try {
      const r = await api<{ status: FirewallStatus }>('PUT', '/firewall', c)
      if (r.status.pending) setPending(20)
      fw.reload()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  async function unban(ip: string) {
    await api('POST', '/firewall/unban', { ip })
    bans.reload()
  }
  const adminLocksMeOut = c.enabled && c.adminNetworks.length > 0 && !c.adminNetworks.some((n) => n === fw.data!.clientIp || n.includes('/'))
  const st = fw.data.status

  return (
    <>
      <PageHeader title={t('firewall.title')} subtitle={t('firewall.subtitle')}>
        {st.active ? <Badge tone="ok" dot>{t('firewall.active')}</Badge> : <Badge tone="warn" dot>{t('firewall.inactive')}</Badge>}
      </PageHeader>
      {!fw.data.helperAvailable && <Alert tone="warn">{t('firewall.noHelper')}</Alert>}
      {pending > 0 && <Alert tone="warn">{t('firewall.pending')}</Alert>}
      {st.rolledBack && <Alert>{t('firewall.rolledBack')}</Alert>}
      <ErrorMessage error={error} />
      <div className="grid gap-4 xl:grid-cols-2">
        <Card>
          <SectionTitle>{t('firewall.rules')}</SectionTitle>
          <div className="flex flex-col gap-3">
            <Toggle label={t('firewall.enable')} hint={t('firewall.enableHint')} checked={c.enabled} onChange={(v) => set('enabled', v)} />
            <Toggle label={t('firewall.sipOpen')} hint={t('firewall.sipOpenHint')} checked={c.sipOpen} onChange={(v) => set('sipOpen', v)} />
            {!c.sipOpen && (
              <>
                <Toggle label={t('firewall.privateSip')} hint={t('firewall.privateSipHint')} checked={c.allowPrivateSip} onChange={(v) => set('allowPrivateSip', v)} />
                <Textarea label={t('firewall.whitelist')} hint={t('firewall.whitelistHint')} value={lines(c.sipWhitelist)} onChange={(e) => set('sipWhitelist', parse(e.target.value))} />
                <div className="text-xs text-subtle">
                  {t('firewall.trunkIps')}: <span className="font-mono text-ink-2">{fw.data.trunkIps.join(', ') || '–'}</span>
                </div>
              </>
            )}
            <Textarea label={t('firewall.admin')} hint={t('firewall.adminHint', { ip: fw.data.clientIp })} value={lines(c.adminNetworks)} onChange={(e) => set('adminNetworks', parse(e.target.value))} />
            {adminLocksMeOut && <Alert tone="warn">{t('firewall.lockout', { ip: fw.data.clientIp })}</Alert>}
          </div>
        </Card>
        <Card>
          <SectionTitle>fail2ban</SectionTitle>
          <div className="flex flex-col gap-3">
            <Toggle label={t('firewall.f2b')} hint={t('firewall.f2bHint')} checked={c.fail2banEnabled} onChange={(v) => set('fail2banEnabled', v)} />
            <div className="grid gap-3 sm:grid-cols-3">
              <Input label={t('firewall.maxRetry')} type="number" min={1} value={c.maxRetry} onChange={(e) => set('maxRetry', Number(e.target.value))} />
              <Input label={t('firewall.findMinutes')} type="number" min={1} value={c.findMinutes} onChange={(e) => set('findMinutes', Number(e.target.value))} />
              <Input label={t('firewall.banMinutes')} type="number" min={1} value={c.banMinutes} onChange={(e) => set('banMinutes', Number(e.target.value))} />
            </div>
            <Textarea label={t('firewall.ignore')} hint={t('firewall.ignoreHint')} value={lines(c.ignoreIps)} onChange={(e) => set('ignoreIps', parse(e.target.value))} />
          </div>
          <h3 className="mb-2 mt-6 text-sm font-medium text-ink-2">{t('firewall.bans')}</h3>
          <Table head={[t('firewall.ip'), t('firewall.jail'), '']}
            empty={(bans.data?.length ?? 0) === 0 && <p className="py-6 text-center text-sm text-muted"><ShieldCheck className="mr-1 inline size-4 text-ok" />{t('firewall.noBans')}</p>}>
            {bans.data?.map((b) => (
              <tr key={b.jail + b.ip}>
                <td className={`${td} font-mono text-ink`}>{b.ip}</td>
                <td className={td}>{b.jail}</td>
                <td className={`${td} text-right`}><Button variant="ghost" onClick={() => unban(b.ip)}><Unlock className="size-4" />{t('firewall.unban')}</Button></td>
              </tr>
            ))}
          </Table>
        </Card>
      </div>
      <GeoCard c={c} set={set} />
      <div className="flex flex-wrap items-center justify-end gap-3">
        <span className="flex items-center gap-1.5 text-xs text-subtle"><ShieldAlert className="size-4" />{t('firewall.safeMode')}</span>
        <Button busy={busy} disabled={!fw.data.helperAvailable} onClick={save}>{t('firewall.apply')}</Button>
      </div>
      <TrustedCard />
      {st.ruleset && (
        <Card>
          <details>
            <summary className="cursor-pointer text-sm text-ink-2">{t('firewall.showRules')}</summary>
            <pre className="mt-3 max-h-96 overflow-auto rounded-xl border border-line bg-field p-4 font-mono text-xs text-ink-2">{st.ruleset}</pre>
          </details>
        </Card>
      )}
    </>
  )
}

function GeoCard({ c, set }: { c: FirewallConfig; set: <K extends keyof FirewallConfig>(k: K, v: FirewallConfig[K]) => void }) {
  const { t, i18n } = useTranslation()
  const geo = useApi<GeoStatus>('/firewall/geo')
  const [pick, setPick] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const mode = c.geoMode || 'off'
  const list = c.geoCountries ?? []
  const add = (codes: string[]) => set('geoCountries', [...new Set([...list, ...codes])].sort())
  const sorted = [...countryCodes].sort((a, b) => countryName(a, i18n.language).localeCompare(countryName(b, i18n.language), i18n.language))
  async function update() {
    setBusy(true)
    setError(null)
    try {
      geo.setData(await api<GeoStatus>('POST', '/firewall/geo/update'))
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Card>
      <SectionTitle action={geo.data?.available
        ? <Badge tone="ok" dot>{t('firewall.geo.dataFrom', { date: formatDate(geo.data.updatedAt ?? '', i18n.language) })}</Badge>
        : <Badge tone="warn" dot>{t('firewall.geo.noData')}</Badge>}>
        <span className="inline-flex items-center gap-2"><Globe2 className="size-5 text-accent" aria-hidden />{t('firewall.geo.title')}</span>
      </SectionTitle>
      <p className="mb-4 max-w-3xl text-sm text-muted">{t('firewall.geo.intro')}</p>
      <ErrorMessage error={error} />
      <div className="flex flex-col gap-4">
        <Segmented label={t('firewall.geo.mode')} value={mode} onChange={(v) => set('geoMode', v)} options={[
          { key: 'off', label: t('firewall.geo.off') },
          { key: 'allow', label: t('firewall.geo.allow'), tone: 'ok' },
          { key: 'block', label: t('firewall.geo.block'), tone: 'danger' },
        ]} />
        {mode !== 'off' && (
          <>
            <div className="flex flex-col gap-2">
              <span className="text-sm font-medium text-ink-2">{mode === 'allow' ? t('firewall.geo.allowList') : t('firewall.geo.blockList')}</span>
              <div className="flex flex-wrap gap-1.5">
                {list.length === 0 && <span className="text-sm text-subtle">{t('firewall.geo.none')}</span>}
                {list.map((code) => (
                  <span key={code} className="inline-flex items-center gap-1 rounded-full border border-line bg-tint/5 py-0.5 pl-2.5 pr-1 text-xs text-ink-2">
                    <span className="font-mono text-subtle">{code}</span> {countryName(code, i18n.language)}
                    <button type="button" aria-label={t('common.remove')} onClick={() => set('geoCountries', list.filter((x) => x !== code))} className="inline-flex size-6 items-center justify-center rounded-full hover:bg-tint/10"><X className="size-3" aria-hidden /></button>
                  </span>
                ))}
              </div>
              <div className="flex flex-wrap items-end gap-2">
                <div className="min-w-60 flex-1">
                  <label className="sr-only" htmlFor="geo-pick">{t('firewall.geo.addCountry')}</label>
                  <select id="geo-pick" value={pick} onChange={(e) => { if (e.target.value) add([e.target.value]); setPick('') }}
                    className="min-h-11 w-full rounded-[10px] border border-line bg-field px-3.5 text-[15px] text-ink">
                    <option value="">{t('firewall.geo.addCountry')}</option>
                    {sorted.filter((x) => !list.includes(x)).map((x) => <option key={x} value={x}>{countryName(x, i18n.language)} ({x})</option>)}
                  </select>
                </div>
                <Button variant="ghost" onClick={() => add(['NL', 'BE'])}>{t('firewall.geo.presetBenelux')}</Button>
                <Button variant="ghost" onClick={() => add(europe)}>{t('firewall.geo.presetEurope')}</Button>
              </div>
            </div>
            {mode === 'allow' && <Alert tone="warn">{t('firewall.geo.allowWarn')}</Alert>}
            <Toggle label={t('firewall.geo.portal')} hint={t('firewall.geo.portalHint')} checked={c.geoPortal} onChange={(v) => set('geoPortal', v)} />
            <p className="text-xs text-subtle">{t('firewall.geo.exceptions')}</p>
          </>
        )}
        <div className="flex flex-wrap items-center justify-between gap-3">
          <span className="text-xs text-subtle">{geo.data?.attribution}</span>
          <Button variant="ghost" busy={busy} onClick={update}><RefreshCw className="size-4" aria-hidden />{t('firewall.geo.update')}</Button>
        </div>
      </div>
    </Card>
  )
}

// Trusted networks: password sign-ins from these ranges skip the MFA step. Saved on its own, it does
// not touch the nftables rules.
function TrustedCard() {
  const { t } = useTranslation()
  const tn = useApi<TrustedNetworksData>('/firewall/trusted')
  const [c, setC] = useState<TrustedNetworks | null>(null)
  const [busy, setBusy] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState<unknown>(null)
  useEffect(() => { if (tn.data && !c) setC(tn.data.settings) }, [tn.data, c])
  if (!c || !tn.data) return null
  const d = tn.data
  const set = (v: Partial<TrustedNetworks>) => { setSaved(false); setC({ ...c, ...v }) }
  const setNet = (i: number, k: 'cidr' | 'description', v: string) => set({ networks: c.networks.map((n, j) => (j === i ? { ...n, [k]: v } : n)) })
  const toggleRole = (r: Role, on: boolean) => set({ roles: on ? [...new Set([...c.roles, r])] : c.roles.filter((x) => x !== r) })
  const broad = c.networks.some((n) => /\/([0-7])$/.test(n.cidr.trim()) || n.cidr.trim() === '::/0')
  async function save() {
    setBusy(true)
    setError(null)
    try {
      const r = await api<TrustedNetworksData>('PUT', '/firewall/trusted', c)
      tn.setData(r)
      setC(r.settings)
      setSaved(true)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Card>
      <SectionTitle action={c.enabled ? <Badge tone="warn" dot>{t('firewall.trusted.on')}</Badge> : <Badge dot>{t('firewall.trusted.off')}</Badge>}>
        <span className="inline-flex items-center gap-2"><KeyRound className="size-5 text-accent" aria-hidden />{t('firewall.trusted.title')}</span>
      </SectionTitle>
      <p className="mb-4 max-w-3xl text-sm text-muted">{t('firewall.trusted.intro')}</p>
      <ErrorMessage error={error} />
      <div className="flex flex-col gap-4">
        <Toggle label={t('firewall.trusted.enable')} hint={t('firewall.trusted.enableHint')} checked={c.enabled} onChange={(v) => set({ enabled: v })} />
        <div className="flex flex-col gap-2">
          <span className="text-sm font-medium text-ink-2">{t('firewall.trusted.networks')}</span>
          {c.networks.length === 0 && <p className="text-sm text-subtle">{t('firewall.trusted.none')}</p>}
          {c.networks.map((n, i) => (
            <div key={i} className="grid items-end gap-2 sm:grid-cols-[minmax(0,14rem)_minmax(0,1fr)_auto]">
              <Input label={t('firewall.trusted.cidr')} className="font-mono" placeholder="192.168.1.0/24" value={n.cidr} onChange={(e) => setNet(i, 'cidr', e.target.value)} />
              <Input label={t('firewall.trusted.description')} placeholder={t('firewall.trusted.descriptionPlaceholder')} maxLength={80} value={n.description} onChange={(e) => setNet(i, 'description', e.target.value)} />
              <Button variant="ghost" aria-label={t('common.remove')} onClick={() => set({ networks: c.networks.filter((_, j) => j !== i) })}><Trash2 className="size-4" aria-hidden /></Button>
            </div>
          ))}
          <div className="flex flex-wrap gap-2">
            <Button variant="ghost" onClick={() => set({ networks: [...c.networks, { cidr: '', description: '' }] })}><Plus className="size-4" aria-hidden />{t('firewall.trusted.add')}</Button>
            {!d.clientNetwork && !c.networks.some((n) => n.cidr === d.clientIp || n.cidr === `${d.clientIp}/32` || n.cidr === `${d.clientIp}/128`) && (
              <Button variant="ghost" onClick={() => set({ networks: [...c.networks, { cidr: d.clientIp, description: '' }] })}><Plus className="size-4" aria-hidden />{t('firewall.trusted.addMine', { ip: d.clientIp })}</Button>
            )}
          </div>
          <p className="text-xs text-subtle">{d.clientNetwork ? t('firewall.trusted.youInside', { ip: d.clientIp, net: d.clientNetwork }) : t('firewall.trusted.youOutside', { ip: d.clientIp })}</p>
        </div>
        {broad && <Alert>{t('firewall.trusted.tooBroad')}</Alert>}
        {d.public.length > 0 && <Alert tone="warn">{t('firewall.trusted.publicWarn', { nets: d.public.join(', ') })}</Alert>}
        <div className="flex flex-col gap-2">
          <span className="text-sm font-medium text-ink-2">{t('firewall.trusted.roles')}</span>
          <div className="grid gap-2 sm:grid-cols-2">
            {allRoles.map((r) => (
              <Toggle key={r} label={t(`roles.${r}`)} hint={r === 'superadmin' ? t('firewall.trusted.superadminHint') : undefined} checked={c.roles.includes(r)} onChange={(v) => toggleRole(r, v)} />
            ))}
          </div>
        </div>
        <p className="text-xs text-subtle">{t('firewall.trusted.notes')}</p>
        <div className="flex flex-wrap items-center justify-end gap-3">
          {saved && <span className="text-sm text-ok">{t('firewall.trusted.saved')}</span>}
          <Button busy={busy} onClick={save}>{t('common.save')}</Button>
        </div>
      </div>
    </Card>
  )
}
