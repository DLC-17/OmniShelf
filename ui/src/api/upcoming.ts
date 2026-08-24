import { request } from './client'

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
