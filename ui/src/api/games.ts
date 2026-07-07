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
