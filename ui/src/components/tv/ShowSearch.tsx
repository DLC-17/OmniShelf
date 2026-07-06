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
 * Search-and-add flow (spec §2.2 steps 1–2): search box → TMDB results → Add
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
      <form onSubmit={handleSubmit} role="search">
        <input
          type="search"
          aria-label="Search TV shows"
          placeholder="Search TV shows…"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          style={{ padding: '0.5rem', width: '16rem', marginRight: '0.5rem' }}
        />
        <button type="submit" disabled={input.trim() === ''}>
          Search
        </button>
      </form>

      {search.isFetching && <p>Searching…</p>}
      {search.isError && (
        <p role="alert" style={{ color: 'crimson' }}>
          {search.error instanceof ApiError && search.error.code === 'tmdb_unavailable'
            ? 'TMDB unreachable, try again'
            : 'Search failed. Try again.'}
        </p>
      )}
      {search.data !== undefined && search.data.length === 0 && (
        <p>No shows found for “{query}”.</p>
      )}
      {search.data !== undefined && search.data.length > 0 && (
        <ul style={{ listStyle: 'none', padding: 0, display: 'grid', gap: '0.5rem' }}>
          {search.data.map((result) => {
            const fb = feedback[result.id]
            return (
              <li
                key={result.id}
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '0.75rem',
                  border: '1px solid #ccc',
                  borderRadius: 8,
                  padding: '0.5rem 0.75rem',
                }}
              >
                <div style={{ flex: 1, minWidth: 0 }}>
                  <strong>{result.name}</strong>
                  {airYear(result) !== '' && <span style={{ color: '#666' }}> ({airYear(result)})</span>}
                  {result.overview !== '' && (
                    <p
                      style={{
                        margin: '0.25rem 0 0',
                        color: '#666',
                        fontSize: '0.85rem',
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
                  <span role={fb.isError ? 'alert' : 'status'} style={fb.isError ? { color: 'crimson' } : undefined}>
                    {fb.text}
                  </span>
                ) : (
                  <button
                    type="button"
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
