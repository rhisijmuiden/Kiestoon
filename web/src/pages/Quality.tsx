import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { Activity, AudioWaveform, Settings2 } from 'lucide-react'
import { Empty, SectionTitle, Stat, Table, Tabs, formatDuration, td, useApi } from '../components/kit'
import { Card, ErrorMessage, PageHeader } from '../components/ui'
import { Legend, MosChart, MosPill, Over, Spark, num, rating, seriesColors } from '../components/quality'
import type { QualityAlert, QualityLimits, QualityOverview } from '../lib/types'

type Range = '24h' | '7d' | '30d'

/** Call quality: MOS, jitter, packet loss and round trip per call, trunk and phone. */
export function Quality() {
  const { t, i18n } = useTranslation()
  const lang = i18n.language
  const [range, setRange] = useState<Range>('24h')
  const d = useApi<QualityOverview>(`/quality?range=${range}`, 5000)
  const data = d.data
  const o = data?.overview
  const lim = data?.limits
  const labels = o ? bucketLabels(o.from, o.bucket, o.buckets.length, lang) : []
  const trunks = o?.trunks ?? []
  const weakest = trunks.filter((g) => g.legs > 0).reduce<(typeof trunks)[number] | null>((a, g) => (!a || g.mos < a.mos ? g : a), null)
  const none = !!o && o.total.legs === 0 && data.live.length === 0

  return (
    <>
      <PageHeader title={t('quality.title')} subtitle={t('quality.subtitle')}>
        <Tabs<Range> value={range} onChange={setRange}
          tabs={[{ key: '24h', label: t('quality.range.24h') }, { key: '7d', label: t('quality.range.7d') }, { key: '30d', label: t('quality.range.30d') }]} />
        <Link to="/quality/settings" className="inline-flex min-h-11 items-center gap-2 rounded-[10px] border border-line px-4 text-sm font-medium text-ink-2 transition hover:bg-tint/5">
          <Settings2 className="size-4" aria-hidden />{t('quality.settings')}
        </Link>
      </PageHeader>
      <ErrorMessage error={d.error} />
      {data && lim && data.alerts.map((a) => <AlertBanner key={a.id} a={a} limits={lim} />)}

      {o && lim && (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <Stat label={t('quality.avgMos')} value={o.total.legs ? num(o.total.mos, 2, lang) : '–'}
            tone={o.total.legs ? toneOf(o.total.mos, lim) : undefined}
            sub={o.total.legs ? t('quality.avgMosSub', { rating: t(`quality.rating.${rating(o.total.mos, lim)}`), count: o.total.calls }) : t('quality.noCalls')} />
          <Stat label={t('quality.poorCalls')} value={o.total.calls ? `${num(o.total.poorPct, 1, lang)}%` : '–'}
            tone={o.total.poor ? 'warn' : o.total.calls ? 'ok' : undefined}
            sub={t('quality.poorCallsSub', { count: o.total.poor, limit: num(lim.poor, 1, lang) })} />
          <Stat label={t('quality.weakestTrunk')} value={weakest ? num(weakest.mos, 2, lang) : '–'} tone={weakest ? toneOf(weakest.mos, lim) : undefined}
            sub={weakest ? `${weakest.name} · ${t('quality.lossShort', { loss: num(weakest.loss, 1, lang) })}` : t('quality.noTrunkCalls')} />
          <Stat label={t('quality.activeAlerts')} value={data.alerts.length} tone={data.alerts.length ? 'warn' : 'ok'}
            sub={data.lastCleared ? t('quality.lastCleared', { time: new Date(data.lastCleared).toLocaleString(lang, { dateStyle: 'short', timeStyle: 'short' }) }) : t('quality.noAlertsYet')} />
        </div>
      )}

      {none && (
        <Card>
          <Empty icon={<AudioWaveform className="size-7" />} title={t('quality.emptyTitle')} text={t('quality.emptyText')} />
        </Card>
      )}

      {data && lim && !none && (
        <>
          <Card>
            <SectionTitle action={<span className="inline-flex items-center gap-1.5 text-[12.5px] text-muted"><Activity className="size-3.5 text-ok" aria-hidden />{t('quality.liveHint')}</span>}>
              {t('quality.live')}
            </SectionTitle>
            <Table head={[t('quality.call'), t('quality.route'), t('quality.codec'), 'MOS', t('quality.jitter'), t('quality.loss'), 'RTT', t('quality.duration')]}
              empty={data.live.length === 0 && <p className="py-6 text-center text-sm text-muted">{t('quality.noLive')}</p>}>
              {data.live.map((c) => (
                <tr key={c.id}>
                  <td className={`${td} whitespace-nowrap`}>
                    <Link to={`/history?call=${encodeURIComponent(c.id)}`} className="hover:text-accent-ink">{c.from || '–'} → {c.to || '–'}</Link>
                  </td>
                  <td className={td}>{c.route || t('quality.internal')}</td>
                  <td className={`${td} font-mono text-xs uppercase`}>{c.codec || '–'}</td>
                  <td className={td}><MosPill mos={c.mos} limits={lim} /></td>
                  <td className={`${td} font-mono text-xs`}><Over value={c.jitter} limit={lim.jitter} unit=" ms" /></td>
                  <td className={`${td} font-mono text-xs`}><Over value={c.loss} limit={lim.loss} unit="%" digits={1} /></td>
                  <td className={`${td} font-mono text-xs`}><Over value={c.rtt} limit={lim.rtt} unit=" ms" /></td>
                  <td className={`${td} font-mono text-xs`}>{formatDuration(c.duration)}</td>
                </tr>
              ))}
            </Table>
          </Card>

          <div className="grid gap-5 2xl:grid-cols-2">
            <Card>
              <SectionTitle action={<Legend names={trunks.slice(0, 5).map((g) => g.name)} />}>{t('quality.chartTitle')}</SectionTitle>
              {trunks.length === 0 ? <p className="py-6 text-sm text-muted">{t('quality.noTrunkCalls')}</p> : (
                <MosChart labels={labels} limits={lim} series={trunks.slice(0, 5).map((g) => ({ name: g.name, values: g.buckets }))} />
              )}
              <p className="mt-2 text-[12.5px] text-muted">{t('quality.chartHint', { limit: num(lim.poor, 1, lang) })}</p>
            </Card>
            <Card>
              <SectionTitle>{t('quality.phonesTitle')}</SectionTitle>
              <Table head={[t('quality.phone'), t('quality.calls'), 'MOS', t('quality.where')]}
                empty={o!.phones.length === 0 && <p className="py-6 text-center text-sm text-muted">{t('quality.noPhoneCalls')}</p>}>
                {o!.phones.map((g) => (
                  <tr key={g.key}>
                    <td className={td}>
                      <div className="whitespace-nowrap text-ink">{g.key} {g.name}</div>
                      {g.remote && <div className="font-mono text-[11.5px] text-subtle">{g.remote}</div>}
                    </td>
                    <td className={`${td} font-mono text-xs`}>{g.legs}</td>
                    <td className={td}><MosPill mos={g.mos} limits={lim} /></td>
                    <td className={`${td} min-w-[150px] text-[12.5px] text-muted`}>{t(`quality.causePhone.${g.cause}`)}</td>
                  </tr>
                ))}
              </Table>
            </Card>
          </div>

          <Card>
            <SectionTitle action={<span className="text-[12.5px] text-muted">{t('quality.trunksHint')}</span>}>{t('quality.trunks')}</SectionTitle>
            <Table head={[t('quality.trunk'), t('quality.calls'), t('quality.avgMosShort'), t('quality.poor'), t('quality.jitter'), t('quality.loss'), 'RTT', t('quality.trend'), t('quality.where')]}
              empty={trunks.length === 0 && <p className="py-6 text-center text-sm text-muted">{t('quality.noTrunkCalls')}</p>}>
              {trunks.map((g, i) => (
                <tr key={g.key}>
                  <td className={`${td} text-ink`}>{g.name}</td>
                  <td className={`${td} font-mono text-xs`}>{g.legs}</td>
                  <td className={td}><MosPill mos={g.mos} limits={lim} /></td>
                  <td className={`${td} font-mono text-xs`}>{num(g.poorPct, 1, lang)}%</td>
                  <td className={`${td} whitespace-nowrap font-mono text-xs`} title={t('quality.rxTx')}>
                    <Over value={g.rxJitter} limit={lim.jitter} unit="" /> / <Over value={g.txJitter} limit={lim.jitter} unit=" ms" />
                  </td>
                  <td className={`${td} whitespace-nowrap font-mono text-xs`} title={t('quality.rxTx')}>
                    <Over value={g.rxLoss} limit={lim.loss} unit="" digits={1} /> / <Over value={g.txLoss} limit={lim.loss} unit="%" digits={1} />
                  </td>
                  <td className={`${td} font-mono text-xs`}><Over value={g.rtt} limit={lim.rtt} unit=" ms" /></td>
                  <td className={td}><Spark values={g.buckets} color={i < 5 ? seriesColors[i] : 'var(--color-subtle)'} /></td>
                  <td className={`${td} min-w-[150px] text-[12.5px] text-muted`}>{t(`quality.causeTrunk.${g.cause}`)}</td>
                </tr>
              ))}
            </Table>
            <p className="mt-3 text-[12.5px] text-subtle">{t('quality.rxTxHint')}</p>
          </Card>
        </>
      )}
    </>
  )
}

function toneOf(mos: number, l: QualityLimits) {
  const r = rating(mos, l)
  return r === 'good' ? 'ok' : r === 'fair' ? 'warn' : 'danger'
}

function bucketLabels(from: string, bucket: number, n: number, lang: string) {
  const start = new Date(from).getTime()
  return Array.from({ length: n }, (_, i) => {
    const d = new Date(start + i * bucket * 1000)
    if (bucket >= 86400) return d.toLocaleDateString(lang, { day: 'numeric', month: 'numeric' })
    if (bucket >= 6 * 3600) return `${d.toLocaleDateString(lang, { weekday: 'short' })} ${String(d.getHours()).padStart(2, '0')}h`
    return `${String(d.getHours()).padStart(2, '0')}:00`
  })
}

export function AlertBanner({ a, limits }: { a: QualityAlert; limits: QualityLimits }) {
  const { t, i18n } = useTranslation()
  const lang = i18n.language
  const since = new Date(a.started).toLocaleTimeString(lang, { hour: '2-digit', minute: '2-digit' })
  return (
    <div role="status" className="flex flex-wrap items-start gap-3 rounded-xl border border-warn/40 bg-warn/10 px-4 py-3 text-sm text-warn-ink">
      <span className="mt-1.5 size-2 flex-none rounded-full bg-warn shadow-[0_0_10px_var(--color-warn)]" aria-hidden />
      <div className="min-w-0 flex-1">
        <b className="text-ink">{t(`quality.alertKind.${a.kind}`, { name: a.name })}</b>{' · '}
        {a.kind === 'trunk'
          ? t('quality.alertTrunk', { mos: num(a.mos, 2, lang), calls: a.calls, limit: num(limits.poor, 2, lang) })
          : t('quality.alertPhone', { poor: a.poor, calls: a.calls, limit: num(limits.poor, 2, lang) })}
        {' '}{t(`quality.cause${a.kind === 'trunk' ? 'Trunk' : 'Phone'}.${a.cause}`)}.
        <div className="mt-0.5 text-xs text-muted">{a.mailed ? t('quality.alertSinceMailed', { time: since }) : t('quality.alertSince', { time: since })}</div>
      </div>
    </div>
  )
}
