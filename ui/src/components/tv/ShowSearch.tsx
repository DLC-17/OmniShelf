import { useState } from 'react'
import type { FormEvent } from 'react'
import { ApiError } from '../../api/client'
import type { TVSearchResult } from '../../api/tv'
import { useAddShow, useShowSearch } from '../../hooks/useShowSearch'

interface AddFeedback {
  text: string
  isError: boolean
}

/** First-air year, e.g. "2011", or empty when TMDB has no date. */
function airYear(result: TVSearchResult): string {
  return result.firstAirDate === '' ? '' : result.firstAirDate.slice(0, 4)
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
                <div className="grow">
                  <strong>{result.name}</strong>
                  {airYear(result) !== '' && <span className="muted"> ({airYear(result)})</span>}
                  {result.overview !== '' && (
                    <p
                      className="meta"
                      style={{
                        marginTop: '0.25rem',
                        overflow: 'hidden',
                        textOverflow: 'ellipsis',
                        whiteSpace: 'nowrap',
                      }}
                    >
                      {result.overview}
                    </p>
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
