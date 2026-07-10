import { useState } from 'react'
import type { FormEvent } from 'react'
import { ApiError } from '../../api/client'
import { useAddGame, useGameSearch } from '../../hooks/useGameSearch'

interface AddFeedback {
  text: string
  isError: boolean
}

/**
 * Initial-letter placeholder for a search hit. IGDB cover art is only cached
 * locally once a game is added, and hotlinking the IGDB CDN is disallowed, so
 * un-added search results show no thumbnail.
 */
function ResultPlaceholder({ title }: { title: string }) {
  return (
    <div
      role="img"
      aria-label={`No cover for ${title}`}
      className="poster placeholder"
      style={{ width: 46, height: 69, fontSize: '1rem' }}
    >
      {title.charAt(0).toUpperCase()}
    </div>
  )
}

/**
 * Search-and-add flow for games by title: search box → IGDB results → Add
 * button. A duplicate add (409 already_tracked) is reported inline as "already
 * in your library" rather than as a failure. Added games land on the watchlist
 * (PLAN_TO) — change the status from the library detail once playing.
 */
export default function GameSearch() {
  const [input, setInput] = useState('')
  const [query, setQuery] = useState('')
  const [feedback, setFeedback] = useState<Record<number, AddFeedback>>({})

  const search = useGameSearch(query)
  const addGame = useAddGame()

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setQuery(input.trim())
    setFeedback({})
  }

  const handleAdd = (igdbId: number) => {
    addGame.mutate(igdbId, {
      onSuccess: () => {
        setFeedback((prev) => ({ ...prev, [igdbId]: { text: 'Added', isError: false } }))
      },
      onError: (err) => {
        if (err instanceof ApiError && err.code === 'already_tracked') {
          setFeedback((prev) => ({
            ...prev,
            [igdbId]: { text: 'Already in your library', isError: false },
          }))
          return
        }
        const text = err instanceof ApiError ? err.message : 'Something went wrong. Try again.'
        setFeedback((prev) => ({ ...prev, [igdbId]: { text, isError: true } }))
      },
    })
  }

  return (
    <section aria-label="Add a game by name">
      <h2>Add a game by name</h2>
      <form className="searchbar" onSubmit={handleSubmit} role="search">
        <input
          type="search"
          aria-label="Search games"
          placeholder="Search games by title…"
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
          {search.error instanceof ApiError && search.error.code === 'upstream_error'
            ? 'Game search is unavailable right now.'
            : 'Search failed. Try again.'}
        </p>
      )}
      {search.data !== undefined && search.data.length === 0 && (
        <p>No games found for “{query}”.</p>
      )}
      {search.data !== undefined && search.data.length > 0 && (
        <ul className="list">
          {search.data.map((result) => {
            const fb = feedback[result.igdbId]
            return (
              <li key={result.igdbId} className="card card-row">
                <ResultPlaceholder title={result.name} />
                <div className="grow">
                  <strong>{result.name}</strong>
                  {result.year !== 0 && <span className="muted"> ({result.year})</span>}
                </div>
                {fb !== undefined ? (
                  <span role={fb.isError ? 'alert' : 'status'} className={fb.isError ? 'alert' : 'muted'}>
                    {fb.text}
                  </span>
                ) : (
                  <button
                    type="button"
                    className="btn-confirm"
                    onClick={() => handleAdd(result.igdbId)}
                    disabled={addGame.isPending}
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
