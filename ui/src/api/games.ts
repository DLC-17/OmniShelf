import { request } from './client'
import type { TrackingItem } from './books'

/** A Game payload from POST /api/games/scan (matches api.gameResponse). */
export interface Game {
  id: number
  barcode: string
  title: string
  platform: string
  coverPath: string
  igdbId: number
  description: string
}

/** One IGDB name-search hit from GET /api/games/search. */
export interface GameSearchResult {
  igdbId: number
  name: string
  /** First release year, or 0 when IGDB has no date. */
  year: number
  /** IGDB cover image_id for the cover proxy; '' when none. */
  coverImageId: string
}

/** POST /api/games/add response: the shared game plus the new tracking item. */
export interface AddGameResponse {
  game: Game
  item: TrackingItem
}

/** Tracking statuses a game may take. */
export type GameStatus = 'PLAYING' | 'PLAN_TO' | 'COMPLETED' | 'STOPPED'

/**
 * Scan a UPC/EAN game barcode via ScanDex. A 404 ({error:"game_not_found",
 * barcode}) surfaces as an ApiError with status 404 so the caller can report
 * the miss without treating it as a hard failure.
 */
export function scanGame(barcode: string): Promise<Game> {
  return request<Game>('/api/games/scan', { method: 'POST', body: { barcode } })
}

/**
 * Track a scanned game. A 409 ({error:"already_tracked"}) surfaces as an
 * ApiError with status 409 so the caller can report it without treating it as
 * a hard failure.
 */
export function trackGame(gameId: number, status: GameStatus): Promise<TrackingItem> {
  return request<TrackingItem>('/api/games/track', {
    method: 'POST',
    body: { gameId, status },
  })
}

/** Search IGDB for games by title (add-by-name flow). */
export async function searchGames(query: string): Promise<GameSearchResult[]> {
  const res = await request<{ results: GameSearchResult[] }>(
    `/api/games/search?q=${encodeURIComponent(query)}`,
  )
  return res.results
}

/**
 * Add a game by its IGDB id (a name-search pick). Defaults to the PLAN_TO
 * watchlist. A 409 ({error:"already_tracked"}) surfaces as an ApiError with
 * status 409 so the caller can report it without treating it as a hard failure.
 */
export function addGameByIgdb(igdbId: number, status: GameStatus = 'PLAN_TO'): Promise<AddGameResponse> {
  return request<AddGameResponse>('/api/games/add', {
    method: 'POST',
    body: { igdbId, status },
  })
}
