import { useEffect, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, ChevronDown, ExternalLink, ListOrdered, Loader2, Plus, X } from 'lucide-react'
import { useApi } from '../../components/kit'
import { nextNumber, refreshDestinations, useSaver } from '../../components/pbx'
import { Button, ErrorMessage } from '../../components/ui'
import type { Prompt, Queue } from '../../lib/types'
import { QueueFields, blankQueue, editableQueue } from '../Queues'

/** A collapsible block inside the queue settings (closed by default except the first two). */
function Group({ title, children, open: initial }: { title: string; children: ReactNode; open?: boolean }) {
  const [open, setOpen] = useState(!!initial)
  return (
    <div className="rounded-xl border border-line bg-surface-2/50">
      <button type="button" onClick={() => setOpen(!open)} aria-expanded={open}
        className="flex w-full items-center justify-between gap-2 px-3 py-2.5 text-left text-[13px] font-semibold text-ink">
        {title}<ChevronDown className={`size-4 text-subtle transition ${open ? 'rotate-180' : ''}`} aria-hidden />
      </button>
      {open && <div className="flex flex-col gap-3 border-t border-line px-3 pb-3 pt-3">{children}</div>}
    </div>
  )
}

/**
 * The settings of the queue a step routes to, edited in the IVR editor itself. Saving writes
 * straight to the queue (it applies wherever the queue is used), separately from the menu.
 */
export function QueueSettings({ queueId, prompts, onCreated, onSaved }: {
  queueId: number; prompts: Prompt[]; onCreated: (id: number) => void; onSaved: () => void
}) {
  const { t } = useTranslation()
  const list = useApi<Queue[]>('/pbx/queues')
  const [draft, setDraft] = useState<Queue | null>(null)
  const [base, setBase] = useState('')
  const [creating, setCreating] = useState(false)
  const [saved, setSaved] = useState(false)
  const current = list.data?.find((q) => q.id === queueId)

  // Load the chosen queue (again) when the step points to another one.
  useEffect(() => {
    if (creating) return
    if (current) {
      const q = editableQueue(current)
      setDraft(q)
      setBase(JSON.stringify(q))
    } else {
      setDraft(null)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queueId, list.data, creating])
  useEffect(() => setSaved(false), [queueId])

  const { busy, error, save, setError } = useSaver<Queue>('/pbx/queues', (q) => {
    const fresh = editableQueue(q)
    setDraft(fresh)
    setBase(JSON.stringify(fresh))
    setSaved(true)
    refreshDestinations()
    onSaved()
    if (creating) {
      setCreating(false)
      onCreated(q.id)
    }
    list.reload()
  })

  async function startNew() {
    const number = await nextNumber('queues', (list.data ?? []).map((q) => q.number))
    setError(null)
    setSaved(false)
    setCreating(true)
    const q = blankQueue(number)
    setDraft(q)
    setBase(JSON.stringify(q))
  }
  function cancelNew() {
    setCreating(false)
    setError(null)
  }

  const dirty = !!draft && JSON.stringify(draft) !== base
  const setQ = (fn: (q: Queue) => Queue) => {
    setSaved(false)
    setDraft((q) => (q ? fn(q) : q))
  }
  const group = (title: string, children: ReactNode, id: string) =>
    <Group key={id} title={title} open={creating || id === 'general' || id === 'agents'}>{children}</Group>

  return (
    <div className="flex flex-col gap-3">
      {!creating && (
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="secondary" onClick={startNew}><Plus className="size-4" aria-hidden />{t('ivr.queue.new')}</Button>
          {current && (
            <a href="/queues" target="_blank" rel="noreferrer" className="inline-flex items-center gap-1.5 text-xs text-accent hover:underline">
              <ExternalLink className="size-3.5" aria-hidden />{t('ivr.queue.openPage')}
            </a>
          )}
        </div>
      )}

      {creating && (
        <div className="flex items-start gap-2 rounded-xl border border-accent/40 bg-accent/[0.06] px-3 py-2.5 text-xs text-ink-2">
          <ListOrdered className="mt-0.5 size-4 flex-none text-accent" aria-hidden />
          <span className="flex-1">{t('ivr.queue.creatingHint')}</span>
        </div>
      )}

      {list.loading && !list.data && <span className="inline-flex items-center gap-2 text-sm text-muted"><Loader2 className="size-4 animate-spin" aria-hidden />{t('app.loading')}</span>}
      {!creating && list.data && !current && <p className="text-xs text-subtle">{t('ivr.queue.pickFirst')}</p>}

      {draft && (
        <>
          <QueueFields q={draft} setQ={setQ} compact group={group} prompts={prompts} />
          <ErrorMessage error={error} />
          <div className="sticky bottom-0 -mx-1 flex flex-wrap items-center gap-2 rounded-xl border border-line bg-surface/95 px-3 py-2.5 shadow-lg backdrop-blur">
            <Button busy={busy} disabled={!dirty && !creating} onClick={() => save(draft)}>
              {creating ? <><Plus className="size-4" aria-hidden />{t('ivr.queue.create')}</> : t('ivr.queue.save')}
            </Button>
            {creating && <Button variant="ghost" onClick={cancelNew}><X className="size-4" aria-hidden />{t('common.cancel')}</Button>}
            {!creating && dirty && <Button variant="ghost" onClick={() => setDraft(JSON.parse(base))}>{t('ivr.queue.undo')}</Button>}
            {saved && !dirty && <span className="inline-flex items-center gap-1 text-xs text-ok"><Check className="size-3.5" aria-hidden />{t('ivr.queue.saved')}</span>}
            {!creating && dirty && <span className="text-[11px] text-warn">{t('ivr.queue.unsaved')}</span>}
          </div>
          <p className="text-[11px] text-subtle">{t('ivr.queue.sharedHint')}</p>
        </>
      )}
    </div>
  )
}
