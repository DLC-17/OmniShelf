import type { ItemStatus, LibraryItem, MediaType } from '../api/library'
import {
  BOOK_STATUSES,
  CARD_STATUSES,
  GAME_STATUSES,
  MOVIE_STATUSES,
  MUSIC_STATUSES,
  TV_STATUSES,
} from '../api/library'

/** A single filterable facet of a library item. */
export type FilterAspect = 'status' | 'author' | 'platform' | 'ownership' | 'tag' | 'artist'

/** Selected values keyed by aspect; an aspect is "active" once it has ≥1 value. */
export type FilterState = Partial<Record<FilterAspect, string[]>>

/** How one aspect reads its values off an item and labels its options. */
export interface AspectDef {
  aspect: FilterAspect
  label: string
  /** The (trimmed, non-empty) values this item contributes to the aspect. */
  values: (item: LibraryItem) => string[]
  /** Pretty label for an option value; defaults to the value itself. */
  optionLabel?: (value: string) => string
}

export const STATUS_LABELS: Record<ItemStatus, string> = {
  WATCHING: 'Watching',
  READING: 'Reading',
  PLAYING: 'Playing',
  LISTENING: 'Listening',
  OWNED: 'Owned',
  PLAN_TO: 'Not started',
  COMPLETED: 'Completed',
  STOPPED: 'Stopped',
}

export const MEDIA_NOUN: Record<MediaType, string> = {
  TV: 'shows',
  BOOK: 'books',
  GAME: 'games',
  MOVIE: 'movies',
  MUSIC: 'albums',
  CARD: 'cards',
}

export const splitCsv = (s: string): string[] =>
  s.split(',').map((v) => v.trim()).filter(Boolean)

export const STATUS_ASPECT: AspectDef = {
  aspect: 'status',
  label: 'Status',
  values: (i) => [i.status],
  optionLabel: (v) => STATUS_LABELS[v as ItemStatus] ?? v,
}
export const AUTHOR_ASPECT: AspectDef = {
  aspect: 'author',
  label: 'Author',
  values: (i) => splitCsv(i.authors),
}
export const PLATFORM_ASPECT: AspectDef = {
  aspect: 'platform',
  label: 'Platform',
  values: (i) => (i.platform ? [i.platform] : []),
}
export const OWNERSHIP_ASPECT: AspectDef = {
  aspect: 'ownership',
  label: 'Ownership',
  values: (i) => i.ownership ?? [],
}
export const TAG_ASPECT: AspectDef = {
  aspect: 'tag',
  label: 'Tag',
  values: (i) => i.tags ?? [],
}
export const ARTIST_ASPECT: AspectDef = {
  aspect: 'artist',
  label: 'Artist',
  values: (i) => (i.artist ? [i.artist] : []),
}

/** Per-media aspects, in dropdown order. Aspects with no values auto-hide. */
export const ASPECTS: Record<MediaType, AspectDef[]> = {
  TV: [STATUS_ASPECT, TAG_ASPECT],
  MOVIE: [STATUS_ASPECT, TAG_ASPECT],
  BOOK: [STATUS_ASPECT, AUTHOR_ASPECT, TAG_ASPECT],
  GAME: [STATUS_ASPECT, PLATFORM_ASPECT, OWNERSHIP_ASPECT, TAG_ASPECT],
  MUSIC: [STATUS_ASPECT, ARTIST_ASPECT, OWNERSHIP_ASPECT, TAG_ASPECT],
  CARD: [STATUS_ASPECT, TAG_ASPECT],
}

export function statusesFor(media: MediaType): ItemStatus[] {
  switch (media) {
    case 'BOOK':
      return BOOK_STATUSES
    case 'GAME':
      return GAME_STATUSES
    case 'MOVIE':
      return MOVIE_STATUSES
    case 'MUSIC':
      return MUSIC_STATUSES
    case 'CARD':
      return CARD_STATUSES
    default:
      return TV_STATUSES
  }
}

/** The distinct option values present for an aspect across the loaded items. */
export function deriveOptions(items: LibraryItem[], def: AspectDef, media: MediaType): string[] {
  const set = new Set<string>()
  for (const it of items) for (const v of def.values(it)) set.add(v)
  const out = [...set]
  if (def.aspect === 'status') {
    const order = statusesFor(media)
    out.sort((a, b) => order.indexOf(a as ItemStatus) - order.indexOf(b as ItemStatus))
  } else {
    out.sort((a, b) => a.localeCompare(b))
  }
  return out
}

/** Lowercased searchable blob for one item: title + key attributes + tags. */
export function itemHaystack(item: LibraryItem): string {
  return [
    item.title,
    item.authors,
    item.platform,
    item.artist,
    STATUS_LABELS[item.status] ?? item.status,
    ...(item.tags ?? []),
    ...(item.ownership ?? []),
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase()
}

/**
 * Filters the loaded library items for a tab by the free-text search (every
 * whitespace token must appear) and the active filters. Values combine OR
 * within an aspect and AND across aspects.
 */
export function applyLibrarySearch(
  items: LibraryItem[],
  search: string,
  filters: FilterState,
  media: MediaType,
): LibraryItem[] {
  const tokens = search.toLowerCase().split(/\s+/).filter(Boolean)
  const defs = ASPECTS[media]
  return items.filter((item) => {
    if (tokens.length > 0) {
      const hay = itemHaystack(item)
      if (!tokens.every((tok) => hay.includes(tok))) return false
    }
    for (const def of defs) {
      const active = filters[def.aspect]
      if (active && active.length > 0) {
        const values = def.values(item)
        if (!values.some((v) => active.includes(v))) return false
      }
    }
    return true
  })
}
