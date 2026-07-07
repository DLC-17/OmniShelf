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
