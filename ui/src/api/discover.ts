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
