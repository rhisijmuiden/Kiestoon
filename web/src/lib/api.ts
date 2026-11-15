export class ApiError extends Error {
  constructor(public status: number, public code: string, public field?: string, public detail?: string) {
    super(code)
  }
}

export async function api<T = unknown>(method: string, path: string, body?: unknown): Promise<T> {
  let res: Response
  try {
    res = await fetch(`/api${path}`, {
      method,
      credentials: 'same-origin',
      headers: method === 'GET' ? {} : { 'Content-Type': 'application/json' },
      body: method === 'GET' ? undefined : JSON.stringify(body ?? {}),
    })
  } catch {
    throw new ApiError(0, 'network')
  }
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    const d = data as { error?: string; field?: string; message?: string }
    throw new ApiError(res.status, d.error ?? 'internal', d.field, d.message)
  }
  return data as T
}

export type User = {
  id: number
  username: string
  displayName: string
  role: 'superadmin' | 'admin' | 'user'
  mfaEnabled: boolean
  language: 'en' | 'nl'
}

export type Me = {
  user: User
  mfaPassed: boolean
  mfaEnrollmentRequired: boolean
  organization: string
}

export type SystemInfo = {
  version: string
  commit: string
  buildDate: string
  channel: 'production' | 'testing'
  hostname: string
  uptime: number
  repo: string
}

export type Release = { version: string; name: string; prerelease: boolean; publishedAt: string; notes: string; url: string }
export type CheckResult = { channel: string; current: string; latest: Release | null; updateAvailable: boolean; downgrade: boolean }
export type UpdateStatus = { state: 'idle' | 'queued' | 'running' | 'success' | 'failed'; version?: string; message?: string }

export const isAdmin = (u?: User) => u?.role === 'admin' || u?.role === 'superadmin'
