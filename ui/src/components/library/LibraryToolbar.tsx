import { useEffect, useId, useMemo, useRef, useState } from 'react'
import {
  BOOK_STATUSES,
  GAME_STATUSES,
  MOVIE_STATUSES,
  TV_STATUSES,
} from '../../api/library'
import type { ItemStatus, LibraryItem, MediaType } from '../../api/library'

/** A single filterable facet of a library item. */
export type FilterAspect = 'status' | 'author' | 'platform' | 'ownership' | 'tag'

/** Selected values keyed by aspect; an aspect is "active" once it has ≥1 value. */
export type FilterState = Partial<Record<FilterAspect, string[]>>

/** How one aspect reads its values off an item and labels its options. */
interface AspectDef {
  aspect: FilterAspect
  label: string
  /** The (trimmed, non-empty) values this item contributes to the aspect. */
  values: (item: LibraryItem) => string[]
  /** Pretty label for an option value; defaults to the value itself. */
  optionLabel?: (value: string) => string
}

const STATUS_LABELS: Record<ItemStatus, string> = {
  WATCHING: 'Watching',
  READING: 'Reading',
  PLAYING: 'Playing',
  PLAN_TO: 'Not started',
  COMPLETED: 'Completed',
  STOPPED: 'Stopped',
}

const MEDIA_NOUN: Record<MediaType, string> = {
  TV: 'shows',
  BOOK: 'books',
  GAME: 'games',
  MOVIE: 'movies',
}

const splitCsv = (s: string): string[] =>
  s.split(',').map((v) => v.trim()).filter(Boolean)

const STATUS_ASPECT: AspectDef = {
  aspect: 'status',
  label: 'Status',
  values: (i) => [i.status],
  optionLabel: (v) => STATUS_LABELS[v as ItemStatus] ?? v,
}
const AUTHOR_ASPECT: AspectDef = {
  aspect: 'author',
  label: 'Author',
  values: (i) => splitCsv(i.authors),
}
const PLATFORM_ASPECT: AspectDef = {
  aspect: 'platform',
  label: 'Platform',
  values: (i) => (i.platform ? [i.platform] : []),
}
const OWNERSHIP_ASPECT: AspectDef = {
  aspect: 'ownership',
  label: 'Ownership',
  values: (i) => i.ownership ?? [],
}
const TAG_ASPECT: AspectDef = {
  aspect: 'tag',
  label: 'Tag',
  values: (i) => i.tags ?? [],
}

/** Per-media aspects, in dropdown order. Aspects with no values auto-hide. */
const ASPECTS: Record<MediaType, AspectDef[]> = {
  TV: [STATUS_ASPECT, TAG_ASPECT],
  MOVIE: [STATUS_ASPECT, TAG_ASPECT],
  BOOK: [STATUS_ASPECT, AUTHOR_ASPECT, TAG_ASPECT],
  GAME: [STATUS_ASPECT, PLATFORM_ASPECT, OWNERSHIP_ASPECT, TAG_ASPECT],
}

function statusesFor(media: MediaType): ItemStatus[] {
  switch (media) {
    case 'BOOK':
      return BOOK_STATUSES
    case 'GAME':
      return GAME_STATUSES
    case 'MOVIE':
      return MOVIE_STATUSES
    default:
      return TV_STATUSES
  }
}

/** The distinct option values present for an aspect across the loaded items. */
function deriveOptions(items: LibraryItem[], def: AspectDef, media: MediaType): string[] {
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
function itemHaystack(item: LibraryItem): string {
  return [
    item.title,
    item.authors,
    item.platform,
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
      if (!tokens.every((t) => hay.includes(t))) return false
    }
    for (const def of defs) {
      const selected = filters[def.aspect]
      if (!selected || selected.length === 0) continue
      if (!def.values(item).some((v) => selected.includes(v))) return false
    }
    return true
  })
}

interface LibraryToolbarProps {
  media: MediaType
  items: LibraryItem[]
  search: string
  onSearchChange: (value: string) => void
  filters: FilterState
  onFiltersChange: (next: FilterState) => void
}

/**
 * The per-media-type library toolbar: a free-text search over the loaded items
 * plus a collapsed-by-default filter dropdown exposing the aspects that make
 * sense for the current tab (status everywhere; author for books; platform and
 * ownership for games; tags where present). Search and filtering both run
 * client-side against the already-fetched items.
 */
export default function LibraryToolbar({
  media,
  items,
  search,
  onSearchChange,
  filters,
  onFiltersChange,
}: LibraryToolbarProps) {
  const [open, setOpen] = useState(false)
  const wrapRef = useRef<HTMLDivElement>(null)
  const panelId = useId()

  // Aspect groups that actually have options for this tab's items.
  const groups = useMemo(
    () =>
      ASPECTS[media]
        .map((def) => ({ def, options: deriveOptions(items, def, media) }))
        .filter((g) => g.options.length > 0),
    [items, media],
  )

  const activeCount = useMemo(
    () => Object.values(filters).reduce((n, vals) => n + (vals?.length ?? 0), 0),
    [filters],
  )

  // Close the dropdown on outside click or Escape while it is open.
  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  const toggleValue = (aspect: FilterAspect, value: string) => {
    const current = filters[aspect] ?? []
    const next = current.includes(value)
      ? current.filter((v) => v !== value)
      : [...current, value]
    const updated = { ...filters }
    if (next.length === 0) delete updated[aspect]
    else updated[aspect] = next
    onFiltersChange(updated)
  }

  return (
    <div className="library-toolbar">
      <input
        type="search"
        className="library-search"
        placeholder={`Search your ${MEDIA_NOUN[media]}…`}
        aria-label={`Search your ${MEDIA_NOUN[media]}`}
        value={search}
        onChange={(e) => onSearchChange(e.target.value)}
      />
      {groups.length > 0 && (
        <div className="filter-dropdown" ref={wrapRef}>
          <button
            type="button"
            className="filter-trigger"
            aria-expanded={open}
            aria-controls={panelId}
            onClick={() => setOpen((v) => !v)}
          >
            Filters
            {activeCount > 0 && <span className="badge">{activeCount}</span>}
            <span className="show-caret" aria-hidden="true">
              {open ? '▴' : '▾'}
            </span>
          </button>
          {open && (
            <div className="filter-panel" id={panelId} role="group" aria-label="Filters">
              {groups.map(({ def, options }) => (
                <fieldset key={def.aspect} className="filter-group">
                  <legend>{def.label}</legend>
                  {options.map((opt) => (
                    <label key={opt} className="filter-option">
                      <input
                        type="checkbox"
                        checked={(filters[def.aspect] ?? []).includes(opt)}
                        onChange={() => toggleValue(def.aspect, opt)}
                      />
                      <span>{def.optionLabel ? def.optionLabel(opt) : opt}</span>
                    </label>
                  ))}
                </fieldset>
              ))}
              {activeCount > 0 && (
                <button
                  type="button"
                  className="btn-ghost filter-clear"
                  onClick={() => onFiltersChange({})}
                >
                  Clear all
                </button>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
