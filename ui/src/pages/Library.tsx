import { useState } from 'react'
import { ApiError } from '../api/client'
import type { ItemStatus, LibraryItem, MediaType } from '../api/library'
import LibraryDetail from '../components/library/LibraryDetail'
import MovieSearch from '../components/movies/MovieSearch'
import Poster from '../components/tv/Poster'
import { BookScan, GameScan } from './Scan'
import { useLibrary } from '../hooks/useLibrary'
import ShowSearch from '../components/tv/ShowSearch'
import GameSearch from '../components/games/GameSearch'
import BookSearch from '../components/books/BookSearch'

const TABS: { value: MediaType; label: string }[] = [
  { value: 'TV', label: 'TV Shows' },
  { value: 'BOOK', label: 'Books' },
  { value: 'GAME', label: 'Games' },
  { value: 'MOVIE', label: 'Movies' },
]

/** The "active" status and its label for each media type. */
const ACTIVE: Record<MediaType, { status: ItemStatus; label: string; stopped: string }> = {
  TV: { status: 'WATCHING', label: 'Watching', stopped: 'Stopped watching' },
  BOOK: { status: 'READING', label: 'Reading', stopped: 'Stopped reading' },
  GAME: { status: 'PLAYING', label: 'Playing', stopped: 'Stopped playing' },
  MOVIE: { status: 'WATCHING', label: 'Watching', stopped: 'Stopped watching' },
}

/** Status sections shown in order, with media-specific labels. */
function sectionsFor(media: MediaType): { status: ItemStatus; label: string }[] {
  const active = ACTIVE[media]
  return [
    { status: active.status, label: active.label },
    { status: 'PLAN_TO', label: 'Not started' },
    { status: 'COMPLETED', label: 'Completed' },
    { status: 'STOPPED', label: active.stopped },
  ]
}

/**
 * Library shelf: a cover-art grid toggled between TV shows, books, games and
 * movies. Clicking a cover opens the item's detail — summary, author/platform,
 * length, a self-rating, inline status/progress editing, and delete. The movies
 * tab also carries a search-and-add box.
 */
export default function Library() {
  const [media, setMedia] = useState<MediaType>('TV')
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [collapsed, setCollapsed] = useState<Set<ItemStatus>>(new Set())

  const toggleSection = (status: ItemStatus) =>
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(status)) next.delete(status)
      else next.add(status)
      return next
    })

  const library = useLibrary({ type: media })

  const items: LibraryItem[] = library.data ?? []
  const selected = items.find((i) => i.id === selectedId) ?? null

  return (
    <section>
      <h1>Library</h1>
      {media === 'TV' && (
        <>
        <ShowSearch />
        <hr />
        </>
      )}
      {media === 'BOOK' && (
        <>
        <BookSearch />
        <BookScan />
        <hr/>
        </>
      )}
      {media=== 'GAME' && (
        <>
        <GameSearch />
        <GameScan/>
        <hr/>
        </>
      )}
      {media === 'MOVIE' && (
        <>
          <MovieSearch />
          <hr />
        </>
      )}
      
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

      {library.isPending && <p className="muted">Loading your library…</p>}
      {library.isError && (
        <p role="alert" className="alert">
          {library.error instanceof ApiError
            ? library.error.message
            : 'Could not load your library. Try refreshing.'}
        </p>
      )}

      {library.data !== undefined && items.length === 0 && (
        <p className="empty">
          {media === 'MOVIE'
            ? 'No movies yet. Search for one below to start your watchlist.'
            : 'No items match these filters. Add a show from Up Next or scan a book to start building your shelf.'}
        </p>
      )}
      

      {items.length > 0 &&
        sectionsFor(media).map(({ status, label }) => {
          const sectionItems = items.filter((i) => i.status === status)
          if (sectionItems.length === 0) return null
          const open = !collapsed.has(status)
          return (
            <section key={status} className="library-section">
              <button
                type="button"
                className="library-section-title"
                aria-expanded={open}
                onClick={() => toggleSection(status)}
              >
                <span className="show-caret" aria-hidden="true">{open ? '▾' : '▸'}</span>
                {label} <span className="badge">{sectionItems.length}</span>
              </button>
              {open && (
                <ul className="cover-grid">
                  {sectionItems.map((item) => (
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
            </section>
          )
        })}

      

      {selected !== null && (
        <LibraryDetail item={selected} onClose={() => setSelectedId(null)} />
      )}
    </section>
  )
}
