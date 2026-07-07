import { request } from './client'

export type MediaType = 'TV' | 'BOOK'

/** All statuses across both media types; per-type validity is enforced server-side. */
export type ItemStatus = 'WATCHING' | 'READING' | 'PLAN_TO' | 'COMPLETED'

export const TV_STATUSES: ItemStatus[] = ['WATCHING', 'COMPLETED', 'PLAN_TO']
export const BOOK_STATUSES: ItemStatus[] = ['READING', 'COMPLETED', 'PLAN_TO']

export interface LibraryItem {
  id: number
  type: MediaType
  externalId: string
  title: string
  status: ItemStatus
  /** Page number; books only (TV progress is derived server-side). */
  progress: number
  /** User's 1–5 self-rating; 0 = unrated. */
  rating: number
  /** Relative /images path for the poster/cover; '' = show a placeholder. */
  artworkPath: string
  /** Books only. */
  authors: string
  pageCount: number
  description: string
  updatedAt: string
}

export interface LibraryFilters {
  type?: MediaType | ''
  status?: ItemStatus | ''
}

export function fetchLibrary(filters: LibraryFilters = {}): Promise<LibraryItem[]> {
  const params = new URLSearchParams()
  if (filters.type) params.set('type', filters.type)
  if (filters.status) params.set('status', filters.status)
  const qs = params.toString()
  return request<LibraryItem[]>(qs === '' ? '/api/library' : `/api/library?${qs}`)
}

export interface UpdateItemPatch {
  status?: ItemStatus
  progress?: number
  rating?: number
}

export function updateItem(id: number, patch: UpdateItemPatch): Promise<LibraryItem> {
  return request<LibraryItem>(`/api/items/${id}`, { method: 'PATCH', body: patch })
}

export function deleteItem(id: number): Promise<void> {
  return request<void>(`/api/items/${id}`, { method: 'DELETE' })
}
