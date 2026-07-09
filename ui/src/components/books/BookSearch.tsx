import { useState } from 'react'
import type { FormEvent } from 'react'
import { ApiError } from '../../api/client'
import type { BookSearchResult } from '../../api/books'
import { useAddBookByIsbn, useBookEditions, useBookSearch } from '../../hooks/useBookSearch'

interface AddFeedback {
  text: string
  isError: boolean
}

/**
 * The ISBN picker for a chosen work: lists its ISBN-13 editions so the user can
 * add the exact one they own. Adding runs scan + track under the hood.
 */
function EditionPicker({ work }: { work: BookSearchResult }) {
  const editions = useBookEditions(work.workKey)
  const addBook = useAddBookByIsbn()
  const [feedback, setFeedback] = useState<Record<string, AddFeedback>>({})

  const handleAdd = (isbn13: string) => {
    addBook.mutate(isbn13, {
      onSuccess: () => {
        setFeedback((prev) => ({ ...prev, [isbn13]: { text: 'Added', isError: false } }))
      },
      onError: (err) => {
        if (err instanceof ApiError && err.status === 409) {
          setFeedback((prev) => ({
            ...prev,
            [isbn13]: { text: 'Already in your library', isError: false },
          }))
          return
        }
        const text = err instanceof ApiError ? err.message : 'Something went wrong. Try again.'
        setFeedback((prev) => ({ ...prev, [isbn13]: { text, isError: true } }))
      },
    })
  }

  if (editions.isFetching) return <p className="muted">Loading editions…</p>
  if (editions.isError) {
    return (
      <p role="alert" className="alert">
        {editions.error instanceof ApiError && editions.error.code === 'upstream_error'
          ? 'OpenLibrary unreachable, try again.'
          : 'Could not load editions. Try again.'}
      </p>
    )
  }
  if (editions.data !== undefined && editions.data.length === 0) {
    return <p className="muted">No editions with an ISBN are available for this title.</p>
  }
  return (
    <ul className="list">
      {editions.data?.map((edition) => {
        const fb = feedback[edition.isbn13]
        return (
          <li key={edition.isbn13} className="card card-row">
            <div className="grow">
              <strong>{edition.title !== '' ? edition.title : work.title}</strong>
              <p className="meta">
                ISBN {edition.isbn13}
                {edition.publishDate !== '' && ` · ${edition.publishDate}`}
              </p>
            </div>
            {fb !== undefined ? (
              <span role={fb.isError ? 'alert' : 'status'} className={fb.isError ? 'alert' : 'muted'}>
                {fb.text}
              </span>
            ) : (
              <button
                type="button"
                className="btn-confirm"
                onClick={() => handleAdd(edition.isbn13)}
                disabled={addBook.isPending}
                aria-label={`Add ISBN ${edition.isbn13}`}
              >
                Add
              </button>
            )}
          </li>
        )
      })}
    </ul>
  )
}

/**
 * Search-and-add flow for books by title: search box → OpenLibrary works →
 * expand a work to its editions → Add a specific ISBN. Added books land on the
 * watchlist (PLAN_TO). Books can also still be added by ISBN scan.
 */
export default function BookSearch() {
  const [input, setInput] = useState('')
  const [query, setQuery] = useState('')
  const [openWork, setOpenWork] = useState<string | null>(null)

  const search = useBookSearch(query)

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setQuery(input.trim())
    setOpenWork(null)
  }

  return (
    <section aria-label="Add a book by name">
      <h2>Add a book by name</h2>
      <form className="searchbar" onSubmit={handleSubmit} role="search">
        <input
          type="search"
          aria-label="Search books"
          placeholder="Search books…"
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
            ? 'OpenLibrary unreachable, try again'
            : 'Search failed. Try again.'}
        </p>
      )}
      {search.data !== undefined && search.data.length === 0 && (
        <p>No books found for “{query}”.</p>
      )}
      {search.data !== undefined && search.data.length > 0 && (
        <ul className="list">
          {search.data.map((work) => {
            const open = openWork === work.workKey
            return (
              <li key={work.workKey} className="card">
                <div className="card-row">
                  <div className="grow">
                    <strong>{work.title}</strong>
                    {work.firstYear !== 0 && <span className="muted"> ({work.firstYear})</span>}
                    {work.authors !== '' && <p className="meta">{work.authors}</p>}
                  </div>
                  <button
                    type="button"
                    className="btn-confirm"
                    aria-expanded={open}
                    onClick={() => setOpenWork(open ? null : work.workKey)}
                  >
                    {open ? 'Hide editions' : `Choose edition (${work.editionCount})`}
                  </button>
                </div>
                {open && <EditionPicker work={work} />}
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
