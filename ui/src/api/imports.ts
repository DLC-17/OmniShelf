import { ApiError, request } from './client'

/** TV Time legacy import endpoints. */

export type ImportJobStatus = 'PENDING' | 'RUNNING' | 'DONE' | 'FAILED'

export interface ImportJob {
  jobId: number
  status: ImportJobStatus
  processed: number
  total: number
  /** Malformed data rows skipped during processing (notes import: rows without a review). */
  skipped: number
  /** Book notes created (Goodreads notes import only; 0 otherwise). */
  notesCreated: number
  /** Title being imported right now (present only while active). */
  current?: string
  /**
   * Show titles that could not be matched on TMDB, or — for the notes import —
   * reviewed books the user does not track.
   */
  unresolved: string[]
  error?: string
}

/** True while the backend is still working the job and polling should continue. */
export function isImportJobActive(status: ImportJobStatus): boolean {
  return status === 'PENDING' || status === 'RUNNING'
}

interface ErrorEnvelope {
  error?: string
  message?: string
}

/**
 * Uploads TV Time export files (CSVs or a zip). Uses fetch directly rather
 * than the shared `request` wrapper because that wrapper JSON-encodes bodies;
 * multipart uploads need a raw FormData body with a browser-set boundary.
 * Error handling mirrors the wrapper: the standard envelope becomes ApiError.
 */
export async function uploadImport(files: File[]): Promise<{ jobId: number }> {
  const form = new FormData()
  for (const file of files) {
    form.append('files', file, file.name)
  }

  const res = await fetch('/api/tv/import', {
    method: 'POST',
    credentials: 'include',
    body: form,
  })
  if (res.ok) {
    return (await res.json()) as { jobId: number }
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
  throw new ApiError(res.status, code, message)
}

/**
 * Uploads a Goodreads library export to the notes import, which maps each
 * row's "My Review" onto the matching tracked book as a note. Same multipart
 * mechanics as uploadImport; only the endpoint differs.
 */
export async function uploadNotesImport(files: File[]): Promise<{ jobId: number }> {
  const form = new FormData()
  for (const file of files) {
    form.append('files', file, file.name)
  }

  const res = await fetch('/api/books/notes/import', {
    method: 'POST',
    credentials: 'include',
    body: form,
  })
  if (res.ok) {
    return (await res.json()) as { jobId: number }
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
  throw new ApiError(res.status, code, message)
}

export function fetchImportJob(jobId: number): Promise<ImportJob> {
  return request<ImportJob>(`/api/tv/import/${jobId}`)
}

/** Maps unresolved titles to TMDB show IDs; returns the updated job. */
export function resolveImport(
  jobId: number,
  mappings: Record<string, number>,
): Promise<ImportJob> {
  return request<ImportJob>(`/api/tv/import/${jobId}/resolve`, {
    method: 'POST',
    body: { mappings },
  })
}
