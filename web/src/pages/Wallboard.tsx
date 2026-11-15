import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { Maximize2, Minimize2, MonitorPlay, Settings2 } from 'lucide-react'
import { Empty, Tabs, useApi } from '../components/kit'
import { Button, Card, ErrorMessage, PageHeader } from '../components/ui'
import { api, roleRank } from '../lib/api'
import { useSession } from '../lib/session'
import type { WallData, WallLayout } from '../lib/types'
import { WallView, type SpyMode } from './wallboard/WallView'

const LAYOUT_KEY = 'kiestoon.wallboard.layout'

/** The wallboard in the portal: the same screen as on a tv, with supervision buttons. */
export function Wallboard() {
  const { t } = useTranslation()
  const { state } = useSession()
  const data = useApi<WallData>('/reception/wallboard', 3000)
  const [full, setFull] = useState(false)
  const [layout, setLayout] = useState<WallLayout | 'rotate'>(() => {
    try { return (localStorage.getItem(LAYOUT_KEY) as WallLayout | 'rotate') || 'overview' } catch { return 'overview' }
  })
  const [spyError, setSpyError] = useState<unknown>(null)
  useEffect(() => {
    const onFs = () => setFull(!!document.fullscreenElement)
    document.addEventListener('fullscreenchange', onFs)
    return () => document.removeEventListener('fullscreenchange', onFs)
  }, [])
  const choose = (l: WallLayout | 'rotate') => { setLayout(l); try { localStorage.setItem(LAYOUT_KEY, l) } catch { /* storage blocked */ } }
  const toggle = () => (document.fullscreenElement ? document.exitFullscreen() : document.getElementById('wallboard')?.requestFullscreen())
  const me = state.kind === 'ready' ? state.me : null
  const admin = !!me && roleRank(me.user.role) >= roleRank('admin')
  const myNumber = me?.extension?.number
  async function spy(agent: string, mode: SpyMode) {
    setSpyError(null)
    try { await api('POST', '/reception/supervise', { agent, mode }) } catch (e) { setSpyError(e) }
  }
  const layouts: WallLayout[] = layout === 'rotate' ? ['overview', 'queue', 'agents'] : [layout]

  return (
    <>
      {!full && (
        <PageHeader title={t('wallboard.title')} subtitle={t('wallboard.subtitle')}>
          {admin && <Link to="/wallboard/settings" className="inline-flex min-h-11 items-center justify-center gap-2 rounded-[10px] border border-line px-4 text-sm font-medium text-ink-2 transition hover:bg-tint/5"><Settings2 className="size-4" aria-hidden />{t('wallboard.settings')}</Link>}
          <Button variant="secondary" onClick={toggle}><Maximize2 className="size-4" aria-hidden />{t('wallboard.fullscreen')}</Button>
        </PageHeader>
      )}
      {!full && (
        <div className="mb-4">
          <Tabs value={layout} onChange={choose} tabs={[
            { key: 'overview', label: t('wallboard.layout.overview') }, { key: 'queue', label: t('wallboard.layout.queue') },
            { key: 'agents', label: t('wallboard.layout.agents') }, { key: 'rotate', label: t('wallboard.layout.rotate') },
          ]} />
        </div>
      )}
      <ErrorMessage error={data.error || spyError} />
      {data.data && data.data.queues.length === 0 && <Card><Empty icon={<MonitorPlay className="size-7" />} title={t('wallboard.emptyTitle')} text={t('wallboard.emptyText')} /></Card>}
      {data.data && data.data.queues.length > 0 && (
        <div id="wallboard" className={full ? 'relative h-screen overflow-hidden bg-bg p-[2vw] text-[clamp(12px,0.9vw,26px)]' : 'text-[13px] xl:text-[14px]'}>
          <WallView data={data.data} layouts={layouts} rotate={layout === 'rotate' ? 30 : 0} onSpy={myNumber ? spy : undefined} self={myNumber} tv={full} />
          {full && <Button variant="ghost" className="absolute right-3 top-3" onClick={toggle} aria-label={t('wallboard.exitFullscreen')}><Minimize2 className="size-4" aria-hidden /></Button>}
        </div>
      )}
    </>
  )
}
