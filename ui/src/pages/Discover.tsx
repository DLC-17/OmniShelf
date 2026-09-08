import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError } from '../api/client'
import {
  fetchBookDiscover,
  fetchDiscover,
  fetchGameDiscover,
  fetchMovieDiscover,
  rejectBookRec,
  rejectGameRec,
  rejectMovieRec,
  rejectRec,
} from '../api/discover'
import { addShow } from '../api/tv'
import { addMovie } from '../api/movies'
import { addGameByIgdb } from '../api/games'
import { listEditions, scanBook, trackBook } from '../api/books'

type DiscoverMedia = 'tv' | 'movie' | 'game' | 'book'

/**
 * A media-agnostic suggestion the page renders and acts on. `key` uniquely
 * identifies the card (TMDB id, IGDB id, or OpenLibrary work key). Images come
 * from one of two sources: TV/movie posters are hotlinked from the TMDB CDN
 * (`posterPath`), while game/book covers are cached server-side and served from
 * `/images` (`coverPath`) — IGDB/OpenLibrary covers are never hotlinked.
 */
interface Suggestion {
  key: string
  title: string
  /** Brief summary: TV/movie synopsis, IGDB game summary, or a book's opening
   * sentence. The card clamps it to a few lines so every media type renders
   * the same on mobile and desktop. */
  overview: string
  /** Secondary byline under the title: a book's authors; blank otherwise. */
  subtitle: string
  year: string
  suggestedBy: string
  posterPath: string // TMDB path (hotlinked) for tv/movie; "" otherwise
  coverPath: string // relative /images path for game/book; "" otherwise
  // Action payloads (exactly one is set, per media type).
  tmdbId?: number
  igdbId?: number
  workKey?: string
}

const TMDB_THUMB = 'https://image.tmdb.org/t/p/w154'

const TABS: { value: DiscoverMedia; label: string }[] = [
  { value: 'tv', label: 'TV Shows' },
  { value: 'movie', label: 'Movies' },
  { value: 'game', label: 'Games' },
  { value: 'book', label: 'Books' },
]

/** Poster/cover thumbnail: internal cover when present, else a hotlinked TMDB
 * poster, else a lettered placeholder. */
function DiscoverPoster({
  posterPath,
  coverPath,
  title,
}: {
  posterPath: string
  coverPath: string
  title: string
}) {
  const [failed, setFailed] = useState(false)

  // Reset failed state if paths change
  useEffect(() => {
    setFailed(false)
  }, [posterPath, coverPath])

  let src = ''
  if (coverPath !== '') {
    src = coverPath.startsWith('/') ? `/images${coverPath}` : `/images/${coverPath}`
  } else if (posterPath !== '') {
    if (posterPath.startsWith('http://') || posterPath.startsWith('https://')) {
      src = posterPath
    } else if (posterPath.startsWith('//')) {
      src = `https:${posterPath}`
    } else if (posterPath.startsWith('/')) {
      src = `${TMDB_THUMB}${posterPath}`
    } else {
      src = `${TMDB_THUMB}/${posterPath}`
    }
  }

  if (src === '' || failed) {
    return (
      <div
        role="img"
        aria-label={`No cover for ${title}`}
        className="poster placeholder"
        style={{ width: '100%', height: '100%' }}
      >
        {title ? title.charAt(0).toUpperCase() : '?'}
      </div>
    )
  }

  return (
    <img
      src={src}
      alt={`Cover for ${title}`}
      className="poster"
      style={{ width: '100%', height: '100%', objectFit: 'cover' }}
      onError={() => setFailed(true)}
    />
  )
}

const yearFromDate = (date: string): string => (date === '' ? '' : date.slice(0, 4))
const yearFromInt = (year: number): string => (year === 0 ? '' : String(year))

/**
 * Discover: suggestions based on what the user already tracks, toggled between
 * TV shows, movies, games, and books. Each card says what it was suggested from
 * and can be added to the library or rejected (hidden from future suggestions).
 * Both actions remove the card.
 */
export default function Discover() {
  const [media, setMedia] = useState<DiscoverMedia>('tv')
  const queryClient = useQueryClient()
  const discoverKey = ['discover', media] as const

  const discover = useQuery({
    queryKey: discoverKey,
    queryFn: async (): Promise<Suggestion[]> => {
      if (media === 'movie') {
        const items = await fetchMovieDiscover()
        return items.map((i) => ({
          key: String(i.tmdbId),
          title: i.title,
          overview: i.overview,
          subtitle: '',
          year: yearFromDate(i.releaseDate),
          suggestedBy: i.suggestedBy,
          posterPath: i.posterPath,
          coverPath: '',
          tmdbId: i.tmdbId,
        }))
      }
      if (media === 'game') {
        const items = await fetchGameDiscover()
        return items.map((i) => ({
          key: String(i.igdbId),
          title: i.title,
          overview: i.summary,
          subtitle: '',
          year: yearFromInt(i.year),
          suggestedBy: i.suggestedBy,
          posterPath: '',
          coverPath: i.coverPath,
          igdbId: i.igdbId,
        }))
      }
      if (media === 'book') {
        const items = await fetchBookDiscover()
        return items.map((i) => ({
          key: i.workKey,
          title: i.title,
          overview: i.summary,
          subtitle: i.authors,
          year: yearFromInt(i.year),
          suggestedBy: i.suggestedBy,
          posterPath: '',
          coverPath: i.coverPath,
          workKey: i.workKey,
        }))
      }
      const items = await fetchDiscover()
      return items.map((i) => ({
        key: String(i.tmdbId),
        title: i.title,
        overview: i.overview,
        subtitle: '',
        year: yearFromDate(i.firstAirDate),
        suggestedBy: i.suggestedBy,
        posterPath: i.posterPath,
        coverPath: '',
        tmdbId: i.tmdbId,
      }))
    },
  })

  // Optimistically remove the acted-on item from the cached list so the UI
  // updates immediately without waiting for a re-fetch.
  const dropItem = (key: string) => {
    queryClient.setQueryData<Suggestion[]>(discoverKey, (old) =>
      old === undefined ? [] : old.filter((item) => item.key !== key),
    )
  }

  const add = useMutation({
    mutationFn: async (item: Suggestion) => {
      if (item.tmdbId !== undefined) {
        if (media === 'movie') {
          await addMovie(item.tmdbId)
        } else {
          await addShow(item.tmdbId)
        }
      } else if (item.igdbId !== undefined) {
        await addGameByIgdb(item.igdbId)
      } else if (item.workKey !== undefined) {
        const editions = await listEditions(item.workKey)
        if (editions.length === 0) {
          throw new Error('No editions found for that book')
        }
        const scanned = await scanBook(editions[0].isbn13)
        await trackBook(scanned.id, 'PLAN_TO')
      }
      return item.key
    },
    onSuccess: (key) => {
      dropItem(key)
      queryClient.invalidateQueries({ queryKey: ['library'] })
      queryClient.invalidateQueries({ queryKey: ['up-next'] })
    },
  })

  const reject = useMutation({
    mutationFn: async (item: Suggestion) => {
      if (item.tmdbId !== undefined) {
        if (media === 'movie') {
          await rejectMovieRec(item.tmdbId)
        } else {
          await rejectRec(item.tmdbId)
        }
      } else if (item.igdbId !== undefined) {
        await rejectGameRec(item.igdbId)
      } else if (item.workKey !== undefined) {
        await rejectBookRec(item.workKey)
      }
      return item.key
    },
    onSuccess: (key) => {
      dropItem(key)
    },
  })

  const noun =
    media === 'tv' ? 'shows' : media === 'movie' ? 'movies' : media === 'game' ? 'games' : 'books'
  const items = discover.data ?? []
  const busy = add.isPending || reject.isPending

  return (
    <section>
      <h1>Discover</h1>
      <div className="tabs" role="tablist" aria-label="Discover media type">
        {TABS.map((tab) => (
          <button
            key={tab.value}
            type="button"
            role="tab"
            aria-selected={media === tab.value}
            className={media === tab.value ? 'tab active' : 'tab'}
            onClick={() => setMedia(tab.value)}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {discover.isPending && <p className="muted">Finding suggestions…</p>}
      {discover.isError && (
        <p role="alert" className="alert">
          {discover.error instanceof ApiError
            ? discover.error.message
            : 'Could not load suggestions. Try again.'}
        </p>
      )}
      {add.isError && (
        <p role="alert" className="alert">
          {add.error instanceof ApiError ? add.error.message : 'Could not add that item. Try again.'}
        </p>
      )}

      {discover.data !== undefined && items.length === 0 && (
        <p className="empty">
          No suggestions yet. Track a few {noun} and we’ll recommend more like them here.
        </p>
      )}

      {items.length > 0 && (
        <ul className="list">
          {items.map((item) => (
            <li key={item.key} className="card">
              <div className="show-card-row">
                <div className="show-card-poster">
                  <DiscoverPoster
                    posterPath={item.posterPath}
                    coverPath={item.coverPath}
                    title={item.title}
                  />
                </div>
                <div className="show-card-content">
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: '0.75rem', flexWrap: 'wrap' }}>
                    <div className="grow">
                      <h3 style={{ margin: '0 0 0.2rem', fontSize: '1.05rem', lineHeight: 1.25 }}>
                        {item.title}
                        {item.year !== '' && <span className="muted" style={{ fontWeight: 400, fontSize: '0.9rem' }}> ({item.year})</span>}
                      </h3>
                      {item.subtitle !== '' && <p className="muted" style={{ margin: '0 0 0.2rem', fontSize: '0.85rem' }}>{item.subtitle}</p>}
                      <p className="meta" style={{ fontSize: '0.8rem', margin: '0 0 0.25rem' }}>
                        Suggested based on {item.suggestedBy}
                      </p>
                      {item.overview !== '' && (
                        <p className="meta search-overview" style={{ fontSize: '0.82rem', margin: 0 }}>
                          {item.overview}
                        </p>
                      )}
                    </div>
                    <div className="cluster" style={{ flexShrink: 0, marginTop: '0.2rem' }}>
                      <button
                        type="button"
                        className="btn-confirm"
                        disabled={busy}
                        aria-label={`Add ${item.title}`}
                        onClick={() => add.mutate(item)}
                      >
                        Add
                      </button>
                      <button
                        type="button"
                        className="btn-ghost"
                        disabled={busy}
                        aria-label={`Reject ${item.title}`}
                        onClick={() => reject.mutate(item)}
                      >
                        Not interested
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
