import { useMemo, useState } from 'react'
import { ApiError } from '../api/client'
import type { ItemStatus, LibraryItem, MediaType } from '../api/library'
import LibraryDetail from '../components/library/LibraryDetail'
import LibraryToolbar, { applyLibrarySearch } from '../components/library/LibraryToolbar'
import type { FilterState } from '../components/library/LibraryToolbar'
import MovieSearch from '../components/movies/MovieSearch'
import MusicSearch from '../components/music/MusicSearch'
import Poster from '../components/tv/Poster'
import { useLibrary } from '../hooks/useLibrary'
import ShowSearch from '../components/tv/ShowSearch'
import GameSearch from '../components/games/GameSearch'
import BookSearch from '../components/books/BookSearch'
import { formatUsd } from '../lib/currency'

const TABS: { value: MediaType; label: string }[] = [
  { value: 'TV', label: 'TV Shows' },
  { value: 'BOOK', label: 'Books' },
  { value: 'GAME', label: 'Games' },
  { value: 'MOVIE', label: 'Movies' },
  { value: 'MUSIC', label: 'Music' },
  { value: 'CARD', label: 'Cards' },
]

/** The "active" status and its label for each media type with a lifecycle
 * (cards are simply OWNED and have their own single section). */
const ACTIVE: Record<Exclude<MediaType, 'CARD'>, { status: ItemStatus; label: string; stopped: string }> = {
  TV: { status: 'WATCHING', label: 'Watching', stopped: 'Stopped watching' },
  BOOK: { status: 'READING', label: 'Reading', stopped: 'Stopped reading' },
  GAME: { status: 'PLAYING', label: 'Playing', stopped: 'Stopped playing' },
  MOVIE: { status: 'WATCHING', label: 'Watching', stopped: 'Stopped watching' },
  MUSIC: { status: 'LISTENING', label: 'Listening', stopped: 'Set aside' },
}

/** Groups albums under their artist, each group's albums title-sorted, with
 * artists ordered alphabetically. Albums with no artist fall under "Unknown
 * artist". */
function groupByArtist(items: LibraryItem[]): { artist: string; albums: LibraryItem[] }[] {
  const groups = new Map<string, LibraryItem[]>()
  for (const item of items) {
    const key = item.artist.trim() === '' ? 'Unknown artist' : item.artist
    const bucket = groups.get(key)
    if (bucket) bucket.push(item)
    else groups.set(key, [item])
  }
  return [...groups.entries()]
    .sort(([a], [b]) => a.localeCompare(b, undefined, { sensitivity: 'base' }))
    .map(([artist, albums]) => ({
      artist,
      albums: [...albums].sort((a, b) => a.title.localeCompare(b.title, undefined, { sensitivity: 'base' })),
    }))
}

/** The TCG a card belongs to, derived from its source-prefixed external id
 * ("ygo:LOB-001" / "ptcg:base1-46"). */
function cardGameLabel(item: LibraryItem): string {
  if (item.externalId.startsWith('ygo:')) return 'Yu-Gi-Oh!'
  if (item.externalId.startsWith('ptcg:')) return 'Pokémon'
  return 'Other'
}

/** Groups cards by TCG, then within each game by set (platform carries the
 * set name, falling back to the printed set code). Games and sets are ordered
 * alphabetically; each set's cards are ordered by collector number then
 * title. Cards with no set fall under "Unknown set". */
function groupByGameAndSet(
  items: LibraryItem[],
): { game: string; sets: { set: string; cards: LibraryItem[] }[] }[] {
  const byGame = new Map<string, Map<string, LibraryItem[]>>()
  for (const item of items) {
    const game = cardGameLabel(item)
    const set = item.platform.trim() === '' ? 'Unknown set' : item.platform
    const sets = byGame.get(game) ?? new Map<string, LibraryItem[]>()
    byGame.set(game, sets)
    const bucket = sets.get(set)
    if (bucket) bucket.push(item)
    else sets.set(set, [item])
  }
  const numberOf = (i: LibraryItem) => parseInt(i.setCode, 10)
  const alpha = (a: string, b: string) => a.localeCompare(b, undefined, { sensitivity: 'base' })
  return [...byGame.entries()].sort(([a], [b]) => alpha(a, b)).map(([game, sets]) => ({
    game,
    sets: [...sets.entries()].sort(([a], [b]) => alpha(a, b)).map(([set, cards]) => ({
      set,
      cards: [...cards].sort((a, b) => {
        const an = numberOf(a)
        const bn = numberOf(b)
        if (!Number.isNaN(an) && !Number.isNaN(bn) && an !== bn) return an - bn
        return alpha(a.title, b.title)
      }),
    })),
  }))
}

/** Status sections shown in order, with media-specific labels. */
function sectionsFor(media: Exclude<MediaType, 'CARD'>): { status: ItemStatus; label: string }[] {
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
  // Per-tab library search + filters, applied client-side to the loaded items.
  const [search, setSearch] = useState('')
  const [filters, setFilters] = useState<FilterState>({})

  const toggleSection = (status: ItemStatus) =>
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(status)) next.delete(status)
      else next.add(status)
      return next
    })

  const library = useLibrary({ type: media })

  const items: LibraryItem[] = library.data ?? []
  const visible = useMemo(
    () => applyLibrarySearch(items, search, filters, media),
    [items, search, filters, media],
  )
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
        <hr/>
        </>
      )}
      {media=== 'GAME' && (
        <>
        <GameSearch />
        <hr/>
        </>
      )}
      {media === 'MOVIE' && (
        <>
          <MovieSearch />
          <hr />
        </>
      )}
      {media === 'MUSIC' && (
        <>
          <MusicSearch />
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
              setSearch('')
              setFilters({})
            }}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {items.length > 0 && (
        <LibraryToolbar
          media={media}
          items={items}
          search={search}
          onSearchChange={setSearch}
          filters={filters}
          onFiltersChange={setFilters}
        />
      )}

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
            : media === 'MUSIC'
              ? 'No albums yet. Scan a barcode or search by name above to start your collection.'
              : media === 'CARD'
                ? 'No cards yet. Photograph one from the Scan page to start your collection.'
                : 'No items match these filters. Add a show from Up Next or scan a book to start building your shelf.'}
        </p>
      )}
      

      {items.length > 0 && visible.length === 0 && (
        <p className="empty">No items match your search or filters.</p>
      )}

      {media === 'MUSIC' &&
        visible.length > 0 &&
        groupByArtist(visible).map(({ artist, albums }) => {
          const open = !collapsed.has(artist as ItemStatus)
          return (
            <section key={artist} className="library-section">
              <button
                type="button"
                className="library-section-title"
                aria-expanded={open}
                onClick={() => toggleSection(artist as ItemStatus)}
              >
                <span className="show-caret" aria-hidden="true">{open ? '▾' : '▸'}</span>
                {artist} <span className="badge">{albums.length}</span>
              </button>
              {open && (
                <ul className="cover-grid">
                  {albums.map((item) => (
                    <li key={item.id}>
                      <button
                        type="button"
                        className="cover-tile"
                        aria-label={`Open ${item.title}`}
                        onClick={() => setSelectedId(item.id)}
                      >
                        <Poster posterPath={item.artworkPath} title={item.title} width={140} height={140} />
                        <span className="cover-title">{item.title}</span>
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </section>
          )
        })}

      {media === 'CARD' &&
        visible.length > 0 &&
        groupByGameAndSet(visible).map(({ game, sets }) => (
          <section key={game} aria-label={game}>
            <h2>{game}</h2>
            {sets.map(({ set, cards }) => {
              // Collapse keys are game-qualified so same-named sets in two
              // TCGs never toggle together.
              const sectionKey = `${game} · ${set}` as ItemStatus
              const open = !collapsed.has(sectionKey)
              return (
                <section key={set} className="library-section">
                  <button
                    type="button"
                    className="library-section-title"
                    aria-expanded={open}
                    onClick={() => toggleSection(sectionKey)}
                  >
                    <span className="show-caret" aria-hidden="true">{open ? '▾' : '▸'}</span>
                    {set} <span className="badge">{cards.length}</span>
                  </button>
                  {open && (
                    <ul className="cover-grid">
                      {cards.map((item) => (
                        <li key={item.id}>
                          <button
                            type="button"
                            className="cover-tile"
                            aria-label={`Open ${item.title}`}
                            onClick={() => setSelectedId(item.id)}
                          >
                            <Poster posterPath={item.artworkPath} title={item.title} width={140} height={195} />
                            <span className="cover-title">{item.title}</span>
                            {item.setCode !== '' && <span className="meta">{item.setCode}</span>}
                            {item.price > 0 && <span className="meta">{formatUsd(item.price)}</span>}
                          </button>
                        </li>
                      ))}
                    </ul>
                  )}
                </section>
              )
            })}
          </section>
        ))}

      {media !== 'MUSIC' &&
        media !== 'CARD' &&
        visible.length > 0 &&
        sectionsFor(media).map(({ status, label }) => {
          const sectionItems = visible.filter((i) => i.status === status)
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
                        {item.type === 'BOOK' && item.pageCount > 0 && (
                          <span className="meta">{item.pageCount} pages</span>
                        )}
                        {item.type === 'CARD' && item.price > 0 && (
                          <span className="meta">{formatUsd(item.price)}</span>
                        )}
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
