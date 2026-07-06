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
}

export function updateItem(id: number, patch: UpdateItemPatch): Promise<LibraryItem> {
  return request<LibraryItem>(`/api/items/${id}`, { method: 'PATCH', body: patch })
}

export function deleteItem(id: number): Promise<void> {
  return request<void>(`/api/items/${id}`, { method: 'DELETE' })
}
