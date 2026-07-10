import { request } from './client'
import type { TrackingItem } from './books'

/** An Album payload from POST /api/music/scan (matches api.albumResponse). */
export interface Album {
  id: number
  externalId: string
  artist: string
  title: string
  year: number
  coverPath: string
  barcode: string
  discogsId: number
  musicBrainzId: string
}

/** One MusicBrainz name-search hit from GET /api/music/search. */
export interface AlbumSearchResult {
  mbid: string
  artist: string
  title: string
  year: number
}

/** Tracking statuses an album may take. */
export type MusicStatus = 'LISTENING' | 'PLAN_TO' | 'COMPLETED' | 'STOPPED'

export interface AddAlbumResponse {
  album: Album
  item: TrackingItem
}

/**
 * Scan a UPC/EAN album barcode via Discogs. A 404 ({error:"music_not_found",
 * barcode}) surfaces as an ApiError with status 404 so the caller can report
 * the miss; a 503 ({error:"music_unconfigured"}) means no Discogs token.
 */
export function scanAlbum(barcode: string): Promise<Album> {
  return request<Album>('/api/music/scan', { method: 'POST', body: { barcode } })
}

/**
 * Track a scanned album. A 409 ({error:"already_tracked"}) surfaces as an
 * ApiError with status 409 so the caller can report it without treating it as
 * a hard failure.
 */
export function trackAlbum(albumId: number, status: MusicStatus): Promise<TrackingItem> {
  return request<TrackingItem>('/api/music/track', {
    method: 'POST',
    body: { albumId, status },
  })
}

/** Search albums by name via MusicBrainz. */
export async function searchMusic(query: string): Promise<AlbumSearchResult[]> {
  const res = await request<{ results: AlbumSearchResult[] }>(
    `/api/music/search?q=${encodeURIComponent(query)}`,
  )
  return res.results
}

/** Add a MusicBrainz-searched album to the library (defaults to LISTENING). */
export function addAlbum(mbid: string, status: MusicStatus = 'LISTENING'): Promise<AddAlbumResponse> {
  return request<AddAlbumResponse>('/api/music', { method: 'POST', body: { mbid, status } })
}
