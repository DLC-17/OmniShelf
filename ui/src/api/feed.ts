import { request } from './client'

/** Social visibility endpoints: activity feed + member shelves. */

export interface FeedUser {
  id: number
  username: string
}

export interface FeedMedia {
  type: 'TV' | 'BOOK'
  title: string
  /** TMDB ID (TV) or ISBN-13 (BOOK), as a string. */
  id: string
}

export interface FeedEntry {
  user: FeedUser
  /** Human-readable phrasing, produced server-side ("watched S01E02 of ..."). */
  action: string
  media: FeedMedia
  timestamp: string
}

export interface FeedPage {
  entries: FeedEntry[]
  /**
   * Opaque composite cursor. Pass back verbatim as `before` for the next page
   * — the server guarantees no duplicates or gaps. Null on the last page.
   */
  nextBefore: string | null
}

export function fetchFeed(before: string | null, limit = 20): Promise<FeedPage> {
  const params = new URLSearchParams({ limit: String(limit) })
  if (before !== null) {
    params.set('before', before)
  }
  return request<FeedPage>(`/api/feed?${params.toString()}`)
}

export interface MemberCounts {
  tv: number
  books: number
}

export interface Member {
  id: number
  username: string
  counts: MemberCounts
}

export function fetchMembers(): Promise<Member[]> {
  return request<Member[]>('/api/users')
}

export type LibraryTypeFilter = 'TV' | 'BOOK'
export type LibraryStatusFilter = 'WATCHING' | 'READING' | 'COMPLETED' | 'PLAN_TO'

/** Same shape as the owner's library items. */
export interface MemberLibraryItem {
  id: number
  type: 'TV' | 'BOOK'
  externalId: string
  title: string
  status: string
  progress: number
  updatedAt: string
}

export interface MemberLibraryFilters {
  type?: LibraryTypeFilter
  status?: LibraryStatusFilter
}

export function fetchMemberLibrary(
  userId: string,
  filters: MemberLibraryFilters = {},
): Promise<MemberLibraryItem[]> {
  const params = new URLSearchParams()
  if (filters.type !== undefined) {
    params.set('type', filters.type)
  }
  if (filters.status !== undefined) {
    params.set('status', filters.status)
  }
  const qs = params.toString()
  return request<MemberLibraryItem[]>(
    `/api/users/${encodeURIComponent(userId)}/library${qs === '' ? '' : `?${qs}`}`,
  )
}
