import { useState } from 'react'
import type { FormEvent } from 'react'
import { ApiError } from '../../api/client'
import type { MovieSearchResult } from '../../api/movies'
import { useAddMovie, useMovieSearch } from '../../hooks/useMovieSearch'

interface AddFeedback {
  text: string
  isError: boolean
}

/** TMDB CDN for search thumbnails (un-added movies aren't cached locally yet). */
const TMDB_THUMB = 'https://image.tmdb.org/t/p/w154'

/** Release year, e.g. "2010", or empty when TMDB has no date. */
function releaseYear(result: MovieSearchResult): string {
  return result.releaseDate === '' ? '' : result.releaseDate.slice(0, 4)
}

/** Small poster for a search hit; falls back to an initial-letter placeholder. */
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
 * Search-and-add flow for movies: search box → TMDB results → Add button. A
 * duplicate add (409 duplicate_item) is reported inline as "already in your
 * library" rather than as a failure. Added movies land on the watchlist
 * (PLAN_TO) — change the status from the library detail once watched.
 */
export default function MovieSearch() {
  const [input, setInput] = useState('')
  const [query, setQuery] = useState('')
  const [feedback, setFeedback] = useState<Record<number, AddFeedback>>({})

  const search = useMovieSearch(query)
  const addMovie = useAddMovie()

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setQuery(input.trim())
    setFeedback({})
  }

  const handleAdd = (tmdbId: number) => {
    addMovie.mutate(tmdbId, {
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
    <section aria-label="Add a movie">
      <h2>Add a movie</h2>
      <form className="searchbar" onSubmit={handleSubmit} role="search">
        <input
          type="search"
          aria-label="Search movies"
          placeholder="Search movies…"
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
        <p>No movies found for “{query}”.</p>
      )}
      {search.data !== undefined && search.data.length > 0 && (
        <ul className="list">
          {search.data.map((result) => {
            const fb = feedback[result.id]
            return (
              <li key={result.id} className="card card-row">
                <ResultPoster posterPath={result.posterPath} title={result.title} />
                <div className="grow">
                  <strong>{result.title}</strong>
                  {releaseYear(result) !== '' && <span className="muted"> ({releaseYear(result)})</span>}
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
                    disabled={addMovie.isPending}
                    aria-label={`Add ${result.title}`}
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
