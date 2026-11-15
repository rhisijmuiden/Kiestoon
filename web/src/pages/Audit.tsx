import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Card, ErrorMessage, PageHeader } from '../components/ui'
import { api } from '../lib/api'

type Entry = { id: number; at: string; user: string; action: string; detail: string; ip: string }

export function Audit() {
  const { t, i18n } = useTranslation()
  const [rows, setRows] = useState<Entry[] | null>(null)
  const [error, setError] = useState<unknown>(null)

  useEffect(() => {
    api<Entry[]>('GET', '/system/audit').then(setRows).catch(setError)
  }, [])

  const th = 'pb-2.5 pr-4 text-left text-[11px] font-medium uppercase tracking-[0.08em] text-subtle'
  const td = 'border-t border-white/[0.06] py-2.5 pr-4 text-[13.5px] text-ink-2 whitespace-nowrap'
  return (
    <>
      <PageHeader title={t('audit.title')} subtitle={t('audit.subtitle')} />
      <ErrorMessage error={error} />
      <Card>
        <div className="overflow-x-auto">
          <table className="w-full border-collapse">
            <thead>
              <tr>
                <th className={th}>{t('audit.at')}</th>
                <th className={th}>{t('audit.user')}</th>
                <th className={th}>{t('audit.action')}</th>
                <th className={th}>{t('audit.detail')}</th>
                <th className={th}>{t('audit.ip')}</th>
              </tr>
            </thead>
            <tbody>
              {rows?.map((r) => (
                <tr key={r.id}>
                  <td className={`${td} font-mono`}>{new Date(r.at).toLocaleString(i18n.language)}</td>
                  <td className={td}>{r.user || '–'}</td>
                  <td className={`${td} ${r.action.endsWith('failed') ? 'text-danger' : ''}`}>{r.action}</td>
                  <td className={td}>{r.detail}</td>
                  <td className={`${td} font-mono`}>{r.ip}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {rows?.length === 0 && <p className="py-6 text-center text-sm text-muted">{t('audit.empty')}</p>}
        </div>
      </Card>
    </>
  )
}
