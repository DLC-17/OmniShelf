import { request } from './client'

/** One Discover suggestion, tagged with the tracked show it came from. */
export interface DiscoverItem {
  tmdbId: number
  title: string
  overview: string
  /** Raw TMDB poster path; the UI thumbnails it from the TMDB CDN. */
  posterPath: string
  firstAirDate: string
  /** Title of the tracked show this was suggested from. */
  suggestedBy: string
}

export async function fetchDiscover(): Promise<DiscoverItem[]> {
  const res = await request<{ items: DiscoverItem[] }>('/api/tv/discover')
  return res.items
}

/** Hide a suggestion so it is not recommended again. */
export function rejectRec(tmdbId: number): Promise<void> {
  return request<void>('/api/tv/discover/reject', { method: 'POST', body: { tmdbId } })
}

/** One movie Discover suggestion; movies use releaseDate where TV uses firstAirDate. */
export interface MovieDiscoverItem {
  tmdbId: number
  title: string
  overview: string
  posterPath: string
  releaseDate: string
  suggestedBy: string
}

export async function fetchMovieDiscover(): Promise<MovieDiscoverItem[]> {
  const res = await request<{ items: MovieDiscoverItem[] }>('/api/movies/discover')
  return res.items
}

/** Hide a movie suggestion so it is not recommended again. */
export function rejectMovieRec(tmdbId: number): Promise<void> {
  return request<void>('/api/movies/discover/reject', { method: 'POST', body: { tmdbId } })
}

/**
 * One game Discover suggestion (IGDB "similar games"). coverPath is a relative
 * /images path — IGDB covers are cached server-side, never hotlinked.
 */
export interface GameDiscoverItem {
  igdbId: number
  title: string
  /** First release year, or 0 when IGDB has no date. */
  year: number
  /** Relative path under /images; empty string means "use the placeholder". */
  coverPath: string
  /** Title of the tracked game this was suggested from. */
  suggestedBy: string
}

export async function fetchGameDiscover(): Promise<GameDiscoverItem[]> {
  const res = await request<{ items: GameDiscoverItem[] }>('/api/games/discover')
  return res.items
}

/** Hide a game suggestion so it is not recommended again. */
export function rejectGameRec(igdbId: number): Promise<void> {
  return request<void>('/api/games/discover/reject', { method: 'POST', body: { igdbId } })
}

/**
 * One book Discover suggestion (author/subject heuristic). Identity is the
 * OpenLibrary workKey; coverPath is a relative /images path (never hotlinked).
 */
export interface BookDiscoverItem {
  workKey: string
  title: string
  authors: string
  /** First publication year, or 0 when OpenLibrary has no date. */
  year: number
  /** Relative path under /images; empty string means "use the placeholder". */
  coverPath: string
  /** "books by <author>" or an OpenLibrary subject. */
  suggestedBy: string
}

export async function fetchBookDiscover(): Promise<BookDiscoverItem[]> {
  const res = await request<{ items: BookDiscoverItem[] }>('/api/books/discover')
  return res.items
}

/** Hide a book suggestion so it is not recommended again. */
export function rejectBookRec(workKey: string): Promise<void> {
  return request<void>('/api/books/discover/reject', { method: 'POST', body: { workKey } })
}
