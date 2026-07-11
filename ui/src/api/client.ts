/**
 * Shared typed fetch wrapper. All API calls use relative URLs so the SPA works
 * from any origin (LAN IP or Tailscale hostname) and include credentials so the
 * HttpOnly JWT cookie rides along.
 */

export class ApiError extends Error {
  readonly status: number
  readonly code: string
  /**
   * The parsed error envelope, including any endpoint-specific fields beyond
   * {error, message} (e.g. the card scanner's card_not_found setCode). {} when
   * the body was not a JSON object.
   */
  readonly details: Record<string, unknown>

  constructor(status: number, code: string, message: string, details: Record<string, unknown> = {}) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.details = details
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

/**
 * Parses the standard error envelope ({"error": "code", "message": "text"},
 * plus any endpoint-specific extras) off a failed response, tolerating
 * non-JSON bodies (e.g. proxy error pages).
 */
async function parseErrorEnvelope(
  res: Response,
): Promise<{ code: string; message: string; details: Record<string, unknown> }> {
  let code = 'unknown_error'
  let message = `Request failed with status ${res.status}`
  let details: Record<string, unknown> = {}
  try {
    const envelope: unknown = await res.json()
    if (envelope !== null && typeof envelope === 'object' && !Array.isArray(envelope)) {
      details = envelope as Record<string, unknown>
      if (typeof details.error === 'string') code = details.error
      if (typeof details.message === 'string') message = details.message
    }
  } catch {
    // Non-JSON error body; keep the fallback values.
  }
  return { code, message, details }
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

  const { code, message, details } = await parseErrorEnvelope(res)

  if (res.status === 401 && unauthorizedHandler !== null) {
    unauthorizedHandler()
  }
  throw new ApiError(res.status, code, message, details)
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

  const { code, message, details } = await parseErrorEnvelope(res)

  if (res.status === 401 && !skipUnauthorizedHandler && unauthorizedHandler !== null) {
    unauthorizedHandler()
  }

  throw new ApiError(res.status, code, message, details)
}
