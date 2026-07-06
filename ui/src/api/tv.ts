import { request } from './client'

/** One TMDB search hit from GET /api/tv/search. */
export interface TVSearchResult {
  id: number
  name: string
  overview: string
  firstAirDate: string
  posterPath: string
}

/** Cached show metadata as returned by the TV endpoints. */
export interface Show {
  id: number
  tmdbId: number
  title: string
  /** Relative path under /images; empty string means "use the placeholder". */
  posterPath: string
  status: string
}

export interface Episode {
  id: number
  showId: number
  season: number
  number: number
  title: string
  /** "YYYY-MM-DD", or null when unannounced. */
  airDate: string | null
}

export interface TrackingItemSummary {
  id: number
  type: string
  externalId: string
  title: string
  status: string
}

export interface UpNextEntry {
  show: Show
  episode: Episode
}

export interface AddShowResponse {
  show: Show
  item: TrackingItemSummary
}

export async function searchShows(query: string): Promise<TVSearchResult[]> {
  const res = await request<{ results: TVSearchResult[] }>(
    `/api/tv/search?q=${encodeURIComponent(query)}`,
  )
  return res.results
}

export function addShow(tmdbId: number): Promise<AddShowResponse> {
  return request<AddShowResponse>('/api/tv/shows', { method: 'POST', body: { tmdbId } })
}

export async function fetchUpNext(): Promise<UpNextEntry[]> {
  const res = await request<{ items: UpNextEntry[] }>('/api/tv/up-next')
  return res.items
}

/** Marks the episode watched; returns the show's new next-up episode (null when none). */
export async function markWatched(episodeId: number): Promise<Episode | null> {
  const res = await request<{ nextUp: Episode | null }>(`/api/tv/episodes/${episodeId}/watch`, {
    method: 'POST',
  })
  return res.nextUp
}

/** Removes the watch mark; returns the show's new next-up episode (null when none). */
export async function unmarkWatched(episodeId: number): Promise<Episode | null> {
  const res = await request<{ nextUp: Episode | null }>(`/api/tv/episodes/${episodeId}/watch`, {
    method: 'DELETE',
  })
  return res.nextUp
}
