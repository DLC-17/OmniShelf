import { useState } from 'react'
import type { FormEvent } from 'react'
import { ApiError } from '../../api/client'
import type { AlbumSearchResult } from '../../api/music'
import { useAddAlbum, useMusicSearch } from '../../hooks/useMusicSearch'

interface AddFeedback {
  text: string
  isError: boolean
}

/**
 * Search-and-add flow for albums: a MusicBrainz name search → results → Add
 * button. A duplicate add (409 already_tracked) is reported inline as "already
 * in your library" rather than as a failure. Added albums land on the shelf as
 * LISTENING; set which formats you own (Vinyl/CD) from the library detail.
 */
export default function MusicSearch() {
  const [input, setInput] = useState('')
  const [query, setQuery] = useState('')
  const [feedback, setFeedback] = useState<Record<string, AddFeedback>>({})

  const search = useMusicSearch(query)
  const addAlbum = useAddAlbum()

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setQuery(input.trim())
    setFeedback({})
  }

  const handleAdd = (mbid: string) => {
    addAlbum.mutate(
      { mbid },
      {
        onSuccess: () => {
          setFeedback((prev) => ({ ...prev, [mbid]: { text: 'Added', isError: false } }))
        },
        onError: (err) => {
          if (err instanceof ApiError && err.code === 'already_tracked') {
            setFeedback((prev) => ({
              ...prev,
              [mbid]: { text: 'Already in your library', isError: false },
            }))
            return
          }
          const text = err instanceof ApiError ? err.message : 'Something went wrong. Try again.'
          setFeedback((prev) => ({ ...prev, [mbid]: { text, isError: true } }))
        },
      },
    )
  }

  return (
    <section aria-label="Add an album">
      <h2>Add an album</h2>
      <form className="searchbar" onSubmit={handleSubmit} role="search">
        <input
          type="search"
          aria-label="Search albums"
          placeholder="Search albums by artist or title…"
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
            ? 'MusicBrainz unreachable, try again'
            : 'Search failed. Try again.'}
        </p>
      )}
      {search.data !== undefined && search.data.length === 0 && (
        <p>No albums found for “{query}”.</p>
      )}
      {search.data !== undefined && search.data.length > 0 && (
        <ul className="list">
          {search.data.map((result: AlbumSearchResult) => {
            const fb = feedback[result.mbid]
            return (
              <li key={result.mbid} className="card card-row">
                <div className="grow">
                  <strong>{result.title}</strong>
                  {result.year > 0 && <span className="muted"> ({result.year})</span>}
                  {result.artist !== '' && <p className="meta">{result.artist}</p>}
                </div>
                {fb !== undefined ? (
                  <span role={fb.isError ? 'alert' : 'status'} className={fb.isError ? 'alert' : 'muted'}>
                    {fb.text}
                  </span>
                ) : (
                  <button
                    type="button"
                    className="btn-confirm"
                    onClick={() => handleAdd(result.mbid)}
                    disabled={addAlbum.isPending}
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
