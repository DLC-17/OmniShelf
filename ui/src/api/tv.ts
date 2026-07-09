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

/** One row of the episode picker: episode fields plus the caller's watch state. */
export interface EpisodeWatchState extends Episode {
  watched: boolean
  /** RFC3339 timestamp, or null when unwatched. */
  watchedAt: string | null
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

/** Media type of an upcoming release, one per tab on the Upcoming section. */
export type UpcomingMediaType = 'tv' | 'movies' | 'games' | 'books'

/** One soon-to-release item on an Upcoming tab. */
export interface UpcomingItem {
  /** "TV" | "MOVIE" — the concrete media type of the row. */
  type: string
  title: string
  /** Relative path under /images; empty string means "use the placeholder". */
  posterPath: string
  /** "YYYY-MM-DD" release/air date. */
  date: string
  /** TV: "S04E01 · Name"; movies: "". */
  detail: string
}

/** GET /api/upcoming groups upcoming releases by media type, one array per tab. */
export interface UpcomingByType {
  tv: UpcomingItem[]
  movies: UpcomingItem[]
  games: UpcomingItem[]
  books: UpcomingItem[]
}

export function fetchUpcoming(): Promise<UpcomingByType> {
  return request<UpcomingByType>('/api/upcoming')
}

/** Recency bucket for the Up Next dashboard toggle. */
export type UpNextFilter = 'recent' | 'stale' | 'unstarted'

export async function fetchUpNext(filter: UpNextFilter = 'recent'): Promise<UpNextEntry[]> {
  const res = await request<{ items: UpNextEntry[] }>(`/api/tv/up-next?filter=${filter}`)
  return res.items
}

/** All episodes of a show with the caller's per-episode watched state. */
export async function fetchEpisodes(showId: number): Promise<EpisodeWatchState[]> {
  const res = await request<{ episodes: EpisodeWatchState[] }>(`/api/tv/shows/${showId}/episodes`)
  return res.episodes
}

/** Marks the episode watched; returns the show's new next-up episode (null when none). */
export async function markWatched(episodeId: number): Promise<Episode | null> {
  const res = await request<{ nextUp: Episode | null }>(`/api/tv/episodes/${episodeId}/watch`, {
    method: 'POST',
  })
  return res.nextUp
}

/** Re-stamps an episode as watched now (rewatch); returns the new next-up episode. */
export async function rewatchEpisode(episodeId: number): Promise<Episode | null> {
  const res = await request<{ nextUp: Episode | null }>(`/api/tv/episodes/${episodeId}/rewatch`, {
    method: 'POST',
  })
  return res.nextUp
}

/** Marks this episode and every earlier aired episode watched; returns the new next-up. */
export async function watchThroughEpisode(episodeId: number): Promise<Episode | null> {
  const res = await request<{ nextUp: Episode | null }>(
    `/api/tv/episodes/${episodeId}/watch-through`,
    { method: 'POST' },
  )
  return res.nextUp
}

/** Marks every aired episode of a whole season watched; returns the new next-up. */
export async function watchSeason(showId: number, season: number): Promise<Episode | null> {
  const res = await request<{ nextUp: Episode | null }>(
    `/api/tv/shows/${showId}/seasons/${season}/watch`,
    { method: 'POST' },
  )
  return res.nextUp
}

/** Removes the watch mark; returns the show's new next-up episode (null when none). */
export async function unmarkWatched(episodeId: number): Promise<Episode | null> {
  const res = await request<{ nextUp: Episode | null }>(`/api/tv/episodes/${episodeId}/watch`, {
    method: 'DELETE',
  })
  return res.nextUp
}
