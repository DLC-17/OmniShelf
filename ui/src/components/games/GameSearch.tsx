import { useState } from 'react'
import type { FormEvent } from 'react'
import { ApiError } from '../../api/client'
import { useAddGame, useGameSearch } from '../../hooks/useGameSearch'
import SearchCover from '../common/SearchCover'

interface AddFeedback {
  text: string
  isError: boolean
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
  const [collapsed, setCollapsed] = useState(false)
  const [feedback, setFeedback] = useState<Record<number, AddFeedback>>({})

  const search = useGameSearch(query)
  const addGame = useAddGame()

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setQuery(input.trim())
    setFeedback({})
    setCollapsed(false) // automatically expand on a new search
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
        <button
          type="button"
          className="btn-ghost"
          style={{
            padding: '0 0.5rem',
            fontSize: '0.9rem',
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            transform: collapsed ? 'rotate(180deg)' : 'rotate(0deg)',
            transition: 'transform 0.2s ease',
          }}
          onClick={() => setCollapsed((prev) => !prev)}
          aria-label={collapsed ? 'Expand search results' : 'Collapse search results'}
          title={collapsed ? 'Expand search results' : 'Collapse search results'}
        >
          ▲
        </button>
        <button type="submit" className="btn-primary" disabled={input.trim() === ''}>
          Search
        </button>
      </form>

      {!collapsed && (
        <>
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
                    <SearchCover
                      src={result.coverImageId !== '' ? `/api/covers/game/${result.coverImageId}` : null}
                      title={result.name}
                    />
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
        </>
      )}
    </section>
  )
}
