import { createContext, useContext, type ReactNode } from 'react'
import { useApi } from '../components/kit'
import type { PortalMe, QueueView, TransferTarget, VoicemailMessage } from '../lib/types'

type Data = {
  me: ReturnType<typeof useApi<PortalMe>>
  targets: ReturnType<typeof useApi<TransferTarget[]>>
  queues: ReturnType<typeof useApi<QueueView[]>>
  voicemail: ReturnType<typeof useApi<VoicemailMessage[]>>
}

const DataContext = createContext<Data | null>(null)

/** What several desktop pages and the tray share, loaded once and kept fresh. */
export function DesktopData({ children }: { children: ReactNode }) {
  const me = useApi<PortalMe>('/portal/me', 30000)
  const targets = useApi<TransferTarget[]>('/portal/targets', 3000)
  const queues = useApi<QueueView[]>('/portal/queues', 5000)
  const voicemail = useApi<VoicemailMessage[]>(me.data?.voicemail ? '/portal/voicemail' : null, 30000)
  return <DataContext.Provider value={{ me, targets, queues, voicemail }}>{children}</DataContext.Provider>
}

export function useDesktop() {
  const c = useContext(DataContext)
  if (!c) throw new Error('useDesktop outside DesktopData')
  return c
}

/** The queues the user can sign in to and out of, and where they are signed in. */
export function myQueues(queues: QueueView[] | null, extId?: number) {
  const mine = (queues ?? []).filter((q) => q.members.some((m) => m.extensionId === extId))
  const member = (q: QueueView) => q.members.find((m) => m.extensionId === extId)
  const dynamic = mine.filter((q) => member(q)?.dynamic)
  const signedIn = mine.filter((q) => member(q)?.loggedIn)
  const paused = mine.some((q) => member(q)?.paused)
  return { mine, dynamic, signedIn, paused, member }
}
