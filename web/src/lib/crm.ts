import { useEffect, useState } from 'react'
import { api } from './api'

export type CrmKind = 'hubspot' | 'salesforce' | 'pipedrive' | 'exact'
export const crmKinds: CrmKind[] = ['hubspot', 'salesforce', 'pipedrive', 'exact']
export const crmNames: Record<CrmKind, string> = { hubspot: 'HubSpot', salesforce: 'Salesforce', pipedrive: 'Pipedrive', exact: 'Exact Online' }
/** Brand colour of each CRM, for its small logo tile (white letter on it). */
export const crmColors: Record<CrmKind, string> = { hubspot: '#FF7A59', salesforce: '#00A1E0', pipedrive: '#1F1F1F', exact: '#E1001A' }

export type CrmItem = { title: string; value?: string; when?: string; url?: string }
export type CrmContact = {
  crm: CrmKind
  id: string
  companyId?: string
  isCompany?: boolean
  name: string
  company?: string
  phone?: string
  email?: string
  owner?: string
  lifecycle?: string
  url?: string
  deals?: CrmItem[]
  tickets?: CrmItem[]
  last?: CrmItem
}
export type CrmResult = { number: string; crm?: CrmKind; contacts: CrmContact[] }

export type CrmConfig = {
  enabled: boolean
  domain?: string
  clientId?: string
  region?: string
  division?: number
  lookupContacts: boolean
  lookupCompanies: boolean
  show: string[]
  logCalls: boolean
  logWhich: 'all' | 'inbound' | 'answered'
  missedTask: boolean
  recordingLink: boolean
  aiSummary: boolean
  createUnknown: boolean
  owners?: Record<string, string>
}
export type CrmSettings = { order: CrmKind[]; deskName: boolean; popupAll: boolean; connectors: Record<CrmKind, CrmConfig> }
export type CrmStatus = {
  kind: CrmKind
  name: string
  configured: boolean
  connected: boolean
  loginExpired: boolean
  account: string
  hasSecret: boolean
  lookups: number
  hits: number
  logged: number
  failed: number
}
export type CrmOverview = { settings: CrmSettings; status: CrmStatus[]; exactRedirect: string; aiCalls: boolean }
export type CrmOwner = { id: string; name: string; email: string }
export type CrmOwnerRow = { id: number; name: string; email: string; ext: string; ownerId: string; match: 'manual' | 'email' | 'none' }
export type CrmLogEntry = { id: number; at: string; crm: CrmKind; action: string; detail: string; ok: boolean; error: string; durationMs: number; callId: number; callState: string }
export type CrmCallInfo = { id: number; crm: CrmKind | ''; state: string; task: boolean; contact: string; company: string; url: string; error?: string }

export type CrmEvent = {
  type: 'ring' | 'answered' | 'stop' | 'ended'
  callId: string
  number?: string
  name?: string
  did?: string
  ext?: string
  result?: CrmResult | null
  at?: string
  talkSec?: number
  loggable?: boolean
}

export type CrmPortalStatus = { active: boolean; crms: CrmKind[]; createIn: CrmKind | '' }

/** Whether a CRM is connected (checked once per page load, then every 5 minutes). */
export function useCrmStatus(enabled = true) {
  const [st, setSt] = useState<CrmPortalStatus | null>(null)
  useEffect(() => {
    if (!enabled) return
    let off = false
    const load = () => api<CrmPortalStatus>('GET', '/portal/crm/status').then((s) => { if (!off) setSt(s) }).catch(() => undefined)
    load()
    const id = window.setInterval(load, 5 * 60_000)
    return () => { off = true; window.clearInterval(id) }
  }, [enabled])
  return st
}

/** Looks a number up in the CRM (for the Windows softphone popup). */
export function useCrmLookup(number: string | null, active: boolean) {
  const [res, setRes] = useState<CrmResult | null>(null)
  useEffect(() => {
    setRes(null)
    if (!number || !active) return
    let off = false
    api<{ result: CrmResult | null }>('GET', `/portal/crm/lookup?number=${encodeURIComponent(number)}`)
      .then((r) => { if (!off) setRes(r.result) })
      .catch(() => undefined)
    return () => { off = true }
  }, [number, active])
  return res
}

export function crmDisplayName(c?: CrmContact) {
  if (!c) return ''
  return c.company && c.company !== c.name ? `${c.name} · ${c.company}` : c.name
}
