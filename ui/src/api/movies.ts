import { request } from './client'
import type { TrackingItemSummary } from './tv'

/** One TMDB movie search hit from GET /api/movies/search. */
export interface MovieSearchResult {
  id: number
  title: string
  overview: string
  releaseDate: string
  posterPath: string
}

/** Cached movie metadata as returned by the movie endpoints. */
export interface Movie {
  id: number
  tmdbId: number
  title: string
  /** Relative path under /images; empty string means "use the placeholder". */
  posterPath: string
  overview: string
  releaseDate: string
}

export interface AddMovieResponse {
  movie: Movie
  item: TrackingItemSummary
}

export async function searchMovies(query: string): Promise<MovieSearchResult[]> {
  const res = await request<{ results: MovieSearchResult[] }>(
    `/api/movies/search?q=${encodeURIComponent(query)}`,
  )
  return res.results
}

export function addMovie(tmdbId: number): Promise<AddMovieResponse> {
  return request<AddMovieResponse>('/api/movies', { method: 'POST', body: { tmdbId } })
}
