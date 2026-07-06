import { useState } from 'react'
import { ApiError } from '../api/client'
import type { ItemStatus, MediaType } from '../api/library'
import LibraryRow from '../components/tv/LibraryRow'
import { useLibrary } from '../hooks/useLibrary'

const TYPE_OPTIONS: MediaType[] = ['TV', 'BOOK']
const STATUS_OPTIONS: ItemStatus[] = ['WATCHING', 'READING', 'PLAN_TO', 'COMPLETED']

/**
 * Library shelf: the user's tracked items with type/status filters
 * and inline status/progress editing plus confirm-gated delete per row.
 */
export default function Library() {
  const [type, setType] = useState<MediaType | ''>('')
  const [status, setStatus] = useState<ItemStatus | ''>('')

  const library = useLibrary({ type, status })

  return (
    <section>
      <h1>Library</h1>

      <div className="toolbar">
        <label className="field">
          <span>Type</span>
          <select
            aria-label="Filter by type"
            value={type}
            onChange={(e) => setType(e.target.value as MediaType | '')}
          >
            <option value="">All</option>
            {TYPE_OPTIONS.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        </label>
        <label className="field">
          <span>Status</span>
          <select
            aria-label="Filter by status"
            value={status}
            onChange={(e) => setStatus(e.target.value as ItemStatus | '')}
          >
            <option value="">All</option>
            {STATUS_OPTIONS.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </label>
      </div>

      {library.isPending && <p className="muted">Loading your library…</p>}
      {library.isError && (
        <p role="alert" className="alert">
          {library.error instanceof ApiError
            ? library.error.message
            : 'Could not load your library. Try refreshing.'}
        </p>
      )}

      {library.data !== undefined && library.data.length === 0 && (
        <p className="empty">
          No items match these filters. Add a show from Up Next or scan a book to start building your
          shelf.
        </p>
      )}

      {library.data !== undefined && library.data.length > 0 && (
        <ul className="list">
          {library.data.map((item) => (
            <LibraryRow key={item.id} item={item} />
          ))}
        </ul>
      )}
    </section>
  )
}
