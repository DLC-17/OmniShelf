import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError } from '../api/client'
import {
  fetchDiscover,
  fetchMovieDiscover,
  rejectMovieRec,
  rejectRec,
} from '../api/discover'
import { addShow } from '../api/tv'
import { addMovie } from '../api/movies'

type DiscoverMedia = 'tv' | 'movie'

/** A media-agnostic suggestion the page renders and acts on. */
interface Suggestion {
  tmdbId: number
  title: string
  overview: string
  posterPath: string
  year: string
  suggestedBy: string
}

const TMDB_THUMB = 'https://image.tmdb.org/t/p/w154'

const TABS: { value: DiscoverMedia; label: string }[] = [
  { value: 'tv', label: 'TV Shows' },
  { value: 'movie', label: 'Movies' },
]

function DiscoverPoster({ posterPath, title }: { posterPath: string; title: string }) {
  const [failed, setFailed] = useState(false)
  if (posterPath === '' || failed) {
    return (
      <div
        role="img"
        aria-label={`No poster for ${title}`}
        className="poster placeholder"
        style={{ width: 60, height: 90, fontSize: '1.1rem' }}
      >
        {title.charAt(0).toUpperCase()}
      </div>
    )
  }
  return (
    <img
      src={`${TMDB_THUMB}${posterPath}`}
      alt={`Poster for ${title}`}
      width={60}
      height={90}
      className="poster"
      onError={() => setFailed(true)}
    />
  )
}

/**
 * Discover: suggestions based on what the user already tracks, toggled between
 * TV shows and movies. Each card says what it was suggested from and can be
 * added to the library or rejected (hidden from future suggestions). Both
 * actions remove the card.
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
          tmdbId: i.tmdbId,
          title: i.title,
          overview: i.overview,
          posterPath: i.posterPath,
          year: i.releaseDate === '' ? '' : i.releaseDate.slice(0, 4),
          suggestedBy: i.suggestedBy,
        }))
      }
      const items = await fetchDiscover()
      return items.map((i) => ({
        tmdbId: i.tmdbId,
        title: i.title,
        overview: i.overview,
        posterPath: i.posterPath,
        year: i.firstAirDate === '' ? '' : i.firstAirDate.slice(0, 4),
        suggestedBy: i.suggestedBy,
      }))
    },
  })

  const removeCard = (tmdbId: number) =>
    queryClient.setQueryData<Suggestion[]>(discoverKey, (old) =>
      old?.filter((i) => i.tmdbId !== tmdbId),
    )

  const add = useMutation({
    mutationFn: async (tmdbId: number) => {
      if (media === 'movie') await addMovie(tmdbId)
      else await addShow(tmdbId)
    },
    onSuccess: (_data, tmdbId) => removeCard(tmdbId),
  })
  const reject = useMutation({
    mutationFn: (tmdbId: number) => (media === 'movie' ? rejectMovieRec(tmdbId) : rejectRec(tmdbId)),
    onSuccess: (_data, tmdbId) => removeCard(tmdbId),
  })
  const busy = add.isPending || reject.isPending

  const items = [...(discover.data ?? [])].sort((a, b) => a.title.localeCompare(b.title))
  const noun = media === 'movie' ? 'movies' : 'TV shows'

  return (
    <section>
      <h1>Discover</h1>
      <p className="muted">Suggestions based on the {noun} you already track.</p>

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

      {discover.data !== undefined && items.length === 0 && (
        <p className="empty">
          No suggestions yet. Track a few {noun} and we’ll recommend more like them here.
        </p>
      )}

      {items.length > 0 && (
        <ul className="list">
          {items.map((item) => (
            <li key={item.tmdbId} className="card card-row">
              <DiscoverPoster posterPath={item.posterPath} title={item.title} />
              <div className="grow">
                <strong>{item.title}</strong>
                {item.year !== '' && <span className="muted"> ({item.year})</span>}
                <p className="meta">Suggested based on {item.suggestedBy}</p>
                {item.overview !== '' && <p className="meta search-overview">{item.overview}</p>}
              </div>
              <span className="cluster">
                <button
                  type="button"
                  className="btn-confirm"
                  disabled={busy}
                  aria-label={`Add ${item.title}`}
                  onClick={() => add.mutate(item.tmdbId)}
                >
                  Add
                </button>
                <button
                  type="button"
                  className="btn-ghost"
                  disabled={busy}
                  aria-label={`Reject ${item.title}`}
                  onClick={() => reject.mutate(item.tmdbId)}
                >
                  Not interested
                </button>
              </span>
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
