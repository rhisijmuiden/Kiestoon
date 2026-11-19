import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Cloud, Cpu, FlaskConical, KeyRound, Sparkles } from 'lucide-react'
import { SectionTitle, Select, Toggle, useApi } from '../components/kit'
import { Alert, Button, Card, ErrorMessage, Input, PageHeader } from '../components/ui'
import { api } from '../lib/api'
import type { AIData, AISettings, AISummary } from '../lib/types'
import { Items } from './Speech'
import { SummaryView } from './ai/Summary'

/** AI transcripts and summaries of calls and voicemail: a local model or, optionally, Claude. */
export function AI() {
  const { t } = useTranslation()
  const [poll, setPoll] = useState(false)
  const d = useApi<AIData>('/ai', poll ? 2000 : undefined)
  const [s, setS] = useState<AISettings | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const [saved, setSaved] = useState(false)
  const [key, setKey] = useState('')
  const [test, setTest] = useState<{ summary: AISummary; engine: string; seconds: number } | null>(null)
  const [testing, setTesting] = useState(false)
  const [testError, setTestError] = useState<unknown>(null)
  useEffect(() => { if (d.data && !s) setS(d.data.settings) }, [d.data, s])
  useEffect(() => { setPoll(!!d.data?.llms.some((i) => i.progress && !i.progress.error && !i.downloaded)) }, [d.data])
  if (!s || !d.data) return <><PageHeader title={t('ai.title')} /><ErrorMessage error={d.error} /></>
  const data = d.data
  const set = (p: Partial<AISettings>) => { setSaved(false); setS({ ...s, ...p }) }

  async function run(fn: () => Promise<AIData>) {
    setBusy(true)
    setError(null)
    setSaved(false)
    try {
      const r = await fn()
      d.setData(r)
      setS(r.settings)
      setSaved(true)
    } catch (e) {
      setError(e)
    } finally {
      setBusy(false)
    }
  }
  const save = () => run(() => api<AIData>('PUT', '/ai', s))
  const saveKey = (k: string) => run(async () => { const r = await api<AIData>('PUT', '/ai/key', { key: k }); setKey(''); return r })
  async function download(id: string) {
    setError(null)
    try {
      await api('POST', '/speech/download', { id })
      setPoll(true)
      d.reload()
    } catch (e) {
      setError(e)
    }
  }
  async function remove(id: string, name: string) {
    if (!window.confirm(t('common.confirmDelete', { name }))) return
    await api('DELETE', `/speech/files/${id}`)
    d.reload()
  }
  async function runTest() {
    setTesting(true)
    setTest(null)
    setTestError(null)
    try {
      setTest(await api('POST', '/ai/test', s))
    } catch (e) {
      setTestError(e)
    } finally {
      setTesting(false)
    }
  }
  const llm = data.llms.find((m) => m.id === s.llm)

  return (
    <>
      <PageHeader title={t('ai.title')} subtitle={t('ai.subtitle')}>
        <Button busy={busy} onClick={save}>{t('common.save')}</Button>
      </PageHeader>
      {!data.sttReady && <Alert tone="warn">{t('ai.needsStt')}</Alert>}
      {saved && <Alert tone="ok">{t('ai.saved')}</Alert>}
      <ErrorMessage error={error} />
      <div className="grid gap-4 xl:grid-cols-2">
        <Card>
          <SectionTitle><span className="inline-flex items-center gap-2"><Sparkles className="size-5 text-accent" aria-hidden />{t('ai.what')}</span></SectionTitle>
          <div className="flex flex-col gap-4">
            <p className="text-sm text-muted">{t('ai.intro')}</p>
            <Toggle name="calls" label={t('ai.calls')} hint={t('ai.callsHint')} checked={s.calls} onChange={(v) => set({ calls: v })} />
            <Toggle name="voicemail" label={t('ai.voicemail')} hint={t('ai.voicemailHint')} checked={s.voicemail} onChange={(v) => set({ voicemail: v })} />
            <div className="grid gap-4 sm:grid-cols-2">
              <Select name="language" label={t('ai.language')} value={s.language} onChange={(e) => set({ language: e.target.value as AISettings['language'] })}>
                <option value="auto">{t('ai.languageAuto')}</option><option value="nl">Nederlands</option><option value="en">English</option>
              </Select>
              <Input name="keepDays" label={t('ai.keepDays')} type="number" min={0} max={3650} value={s.keepDays} hint={t('ai.keepDaysHint')}
                onChange={(e) => set({ keepDays: Math.max(0, Number(e.target.value) || 0) })} />
            </div>
          </div>
        </Card>
        <Card>
          <SectionTitle><span className="inline-flex items-center gap-2"><Cpu className="size-5 text-accent" aria-hidden />{t('ai.engine')}</span></SectionTitle>
          <div className="flex flex-col gap-4">
            <div role="radiogroup" aria-label={t('ai.engine')} className="grid gap-3 sm:grid-cols-2">
              {(['local', 'cloud'] as const).map((e) => (
                <button key={e} type="button" role="radio" aria-checked={s.engine === e} onClick={() => set({ engine: e })}
                  className={`flex flex-col gap-1 rounded-xl border p-4 text-left transition ${s.engine === e ? 'border-accent bg-accent/10' : 'border-line hover:bg-tint/5'}`}>
                  <span className="inline-flex items-center gap-2 font-medium text-ink">
                    {e === 'local' ? <Cpu className="size-4" aria-hidden /> : <Cloud className="size-4" aria-hidden />}{t(`ai.engines.${e}`)}
                  </span>
                  <span className="text-xs text-muted">{t(`ai.engines.${e}Hint`)}</span>
                </button>
              ))}
            </div>
            {s.engine === 'local' ? (
              <>
                {!data.llmInstalled && <Alert tone="warn">{t('ai.notInstalled')}</Alert>}
                <Select name="llm" label={t('ai.llm')} value={s.llm} onChange={(e) => set({ llm: e.target.value })}
                  hint={llm ? t('ai.ram', { gb: Math.round(llm.ram / 2 ** 30) }) : undefined}>
                  {data.llms.map((m) => <option key={m.id} value={m.id}>{m.name}</option>)}
                </Select>
                <Items items={data.llms} onDownload={download} onRemove={(i) => remove(i.id, i.name)} />
                <p className="text-xs text-subtle">{t('ai.localHint')}</p>
              </>
            ) : (
              <>
                <Select name="cloudModel" label={t('ai.cloudModel')} value={s.cloudModel} onChange={(e) => set({ cloudModel: e.target.value })}>
                  {data.cloudModels.map((m) => <option key={m} value={m}>{t(`ai.models.${m}`, { defaultValue: m })}</option>)}
                </Select>
                <div className="flex flex-col gap-2 rounded-xl border border-line bg-surface-2/60 p-4">
                  <span className="inline-flex items-center gap-2 text-sm font-medium text-ink"><KeyRound className="size-4 text-accent" aria-hidden />{t('ai.key')}</span>
                  <p className="text-xs text-muted">{data.keySet ? t('ai.keySet', { hint: data.keyHint }) : t('ai.keyNone')}</p>
                  <div className="flex flex-wrap items-end gap-2">
                    <div className="min-w-56 flex-1">
                      <Input label={t('ai.keyNew')} type="password" autoComplete="off" value={key} placeholder="sk-ant-…" onChange={(e) => setKey(e.target.value)} />
                    </div>
                    <Button variant="secondary" busy={busy} disabled={!key.trim()} onClick={() => saveKey(key)}>{t('ai.keySave')}</Button>
                    {data.keySet && <Button variant="secondary" busy={busy} onClick={() => saveKey('')}>{t('ai.keyRemove')}</Button>}
                  </div>
                </div>
                <p className="text-xs text-subtle">{t('ai.cloudHint')}</p>
              </>
            )}
            <div className="flex flex-col gap-3 rounded-xl border border-line p-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <span className="text-sm text-muted">{t('ai.testIntro')}</span>
                <Button variant="secondary" busy={testing} onClick={runTest}><FlaskConical className="size-4" aria-hidden />{t('ai.test')}</Button>
              </div>
              <ErrorMessage error={testError} />
              {test && (
                <>
                  <SummaryView summary={test.summary} />
                  <p className="text-xs text-subtle">{t('ai.testDone', { engine: test.engine, seconds: test.seconds.toFixed(1) })}</p>
                </>
              )}
            </div>
          </div>
        </Card>
      </div>
    </>
  )
}
