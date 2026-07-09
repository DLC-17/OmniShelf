/**
 * Shared typed fetch wrapper. All API calls use relative URLs so the SPA works
 * from any origin (LAN IP or Tailscale hostname) and include credentials so the
 * HttpOnly JWT cookie rides along.
 */

export class ApiError extends Error {
  readonly status: number
  readonly code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

type UnauthorizedHandler = () => void

let unauthorizedHandler: UnauthorizedHandler | null = null

/** Registered once by the app shell; invoked on any 401 (E12). */
export function setUnauthorizedHandler(handler: UnauthorizedHandler | null): void {
  unauthorizedHandler = handler
}

export interface RequestOptions {
  method?: string
  body?: unknown
  /**
   * Skip the global 401 handler. Used by the auth/me probe, where a 401 simply
   * means "signed out" and must not trigger a redirect loop on the login page.
   */
  skipUnauthorizedHandler?: boolean
}

interface ErrorEnvelope {
  error?: string
  message?: string
}

/**
 * Uploads multipart form data (e.g. a file) to path and returns the parsed JSON
 * response. Unlike `request` it does not set Content-Type — the browser sets
 * the multipart boundary itself — but shares the same 401 handling and error
 * envelope parsing.
 */
export async function requestUpload<T>(path: string, form: FormData, method = 'PUT'): Promise<T> {
  const res = await fetch(path, { method, credentials: 'include', body: form })

  if (res.ok) {
    const text = await res.text()
    return text === '' ? (undefined as T) : (JSON.parse(text) as T)
  }

  let code = 'unknown_error'
  let message = `Request failed with status ${res.status}`
  try {
    const envelope = (await res.json()) as ErrorEnvelope
    if (typeof envelope.error === 'string') code = envelope.error
    if (typeof envelope.message === 'string') message = envelope.message
  } catch {
    // Non-JSON error body; keep the fallback values.
  }

  if (res.status === 401 && unauthorizedHandler !== null) {
    unauthorizedHandler()
  }
  throw new ApiError(res.status, code, message)
}

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', body, skipUnauthorizedHandler = false } = options

  const res = await fetch(path, {
    method,
    credentials: 'include',
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })

  if (res.ok) {
    const text = await res.text()
    if (text === '') {
      return undefined as T
    }
    return JSON.parse(text) as T
  }

  // Parse the standard error envelope: {"error": "code", "message": "text"}.
  let code = 'unknown_error'
  let message = `Request failed with status ${res.status}`
  try {
    const envelope = (await res.json()) as ErrorEnvelope
    if (typeof envelope.error === 'string') code = envelope.error
    if (typeof envelope.message === 'string') message = envelope.message
  } catch {
    // Non-JSON error body (e.g. proxy error page); keep the fallback values.
  }

  if (res.status === 401 && !skipUnauthorizedHandler && unauthorizedHandler !== null) {
    unauthorizedHandler()
  }

  throw new ApiError(res.status, code, message)
}
