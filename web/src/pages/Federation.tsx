import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { BookUser, Link2, Network, Pencil, Plus, RefreshCw, Trash2 } from 'lucide-react'
import { Badge, CopyField, Empty, Modal, SectionTitle, Table, Toggle, formatDate, td, useApi } from '../components/kit'
import { IconButton, resetDestinations, stateTone } from '../components/pbx'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { FedPeer, FederationView } from '../lib/types'

/** Linked Kiestoon systems: shared phone book, site prefixes, internal calls and BLF across sites. */
export function Federation() {
  const { t, i18n } = useTranslation()
  const view = useApi<FederationView>('/federation', 10000)
  const [invite, setInvite] = useState(false)
  const [join, setJoin] = useState(false)
  const [code, setCode] = useState('')
  const [edit, setEdit] = useState<FedPeer | null>(null)
  const [dir, setDir] = useState<FedPeer | null>(null)
  const [error, setError] = useState<unknown>(null)
  const v = view.data

  async function remove(p: FedPeer) {
    if (!window.confirm(t('federation.confirmDelete', { name: p.name }))) return
    await api('DELETE', `/federation/peers/${p.id}`).catch(setError)
    resetDestinations()
    view.reload()
  }
  async function sync(p: FedPeer) {
    setError(null)
    await api('POST', `/federation/peers/${p.id}/sync`, {}).catch(setError)
    resetDestinations()
    view.reload()
  }

  return (
    <>
      <PageHeader title={t('federation.title')} subtitle={t('federation.subtitle')}>
        <Button variant="ghost" onClick={() => setJoin(true)}><Link2 className="size-4" aria-hidden /> {t('federation.join')}</Button>
        <Button onClick={() => setInvite(true)} disabled={!v?.hubEnabled}><Plus className="size-4" aria-hidden /> {t('federation.invite')}</Button>
      </PageHeader>
      <ErrorMessage error={view.error ?? error} />
      {v && !v.hubEnabled && <Alert tone="warn">{t('federation.hubOff')}</Alert>}
      <Card>
        <Table head={[t('common.name'), t('federation.prefix'), t('common.status'), t('federation.entries'), t('sites.lastSeen'), '']}
          empty={v?.peers.length === 0 && <Empty icon={<Network className="size-7" />} title={t('federation.emptyTitle')} text={t('federation.emptyText')} />}>
          {v?.peers.map((p) => (
            <tr key={p.id}>
              <td className={`${td} text-ink`}>
                <div>{p.name}</div>
                <div className="text-xs text-subtle">{t(`federation.role.${p.role}`)}{p.version && ` · ${p.version}`}{p.directory.timezone && ` · ${p.directory.timezone}`}</div>
              </td>
              <td className={`${td} font-mono text-base text-ink`}>{p.prefix}</td>
              <td className={td}>
                {p.state === 'invited' ? <Badge tone="warn">{t('federation.invited')}</Badge>
                  : p.connected ? <Badge tone="ok" dot>{t('sites.connected')}</Badge> : <Badge tone="danger" dot>{t('sites.offline')}</Badge>}
              </td>
              <td className={`${td} font-mono`}>{p.directory.entries?.length ?? 0}</td>
              <td className={td}>{p.lastSeen ? formatDate(p.lastSeen, i18n.language) : '–'}</td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <IconButton label={t('federation.directory')} onClick={() => setDir(p)} disabled={!p.directory.entries?.length}><BookUser className="size-4" aria-hidden /></IconButton>
                <IconButton label={t('federation.sync')} onClick={() => sync(p)} disabled={p.state === 'invited'}><RefreshCw className="size-4" aria-hidden /></IconButton>
                <IconButton label={t('common.edit')} onClick={() => setEdit(p)}><Pencil className="size-4" aria-hidden /></IconButton>
                <IconButton label={t('common.delete')} danger onClick={() => remove(p)}><Trash2 className="size-4" aria-hidden /></IconButton>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
      <Card>
        <SectionTitle>{t('federation.howTitle')}</SectionTitle>
        <ul className="list-disc space-y-1.5 pl-5 text-sm text-ink-2">
          <li>{t('federation.how1')}</li>
          <li>{t('federation.how2')}</li>
          <li>{t('federation.how3')}</li>
          <li>{t('federation.how4')}</li>
        </ul>
      </Card>
      {invite && <InviteDialog onClose={() => setInvite(false)} onCreated={(c) => { setInvite(false); setCode(c); view.reload() }} />}
      {code && (
        <Modal title={t('federation.codeTitle')} onClose={() => setCode('')} footer={<Button onClick={() => setCode('')}>{t('common.close')}</Button>}>
          <p className="text-sm text-muted">{t('federation.codeText')}</p>
          <CopyField label={t('sites.pairingCode')} value={code} />
          <Alert tone="warn">{t('sites.codeValid')}</Alert>
        </Modal>
      )}
      {join && <JoinDialog onClose={() => setJoin(false)} onJoined={() => { setJoin(false); resetDestinations(); view.reload() }} />}
      {edit && <PeerForm peer={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); resetDestinations(); view.reload() }} />}
      {dir && <DirectoryDialog peer={dir} onClose={() => setDir(null)} />}
    </>
  )
}

function InviteDialog({ onClose, onCreated }: { onClose: () => void; onCreated: (code: string) => void }) {
  const { t } = useTranslation()
  const [f, setF] = useState({ name: '', prefix: '', url: '' })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  async function create() {
    setBusy(true)
    setError(null)
    try {
      const r = await api<{ code: string }>('POST', '/federation/invites', f)
      onCreated(r.code)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={t('federation.invite')} subtitle={t('federation.inviteText')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={create}>{t('sites.createCode')}</Button></>}>
      <ErrorMessage error={error} />
      <div className="grid gap-4 sm:grid-cols-[1fr_140px]">
        <Input label={t('federation.siteName')} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder={t('federation.siteNamePlaceholder')} autoFocus />
        <Input label={t('federation.prefix')} value={f.prefix} onChange={(e) => setF({ ...f, prefix: e.target.value.replace(/\D/g, '') })} inputMode="numeric" className="font-mono" placeholder="8" />
      </div>
      <p className="text-xs text-muted">{t('federation.prefixHint')}</p>
      <Input label={t('sites.portalUrl')} hint={t('federation.urlHint')} value={f.url} onChange={(e) => setF({ ...f, url: e.target.value })} placeholder="https://pbx.example.com:8443" />
    </Modal>
  )
}

function JoinDialog({ onClose, onJoined }: { onClose: () => void; onJoined: () => void }) {
  const { t } = useTranslation()
  const [f, setF] = useState({ code: '', name: '', prefix: '' })
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  async function go() {
    setBusy(true)
    setError(null)
    try {
      await api('POST', '/federation/join', f)
      onJoined()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={t('federation.join')} subtitle={t('federation.joinText')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={go}>{t('federation.join')}</Button></>}>
      <ErrorMessage error={error} />
      <Input label={t('sites.pairingCode')} value={f.code} onChange={(e) => setF({ ...f, code: e.target.value })} className="font-mono" placeholder="KT1-F-…" autoFocus />
      <div className="grid gap-4 sm:grid-cols-[1fr_140px]">
        <Input label={t('federation.siteName')} hint={t('federation.siteNameJoinHint')} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} />
        <Input label={t('federation.prefix')} value={f.prefix} onChange={(e) => setF({ ...f, prefix: e.target.value.replace(/\D/g, '') })} inputMode="numeric" className="font-mono" placeholder="6" />
      </div>
      <p className="text-xs text-muted">{t('federation.prefixJoinHint')}</p>
    </Modal>
  )
}

function PeerForm({ peer, onClose, onSaved }: { peer: FedPeer; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [f, setF] = useState({ name: peer.name, prefix: peer.prefix, directDial: peer.directDial })
  const [error, setError] = useState<unknown>(null)
  async function save() {
    try {
      await api('PUT', `/federation/peers/${peer.id}`, f)
      onSaved()
    } catch (e) {
      setError(e)
    }
  }
  return (
    <Modal title={t('common.edit')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <div className="grid gap-4 sm:grid-cols-[1fr_140px]">
        <Input label={t('federation.siteName')} value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} />
        <Input label={t('federation.prefix')} value={f.prefix} onChange={(e) => setF({ ...f, prefix: e.target.value.replace(/\D/g, '') })} inputMode="numeric" className="font-mono" />
      </div>
      <Toggle label={t('federation.directDial')} hint={t('federation.directDialHint')} checked={f.directDial} onChange={(v) => setF({ ...f, directDial: v })} />
      <div className="grid gap-4 sm:grid-cols-2">
        <CopyField label={t('sites.tunnel')} value={`${peer.localIp} → ${peer.tunnelIp}`} />
        {peer.endpoint && <CopyField label={t('federation.endpoint')} value={peer.endpoint} />}
      </div>
    </Modal>
  )
}

function DirectoryDialog({ peer, onClose }: { peer: FedPeer; onClose: () => void }) {
  const { t } = useTranslation()
  const [q, setQ] = useState('')
  const rows = (peer.directory.entries ?? []).filter((e) => !q || `${e.number} ${e.name}`.toLowerCase().includes(q.toLowerCase()))
  return (
    <Modal wide title={t('federation.directoryOf', { name: peer.name })} onClose={onClose} footer={<Button onClick={onClose}>{t('common.close')}</Button>}>
      <Input label={t('common.search')} value={q} onChange={(e) => setQ(e.target.value)} />
      <Table head={[t('federation.dial'), t('common.name'), t('federation.kind'), t('common.status')]}>
        {rows.map((e) => {
          const st = peer.states?.[e.number] ?? ''
          return (
            <tr key={e.number}>
              <td className={`${td} font-mono text-ink`}>{peer.prefix}{e.number}</td>
              <td className={td}>{e.name}</td>
              <td className={td}>{t(`federation.kinds.${e.kind}`, { defaultValue: e.kind })}</td>
              <td className={td}>{st && <Badge tone={stateTone(st)} dot>{t(`states.${st}`, { defaultValue: st })}</Badge>}</td>
            </tr>
          )
        })}
      </Table>
    </Modal>
  )
}
