import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ShieldAlert, ShieldCheck, Unlock } from 'lucide-react'
import { Badge, SectionTitle, Table, Textarea, Toggle, td, useApi } from '../components/kit'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { Ban, FirewallConfig, FirewallStatus } from '../lib/types'

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
      <div className="flex flex-wrap items-center justify-end gap-3">
        <span className="flex items-center gap-1.5 text-xs text-subtle"><ShieldAlert className="size-4" />{t('firewall.safeMode')}</span>
        <Button busy={busy} disabled={!fw.data.helperAvailable} onClick={save}>{t('firewall.apply')}</Button>
      </div>
      {st.ruleset && (
        <Card>
          <details>
            <summary className="cursor-pointer text-sm text-ink-2">{t('firewall.showRules')}</summary>
            <pre className="mt-3 max-h-96 overflow-auto rounded-xl border border-line bg-[#0A1224] p-4 font-mono text-xs text-ink-2">{st.ruleset}</pre>
          </details>
        </Card>
      )}
    </>
  )
}
