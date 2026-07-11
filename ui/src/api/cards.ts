import { request, requestUpload } from './client'
import type { TrackingItem } from './books'

/** Which trading-card game an identified card belongs to. */
export type CardGame = 'YUGIOH' | 'POKEMON'

/** A Card payload from POST /api/cards/scan (matches api.cardResponse). */
export interface Card {
  game: CardGame
  /** Source-scoped id, e.g. "ygo:LOB-001" or "ptcg:base1-4". */
  externalId: string
  name: string
  cardType: string
  race: string
  /** Illustrator credit (Pokémon); '' when the source has none. */
  artist: string
  setCode: string
  setName: string
  /** Market price in USD; 0 when the source has no price. */
  price: number
  /**
   * Cached artwork path relative to the images root (e.g. "card/ygo_LOB-001.jpg",
   * served at /images/<coverPath>); '' when no art was cached.
   */
  coverPath: string
}

/** POST /api/cards/add response: the shared card plus the new tracking item. */
export interface AddCardResponse {
  card: Card
  item: TrackingItem
}

/**
 * Identify a photographed trading card. The JPEG is uploaded as multipart form
 * data under the `card_image` key. Identification misses surface as ApiError
 * with codes no_text_detected / card_not_found (404, the latter with optional
 * game/setCode in `details`) or unsupported_card (422) so the caller can
 * explain the miss and offer a retry.
 */
export function scanCard(image: Blob): Promise<Card> {
  const form = new FormData()
  form.append('card_image', image, 'card.jpg')
  return requestUpload<Card>('/api/cards/scan', form, 'POST')
}

/**
 * Add an identified card to the shelf. When status is omitted the server
 * defaults to OWNED. A 409 ({error:"already_tracked"}) surfaces as an ApiError
 * with status 409 so the caller can report it without treating it as a hard
 * failure.
 */
export function addCard(card: Card, status?: string): Promise<AddCardResponse> {
  const body: Record<string, unknown> = {
    game: card.game,
    externalId: card.externalId,
    name: card.name,
    cardType: card.cardType,
    race: card.race,
    artist: card.artist,
    setCode: card.setCode,
    setName: card.setName,
    price: card.price,
    // Echo the scan-cached artwork so the shared Card row keeps its art; the
    // server only accepts paths inside its card/ cache namespace.
    coverPath: card.coverPath,
  }
  if (status !== undefined) body.status = status
  return request<AddCardResponse>('/api/cards/add', { method: 'POST', body })
}
