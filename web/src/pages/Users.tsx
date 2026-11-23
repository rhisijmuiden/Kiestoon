import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { KeyRound, Mail, Pencil, Plus, ShieldCheck, Trash2, UserCog } from 'lucide-react'
import { Badge, Empty, Modal, Select, Table, Toggle, formatDate, td, useApi } from '../components/kit'
import { IconButton } from '../components/pbx'
import { Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api, roles, type Role, type User } from '../lib/api'
import { useSession } from '../lib/session'
import type { Extension, SsoProvider, WelcomeResult } from '../lib/types'
import { WelcomeNotice } from '../components/WelcomeNotice'
import { useCallerLanguage } from '../components/CallerLanguage'

type Form = { id: number; username: string; displayName: string; email: string; role: Role; extensionId: number | null; language: 'en' | 'nl'; enabled: boolean; password: string }

const roleTone = { superadmin: 'danger', admin: 'accent', reception: 'warn', user: 'muted' } as const

export function Users() {
  const { t, i18n } = useTranslation()
  const { state } = useSession()
  const me = state.kind === 'ready' ? state.me.user : undefined
  const list = useApi<User[]>('/users')
  const exts = useApi<Extension[]>('/pbx/extensions')
  const callerLang = useCallerLanguage()
  const sso = useApi<{ providers: SsoProvider[] }>('/sso/providers')
  const [edit, setEdit] = useState<Form | null>(null)
  const [error, setError] = useState<unknown>(null)
  const [notice, setNotice] = useState<WelcomeResult | null>(null)
  async function sendWelcome(u: User) {
    if (!window.confirm(t('welcome.confirm', { email: u.email }))) return
    setError(null)
    setNotice(null)
    try {
      setNotice(await api<WelcomeResult>('POST', `/users/${u.id}/welcome`, {}))
    } catch (e) {
      setError(e)
    }
  }
  const extName = (id: number | null) => exts.data?.find((e) => e.id === id)
  const provName = (id: number) => sso.data?.providers.find((p) => p.id === id)?.name
  async function act(fn: () => Promise<unknown>) {
    setError(null)
    try {
      await fn()
      list.reload()
    } catch (e) {
      setError(e)
    }
  }
  const toForm = (u: User): Form => ({ id: u.id, username: u.username, displayName: u.displayName, email: u.email, role: u.role, extensionId: u.extensionId, language: u.language, enabled: u.enabled, password: '' })
  return (
    <>
      <PageHeader title={t('users.title')} subtitle={t('users.subtitle')}>
        <Button onClick={() => setEdit({ id: 0, username: '', displayName: '', email: '', role: 'user', extensionId: null, language: callerLang ?? (i18n.language === 'nl' ? 'nl' : 'en'), enabled: true, password: '' })}>
          <Plus className="size-4" aria-hidden /> {t('users.add')}
        </Button>
      </PageHeader>
      <ErrorMessage error={list.error || error} />
      {notice && <WelcomeNotice result={notice} />}
      <Card>
        <Table head={[t('users.user'), t('users.role'), t('users.extension'), t('users.signIn'), t('users.lastLogin'), '']}
          empty={list.data?.length === 0 && <Empty icon={<UserCog className="size-7" />} title={t('users.emptyTitle')} />}>
          {list.data?.map((u) => {
            const e = extName(u.extensionId)
            return (
              <tr key={u.id} className={u.enabled ? '' : 'opacity-50'}>
                <td className={td}>
                  <div className="text-ink">{u.displayName} {u.id === me?.id && <span className="text-xs text-subtle">({t('users.you')})</span>}</div>
                  <div className="text-xs text-subtle">{u.username}{u.email && ` · ${u.email}`}</div>
                </td>
                <td className={td}><Badge tone={roleTone[u.role]}>{t(`roles.${u.role}`)}</Badge></td>
                <td className={td}>{e ? <span><span className="font-mono text-ink">{e.number}</span> <span className="text-xs text-subtle">{e.name}</span></span> : '–'}</td>
                <td className={td}>
                  <div className="flex flex-wrap gap-1">
                    {u.ssoProvider ? <Badge tone="accent">{provName(u.ssoProvider) ?? 'SSO'}</Badge> : <Badge>{t('users.password')}</Badge>}
                    {u.mfaEnabled && <Badge tone="ok"><ShieldCheck className="size-3" aria-hidden /> MFA</Badge>}
                  </div>
                </td>
                <td className={`${td} text-xs`}>{u.lastLoginAt ? formatDate(u.lastLoginAt, i18n.language) : t('users.never')}</td>
                <td className={`${td} whitespace-nowrap text-right`}>
                  {u.mfaEnabled && u.id !== me?.id && (
                    <IconButton label={t('users.resetMfa')} onClick={() => window.confirm(t('users.resetMfaConfirm', { name: u.displayName })) && act(() => api('POST', `/users/${u.id}/mfa-reset`))}>
                      <KeyRound className="size-4" aria-hidden />
                    </IconButton>
                  )}
                  {u.email && u.enabled && u.id !== me?.id && (
                    <IconButton label={t('welcome.resend')} onClick={() => sendWelcome(u)}><Mail className="size-4" aria-hidden /></IconButton>
                  )}
                  <IconButton label={t('common.edit')} onClick={() => setEdit(toForm(u))}><Pencil className="size-4" aria-hidden /></IconButton>
                  <IconButton label={t('common.delete')} danger disabled={u.id === me?.id}
                    onClick={() => window.confirm(t('common.confirmDelete', { name: u.displayName })) && act(() => api('DELETE', `/users/${u.id}`))}>
                    <Trash2 className="size-4" aria-hidden />
                  </IconButton>
                </td>
              </tr>
            )
          })}
        </Table>
      </Card>
      {edit && <UserForm value={edit} self={edit.id === me?.id} superadmin={me?.role === 'superadmin'} extensions={exts.data ?? []}
        onClose={() => setEdit(null)} onSaved={() => { setEdit(null); list.reload() }} />}
    </>
  )
}

function UserForm({ value, self, superadmin, extensions, onClose, onSaved }: {
  value: Form; self: boolean; superadmin: boolean; extensions: Extension[]; onClose: () => void; onSaved: () => void
}) {
  const { t } = useTranslation()
  const [u, setU] = useState(value)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const set = <K extends keyof Form>(k: K, v: Form[K]) => setU((x) => ({ ...x, [k]: v }))
  async function save() {
    setBusy(true)
    setError(null)
    try {
      await api(u.id ? 'PUT' : 'POST', u.id ? `/users/${u.id}` : '/users', u)
      onSaved()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  return (
    <Modal title={u.id ? t('users.edit', { name: value.displayName }) : t('users.add')} onClose={onClose}
      footer={<><Button variant="ghost" onClick={onClose}>{t('common.cancel')}</Button><Button busy={busy} onClick={save}>{t('common.save')}</Button></>}>
      <ErrorMessage error={error} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Input name="username" label={t('users.username')} value={u.username} onChange={(e) => set('username', e.target.value.toLowerCase())} autoFocus autoComplete="off" />
        <Input name="displayName" label={t('users.displayName')} value={u.displayName} onChange={(e) => set('displayName', e.target.value)} />
      </div>
      <Input name="email" label={t('users.email')} type="email" value={u.email} onChange={(e) => set('email', e.target.value)} hint={t('users.emailHint')} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Select name="role" label={t('users.role')} value={u.role} disabled={self} onChange={(e) => set('role', e.target.value as Role)} hint={t(`roles.hints.${u.role}`)}>
          {roles.filter((r) => r !== 'superadmin' || superadmin).map((r) => <option key={r} value={r}>{t(`roles.${r}`)}</option>)}
        </Select>
        <Select name="extensionId" label={t('users.extension')} value={u.extensionId ?? 0} onChange={(e) => set('extensionId', Number(e.target.value) || null)} hint={t('users.extensionHint')}>
          <option value={0}>{t('users.noExtension')}</option>
          {extensions.map((e) => <option key={e.id} value={e.id}>{e.number} {e.name}</option>)}
        </Select>
      </div>
      <Select name="language" label={t('account.language')} value={u.language} onChange={(e) => set('language', e.target.value as 'en' | 'nl')}>
        <option value="en">English</option>
        <option value="nl">Nederlands</option>
      </Select>
      <Input name="password" label={u.id ? t('users.newPassword') : t('users.password')} type="password" value={u.password} onChange={(e) => set('password', e.target.value)} autoComplete="new-password"
        hint={u.id ? t('users.newPasswordHint') : t('users.passwordHint')} />
      <Toggle name="enabled" label={t('common.enabled')} hint={t('users.enabledHint')} checked={u.enabled} disabled={self} onChange={(v) => set('enabled', v)} />
    </Modal>
  )
}
