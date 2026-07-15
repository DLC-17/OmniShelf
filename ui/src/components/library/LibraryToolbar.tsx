import { useEffect, useId, useMemo, useRef, useState } from 'react'
import type { LibraryItem, MediaType } from '../../api/library'
import { ASPECTS, deriveOptions, MEDIA_NOUN } from '../../lib/librarySearch'
import type { FilterAspect, FilterState } from '../../lib/librarySearch'

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

  // Top tags for the quick-pill rail (up to 8, sorted by frequency)
  const topTags = useMemo(() => {
    const tagDef = ASPECTS[media].find((d) => d.aspect === 'tag')
    if (!tagDef) return []
    const freq = new Map<string, number>()
    for (const item of items) {
      for (const tag of tagDef.values(item)) {
        freq.set(tag, (freq.get(tag) ?? 0) + 1)
      }
    }
    return [...freq.entries()]
      .sort((a, b) => b[1] - a[1])
      .slice(0, 8)
      .map(([tag]) => tag)
  }, [items, media])

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
            {groups.length === 0 && (
              <p className="filter-empty">No filter options available.</p>
            )}
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
      {topTags.length > 0 && (
        <div className="tag-pill-rail" aria-label="Quick tag filters">
          {topTags.map((tag) => {
            const active = (filters.tag ?? []).includes(tag)
            return (
              <button
                key={tag}
                type="button"
                className={active ? 'tag-pill active' : 'tag-pill'}
                aria-pressed={active}
                onClick={() => toggleValue('tag', tag)}
              >
                {tag}
              </button>
            )
          })}
        </div>
      )}
    </div>
  )
}
