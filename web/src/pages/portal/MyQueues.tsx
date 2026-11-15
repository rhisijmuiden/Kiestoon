import { useTranslation } from 'react-i18next'
import { LogIn, LogOut, UsersRound } from 'lucide-react'
import { Badge, Empty, SectionTitle, useApi } from '../../components/kit'
import { stateTone } from '../../components/pbx'
import { Button, Card, ErrorMessage, PageHeader } from '../../components/ui'
import { api } from '../../lib/api'
import type { PortalMe, QueueView, RingGroupStatus } from '../../lib/types'
import { QueueBoard } from '../Reception'

/** The user's queues (sign in and out, pause) and the status of their hunt groups. */
export function MyQueues() {
  const { t } = useTranslation()
  const me = useApi<PortalMe>('/portal/me')
  const queues = useApi<QueueView[]>('/portal/queues', 3000)
  const groups = useApi<RingGroupStatus[]>('/portal/groups', 5000)
  async function act(q: number, action: string) {
    await api('POST', `/portal/queues/${q}`, { action })
    queues.reload()
  }
  const dynamic = (queues.data ?? []).filter((q) => q.members.some((m) => m.extensionId === me.data?.id && m.dynamic))
  const allIn = dynamic.every((q) => q.members.find((m) => m.extensionId === me.data?.id)?.loggedIn)
  return (
    <>
      <PageHeader title={t('portal.queuesGroups')} subtitle={t('portal.queuesGroupsHint')}>
        {dynamic.length > 1 && (
          <Button variant={allIn ? 'ghost' : 'secondary'} onClick={() => act(0, allIn ? 'logout' : 'login')}>
            {allIn ? <LogOut className="size-4" aria-hidden /> : <LogIn className="size-4" aria-hidden />} {allIn ? t('portal.signOutAll') : t('portal.signInAll')}
          </Button>
        )}
      </PageHeader>
      <ErrorMessage error={queues.error || groups.error} />
      <QueueBoard queues={queues.data ?? []} myExtensionId={me.data?.id} onAgent={(q, _ext, action) => act(q, action)} />
      <Card>
        <SectionTitle>{t('portal.myGroups')}</SectionTitle>
        {groups.data?.length === 0 && <Empty icon={<UsersRound className="size-7" />} title={t('portal.noGroups')} />}
        <div className="grid gap-4 md:grid-cols-2">
          {groups.data?.map((g) => (
            <div key={g.id} className="rounded-xl border border-line p-4">
              <div className="mb-3 flex items-center justify-between">
                <div><div className="font-medium text-ink">{g.name}</div><div className="font-mono text-xs text-subtle">{g.number}</div></div>
                <Badge tone="accent">{t(`ringGroups.strategies.${g.strategy}`)}</Badge>
              </div>
              <div className="flex flex-col gap-1.5">
                {g.members.map((m) => (
                  <div key={m.number} className="flex items-center gap-3 text-sm">
                    <span className="font-mono text-ink">{m.number}</span>
                    <span className="flex-1 truncate text-muted">{m.name || t('ringGroups.externalNumber')}</span>
                    <Badge tone={stateTone(m.state)} dot>{t(`states.${m.state || 'unknown'}`, { defaultValue: m.state })}</Badge>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </Card>
    </>
  )
}
