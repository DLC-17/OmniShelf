import { useState } from 'react'
import { ApiError } from '../api/client'
import type { LibraryItem, MediaType } from '../api/library'
import LibraryDetail from '../components/library/LibraryDetail'
import Poster from '../components/tv/Poster'
import { useLibrary } from '../hooks/useLibrary'

type MediaTab = MediaType | 'MOVIE'

const TABS: { value: MediaTab; label: string }[] = [
  { value: 'TV', label: 'TV Shows' },
  { value: 'BOOK', label: 'Books' },
  { value: 'MOVIE', label: 'Movies' },
]

/**
 * Library shelf: a cover-art grid toggled between TV shows, books, and movies
 * (coming soon). Clicking a cover opens the item's detail — summary, author,
 * length, a self-rating, inline status/progress editing, and delete.
 */
export default function Library() {
  const [media, setMedia] = useState<MediaTab>('TV')
  const [selectedId, setSelectedId] = useState<number | null>(null)

  const isMovie = media === 'MOVIE'
  const library = useLibrary({ type: isMovie ? '' : media }, !isMovie)

  const items: LibraryItem[] = library.data ?? []
  const selected = items.find((i) => i.id === selectedId) ?? null

  return (
    <section>
      <h1>Library</h1>

      <div className="tabs" role="tablist" aria-label="Media type">
        {TABS.map((tab) => (
          <button
            key={tab.value}
            type="button"
            role="tab"
            aria-selected={media === tab.value}
            className={media === tab.value ? 'tab active' : 'tab'}
            onClick={() => {
              setMedia(tab.value)
              setSelectedId(null)
            }}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {isMovie && (
        <p className="empty">Movies are coming soon — you’ll be able to track films here.</p>
      )}

      {!isMovie && library.isPending && <p className="muted">Loading your library…</p>}
      {!isMovie && library.isError && (
        <p role="alert" className="alert">
          {library.error instanceof ApiError
            ? library.error.message
            : 'Could not load your library. Try refreshing.'}
        </p>
      )}

      {!isMovie && library.data !== undefined && items.length === 0 && (
        <p className="empty">
          No items match these filters. Add a show from Up Next or scan a book to start building your
          shelf.
        </p>
      )}

      {!isMovie && items.length > 0 && (
        <ul className="cover-grid">
          {items.map((item) => (
            <li key={item.id}>
              <button
                type="button"
                className="cover-tile"
                aria-label={`Open ${item.title}`}
                onClick={() => setSelectedId(item.id)}
              >
                <Poster posterPath={item.artworkPath} title={item.title} width={140} height={210} />
                <span className="cover-title">{item.title}</span>
              </button>
            </li>
          ))}
        </ul>
      )}

      {selected !== null && (
        <LibraryDetail item={selected} onClose={() => setSelectedId(null)} />
      )}
    </section>
  )
}
