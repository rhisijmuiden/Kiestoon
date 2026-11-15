import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowLeftRight, Power, Server, ShieldCheck } from 'lucide-react'
import { Badge, CopyField, Modal, SectionTitle, Select, Stat, formatBytes, formatDate, useApi } from '../components/kit'
import { Segmented } from '../components/pbx'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { ClusterView } from '../lib/types'

/** High availability: an active/passive pair with a virtual IP, replication and switch-over. */
export function Cluster() {
  const { t, i18n } = useTranslation()
  const view = useApi<ClusterView>('/cluster', 5000)
  const [code, setCode] = useState('')
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState(false)
  const v = view.data
  const n = v?.node

  async function switchover() {
    if (!window.confirm(t('cluster.confirmSwitchover'))) return
    setBusy(true)
    setError(null)
    try {
      await api('POST', '/cluster/switchover', {})
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
      view.reload()
    }
  }
  async function disable() {
    if (!window.confirm(t('cluster.confirmDisable'))) return
    await api('POST', '/cluster/disable', {}).catch(setError)
    view.reload()
  }
  async function newCode() {
    if (!n) return
    try {
      const r = await api<{ code: string }>('POST', '/cluster/enable', { mode: n.mode, vip: n.vip, address: n.address, interface: n.interface })
      setCode(r.code)
      view.reload()
    } catch (e) {
      setError(e)
    }
  }

  return (
    <>
      <PageHeader title={t('cluster.title')} subtitle={t('cluster.subtitle')}>
        {n?.enabled && n.peer && (
          <Button busy={busy || v?.busy} onClick={switchover}><ArrowLeftRight className="size-4" aria-hidden /> {t('cluster.switchover')}</Button>
        )}
        {n?.enabled && <Button variant="ghost" onClick={disable}><Power className="size-4" aria-hidden /> {t('cluster.disable')}</Button>}
      </PageHeader>
      <ErrorMessage error={view.error ?? error} />
      {v?.lastError && <Alert>{t(`errors.${v.lastError}`, { defaultValue: v.lastError })}</Alert>}
      {v?.busy && <Alert tone="warn">{t('cluster.switching')}</Alert>}
      {v && !v.helperAvailable && <Alert tone="warn">{t('sites.helperMissing')}</Alert>}
      {v && !n?.enabled && <EnableCard view={v} onEnabled={(c) => { setCode(c); view.reload() }} />}
      {v && n?.enabled && (
        <>
          <div className="grid gap-4 md:grid-cols-3">
            <Stat label={t('cluster.thisNode')} value={t(`cluster.roles.${v.role.role}`)} tone="ok" sub={`${n.name} · ${n.address}`} />
            <Stat label={t('cluster.standby')} value={!n.peer ? t('cluster.noStandby') : v.peerState?.online ? t('sites.connected') : t('sites.offline')}
              tone={!n.peer ? 'warn' : v.peerState?.online ? 'ok' : 'danger'} sub={n.peer ? `${n.peer.name} · ${n.peer.address}` : undefined} />
            <Stat label={t('cluster.replication')}
              value={!v.peerState ? '–' : v.peerState.inSync ? t('cluster.inSync') : t('cluster.behind', { seconds: Math.round(v.peerState.lagSeconds) })}
              tone={!v.peerState ? undefined : v.peerState.inSync ? 'ok' : 'warn'}
              sub={v.snapshot.seq ? t('cluster.snapshot', { seq: v.snapshot.seq, size: formatBytes(v.snapshot.size) }) : undefined} />
          </div>
          <Card>
            <SectionTitle>{t('cluster.details')}</SectionTitle>
            <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-[200px_1fr]">
              <dt className="text-muted">{t('cluster.mode')}</dt><dd>{t(`cluster.modes.${n.mode}`)}</dd>
              {n.mode === 'vip' && <><dt className="text-muted">{t('cluster.vip')}</dt><dd className="font-mono">{n.vip} {v.status.vipHere && <Badge tone="ok">{t('cluster.vipHere')}</Badge>}</dd></>}
              {n.mode === 'vip' && <><dt className="text-muted">keepalived</dt><dd>{v.status.keepalivedActive ? <Badge tone="ok" dot>{v.status.vrrpState || 'active'}</Badge> : <Badge tone="danger">{v.status.keepalivedInstalled ? t('cluster.stopped') : t('cluster.notInstalled')}</Badge>}</dd></>}
              <dt className="text-muted">{t('sites.tunnel')}</dt><dd className="font-mono">{n.tunnel}{n.peer && ` ↔ ${n.peer.tunnel}`}</dd>
              <dt className="text-muted">{t('cluster.epoch')}</dt><dd className="font-mono">{v.role.epoch}</dd>
              {v.peerState && <><dt className="text-muted">{t('sites.lastSeen')}</dt><dd>{formatDate(v.peerState.lastSeen, i18n.language)} · {v.peerState.version}</dd></>}
              {v.peerState?.lastFileSync && <><dt className="text-muted">{t('cluster.files')}</dt><dd>{t('cluster.filesSynced', { count: v.peerState.files })} · {formatDate(v.peerState.lastFileSync, i18n.language)}</dd></>}
            </dl>
            {!n.peer && (
              <div className="mt-4 flex flex-wrap items-center gap-3">
                <p className="text-sm text-muted">{n.joinOpen ? t('cluster.waitingJoin') : t('cluster.joinExpired')}</p>
                <Button variant="ghost" onClick={newCode}>{t('cluster.newCode')}</Button>
              </div>
            )}
          </Card>
          <Card>
            <SectionTitle>{t('cluster.whatTitle')}</SectionTitle>
            <ul className="list-disc space-y-1.5 pl-5 text-sm text-ink-2">
              <li>{t('cluster.what1')}</li>
              <li>{t('cluster.what2')}</li>
              <li>{t('cluster.what3')}</li>
            </ul>
          </Card>
        </>
      )}
      {code && (
        <Modal title={t('cluster.codeTitle')} onClose={() => setCode('')} footer={<Button onClick={() => setCode('')}>{t('common.close')}</Button>}>
          <ol className="list-decimal space-y-1.5 pl-5 text-sm text-ink-2">
            <li>{t('cluster.code1')}</li>
            <li>{t('cluster.code2')}</li>
            <li>{t('cluster.code3')}</li>
          </ol>
          <CopyField label={t('sites.pairingCode')} value={code} />
          <Alert tone="warn">{t('sites.codeValid')}</Alert>
        </Modal>
      )}
    </>
  )
}

function EnableCard({ view, onEnabled }: { view: ClusterView; onEnabled: (code: string) => void }) {
  const { t } = useTranslation()
  const [f, setF] = useState({ mode: 'vip' as 'vip' | 'manual', vip: '', address: view.defaultAddress, interface: '' })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  async function enable() {
    setBusy(true)
    setError(null)
    try {
      const r = await api<{ code: string }>('POST', '/cluster/enable', f)
      onEnabled(r.code)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Card>
      <div className="mb-4 flex items-start gap-3">
        <span className="grid size-10 flex-none place-items-center rounded-xl bg-accent/15 text-accent"><ShieldCheck className="size-5" aria-hidden /></span>
        <div>
          <h2 className="font-display text-lg font-semibold">{t('cluster.enableTitle')}</h2>
          <p className="text-sm text-muted">{t('cluster.enableText')}</p>
        </div>
      </div>
      <ErrorMessage error={error} />
      <div className="space-y-4">
        <Segmented label={t('cluster.mode')} value={f.mode} onChange={(m) => setF({ ...f, mode: m })} options={[
          { key: 'vip', label: t('cluster.modes.vip') },
          { key: 'manual', label: t('cluster.modes.manual') },
        ]} />
        <p className="text-xs text-muted">{t(`cluster.modeHint.${f.mode}`)}</p>
        <div className="grid gap-4 sm:grid-cols-3">
          <Input label={t('cluster.address')} hint={t('cluster.addressHint')} value={f.address} onChange={(e) => setF({ ...f, address: e.target.value })} className="font-mono" />
          {f.mode === 'vip' && <Input label={t('cluster.vip')} hint={t('cluster.vipHint')} value={f.vip} onChange={(e) => setF({ ...f, vip: e.target.value })} className="font-mono" placeholder="192.168.1.10/24" />}
          {f.mode === 'vip' && (
            <Select label={t('cluster.interface')} value={f.interface} onChange={(e) => setF({ ...f, interface: e.target.value })}>
              <option value="">{t('cluster.interfaceAuto')}</option>
              {view.status.interface && <option value={view.status.interface}>{view.status.interface}</option>}
            </Select>
          )}
        </div>
        <Alert tone="warn">{t('cluster.warning')}</Alert>
        <div><Button busy={busy} onClick={enable}><Server className="size-4" aria-hidden /> {t('cluster.enable')}</Button></div>
      </div>
    </Card>
  )
}
