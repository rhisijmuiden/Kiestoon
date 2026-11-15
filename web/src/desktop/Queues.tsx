import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LogIn, LogOut } from 'lucide-react'
import { formatDuration } from '../components/kit'
import { Button, ErrorMessage } from '../components/ui'
import { api } from '../lib/api'
import { Title } from './Contacts'
import { myQueues, useDesktop } from './data'
import { Switch } from './Switch'

const tones = ['text-accent bg-accent/15', 'text-accent-2 bg-accent-2/15', 'text-warn bg-warn/15', 'text-ok bg-ok/15']

/** Sign in to and out of queues, and pause. Static members are always signed in. */
export function Queues() {
  const { t } = useTranslation()
  const { queues, me } = useDesktop()
  const [error, setError] = useState<unknown>(null)
  const [busy, setBusy] = useState(false)
  const q = myQueues(queues.data, me.data?.id)
  async function act(id: number, action: string) {
    setBusy(true)
    setError(null)
    try {
      await api('POST', `/portal/queues/${id}`, { action })
      await queues.reload()
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const allIn = q.dynamic.every((x) => q.member(x)?.loggedIn)
  return (
    <div className="pb-4">
      <Title sub={t('desktop.queuesHint')}>{t('desktop.nav.queues')}</Title>
      <ErrorMessage error={error || queues.error} />
      {q.mine.length === 0 && !queues.loading && <p className="px-4 py-8 text-center text-sm text-muted">{t('desktop.noQueues')}</p>}
      <div className="flex flex-col gap-2.5 px-3.5">
        {q.mine.map((x, i) => {
          const m = q.member(x)!
          const live = x.live
          return (
            <div key={x.id} className="flex items-center gap-3 rounded-2xl border border-line bg-surface px-3.5 py-3">
              <span className={`inline-flex size-9 flex-none items-center justify-center rounded-[10px] font-semibold ${tones[i % tones.length]}`} aria-hidden>{x.name.slice(0, 1).toUpperCase()}</span>
              <span className="min-w-0 flex-1">
                <span className="block truncate text-[13.5px] font-semibold text-ink">{x.name} <span className="font-mono text-[11px] font-normal text-subtle">{x.number}</span></span>
                <span className="flex flex-wrap gap-x-2.5 text-[11.5px] text-muted">
                  <span>{t('desktop.waiting', { count: live?.calls ?? 0 })}</span>
                  {!!live?.longestWait && <span>{t('desktop.longest', { time: formatDuration(live.longestWait) })}</span>}
                  <span>{t('desktop.agents', { count: (live?.agents ?? []).length || x.members.filter((y) => y.loggedIn).length })}</span>
                  {!m.dynamic && <span className="text-subtle">{t('desktop.fixedMember')}</span>}
                </span>
              </span>
              <Switch checked={m.loggedIn} disabled={!m.dynamic || busy} label={t('desktop.signedInTo', { name: x.name })} onChange={(v) => act(x.id, v ? 'login' : 'logout')} />
            </div>
          )
        })}
        {q.mine.length > 0 && (
          <div className="flex items-center gap-3 rounded-2xl border border-line bg-surface px-3.5 py-3">
            <span className="min-w-0 flex-1">
              <span className="block text-[13.5px] font-semibold text-ink">{t('desktop.pause')}</span>
              <span className="block text-[11.5px] text-muted">{t('desktop.pauseHint')}</span>
            </span>
            <Switch checked={q.paused} disabled={busy} label={t('desktop.pause')} onChange={(v) => act(0, v ? 'pause' : 'unpause')} />
          </div>
        )}
        {q.dynamic.length > 1 && (
          <div className="mt-1 grid grid-cols-2 gap-2">
            <Button variant="ghost" className="min-h-9 text-[12.5px]" disabled={busy || q.signedIn.length === 0} onClick={() => act(0, 'logout')}><LogOut className="size-4" aria-hidden />{t('portal.signOutAll')}</Button>
            <Button className="min-h-9 text-[12.5px]" disabled={busy || allIn} onClick={() => act(0, 'login')}><LogIn className="size-4" aria-hidden />{t('portal.signInAll')}</Button>
          </div>
        )}
      </div>
    </div>
  )
}
