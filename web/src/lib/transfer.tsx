import { createContext, useCallback, useContext, useEffect, useRef, useState, type PointerEvent as ReactPointerEvent, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { PhoneForwarded } from 'lucide-react'
import { api } from './api'

// Drag-and-drop call transfer. A call card is the drag source; colleagues, ring groups and queues
// are drop targets (any element with data-xfer-target="<number>"). It uses pointer events, so it
// works with a mouse, a pen and on touch screens. For keyboard users and as a fallback, the
// "Transfer" button on a call starts pick mode: the next target that is clicked or chosen with
// Enter receives the call. Esc cancels.

export type TransferMode = 'blind' | 'attended'
/** portal: the user's own call (blind or attended); reception: any call leg, moved with a redirect. */
export type TransferScope = 'portal' | 'reception'
export type TransferSource = { channel: string; label: string; scope: TransferScope }
export type TransferNotice = { tone: 'ok' | 'danger'; text: string; error?: unknown }

type Ctx = {
  mode: TransferMode
  setMode: (m: TransferMode) => void
  /** The call being dragged or picked, if any. */
  active: TransferSource | null
  dragging: boolean
  hover: string | null
  busy: boolean
  notice: TransferNotice | null
  clearNotice: () => void
  pick: (s: TransferSource | null) => void
  /** Transfers the active (or given) call to the target number. */
  drop: (target: string, targetName: string, source?: TransferSource) => Promise<void>
  /** Pointer handlers for a drag source. */
  dragHandlers: (s: TransferSource) => { onPointerDown: (e: ReactPointerEvent<HTMLElement>) => void }
  onDone?: () => void
}

const TransferContext = createContext<Ctx | null>(null)

const MODE_KEY = 'kiestoon.transferMode'

function storedMode(): TransferMode {
  try {
    return localStorage.getItem(MODE_KEY) === 'attended' ? 'attended' : 'blind'
  } catch {
    return 'blind'
  }
}

function targetAt(x: number, y: number): { number: string; name: string } | null {
  const el = document.elementFromPoint(x, y)?.closest<HTMLElement>('[data-xfer-target]')
  if (!el) return null
  return { number: el.dataset.xferTarget ?? '', name: el.dataset.xferName ?? '' }
}

export function TransferProvider({ children, onDone }: { children: ReactNode; onDone?: () => void }) {
  const { t } = useTranslation()
  const [mode, setModeState] = useState<TransferMode>(storedMode)
  const [active, setActive] = useState<TransferSource | null>(null)
  const [dragging, setDragging] = useState(false)
  const [hover, setHover] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [notice, setNotice] = useState<TransferNotice | null>(null)
  const ghost = useRef<HTMLDivElement>(null)
  const doneRef = useRef(onDone)
  doneRef.current = onDone

  const setMode = (m: TransferMode) => {
    setModeState(m)
    try {
      localStorage.setItem(MODE_KEY, m)
    } catch {
      /* ignore */
    }
  }

  const drop = useCallback(async (target: string, targetName: string, source?: TransferSource) => {
    const s = source ?? active
    setActive(null)
    setHover(null)
    if (!s || !target) return
    setBusy(true)
    setNotice(null)
    const who = targetName ? `${targetName} (${target})` : target
    try {
      if (s.scope === 'reception') {
        await api('POST', '/reception/transfer', { channel: s.channel, target })
        setNotice({ tone: 'ok', text: t('transfer.done', { caller: s.label, target: who }) })
      } else {
        await api('POST', '/portal/transfer', { channel: s.channel, target, mode })
        setNotice({ tone: 'ok', text: mode === 'blind' ? t('transfer.done', { caller: s.label, target: who }) : t('transfer.consulting', { target: who }) })
      }
    } catch (e) {
      setNotice({ tone: 'danger', text: '', error: e })
    } finally {
      setBusy(false)
      doneRef.current?.()
    }
  }, [active, mode, t])

  const dragHandlers = (s: TransferSource) => ({
    onPointerDown: (e: ReactPointerEvent<HTMLElement>) => {
      if (e.button !== 0 || busy) return
      // Buttons inside the card keep working.
      if ((e.target as HTMLElement).closest('button, a, input, [role="radio"]')) return
      const startX = e.clientX
      const startY = e.clientY
      let started = false
      const el = e.currentTarget
      const id = e.pointerId
      try {
        el.setPointerCapture(id)
      } catch {
        /* ignore */
      }
      const move = (ev: PointerEvent) => {
        if (!started) {
          if (Math.hypot(ev.clientX - startX, ev.clientY - startY) < 6) return
          started = true
          setActive(s)
          setDragging(true)
        }
        ev.preventDefault()
        if (ghost.current) ghost.current.style.transform = `translate(${ev.clientX + 14}px, ${ev.clientY + 14}px)`
        setHover(targetAt(ev.clientX, ev.clientY)?.number ?? null)
      }
      const up = (ev: PointerEvent) => {
        el.removeEventListener('pointermove', move)
        el.removeEventListener('pointerup', up)
        el.removeEventListener('pointercancel', cancel)
        setDragging(false)
        if (!started) return
        const tg = targetAt(ev.clientX, ev.clientY)
        if (tg?.number) drop(tg.number, tg.name, s)
        else {
          setActive(null)
          setHover(null)
        }
      }
      const cancel = () => {
        el.removeEventListener('pointermove', move)
        el.removeEventListener('pointerup', up)
        el.removeEventListener('pointercancel', cancel)
        setDragging(false)
        setActive(null)
        setHover(null)
      }
      el.addEventListener('pointermove', move)
      el.addEventListener('pointerup', up)
      el.addEventListener('pointercancel', cancel)
    },
  })

  useEffect(() => {
    if (!active) return
    const key = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setActive(null)
        setDragging(false)
        setHover(null)
      }
    }
    window.addEventListener('keydown', key)
    return () => window.removeEventListener('keydown', key)
  }, [active])

  useEffect(() => {
    if (notice?.tone !== 'ok') return
    const id = window.setTimeout(() => setNotice(null), 6000)
    return () => window.clearTimeout(id)
  }, [notice])

  const ctx: Ctx = {
    mode, setMode, active, dragging, hover, busy, notice, clearNotice: () => setNotice(null),
    pick: setActive, drop, dragHandlers, onDone,
  }
  return (
    <TransferContext.Provider value={ctx}>
      {children}
      {dragging && active && (
        <div ref={ghost} aria-hidden className="pointer-events-none fixed left-0 top-0 z-50 flex items-center gap-2 rounded-xl border border-accent/50 bg-surface/95 px-3 py-2 text-sm text-ink shadow-2xl backdrop-blur"
          style={{ transform: 'translate(-999px, -999px)' }}>
          <PhoneForwarded className="size-4 text-accent" />
          <span className="font-mono">{active.label}</span>
          {active.scope === 'portal' && <span className="text-xs text-subtle">{t(`transfer.mode.${mode}`)}</span>}
        </div>
      )}
    </TransferContext.Provider>
  )
}

export function useTransfer() {
  const c = useContext(TransferContext)
  if (!c) throw new Error('useTransfer outside TransferProvider')
  return c
}
