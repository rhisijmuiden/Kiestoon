import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Building2, KeyRound, Pencil, Plus, Trash2 } from 'lucide-react'
import { Badge, CopyField, Empty, Modal, SectionTitle, Table, Toggle, formatDate, td, useApi } from '../components/kit'
import { IconButton } from '../components/pbx'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { EdgeSite, SitesView } from '../lib/types'

/** Branch offices: the WireGuard hub of this Kiestoon and the Kiestoon Edge satellites. */
export function Sites() {
  const { t, i18n } = useTranslation()
  const view = useApi<SitesView>('/sites', 10000)
  const [add, setAdd] = useState(false)
  const [code, setCode] = useState<{ name: string; code: string } | null>(null)
  const [rename, setRename] = useState<EdgeSite | null>(null)
  const [error, setError] = useState<unknown>(null)
  const v = view.data

  async function remove(e: EdgeSite) {
    if (!window.confirm(t('sites.confirmDelete', { name: e.name }))) return
    await api('DELETE', `/sites/edges/${e.id}`).catch(setError)
    view.reload()
  }
  async function repair(e: EdgeSite) {
    if (e.paired && !window.confirm(t('sites.confirmRepair', { name: e.name }))) return
    try {
      const r = await api<{ code: string }>('POST', `/sites/edges/${e.id}/pairing`, {})
      setCode({ name: e.name, code: r.code })
      view.reload()
    } catch (err) {
      setError(err)
    }
  }

  return (
    <>
      <PageHeader title={t('sites.title')} subtitle={t('sites.subtitle')}>
        <Button onClick={() => setAdd(true)} disabled={!v?.hub.enabled}>
          <Plus className="size-4" aria-hidden /> {t('sites.add')}
        </Button>
      </PageHeader>
      <ErrorMessage error={view.error ?? error} />
      {v && !v.helperAvailable && <Alert tone="warn">{t('sites.helperMissing')}</Alert>}
      {v && v.helperAvailable && !v.wireguardInstalled && <Alert tone="warn">{t('sites.wireguardMissing')}</Alert>}
      {v?.tunnelErrors && Object.entries(v.tunnelErrors).map(([k, e]) => <Alert key={k}>{k}: {e}</Alert>)}
      {v && <HubCard view={v} onSaved={view.reload} />}
      <Card>
        <SectionTitle>{t('sites.edges')}</SectionTitle>
        <Table head={[t('common.name'), t('common.status'), t('sites.office'), t('sites.tunnel'), t('sites.lastSeen'), '']}
          empty={v?.edges.length === 0 && <Empty icon={<Building2 className="size-7" />} title={t('sites.emptyTitle')} text={t('sites.emptyText')} />}>
          {v?.edges.map((e) => (
            <tr key={e.id}>
              <td className={`${td} text-ink`}>
                <div>{e.name}</div>
                {e.hostname && <div className="text-xs text-subtle">{e.hostname} · {e.version}</div>}
              </td>
              <td className={td}>
                {!e.paired ? <Badge tone="warn">{t('sites.waitingPairing')}</Badge>
                  : e.connected ? <Badge tone="ok" dot>{t('sites.connected')}</Badge> : <Badge tone="danger" dot>{t('sites.offline')}</Badge>}
                {e.paired && e.services && Object.entries(e.services).filter(([, s]) => s !== 'active').map(([k]) => (
                  <div key={k} className="mt-1"><Badge tone="danger">{k}</Badge></div>
                ))}
              </td>
              <td className={`${td} font-mono`}>{e.lanAddress}{e.lanNet && <div className="text-xs text-subtle">{e.lanNet}</div>}</td>
              <td className={`${td} font-mono`}>{e.tunnelIp}{e.remote && <div className="text-xs text-subtle">{e.remote}</div>}</td>
              <td className={td}>{e.lastSeen ? formatDate(e.lastSeen, i18n.language) : '–'}</td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <IconButton label={t('sites.newCode')} onClick={() => repair(e)}><KeyRound className="size-4" aria-hidden /></IconButton>
                <IconButton label={t('common.edit')} onClick={() => setRename(e)}><Pencil className="size-4" aria-hidden /></IconButton>
                <IconButton label={t('common.delete')} danger onClick={() => remove(e)}><Trash2 className="size-4" aria-hidden /></IconButton>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
      <Card>
        <SectionTitle>{t('sites.howTitle')}</SectionTitle>
        <ol className="list-decimal space-y-1.5 pl-5 text-sm text-ink-2">
          <li>{t('sites.how1')}</li>
          <li>{t('sites.how2')}</li>
          <li>{t('sites.how3')}</li>
          <li>{t('sites.how4')}</li>
        </ol>
      </Card>
      {add && v && <AddEdge portalUrl={v.portalUrl} onClose={() => setAdd(false)} onCreated={(name, c) => { setAdd(false); setCode({ name, code: c }); view.reload() }} />}
      {code && <CodeDialog name={code.name} code={code.code} onClose={() => setCode(null)} />}
      {rename && <RenameEdge edge={rename} onClose={() => setRename(null)} onSaved={() => { setRename(null); view.reload() }} />}
    </>
  )
}

function HubCard({ view, onSaved }: { view: SitesView; onSaved: () => void }) {
  const { t } = useTranslation()
  const [h, setH] = useState({ enabled: view.hub.enabled, host: view.hub.host, listenPort: view.hub.listenPort || 51820 })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  async function save() {
    setBusy(true)
    setError(null)
    setSaved(false)
    try {
      await api('PUT', '/sites/hub', h)
      setSaved(true)
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const up = view.tunnelsUp?.includes('kt0')
  return (
    <Card>
      <SectionTitle action={view.hub.enabled ? (up ? <Badge tone="ok" dot>{t('sites.hubUp')}</Badge> : <Badge tone="warn">{t('sites.hubDown')}</Badge>) : undefined}>
        {t('sites.hub')}
      </SectionTitle>
      <p className="mb-4 text-sm text-muted">{t('sites.hubText')}</p>
      <ErrorMessage error={error} />
      {saved && <Alert tone="ok">{t('common.saved')}</Alert>}
      <div className="space-y-4">
        <Toggle name="enabled" label={t('sites.hubEnabled')} hint={t('sites.hubEnabledHint')} checked={h.enabled} onChange={(v) => setH({ ...h, enabled: v })} />
        <div className="grid gap-4 sm:grid-cols-[1fr_160px]">
          <Input name="host" label={t('sites.publicHost')} hint={t('sites.publicHostHint')} value={h.host} onChange={(e) => setH({ ...h, host: e.target.value })} placeholder="pbx.example.com" />
          <Input name="listenPort" label={t('sites.udpPort')} type="number" min={1024} max={65535} value={h.listenPort} onChange={(e) => setH({ ...h, listenPort: Number(e.target.value) })} />
        </div>
        {view.hub.enabled && (
          <div className="grid gap-4 sm:grid-cols-2">
            <CopyField label={t('sites.subnet')} value={view.hub.subnet} />
            <CopyField label={t('sites.publicKey')} value={view.hub.publicKey} />
          </div>
        )}
        <Alert tone="warn">{t('sites.portHint', { port: h.listenPort })}</Alert>
        <div><Button busy={busy} onClick={save}>{t('common.save')}</Button></div>
      </div>
    </Card>
  )
}

function AddEdge({ portalUrl, onClose, onCreated }: { portalUrl: string; onClose: () => void; onCreated: (name: string, code: string) => void }) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [url, setUrl] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  async function create() {
    setBusy(true)
    setError(null)
    try {
      const r = await api<{ edge: EdgeSite; code: string }>('POST', '/sites/edges', { name, url })
      onCreated(r.edge.name, r.code)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={t('sites.add')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={create}>{t('sites.createCode')}</Button></>}>
      <ErrorMessage error={error} />
      <Input name="name" label={t('common.name')} value={name} onChange={(e) => setName(e.target.value)} placeholder={t('sites.namePlaceholder')} autoFocus />
      <Input name="portalUrl" label={t('sites.portalUrl')} hint={t('sites.portalUrlHint', { url: portalUrl })} value={url} onChange={(e) => setUrl(e.target.value)} placeholder={portalUrl} />
    </Modal>
  )
}

function CodeDialog({ name, code, onClose }: { name: string; code: string; onClose: () => void }) {
  const { t } = useTranslation()
  return (
    <Modal title={t('sites.codeTitle', { name })} onClose={onClose} footer={<Button onClick={onClose}>{t('common.close')}</Button>}>
      <p className="text-sm text-muted">{t('sites.codeText')}</p>
      <CopyField label={t('sites.pairingCode')} value={code} />
      <Alert tone="warn">{t('sites.codeValid')}</Alert>
    </Modal>
  )
}

function RenameEdge({ edge, onClose, onSaved }: { edge: EdgeSite; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [name, setName] = useState(edge.name)
  const [error, setError] = useState<unknown>(null)
  async function save() {
    try {
      await api('PUT', `/sites/edges/${edge.id}`, { name })
      onSaved()
    } catch (e) {
      setError(e)
    }
  }
  return (
    <Modal title={t('common.edit')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <Input name="name" label={t('common.name')} value={name} onChange={(e) => setName(e.target.value)} autoFocus />
    </Modal>
  )
}
