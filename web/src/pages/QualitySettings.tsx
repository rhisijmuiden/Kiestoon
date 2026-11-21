import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from 'react-router'
import { ArrowLeft, BellRing, Gauge, Send } from 'lucide-react'
import { SectionTitle, Toggle, useApi } from '../components/kit'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { num } from '../components/quality'
import { api } from '../lib/api'
import type { QualitySettings as Settings } from '../lib/types'

/** Limits and alerts of call quality. */
export function QualitySettings() {
  const { t, i18n } = useTranslation()
  const lang = i18n.language
  const d = useApi<Settings>('/quality/settings')
  const [s, setS] = useState<Settings | null>(null)
  const [rcpt, setRcpt] = useState('')
  // The limits as typed, so a decimal comma or point can be typed in either language.
  const [lim, setLim] = useState<Record<LimitKey, string>>({ good: '', poor: '', jitter: '', loss: '', rtt: '' })
  const [busy, setBusy] = useState<'' | 'save' | 'test'>('')
  const [error, setError] = useState<unknown>(null)
  const [done, setDone] = useState<'' | 'saved' | 'tested'>('')
  useEffect(() => {
    if (d.data) {
      setS(d.data)
      setRcpt(d.data.recipients.join(', '))
      setLim(limitText(d.data, i18n.language))
    }
  }, [d.data, i18n.language])
  if (!s) return <><PageHeader title={t('quality.settingsTitle')} /><ErrorMessage error={d.error} /></>

  const set = (p: Partial<Settings>) => { setDone(''); setS({ ...s, ...p }) }
  const n = (v: string) => (v === '' ? 0 : Number(v.replace(',', '.')))
  const recipients = () => rcpt.split(/[\s,;]+/).filter(Boolean)
  async function run(kind: 'save' | 'test') {
    setBusy(kind)
    setError(null)
    setDone('')
    try {
      if (kind === 'save') {
        const limits = Object.fromEntries(limitKeys.map((k) => [k, n(lim[k])]))
        const saved = await api<Settings>('PUT', '/quality/settings', { ...s, ...limits, recipients: recipients() })
        setS(saved)
        setRcpt(saved.recipients.join(', '))
        setLim(limitText(saved, lang))
        setDone('saved')
      } else {
        await api('POST', '/quality/test-mail', { recipients: recipients() })
        setDone('tested')
      }
    } catch (e) {
      setError(e)
    } finally {
      setBusy('')
    }
  }
  const field = (k: LimitKey, unit?: string) => (
    <Input name={k} label={t(`quality.limit.${k}`)} inputMode="decimal" value={lim[k]}
      onChange={(e) => { setDone(''); setLim({ ...lim, [k]: e.target.value }) }} hint={unit} />
  )
  const good = n(lim.good) || s.good
  const poor = n(lim.poor) || s.poor
  const ruleNum = (rule: 'trunk' | 'phone', k: 'minutes' | 'count', label: string) => (
    <Input name={`${rule}${k === 'minutes' ? 'Minutes' : 'Count'}`} label={label} inputMode="numeric" value={String(s[rule][k])}
      onChange={(e) => set({ [rule]: { ...s[rule], [k]: n(e.target.value) } } as Partial<Settings>)} />
  )

  return (
    <>
      <PageHeader title={t('quality.settingsTitle')} subtitle={t('quality.settingsSubtitle')}>
        <Link to="/quality" className="inline-flex min-h-11 items-center gap-2 rounded-[10px] border border-line px-4 text-sm font-medium text-ink-2 transition hover:bg-tint/5">
          <ArrowLeft className="size-4" aria-hidden />{t('quality.title')}
        </Link>
        <Button variant="ghost" busy={busy === 'test'} onClick={() => run('test')} disabled={recipients().length === 0}><Send className="size-4" aria-hidden />{t('quality.testMail')}</Button>
        <Button busy={busy === 'save'} onClick={() => run('save')}>{t('common.save')}</Button>
      </PageHeader>
      <ErrorMessage error={error} />
      {done === 'saved' && <Alert tone="ok">{t('quality.saved')}</Alert>}
      {done === 'tested' && <Alert tone="ok">{t('quality.tested')}</Alert>}

      <Card>
        <SectionTitle><span className="inline-flex items-center gap-2"><Gauge className="size-4 text-accent" aria-hidden />{t('quality.limitsTitle')}</span></SectionTitle>
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-5">
          {field('good')}
          {field('poor')}
          {field('jitter', 'ms')}
          {field('loss', '%')}
          {field('rtt', 'ms')}
        </div>
        <div className="mt-4 grid grid-cols-3 overflow-hidden rounded-[10px] text-center text-xs font-medium">
          <div className="bg-danger/15 px-1 py-2 text-danger-ink">{t('quality.scale.poor', { v: num(poor, 1, lang) })}</div>
          <div className="bg-warn/15 px-1 py-2 text-warn-ink">{t('quality.scale.fair', { a: num(poor, 1, lang), b: num(good, 1, lang) })}</div>
          <div className="bg-ok/15 px-1 py-2 text-ok-ink">{t('quality.scale.good', { v: num(good, 1, lang) })}</div>
        </div>
        <p className="mt-3 max-w-[80ch] text-[12.5px] text-muted">{t('quality.mosHint')}</p>
      </Card>

      <div className="grid gap-5 xl:grid-cols-2">
        <Card>
          <SectionTitle><span className="inline-flex items-center gap-2"><BellRing className="size-4 text-accent" aria-hidden />{t('quality.rulesTitle')}</span></SectionTitle>
          <div className="flex flex-col gap-3">
            <Toggle name="trunk" label={t('quality.ruleTrunk')} hint={t('quality.ruleTrunkHint', { minutes: s.trunk.minutes, count: s.trunk.count })}
              checked={s.trunk.enabled} onChange={(v) => set({ trunk: { ...s.trunk, enabled: v } })} />
            {s.trunk.enabled && (
              <div className="grid gap-3 sm:grid-cols-2">
                {ruleNum('trunk', 'minutes', t('quality.minutes'))}
                {ruleNum('trunk', 'count', t('quality.minCalls'))}
              </div>
            )}
            <Toggle name="phone" label={t('quality.rulePhone')} hint={t('quality.rulePhoneHint', { minutes: s.phone.minutes, count: s.phone.count })}
              checked={s.phone.enabled} onChange={(v) => set({ phone: { ...s.phone, enabled: v } })} />
            {s.phone.enabled && (
              <div className="grid gap-3 sm:grid-cols-2">
                {ruleNum('phone', 'count', t('quality.poorCount'))}
                {ruleNum('phone', 'minutes', t('quality.minutes'))}
              </div>
            )}
            <Toggle name="eachCall" label={t('quality.ruleEach')} hint={t('quality.ruleEachHint')} checked={s.eachCall} onChange={(v) => set({ eachCall: v })} />
            <Toggle name="recovered" label={t('quality.ruleRecovered')} hint={t('quality.ruleRecoveredHint')} checked={s.recovered} onChange={(v) => set({ recovered: v })} />
          </div>
        </Card>
        <Card>
          <SectionTitle>{t('quality.notifyTitle')}</SectionTitle>
          <div className="flex flex-col gap-4">
            <Toggle name="email" label={t('quality.email')} hint={t('quality.emailHint')} checked={s.email} onChange={(v) => set({ email: v })} />
            <Input name="recipients" label={t('quality.recipients')} value={rcpt} onChange={(e) => { setDone(''); setRcpt(e.target.value) }}
              placeholder="it@example.com, beheer@example.com" hint={t('quality.recipientsHint')} />
            <div className="grid gap-4 sm:grid-cols-2">
              <Input name="quiet" label={t('quality.quiet')} inputMode="numeric" value={String(s.quiet)} onChange={(e) => set({ quiet: n(e.target.value) })} hint={t('quality.quietHint')} />
              <Input name="keepDays" label={t('quality.keepDays')} inputMode="numeric" value={String(s.keepDays)} onChange={(e) => set({ keepDays: n(e.target.value) })} hint={t('quality.keepDaysHint')} />
            </div>
            <p className="text-[12.5px] text-muted">{t('quality.notifyHint')}</p>
          </div>
        </Card>
      </div>
    </>
  )
}

type LimitKey = 'good' | 'poor' | 'jitter' | 'loss' | 'rtt'
const limitKeys: LimitKey[] = ['good', 'poor', 'jitter', 'loss', 'rtt']

function limitText(s: Settings, lang: string): Record<LimitKey, string> {
  const f = (v: number) => String(v).replace('.', lang === 'nl' ? ',' : '.')
  return { good: f(s.good), poor: f(s.poor), jitter: f(s.jitter), loss: f(s.loss), rtt: f(s.rtt) }
}
