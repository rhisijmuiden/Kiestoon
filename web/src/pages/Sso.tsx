import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { KeyRound, Pencil, Plus, Trash2, X } from 'lucide-react'
import { Badge, CopyField, Empty, Modal, Select, Table, Textarea, Toggle, td, useApi } from '../components/kit'
import { IconButton, useSaver } from '../components/pbx'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api, roles } from '../lib/api'
import type { SsoProvider } from '../lib/types'

type List = { providers: SsoProvider[]; redirectUri: string; kinds: SsoProvider['kind'][]; samlEntityId: string; samlAcs: string }

const presets: Record<SsoProvider['kind'], Partial<SsoProvider>> = {
  entra: { issuer: 'https://login.microsoftonline.com/<tenant-id>/v2.0', scopes: 'openid profile email', groupsClaim: 'groups' },
  google: { issuer: 'https://accounts.google.com', scopes: 'openid profile email', groupsClaim: '' },
  keycloak: { issuer: 'https://sso.example.com/realms/<realm>', scopes: 'openid profile email', groupsClaim: 'groups' },
  authentik: { issuer: 'https://auth.example.com/application/o/<slug>/', scopes: 'openid profile email', groupsClaim: 'groups' },
  generic: { issuer: '', scopes: 'openid profile email', groupsClaim: 'groups' },
  saml: { issuer: '', scopes: '', groupsClaim: 'groups', clientId: '' },
}

const blank: SsoProvider = {
  id: 0, name: 'Microsoft', kind: 'entra', issuer: presets.entra.issuer!, clientId: '', clientSecret: '', secretSet: false, scopes: 'openid profile email',
  groupsClaim: 'groups', roleMap: [], defaultRole: 'user', autoCreate: true, trustMfa: true, enabled: true,
}

export function Sso() {
  const { t } = useTranslation()
  const list = useApi<List>('/sso/providers')
  const [edit, setEdit] = useState<SsoProvider | null>(null)
  async function remove(p: SsoProvider) {
    if (!window.confirm(t('common.confirmDelete', { name: p.name }))) return
    await api('DELETE', `/sso/providers/${p.id}`)
    list.reload()
  }
  return (
    <>
      <PageHeader title={t('sso.title')} subtitle={t('sso.subtitle')}>
        <Button onClick={() => setEdit(blank)}><Plus className="size-4" aria-hidden /> {t('sso.add')}</Button>
      </PageHeader>
      <ErrorMessage error={list.error} />
      {list.data && (
        <Card>
          <p className="mb-3 text-sm text-muted">{t('sso.redirectHint')}</p>
          <CopyField label={t('sso.redirectUri')} value={list.data.redirectUri} />
          <p className="mb-3 mt-5 text-sm text-muted">{t('sso.samlHint')}</p>
          <div className="grid gap-3 lg:grid-cols-2">
            <CopyField label={t('sso.samlEntityId')} value={list.data.samlEntityId} />
            <CopyField label={t('sso.samlAcs')} value={list.data.samlAcs} />
          </div>
        </Card>
      )}
      <Card>
        <Table head={[t('common.name'), t('sso.kind'), t('sso.defaultRole'), t('sso.roleMap'), '', '']}
          empty={list.data?.providers.length === 0 && <Empty icon={<KeyRound className="size-7" />} title={t('sso.emptyTitle')} text={t('sso.emptyText')} />}>
          {list.data?.providers.map((p) => (
            <tr key={p.id} className={p.enabled ? '' : 'opacity-50'}>
              <td className={`${td} text-ink`}>{p.name}</td>
              <td className={td}>{t(`sso.kinds.${p.kind}`)}</td>
              <td className={td}>{t(`roles.${p.defaultRole}`)}</td>
              <td className={`${td} text-xs`}>{p.roleMap.length ? t('sso.mappings', { count: p.roleMap.length }) : '–'}</td>
              <td className={td}>
                <div className="flex flex-wrap gap-1">
                  {p.autoCreate && <Badge tone="accent">{t('sso.autoCreateShort')}</Badge>}
                  {p.trustMfa && <Badge tone="ok">MFA</Badge>}
                </div>
              </td>
              <td className={`${td} whitespace-nowrap text-right`}>
                <IconButton label={t('common.edit')} onClick={() => setEdit({ ...p, clientSecret: '' })}><Pencil className="size-4" aria-hidden /></IconButton>
                <IconButton label={t('common.delete')} danger onClick={() => remove(p)}><Trash2 className="size-4" aria-hidden /></IconButton>
              </td>
            </tr>
          ))}
        </Table>
      </Card>
      {edit && <ProviderForm value={edit} onClose={() => setEdit(null)} onSaved={() => { setEdit(null); list.reload() }} />}
    </>
  )
}

function ProviderForm({ value, onClose, onSaved }: { value: SsoProvider; onClose: () => void; onSaved: () => void }) {
  const { t } = useTranslation()
  const [p, setP] = useState(value)
  const { busy, error, save } = useSaver<SsoProvider>('/sso/providers', onSaved)
  const set = <K extends keyof SsoProvider>(k: K, v: SsoProvider[K]) => setP((x) => ({ ...x, [k]: v }))
  const setMap = (i: number, patch: Partial<SsoProvider['roleMap'][number]>) => set('roleMap', p.roleMap.map((m, j) => (j === i ? { ...m, ...patch } : m)))
  return (
    <Modal wide title={p.id ? t('sso.edit', { name: value.name }) : t('sso.add')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={() => save(p)}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Select label={t('sso.kind')} value={p.kind} onChange={(e) => {
          const kind = e.target.value as SsoProvider['kind']
          setP((x) => ({ ...x, kind, ...(x.id ? {} : presets[kind]), name: x.id ? x.name : t(`sso.kinds.${kind}`) }))
        }}>
          {(Object.keys(presets) as SsoProvider['kind'][]).map((k) => <option key={k} value={k}>{t(`sso.kinds.${k}`)}</option>)}
        </Select>
        <Input label={t('sso.buttonName')} value={p.name} onChange={(e) => set('name', e.target.value)} hint={t('sso.buttonNameHint')} />
      </div>
      {p.kind === 'saml' ? (
        <>
          <Input label={t('sso.samlMetadataUrl')} value={p.issuer} onChange={(e) => set('issuer', e.target.value.trim())} className="font-mono text-sm"
            placeholder="https://login.microsoftonline.com/<tenant>/federationmetadata/2007-06/federationmetadata.xml?appid=…" hint={t('sso.samlMetadataUrlHint')} />
          <Textarea label={t('sso.samlMetadata')} value={p.samlMetadata ?? ''} onChange={(e) => set('samlMetadata', e.target.value)} rows={5}
            placeholder={'<EntityDescriptor …>'} hint={t('sso.samlMetadataHint')} />
          <Input label={t('sso.samlGroups')} value={p.groupsClaim} onChange={(e) => set('groupsClaim', e.target.value.trim())} className="font-mono text-sm" hint={t('sso.samlGroupsHint')} />
        </>
      ) : (
        <>
          <Input label={t('sso.issuer')} value={p.issuer} onChange={(e) => set('issuer', e.target.value.trim())} className="font-mono text-sm" hint={t(`sso.issuerHints.${p.kind}`)} />
        <div className="grid gap-4 sm:grid-cols-2">
          <Input label={t('sso.clientId')} value={p.clientId} onChange={(e) => set('clientId', e.target.value.trim())} className="font-mono text-sm" />
          <Input label={t('sso.clientSecret')} type="password" value={p.clientSecret ?? ''} onChange={(e) => set('clientSecret', e.target.value)} autoComplete="off"
            placeholder={p.secretSet ? '••••••••' : ''} hint={p.secretSet ? t('sso.secretKeep') : undefined} />
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <Input label={t('sso.scopes')} value={p.scopes} onChange={(e) => set('scopes', e.target.value)} className="font-mono text-sm" />
          <Input label={t('sso.groupsClaim')} value={p.groupsClaim} onChange={(e) => set('groupsClaim', e.target.value.trim())} className="font-mono text-sm" hint={t('sso.groupsClaimHint')} />
        </div>
        </>
      )}
      <div className="flex flex-col gap-2">
        <span className="text-sm font-medium text-ink-2">{t('sso.roleMap')}</span>
        <p className="text-xs text-subtle">{t('sso.roleMapHint')}</p>
        {p.roleMap.map((m, i) => (
          <div key={i} className="flex items-end gap-2">
            <div className="flex-1"><Input label={t('sso.group')} value={m.group} onChange={(e) => setMap(i, { group: e.target.value.trim() })} className="font-mono text-sm" placeholder="00000000-0000-…" /></div>
            <div className="w-44"><Select label={t('users.role')} value={m.role} onChange={(e) => setMap(i, { role: e.target.value })}>{roles.map((r) => <option key={r} value={r}>{t(`roles.${r}`)}</option>)}</Select></div>
            <IconButton label={t('common.remove')} danger onClick={() => set('roleMap', p.roleMap.filter((_, j) => j !== i))}><X className="size-4" aria-hidden /></IconButton>
          </div>
        ))}
        <div><Button variant="ghost" onClick={() => set('roleMap', [...p.roleMap, { group: '', role: 'admin' }])}><Plus className="size-4" aria-hidden /> {t('sso.addMapping')}</Button></div>
      </div>
      <Select label={t('sso.defaultRole')} value={p.defaultRole} onChange={(e) => set('defaultRole', e.target.value)} hint={t('sso.defaultRoleHint')}>
        <option value="">{t('sso.noAccess')}</option>
        {roles.filter((r) => r !== 'superadmin').map((r) => <option key={r} value={r}>{t(`roles.${r}`)}</option>)}
      </Select>
      <Toggle label={t('sso.autoCreate')} hint={t('sso.autoCreateHint')} checked={p.autoCreate} onChange={(v) => set('autoCreate', v)} />
      <Toggle label={t('sso.trustMfa')} hint={t('sso.trustMfaHint')} checked={p.trustMfa} onChange={(v) => set('trustMfa', v)} />
      <Toggle label={t('common.enabled')} checked={p.enabled} onChange={(v) => set('enabled', v)} />
    </Modal>
  )
}
