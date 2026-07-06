import { useState } from 'react'
import type { FormEvent } from 'react'
import { ApiError } from '../../api/client'
import type { TVSearchResult } from '../../api/tv'
import { useAddShow, useShowSearch } from '../../hooks/useShowSearch'

interface AddFeedback {
  text: string
  isError: boolean
}

/** TMDB CDN for search thumbnails (un-added shows aren't cached locally yet). */
const TMDB_THUMB = 'https://image.tmdb.org/t/p/w154'

/** First-air year, e.g. "2011", or empty when TMDB has no date. */
function airYear(result: TVSearchResult): string {
  return result.firstAirDate === '' ? '' : result.firstAirDate.slice(0, 4)
}

/**
 * Small poster for a search hit. A raw TMDB path (leading slash) loads from the
 * TMDB CDN; a cached path resolves under /images. Empty paths and load failures
 * fall back to a neutral initial-letter placeholder.
 */
function ResultPoster({ posterPath, title }: { posterPath: string; title: string }) {
  const [failed, setFailed] = useState(false)
  if (posterPath === '' || failed) {
    return (
      <div
        role="img"
        aria-label={`No poster for ${title}`}
        className="poster placeholder"
        style={{ width: 46, height: 69, fontSize: '1rem' }}
      >
        {title.charAt(0).toUpperCase()}
      </div>
    )
  }
  const src = posterPath.startsWith('/') ? `${TMDB_THUMB}${posterPath}` : `/images/${posterPath}`
  return (
    <img
      src={src}
      alt={`Poster for ${title}`}
      width={46}
      height={69}
      className="poster"
      onError={() => setFailed(true)}
    />
  )
}

/**
 * Search-and-add flow: search box → TMDB results → Add
 * button. A duplicate add (409 duplicate_item, E16) is reported inline as
 * "already in your library" rather than as a failure.
 */
export default function ShowSearch() {
  const [input, setInput] = useState('')
  const [query, setQuery] = useState('')
  const [feedback, setFeedback] = useState<Record<number, AddFeedback>>({})

  const search = useShowSearch(query)
  const addShow = useAddShow()

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setQuery(input.trim())
    setFeedback({})
  }

  const handleAdd = (tmdbId: number) => {
    addShow.mutate(tmdbId, {
      onSuccess: () => {
        setFeedback((prev) => ({ ...prev, [tmdbId]: { text: 'Added', isError: false } }))
      },
      onError: (err) => {
        if (err instanceof ApiError && err.code === 'duplicate_item') {
          setFeedback((prev) => ({
            ...prev,
            [tmdbId]: { text: 'Already in your library', isError: false },
          }))
          return
        }
        const text = err instanceof ApiError ? err.message : 'Something went wrong. Try again.'
        setFeedback((prev) => ({ ...prev, [tmdbId]: { text, isError: true } }))
      },
    })
  }

  return (
    <section aria-label="Add a show">
      <h2>Add a show</h2>
      <form className="searchbar" onSubmit={handleSubmit} role="search">
        <input
          type="search"
          aria-label="Search TV shows"
          placeholder="Search TV shows…"
          value={input}
          onChange={(e) => setInput(e.target.value)}
        />
        <button type="submit" className="btn-primary" disabled={input.trim() === ''}>
          Search
        </button>
      </form>

      {search.isFetching && <p className="muted">Searching…</p>}
      {search.isError && (
        <p role="alert" className="alert">
          {search.error instanceof ApiError && search.error.code === 'tmdb_unavailable'
            ? 'TMDB unreachable, try again'
            : 'Search failed. Try again.'}
        </p>
      )}
      {search.data !== undefined && search.data.length === 0 && (
        <p>No shows found for “{query}”.</p>
      )}
      {search.data !== undefined && search.data.length > 0 && (
        <ul className="list">
          {search.data.map((result) => {
            const fb = feedback[result.id]
            return (
              <li key={result.id} className="card card-row">
                <ResultPoster posterPath={result.posterPath} title={result.name} />
                <div className="grow">
                  <strong>{result.name}</strong>
                  {airYear(result) !== '' && <span className="muted"> ({airYear(result)})</span>}
                  {result.overview !== '' && (
                    <p className="meta search-overview">{result.overview}</p>
                  )}
                </div>
                {fb !== undefined ? (
                  <span role={fb.isError ? 'alert' : 'status'} className={fb.isError ? 'alert' : 'muted'}>
                    {fb.text}
                  </span>
                ) : (
                  <button
                    type="button"
                    className="btn-confirm"
                    onClick={() => handleAdd(result.id)}
                    disabled={addShow.isPending}
                    aria-label={`Add ${result.name}`}
                  >
                    Add
                  </button>
                )}
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
