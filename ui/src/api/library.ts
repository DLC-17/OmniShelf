import { request } from './client'

export type MediaType = 'TV' | 'BOOK' | 'GAME' | 'MOVIE' | 'MUSIC'

/** All statuses across every media type; per-type validity is enforced server-side. */
export type ItemStatus =
  | 'WATCHING'
  | 'READING'
  | 'PLAYING'
  | 'LISTENING'
  | 'PLAN_TO'
  | 'COMPLETED'
  | 'STOPPED'

export const TV_STATUSES: ItemStatus[] = ['WATCHING', 'PLAN_TO', 'COMPLETED', 'STOPPED']
export const BOOK_STATUSES: ItemStatus[] = ['READING', 'PLAN_TO', 'COMPLETED', 'STOPPED']
export const GAME_STATUSES: ItemStatus[] = ['PLAYING', 'PLAN_TO', 'COMPLETED', 'STOPPED']
export const MOVIE_STATUSES: ItemStatus[] = ['WATCHING', 'PLAN_TO', 'COMPLETED', 'STOPPED']
export const MUSIC_STATUSES: ItemStatus[] = ['LISTENING', 'PLAN_TO', 'COMPLETED', 'STOPPED']

/**
 * Fixed ownership-format option set for games (multi-select). The server
 * validates against the same set; #11 adds a music equivalent (Vinyl, CD).
 */
export const GAME_OWNERSHIP: string[] = ['Physical', 'GOG']

/** Fixed ownership-format option set for music (multi-select: Vinyl, CD). */
export const MUSIC_OWNERSHIP: string[] = ['Vinyl', 'CD']

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
  /** Internal show id for TV items (0 for books); used by the episode picker. */
  showId: number
  /** Books only. */
  authors: string
  pageCount: number
  description: string
  /** Games only. */
  platform: string
  /** Music only. */
  artist: string
  year: number
  /** Source-derived tags/keywords (TMDB/IGDB/OpenLibrary); [] when none. */
  tags: string[]
  /** User-selected ownership formats (games: Physical/GOG; music: Vinyl/CD); [] when none. */
  ownership: string[]
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

/**
 * Replaces the ownership formats on a tracked item (multi-select; pass the full
 * set). Returns the server-normalized set in canonical order.
 */
export function updateOwnership(id: number, formats: string[]): Promise<string[]> {
  return request<{ ownership: string[] }>(`/api/items/${id}/ownership`, {
    method: 'PUT',
    body: { formats },
  }).then((r) => r.ownership)
}
