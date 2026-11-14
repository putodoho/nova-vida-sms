// API client: same-origin fetch with the session cookie and the active entity
// header. Errors become ApiError with the server's code, message and fields.

export class ApiError extends Error {
  status: number
  code: string
  fields: Record<string, string>
  constructor(status: number, code: string, message: string, fields: Record<string, string> = {}) {
    super(message)
    this.status = status
    this.code = code
    this.fields = fields
  }
}

let entityHeader: string | null = null

/** Set by the session provider whenever the active entity changes. */
export function setEntityHeader(id: string | null) {
  entityHeader = id
}

let onUnauthorized: (() => void) | null = null
export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorized = fn
}

type Query = Record<string, string | number | boolean | null | undefined>

function url(path: string, query?: Query) {
  const u = new URL('/api/v1' + path, window.location.origin)
  if (query) {
    for (const [k, v] of Object.entries(query)) {
      if (v !== undefined && v !== null && v !== '') u.searchParams.set(k, String(v))
    }
  }
  return u.toString()
}

async function request<T>(method: string, path: string, body?: unknown, query?: Query, entity?: string | null): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const ent = entity === undefined ? entityHeader : entity
  if (ent) headers['X-Entity-ID'] = ent
  const res = await fetch(url(path, query), {
    method,
    headers,
    credentials: 'same-origin',
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (res.status === 204) return undefined as T
  const text = await res.text()
  const data = text ? JSON.parse(text) : undefined
  if (!res.ok) {
    const e = data?.error ?? {}
    if (res.status === 401 && path !== '/auth/login') onUnauthorized?.()
    throw new ApiError(res.status, e.code ?? 'error', e.message ?? 'Erro inesperado.', e.fields ?? {})
  }
  return data as T
}

export const api = {
  get: <T>(path: string, query?: Query) => request<T>('GET', path, undefined, query),
  post: <T>(path: string, body?: unknown, query?: Query) => request<T>('POST', path, body ?? {}, query),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body ?? {}),
  del: <T>(path: string) => request<T>('DELETE', path),
  /** GET with an explicit entity header ('all' for aggregate reads). */
  getAs: <T>(entity: string | null, path: string, query?: Query) => request<T>('GET', path, undefined, query, entity),
}

/** Fetch a binary document with the entity header and open it in a new tab. */
export async function openDocument(path: string, format: 'pdf' | 'html' = 'pdf', method: 'GET' | 'POST' = 'GET', body?: unknown) {
  const win = window.open('', '_blank')
  const headers: Record<string, string> = {}
  if (entityHeader) headers['X-Entity-ID'] = entityHeader
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const res = await fetch(url(path, { format }), {
    method,
    headers,
    credentials: 'same-origin',
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    win?.close()
    const data = await res.json().catch(() => ({}))
    throw new ApiError(res.status, data?.error?.code ?? 'error', data?.error?.message ?? 'Não foi possível abrir o documento.')
  }
  const blob = await res.blob()
  const href = URL.createObjectURL(blob)
  if (win) win.location.href = href
  else window.location.href = href
}

/** Download a report as .xlsx. */
export async function downloadXlsx(path: string, query: Query, filename: string) {
  const headers: Record<string, string> = {}
  if (entityHeader) headers['X-Entity-ID'] = entityHeader
  const res = await fetch(url(path, { ...query, format: 'xlsx' }), { headers, credentials: 'same-origin' })
  if (!res.ok) throw new ApiError(res.status, 'error', 'Não foi possível exportar.')
  const blob = await res.blob()
  const a = document.createElement('a')
  a.href = URL.createObjectURL(blob)
  a.download = filename
  a.click()
  URL.revokeObjectURL(a.href)
}
