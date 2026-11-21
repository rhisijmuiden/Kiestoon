import { useTranslation } from 'react-i18next'
import { Link, useOutletContext } from 'react-router'
import { ArrowDownLeft, ArrowUpRight, PhoneCall, PhoneOff, Smartphone, Waypoints } from 'lucide-react'
import { Badge, Empty, SectionTitle, Stat, Table, formatDuration, td, useApi } from '../components/kit'
import { Alert, Card, PageHeader } from '../components/ui'
import type { SystemInfo } from '../lib/api'
import type { Live, PbxStatus } from '../lib/types'
import { useSession } from '../lib/session'
import { MosPill } from '../components/quality'
import { AlertBanner } from './Quality'

const okTrunk = new Set(['registered', 'reachable', 'nonqualified'])

export function Dashboard() {
  const { t } = useTranslation()
  const { state } = useSession()
  const { info, openInfo } = useOutletContext<{ info: SystemInfo | null; openInfo: () => void }>()
  const me = state.kind === 'ready' ? state.me : null
  const live = useApi<Live>('/live', 3000)
  const status = useApi<PbxStatus>('/pbx/status', 15000)
  const l = live.data
  const st = status.data
  const trunksDown = l?.trunks.filter((tr) => !okTrunk.has(tr.status)).length ?? 0
  const healthy = !!st?.connected && trunksDown === 0

  return (
    <>
      <PageHeader title={t('dashboard.title')} subtitle={me?.organization ? `${me.organization} · ${t('dashboard.subtitle')}` : t('dashboard.subtitle')}>
        {st && (
          <span className={`inline-flex min-h-11 items-center gap-2 rounded-xl border px-3.5 text-[13px] ${healthy ? 'border-ok/35 bg-ok/10 text-ok-ink' : 'border-warn/35 bg-warn/10 text-warn-ink'}`}>
            <span className={`size-2 rounded-full ${healthy ? 'bg-ok glow-ok' : 'bg-warn'}`} aria-hidden />
            {!st.connected ? t('dashboard.pbxDown') : trunksDown ? t('dashboard.trunksDown', { count: trunksDown }) : t('dashboard.status')}
          </span>
        )}
        <button type="button" onClick={openInfo} className="inline-flex min-h-11 items-center rounded-xl border border-line px-3.5 font-mono text-[13px] text-muted hover:text-ink">
          {info?.version ?? '…'}
          {info?.channel === 'testing' && <span className="ml-2 text-warn">· {t('info.testing')}</span>}
        </button>
      </PageHeader>

      {st?.restartNeeded && (
        <Alert tone="warn">{t('dashboard.restartNeeded')} <Link to="/settings" className="underline">{t('nav.settings')}</Link></Alert>
      )}
      {st?.lastError && <Alert>{t('settings.applyError', { error: st.lastError })}</Alert>}
      {l?.quality && l.quality.alerts.map((a) => <AlertBanner key={a.id} a={a} limits={l.quality!.limits} />)}

      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <Stat label={t('dashboard.activeCalls')} value={l?.calls.length ?? '…'} tone={l?.calls.length ? 'ok' : undefined} sub={t('dashboard.liveHint')} />
        <Stat label={t('dashboard.callsToday')} value={l?.stats.today ?? '…'} sub={l && t('dashboard.inOut', { inbound: l.stats.inbound, outbound: l.stats.outbound })} />
        <Stat label={t('dashboard.missedToday')} value={l?.stats.missed ?? '…'} tone={l?.stats.missed ? 'warn' : undefined} sub={l && t('dashboard.answered', { count: l.stats.answered })} />
        <Stat label={t('dashboard.phonesOnline')} value={l ? `${l.registered}/${l.extensions}` : '…'} sub={t('dashboard.phonesHint')} />
      </div>

      <Card>
        <SectionTitle>{t('dashboard.activeCalls')}</SectionTitle>
        <Table head={[t('dashboard.direction'), t('dashboard.from'), t('dashboard.to'), t('dashboard.state'), t('dashboard.duration'), 'MOS']}
          empty={l && l.calls.length === 0 && <Empty icon={<PhoneOff className="size-7" />} title={t('dashboard.noCalls')} />}>
          {l?.calls.map((c) => (
            <tr key={c.id}>
              <td className={td}>
                {c.direction === 'inbound' ? <ArrowDownLeft className="inline size-4 text-accent" aria-label={t('history.dir.inbound')} />
                  : c.direction === 'outbound' ? <ArrowUpRight className="inline size-4 text-ok" aria-label={t('history.dir.outbound')} />
                  : <PhoneCall className="inline size-4 text-muted" aria-label={t('history.dir.internal')} />}
              </td>
              <td className={td}><span className="font-mono text-ink">{c.from}</span> {c.fromName && <span className="text-xs text-subtle">{c.fromName}</span>}</td>
              <td className={`${td} font-mono text-ink`}>{c.to}</td>
              <td className={td}><Badge tone={c.state === 'talking' ? 'ok' : c.state === 'ringing' ? 'accent' : 'muted'} dot>{c.state === 'talking' ? t('dashboard.connected') : c.state === 'ringing' ? t('dashboard.ringing') : c.state}</Badge></td>
              <td className={`${td} font-mono`}>{formatDuration(c.duration)}</td>
              <td className={td}>{l.quality?.calls[c.id] ? <MosPill mos={l.quality.calls[c.id]} limits={l.quality.limits} /> : <span className="text-subtle">–</span>}</td>
            </tr>
          ))}
        </Table>
      </Card>

      <div className="grid gap-4 xl:grid-cols-2">
        <Card>
          <SectionTitle action={<Link to="/trunks" className="text-sm text-accent hover:underline">{t('dashboard.manage')}</Link>}>
            <span className="inline-flex items-center gap-2"><Waypoints className="size-5 text-accent" />{t('nav.trunks')}</span>
          </SectionTitle>
          {l && l.trunks.length === 0 && <p className="py-4 text-sm text-muted">{t('dashboard.noTrunks')}</p>}
          <ul className="flex flex-col gap-2">
            {l?.trunks.map((tr) => (
              <li key={tr.id} className="flex items-center justify-between gap-3 rounded-xl border border-line bg-surface-2/60 px-4 py-2.5">
                <span className="text-sm text-ink">{tr.name}</span>
                <Badge tone={okTrunk.has(tr.status) ? 'ok' : tr.status === 'unknown' ? 'muted' : 'danger'} dot>{t(`dashboard.trunkStatus.${tr.status}`, { defaultValue: tr.status })}</Badge>
              </li>
            ))}
          </ul>
        </Card>
        <Card>
          <SectionTitle action={<Link to="/extensions" className="text-sm text-accent hover:underline">{t('dashboard.manage')}</Link>}>
            <span className="inline-flex items-center gap-2"><Smartphone className="size-5 text-accent" />{t('dashboard.registered')}</span>
          </SectionTitle>
          <Table head={[t('extensions.number'), t('dashboard.device'), 'RTT']}
            empty={l && l.contacts.filter((c) => !c.endpoint.startsWith('trunk-')).length === 0 && <p className="py-4 text-sm text-muted">{t('dashboard.noPhones')}</p>}>
            {l?.contacts.filter((c) => !c.endpoint.startsWith('trunk-')).map((c) => (
              <tr key={c.uri}>
                <td className={`${td} font-mono text-ink`}>{c.endpoint}</td>
                <td className={`${td} max-w-[220px] truncate text-xs`} title={c.userAgent}>{c.userAgent || c.uri}</td>
                <td className={`${td} font-mono text-xs`}>{c.rttMs ? `${c.rttMs} ms` : '–'}</td>
              </tr>
            ))}
          </Table>
        </Card>
      </div>
    </>
  )
}
