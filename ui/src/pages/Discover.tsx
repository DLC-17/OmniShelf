import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError } from '../api/client'
import { fetchDiscover, rejectRec } from '../api/discover'
import type { DiscoverItem } from '../api/discover'
import { addShow } from '../api/tv'

const DISCOVER_KEY = ['discover'] as const
const TMDB_THUMB = 'https://image.tmdb.org/t/p/w154'

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
 * Discover: TV suggestions based on the shows the user already tracks. Each
 * card says what it was suggested from and can be added to the library or
 * rejected (hidden from future suggestions). Both actions remove the card.
 */
export default function Discover() {
  const queryClient = useQueryClient()
  const discover = useQuery({ queryKey: DISCOVER_KEY, queryFn: fetchDiscover })

  const removeCard = (tmdbId: number) =>
    queryClient.setQueryData<DiscoverItem[]>(DISCOVER_KEY, (old) =>
      old?.filter((i) => i.tmdbId !== tmdbId),
    )

  const add = useMutation({
    mutationFn: (tmdbId: number) => addShow(tmdbId),
    onSuccess: (_data, tmdbId) => removeCard(tmdbId),
  })
  const reject = useMutation({
    mutationFn: (tmdbId: number) => rejectRec(tmdbId),
    onSuccess: (_data, tmdbId) => removeCard(tmdbId),
  })
  const busy = add.isPending || reject.isPending

  const items = [...(discover.data ?? [])].sort((a, b) => a.title.localeCompare(b.title))

  return (
    <section>
      <h1>Discover</h1>
      <p className="muted">Suggestions based on the shows you already track.</p>

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
          No suggestions yet. Track a few TV shows and we’ll recommend more like them here.
        </p>
      )}

      {items.length > 0 && (
        <ul className="list">
          {items.map((item) => {
            const year = item.firstAirDate === '' ? '' : item.firstAirDate.slice(0, 4)
            return (
              <li key={item.tmdbId} className="card card-row">
                <DiscoverPoster posterPath={item.posterPath} title={item.title} />
                <div className="grow">
                  <strong>{item.title}</strong>
                  {year !== '' && <span className="muted"> ({year})</span>}
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
            )
          })}
        </ul>
      )}
    </section>
  )
}
